// Package compatibility owns read-only classification of persisted Axiom
// state and the separately authorized preservation of recognized POC state.
package compatibility

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"

	"github.com/rgomids/axiom/internal/codexruntime"
	"github.com/rgomids/axiom/internal/local"
)

// The historical POC signature is frozen from the merged v0.1.0-poc.1 tag.
const (
	HistoricalPOCTag      = "v0.1.0-poc.1"
	HistoricalPOCRevision = "242d67c4cf2d4c3efe534dd894cb56a05558e139"
	maxFindings           = 32
)

type Classification string

const (
	AbsentV1         Classification = "absent_v1"
	ValidV1          Classification = "valid_v1"
	RecognizedPOC    Classification = "recognized_poc"
	Malformed        Classification = "malformed"
	UnsupportedOlder Classification = "unsupported_older"
	UnsupportedNewer Classification = "unsupported_newer"
	RecoveryRequired Classification = "recovery_required"
)

const (
	CategoryProjects = "projects"
	CategoryState    = "state"
	CategorySkills   = "skills"
)

// Roots are explicit absolute roots. An empty root is not inspected.
type Roots struct {
	Projects string
	State    string
	Skills   string
}

type Object struct {
	Category string              `json:"category"`
	Relative string              `json:"relative"`
	Kind     local.InventoryKind `json:"kind"`
	Digest   string              `json:"sha256,omitempty"`
	Bytes    int64               `json:"bytes,omitempty"`
}

type CategorySummary struct {
	Category string         `json:"category"`
	Present  bool           `json:"present"`
	Entries  int            `json:"entries"`
	Bytes    int64          `json:"bytes"`
	Kinds    map[string]int `json:"kinds"`
}

type Finding struct {
	Category string `json:"category"`
	Relative string `json:"relative"`
	Kind     string `json:"kind"`
}

// Report is bounded and content-free: it carries categories, kinds, relative
// owned names, and digests, never file bytes.
type Report struct {
	Classification    Classification         `json:"classification"`
	Reason            string                 `json:"reason"`
	Next              []string               `json:"next"`
	Categories        []CategorySummary      `json:"categories"`
	SkillSet          codexruntime.Inventory `json:"skillSet"`
	Findings          []Finding              `json:"findings"`
	FindingsTruncated bool                   `json:"findingsTruncated,omitempty"`
	POCTag            string                 `json:"pocTag,omitempty"`
	POCRevision       string                 `json:"pocRevision,omitempty"`
	Digest            string                 `json:"digest"`
	objects           []Object
}

var ErrUnsafeRoot = errors.New("unsafe compatibility root")

// Inspect performs only bounded reads. Missing roots are not created, locks
// are not taken, and uncertainty is never reported as absence.
func Inspect(ctx context.Context, roots Roots) (Report, error) {
	for _, root := range []string{roots.Projects, roots.State, roots.Skills} {
		if root != "" && (!filepath.IsAbs(root) || filepath.Clean(root) == string(filepath.Separator)) {
			return Report{}, ErrUnsafeRoot
		}
	}
	var observed observation
	for _, source := range []struct {
		category string
		path     string
		inspect  func(context.Context, string) (local.Inventory, error)
	}{{CategoryProjects, roots.Projects, local.InspectPortableInventory}, {CategoryState, roots.State, local.InspectStateInventory}} {
		summary := CategorySummary{Category: source.category, Kinds: map[string]int{}}
		if source.path != "" {
			inventory, err := source.inspect(ctx, source.path)
			if errors.Is(err, local.ErrInventoryBound) {
				observed.bounded = true
			} else if err != nil {
				return Report{}, err
			}
			summary.Present = inventory.Present
			for _, entry := range inventory.Entries {
				observed.add(Object{Category: source.category, Relative: entry.Relative, Kind: entry.Kind, Digest: entry.Digest, Bytes: entry.Bytes})
				summary.Entries++
				summary.Bytes += entry.Bytes
				summary.Kinds[string(entry.Kind)]++
			}
		}
		observed.categories = append(observed.categories, summary)
	}
	skills := CategorySummary{Category: CategorySkills, Kinds: map[string]int{}}
	observed.skills = codexruntime.Inventory{State: codexruntime.SkillSetAbsent, Receipt: "absent", Skills: []codexruntime.InventorySkill{}}
	if roots.Skills != "" {
		service, err := codexruntime.New(roots.Skills)
		if err != nil {
			return Report{}, ErrUnsafeRoot
		}
		inventory, err := service.Inventory(ctx)
		if err != nil {
			return Report{}, err
		}
		observed.skills = inventory
		for _, skill := range inventory.Skills {
			if skill.State == "missing" {
				continue
			}
			skills.Present = true
			skills.Entries++
			skills.Kinds[skill.State]++
			if skill.State == "legacy" || skill.State == "current" {
				observed.add(Object{Category: CategorySkills, Relative: skill.Name + "/SKILL.md", Kind: local.InventoryKind("skill_" + skill.State), Digest: skill.SHA256, Bytes: skill.Bytes})
			}
		}
	}
	observed.categories = append(observed.categories, skills)
	return observed.report(), nil
}

type observation struct {
	objects    []Object
	categories []CategorySummary
	skills     codexruntime.Inventory
	bounded    bool
}

