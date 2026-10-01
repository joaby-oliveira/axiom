package projectapp_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rgomids/axiom/internal/manifest"
	"github.com/rgomids/axiom/internal/project"
	"github.com/rgomids/axiom/internal/projectapp"
)

const editProjectID = "123e4567-e89b-42d3-a456-426614174000"

type editFixture struct {
	selection projectapp.EditSelection
	failure   projectapp.EditFailure
	selectors []string
}

func (f *editFixture) SelectForEdit(_ context.Context, selector string) (projectapp.EditSelection, projectapp.EditFailure) {
	f.selectors = append(f.selectors, selector)
	if f.failure != projectapp.EditOK {
		return projectapp.EditSelection{}, f.failure
	}
	return f.selection, projectapp.EditOK
}

type checkoutFixture struct{ observed []string }

func (c *checkoutFixture) ObserveCheckout(_ context.Context, request projectapp.CheckoutRequest) (projectapp.CheckoutFacts, []projectapp.Issue) {
	c.observed = append(c.observed, request.ExplicitPath)
	if strings.Contains(request.ExplicitPath, "missing") {
		return projectapp.CheckoutFacts{}, []projectapp.Issue{{Code: projectapp.InvalidSnapshot}}
	}
	return projectapp.CheckoutFacts{CanonicalIdentity: "fs:" + request.ExplicitPath, Observation: projectapp.Observation{Availability: projectapp.Unverified, Basis: projectapp.NotChecked}}, nil
}

var observedAt = time.Unix(1_788_000_000, 0)

// localCodecFixture is a pure stand-in for the local adapter codec: it binds
// every local field, including the recorded portable revision, into the wire.
type localCodecFixture struct{}

func (localCodecFixture) EncodeLocal(state projectapp.LocalRecordState) ([]byte, []projectapp.Issue) {
	revision, ok := state.PortableRevision.Digest()
	keys := map[string]bool{}
	for _, binding := range state.Repositories {
		if keys[binding.RepositoryKey] {
			ok = false
		}
		keys[binding.RepositoryKey] = true
	}
	wire, err := json.Marshal(struct {
		State    projectapp.LocalRecordState
		Revision [32]byte
	}{state, revision})
	if !ok || err != nil {
		return nil, []projectapp.Issue{{Code: projectapp.InvalidPreview}}
	}
	return wire, nil
}

