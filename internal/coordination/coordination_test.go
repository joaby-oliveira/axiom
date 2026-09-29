package coordination

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/executiongraph"
)

type memoryStore struct{ records []Record }

func (s *memoryStore) Latest(_ context.Context, parentID, childID string) (Record, bool, error) {
	for index := len(s.records) - 1; index >= 0; index-- {
		if s.records[index].ParentID == parentID && s.records[index].ChildID == childID {
			return s.records[index], true, nil
		}
	}
	return Record{}, false, nil
}
func (s *memoryStore) Publish(_ context.Context, record Record) error {
	s.records = append(s.records, record)
	return nil
}

func TestPublishEveryStructuredKindWithLineage(t *testing.T) {
	graph := coordinationGraph(t)
	store := &memoryStore{}
	next := 100
	service := New(store, func() (string, error) { next++; return fmt.Sprintf("00000000-0000-4000-8000-%012d", next), nil }, func() time.Time { return time.Unix(20, 0).UTC() })
	kinds := []Kind{QuestionRequest, Answer, ContractProposal, ContractAcceptance, Blocker, DependencyResolution, ArtifactPublication, Progress, Result}
	previous := ""
	child := graph.Children[0]
	for index, kind := range kinds {
		field := kindField(kind)
		record, err := service.Publish(context.Background(), graph, Input{Kind: kind, ParentID: graph.Parent.ExecutionID, ChildID: child.ExecutionID, GraphRevision: graph.Parent.GraphRevision, ExpectedRevision: uint64(index), PreviousDigest: previous, Provenance: testProvenance(), Fields: []Field{field}})
		if err != nil || record.Revision != uint64(index+1) || record.Digest == "" {
			t.Fatalf("kind=%s record=%+v err=%v", kind, record, err)
		}
		previous = record.Digest
	}
}

func TestRejectsStaleControlRawChatCredentialAndOversize(t *testing.T) {
	graph := coordinationGraph(t)
	child := graph.Children[0]
	service := New(&memoryStore{}, nil, nil)
	tests := []Input{
		{Kind: Progress, ParentID: graph.Parent.ExecutionID, ChildID: child.ExecutionID, GraphRevision: graph.Parent.GraphRevision, ExpectedRevision: 1, Fields: []Field{{Name: "phase", Value: "start"}}},
		{Kind: Progress, ParentID: graph.Parent.ExecutionID, ChildID: child.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Fields: []Field{{Name: "authority", Value: "expand"}}},
		{Kind: Progress, ParentID: graph.Parent.ExecutionID, ChildID: child.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Fields: []Field{{Name: "raw_chat", Value: "hidden"}}},
		{Kind: Progress, ParentID: graph.Parent.ExecutionID, ChildID: child.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Fields: []Field{{Name: "credential", Value: "SECRET_SENTINEL"}}},
		{Kind: Progress, ParentID: graph.Parent.ExecutionID, ChildID: child.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Fields: []Field{{Name: "phase", Value: string(make([]byte, maxFieldBytes+1))}}},
	}
	for index := range tests {
		tests[index].Provenance = testProvenance()
		if _, err := service.Publish(context.Background(), graph, tests[index]); err == nil {
			t.Fatalf("input[%d] accepted", index)
		}
	}
}

func TestRejectsSensitiveCoordinationValuesWithoutKeywordFalsePositives(t *testing.T) {
	graph := coordinationGraph(t)
	child := graph.Children[0]
	tests := []struct {
		name  string
		field Field
	}{
		{"password assignment in answer", Field{Name: "answer", Value: "pass" + "word=synthetic-value"}},
		{"bearer authorization", Field{Name: "answer", Value: "Authorization: " + "Bearer synthetic-token-value"}},
		{"private key", Field{Name: "answer", Value: "-----BEGIN " + "PRIVATE KEY----- synthetic"}},
		{"RSA private key", Field{Name: "answer", Value: "-----BEGIN RSA " + "PRIVATE KEY----- synthetic"}},
		{"api key assignment", Field{Name: "summary", Value: "api_" + "key=synthetic-value"}},
		{"secret assignment", Field{Name: "contract", Value: "sec" + "ret: synthetic-value"}},
		{"raw chat marker", Field{Name: "answer", Value: "<|user|> copied transcript"}},
		{"multiline transcript", Field{Name: "answer", Value: "User asks\nAssistant answers"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := New(&memoryStore{}, nil, nil)
			input := Input{Kind: Answer, ParentID: graph.Parent.ExecutionID, ChildID: child.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Provenance: testProvenance(), Fields: []Field{test.field}}
			if test.field.Name == "summary" {
				input.Kind = Progress
			}
			if test.field.Name == "contract" {
				input.Kind = ContractProposal
			}
			if _, err := service.Publish(context.Background(), graph, input); err == nil {
				t.Fatalf("value accepted: %q", test.field.Value)
			}
		})
	}
	for _, value := range []string{"token budget governance is deferred", "secret handling must remain local"} {
		service := New(&memoryStore{}, nil, nil)
		_, err := service.Publish(context.Background(), graph, Input{Kind: Answer, ParentID: graph.Parent.ExecutionID, ChildID: child.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Provenance: testProvenance(), Fields: []Field{{Name: "answer", Value: value}}})
		if err != nil {
			t.Fatalf("legitimate value %q rejected: %v", value, err)
		}
	}
}

