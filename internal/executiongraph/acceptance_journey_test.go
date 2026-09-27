package executiongraph_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/coordination"
	. "github.com/rgomids/axiom/internal/executiongraph"
)

// These journey tests derive coordination facts only through the production
// coordination.AcceptanceVerifier over records published by coordination.Service.

var verifier = coordination.AcceptanceVerifier{}

func TestBuildEvidenceDerivesRealRuntimeStatusFromCompleteJourney(t *testing.T) {
	graph, evidence, _ := completeRuntimeEvidenceFixture(t)
	tests := []struct {
		name   string
		mutate func(*Evidence)
	}{
		{"only Codex", func(evidence *Evidence) { evidence.RuntimeExecutions = evidence.RuntimeExecutions[:1] }},
		{"only Claude", func(evidence *Evidence) { evidence.RuntimeExecutions = evidence.RuntimeExecutions[1:] }},
		{"incomplete rollup", func(evidence *Evidence) {
			evidence.Rollup.Status = "partial"
			evidence.Rollup.IntegrationResultTree = ""
			for index := range evidence.Rollup.ChildOutcomes {
				if evidence.Rollup.ChildOutcomes[index].ChildID == evidence.Rollup.IntegrationChildID {
					evidence.Rollup.ChildOutcomes[index].Status = AttemptUnknown
				}
			}
		}},
		{"missing combined validation", func(evidence *Evidence) { evidence.Rollup.ValidationResults = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := evidence
			candidate.RuntimeExecutions = append([]RuntimeExecutionEvidence(nil), evidence.RuntimeExecutions...)
			candidate.Rollup.ChildOutcomes = append([]ChildOutcome(nil), evidence.Rollup.ChildOutcomes...)
			candidate.Rollup.ValidationResults = append([]ValidationResult(nil), evidence.Rollup.ValidationResults...)
			test.mutate(&candidate)
			built, err := BuildEvidence(graph, candidate, verifier)
			if err != nil || built.Status != "deterministic_preparation_only" {
				t.Fatalf("built=%+v err=%v", built, err)
			}
		})
	}
	built, err := BuildEvidence(graph, evidence, verifier)
	if err != nil || built.Status != "real_run_recorded" || built.Digest == "" || len(built.Coordination) != 2 || len(built.CoordinationRecords) != 2 {
		t.Fatalf("built=%+v err=%v", built, err)
	}
}

func TestBuildEvidenceRejectsForeignRuntimeProfileOrAttempt(t *testing.T) {
	graph, evidence, _ := completeRuntimeEvidenceFixture(t)
	tests := []struct {
		name   string
		mutate func(*RuntimeExecutionEvidence)
	}{
		{"runtime", func(record *RuntimeExecutionEvidence) { record.RuntimeID = "claude" }},
		{"profile", func(record *RuntimeExecutionEvidence) { record.ModelProfileID = "foreign-profile" }},
		{"attempt", func(record *RuntimeExecutionEvidence) { record.AttemptID = "00000000-0000-4000-8000-999999999999" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := evidence
			candidate.RuntimeExecutions = append([]RuntimeExecutionEvidence(nil), evidence.RuntimeExecutions...)
			test.mutate(&candidate.RuntimeExecutions[0])
			if _, err := BuildEvidence(graph, candidate, verifier); err == nil {
				t.Fatal("foreign runtime evidence accepted")
			}
		})
	}
}