// newEditFixture builds a complete, coherent installed Project whose portable
// and local state both carry fields unrelated to configure intent.
func newEditFixture(t *testing.T) *editFixture {
	t.Helper()
	state := project.State{
		SchemaVersion: 1, ID: editProjectID, Slug: "sample", Name: "Sample",
		Repositories: project.Configured([]project.Repository{
			{Key: "api", Remote: project.Configured("https://github.com/owner/api.git")},
			{Key: "web"},
		}),
		Runtime: project.Configured(project.Runtime{ID: "codex"}),
		Providers: project.Configured([]project.Provider{
			{Key: "chat", ID: "slack"},
			{Key: "work-items", ID: "github"},
		}),
		Integrations: project.Configured([]project.Integration{
			{Key: "chat", ProviderRef: project.Configured("chat"), CredentialRef: project.Configured("chat-token")},
			{Key: "work-items", ProviderRef: project.Configured("work-items"), Capabilities: project.Configured([]string{"work-item"})},
		}),
		CredentialReferences: project.Configured([]project.CredentialReference{{Key: "chat-token", SourceHint: project.Configured("keychain")}}),
		BusinessContext:      project.Configured(project.BusinessContext{Text: project.Configured("Unrelated portable context")}),
	}
	current, issues := project.New(state)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	wire, codecIssues := manifest.Codec{}.Encode(current)
	if len(codecIssues) != 0 {
		t.Fatal(codecIssues)
	}
	snapshot, snapshotIssues := projectapp.ReadSnapshot(manifest.Codec{}, wire, nil)
	if len(snapshotIssues) != 0 {
		t.Fatal(snapshotIssues)
	}
	record := projectapp.LocalRecordState{
		ProjectID: editProjectID, ObservedSlug: "sample", SourceLocation: "/portable/sample",
		PortableRevision: snapshot.Revision(), ArtifactDigests: snapshot.Digests(),
		Repositories: []projectapp.RepositoryBinding{
			{RepositoryKey: "api", ExplicitPath: "/work/api", CanonicalIdentity: "fs:/work/api", Observation: projectapp.Observation{Availability: projectapp.Available, Basis: projectapp.PresentMetadata, ObservedAt: observedAt}},
			{RepositoryKey: "web", ExplicitPath: "/work/web-broken", CanonicalIdentity: "fs:/work/web-broken", Observation: projectapp.Observation{Availability: projectapp.Unavailable, Basis: projectapp.MissingMetadata, ObservedAt: observedAt}},
		},
		Credentials: []projectapp.CredentialBinding{{ReferenceKey: "chat-token", SourceKind: "keychain", ItemReference: "axiom/chat-item"}},
		Runtime:     projectapp.RuntimeBinding{RuntimeID: "codex", ExplicitPath: "/opt/runtime-secret-location", Observation: projectapp.Observation{Availability: projectapp.Available, Basis: projectapp.PresentMetadata, ObservedAt: observedAt}},
		Attempt:     projectapp.AttemptMetadata{Correlation: "attempt-correlation-marker", At: observedAt},
	}
	localWire, localIssues := localCodecFixture{}.EncodeLocal(record)
	if len(localIssues) != 0 {
		t.Fatal(localIssues)
	}
	localDigest := sha256.Sum256(localWire)
	portableDigest, _ := snapshot.Revision().Digest()
	return &editFixture{selection: projectapp.EditSelection{
		Portable: snapshot, Local: record, LocalWire: localWire,
		PortableDestination: "/portable/sample", LocalDestination: "/state/projects/" + editProjectID,
		PortableRevision: hex.EncodeToString(portableDigest[:]), LocalRevision: hex.EncodeToString(localDigest[:]),
	}}
}

func (f *editFixture) preview(t *testing.T, intent projectapp.EditIntent) (projectapp.EditProposal, projectapp.EditFailure) {
	t.Helper()
	if intent.Selector == "" {
		intent.Selector = "sample"
	}
	return projectapp.PreviewEdit(context.Background(), projectapp.EditPorts{Source: f, Checkouts: &checkoutFixture{}, Manifest: manifest.Codec{}, Local: localCodecFixture{}}, intent)
}

func mustPreview(t *testing.T, f *editFixture, intent projectapp.EditIntent) projectapp.EditProposal {
	t.Helper()
	proposal, failure := f.preview(t, intent)
	if failure != projectapp.EditOK {
		t.Fatalf("edit failure = %d", failure)
	}
	return proposal
}

func set(value string) projectapp.OptionalText {
	return projectapp.OptionalText{Supplied: true, Value: value}
}

func workItemProvider(t *testing.T, p project.Project) (string, bool) {
	t.Helper()
	providers, _ := p.State().Providers.Value()
	for _, provider := range providers {
		if provider.Key == "work-items" {
			return provider.ID, true
		}
	}
	return "", false
}

func effectCodes(effects []projectapp.EditEffect) []string {
	codes := make([]string, 0, len(effects))
	for _, effect := range effects {
		code := effect.Scope + ":" + effect.Code
		if effect.Key != "" {
			code += ":" + effect.Key
		}
		codes = append(codes, code)
	}
	return codes
}

