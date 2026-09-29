package executiongraph

import (
	"context"
	"errors"
	"testing"
	"time"
)

type capabilitySpy struct {
	requests []CapabilityRequest
	reject   string
}

func (s *capabilitySpy) ValidateCapability(_ context.Context, request CapabilityRequest) error {
	s.requests = append(s.requests, request)
	for _, capability := range request.Capabilities {
		if capability == s.reject {
			return errors.New("unsupported")
		}
	}
	return nil
}

func TestPlannerDerivesVariableTopologyAndDeterministicDigest(t *testing.T) {
	spy := &capabilitySpy{}
	planner := NewPlanner(spy)
	plan := testPlan()
	first, err := planner.Propose(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.Work[0], plan.Work[1] = plan.Work[1], plan.Work[0]
	second, err := planner.Propose(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest || len(first.Nodes) != 3 || first.Nodes[0].Key != "api-contract" || first.Nodes[2].Key != "integrate" {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	if len(spy.requests) != 6 {
		t.Fatalf("capability requests = %d", len(spy.requests))
	}
}

func TestPlannerRejectsCycleMissingDependencyOrOrphan(t *testing.T) {
	tests := []struct {
		name string
		edit func(*ApprovedPlan)
		want error
	}{
		{"cycle", func(plan *ApprovedPlan) { plan.Work[0].Dependencies = []string{"integrate"} }, ErrCycle},
		{"missing", func(plan *ApprovedPlan) { plan.Work[0].Dependencies = []string{"missing"} }, ErrInvalidGraph},
		{"orphan", func(plan *ApprovedPlan) { plan.Work[2].Dependencies = []string{"api-contract"} }, ErrInvalidGraph},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := testPlan()
			test.edit(&plan)
			_, err := NewPlanner(&capabilitySpy{}).Propose(context.Background(), plan)
			if !errors.Is(err, test.want) {
				t.Fatalf("err=%v want=%v", err, test.want)
			}
		})
	}
}

func TestPlannerRejectsUnsafeParallelOverlapAndUnresolvableCapability(t *testing.T) {
	plan := testPlan()
	plan.Work[1].Effects = []Effect{{Kind: "repository-write", Target: "internal/shared"}}
	plan.Work[0].Effects = []Effect{{Kind: "repository-write", Target: "internal/shared/file.go"}}
	if _, err := NewPlanner(&capabilitySpy{}).Propose(context.Background(), plan); !errors.Is(err, ErrUnsafeOverlap) {
		t.Fatalf("overlap err=%v", err)
	}
	plan = testPlan()
	if _, err := NewPlanner(&capabilitySpy{reject: "documentation"}).Propose(context.Background(), plan); !errors.Is(err, ErrUnresolvable) {
		t.Fatalf("capability err=%v", err)
	}
}

func TestPlannerTreatsInjectionAsRejectedDataWithNoEffects(t *testing.T) {
	spy := &capabilitySpy{}
	plan := testPlan()
	plan.Work[0].Key = "run; rm -rf"
	if _, err := NewPlanner(spy).Propose(context.Background(), plan); !errors.Is(err, ErrInvalidGraph) {
		t.Fatalf("err=%v", err)
	}
	if len(spy.requests) != 0 {
		t.Fatalf("validator called for invalid data: %d", len(spy.requests))
	}
}

func testPlan() ApprovedPlan {
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	controls := ExecutionControls{Timeout: 10 * time.Minute, MaximumAttempts: 2}
	return ApprovedPlan{Approved: true, PlanRevision: "plan-7", PlanDigest: digest, MaximumNodes: 5, Work: []WorkUnit{
		{Key: "api-contract", Capability: CapabilityRequest{Role: "contract", Complexity: "medium", Capabilities: []string{"go"}}, Inputs: []string{"plan"}, Outputs: []string{"contract"}, Scope: Scope{ProjectID: "project-1", RepositoryKey: "main", Paths: []string{"internal/api"}}, Effects: []Effect{{Kind: "repository-write", Target: "internal/api"}}, Controls: controls},
		{Key: "docs", Capability: CapabilityRequest{Role: "documentation", Complexity: "low", Capabilities: []string{"documentation"}}, Inputs: []string{"plan"}, Outputs: []string{"docs"}, Scope: Scope{ProjectID: "project-1", RepositoryKey: "main", Paths: []string{"docs/runtime.md"}}, Effects: []Effect{{Kind: "repository-write", Target: "docs/runtime.md"}}, Controls: controls},
		{Key: "integrate", Capability: CapabilityRequest{Role: "integration", Complexity: "high", Capabilities: []string{"go", "validation"}}, Dependencies: []string{"api-contract", "docs"}, Inputs: []string{"contract", "docs"}, Outputs: []string{"delivery"}, Scope: Scope{ProjectID: "project-1", RepositoryKey: "main", Paths: []string{"internal/api", "docs/runtime.md"}}, Effects: []Effect{{Kind: "integration", Target: "internal/api"}, {Kind: "integration", Target: "docs/runtime.md"}}, ValidationOwner: true, IntegrationOwner: true, Controls: controls},
	}}
}