func (o *observation) add(object Object) { o.objects = append(o.objects, object) }

func (o observation) report() Report {
	sort.Slice(o.objects, func(i, j int) bool {
		if o.objects[i].Category != o.objects[j].Category {
			return o.objects[i].Category < o.objects[j].Category
		}
		return o.objects[i].Relative < o.objects[j].Relative
	})
	counts := map[local.InventoryKind]int{}
	for _, object := range o.objects {
		counts[object.Kind]++
	}
	pocWorkflow := counts[local.InventoryPOCWorkflow] > 0
	v1Only := false
	supported := false
	for kind, count := range counts {
		if count == 0 {
			continue
		}
		if kind.V1Only() {
			v1Only = true
		}
		if kind.Supported() {
			supported = true
		}
	}
	skillState := o.skills.State
	report := Report{Categories: o.categories, SkillSet: o.skills, Findings: []Finding{}, objects: o.objects}
	switch {
	case counts[local.InventoryRecovery] > 0 || skillState == codexruntime.SkillSetInterrupted:
		report.Classification, report.Reason = RecoveryRequired, "interrupted_protocol_state"
		report.Next = []string{"Run `axiom recovery inspect` and authorize only an exact recognized recovery plan", "Do not overwrite, migrate, or clean the preserved state meanwhile"}
	case counts[local.InventoryNewer] > 0:
		report.Classification, report.Reason = UnsupportedNewer, "newer_format_version"
		report.Next = []string{"Use an Axiom version that supports the newer format", "Preserve all state; this version will not overwrite it"}
	case counts[local.InventoryOlder] > 0:
		report.Classification, report.Reason = UnsupportedOlder, "older_format_version"
		report.Next = []string{"Preserve all state; no supported compatibility path exists for this older format", "Configure a separate clean v1 root if new work is needed"}
	case o.bounded:
		report.Classification, report.Reason = Malformed, "entry_bound_exceeded"
		report.Next = []string{"Preserve state for operator review; inspection exceeded its bounded entry limit"}
	case counts[local.InventoryMalformed] > 0 || counts[local.InventoryUnsafe] > 0 || counts[local.InventoryUnknown] > 0 || skillState == codexruntime.SkillSetForeign:
		report.Classification, report.Reason = Malformed, "unrecognized_or_unsafe_content"
		report.Next = []string{"Preserve state for operator review; do not overwrite, migrate, or clean it"}
	case pocWorkflow && v1Only:
		report.Classification, report.Reason = Malformed, "mixed_poc_and_v1_state"
		report.Next = []string{"Preserve the mixed POC and v1 state for operator review; no automatic separation is supported"}
	case pocWorkflow:
		report.Classification, report.Reason = RecognizedPOC, "complete_poc_workflow_signature"
		report.POCTag, report.POCRevision = HistoricalPOCTag, HistoricalPOCRevision
		report.Next = []string{
			"POC state is preserved; it is never migrated in place and POC workflow history does not become v1 state",
			"Optional local preservation: `axiom compatibility backup --target <absent-directory>` and authorize the exact preview digest",
			"Optional portable intent: `axiom compatibility export --target <absent-directory>` and authorize the exact preview digest",
			"Configure clean v1 state in a separate LINGO_STATE_ROOT and run `axiom project configure` explicitly",
		}
	case supported || skillState == codexruntime.SkillSetCurrent || skillState == codexruntime.SkillSetUpgradable:
		report.Classification, report.Reason = ValidV1, "v1_readable_state"
		report.Next = []string{"Continue with v1 operations"}
	default:
		report.Classification, report.Reason = AbsentV1, "no_owned_state"
		report.Next = []string{"Run `axiom runtime codex install`, then `axiom project configure` explicitly"}
	}
	if skillState == codexruntime.SkillSetUpgradable && (report.Classification == ValidV1 || report.Classification == RecognizedPOC) {
		report.Next = append(report.Next, "Run `axiom runtime codex install` to replace the known older owned Codex skill set")
	}
	for _, object := range o.objects {
		switch object.Kind {
		case local.InventoryRecovery, local.InventoryNewer, local.InventoryOlder, local.InventoryMalformed, local.InventoryUnsafe, local.InventoryUnknown, local.InventoryPOCWorkflow:
			if len(report.Findings) == maxFindings {
				report.FindingsTruncated = true
				continue
			}
			report.Findings = append(report.Findings, Finding{Category: object.Category, Relative: object.Relative, Kind: string(object.Kind)})
		}
	}
	for _, skill := range o.skills.Skills {
		if skill.State == "foreign" {
			if len(report.Findings) == maxFindings {
				report.FindingsTruncated = true
				continue
			}
			report.Findings = append(report.Findings, Finding{Category: CategorySkills, Relative: skill.Name, Kind: "foreign"})
		}
	}
	wire, _ := json.Marshal(struct {
		Classification Classification         `json:"classification"`
		Objects        []Object               `json:"objects"`
		Skills         codexruntime.Inventory `json:"skills"`
		Bounded        bool                   `json:"bounded"`
	}{report.Classification, o.objects, o.skills, o.bounded})
	digest := sha256.Sum256(wire)
	report.Digest = hex.EncodeToString(digest[:])
	return report
}