func TestEditScalarPreserveSetAndRemove(t *testing.T) {
	f := newEditFixture(t)
	current := f.selection.Portable.Project()

	preserved := mustPreview(t, f, projectapp.EditIntent{})
	if !preserved.Project().Equivalent(current) || len(preserved.Preview().Effects) != 0 || string(preserved.LocalWire()) != string(f.selection.LocalWire) {
		t.Fatalf("empty intent changed state: effects=%v", preserved.Preview().Effects)
	}
	if preserved.Preview().Name != "Sample" {
		t.Fatal("name was not preserved")
	}

	renamed := mustPreview(t, f, projectapp.EditIntent{Name: set("Renamed")})
	if renamed.Project().State().Name != "Renamed" || renamed.Project().State().Slug != "sample" || renamed.Project().State().ID != editProjectID {
		t.Fatal("name set changed identity or failed")
	}
	if id, ok := workItemProvider(t, renamed.Project()); !ok || id != "github" {
		t.Fatal("name set did not preserve provider")
	}

	linear := mustPreview(t, f, projectapp.EditIntent{WorkItemProvider: set("linear")})
	if id, ok := workItemProvider(t, linear.Project()); !ok || id != "linear" || linear.Preview().Capability.Readiness != projectapp.CapabilityUnsupported {
		t.Fatalf("provider set = %q readiness=%s", id, linear.Preview().Capability.Readiness)
	}
	if linear.Project().State().Name != "Sample" {
		t.Fatal("provider set did not preserve name")
	}

	removed := mustPreview(t, f, projectapp.EditIntent{RemoveWorkItemProvider: true})
	if _, ok := workItemProvider(t, removed.Project()); ok || removed.Preview().Capability.Readiness != projectapp.CapabilityMissing {
		t.Fatal("provider remove kept work-items provider")
	}
	integrations, _ := removed.Project().State().Integrations.Value()
	if len(integrations) != 1 || integrations[0].Key != "chat" {
		t.Fatalf("provider remove did not preserve unrelated integration: %+v", integrations)
	}
	providers, _ := removed.Project().State().Providers.Value()
	if len(providers) != 1 || providers[0].Key != "chat" {
		t.Fatalf("provider remove did not preserve unrelated provider: %+v", providers)
	}
	again, failure := f.preview(t, projectapp.EditIntent{Selector: "sample", RemoveWorkItemProvider: true})
	if failure != projectapp.EditOK || !again.Project().Equivalent(removed.Project()) {
		t.Fatal("provider removal is not deterministic")
	}

	if _, failure := f.preview(t, projectapp.EditIntent{WorkItemProvider: set("github"), RemoveWorkItemProvider: true}); failure != projectapp.EditInvalidIntent {
		t.Fatalf("provider set+remove failure = %d", failure)
	}
	if len(f.selectors) != 5 {
		t.Fatalf("set+remove conflict reached state selection: %v", f.selectors)
	}
}

func TestEditRemovingOnlyProviderUsesUnconfiguredDeclarations(t *testing.T) {
	f := newEditFixture(t)
	state := f.selection.Portable.Project().State()
	state.Providers = project.Configured([]project.Provider{{Key: "work-items", ID: "github"}})
	state.Integrations = project.Configured([]project.Integration{{Key: "work-items", ProviderRef: project.Configured("work-items"), Capabilities: project.Configured([]string{"work-item"})}})
	f.replacePortable(t, state)
	removed := mustPreview(t, f, projectapp.EditIntent{RemoveWorkItemProvider: true})
	if removed.Project().State().Providers.Form() != project.NotConfigured || removed.Project().State().Integrations.Form() != project.NotConfigured {
		t.Fatal("removing the only provider did not match CREATE's unconfigured form")
	}
	unchanged := mustPreview(t, &editFixture{selection: f.selection}, projectapp.EditIntent{Name: set("Sample")})
	if len(unchanged.Preview().Effects) != 0 {
		t.Fatalf("equivalent explicit name produced effects %v", unchanged.Preview().Effects)
	}
}