func TestUsageObservationIsTruthful(t *testing.T) {
	graph := coordinationGraph(t)
	child := graph.Children[0]
	service := New(&memoryStore{}, nil, nil)
	for _, usage := range []UsageObservation{{Source: "runtime", Status: UsageUnavailable}, {Source: "runtime", Unit: "tokens", Value: "12", Status: UsageMeasured}, {Source: "runtime", Unit: "tokens", Value: "partial", Status: UsagePartial}} {
		service = New(&memoryStore{}, nil, nil)
		_, err := service.Publish(context.Background(), graph, Input{Kind: Result, ParentID: graph.Parent.ExecutionID, ChildID: child.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Provenance: testProvenance(), Fields: []Field{{Name: "outcome", Value: "succeeded"}}, Usage: &usage})
		if err != nil {
			t.Fatalf("usage=%+v err=%v", usage, err)
		}
	}
	invalid := UsageObservation{Source: "runtime", Unit: "tokens", Value: "estimate", Status: UsageUnavailable}
	if _, err := service.Publish(context.Background(), graph, Input{Kind: Result, ParentID: graph.Parent.ExecutionID, ChildID: child.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Provenance: testProvenance(), Fields: []Field{{Name: "outcome", Value: "succeeded"}}, Usage: &invalid}); err == nil {
		t.Fatal("unavailable estimate accepted")
	}
}

