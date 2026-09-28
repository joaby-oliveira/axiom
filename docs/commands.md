# Project Commands

Execute estes comandos na raiz do repositório.

## Lingo Project lifecycle

O lifecycle portátil básico é local. Comandos separados de Runtime/Work Item
podem acessar Codex user-global e GitHub quando explicitamente invocados. Use um
root temporário/isolado enquanto avalia; `LINGO_PROJECTS_ROOT` deve ser absoluto.
Sem override, Lingo usa `~/.axiom/projects` para configuração portátil. Estado
local usa `$XDG_STATE_HOME/lingo` no Linux quando absoluto, caso contrário
`$HOME/.local/state/lingo`; no macOS, usa
`$HOME/Library/Application Support/Lingo`. `LINGO_STATE_ROOT` substitui esse
destino para desenvolvimento/testes e deve ser absoluto, não vazio e separado
do root portátil.

```bash
export LINGO_PROJECTS_ROOT="$(mktemp -d)"
export LINGO_STATE_ROOT="$(mktemp -d)"

go run ./cmd/lingo project init --slug sample --name "Sample"
go run ./cmd/lingo project validate --slug sample
go run ./cmd/lingo project reopen --slug sample
go run ./cmd/lingo project install --source "$LINGO_PROJECTS_ROOT/sample"
go run ./cmd/lingo project update --slug sample --name "Sample renamed"
```

Cada operação escreve resumo humano por padrão; prefixe o comando com `--json`
para evento estruturado. `version`, `first-run`, `project configure`,
`project validate` e `project show` usam o contrato canônico de completion;
demais comandos POC preservam temporariamente o evento histórico. Exit codes:
`0` sucesso, `1` erro/conflito, `2`
interrupção/cancelamento. `init` é create/no-op/conflito: não
renomeia nem atualiza um Project existente; use `update` para alterar o nome.

`install` grava o record local estrito em `<state-root>/projects/<project-id>/installation.json`;
não altera o manifest de origem. O store portátil básico aceita somente Project
mínimo de um único `axiom.yaml` e recusa symlinks, artefatos extras e roots
relativos. Bindings, Runtime Codex, GitHub Work Items e workflow usam
stores/adapters separados; rename e documentos opcionais completos continuam
fora do POC.

Execute o dogfooding reproduzível deste incremento em roots temporários:

```bash
./scripts/dogfood-poc.sh
```

O script exige Go 1.26, Bash, `find`, `wc`, `tr`, `grep`, `sed`, `awk` e
`shasum`. Ele instala Lingo em um PATH isolado, inicia fora do Project, instala e
verifica as cinco skills, configura/resolve Project, usa um Provider GitHub fake
limitado, executa todos os gates, cobre interrupção/retomada e completa o Work
Item somente com authority explícita. A saída final é Evidence JSON versionada
com hashes SHA-256. Instalação usa publicação sem substituição; resíduos de
tentativas interrompidas retornam `recovery_required` e exigem inspeção humana.
No macOS, builds com e sem cgo inspecionam ACLs no objeto aberto. O caminho sem
cgo usa `fgetattrlist` e confirma suporte do volume a extended security antes de
aceitar ausência de ACL; resposta incompleta ou estado indeterminado falha
fechado.
Consulte o [procedimento manual de recovery](specifications/002-lingo-project-initialization/recovery-poc.md)
antes de mover qualquer artefato. A matriz macOS/Linux executa os mesmos checks
em [POC verification](../.github/workflows/poc-verification.yml).

## T01–T04 validation

Go 1.26 instalado. T01/T02/T04 usam biblioteca padrão; T03 usa o parser fixado
em `go.mod`/`go.sum`. O store POC usa `golang.org/x/sys/unix@v0.44.0` em
Linux/macOS. Em cache novo, preparação explícita com rede:

```bash
GOTOOLCHAIN=local go mod download go.yaml.in/yaml/v3@v3.0.5
GOTOOLCHAIN=local go mod download golang.org/x/sys@v0.44.0
```

Depois, os checks abaixo rodam offline. Build valida pacotes e o executável Lingo POC.

```bash
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
go test -cover ./...
go test -race -shuffle=on -count=10 ./...
go vet ./...
go build ./...
go mod verify
go test -fuzz=FuzzRecordRoundTrip -fuzztime=20s -parallel=2 ./internal/local
go test -fuzz=FuzzDecodeSafeRoundTrip -fuzztime=30s -parallel=2 ./internal/manifest
go test -run '^$' -bench=BenchmarkHostileBounds -benchtime=1x -benchmem ./internal/manifest
go run ./scripts/check-project-domain.go
bash scripts/test-check-project-domain.sh
go run ./scripts/check-projectapp.go
bash scripts/test-check-projectapp.sh
```

