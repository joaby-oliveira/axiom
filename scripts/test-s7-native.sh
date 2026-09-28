#!/usr/bin/env bash
# S7 / T22 exact-target native filesystem, install, and upgrade Evidence.
# Runs only on an approved release row and only against isolated temporary
# roots. Unsupported rows exit 78 and are recorded as blocked, never passed.
set -uo pipefail
umask 077

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
temporary=$(mktemp -d)
temporary=$(cd "$temporary" && pwd -P)
trap 'rm -rf -- "$temporary"' EXIT
failures=0

step() {
  local label=$1
  shift
  if "$@" >"$temporary/step.log" 2>&1; then
    printf 'step=%s result=pass\n' "$label"
  else
    printf 'step=%s result=fail\n' "$label"
    sed -n '1,20p' "$temporary/step.log" | sed 's/^/  log: /'
    failures=$((failures + 1))
  fi
}

# tests <label> <package> <exact names...>: fails when a name no longer exists.
tests() {
  local label=$1 package=$2
  shift 2
  local pattern listed name
  pattern="^($(IFS='|'; printf '%s' "$*"))\$"
  listed=$(cd "$repository_root" && go test "$package" -list "$pattern" 2>/dev/null | grep -E '^Test' || true)
  for name in "$@"; do
    if ! grep -qx "$name" <<<"$listed"; then
      printf 'step=%s result=missing_test test=%s\n' "$label" "$name"
      failures=$((failures + 1))
      return
    fi
  done
  step "$label" bash -c "cd '$repository_root' && go test '$package' -run '$pattern' -count=1"
}

system=$(uname -s)
architecture=$(uname -m)
printf 'suite=s7-native-v1\n'
printf 'source_revision=%s\n' "$(git -C "$repository_root" rev-parse HEAD)"
printf 'source_state=%s\n' "$( [[ -z $(git -C "$repository_root" status --porcelain --untracked-files=no) ]] && printf clean || printf dirty )"
printf 'untracked_entries=%s\n' "$(git -C "$repository_root" status --porcelain | grep -c '^??' || true)"
printf 'go=%s\n' "$(go env GOVERSION)"
printf 'system=%s\narchitecture=%s\nkernel=%s\n' "$system" "$architecture" "$(uname -r)"

row=
case "$system:$architecture" in
  Darwin:arm64)
    os_version=$(sw_vers -productVersion 2>/dev/null || true)
    printf 'os_version=%s\nos_build=%s\n' "$os_version" "$(sw_vers -buildVersion 2>/dev/null || true)"
    device=$(df "$temporary" | awk 'NR == 2 {print $1}')
    filesystem=$(diskutil info "$device" 2>/dev/null | awk -F: '$1 ~ /File System Personality/ {gsub(/^[ \t]+/, "", $2); print $2}')
    printf 'filesystem=%s\n' "$filesystem"
    [[ "$os_version" == 27.0 && "$filesystem" == APFS ]] && row=macos-27.0-arm64-apfs
    ;;
  Linux:x86_64|Linux:aarch64)
    os_id=$(awk -F= '$1 == "ID" {gsub(/"/, "", $2); print $2}' /etc/os-release 2>/dev/null)
    os_version=$(awk -F= '$1 == "VERSION_ID" {gsub(/"/, "", $2); print $2}' /etc/os-release 2>/dev/null)
    filesystem=$(findmnt -n -o FSTYPE --target "$temporary" 2>/dev/null || stat -f -c %T "$temporary")
    printf 'os_id=%s\nos_version=%s\nfilesystem=%s\n' "$os_id" "$os_version" "$filesystem"
    goarch=amd64
    [[ "$architecture" == aarch64 ]] && goarch=arm64
    [[ "$os_id:$os_version:$filesystem" == ubuntu:26.04:ext4 ]] && row=ubuntu-26.04-${goarch}-ext4
    ;;
esac
if [[ -z "$row" ]]; then
  printf 'native_row=unsupported\nresult=blocked\n'
  exit 78
fi
printf 'native_row=%s\n' "$row"

mkdir "$temporary/probe"
printf x >"$temporary/probe/AxiomCase"
case_behavior=sensitive
[[ -e "$temporary/probe/axiomcase" ]] && case_behavior=insensitive
printf 'case_behavior=%s\n' "$case_behavior"
if [[ "$row" == macos-* && "$case_behavior" != insensitive ]]; then
  printf 'result=blocked_case_sensitive_apfs\n'
  exit 78
fi
if ln "$temporary/probe/AxiomCase" "$temporary/probe/hardlink" 2>/dev/null; then
  printf 'hardlink_primitive=available\n'
else
  printf 'hardlink_primitive=unavailable\n'
fi