// replacePortable swaps the fixture's portable snapshot and re-records the
// local portable revision so the selection stays coherent.
func (f *editFixture) replacePortable(t *testing.T, state project.State) {
	t.Helper()
	p, issues := project.New(state)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	wire, codecIssues := manifest.Codec{}.Encode(p)
	if len(codecIssues) != 0 {
		t.Fatal(codecIssues)
	}
	snapshot, snapshotIssues := projectapp.ReadSnapshot(manifest.Codec{}, wire, nil)
	if len(snapshotIssues) != 0 {
		t.Fatal(snapshotIssues)
	}
	f.selection.Portable = snapshot
	f.selection.Local.PortableRevision, f.selection.Local.ArtifactDigests = snapshot.Revision(), snapshot.Digests()
	localWire, localIssues := localCodecFixture{}.EncodeLocal(f.selection.Local)
	if len(localIssues) != 0 {
		t.Fatal(localIssues)
	}
	portableDigest, _ := snapshot.Revision().Digest()
	localDigest := sha256.Sum256(localWire)
	f.selection.LocalWire = localWire
	f.selection.PortableRevision, f.selection.LocalRevision = hex.EncodeToString(portableDigest[:]), hex.EncodeToString(localDigest[:])
}

func TestEditRepositoryAddUpdateRemoveByStableKey(t *testing.T) {
	f := newEditFixture(t)
	proposal := mustPreview(t, f, projectapp.EditIntent{
		RepositoryUpserts:  []projectapp.RepositoryUpsert{{Key: "web", Path: "/work/web-fixed"}, {Key: "docs", Path: "/work/docs/"}},
		RepositoryRemovals: []string{"api"},
	})
	repositories, _ := proposal.Project().State().Repositories.Value()
	if len(repositories) != 2 || repositories[0].Key != "docs" || repositories[1].Key != "web" {
		t.Fatalf("portable repositories = %+v", repositories)
	}
	if _, present := repositories[1].Remote.Value(); present {
		t.Fatal("update invented a portable remote")
	}
	bindings := proposal.Local().Repositories
	if len(bindings) != 2 || bindings[0].RepositoryKey != "docs" || bindings[0].ExplicitPath != "/work/docs" || bindings[1].RepositoryKey != "web" || bindings[1].ExplicitPath != "/work/web-fixed" || bindings[1].CanonicalIdentity != "fs:/work/web-fixed" {
		t.Fatalf("local bindings = %+v", bindings)
	}
	want := []string{
		"portable:update_portable_project",
		"portable:remove_portable_repository:api",
		"portable:add_portable_repository:docs",
		"local:update_local_record",
		"local:remove_local_binding:api",
		"local:add_local_binding:docs",
		"local:update_local_binding:web",
	}
	if got := effectCodes(proposal.Preview().Effects); !reflect.DeepEqual(got, want) {
		t.Fatalf("effects = %v", got)
	}
	changes := map[string]string{}
	for _, repository := range proposal.Preview().Repositories {
		changes[repository.Key] = repository.Change
	}
	if !reflect.DeepEqual(changes, map[string]string{"api": "removed", "docs": "added", "web": "updated"}) {
		t.Fatalf("repository changes = %v", changes)
	}
}

func TestEditRepositoryUpdatePreservesPortableFieldsAndOmittedBindings(t *testing.T) {
	f := newEditFixture(t)
	checkouts := &checkoutFixture{}
	proposal, failure := projectapp.PreviewEdit(context.Background(), projectapp.EditPorts{Source: f, Checkouts: checkouts, Manifest: manifest.Codec{}, Local: localCodecFixture{}}, projectapp.EditIntent{
		Selector:          "sample",
		RepositoryUpserts: []projectapp.RepositoryUpsert{{Key: "api", Path: "/work/api-moved"}},
	})
	if failure != projectapp.EditOK {
		t.Fatal(failure)
	}
	if !reflect.DeepEqual(checkouts.observed, []string{"/work/api-moved"}) {
		t.Fatalf("observed paths = %v; only explicitly supplied paths may be inspected", checkouts.observed)
	}
	if !proposal.Project().Equivalent(f.selection.Portable.Project()) || effectCodes(proposal.Preview().Effects)[0] != "local:update_local_record" {
		t.Fatalf("path-only update changed portable state: %v", proposal.Preview().Effects)
	}
	repositories, _ := proposal.Project().State().Repositories.Value()
	if remote, ok := repositories[0].Remote.Value(); !ok || remote != "https://github.com/owner/api.git" {
		t.Fatal("upsert lost existing portable Repository fields")
	}
	web := proposal.Local().Repositories[1]
	if web != f.selection.Local.Repositories[1] {
		t.Fatalf("omitted (broken) binding was not preserved exactly: %+v", web)
	}
}

