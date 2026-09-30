package runtimeadapter

import (
	"context"
	"github.com/rgomids/axiom/internal/testfs"
	"reflect"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/executiongraph"
	"github.com/rgomids/axiom/internal/runtimeprofile"
)

type credentialFake struct{ references []string }

func (f *credentialFake) ResolveEnvironment(_ context.Context, reference string) ([]string, error) {
	f.references = append(f.references, reference)
	return []string{"RUNTIME_TOKEN=SECRET_SENTINEL"}, nil
}

func TestInventoryExposesOnlyObservedCodexAndClaude(t *testing.T) {
	observedAt := time.Unix(1, 0).UTC()
	inventory, err := NewInventory([]runtimeprofile.Observation{{RuntimeID: "codex", Adapter: "codex", Installed: true, Available: true, Revision: 1, ObservedAt: observedAt}, {RuntimeID: "claude", Adapter: "claude", Installed: true, Available: false, Revision: 1, ObservedAt: observedAt}})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := inventory.Observe(context.Background(), "claude")
	if err != nil || observation.Available {
		t.Fatalf("observation=%+v err=%v", observation, err)
	}
	if _, err := inventory.Observe(context.Background(), "other"); err == nil {
		t.Fatal("unknown adapter observed")
	}
}

func TestConcreteResolversProduceExplicitArgvAndResolveCredentialByReference(t *testing.T) {
	credentials := &credentialFake{}
	resolver, err := NewInvocationResolver([]CommandProfile{
		{RuntimeID: "codex", ModelProfileID: "codex-high", Executable: testfs.Path("/opt/bin/codex"), Model: "local-codex-model", Arguments: []string{"--json"}, Environment: []string{"PATH=/opt/bin"}, CredentialReference: "keychain:codex", OutputMax: 1024},
		{RuntimeID: "claude", ModelProfileID: "claude-high", Executable: testfs.Path("/opt/bin/claude"), Model: "local-claude-model", Arguments: []string{"--output-format", "json"}, Environment: []string{"PATH=/opt/bin"}, OutputMax: 1024},
	}, credentials)
	if err != nil {
		t.Fatal(err)
	}
	codex := adapterChild("codex", "codex-high")
	invocation, err := resolver.ResolveInvocation(context.Background(), codex)
	if err != nil || !reflect.DeepEqual(invocation.Argv, []string{testfs.Path("/opt/bin/codex"), "exec", "--model", "local-codex-model", "--json"}) || invocation.CWD != codex.Envelope.Workspace {
		t.Fatalf("invocation=%+v err=%v", invocation, err)
	}
	if !reflect.DeepEqual(credentials.references, []string{"keychain:codex"}) {
		t.Fatalf("references=%v", credentials.references)
	}
	claude := adapterChild("claude", "claude-high")
	invocation, err = resolver.ResolveInvocation(context.Background(), claude)
	if err != nil || !reflect.DeepEqual(invocation.Argv, []string{testfs.Path("/opt/bin/claude"), "--print", "--model", "local-claude-model", "--output-format", "json"}) {
		t.Fatalf("invocation=%+v err=%v", invocation, err)
	}
}

func adapterChild(runtimeID, profileID string) executiongraph.ChildExecution {
	return executiongraph.ChildExecution{Envelope: executiongraph.ChildEnvelope{Workspace: "/tmp/isolated", Resolution: executiongraph.Resolution{RuntimeID: runtimeID, ModelProfileID: profileID, ConfigurationRevision: 1, ObservationRevision: 1}}}
}