tests f0-f8-publication ./internal/local TestExecutionStoreFailsClosedAcrossF0F8 TestWorkItemStoreRequiresExpectedRevisionAndFailsClosedAcrossF0F8 TestArtifactStoreFaultStagesPreserveCommitTruthAndFailClosed
tests two-process-barriers ./internal/local TestExecutionStoreTwoProcessBarrierHasOneWinningRevision TestExecutionStoreProcessCrashBoundariesFailClosed TestArtifactStoreCoordinatesProcessesAndPreservesInterruptedState TestPortableLockSerializesProcessesAndSurvivesCrash TestPortableReadersAndWritersConflictWithOtherProcess TestInstallationCrashBoundaryRequiresRecovery
tests traversal-link-replacement ./internal/local TestArtifactStoreCreateReadAndConfinement TestArtifactStoreRejectsDigestModeLinkAndTypeChanges TestPortableStoreRejectsUserSymlinkAncestor TestPortableStoreRejectsHardLinkedManifest TestPortableCreateRejectsAncestorReplacementBeforePublication TestInstallationRejectsTargetReplacementBeforePublication TestInstallationRejectsStagedHardLinkBeforePublication
if [[ "$system" == Darwin ]]; then
  tests ownership-mode-acl ./internal/local TestPrivateRootRejectsPermissiveACLDespiteMode0700 TestPrivateFileRejectsPermissiveACL TestPortableUpdatePermissionFailurePreservesOldBytes
else
  tests ownership-mode-acl ./internal/local TestPortableUpdatePermissionFailurePreservesOldBytes
fi
tests artifact-capacity-cleanup ./internal/local TestArtifactStoreCapacityAndCollisionFailWithoutEviction TestCapacityExhaustionNeverEvictsAndExplicitCleanupRestoresCapacity TestArtifactCleanupEligibilityMatrixUsesAuthoritativeReferences TestArtifactCleanupRequiresExactAuthorityAndRecordsBoundedAudit TestArtifactCleanupStaleReferenceAndConcurrentWriterDenyAuthority TestArtifactCleanupPartialRemovalIsTruthfulAndRecorded TestCleanupRecordRetentionAndBatchBounds TestRetentionEligibilityMatrix30_365_90 TestRetirementDeniedWhileReferenced TestRetirementStaleAuthorityAndDuplicateAreDenied TestRetirementCorruptStaleOrUnsafeRecordPreservesEvidence TestRetirementNamespaceFailsClosed TestReReferenceSupersedesRetirementAndRequiresNewRetirement TestRetirementChangedAfterCleanupReviewDeniesAuthority TestRetirementPublicationInterruptionRequiresRecovery TestRetirementRecordIsInventoried
tests guided-recovery ./internal/local TestGuidedRecoveryCoversRecognizedFileFaultStages TestGuidedRecoveryStageRemovedMarkerLeftRestoresPrior TestGuidedRecoveryPreservesAmbiguousCorruptAndUnknownState TestGuidedRecoveryRejectsStaleAuthorityAndConcurrentWriter TestGuidedRecoveryDirectoryPublications TestRecoveryAttemptLeftoversArePreservedForReview
tests compatibility-transfer ./internal/compatibility TestRecognizedPOCFromHistoricalBinaryOutput TestClassificationMatrix TestUnsafeFilesystemFactsFailClosed TestBackupAndExportUseSeparateExactAuthorities TestTransferRejectsUnsafeTargetsAndStaleSource TestTransferCapacityAndInterruptionPreserveSource
tests upgrade-units ./internal/install TestLoadCandidateMirrorsInstallerVerification TestUpgradeOrdersConfirmedEffectsAndPreservesInstalledAt TestUpgradeInterruptionAfterBinaryIsPartialAndResumable TestUpgradeResumeAfterAllEffectsFinalizesMarker TestUpgradeRefusalsHaveZeroEffects TestUpgradePublishesOwnedSkillFilesAfterBinaryAndReceipt TestUpgradeInterruptionAtEachOrderedEffectResumes TestUpgradeSkillConflictsHaveZeroEffects
step race bash -c "cd '$repository_root' && go test -race ./internal/compatibility ./internal/install ./internal/local -count=1"
step release-archive-suite "$repository_root/scripts/test-release-archives.sh"

# Real archives through the published installer, then the owned upgrade path.
platform_bundle=
case "$row" in
  macos-*) platform_bundle=macos-27-arm64 ;;
  ubuntu-26.04-amd64-*) platform_bundle=ubuntu-26.04-amd64 ;;
  ubuntu-26.04-arm64-*) platform_bundle=ubuntu-26.04-arm64 ;;