func TestEditPreservesUnrelatedPortableAndLocalFields(t *testing.T) {
	f := newEditFixture(t)
	proposal := mustPreview(t, f, projectapp.EditIntent{Name: set("Renamed"), RepositoryUpserts: []projectapp.RepositoryUpsert{{Key: "docs", Path: "/work/docs"}}})
	before, after := f.selection.Portable.Project().State(), proposal.Project().State()
	for name, pair := range map[string][2]any{
		"runtime":              {before.Runtime, after.Runtime},
		"providers":            {before.Providers, after.Providers},
		"integrations":         {before.Integrations, after.Integrations},
		"credentialReferences": {before.CredentialReferences, after.CredentialReferences},
		"businessContext":      {before.BusinessContext, after.BusinessContext},
		"modelProfiles":        {before.ModelProfiles, after.ModelProfiles},
		"policies":             {before.Policies, after.Policies},
	} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Fatalf("portable %s not preserved", name)
		}
	}
	localBefore, localAfter := f.selection.Local, proposal.Local()
	if !reflect.DeepEqual(localBefore.Credentials, localAfter.Credentials) || localBefore.Runtime != localAfter.Runtime || localBefore.Attempt != localAfter.Attempt || localBefore.SourceLocation != localAfter.SourceLocation || localBefore.ObservedSlug != localAfter.ObservedSlug {
		t.Fatal("unrelated local metadata was not preserved in the internal candidate")
	}
	if localAfter.PortableRevision == localBefore.PortableRevision {
		t.Fatal("local candidate did not bind the new portable revision")
	}
	if !reflect.DeepEqual(localAfter.Repositories[:1], localBefore.Repositories[:1]) {
		t.Fatal("omitted api binding changed")
	}
}

func TestEditRejectsConflictingAndUnknownRepositoryOperations(t *testing.T) {
	f := newEditFixture(t)
	for name, intent := range map[string]projectapp.EditIntent{
		"upsert and remove same key": {RepositoryUpserts: []projectapp.RepositoryUpsert{{Key: "api", Path: "/work/api"}}, RepositoryRemovals: []string{"api"}},
		"duplicate upsert":           {RepositoryUpserts: []projectapp.RepositoryUpsert{{Key: "api", Path: "/work/a"}, {Key: "api", Path: "/work/b"}}},
		"duplicate removal":          {RepositoryRemovals: []string{"web", "web"}},
		"relative path":              {RepositoryUpserts: []projectapp.RepositoryUpsert{{Key: "api", Path: "work/api"}}},
		"invalid key":                {RepositoryUpserts: []projectapp.RepositoryUpsert{{Key: "API", Path: "/work/api"}}},
		"empty name":                 {Name: set("")},
		"invalid provider":           {WorkItemProvider: set("Git Hub")},
	} {
		t.Run(name, func(t *testing.T) {
			f.selectors = nil
			if _, failure := f.preview(t, intent); failure != projectapp.EditInvalidIntent || len(f.selectors) != 0 {
				t.Fatalf("failure=%d selections=%v", failure, f.selectors)
			}
		})
	}
	if _, failure := f.preview(t, projectapp.EditIntent{RepositoryRemovals: []string{"unknown"}}); failure != projectapp.EditUnknownRepository {
		t.Fatalf("unknown removal failure = %d", failure)
	}
	if _, failure := f.preview(t, projectapp.EditIntent{RepositoryUpserts: []projectapp.RepositoryUpsert{{Key: "docs", Path: "/work/missing"}}}); failure != projectapp.EditRepositoryUnavailable {
		t.Fatalf("unavailable path failure = %d", failure)
	}
}

