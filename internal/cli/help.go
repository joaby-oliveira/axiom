package cli

import "io"

const helpText = `Axiom — Lingo local control plane

Usage:
  axiom [--human|--json] <command>
  axiom help

Commands:
  first-run
  runtime codex install|status
  runtime claude install|status
  runtime profile validate
  project configure|show|resolve|init|validate|reopen|update|install
  work-item create|select|show|comment|complete
  workflow start|advance|fact|resume|status|evidence|reconcile
  compatibility inspect|backup|export
  artifact cleanup|retire
  recovery inspect|apply
  upgrade --archive <path> --checksums <path> --bin-dir <dir> --receipt-dir <dir>

Stable Codex skill mapping:
  $axiom-project-configure -> axiom --json project configure
  $axiom-project-show      -> axiom --json project show
  $axiom-work-item-create  -> axiom --json work-item create|select
  $axiom-work-item-run     -> axiom --json workflow start|advance|resume|reconcile
  $axiom-work-item-status  -> axiom --json workflow status|evidence

first-run finds Codex and Claude by their executables on PATH and installs or
upgrades Axiom's user-global skills for each one found (Codex:
$HOME/.agents/skills; Claude: <CLAUDE_CONFIG_DIR or ~/.claude>/skills). It never
installs a Runtime or touches credentials; no Runtime found is success.

Runtime profile validation reads local configuration without changing state,
invoking a runtime, or probing authentication. It accepts no flags or arguments.

Maintenance commands are read-only previews unless repeated with the exact
--preview-digest and --authorize-local. Backup/export targets must be absent
absolute paths. Recovery applies one plan selected by its digest.

Use --json for machine-readable output. Default and --human output are readable
status summaries. Mutation authority remains explicit through
--authorize-external or --authorize-local.

project configure without --project creates a new Project; an already
configured slug or --project-id fails without changes. project configure
--project <project-uuid-or-slug> previews an edit of an existing Project and
never writes:
  --name <name>, --work-item-provider <id> | --remove-work-item-provider
  --repository <key>=<absolute-path> (repeatable add/update)
  --remove-repository <key> (repeatable)
Omitted values are preserved. Edit rejects --slug (rename) and, because edit
publication is not available, --project-id, --preview-digest and
--authorize-local.

Strict selector vocabulary:
  --project <project-uuid-or-slug>
  --repository <project-scoped-key>
  --work-item github:<owner>/<repository>#<number>
  --execution <execution-id> (all workflow operations except start)
  --runtime codex|claude (workflow start only; default codex; kept by the Execution)

Fully specified selectors require no prompt. Missing selectors may be prompted;
unknown, duplicate, conflicting, or ambiguous selectors fail validation without
CWD, Git, Provider, or Runtime fallback.
`

func Help(writer io.Writer) int {
	if writer == nil {
		return ExitFailure
	}
	if _, err := io.WriteString(writer, helpText); err != nil {
		return ExitFailure
	}
	return ExitSuccess
}
