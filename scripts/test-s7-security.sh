#!/usr/bin/env bash
# S7 / T21 cross-slice security and bounded-I/O regression.
# Every case names exact tests; a name that no longer exists fails the suite
# instead of passing as "no tests to run". Attack strings stay test data.
set -uo pipefail

repository_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
cd "$repository_root" || exit 1

failures=0
printf 'suite=s7-security-v1\n'
printf 'source_revision=%s\n' "$(git rev-parse HEAD)"
printf 'source_state=%s\n' "$( [[ -z $(git status --porcelain --untracked-files=no) ]] && printf clean || printf dirty )"
printf 'go=%s\n' "$(go env GOVERSION)"

# case <requirement aliases> <package> <exact test names...>
case_group() {
  local aliases=$1 package=$2
  shift 2
  local pattern listed missing=0 name
  pattern="^($(IFS='|'; printf '%s' "$*"))\$"
  listed=$(go test "$package" -list "$pattern" 2>/dev/null | grep -E '^(Test|Fuzz)' || true)
  for name in "$@"; do
    if ! grep -qx "$name" <<<"$listed"; then
      printf 'case=%s package=%s test=%s result=missing\n' "$aliases" "$package" "$name"
      missing=1
    fi
  done
  if ((missing)); then
    failures=$((failures + 1))
    return
  fi
  if go test "$package" -run "$pattern" -count=1 >/dev/null 2>&1; then
    printf 'case=%s package=%s tests=%d result=pass\n' "$aliases" "$package" "$#"
  else
    printf 'case=%s package=%s tests=%d result=fail\n' "$aliases" "$package" "$#"
    failures=$((failures + 1))
  fi
}

