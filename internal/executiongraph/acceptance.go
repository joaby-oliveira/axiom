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
	RuntimeExecutions    []RuntimeExecutionEvidence
	CoordinationRecords  [][]byte
	Coordination         []CoordinationEvidence
	Rollup               ParentRollup
	ValidationReferences []string
	Limitations          []string
	Digest               string
}

type RuntimeExecutionEvidence struct {
	ChildID               string
	AttemptID             string
	RuntimeID             string
	ModelProfileID        string
	RuntimeVersion        string
	ConfigurationRevision uint64
	ObservationRevision   uint64
	InvocationDigest      string
	ResultReference       string
}

const (
	CoordinationQuestionRequest    = "question_request"
	CoordinationAnswer             = "answer"
	CoordinationContractProposal   = "contract_proposal"
	CoordinationContractAcceptance = "contract_acceptance"
	CoordinationAccepted           = "accepted"
)

// CoordinationEvidence is one coordination fact derived by BuildEvidence
// through a CoordinationVerifier from a canonical coordination record. Callers
// supply canonical records in Evidence.CoordinationRecords; BuildEvidence
// rejects caller-populated Coordination so projected fields are never
// self-asserted. Lineage is then validated against the graph and the T36
// exchange is derived from Kind, Reference and Decision.
type CoordinationEvidence struct {
	RecordID      string
	Kind          string
	ParentID      string
	GraphRevision uint64
	ChildID       string
	AttemptID     string
	Digest        string
	Reference     string
	Decision      string
}

// CoordinationVerifier is the acceptance-owned port through which
// BuildEvidence derives coordination facts. An implementation decodes one
// canonical coordination record, recomputes its digest from that content and
// projects it. The coordination package owns the canonical format and provides
// the production implementation.
type CoordinationVerifier interface {
	VerifyCoordination(record []byte) (CoordinationEvidence, error)
}