func TestEditIsDeterministicAcrossOperationOrderAndSelector(t *testing.T) {
	f := newEditFixture(t)
	first := mustPreview(t, f, projectapp.EditIntent{Selector: "sample", RepositoryUpserts: []projectapp.RepositoryUpsert{{Key: "zeta", Path: "/work/zeta"}, {Key: "alpha", Path: "/work/alpha"}}, RepositoryRemovals: []string{"web", "api"}})
	second := mustPreview(t, f, projectapp.EditIntent{Selector: editProjectID, RepositoryUpserts: []projectapp.RepositoryUpsert{{Key: "alpha", Path: "/work/alpha"}, {Key: "zeta", Path: "/work/zeta"}}, RepositoryRemovals: []string{"api", "web"}})
	if !reflect.DeepEqual(first.Preview(), second.Preview()) || string(first.Manifest()) != string(second.Manifest()) || string(first.LocalWire()) != string(second.LocalWire()) {
		t.Fatal("equivalent intent through slug and UUID produced different candidates")
	}
	keys := []string{}
	for _, repository := range first.Preview().Repositories {
		keys = append(keys, repository.Key)
	}
	if !reflect.DeepEqual(keys, []string{"alpha", "api", "web", "zeta"}) {
		t.Fatalf("preview order = %v", keys)
	}
	if first.Preview().Digest == "" || first.Preview().Digest != second.Preview().Digest {
		t.Fatal("digest is not deterministic for equivalent intent")
	}
	again := mustPreview(t, f, projectapp.EditIntent{Selector: "sample", RepositoryUpserts: []projectapp.RepositoryUpsert{{Key: "zeta", Path: "/work/zeta"}, {Key: "alpha", Path: "/work/alpha"}}, RepositoryRemovals: []string{"web", "api"}})
	if again.Preview().Digest != first.Preview().Digest {
		t.Fatal("digest is not stable across repeated previews")
	}
}

// The digest binds every component of the authority envelope, including local
// candidate fields that the user-visible preview never shows.
func TestEditDigestBindsEveryEnvelopeComponent(t *testing.T) {
	intent := projectapp.EditIntent{Name: set("Renamed")}
	base := mustPreview(t, newEditFixture(t), intent).Preview().Digest
	reencode := func(t *testing.T, f *editFixture) {
		t.Helper()
		wire, issues := localCodecFixture{}.EncodeLocal(f.selection.Local)
		if len(issues) != 0 {
			t.Fatal(issues)
		}
		digest := sha256.Sum256(wire)
		f.selection.LocalWire, f.selection.LocalRevision = wire, hex.EncodeToString(digest[:])
	}
	for name, change := range map[string]struct {
		mutate func(*testing.T, *editFixture)
		intent projectapp.EditIntent
	}{
		"portable candidate": {intent: projectapp.EditIntent{Name: set("Other")}},
		"local binding":      {intent: projectapp.EditIntent{Name: set("Renamed"), RepositoryUpserts: []projectapp.RepositoryUpsert{{Key: "web", Path: "/work/web-fixed"}}}},
		"local destination": {intent: intent, mutate: func(_ *testing.T, f *editFixture) {
			f.selection.LocalDestination = "/other-state/projects/" + editProjectID
		}},
		"portable destination": {intent: intent, mutate: func(t *testing.T, f *editFixture) {
			f.selection.PortableDestination, f.selection.Local.SourceLocation = "/elsewhere/sample", "/elsewhere/sample"
			reencode(t, f)
		}},
		"hidden credential reference": {intent: intent, mutate: func(t *testing.T, f *editFixture) {
			f.selection.Local.Credentials = []projectapp.CredentialBinding{{ReferenceKey: "chat-token", SourceKind: "keychain", ItemReference: "axiom/other-item"}}
			reencode(t, f)
		}},
		"hidden runtime binding": {intent: intent, mutate: func(t *testing.T, f *editFixture) {
			f.selection.Local.Runtime.ExplicitPath = "/opt/other-runtime"
			reencode(t, f)
		}},
		"hidden preserved binding observation": {intent: intent, mutate: func(t *testing.T, f *editFixture) {
			f.selection.Local.Repositories[0].Observation.ObservedAt = time.Unix(1_788_003_600, 0)
			reencode(t, f)
		}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newEditFixture(t)
			if change.mutate != nil {
				change.mutate(t, f)
			}
			if digest := mustPreview(t, f, change.intent).Preview().Digest; digest == base {
				t.Fatal("digest did not change")
			}
		})
	}
}