O checker AST inspeciona imports/símbolos puros e helpers dos testes fora da suíte
de domínio. O runner Go e ferramentas de verificação fazem I/O de compilação e
relatório; comportamento de domínio/testes não faz I/O de aplicação.
[Evidence e limites de T01](specifications/002-lingo-project-initialization/evidence-t01.md) e
[T02](specifications/002-lingo-project-initialization/evidence-t02.md). Checker de aplicação
bloqueia imports externos à fronteira e I/O ambiente, inclusive nos testes.
Spies/barreiras em memória provam contratos; não provam protocolo de filesystem.
[T03 Evidence](specifications/002-lingo-project-initialization/evidence-t03.md) detalha
parser, schema, limites, security policy v1 e cobertura. Teste AST do codec verifica
imports de produção; fixtures são sintéticas.
[T04 Evidence](specifications/002-lingo-project-initialization/evidence-t04.md) registra
JSON estrito, metadados locais, ausência versus corrupção e testes de fronteira.
`go test ./internal/local` inclui matrizes, round-trips, inspeção estática do DTO/imports
e integração com fake do port T02. Além do lifecycle portátil, este POC inclui
`lingo project install`, que persiste `installation.json` em
`<state-root>/projects/<project-id>/installation.json`; `reopen` reconhece esse
estado local quando disponível. Configuração portátil e estado local permanecem
separados. Credenciais continuam externas no `gh`; recuperação automatizada e a
matriz completa de falhas de filesystem continuam fora da evidência concluída.
#19/#20 seguem abertos e #21 está pronta para aceite humano.

## Harness validation

Valide o bootstrap completo:

```bash
./scripts/validate-repository.sh .
```

Valide somente a estrutura do harness:

```bash
./scripts/validate-agent-package.sh .
```

Teste o comportamento do validador de pacotes, inclusive rejeição de symlinks e arquivos sensíveis:

```bash
./scripts/test-validate-agent-package.sh
```

## Security validation

Teste o checker local:

```bash
./scripts/test-check-sensitive-files.sh
```

Escaneie arquivos versionados e não ignorados no worktree:

```bash
./scripts/check-sensitive-files.sh .
```

Escaneie exatamente paths e conteúdo staged:

```bash
./scripts/check-sensitive-files.sh --staged .
```

Quando `gitleaks` estiver disponível:

```bash
gitleaks detect --source . --no-git
```

## Git review

```bash
git status --short
git diff
git diff --cached
git diff --check
git remote -v
git branch --show-current
```

Não use Makefile como interface principal. Este repositório não possui Makefile.

## Install the E2E POC locally

Install Axiom from the current checkout. The default user binary destination is
`$HOME/.local/bin`. The installer reports `pathConfigured: false` and prints a
shell-safe export when that directory is absent from `PATH`; it does not mutate
shell profiles:

```bash
./scripts/install-axiom.sh
export PATH="$HOME/.local/bin:$PATH"
command -v lingo
lingo version
lingo --json version
```

For an isolated or custom user destination:

```bash
AXIOM_BIN_DIR=/absolute/path/to/bin \
AXIOM_INSTALL_STATE_ROOT=/absolute/path/to/state \
./scripts/install-axiom.sh
export PATH="/absolute/path/to/bin:$PATH"
```

The installer is safe to rerun. It replaces only a prior binary whose exact
checksum matches its protected receipt; an unrelated or modified destination is
refused. `lingo version` reports `development` for source builds plus short revision
or `unavailable` and source state `clean`, `dirty`, or `unknown`; it never invents
a release version. The receipt retains the full source commit and dirty flag. Test
missing-PATH guidance, dirty metadata,
installation and unrelated-CWD invocation with:

```bash
./scripts/test-install-axiom.sh
```

## Configure Codex Runtime

Install and inspect the global thin Axiom skills:

```bash
lingo runtime codex install
lingo runtime codex status
lingo first-run
```

Codex standalone skill names accept lowercase letters, digits and hyphens, so
the requested semantic `axiom:<skill>` names are invoked as:

```text
$axiom-project-configure
$axiom-project-show
$axiom-work-item-create
$axiom-work-item-run
$axiom-work-item-status
```