esac
build_flag=()
[[ -n $(git -C "$repository_root" status --porcelain --untracked-files=normal) ]] && build_flag=(--development)
printf 'archive_kind=%s\n' "$( ((${#build_flag[@]})) && printf development || printf release )"
step build-archive-1.0.0 "$repository_root/scripts/build-release-archives.sh" --version 1.0.0 --output "$temporary/r100" ${build_flag[@]+"${build_flag[@]}"}
step build-archive-1.1.0 "$repository_root/scripts/build-release-archives.sh" --version 1.1.0 --output "$temporary/r110" ${build_flag[@]+"${build_flag[@]}"}
old_archive="$temporary/r100/axiom-1.0.0-$platform_bundle.tar.gz"
new_archive="$temporary/r110/axiom-1.1.0-$platform_bundle.tar.gz"
bin="$temporary/install/bin"
receipts="$temporary/install/receipts"
mkdir -p "$temporary/install" && chmod 700 "$temporary/install"
mkdir -p "$temporary/extract" && tar -xzf "$old_archive" -C "$temporary/extract"
installer=$(find "$temporary/extract" -name install.sh -type f | head -1)
step clean-install "$installer" --archive "$old_archive" --checksums "$temporary/r100/SHA256SUMS" --bin-dir "$bin" --receipt-dir "$receipts"
step equivalent-reinstall-unchanged bash -c "'$installer' --archive '$old_archive' --checksums '$temporary/r100/SHA256SUMS' --bin-dir '$bin' --receipt-dir '$receipts' | grep -qx 'install_status=unchanged'"
step installer-refuses-owned-upgrade bash -c "! '$installer' --archive '$new_archive' --checksums '$temporary/r110/SHA256SUMS' --bin-dir '$bin' --receipt-dir '$receipts'"
export LINGO_PROJECTS_ROOT="$temporary/roots/projects" LINGO_STATE_ROOT="$temporary/roots/state" AXIOM_CODEX_SKILLS_ROOT="$temporary/roots/skills"
mkdir -p "$temporary/roots" && chmod 700 "$temporary/roots"
upgrade_args=(upgrade --archive "$new_archive" --checksums "$temporary/r110/SHA256SUMS" --bin-dir "$bin" --receipt-dir "$receipts")
"$bin/axiom" --json "${upgrade_args[@]}" >"$temporary/preview.json" 2>/dev/null
digest=$(sed -n 's/.*"references":\["upgrade:\([0-9a-f]\{64\}\)"\].*/\1/p' "$temporary/preview.json")
old_binary_sha=$(shasum -a 256 "$bin/axiom" | awk '{print $1}')
step upgrade-preview-read-only bash -c "[[ -n '$digest' ]] && grep -q '\"status\":\"success\"' '$temporary/preview.json' && grep -qx 'version=1.0.0' '$receipts/installation.receipt' && [[ \$(shasum -a 256 '$bin/axiom' | awk '{print \$1}') == '$old_binary_sha' ]]"
step upgrade-stale-digest-denied bash -c "'$bin/axiom' --json ${upgrade_args[*]} --preview-digest $(printf '0%.0s' {1..64}) --authorize-local | grep -q '\"status\":\"denied_authority\"'"
step upgrade-apply bash -c "'$bin/axiom' --json ${upgrade_args[*]} --preview-digest '$digest' --authorize-local | grep -q '\"status\":\"success\"'"
step upgraded-version bash -c "grep -qx 'version=1.1.0' '$receipts/installation.receipt' && grep -qx \"sha256=\$(shasum -a 256 '$bin/axiom' | awk '{print \$1}')\" '$receipts/installation.receipt' && ! grep -qx 'sha256=$old_binary_sha' '$receipts/installation.receipt'"
step upgrade-equivalent-no-op bash -c "'$bin/axiom' --json ${upgrade_args[*]} | grep -q 'Installation already matches the candidate'"
step installer-accepts-upgraded-receipt bash -c "'$installer' --archive '$new_archive' --checksums '$temporary/r110/SHA256SUMS' --bin-dir '$bin' --receipt-dir '$receipts' | grep -qx 'install_status=unchanged'"
step downgrade-refused bash -c "'$bin/axiom' --json upgrade --archive '$old_archive' --checksums '$temporary/r100/SHA256SUMS' --bin-dir '$bin' --receipt-dir '$receipts' | grep -q 'downgrade_refused'"