func BuildEvidence(graph Graph, evidence Evidence, verifier CoordinationVerifier) (Evidence, error) {
	if len(evidence.Coordination) != 0 {
		return Evidence{}, ErrInvalidRunEnvelope
	}
	records, facts, ok := verifiedCoordination(evidence.CoordinationRecords, verifier)
	if !ok {
		return Evidence{}, ErrInvalidRunEnvelope
	}
	evidence.CoordinationRecords, evidence.Coordination = records, facts
	if !ValidGraph(graph) || evidence.FormatVersion != 1 || !validSourceRevision(evidence.BaseRevision) || !validDigest(evidence.ConfigurationDigest) || evidence.ParentID != graph.Parent.ExecutionID || evidence.GraphRevision != graph.Parent.GraphRevision || !validCoordinationEvidence(graph, evidence.Coordination) || !boundedReferences(evidence.ValidationReferences) || len(evidence.Limitations) == 0 || !boundedReferences(evidence.Limitations) || !validEvidenceRollup(graph, evidence.Rollup) || !validDispatchEvidence(graph, evidence.Dispatch) || !validRuntimeExecutionEvidence(graph, evidence.RuntimeExecutions) {
		return Evidence{}, ErrInvalidRunEnvelope
	}
	if completeRealRuntimeJourney(graph, evidence) {
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
	sort.Slice(evidence.Dispatch, func(i, j int) bool {
		return evidence.Dispatch[i].ChildID+"\x00"+evidence.Dispatch[i].AttemptID < evidence.Dispatch[j].ChildID+"\x00"+evidence.Dispatch[j].AttemptID
	})
	sort.Slice(evidence.RuntimeExecutions, func(i, j int) bool {
		return evidence.RuntimeExecutions[i].ChildID+"\x00"+evidence.RuntimeExecutions[i].AttemptID < evidence.RuntimeExecutions[j].ChildID+"\x00"+evidence.RuntimeExecutions[j].AttemptID
	})
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

func validEvidenceRollup(graph Graph, rollup ParentRollup) bool {
	if rollup.ParentID != graph.Parent.ExecutionID || rollup.GraphRevision != graph.Parent.GraphRevision || rollup.IntegrationChildID != graph.Parent.IntegrationChild || len(rollup.ChildOutcomes) != len(graph.Children) {
		return false
	}
	if rollup.Status != "partial" && rollup.Status != "success" && rollup.Status != "failure" && rollup.Status != "recovery_required" {
		return false
	}
	if rollup.IntegrationResultTree != "" && !validSourceRevision(rollup.IntegrationResultTree) {
		return false
	}
	if len(rollup.ValidationResults) > 0 && !validValidationResults(rollup.ValidationResults) {
		return false
	}
	if len(rollup.References) > 0 && !boundedReferences(rollup.References) {
		return false
	}
	children := make(map[string]ChildExecution, len(graph.Children))
	for _, child := range graph.Children {
		children[child.ExecutionID] = child
	}
	// Every outcome, including Integration/Reconciliation, must equal the last
	// persisted attempt; a roll-up cannot assert an outcome the graph lacks.
	seen := map[string]bool{}
	for _, outcome := range rollup.ChildOutcomes {
		child, ok := children[outcome.ChildID]
		if !ok || seen[outcome.ChildID] || outcome.Required != !child.Envelope.Optional || !validAttemptStatus(outcome.Status) {
			return false
		}
		expected := AttemptNotStarted
		if len(child.Attempts) > 0 {
			expected = child.Attempts[len(child.Attempts)-1].Status
		}
		if outcome.Status != expected {
			return false
		}
		if rollup.Status == "success" && outcome.Required && outcome.Status != AttemptSucceeded {
			return false
		}
		seen[outcome.ChildID] = true
	}
	if rollup.Status == "success" {
		integration, ok := integrationChild(graph)
		if !ok || len(integration.Attempts) == 0 || !validSourceRevision(rollup.IntegrationResultTree) || !validValidationResults(rollup.ValidationResults) {
			return false
		}
		last := integration.Attempts[len(integration.Attempts)-1]
		if last.Status != AttemptSucceeded || last.ResultReference != IntegrationResultReference(rollup.IntegrationResultTree) {
			return false
		}
	}
	return true
}

func validDispatchEvidence(graph Graph, records []DispatchRecord) bool {
	seen := map[string]bool{}
	for _, record := range records {
		child, attempt, ok := graphAttempt(graph, record.ChildID, record.AttemptID)
		key := record.ChildID + "\x00" + record.AttemptID
		if !ok || child.Envelope.IntegrationOwner || seen[key] || record.Status != attempt.Status || record.StartedAt.IsZero() || record.EndedAt.IsZero() || record.EndedAt.Before(record.StartedAt) || attempt.StartedAt == nil || attempt.FinishedAt == nil || !record.StartedAt.Equal(*attempt.StartedAt) || !record.EndedAt.Equal(*attempt.FinishedAt) {
			return false
		}
		seen[key] = true
	}
	return true
}

func validRuntimeExecutionEvidence(graph Graph, records []RuntimeExecutionEvidence) bool {
	seen := map[string]bool{}
	for _, record := range records {
		child, attempt, ok := graphAttempt(graph, record.ChildID, record.AttemptID)
		key := record.ChildID + "\x00" + record.AttemptID
		resolution := child.Envelope.Resolution
		if !ok || child.Envelope.IntegrationOwner || seen[key] || attempt.Status != AttemptSucceeded || !validDigest(attempt.OutputDigest) || !validTarget(attempt.ResultReference) || record.RuntimeID != resolution.RuntimeID || record.ModelProfileID != resolution.ModelProfileID || record.ConfigurationRevision != resolution.ConfigurationRevision || record.ObservationRevision != resolution.ObservationRevision || !validToken(record.RuntimeVersion) || !validDigest(record.InvocationDigest) || record.ResultReference != attempt.ResultReference {
			return false
		}
		seen[key] = true
	}
	return true
}

func completeRealRuntimeJourney(graph Graph, evidence Evidence) bool {
	if evidence.Rollup.Status != "success" || !validSourceRevision(evidence.Rollup.IntegrationResultTree) || !validValidationResults(evidence.Rollup.ValidationResults) || len(evidence.Rollup.References) == 0 {
		return false
	}
	validationReferences := map[string]bool{}
	for _, reference := range evidence.ValidationReferences {
		validationReferences[reference] = true
	}
	for _, result := range evidence.Rollup.ValidationResults {
		if !validationReferences[result.CommandReference] {
			return false
		}
	}
	for _, outcome := range evidence.Rollup.ChildOutcomes {
		if outcome.Required && outcome.Status != AttemptSucceeded {
			return false
		}
	}
	dispatched := map[string]DispatchRecord{}
	for _, record := range evidence.Dispatch {
		if record.Status == AttemptSucceeded {
			dispatched[record.ChildID+"\x00"+record.AttemptID] = record
		}
	}
	children := make(map[string]ChildExecution, len(graph.Children))
	for _, child := range graph.Children {
		children[child.ExecutionID] = child
	}
	executed := map[string]bool{}
	var codexRuns, claudeRuns []DispatchRecord
	for _, record := range evidence.RuntimeExecutions {
		key := record.ChildID + "\x00" + record.AttemptID
		dispatch, ok := dispatched[key]
		if !ok {
			return false
		}
		executed[key] = true
		switch record.RuntimeID {
		case "codex":
			codexRuns = append(codexRuns, dispatch)
		case "claude":
			claudeRuns = append(claudeRuns, dispatch)
		}
	}
	if !coordinatedExchange(evidence.Coordination, executed) {
		return false
	}
	for _, codex := range codexRuns {
		for _, claude := range claudeRuns {
			if codex.ChildID != claude.ChildID && !childDependsTransitively(children, codex.ChildID, claude.ChildID) && !childDependsTransitively(children, claude.ChildID, codex.ChildID) && codex.StartedAt.Before(claude.EndedAt) && claude.StartedAt.Before(codex.EndedAt) {
				return true
			}
		}
	}
	return false
}

// verifiedCoordination derives coordination facts only through the verifier
// and returns records and facts in the same RecordID order.
func verifiedCoordination(wires [][]byte, verifier CoordinationVerifier) ([][]byte, []CoordinationEvidence, bool) {
	if len(wires) == 0 {
		return nil, nil, true
	}
	if verifier == nil || len(wires) > maxListItems {
		return nil, nil, false
	}
	type verified struct {
		wire []byte
		fact CoordinationEvidence
	}
	items := make([]verified, 0, len(wires))
	for _, wire := range wires {
		fact, err := verifier.VerifyCoordination(wire)
		if err != nil {
			return nil, nil, false
		}
		items = append(items, verified{wire: append([]byte(nil), wire...), fact: fact})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].fact.RecordID < items[j].fact.RecordID })
	records := make([][]byte, 0, len(items))
	facts := make([]CoordinationEvidence, 0, len(items))
	for _, item := range items {
		records = append(records, item.wire)
		facts = append(facts, item.fact)
	}
	return records, facts, true
}

