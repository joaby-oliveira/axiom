package executiongraph

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type memoryGraphStore struct{ graph Graph }

func (s *memoryGraphStore) Create(_ context.Context, graph Graph) error        { s.graph = graph; return nil }
func (s *memoryGraphStore) Load(_ context.Context, _, _ string) (Graph, error) { return s.graph, nil }

func TestGraphPublicationAllocatesStableLineageAndBoundedEnvelopes(t *testing.T) {
	proposal := mustProposal(t)
	store := &memoryGraphStore{}
	next := 0
	allocate := func() (string, error) {
		next++
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", next), nil
	}
	request := publicationRequest(proposal)
	graph, err := NewGraphService(store, allocate, func() time.Time { return time.Unix(10, 0).UTC() }).Publish(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if graph.Parent.ExecutionID == graph.Children[0].ExecutionID || graph.Parent.GraphRevision != 3 || len(graph.Children) != 3 || graph.Parent.IntegrationChild == "" {
		t.Fatalf("graph=%+v", graph)
	}
	for _, child := range graph.Children {
		if child.ParentID != graph.Parent.ExecutionID || child.GraphRevision != graph.Parent.GraphRevision || child.Envelope.AuthorityReference == "" || child.Envelope.Controls.MaximumAttempts != 2 {
			t.Fatalf("child=%+v", child)
		}
	}
	wire, err := EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeGraph(wire)
	if err != nil || decoded.Parent.ExecutionID != graph.Parent.ExecutionID {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}
}

func TestGraphPublicationRejectsStaleDigestAndAuthorityExpansion(t *testing.T) {
	proposal := mustProposal(t)
	request := publicationRequest(proposal)
	request.ExpectedDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	service := NewGraphService(&memoryGraphStore{}, nil, nil)
	if _, err := service.Publish(context.Background(), request); !errors.Is(err, ErrStaleGraph) {
		t.Fatalf("stale err=%v", err)
	}
	request = publicationRequest(proposal)
	request.ChildAuthorities["docs"] = append(request.ChildAuthorities["docs"], Effect{Kind: "repository-write", Target: "outside"})
	if _, err := service.Publish(context.Background(), request); !errors.Is(err, ErrAuthoritySubset) {
		t.Fatalf("authority err=%v", err)
	}
}

func mustProposal(t *testing.T) Proposal {
	t.Helper()
	proposal, err := NewPlanner(&capabilitySpy{}).Propose(context.Background(), testPlan())
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func publicationRequest(proposal Proposal) PublicationRequest {
	request := PublicationRequest{
		Proposal: proposal, ExpectedDigest: proposal.Digest, GraphRevision: 3,
		ChildAuthorities: map[string][]Effect{}, AuthorityReferences: map[string]string{}, Workspaces: map[string]string{}, Resolutions: map[string]Resolution{},
	}
	for _, node := range proposal.Nodes {
		request.ParentAuthority = append(request.ParentAuthority, node.Effects...)
		request.ChildAuthorities[node.Key] = append([]Effect(nil), node.Effects...)
		request.AuthorityReferences[node.Key] = "authority:" + node.Key
		request.Workspaces[node.Key] = "/tmp/axiom-s8/" + node.Key
		request.Resolutions[node.Key] = Resolution{RuntimeID: "codex", ModelProfileID: "profile-1", ConfigurationRevision: 7, ObservationRevision: 7}
	}
	return request
}