The default user-global root is `$HOME/.agents/skills`. For isolated validation:

```bash
AXIOM_CODEX_SKILLS_ROOT=/absolute/test/root lingo runtime codex install
./scripts/test-codex-skills.sh
```

Known prior Axiom skill content is upgraded atomically. Changed or unrelated
content remains a conflict and is never overwritten. `first-run` reports binary
compatibility plus the exact digest/state of each of the five skills, then directs
the user to explicit Project setup. It does not invoke Codex or infer a Project.

## Validate the Runtime Profile store

Inspect the isolated S8 Runtime Profile configuration without creating, writing,
or repairing anything:

```bash
lingo runtime profile validate
lingo --json runtime profile validate
```

The command takes no flags; any extra argument is rejected before the store is
read. It loads `local.RuntimeProfileStore` from the absolute `LINGO_STATE_ROOT`
and calls `runtimeprofile.Validate` against the published configuration. It is
strictly read-only: it never creates or repairs state, and it never invokes or
authenticates a Runtime/model adapter. It does not perform capability discovery;
it validates only the shape of the persisted configuration.

A published, well-formed configuration returns the canonical `success`
completion. A missing store or missing published configuration, and a present
but structurally or semantically invalid configuration, both fail closed and
return the canonical `validation_failure` completion with a sanitized category;
neither case creates state or exposes internal error detail. Human output
prints the concise status by default; prefix the command with `--json` for the
structured event, as with other Lingo commands.

## Build and install exact-version S2 archives

A release build requires a clean checkout, an exact semantic version, and an
absolute output directory. It emits three checksummed archives plus
`SHA256SUMS`:

```bash
./scripts/build-release-archives.sh \
  --version 0.1.0 \
  --output /absolute/release
```

Supported archive rows are the exact approved baselines macOS 27.0/arm64,
Ubuntu 26.04/amd64, and Ubuntu 26.04/arm64. Other macOS versions, Linux
distributions, Ubuntu versions, and architectures fail closed. Install the
archive matching the current host into explicit user-owned destinations:

```bash
./scripts/install-release.sh \
  --archive /absolute/release/axiom-0.1.0-macos-27-arm64.tar.gz \
  --checksums /absolute/release/SHA256SUMS \
  --bin-dir /absolute/user-owned/bin \
  --receipt-dir /absolute/user-owned/state
```