func TestBuildEvidenceRequiresObservedConcurrentExecution(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Graph, *Evidence, string, string)
		status string
	}{
		{"independent overlapping", func(*Graph, *Evidence, string, string) {}, "real_run_recorded"},
		{"independent sequential", func(graph *Graph, evidence *Evidence, codex, claude string) {
			setAttemptWindow(graph, evidence, codex, 30, 40)
			setAttemptWindow(graph, evidence, claude, 50, 60)
		}, "deterministic_preparation_only"},
		{"end equals start", func(graph *Graph, evidence *Evidence, codex, claude string) {
			setAttemptWindow(graph, evidence, codex, 30, 40)
			setAttemptWindow(graph, evidence, claude, 40, 50)
		}, "deterministic_preparation_only"},
		{"start equals end", func(graph *Graph, evidence *Evidence, codex, claude string) {
			setAttemptWindow(graph, evidence, codex, 40, 50)
			setAttemptWindow(graph, evidence, claude, 30, 40)
		}, "deterministic_preparation_only"},
		{"dependent overlapping", func(graph *Graph, _ *Evidence, codex, claude string) {
			for index := range graph.Children {
				if graph.Children[index].ExecutionID == claude {
					graph.Children[index].Envelope.Dependencies = append(append([]string(nil), graph.Children[index].Envelope.Dependencies...), codex)
				}
			}
		}, "deterministic_preparation_only"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, evidence, _ := completeRuntimeEvidenceFixture(t)
			test.mutate(&graph, &evidence, evidence.RuntimeExecutions[0].ChildID, evidence.RuntimeExecutions[1].ChildID)
			built, err := BuildEvidence(graph, evidence, verifier)
			if err != nil || built.Status != test.status {
				t.Fatalf("built=%+v err=%v", built, err)
			}
		})
	}
}

func TestBuildEvidenceRequiresStructuredCoordinationExchange(t *testing.T) {
	unknownRequest := "00000000-0000-4000-8000-000000000999"
	tests := []struct {
		name   string
		build  func(*journey) []coordination.Record
		status string
	}{
		{"question and correlated answer", func(j *journey) []coordination.Record {
			question := j.publish(coordination.QuestionRequest, j.codex, field("question", "Which contract?"))
			return []coordination.Record{question, j.publish(coordination.Answer, j.claude, field("answer", "Use v1"), field("question_reference", question.RecordID))}
		}, "real_run_recorded"},
		{"contract proposal and correlated acceptance", func(j *journey) []coordination.Record {
			proposal := j.publish(coordination.ContractProposal, j.codex, field("contract", "v1"))
			return []coordination.Record{proposal, j.publish(coordination.ContractAcceptance, j.claude, field("contract_reference", proposal.RecordID), field("decision", "accepted"))}
		}, "real_run_recorded"},
		{"only question", func(j *journey) []coordination.Record {
			return []coordination.Record{j.publish(coordination.QuestionRequest, j.codex, field("question", "Which contract?"))}
		}, "deterministic_preparation_only"},
		{"only answer", func(j *journey) []coordination.Record {
			question := j.publish(coordination.QuestionRequest, j.codex, field("question", "Which contract?"))
			return []coordination.Record{j.publish(coordination.Answer, j.claude, field("answer", "Use v1"), field("question_reference", question.RecordID))}
		}, "deterministic_preparation_only"},
		{"proposal without acceptance", func(j *journey) []coordination.Record {
			return []coordination.Record{j.publish(coordination.ContractProposal, j.codex, field("contract", "v1"))}
		}, "deterministic_preparation_only"},
		{"proposal with rejected decision", func(j *journey) []coordination.Record {
			proposal := j.publish(coordination.ContractProposal, j.codex, field("contract", "v1"))
			return []coordination.Record{proposal, j.publish(coordination.ContractAcceptance, j.claude, field("contract_reference", proposal.RecordID), field("decision", "rejected"))}
		}, "deterministic_preparation_only"},
		{"answer to nonexistent question", func(j *journey) []coordination.Record {
			question := j.publish(coordination.QuestionRequest, j.codex, field("question", "Which contract?"))
			return []coordination.Record{question, j.publish(coordination.Answer, j.claude, field("answer", "Use v1"), field("question_reference", unknownRequest))}
		}, "deterministic_preparation_only"},
		{"acceptance of a question", func(j *journey) []coordination.Record {
			question := j.publish(coordination.QuestionRequest, j.codex, field("question", "Which contract?"))
			return []coordination.Record{question, j.publish(coordination.ContractAcceptance, j.claude, field("contract_reference", question.RecordID), field("decision", "accepted"))}
		}, "deterministic_preparation_only"},
		{"same child exchange", func(j *journey) []coordination.Record {
			question := j.publish(coordination.QuestionRequest, j.codex, field("question", "Which contract?"))
			return []coordination.Record{question, j.publish(coordination.Answer, j.codex, field("answer", "Use v1"), field("question_reference", question.RecordID))}
		}, "deterministic_preparation_only"},
		{"exchange outside runtime attempts", func(j *journey) []coordination.Record {
			codex, claude := j.codex, j.claude
			codex.AttemptID, claude.AttemptID = "", ""
			question := j.publish(coordination.QuestionRequest, codex, field("question", "Which contract?"))
			return []coordination.Record{question, j.publish(coordination.Answer, claude, field("answer", "Use v1"), field("question_reference", question.RecordID))}
		}, "deterministic_preparation_only"},
		{"no coordination", func(*journey) []coordination.Record { return nil }, "deterministic_preparation_only"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, evidence, _ := completeRuntimeEvidenceFixture(t)
			evidence.CoordinationRecords = encode(t, test.build(newJourney(t, graph, evidence))...)
			built, err := BuildEvidence(graph, evidence, verifier)
			if err != nil || built.Status != test.status {
				t.Fatalf("built=%+v err=%v", built, err)
			}
		})
	}
}