func TestEditPreviewIsCompleteSafeAndDigestBindsInternalLocalCandidate(t *testing.T) {
	f := newEditFixture(t)
	proposal := mustPreview(t, f, projectapp.EditIntent{RepositoryRemovals: []string{"web"}})
	preview := proposal.Preview()
	if preview.Mode != projectapp.EditMode || preview.ProjectID != editProjectID || preview.PortableDestination != "/portable/sample" || preview.LocalDestination != "/state/projects/"+editProjectID || preview.PortableRevision != f.selection.PortableRevision || preview.LocalRevision != f.selection.LocalRevision {
		t.Fatalf("preview identity/destinations/observations = %+v", preview)
	}
	if preview.PortableManifest != string(proposal.Manifest()) {
		t.Fatal("preview did not carry the complete portable candidate")
	}
	for _, expected := range []string{"slug: sample", "name: Sample", "key: api", "Unrelated portable context", "chat-token"} {
		if !strings.Contains(preview.PortableManifest, expected) {
			t.Fatalf("complete portable result missing %q:\n%s", expected, preview.PortableManifest)
		}
	}
	if strings.Contains(preview.PortableManifest, "key: web") || strings.Contains(preview.PortableManifest, "/work/") {
		t.Fatal("portable candidate kept removed association or leaked local paths")
	}
	wire, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{"axiom/chat-item", "/opt/runtime-secret-location", "attempt-correlation-marker", "/work/api", "observedAt", "availability", "present_metadata"} {
		if strings.Contains(string(wire), hidden) {
			t.Fatalf("user-visible preview exposed unrelated local metadata %q: %s", hidden, wire)
		}
	}
	if !strings.Contains(string(wire), "/work/web-broken") {
		t.Fatal("removed binding path is not reviewable")
	}
	if !strings.Contains(string(proposal.LocalWire()), "axiom/chat-item") || !strings.Contains(string(proposal.LocalWire()), "attempt-correlation-marker") {
		t.Fatal("internal local candidate lost hidden metadata")
	}
	want := []string{"portable:update_portable_project", "portable:remove_portable_repository:web", "local:update_local_record", "local:remove_local_binding:web"}
	if got := effectCodes(preview.Effects); !reflect.DeepEqual(got, want) {
		t.Fatalf("removal effects = %v", got)
	}

	// Hidden local metadata is bound by the digest even though it is not shown.
	changed := newEditFixture(t)
	changed.selection.Local.Attempt.Correlation = "attempt-correlation-other"
	localWire, issues := localCodecFixture{}.EncodeLocal(changed.selection.Local)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	digest := sha256.Sum256(localWire)
	changed.selection.LocalWire, changed.selection.LocalRevision = localWire, hex.EncodeToString(digest[:])
	other := mustPreview(t, changed, projectapp.EditIntent{RepositoryRemovals: []string{"web"}})
	if other.Preview().Digest == preview.Digest {
		t.Fatal("digest did not bind hidden local metadata")
	}
}