The install is checksum-first. Existing binary and receipt roots must be owned by
the current user, mode `0700`, and free of extended ACLs; unsafe roots are
preserved, not repaired. The closed receipt includes an RFC 3339 UTC
`installedAt` value created for the successful installation generation and
preserved on equivalent reinstall. Exact owned reinstall is a no-op. Platform,
ownership, permission, ACL, link, type, schema, and content conflicts fail closed.
The installer never edits shell profiles or `PATH`. It deliberately refuses
version upgrades; use the owned upgrade path in
[Compatibility, cleanup, recovery, and upgrade](#compatibility-cleanup-recovery-and-upgrade).

Validate archive structure, clean install, no-op, conflicts, interruption, and
recovery markers with:

```bash
./scripts/test-release-archives.sh
```

## CLI output and help

Direct Lingo use defaults to a concise human status. Skills and scripts use the
stable JSON surface by putting `--json` before the command:

```bash
lingo project show --selector my-project
lingo --json project show --selector my-project
lingo help
```

Strict S5 workflow selectors are explicit and independent of current directory:

```bash
lingo --json workflow start \
  --project <project-uuid-or-slug> \
  --repository <project-scoped-key> \
  --work-item 'github:<owner>/<repository>#<number>'

lingo --json workflow status \
  --project <project-uuid-or-slug> \
  --repository <project-scoped-key> \
  --work-item 'github:<owner>/<repository>#<number>' \
  --execution <execution-id>
```

A complete call asks zero questions. Interactive calls preserve supplied valid
values and ask only missing selectors. Unknown, duplicate, conflicting,
ambiguous, or malformed selectors return canonical `validation_failure` before
effects. A selector never grants local or Provider mutation authority.

Canonical JSON for current MVP surfaces, including workflow selector operations, uses
`status`, `result`, optional `references`, optional `next`, optional `details`, and
mandatory `provenance`. Human output renders the same semantic value. Other POC
operations temporarily retain `operation`, `status`, `category`, and applicable
typed payloads until their authorized MVP Tasks migrate them. Exit codes remain
`0` for success, `1` for failure, and `2` for interruption/cancellation.

| Codex skill | Stable Lingo entrypoint |
|---|---|
| `$axiom-project-configure` | `lingo --json project configure` |
| `$axiom-project-show` | `lingo --json project show --selector ...` |
| `$axiom-work-item-create` | `lingo --json work-item create\|select ...` |
| `$axiom-work-item-run` | `lingo --json workflow start\|advance\|fact\|resume\|reconcile ...` |
| `$axiom-work-item-status` | `lingo --json workflow status\|evidence ...` |

Skills collect missing selectors conversationally, but Lingo retains validation,
repository resolution, workflow ordering, and external-mutation authority.

## Resolve a configured Project globally

Use a canonical Project UUID or installation-unique slug from any directory:

```bash
lingo project resolve --selector my-project
lingo project show --selector my-project
```

Resolution reads protected machine-local state. It never searches the caller's
current directory and fails explicitly for ambiguous Projects or unavailable
portable/repository locations.

## Configure a Project

Guided CLI:

```bash
lingo project configure
```

Repeatable non-interactive form:

```bash
lingo --json project configure \
  --slug my-project \
  --name "My Project" \
  --repository main=/absolute/path/to/working-copy \
  --work-item-provider github
```

This first call is read-only and returns `setup.projectId` plus an exact
`setup.digest`. After review, repeat the same facts with:

```bash
lingo --json project configure \
  --project-id <preview-project-id> \
  --slug my-project \
  --name "My Project" \
  --repository main=/absolute/path/to/working-copy \
  --work-item-provider github \
  --preview-digest "$PREVIEW_DIGEST" \
  --authorize-local
```

Repeat `--repository` for multi-repository Projects. Keys and capability intent
enter portable state; absolute paths and observed revisions remain only in
protected machine-local state and the review preview. Replaced bindings or changed
state invalidate authority. Guided mode previews the same normalized proposal and
asks before publication. Codex uses the same command through
`$axiom-project-configure`; neither path infers identity from CWD or Git.

## GitHub Work Items

Create begins with a read-only, provider-neutral draft preview. Guided mode asks
only missing fields, prints the exact draft/target/effects/digest, and accepts
only the literal `yes` before the external effect:

```bash
lingo work-item create \
  --project my-project \
  --repository main \
  --provider-repository owner/repository
```

For non-interactive use, provide all seven sections. The first call is read-only:

```bash
lingo --json work-item create \
  --project my-project \
  --repository main \
  --provider-repository owner/repository \
  --problem "Observed behavior blocks delivery" \
  --desired-outcome "Delivery proceeds safely" \
  --context "Observed on supported hosts" \
  --scope "Bounded application change" \
  --constraints "Preserve exact authority" \
  --non-goals "No workflow execution" \
  --acceptance "Deterministic checks pass"
```

After reviewing every preview fact, repeat the exact same fields with the returned
digest and explicit external authority:

```bash
lingo --json work-item create \
  --project my-project \
  --repository main \
  --provider-repository owner/repository \
  --problem "Observed behavior blocks delivery" \
  --desired-outcome "Delivery proceeds safely" \
  --context "Observed on supported hosts" \
  --scope "Bounded application change" \
  --constraints "Preserve exact authority" \
  --non-goals "No workflow execution" \
  --acceptance "Deterministic checks pass" \
  --preview-digest "$PREVIEW_DIGEST" \
  --authorize-external
```

`--intent` may supply the problem section when `--problem` is absent. Changed
facts invalidate the digest. Authentication comes only from the existing `gh`
CLI session; Axiom does not persist its credential or infer a Git remote.

Selecting an existing Issue is a separate local-publication review. Preview the
exact provider identity/state first, then repeat it with local authority:

```bash
lingo --json work-item select \
  --project my-project \
  --repository main \
  --provider-repository owner/repository \
  --number 123

lingo --json work-item select \
  --project my-project \
  --repository main \
  --provider-repository owner/repository \
  --number 123 \
  --preview-digest <preview-digest> \
  --authorize-local

lingo work-item show --project my-project --repository main --number 123
```

The reviewed effect set includes the local create-attempt fence. Before POST,
Axiom durably reserves the exact target/draft create attempt. An
ambiguous result leaves that fence `pending`; every later process reconciles the
persisted correlation only and cannot emit another POST until the ambiguity is
resolved. Zero matches remain fail-closed without a time heuristic; one exact
match is linked; multiple matches require operator review. Confirmed GitHub
effect plus local failure is reported as canonical `partial` with the Issue
reference. New local link filenames bind provider, resource, and external ID;
existing v1 records remain readable through exact identity validation. Historical POC `comment` and
`complete` commands remain compatibility surfaces only; they are not part of S3
and must not be treated as workflow progress or human acceptance.

## Execute the bounded workflow

Start one workflow from an already linked Work Item:

```bash
lingo workflow start --project my-project --repository main --number 123
lingo workflow status --project my-project --repository main --number 123
```

Advance gates in fixed order from the exact current revision. Optional references
are either a machine-local detail artifact or a repository-relative regular
Evidence file no larger than 1 MiB. The caller supplies the expected SHA-256;
Lingo re-reads and validates it before committing the transition. Repository
resolution comes from Project state, not caller CWD.

```bash
lingo workflow advance --project my-project --repository main --number 123 \
  --expected-revision 1 --gate intake --outcome pass \
  --reference evidence:docs/intent.md:<sha256> --next "Review specification"
```

Gate order: `intake`, `specification`, `clarification`, `plan`, `tasks`,
`implementation`, `review`, `evidence`, `reconciliation`, `completion`.
References use `evidence:<repository-relative-path>:<sha256>` or
`artifact:<artifact-id>:<sha256>`; lifecycle boundary facts additionally accept
the closed `specification`, `decision`, `plan`, `tasks`, and `pull_request`
reference kinds. A failed gate records an interrupted
transition at the same stage. Resume requires the new exact revision. Neither
operation closes the Work Item:

```bash
lingo workflow advance --project my-project --repository main --number 123 \
  --expected-revision 6 --gate implementation --outcome fail \
  --reference evidence:evidence/test-failure.txt:<sha256>
lingo workflow resume --project my-project --repository main --number 123 \
  --expected-revision 7
lingo workflow evidence --project my-project --repository main --number 123
```

The ten-stage Work Item lifecycle is derived from canonical gates plus explicit,
revisioned local facts; it is not a second state machine. Record one fact with an
exact revision, a validated reference, and local authority:

```bash
lingo --json workflow fact \
  --project my-project --repository main --number 123 \
  --execution <execution-id> --expected-revision <revision> \
  --fact planning-authority --active \
  --reference specification:docs/specifications/example/spec.md:<sha256> \
  --authorize-local
```

Supported facts are `planning-authority`, `implementation-authority`,
`review-started`, `human-acceptance`, `blocked`, `needs-decision`, and
`needs-approval`. Omit `--active` only to clear an auxiliary condition. Authority
facts must be active. GitHub state, merge, CI, review, or Issue closure never
records a fact or produces `accepted`; terminal completion plus explicit human
acceptance is required.

Local completion is a local transition only. It requires the exact current
revision and never closes the GitHub Issue:

```bash
lingo workflow advance --project my-project --repository main --number 123 \
  --expected-revision 10 --gate completion --outcome pass
```

GitHub progress is a separate, post-commit projection. First inspect the current
Provider state and review the returned target, Execution revision, projection
key, comment, exact effects, observation digest, and preview digest:

```bash
lingo --json workflow reconcile \
  --project my-project --repository main --number 123 \
  --expected-revision 2
```

Only after explicit review, repeat the same target/revision with the exact digest
and external authority:

```bash
lingo --json workflow reconcile \
  --project my-project --repository main --number 123 \
  --expected-revision 2 \
  --preview-digest <preview-digest> \
  --authorize-external
```

Projection uses exactly one of `intake`, `specifying`, `specified`, `planning`,
`planned`, `implementing`, `implemented`, `reviewing`, `reviewed`, or `accepted`
under `axiom:stage:*`, plus applicable `axiom:blocked`,
`axiom:needs-decision`, `axiom:needs-approval`, and
`axiom:recovery-required` flags. Zero, multiple, unknown, or contradictory Axiom
markers require recovery. Projection removes only positively identified obsolete
managed labels and posts at most one provenance-marked comment per Execution
revision. It preserves foreign labels/content. Every
mutation is reinspected before its intended/confirmed ledger advances. Ambiguous
or unavailable results are reconcile-first; a confirmed Provider effect followed
by local bookkeeping failure is `partial`. Replaying the same revision converges
without a duplicate comment. Projection never advances, repairs, or completes
local workflow truth.

## Compatibility, cleanup, recovery, and upgrade

These S7 maintenance commands follow one rule: without `--authorize-local` they
are read-only previews; a mutation requires repeating the command with the exact
`--preview-digest` of a current preview. Any change between review and apply
denies authority. Targets and roots are explicit; the CWD is never used.

Classify the configured roots (`LINGO_PROJECTS_ROOT`, `LINGO_STATE_ROOT`, and the
Codex skill root) without creating, locking, or changing anything:

```bash
lingo --json compatibility inspect
```

The closed classifications are `absent_v1`, `valid_v1`, `recognized_poc`,
`malformed`, `unsupported_older`, `unsupported_newer`, and `recovery_required`.
Recognition of historical `v0.1.0-poc.1` state requires the complete positive
POC workflow signature; mixed POC/v1, partial, unknown, unsafe (link, mode,
ownership, ACL, type, size), and uncertain state is preserved for review and is
never reported as absent. Only Axiom-owned skill names in the shared skill root
are inspected. The report contains categories, kinds, owned relative names, and
digests, never file content.

Recognized POC state is never migrated in place. Preserve it with separate
authorities, each writing only new private objects into an absent target on the
same local filesystem as the source, outside every owned root:

```bash
lingo --json compatibility backup --target /absolute/new/poc-backup
lingo --json compatibility export --target /absolute/new/poc-export
# after review, repeat each with --preview-digest <digest> --authorize-local
```

Backup copies every recognized POC object; export copies only validated
portable Project manifests to `<target>/projects`, which a separate clean
`LINGO_STATE_ROOT` can then configure explicitly with `lingo project configure`.
`manifest.json` is written last; a target without it is incomplete and is never
adopted. An equivalent completed target is reported as a no-op.

Explicit, reference-aware artifact cleanup:

```bash
lingo --json artifact cleanup
lingo --json artifact cleanup --preview-digest <digest> --authorize-local
lingo --json artifact retire --artifact <artifact-id>
lingo --json artifact retire --artifact <artifact-id> --preview-digest <digest> --authorize-local
```

Eligible: unreferenced `diagnostic` artifacts at least 30 days old, `evidence`
artifacts at least 365 days after an explicit retirement, and confirmed cleanup
records at least 90 days old. References come from every local Execution record
and artifact under the state-root lock; uncertain reference state fails closed.
`active`, `preserved_review`, and referenced artifacts are always preserved.

`artifact retire` records that Evidence has no live reference; it removes
nothing. It is denied for any referenced, non-Evidence, or already retired
artifact, and writes `artifacts/v1/retirements/<id>.json` (format 1, separate
from metadata format 1) bound to the exact artifact revision. Evidence without a
retirement, or with a corrupt, stale, or other-revision retirement, is preserved.
A new artifact that references a retired artifact supersedes its retirement, so
a later retirement must be recorded explicitly. Each authorized batch removes
at most 128 exact objects and publishes a bounded cleanup record before removal.
Capacity exhaustion never deletes anything.

Guided recovery of interrupted local publication:

```bash
lingo --json recovery inspect
lingo --json recovery apply --preview-digest <plan-digest> --authorize-local
```

Inspection lists each interrupted protocol directory, its marker, generations,
fault stage, and proposed action. Only `restore_prior` (before the commit point)
and `finalize_committed` (after it) are ever applied, under a fresh exact plan
digest and the owning store's lock order; a committed generation is never rolled
back. Ambiguous, contradictory, corrupt, or unknown state and interrupted S2
attempt markers remain `preserved_review`.

Owned upgrade of a release installation made by `install-release.sh`:

```bash
lingo --json upgrade \
  --archive /absolute/release/axiom-0.2.0-macos-27-arm64.tar.gz \
  --checksums /absolute/release/SHA256SUMS \
  --bin-dir /absolute/user-owned/bin \
  --receipt-dir /absolute/user-owned/state
# after review, repeat with --preview-digest <digest> --authorize-local
```

The archive is verified exactly as the installer does before anything else.
Preflight requires the exact approved host row, an unmodified owned binary and
receipt, a newer version (downgrade and divergent same-version replacement are
refused), `absent_v1` or `valid_v1` state, and observed free space. The binary
and then the receipt are published and re-read as separate confirmed effects;
`installedAt` is preserved. A later failure is `partial`: the installer's
`.axiom-install-operation` marker records the exact archive, so only the same
archive can resume, and the installer refuses in the meantime. If installed
Codex skills do not match the new version the result is `partial` and the next
action is `lingo runtime codex install` with the upgraded binary. There is no
automatic update, rollback, or cross-root transaction.