func TestBuildEvidenceRejectsForgedOrTamperedCoordination(t *testing.T) {
	graph, evidence, records := completeRuntimeEvidenceFixture(t)
	answer := records[1]
	tampered := answer
	tampered.Fields = []coordination.Field{{Name: "answer", Value: "Use v2"}, {Name: "question_reference", Value: records[0].RecordID}}
	arbitrary := answer
	arbitrary.Digest = DigestOf("answer")
	nonCanonical, err := json.MarshalIndent(answer, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		wire []byte
	}{
		{"content altered after digest", marshal(t, tampered)},
		{"arbitrary valid SHA-256 digest", marshal(t, arbitrary)},
		{"non-canonical encoding", append(nonCanonical, '\n')},
		{"unstructured bytes", []byte("User asks\nAssistant answers\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := verifier.VerifyCoordination(test.wire); err == nil {
				t.Fatal("verifier accepted non-canonical coordination")
			}
			candidate := evidence
			candidate.CoordinationRecords = [][]byte{evidence.CoordinationRecords[0], test.wire}
			if _, err := BuildEvidence(graph, candidate, verifier); err == nil {
				t.Fatal("forged coordination accepted")
			}
		})
	}
	t.Run("caller-asserted facts with syntactically valid identities", func(t *testing.T) {
		candidate := evidence
		candidate.CoordinationRecords = nil
		candidate.Coordination = []CoordinationEvidence{
			{RecordID: records[0].RecordID, Kind: CoordinationQuestionRequest, ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, ChildID: records[0].ChildID, AttemptID: records[0].AttemptID, Digest: DigestOf("question")},
			{RecordID: records[1].RecordID, Kind: CoordinationAnswer, ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, ChildID: records[1].ChildID, AttemptID: records[1].AttemptID, Digest: DigestOf("answer"), Reference: records[0].RecordID},
		}
		if built, err := BuildEvidence(graph, candidate, verifier); err == nil {
			t.Fatalf("caller-asserted coordination accepted: status=%s", built.Status)
		}
		candidate.CoordinationRecords = evidence.CoordinationRecords
		if _, err := BuildEvidence(graph, candidate, verifier); err == nil {
			t.Fatal("caller-asserted coordination accepted beside canonical records")
		}
	})
	t.Run("duplicate record", func(t *testing.T) {
		candidate := evidence
		candidate.CoordinationRecords = append(append([][]byte(nil), evidence.CoordinationRecords...), evidence.CoordinationRecords[1])
		if _, err := BuildEvidence(graph, candidate, verifier); err == nil {
			t.Fatal("duplicate coordination record accepted")
		}
	})
}

