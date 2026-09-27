package executiongraph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

var ErrInvalidRunEnvelope = errors.New("invalid T36 run envelope")

type ChildRunPlan struct {
	ChildID, RuntimeID, ModelProfileID string
	Repository, Workspace              string
	Scope                              Scope
	Dependencies                       []string
	Effects                            []Effect
	Argv                               []string
	Timeout                            string
	MaximumAttempts                    uint32
}

type RunEnvelope struct {
	FormatVersion       int
	Activity            string
	BaseRevision        string
	ParentID            string
	GraphRevision       uint64
	ConfigurationRef    string
	ConfigurationDigest string
	Children            []ChildRunPlan
	Validators          []string
	ExpectedEvidence    []string
	CleanupDisposition  string
	Digest              string
}

func PrepareRunEnvelope(graph Graph, envelope RunEnvelope) (RunEnvelope, error) {
	if !ValidGraph(graph) || envelope.FormatVersion != 1 || !validTarget(envelope.Activity) || !validSourceRevision(envelope.BaseRevision) || envelope.ParentID != graph.Parent.ExecutionID || envelope.GraphRevision != graph.Parent.GraphRevision || !validTarget(envelope.ConfigurationRef) || !validDigest(envelope.ConfigurationDigest) || len(envelope.Children) != len(graph.Children) || len(envelope.Validators) == 0 || len(envelope.ExpectedEvidence) == 0 || envelope.CleanupDisposition != "preserve_pending_human_review" {
		return RunEnvelope{}, ErrInvalidRunEnvelope
	}
	children := make(map[string]ChildExecution, len(graph.Children))
	for _, child := range graph.Children {
		children[child.ExecutionID] = child
	}
	seen := map[string]bool{}
	for _, plan := range envelope.Children {
		child, exists := children[plan.ChildID]
		if !exists || seen[plan.ChildID] || plan.RuntimeID != child.Envelope.Resolution.RuntimeID || plan.ModelProfileID != child.Envelope.Resolution.ModelProfileID || plan.Workspace != child.Envelope.Workspace || !filepath.IsAbs(plan.Repository) || !filepath.IsAbs(plan.Workspace) || !sameScope(plan.Scope, child.Envelope.Scope) || !sameStrings(plan.Dependencies, child.Envelope.Dependencies) || !sameEffects(plan.Effects, child.Envelope.AllowedEffects) || plan.Timeout != child.Envelope.Controls.Timeout.String() || plan.MaximumAttempts != child.Envelope.Controls.MaximumAttempts {
			return RunEnvelope{}, ErrInvalidRunEnvelope
		}
		invocation := Invocation{RuntimeID: plan.RuntimeID, Argv: plan.Argv, CWD: plan.Workspace, Env: []string{}, OutputMax: MaxCapturedOutputBytes}
		if !validInvocation(child, invocation) {
			return RunEnvelope{}, ErrInvalidRunEnvelope
		}
		seen[plan.ChildID] = true
	}
	if !boundedReferences(envelope.Validators) || !boundedReferences(envelope.ExpectedEvidence) {
		return RunEnvelope{}, ErrInvalidRunEnvelope
	}
	sort.Slice(envelope.Children, func(i, j int) bool { return envelope.Children[i].ChildID < envelope.Children[j].ChildID })
	for index := range envelope.Children {
		envelope.Children[index].Dependencies = sortedStringsCopy(envelope.Children[index].Dependencies)
		envelope.Children[index].Effects = sortedEffects(envelope.Children[index].Effects)
	}
	envelope.Validators = sortedStringsCopy(envelope.Validators)
	envelope.ExpectedEvidence = sortedStringsCopy(envelope.ExpectedEvidence)
	digest, err := runEnvelopeDigest(envelope)
	if err != nil {
		return RunEnvelope{}, err
	}
	envelope.Digest = digest
	return envelope, nil
}

type Evidence struct {
	FormatVersion        int
	Status               string
	BaseRevision         string
	ConfigurationDigest  string
	ParentID             string
	GraphRevision        uint64
	GraphDigest          string
	Dispatch             []DispatchRecord
	CoordinationDigests  []string
	Rollup               ParentRollup
	ValidationReferences []string
	Limitations          []string
	RealRuntimeRun       bool
	Digest               string
}

func BuildEvidence(graph Graph, evidence Evidence) (Evidence, error) {
	if !ValidGraph(graph) || evidence.FormatVersion != 1 || !validSourceRevision(evidence.BaseRevision) || !validDigest(evidence.ConfigurationDigest) || evidence.ParentID != graph.Parent.ExecutionID || evidence.GraphRevision != graph.Parent.GraphRevision || evidence.Rollup.ParentID != graph.Parent.ExecutionID || evidence.Rollup.GraphRevision != graph.Parent.GraphRevision || !boundedDigests(evidence.CoordinationDigests) || !boundedReferences(evidence.ValidationReferences) || len(evidence.Limitations) == 0 || !boundedReferences(evidence.Limitations) {
		return Evidence{}, ErrInvalidRunEnvelope
	}
	if evidence.RealRuntimeRun {
		evidence.Status = "real_run_recorded"
	} else {
		evidence.Status = "deterministic_preparation_only"
	}
	graphWire, err := EncodeGraph(graph)
	if err != nil {
		return Evidence{}, err
	}
	graphDigest := sha256.Sum256(graphWire)
	evidence.GraphDigest = hex.EncodeToString(graphDigest[:])
	sort.Strings(evidence.CoordinationDigests)
	sort.Strings(evidence.ValidationReferences)
	sort.Strings(evidence.Limitations)
	evidence.Digest = ""
	wire, err := json.Marshal(evidence)
	if err != nil {
		return Evidence{}, err
	}
	digest := sha256.Sum256(wire)
	evidence.Digest = hex.EncodeToString(digest[:])
	return evidence, nil
}

func runEnvelopeDigest(envelope RunEnvelope) (string, error) {
	envelope.Digest = ""
	wire, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:]), nil
}

func validSourceRevision(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && (len(decoded) == 20 || len(decoded) == sha256.Size)
}

func boundedReferences(values []string) bool {
	if len(values) == 0 || len(values) > maxListItems {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !validTarget(value) || strings.Contains(strings.ToLower(value), "raw chat") || strings.Contains(strings.ToLower(value), "hidden reasoning") || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func boundedDigests(values []string) bool {
	if len(values) == 0 || len(values) > maxListItems {
		return false
	}
	for _, value := range values {
		if !validDigest(value) {
			return false
		}
	}
	return true
}

func sameStrings(left, right []string) bool {
	left = sortedStringsCopy(left)
	right = sortedStringsCopy(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameScope(left, right Scope) bool {
	return left.ProjectID == right.ProjectID && left.RepositoryKey == right.RepositoryKey && sameStrings(left.Paths, right.Paths)
}

func sortedStringsCopy(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