func TestAcceptanceEvidenceProjectsVerifiedCorrelation(t *testing.T) {
	graph := coordinationGraph(t)
	child := graph.Children[0]
	next := 200
	service := New(&memoryStore{}, func() (string, error) { next++; return fmt.Sprintf("00000000-0000-4000-8000-%012d", next), nil }, func() time.Time { return time.Unix(20, 0).UTC() })
	proposal, err := service.Publish(context.Background(), graph, Input{Kind: ContractProposal, ParentID: graph.Parent.ExecutionID, ChildID: child.ExecutionID, GraphRevision: graph.Parent.GraphRevision, Provenance: testProvenance(), Fields: []Field{{Name: "contract", Value: "v1"}}})
	if err != nil {
		t.Fatal(err)
	}
	acceptance, err := service.Publish(context.Background(), graph, Input{Kind: ContractAcceptance, ParentID: graph.Parent.ExecutionID, ChildID: child.ExecutionID, GraphRevision: graph.Parent.GraphRevision, ExpectedRevision: 1, PreviousDigest: proposal.Digest, Provenance: testProvenance(), Fields: []Field{{Name: "contract_reference", Value: proposal.RecordID}, {Name: "decision", Value: "accepted"}}})
	if err != nil {
		t.Fatal(err)
	}
	for kind, boundary := range map[Kind]string{QuestionRequest: executiongraph.CoordinationQuestionRequest, Answer: executiongraph.CoordinationAnswer, ContractProposal: executiongraph.CoordinationContractProposal, ContractAcceptance: executiongraph.CoordinationContractAcceptance} {
		if string(kind) != boundary {
			t.Fatalf("kind %q diverges from acceptance boundary %q", kind, boundary)
		}
	}
	wire, err := Encode(acceptance)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := AcceptanceVerifier{}.VerifyCoordination(wire)
	if err != nil || evidence.Kind != executiongraph.CoordinationContractAcceptance || evidence.Reference != proposal.RecordID || evidence.Decision != executiongraph.CoordinationAccepted || evidence.Digest != acceptance.Digest || evidence.ParentID != graph.Parent.ExecutionID || evidence.ChildID != child.ExecutionID || evidence.GraphRevision != graph.Parent.GraphRevision {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
	forged := acceptance
	forged.Fields = []Field{{Name: "contract_reference", Value: "00000000-0000-4000-8000-000000000999"}, {Name: "decision", Value: "accepted"}}
	forgedWire, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (AcceptanceVerifier{}).VerifyCoordination(append(forgedWire, '\n')); err == nil {
		t.Fatal("record with stale digest projected")
	}
	for _, wire := range [][]byte{nil, wire[:len(wire)-1], append(append([]byte(nil), wire...), wire...), []byte(strings.Replace(string(wire), `"formatVersion"`, `"extra":1,"formatVersion"`, 1))} {
		if _, err := Decode(wire); err == nil {
			t.Fatalf("non-canonical record decoded: %q", wire)
		}
	}
}

func kindField(kind Kind) Field {
	switch kind {
	case QuestionRequest:
		return Field{Name: "question", Value: "Which contract?"}
	case Answer:
		return Field{Name: "answer", Value: "Use v1"}
	case ContractProposal:
		return Field{Name: "contract", Value: "v1"}
	case ContractAcceptance:
		return Field{Name: "decision", Value: "accepted"}
	case Blocker:
		return Field{Name: "code", Value: "missing_input"}
	case DependencyResolution:
		return Field{Name: "resolution", Value: "artifact:1"}
	case ArtifactPublication:
		return Field{Name: "artifact_id", Value: "artifact:1"}
	case Progress:
		return Field{Name: "phase", Value: "testing"}
	default:
		return Field{Name: "outcome", Value: "succeeded"}
	}
}

func testProvenance() Provenance {
	return Provenance{Product: "Axiom", Version: "dev", Revision: "abc123", SourceState: "clean"}
}

func coordinationGraph(t *testing.T) executiongraph.Graph {
	t.Helper()
	effects := []executiongraph.Effect{{Kind: "integration", Target: "internal/api"}, {Kind: "repository-write", Target: "internal/api"}}
	wire, err := json.Marshal(effects)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(wire)
	parentID := "00000000-0000-4000-8000-000000000001"
	childID := "00000000-0000-4000-8000-000000000002"
	integrationID := "00000000-0000-4000-8000-000000000003"
	controls := executiongraph.ExecutionControls{Timeout: time.Minute, MaximumAttempts: 2}
	resolution := executiongraph.Resolution{RuntimeID: "codex", ModelProfileID: "profile-1", ConfigurationRevision: 1, ObservationRevision: 1}
	graph := executiongraph.Graph{FormatVersion: executiongraph.GraphFormatVersion, Parent: executiongraph.ParentExecution{ExecutionID: parentID, GraphRevision: 1, ProposalDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AuthorityDigest: hex.EncodeToString(digest[:]), AuthorityEffects: effects, IntegrationChild: integrationID, CreatedAt: time.Unix(1, 0).UTC()}, Children: []executiongraph.ChildExecution{
		{ExecutionID: childID, ParentID: parentID, GraphRevision: 1, NodeKey: "implementation", Envelope: executiongraph.ChildEnvelope{Capability: executiongraph.CapabilityRequest{Role: "implementation", Complexity: "low", Capabilities: []string{"go"}}, Inputs: []string{"plan"}, Outputs: []string{"result"}, Scope: executiongraph.Scope{ProjectID: "project-1", RepositoryKey: "main", Paths: []string{"internal/api"}}, Workspace: "/tmp/work-1", AllowedEffects: []executiongraph.Effect{{Kind: "repository-write", Target: "internal/api"}}, AuthorityReference: "authority:implementation", Controls: controls, Resolution: resolution}},
		{ExecutionID: integrationID, ParentID: parentID, GraphRevision: 1, NodeKey: "integration", Envelope: executiongraph.ChildEnvelope{Capability: executiongraph.CapabilityRequest{Role: "integration", Complexity: "high", Capabilities: []string{"validation"}}, Inputs: []string{"result"}, Outputs: []string{"delivery"}, Scope: executiongraph.Scope{ProjectID: "project-1", RepositoryKey: "main", Paths: []string{"internal/api"}}, Workspace: "/tmp/work-2", Dependencies: []string{childID}, AllowedEffects: []executiongraph.Effect{{Kind: "integration", Target: "internal/api"}}, AuthorityReference: "authority:integration", Controls: controls, ValidationOwner: true, IntegrationOwner: true, Resolution: resolution}},
	}}
	if !executiongraph.ValidGraph(graph) {
		t.Fatal("invalid coordination graph fixture")
	}
	return graph
}