func TestBuildEvidenceRejectsForeignCoordinationLineage(t *testing.T) {
	foreign := "00000000-0000-4000-8000-000000000999"
	tests := []struct {
		name   string
		mutate func(Graph, *coordination.Record)
	}{
		{"foreign parent", func(_ Graph, record *coordination.Record) { record.ParentID = foreign }},
		{"foreign graph revision", func(_ Graph, record *coordination.Record) { record.GraphRevision++ }},
		{"foreign child", func(_ Graph, record *coordination.Record) { record.ChildID = foreign }},
		{"foreign attempt", func(_ Graph, record *coordination.Record) { record.AttemptID = foreign }},
		{"attempt of another child", func(graph Graph, record *coordination.Record) {
			for _, child := range graph.Children {
				if child.ExecutionID != record.ChildID && len(child.Attempts) > 0 {
					record.AttemptID = child.Attempts[0].AttemptID
				}
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph, evidence, records := completeRuntimeEvidenceFixture(t)
			answer := records[1]
			test.mutate(graph, &answer)
			wire := reseal(t, answer)
			if _, err := verifier.VerifyCoordination(wire); err != nil {
				t.Fatalf("resealed canonical record rejected by verifier: %v", err)
			}
			evidence.CoordinationRecords = [][]byte{evidence.CoordinationRecords[0], wire}
			if _, err := BuildEvidence(graph, evidence, verifier); err == nil {
				t.Fatal("foreign coordination lineage accepted")
			}
		})
	}
}

type journey struct {
	t             *testing.T
	graph         Graph
	store         *coordinationStore
	service       coordination.Service
	codex, claude RuntimeExecutionEvidence
}

func newJourney(t *testing.T, graph Graph, evidence Evidence) *journey {
	t.Helper()
	store := &coordinationStore{}
	next := 700
	allocate := func() (string, error) { next++; return fmt.Sprintf("00000000-0000-4000-8000-%012d", next), nil }
	service := coordination.New(store, allocate, func() time.Time { return time.Unix(35, 0).UTC() })
	return &journey{t: t, graph: graph, store: store, service: service, codex: evidence.RuntimeExecutions[0], claude: evidence.RuntimeExecutions[1]}
}

func (j *journey) publish(kind coordination.Kind, execution RuntimeExecutionEvidence, fields ...coordination.Field) coordination.Record {
	j.t.Helper()
	input := coordination.Input{Kind: kind, ParentID: j.graph.Parent.ExecutionID, ChildID: execution.ChildID, GraphRevision: j.graph.Parent.GraphRevision, AttemptID: execution.AttemptID, Provenance: coordination.Provenance{Product: "Axiom", Version: "dev", Revision: "abc123", SourceState: "clean"}, Fields: fields}
	if latest, exists, _ := j.store.Latest(context.Background(), input.ParentID, input.ChildID); exists {
		input.ExpectedRevision, input.PreviousDigest = latest.Revision, latest.Digest
	}
	record, err := j.service.Publish(context.Background(), j.graph, input)
	if err != nil {
		j.t.Fatalf("publish %s: %v", kind, err)
	}
	return record
}

type coordinationStore struct{ records []coordination.Record }

func (s *coordinationStore) Latest(_ context.Context, parentID, childID string) (coordination.Record, bool, error) {
	for index := len(s.records) - 1; index >= 0; index-- {
		if s.records[index].ParentID == parentID && s.records[index].ChildID == childID {
			return s.records[index], true, nil
		}
	}
	return coordination.Record{}, false, nil
}

func (s *coordinationStore) Publish(_ context.Context, record coordination.Record) error {
	s.records = append(s.records, record)
	return nil
}

func field(name, value string) coordination.Field {
	return coordination.Field{Name: name, Value: value}
}

func encode(t *testing.T, records ...coordination.Record) [][]byte {
	t.Helper()
	var wires [][]byte
	for _, record := range records {
		wire, err := coordination.Encode(record)
		if err != nil {
			t.Fatal(err)
		}
		wires = append(wires, wire)
	}
	return wires
}

func marshal(t *testing.T, record coordination.Record) []byte {
	t.Helper()
	wire, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return append(wire, '\n')
}

// reseal recomputes a record digest the way a forger would, producing a
// canonical record whose integrity holds but whose lineage is foreign.
func reseal(t *testing.T, record coordination.Record) []byte {
	t.Helper()
	record.Digest = ""
	wire, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(wire)
	record.Digest = hex.EncodeToString(digest[:])
	return encode(t, record)[0]
}

func setAttemptWindow(graph *Graph, evidence *Evidence, childID string, start, end int64) {
	startedAt, finishedAt := time.Unix(start, 0).UTC(), time.Unix(end, 0).UTC()
	for index := range graph.Children {
		if graph.Children[index].ExecutionID == childID {
			graph.Children[index].Attempts[0].StartedAt = &startedAt
			graph.Children[index].Attempts[0].FinishedAt = &finishedAt
		}
	}
	for index := range evidence.Dispatch {
		if evidence.Dispatch[index].ChildID == childID {
			evidence.Dispatch[index].StartedAt = startedAt
			evidence.Dispatch[index].EndedAt = finishedAt
		}
	}
}

// completeRuntimeEvidenceFixture returns a complete Codex + Claude journey
// whose coordination is a question/answer exchange published by
// coordination.Service and carried as canonical encoded records.
func completeRuntimeEvidenceFixture(t *testing.T) (Graph, Evidence, []coordination.Record) {
	t.Helper()
	graph := MustGraph(t)
	dispatch := []DispatchRecord{}
	runtimeExecutions := []RuntimeExecutionEvidence{}
	runtimeIndex := 0
	for index := range graph.Children {
		child := &graph.Children[index]
		if child.Envelope.IntegrationOwner {
			continue
		}
		runtimeID := "codex"
		if runtimeIndex == 1 {
			runtimeID = "claude"
		}
		startedAt := time.Unix(int64(30+5*runtimeIndex), 0).UTC()
		finishedAt := startedAt.Add(10 * time.Second)
		child.Envelope.Resolution.RuntimeID = runtimeID
		child.Envelope.Resolution.ModelProfileID = runtimeID + "-profile"
		attemptID := fmt.Sprintf("00000000-0000-4000-8000-%012d", 500+runtimeIndex)
		resultReference := "result:" + child.NodeKey
		child.Attempts = []Attempt{{AttemptID: attemptID, Number: 1, Status: AttemptSucceeded, StartedAt: &startedAt, FinishedAt: &finishedAt, ResultReference: resultReference, OutputDigest: DigestOf("output-" + runtimeID)}}
		dispatch = append(dispatch, DispatchRecord{ChildID: child.ExecutionID, AttemptID: attemptID, Status: AttemptSucceeded, StartedAt: startedAt, EndedAt: finishedAt})
		runtimeExecutions = append(runtimeExecutions, RuntimeExecutionEvidence{ChildID: child.ExecutionID, AttemptID: attemptID, RuntimeID: runtimeID, ModelProfileID: child.Envelope.Resolution.ModelProfileID, RuntimeVersion: runtimeID + "-1.0", ConfigurationRevision: child.Envelope.Resolution.ConfigurationRevision, ObservationRevision: child.Envelope.Resolution.ObservationRevision, InvocationDigest: DigestOf("invocation-" + runtimeID), ResultReference: resultReference})
		runtimeIndex++
	}
	if !ValidGraph(graph) || runtimeIndex < 2 {
		t.Fatal("invalid complete runtime graph")
	}
	rollup := RollupGraph(graph)
	rollup.Status = "success"
	rollup.IntegrationResultTree = DigestOf("integrated")
	rollup.ValidationResults = []ValidationResult{{CommandReference: "go-test", ExitCode: 0, OutputDigest: DigestOf("validation")}}
	rollup.References = []string{"integration:result"}
	for index := range rollup.ChildOutcomes {
		if rollup.ChildOutcomes[index].ChildID == rollup.IntegrationChildID {
			rollup.ChildOutcomes[index].Status = AttemptSucceeded
		}
	}
	evidence := Evidence{FormatVersion: 1, BaseRevision: "8a9ca19fd260bc19f5bb288b449e8dc6df618194", ConfigurationDigest: DigestOf("config"), ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Dispatch: dispatch, RuntimeExecutions: runtimeExecutions, Rollup: rollup, ValidationReferences: []string{"go-test"}, Limitations: []string{"usage unavailable"}}
	j := newJourney(t, graph, evidence)
	question := j.publish(coordination.QuestionRequest, j.codex, field("question", "Which contract?"))
	answer := j.publish(coordination.Answer, j.claude, field("answer", "Use v1"), field("question_reference", question.RecordID))
	evidence.CoordinationRecords = encode(t, question, answer)
	return graph, evidence, []coordination.Record{question, answer}
}