func TestEditFailsClosedOnSelectionAndIncoherentState(t *testing.T) {
	for _, failure := range []projectapp.EditFailure{projectapp.EditProjectNotFound, projectapp.EditProjectAmbiguous, projectapp.EditStateUnsafe, projectapp.EditRecoveryRequired} {
		f := newEditFixture(t)
		f.failure = failure
		if _, got := f.preview(t, projectapp.EditIntent{Name: set("Renamed")}); got != failure {
			t.Fatalf("selection failure %d became %d", failure, got)
		}
	}
	for name, mutate := range map[string]func(*projectapp.EditSelection){
		"stale local portable revision": func(s *projectapp.EditSelection) {
			s.Local.PortableRevision = projectapp.RecordedPortableRevision([32]byte{9})
		},
		"source not the read destination": func(s *projectapp.EditSelection) { s.Local.SourceLocation = "/elsewhere/sample" },
		"slug drift":                      func(s *projectapp.EditSelection) { s.Local.ObservedSlug = "other" },
		"local revision":                  func(s *projectapp.EditSelection) { s.LocalRevision = "0000" },
		"binding key drift":               func(s *projectapp.EditSelection) { s.Local.Repositories = s.Local.Repositories[:1] },
		"missing local bytes":             func(s *projectapp.EditSelection) { s.LocalWire = nil },
	} {
		t.Run(name, func(t *testing.T) {
			f := newEditFixture(t)
			mutate(&f.selection)
			if _, failure := f.preview(t, projectapp.EditIntent{}); failure != projectapp.EditStateUnsafe {
				t.Fatalf("failure = %d", failure)
			}
		})
	}
}

func TestEditCandidateUsesProjectProposeInvariants(t *testing.T) {
	f := newEditFixture(t)
	state := f.selection.Portable.Project().State()
	integrations, _ := state.Integrations.Value()
	// A retained unrelated integration referencing the work-items Provider makes
	// its removal invalid; the complete candidate must be rejected.
	integrations = append(integrations, project.Integration{Key: "reports", ProviderRef: project.Configured("work-items")})
	state.Integrations = project.Configured(integrations)
	f.replacePortable(t, state)
	if _, failure := f.preview(t, projectapp.EditIntent{RemoveWorkItemProvider: true}); failure != projectapp.EditCandidateInvalid {
		t.Fatalf("dangling reference failure = %d", failure)
	}
}

// Preview readiness follows the Work Item execution contract: a preserved
// github work-items Provider without its declaring Integration is not ready.
func TestEditPreviewCapabilityFollowsExecutionContract(t *testing.T) {
	if ready := mustPreview(t, newEditFixture(t), projectapp.EditIntent{Name: set("Renamed")}); ready.Preview().Capability.Readiness != projectapp.CapabilityReady {
		t.Fatalf("complete declaration readiness = %s", ready.Preview().Capability.Readiness)
	}
	chat := project.Integration{Key: "chat", ProviderRef: project.Configured("chat"), CredentialRef: project.Configured("chat-token")}
	for name, integrations := range map[string][]project.Integration{
		"integration absent":    {chat},
		"different providerRef": {chat, {Key: "work-items", ProviderRef: project.Configured("chat"), Capabilities: project.Configured([]string{"work-item"})}},
		"capability absent":     {chat, {Key: "work-items", ProviderRef: project.Configured("work-items"), Capabilities: project.Configured([]string{"other"})}},
		"capabilities omitted":  {chat, {Key: "work-items", ProviderRef: project.Configured("work-items")}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newEditFixture(t)
			state := f.selection.Portable.Project().State()
			state.Integrations = project.Configured(integrations)
			f.replacePortable(t, state)
			if projectapp.GitHubWorkItemCapability(state) {
				t.Fatal("execution contract accepted an incomplete declaration")
			}
			renamed := mustPreview(t, f, projectapp.EditIntent{Name: set("Renamed")})
			if capability := renamed.Preview().Capability; capability.Provider != "github" || capability.Readiness != projectapp.CapabilityMissing {
				t.Fatalf("name-only edit capability = %+v", capability)
			}
			repaired := mustPreview(t, f, projectapp.EditIntent{WorkItemProvider: set("github")})
			if !projectapp.GitHubWorkItemCapability(repaired.Project().State()) || repaired.Preview().Capability.Readiness != projectapp.CapabilityReady {
				t.Fatalf("explicit provider set did not restore the declaration: %+v", repaired.Preview().Capability)
			}
		})
	}
}