case_group "MVP-SEC-01,SEC-004" ./internal/compatibility TestBackupAndExportUseSeparateExactAuthorities TestTransferRejectsUnsafeTargetsAndStaleSource
case_group "MVP-SEC-01,MVP-SEC-09" ./internal/local TestArtifactCleanupRequiresExactAuthorityAndRecordsBoundedAudit TestArtifactCleanupStaleReferenceAndConcurrentWriterDenyAuthority TestGuidedRecoveryRejectsStaleAuthorityAndConcurrentWriter
case_group "MVP-SEC-01,MVP-SEC-05" ./internal/install TestUpgradeRefusalsHaveZeroEffects TestUpgradeStaleAuthorityAndSpaceHaveZeroEffects TestLoadCandidateMirrorsInstallerVerification TestUpgradeSkillStaleAuthorityAndConcurrentInstallHaveZeroEffects
case_group "MVP-SEC-01,SEC-002" ./internal/projectapp TestAuthorityDenialCallsNoStore TestLocalAuthorityCannotAuthorizePortableAndViceVersa TestDeniedEffectsAndCancelledGate
case_group "MVP-SEC-01,MVP-SEC-04" ./internal/workitem TestCreateRequiresExactPreviewAuthorityAndPersistsConfirmedIssue TestSelectRejectsStaleLocalRevision TestPrepareCancellationAndCapabilityMismatchHaveZeroEffects
case_group "MVP-SEC-01,MVP-SEC-04" ./internal/workflow TestRecordFactAuthorityAndBlockedTransitionHaveZeroUnauthorizedEffects TestS4ChangedIssueStateInvalidatesProjectionAuthority TestS4ConfirmedProviderEffectThenBookkeepingFailureIsPartial TestReconciliationContradictionsStayReadOnlyAndRequireHumanDecision TestMissingLocalStateNeverSynthesizesExecution
case_group "MVP-SEC-02,SEC-001" ./internal/manifest TestStructuralSecretAndLocalStateExclusion TestURLSensitiveParameterPolicyV1
case_group "MVP-SEC-02,SEC-001" ./internal/detailartifact TestArtifactBoundsAndSanitization TestArtifactRejectsSensitiveReferenceMetadataOnCreateAndDecode TestArtifactRejectsCredentialURLsInReferenceMetadataOnCreateAndDecode
case_group "MVP-SEC-02,MVP-SEC-03" ./internal/workitem TestPrepareRejectsSensitiveOversizedAndInvalidTextWithoutProviderEffects TestPrepareCompleteDraftIsDeterministicReadOnlyAndPreservesAuthorship
case_group "MVP-SEC-02,MVP-SEC-03,MVP-NFR-02" ./internal/cli TestRunDoesNotExposeRejectedInput TestRunDoesNotExposeRejectedCommand TestCompletionOutputIsBounded TestWorkItemPreviewWithWorstCaseEscapingRemainsBounded
case_group "MVP-SEC-02,MVP-SEC-07" ./internal/compatibility TestClassificationMatrix TestRecognizedPOCFromHistoricalBinaryOutput
case_group "MVP-SEC-03,SEC-003" ./internal/githubissues TestRenderSeparatesAuthorshipAndNeutralizesMarkdownStructure TestAdapterUsesBoundedAPICommandsAndStdinForUntrustedBody
case_group "MVP-SEC-03,MVP-SEC-05" ./cmd/lingo TestExecutableMaintenanceInputIsStrictAndSafe TestExecutableRejectsSingleHyphenSelectorFlagsBeforeEffects TestWorkflowSelectorAmbiguityFailsBeforeFallbackOrEffects
case_group "MVP-SEC-04,MVP-NFR-03" ./internal/githubissues TestAdapterBoundsTimeoutAndOutput TestAdapterStrictlyRejectsMismatchedResponseAndStructuredRateLimit TestAdapterCreateTreatsUnknownOrInvalidSuccessResponseAsAmbiguous TestAdapterUnknownCLIErrorIsNotRetryable
case_group "MVP-SEC-06,MVP-SEC-08,SEC-005" ./internal/compatibility TestUnsafeFilesystemFactsFailClosed TestInspectRejectsUnsafeRootsWithoutEffects
case_group "MVP-SEC-06,MVP-SEC-08,SEC-005" ./internal/codexruntime TestInstallRejectsSymlinkSkill TestInspectRejectsHardLinkedOwnedSkill TestInstallRejectsRootWithExtendedACL TestInspectRejectsUnsafeSkillPermissions
case_group "MVP-SEC-06,MVP-SEC-08,SEC-005" ./cmd/lingo TestInstallRejectsSymlinkRecordWithoutWritingOutside TestInstallRejectsHardLinkedRecordWithoutWritingOutside TestCompositionRejectsSymlinkAliasBeforeCreation TestCompositionRejectsOverlappingRootsBeforeCreation
case_group "MVP-SEC-07,FR-030" ./internal/compatibility TestBackupAndExportUseSeparateExactAuthorities
case_group "MVP-SEC-08,MVP-SEC-09,FR-025" ./internal/local TestGuidedRecoveryCoversRecognizedFileFaultStages TestGuidedRecoveryPreservesAmbiguousCorruptAndUnknownState TestGuidedRecoveryDirectoryPublications TestRecoveryAttemptLeftoversArePreservedForReview TestArtifactCleanupFailsClosedOnUncertainReferenceState
case_group "MVP-SEC-09,FR-033" ./internal/local TestArtifactCleanupEligibilityMatrixUsesAuthoritativeReferences TestCapacityExhaustionNeverEvictsAndExplicitCleanupRestoresCapacity TestCleanupRecordRetentionAndBatchBounds TestArtifactCleanupPartialRemovalIsTruthfulAndRecorded TestRetentionEligibilityMatrix30_365_90 TestRetirementDeniedWhileReferenced TestRetirementStaleAuthorityAndDuplicateAreDenied TestRetirementCorruptStaleOrUnsafeRecordPreservesEvidence TestRetirementNamespaceFailsClosed TestReReferenceSupersedesRetirementAndRequiresNewRetirement TestRetirementChangedAfterCleanupReviewDeniesAuthority TestRetirementPublicationInterruptionRequiresRecovery TestRetirementRecordIsInventoried
case_group "MVP-NFR-01" ./internal/completion TestStatusEffectClassificationMatrix TestClassificationRejectsAmbiguousOrFalseSuccess
case_group "MVP-NFR-03" ./internal/manifest TestByteDepthAndNodeLimits TestParserContractStreamAndHostileBounds TestBoundedOutputWriter
case_group "MVP-NFR-03" ./internal/local TestInventoryEntryBoundFailsClosed
case_group "MVP-NFR-04,MVP-SEC-06" ./internal/compatibility TestTransferCapacityAndInterruptionPreserveSource
case_group "SEC-004,FR-023" ./internal/install TestUpgradeInterruptionAfterBinaryIsPartialAndResumable TestUpgradeResumeAfterAllEffectsFinalizesMarker TestUpgradeInterruptionAtEachOrderedEffectResumes TestUpgradeResumeRefusesSkillChangedAfterInterruption
case_group "SEC-004,FR-023" ./internal/workitem TestConfirmedProviderEffectAndLocalFailureIsPartial
case_group "FR-036,MVP-SEC-06" ./internal/codexruntime TestHistoricalPOCSkillSetIsKnownLegacy TestInstallRefusesConflictAndRollsBackCurrentAttempt TestPublishUpgradeSkillRequiresExpectedRevision TestInspectUpgradeReportsLeftoversAndRefusesUnknownEntries
case_group "FR-036,MVP-SEC-06,SEC-005" ./internal/install TestUpgradePublishesOwnedSkillFilesAfterBinaryAndReceipt TestUpgradeReplacesKnownLegacySkillsAndCreatesMissingOnes TestUpgradeSkillConflictsHaveZeroEffects

printf 'case=race-s7\n'
if go test -race ./internal/compatibility ./internal/install ./internal/local ./internal/detailartifact ./internal/codexruntime -count=1 >/dev/null 2>&1; then
  printf 'race=pass\n'
else
  printf 'race=fail\n'
  failures=$((failures + 1))
fi

if ./scripts/check-sensitive-files.sh . >/dev/null 2>&1; then
  printf 'sensitive_files=pass\n'
else
  printf 'sensitive_files=fail\n'
  failures=$((failures + 1))
fi

if command -v gitleaks >/dev/null 2>&1; then
  if gitleaks detect --source . --no-git --no-banner --redact >/dev/null 2>&1; then
    printf 'gitleaks_worktree=pass\n'
  else
    printf 'gitleaks_worktree=fail\n'
    failures=$((failures + 1))
  fi
else
  printf 'gitleaks=unavailable\n'
fi

printf 'failures=%d\n' "$failures"
if ((failures)); then
  printf 'result=fail\n'
  exit 1
fi
printf 'result=pass\n'
