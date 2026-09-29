//go:build ignore

// S9/T24 graph journey runner: maintainer acceptance tooling, not product.
//
// The published `axiom` CLI has no Execution Graph surface (T36 polish finding
// 6), so T24 drives the delivered S8 path through internal/graphapplication,
// exactly as the T36 operator did. Build it only from a checkout whose product
// tree equals the exact RC revision (the envelope checks this first):
//
//	go build -o <lab>/bin/t24-graph-runner ./scripts/s9-graph-runner.go
//
// Modes, each bound to one reviewed spec file (JSON) and run under its own
// envelope phase:
//
//	prepare     --spec S --project-id P       observe Runtime versions, persist profiles,
//	                                           publish the graph, write the run envelope
//	run         --spec S --parent X --run-envelope-sha256 D [--retry-child K]
//	                                           dispatch, integrate locally, build Evidence
//	question|answer|wait-answer --spec S [--parent X]
//	                                           child-side structured coordination (from the
//	                                           child worktree; defaults to the prepared parent)
//
// It never pushes, opens PRs, mutates a Provider, retries automatically, or
// deletes anything; cleanup is preserved for human review.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rgomids/axiom/internal/coordination"
	"github.com/rgomids/axiom/internal/executiongraph"
	"github.com/rgomids/axiom/internal/gitworkspace"
	"github.com/rgomids/axiom/internal/graphapplication"
	"github.com/rgomids/axiom/internal/local"
	"github.com/rgomids/axiom/internal/runtimeadapter"
	"github.com/rgomids/axiom/internal/runtimeprofile"
)

const specSchema = "axiom-s9-graph-spec/v1"

var (
	uuidPattern    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	versionPattern = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+[0-9A-Za-z.-]*`)
)

type runtimeSpec struct {
	Executable string `json:"executable"`
	Model      string `json:"model"`
}

type childSpec struct {
	Key            string   `json:"key"`
	Runtime        string   `json:"runtime"`
	ModelProfile   string   `json:"modelProfile"`
	Role           string   `json:"role"`
	Capability     string   `json:"capability"`
	Paths          []string `json:"paths"`
	Arguments      []string `json:"arguments"`
	Prompt         string   `json:"prompt"`
	Coordination   string   `json:"coordination"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
	MaxAttempts    uint32   `json:"maxAttempts"`
}

type spec struct {
	Schema         string                 `json:"schema"`
	RCTag          string                 `json:"rcTag"`
	SourceRevision string                 `json:"sourceRevision"`
	Activity       string                 `json:"activity"`
	PlanStatement  string                 `json:"planStatement"`
	Repository     string                 `json:"repository"`
	StateRoot      string                 `json:"stateRoot"`
	WorkspaceRoot  string                 `json:"workspaceRoot"`
	EvidenceRoot   string                 `json:"evidenceRoot"`
	RepositoryKey  string                 `json:"repositoryKey"`
	Authority      string                 `json:"authorityReference"`
	Runtimes       map[string]runtimeSpec `json:"runtimes"`
	Environment    []string               `json:"environment"`
	Children       []childSpec            `json:"children"`
	Integration    childSpec              `json:"integration"`
	Question       string                 `json:"question"`
	Answer         string                 `json:"answer"`
	Validators     []struct {
		Reference string   `json:"reference"`
		Argv      []string `json:"argv"`
	} `json:"validators"`
	OutputMax int `json:"outputMax"`
}

var ctx = context.Background()