func validCoordinationEvidence(graph Graph, records []CoordinationEvidence) bool {
	if len(records) > maxListItems {
		return false
	}
	seen := map[string]bool{}
	for _, record := range records {
		if !validOpaqueID(record.RecordID) || !validToken(record.Kind) || record.ParentID != graph.Parent.ExecutionID || record.GraphRevision != graph.Parent.GraphRevision || !validDigest(record.Digest) || seen[record.RecordID] || seen[record.Digest] {
			return false
		}
		if record.Reference != "" && !validOpaqueID(record.Reference) || record.Decision != "" && !validToken(record.Decision) {
			return false
		}
		if record.AttemptID == "" {
			if !graphChild(graph, record.ChildID) {
				return false
			}
		} else if _, _, ok := graphAttempt(graph, record.ChildID, record.AttemptID); !ok {
			return false
		}
		seen[record.RecordID] = true
		seen[record.Digest] = true
	}
	return true
}

// coordinatedExchange reports whether two distinct children exchanged a
// question/answer or contract proposal/acceptance while running executed
// Runtime attempts.
func coordinatedExchange(records []CoordinationEvidence, executed map[string]bool) bool {
	byID := make(map[string]CoordinationEvidence, len(records))
	for _, record := range records {
		byID[record.RecordID] = record
	}
	for _, response := range records {
		requestKind := ""
		switch {
		case response.Kind == CoordinationAnswer:
			requestKind = CoordinationQuestionRequest
		case response.Kind == CoordinationContractAcceptance && response.Decision == CoordinationAccepted:
			requestKind = CoordinationContractProposal
		default:
			continue
		}
		request, ok := byID[response.Reference]
		if ok && request.Kind == requestKind && request.ChildID != response.ChildID && executed[request.ChildID+"\x00"+request.AttemptID] && executed[response.ChildID+"\x00"+response.AttemptID] {
			return true
		}
	}
	return false
}

func graphChild(graph Graph, childID string) bool {
	for _, child := range graph.Children {
		if child.ExecutionID == childID {
			return true
		}
	}
	return false
}

func graphAttempt(graph Graph, childID, attemptID string) (ChildExecution, Attempt, bool) {
	for _, child := range graph.Children {
		if child.ExecutionID != childID {
			continue
		}
		for _, attempt := range child.Attempts {
			if attempt.AttemptID == attemptID {
				return child, attempt, true
			}
		}
	}
	return ChildExecution{}, Attempt{}, false
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