# Partial resume on the native filesystem: binary committed, receipt pending.
bin2="$temporary/install2/bin"
receipts2="$temporary/install2/receipts"
mkdir -p "$temporary/install2" && chmod 700 "$temporary/install2"
step resume-install-1.0.0 "$installer" --archive "$old_archive" --checksums "$temporary/r100/SHA256SUMS" --bin-dir "$bin2" --receipt-dir "$receipts2"
new_sha=$(awk '{print $1}' <(grep " $(basename "$new_archive")\$" "$temporary/r110/SHA256SUMS"))
cp "$bin/axiom" "$bin2/.axiom-binary-stage.native" && chmod 700 "$bin2/.axiom-binary-stage.native" && mv "$bin2/.axiom-binary-stage.native" "$bin2/axiom"
printf 'formatVersion=1\nstage=binary_committed\narchiveSha256=%s\noperation=upgrade\n' "$new_sha" >"$receipts2/.axiom-install-operation"
chmod 600 "$receipts2/.axiom-install-operation"
step installer-refuses-interrupted-upgrade bash -c "! '$installer' --archive '$old_archive' --checksums '$temporary/r100/SHA256SUMS' --bin-dir '$bin2' --receipt-dir '$receipts2'"
resume_args=(upgrade --archive "$new_archive" --checksums "$temporary/r110/SHA256SUMS" --bin-dir "$bin2" --receipt-dir "$receipts2")
"$bin2/axiom" --json "${resume_args[@]}" >"$temporary/resume.json" 2>/dev/null
resume_digest=$(sed -n 's/.*"references":\["upgrade:\([0-9a-f]\{64\}\)"\].*/\1/p' "$temporary/resume.json")
step resume-preview-receipt-only bash -c "grep -q '\"resume\":true' '$temporary/resume.json' && [[ \$(grep -o '\"kind\":\"[a-z]*\"' '$temporary/resume.json' | sort -u | tr '\n' ' ') == '\"kind\":\"receipt\" ' ]]"
step resume-apply bash -c "'$bin2/axiom' --json ${resume_args[*]} --preview-digest '$resume_digest' --authorize-local | grep -q '\"status\":\"success\"'"
step resume-marker-cleared bash -c "[[ ! -e '$receipts2/.axiom-install-operation' ]]"

# Native skill publication: a known-legacy (historical POC) Axiom skill set is
# replaced by the candidate's verified skill files inside the upgrade itself.
bin3="$temporary/install3/bin"
receipts3="$temporary/install3/receipts"
skills3="$temporary/install3/skills"
mkdir -p "$temporary/install3" && chmod 700 "$temporary/install3"
step skills-install-1.0.0 "$installer" --archive "$old_archive" --checksums "$temporary/r100/SHA256SUMS" --bin-dir "$bin3" --receipt-dir "$receipts3"
mkdir -m 700 "$skills3"
for skill in "$repository_root"/internal/compatibility/testdata/poc-v0.1.0-poc.1/skills/*; do
  mkdir -m 700 "$skills3/$(basename "$skill")"
  cp "$skill/SKILL.md" "$skills3/$(basename "$skill")/SKILL.md" && chmod 600 "$skills3/$(basename "$skill")/SKILL.md"
done
skill_args=(upgrade --archive "$new_archive" --checksums "$temporary/r110/SHA256SUMS" --bin-dir "$bin3" --receipt-dir "$receipts3")
AXIOM_CODEX_SKILLS_ROOT="$skills3" "$bin3/axiom" --json "${skill_args[@]}" >"$temporary/skills.json" 2>/dev/null
skill_digest=$(sed -n 's/.*"references":\["upgrade:\([0-9a-f]\{64\}\)"\].*/\1/p' "$temporary/skills.json")
step skills-preview-five-effects bash -c "[[ \$(grep -o '\"kind\":\"skill\"' '$temporary/skills.json' | wc -l | tr -d ' ') == 5 ]]"
step skills-apply-partial-receipt-refresh bash -c "AXIOM_CODEX_SKILLS_ROOT='$skills3' '$bin3/axiom' --json ${skill_args[*]} --preview-digest '$skill_digest' --authorize-local | grep -q '\"skillReceipt\":\"refresh_required\"'"
mkdir -p "$temporary/extract110" && tar -xzf "$new_archive" -C "$temporary/extract110"
step skills-published-match-candidate bash -c "for skill in '$temporary'/extract110/*/skills/*; do cmp -s \"\$skill/SKILL.md\" '$skills3/'\$(basename \"\$skill\")/SKILL.md || exit 1; done"
step skills-no-staging-or-marker bash -c "[[ -z \$(find '$skills3' -name '.axiom-upgrade-skill.*') && ! -e '$receipts3/.axiom-install-operation' ]]"
step skills-receipt-refresh-by-upgraded-binary bash -c "AXIOM_CODEX_SKILLS_ROOT='$skills3' '$bin3/axiom' --json runtime codex install | grep -q '\"status\":\"success\"' && AXIOM_CODEX_SKILLS_ROOT='$skills3' '$bin3/axiom' --json runtime codex status | grep -q '\"status\":\"success\",\"result\":\"Lingo and Codex skills are compatible\"'"

printf 'failures=%d\n' "$failures"
if ((failures)); then
  printf 'result=fail\n'
  exit 1
fi
printf 'result=pass\n'