func main() {
	if len(os.Args) < 2 {
		fatal(errors.New("mode required: prepare|run|question|answer|wait-answer"))
	}
	mode := os.Args[1]
	flags := flag.NewFlagSet(mode, flag.ExitOnError)
	specPath := flags.String("spec", "", "absolute reviewed graph spec")
	projectID := flags.String("project-id", "", "configured Axiom Project ID (prepare)")
	parentID := flags.String("parent", "", "parent Execution ID (run, coordination)")
	approved := flags.String("run-envelope-sha256", "", "human-authorized run envelope digest (run)")
	retryChild := flags.String("retry-child", "", "child key for one separately authorized retry (run)")
	_ = flags.Parse(os.Args[2:])
	s, specDigest := loadSpec(*specPath)
	switch mode {
	case "prepare":
		prepare(s, specDigest, *projectID)
	case "run":
		run(s, specDigest, *parentID, *approved, *retryChild)
	case "question", "answer", "wait-answer":
		coordinate(s, mode, *parentID)
	default:
		fatal(fmt.Errorf("unknown mode %q", mode))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "t24-graph-runner:", err)
	os.Exit(1)
}

func must(err error) {
	if err != nil {
		fatal(err)
	}
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func loadSpec(path string) (spec, string) {
	if !filepath.IsAbs(path) {
		fatal(errors.New("--spec must be absolute"))
	}
	raw, err := os.ReadFile(path)
	must(err)
	var s spec
	must(json.Unmarshal(raw, &s))
	absolute := []string{s.Repository, s.StateRoot, s.WorkspaceRoot, s.EvidenceRoot}
	for _, value := range absolute {
		if !filepath.IsAbs(value) {
			fatal(errors.New("spec paths must be absolute"))
		}
	}
	if s.Schema != specSchema || len(s.Children) < 2 || s.Integration.Key == "" || len(s.Validators) == 0 || s.OutputMax <= 0 {
		fatal(errors.New("spec is not a valid T24 graph spec"))
	}
	runtimes := map[string]bool{}
	for _, child := range s.Children {
		runtimes[child.Runtime] = true
	}
	if !runtimes["codex"] || !runtimes["claude"] {
		fatal(errors.New("spec needs at least one independent Codex child and one Claude child"))
	}
	return s, digest(raw)
}

// write persists one Evidence record and refuses to overwrite earlier Evidence.
func write(s spec, name string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	must(err)
	must(os.MkdirAll(s.EvidenceRoot, 0o700))
	file, err := os.OpenFile(filepath.Join(s.EvidenceRoot, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	must(err)
	defer file.Close()
	_, err = file.Write(append(data, '\n'))
	must(err)
}

func read(s spec, name string, value any) {
	data, err := os.ReadFile(filepath.Join(s.EvidenceRoot, name))
	must(err)
	must(json.Unmarshal(data, value))
}

func gitHead(repository string) string {
	command := exec.Command("git", "rev-parse", "--verify", "HEAD")
	command.Dir = repository
	output, err := command.Output()
	must(err)
	return strings.TrimSpace(string(output))
}

// runtimeVersion runs only `<runtime> --version`; it is the observation the
// Inventory needs and the only Runtime process prepare starts.
func runtimeVersion(s spec, runtime string) string {
	command := exec.Command(s.Runtimes[runtime].Executable, "--version")
	command.Env = s.Environment
	output, err := command.Output()
	must(err)
	return strings.TrimSpace(string(output))
}

// versionToken extracts the version token Evidence requires from a CLI banner
// such as "codex-cli 0.157.1" or "2.1.284 (Claude Code)".
func versionToken(banner string) string {
	token := versionPattern.FindString(banner)
	if token == "" {
		fatal(fmt.Errorf("no version token in Runtime banner %q", banner))
	}
	return token
}

func childSpecs(s spec) []childSpec {
	return append(append([]childSpec{}, s.Children...), s.Integration)
}

func configuration(s spec) runtimeprofile.Configuration {
	allow := map[string][]string{}
	cfg := runtimeprofile.Configuration{FormatVersion: 1, Revision: 1}
	for _, child := range childSpecs(s) {
		allow[child.Runtime] = append(allow[child.Runtime], child.ModelProfile)
		cfg.ModelProfiles = append(cfg.ModelProfiles, runtimeprofile.ModelProfile{ID: child.ModelProfile, RuntimeID: child.Runtime, Model: s.Runtimes[child.Runtime].Model, Capabilities: []string{child.Capability}})
		cfg.Preferences = append(cfg.Preferences, runtimeprofile.Preference{Role: child.Role, Complexity: "low", ModelProfileID: child.ModelProfile})
	}
	for _, id := range []string{"codex", "claude"} {
		cfg.Runtimes = append(cfg.Runtimes, runtimeprofile.Runtime{ID: id, Adapter: id, Enabled: true, AllowlistedProfileIDs: allow[id]})
	}
	return cfg
}

type capabilities struct {
	cfg      runtimeprofile.Configuration
	resolver runtimeprofile.Resolver
}

func (c capabilities) ValidateCapability(ctx context.Context, request executiongraph.CapabilityRequest) error {
	_, err := c.resolver.Resolve(ctx, c.cfg, runtimeprofile.Request{ConfigurationRevision: c.cfg.Revision, Role: request.Role, Complexity: request.Complexity, Capabilities: request.Capabilities})
	return err
}

func commandProfiles(s spec) []runtimeadapter.CommandProfile {
	profiles := []runtimeadapter.CommandProfile{}
	for _, child := range childSpecs(s) {
		arguments := append(append([]string{}, child.Arguments...), child.Prompt)
		profiles = append(profiles, runtimeadapter.CommandProfile{RuntimeID: child.Runtime, ModelProfileID: child.ModelProfile, Executable: s.Runtimes[child.Runtime].Executable, Model: s.Runtimes[child.Runtime].Model, Arguments: arguments, Environment: s.Environment, OutputMax: s.OutputMax})
	}
	return profiles
}

func validators(s spec) []gitworkspace.ValidationCommand {
	commands := []gitworkspace.ValidationCommand{}
	for _, validator := range s.Validators {
		if !filepath.IsAbs(validator.Argv[0]) {
			fatal(fmt.Errorf("validator %s must use an absolute executable", validator.Reference))
		}
		commands = append(commands, gitworkspace.ValidationCommand{Reference: validator.Reference, Argv: validator.Argv, Env: s.Environment, OutputMax: s.OutputMax})
	}
	return commands
}

func prepare(s spec, specDigest, projectID string) {
	if !uuidPattern.MatchString(projectID) {
		fatal(errors.New("--project-id must be the configured Project UUID"))
	}
	base := gitHead(s.Repository)
	cfg := configuration(s)
	profileStore, err := local.NewRuntimeProfileStore(s.StateRoot)
	must(err)
	if _, loadErr := profileStore.Load(ctx); loadErr != nil {
		must(profileStore.Create(ctx, cfg))
	}
	cfg, err = profileStore.Load(ctx)
	must(err)
	observations := []runtimeprofile.Observation{}
	for _, runtime := range []string{"codex", "claude"} {
		status := map[string]runtimeprofile.CapabilityStatus{}
		for _, child := range childSpecs(s) {
			if child.Runtime == runtime {
				status[child.Capability] = runtimeprofile.CapabilityProven
			}
		}
		observations = append(observations, runtimeprofile.Observation{RuntimeID: runtime, Adapter: runtime, Installed: true, Available: true, Version: runtimeVersion(s, runtime), Revision: 1, ObservedAt: time.Now().UTC(), CapabilityStatus: status})
	}
	inventory, err := runtimeadapter.NewInventory(observations)
	must(err)
	resolver := runtimeprofile.NewResolver(inventory)
	plan := executiongraph.ApprovedPlan{Approved: true, PlanRevision: "t24-rc2-plan-1", PlanDigest: digest([]byte(s.PlanStatement)), MaximumNodes: len(childSpecs(s))}
	for _, child := range childSpecs(s) {
		kind := "repository-write"
		unit := executiongraph.WorkUnit{Key: child.Key, Capability: executiongraph.CapabilityRequest{Role: child.Role, Complexity: "low", Capabilities: []string{child.Capability}}, Inputs: []string{"approved-plan"}, Outputs: []string{child.Key + "-result"}, Scope: executiongraph.Scope{ProjectID: projectID, RepositoryKey: s.RepositoryKey, Paths: child.Paths}, Controls: executiongraph.ExecutionControls{Timeout: time.Duration(child.TimeoutSeconds) * time.Second, MaximumAttempts: child.MaxAttempts}}
		if child.Key == s.Integration.Key {
			kind = "integration"
			unit.IntegrationOwner, unit.ValidationOwner = true, true
			for _, dependency := range s.Children {
				unit.Dependencies = append(unit.Dependencies, dependency.Key)
			}
		}
		for _, path := range child.Paths {
			unit.Effects = append(unit.Effects, executiongraph.Effect{Kind: kind, Target: path})
		}
		plan.Work = append(plan.Work, unit)
	}
	write(s, "approved-plan.json", plan)
	proposal, err := executiongraph.NewPlanner(capabilities{cfg, resolver}).Propose(ctx, plan)
	must(err)
	request := executiongraph.PublicationRequest{Proposal: proposal, ExpectedDigest: proposal.Digest, GraphRevision: 1, ChildAuthorities: map[string][]executiongraph.Effect{}, AuthorityReferences: map[string]string{}, Workspaces: map[string]string{}, Resolutions: map[string]executiongraph.Resolution{}}
	for _, node := range proposal.Nodes {
		resolution, resolveErr := resolver.Resolve(ctx, cfg, runtimeprofile.Request{ConfigurationRevision: cfg.Revision, Role: node.Capability.Role, Complexity: node.Capability.Complexity, Capabilities: node.Capability.Capabilities})
		must(resolveErr)
		request.Resolutions[node.Key] = executiongraph.Resolution{RuntimeID: resolution.Choice.RuntimeID, ModelProfileID: resolution.Choice.ModelProfileID, ConfigurationRevision: cfg.Revision, ObservationRevision: 1}
		request.ChildAuthorities[node.Key] = node.Effects
		request.ParentAuthority = append(request.ParentAuthority, node.Effects...)
		request.AuthorityReferences[node.Key] = s.Authority + ":" + node.Key
		request.Workspaces[node.Key] = filepath.Join(s.WorkspaceRoot, node.Key)
	}
	graphStore, err := local.NewGraphStore(s.StateRoot)
	must(err)
	graph, err := executiongraph.NewGraphService(graphStore, nil, nil).Publish(ctx, request)
	must(err)
	write(s, "graph-initial.json", graph)
	profiles := commandProfiles(s)
	invocations, err := runtimeadapter.NewInvocationResolver(profiles, nil)
	must(err)
	write(s, "command-profiles.json", profiles)
	encoded, err := runtimeprofile.Encode(cfg)
	must(err)
	references := []string{}
	for _, validator := range s.Validators {
		references = append(references, validator.Reference)
	}
	envelope := executiongraph.RunEnvelope{FormatVersion: 1, Activity: s.Activity, BaseRevision: base, ParentID: graph.Parent.ExecutionID, GraphRevision: graph.Parent.GraphRevision, ConfigurationRef: "state/runtime-profiles/v1/configuration.json", ConfigurationDigest: digest(encoded), Validators: references, ExpectedEvidence: []string{"real-runtime-overlap", "structured-coordination", "integration", "parent-child-evidence"}, CleanupDisposition: "preserve_pending_human_review"}
	for _, child := range graph.Children {
		invocation, resolveErr := invocations.ResolveInvocation(ctx, child)
		must(resolveErr)
		envelope.Children = append(envelope.Children, executiongraph.ChildRunPlan{ChildID: child.ExecutionID, RuntimeID: child.Envelope.Resolution.RuntimeID, ModelProfileID: child.Envelope.Resolution.ModelProfileID, Repository: s.Repository, Workspace: child.Envelope.Workspace, Scope: child.Envelope.Scope, Dependencies: child.Envelope.Dependencies, Effects: child.Envelope.AllowedEffects, Argv: invocation.Argv, Timeout: child.Envelope.Controls.Timeout.String(), MaximumAttempts: child.Envelope.Controls.MaximumAttempts})
	}
	envelope, err = executiongraph.PrepareRunEnvelope(graph, envelope)
	must(err)
	write(s, "run-envelope.json", envelope)
	write(s, "prepare.json", map[string]any{"rcTag": s.RCTag, "sourceRevision": s.SourceRevision, "specSHA256": specDigest, "base": base, "projectId": projectID, "parentId": graph.Parent.ExecutionID, "runEnvelopeDigest": envelope.Digest, "runtimeObservations": observations})
	fmt.Printf("{\"parentId\":%q,\"runEnvelopeDigest\":%q,\"base\":%q}\n", graph.Parent.ExecutionID, envelope.Digest, base)
}

// loadGraph loads the prepared graph; an empty parentID (child-side helper)
// means the one parent recorded by prepare.
func loadGraph(s spec, parentID string) (local.GraphStore, executiongraph.Graph, string) {
	graph, projectID, err := loadGraphErr(s, parentID)
	must(err)
	store, err := local.NewGraphStore(s.StateRoot)
	must(err)
	return store, graph, projectID
}

func loadGraphErr(s spec, parentID string) (executiongraph.Graph, string, error) {
	var prepared struct {
		ProjectID string `json:"projectId"`
		ParentID  string `json:"parentId"`
	}
	read(s, "prepare.json", &prepared)
	if parentID == "" {
		parentID = prepared.ParentID
	}
	if !uuidPattern.MatchString(parentID) || prepared.ParentID != parentID {
		fatal(errors.New("parent differs from the prepared graph"))
	}
	store, err := local.NewGraphStore(s.StateRoot)
	if err != nil {
		return executiongraph.Graph{}, "", err
	}
	graph, err := store.Load(ctx, prepared.ProjectID, parentID)
	return graph, prepared.ProjectID, err
}

func run(s spec, specDigest, parentID, approved, retryChild string) {
	var prepared struct {
		SpecSHA256        string `json:"specSHA256"`
		RunEnvelopeDigest string `json:"runEnvelopeDigest"`
		Base              string `json:"base"`
	}
	read(s, "prepare.json", &prepared)
	if parentID == "" {
		fatal(errors.New("--parent is required for run"))
	}
	if prepared.SpecSHA256 != specDigest || prepared.RunEnvelopeDigest != approved || gitHead(s.Repository) != prepared.Base {
		fatal(errors.New("spec, run envelope or base drift; obtain fresh human authority"))
	}
	store, graph, projectID := loadGraph(s, parentID)
	var profiles []runtimeadapter.CommandProfile
	read(s, "command-profiles.json", &profiles)
	suffix := ""
	retry := map[string]bool(nil)
	if retryChild != "" {
		suffix = "-retry-" + retryChild
		retry = map[string]bool{}
		for _, child := range graph.Children {
			if child.NodeKey == retryChild {
				retry[child.ExecutionID] = true
			}
		}
		if len(retry) != 1 {
			fatal(errors.New("unknown retry child"))
		}
	}
	service, err := graphapplication.NewLocalService(ctx, graphapplication.LocalConfiguration{Repository: s.Repository, WorkspaceRoot: s.WorkspaceRoot, BaseRevision: prepared.Base, Graph: graph, GraphStore: store, RuntimeProfiles: profiles, Validators: validators(s)})
	must(err)
	// A retry reuses the child's existing worktree; the scheduler requires it
	// clean (a failed attempt that left edits cannot be retried in place).
	if retry == nil {
		workspaces, prepareErr := service.PrepareWorkspaces(ctx)
		must(prepareErr)
		write(s, "workspaces.json", workspaces)
	}
	fmt.Println("dispatch starting", time.Now().UTC().Format(time.RFC3339))
	dispatch, err := service.DispatchReady(ctx, retry, false)
	write(s, "dispatch"+suffix+".json", dispatch)
	write(s, "graph-after-dispatch"+suffix+".json", service.Graph())
	must(err)
	fmt.Println("dispatch ended", time.Now().UTC().Format(time.RFC3339))
	for _, record := range dispatch.Records {
		fmt.Println(record.ChildID, record.Status)
		if record.Status != executiongraph.AttemptSucceeded {
			fatal(errors.New("runtime attempt unsuccessful; canonical state preserved, no automatic retry"))
		}
	}
	coordinationStore, err := local.NewCoordinationStore(s.StateRoot, projectID)
	must(err)
	for _, child := range service.Graph().Children {
		if child.Envelope.IntegrationOwner {
			continue
		}
		diff := exec.Command("git", "diff", "--name-only", "HEAD")
		diff.Dir = child.Envelope.Workspace
		changed, diffErr := diff.Output()
		must(diffErr)
		if len(changed) == 0 {
			fatal(fmt.Errorf("child %s produced no tracked changes", child.NodeKey))
		}
		record, ok, latestErr := coordinationStore.Latest(ctx, service.Graph().Parent.ExecutionID, child.ExecutionID)
		must(latestErr)
		if !ok || record.AttemptID != child.Attempts[len(child.Attempts)-1].AttemptID {
			fatal(fmt.Errorf("child %s has no correlated coordination record", child.NodeKey))
		}
	}
	preview, observed, err := service.PreviewIntegration(ctx, nil)
	write(s, "integration-observations"+suffix+".json", observed)
	write(s, "integration-preview"+suffix+".json", preview)
	must(err)
	authority := executiongraph.IntegrationAuthority{PreviewDigest: preview.Digest, TargetRevision: preview.TargetRevision, Effects: preview.Effects, Reference: s.Authority + ":integration"}
	write(s, "integration-authority"+suffix+".json", authority)
	rollup, err := service.ExecuteIntegration(ctx, preview, authority)
	write(s, "rollup"+suffix+".json", rollup)
	write(s, "graph-final"+suffix+".json", service.Graph())
	must(err)
	var envelope executiongraph.RunEnvelope
	read(s, "run-envelope.json", &envelope)
	evidence := executiongraph.Evidence{FormatVersion: 1, BaseRevision: prepared.Base, ConfigurationDigest: envelope.ConfigurationDigest, ParentID: parentID, GraphRevision: service.Graph().Parent.GraphRevision, Dispatch: dispatch.Records, Rollup: rollup, ValidationReferences: envelope.Validators, Limitations: []string{"graph driven through internal/graphapplication; the published CLI has no graph surface", "usage and cost unavailable", "integration performed by the concrete local service"}}
	invocations, err := runtimeadapter.NewInvocationResolver(profiles, nil)
	must(err)
	for _, child := range service.Graph().Children {
		if child.Envelope.IntegrationOwner {
			continue
		}
		attempt := child.Attempts[len(child.Attempts)-1]
		invocation, resolveErr := invocations.ResolveInvocation(ctx, child)
		must(resolveErr)
		encoded, encodeErr := json.Marshal(invocation)
		must(encodeErr)
		evidence.RuntimeExecutions = append(evidence.RuntimeExecutions, executiongraph.RuntimeExecutionEvidence{ChildID: child.ExecutionID, AttemptID: attempt.AttemptID, RuntimeID: child.Envelope.Resolution.RuntimeID, ModelProfileID: child.Envelope.Resolution.ModelProfileID, RuntimeVersion: versionToken(runtimeVersion(s, child.Envelope.Resolution.RuntimeID)), ConfigurationRevision: child.Envelope.Resolution.ConfigurationRevision, ObservationRevision: child.Envelope.Resolution.ObservationRevision, InvocationDigest: digest(encoded), ResultReference: attempt.ResultReference})
		record, exists, latestErr := coordinationStore.Latest(ctx, parentID, child.ExecutionID)
		must(latestErr)
		if exists {
			encodedRecord, recordErr := coordination.Encode(record)
			must(recordErr)
			evidence.CoordinationRecords = append(evidence.CoordinationRecords, encodedRecord)
		}
	}
	evidence, err = service.BuildEvidence(evidence)
	must(err)
	write(s, "acceptance-evidence"+suffix+".json", evidence)
	fmt.Println(evidence.Status, rollup.Status)
}

// transient retries an operation that lost a non-blocking store lock
// (local.ErrConflict) to the scheduler or the sibling child; the stores fail
// closed on contention and publish nothing, so a bounded retry is safe.
func transient[T any](deadline time.Time, operation func() (T, error)) T {
	for {
		value, err := operation()
		if err == nil {
			return value
		}
		if !errors.Is(err, local.ErrConflict) || time.Now().After(deadline) {
			fatal(err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

type loaded struct {
	graph     executiongraph.Graph
	projectID string
}

// coordinate runs inside a child's persisted Runtime attempt, from its worktree.
func coordinate(s spec, mode, parentID string) {
	deadline := time.Now().Add(10 * time.Minute)
	state := transient(deadline, func() (loaded, error) {
		graph, projectID, err := loadGraphErr(s, parentID)
		return loaded{graph, projectID}, err
	})
	graph, projectID := state.graph, state.projectID
	directory, err := os.Getwd()
	must(err)
	var self, other executiongraph.ChildExecution
	for _, child := range graph.Children {
		if child.Envelope.Workspace == directory {
			self = child
		}
	}
	for _, child := range graph.Children {
		if child.ExecutionID != self.ExecutionID && !child.Envelope.IntegrationOwner {
			other = child
		}
	}
	if self.ExecutionID == "" || len(self.Attempts) == 0 || self.Attempts[len(self.Attempts)-1].Status != executiongraph.AttemptRunning {
		fatal(errors.New("not a running child worktree"))
	}
	attempt := self.Attempts[len(self.Attempts)-1]
	store, err := local.NewCoordinationStore(s.StateRoot, projectID)
	must(err)
	var question coordination.Record
	if mode != "question" {
		for {
			type latest struct {
				record coordination.Record
				ok     bool
			}
			found := transient(deadline, func() (latest, error) {
				record, ok, latestErr := store.Latest(ctx, graph.Parent.ExecutionID, other.ExecutionID)
				return latest{record, ok}, latestErr
			})
			record, ok := found.record, found.ok
			if ok {
				question = record
				break
			}
			if time.Now().After(deadline) {
				fatal(errors.New("coordination wait expired"))
			}
			time.Sleep(time.Second)
		}
		if mode == "wait-answer" {
			encoded, encodeErr := coordination.Encode(question)
			must(encodeErr)
			fmt.Println(string(encoded))
			return
		}
	}
	input := coordination.Input{ParentID: graph.Parent.ExecutionID, ChildID: self.ExecutionID, GraphRevision: graph.Parent.GraphRevision, AttemptID: attempt.AttemptID, Provenance: coordination.Provenance{Product: "axiom", Version: s.RCTag, Revision: s.SourceRevision, SourceState: "clean"}}
	if mode == "question" {
		input.Kind = coordination.QuestionRequest
		input.Fields = []coordination.Field{{Name: "question", Value: s.Question}}
	} else {
		input.Kind = coordination.Answer
		input.Fields = []coordination.Field{{Name: "answer", Value: s.Answer}, {Name: "question_reference", Value: question.RecordID}}
	}
	record := transient(deadline, func() (coordination.Record, error) {
		return coordination.New(store, nil, nil).Publish(ctx, graph, input)
	})
	encoded, err := coordination.Encode(record)
	must(err)
	write(s, "coordination-"+mode+".json", json.RawMessage(encoded))
	fmt.Println(string(encoded))
}
