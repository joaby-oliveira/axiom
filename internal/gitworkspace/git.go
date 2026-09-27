// Package gitworkspace implements the concrete local Git boundary for S8.
package gitworkspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/rgomids/axiom/internal/executiongraph"
)

var (
	ErrInvalidConfiguration = errors.New("invalid Git workspace configuration")
	ErrWorkspaceCollision   = errors.New("Git workspace collision")
	ErrWorkspaceInvalid     = errors.New("Git workspace validation failed")
	ErrForeignMutation      = errors.New("foreign Git workspace mutation")
	ErrIntegrationConflict  = errors.New("Git integration conflict")
	ErrNoResult             = errors.New("child has no successful result")
)

const ownershipDirectory = ".axiom-workspace-owners"

type WorkspaceObservation struct {
	Repository, CommonDirectory string
	ParentID, ChildID           string
	GraphRevision               uint64
	EnvelopeDigest              string
	Workspace, BaseRevision     string
	HeadRevision, HeadTree      string
	ResultTree                  string
	ChangedPaths                []string
	Clean                       bool
}

type ownerRecord struct {
	FormatVersion   int    `json:"formatVersion"`
	Repository      string `json:"repository"`
	CommonDirectory string `json:"commonDirectory"`
	ParentID        string `json:"parentId"`
	ChildID         string `json:"childId"`
	GraphRevision   uint64 `json:"graphRevision"`
	EnvelopeDigest  string `json:"envelopeDigest"`
	Workspace       string `json:"workspace"`
	BaseRevision    string `json:"baseRevision"`
	BaseTree        string `json:"baseTree"`
}

// Manager owns one repository/base/root binding and preserves every worktree.
// It intentionally exposes no cleanup operation.
type Manager struct {
	git, repository, commonDirectory, root string
	baseRevision, baseTree                 string

	mu    sync.RWMutex
	graph executiongraph.Graph
}

func NewManager(ctx context.Context, repository, root, baseRevision string, graph executiongraph.Graph) (*Manager, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, ErrInvalidConfiguration
	}
	git, err = filepath.Abs(git)
	if err != nil {
		return nil, ErrInvalidConfiguration
	}
	repository, err = cleanExistingDirectory(repository)
	if err != nil || !validObjectID(baseRevision) || !executiongraph.ValidGraph(graph) {
		return nil, ErrInvalidConfiguration
	}
	root, err = cleanAuthorizedRoot(root)
	if err != nil || samePath(root, repository) || isWithin(repository, root) {
		return nil, ErrInvalidConfiguration
	}
	m := &Manager{git: git, repository: repository, root: root, baseRevision: strings.ToLower(baseRevision), graph: graph}
	m.commonDirectory, err = m.gitPath(ctx, repository, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, ErrInvalidConfiguration
	}
	head, err := m.revision(ctx, repository, "HEAD")
	if err != nil || head != m.baseRevision {
		return nil, ErrInvalidConfiguration
	}
	m.baseRevision, err = m.revision(ctx, repository, m.baseRevision)
	if err != nil {
		return nil, ErrInvalidConfiguration
	}
	m.baseTree, err = m.tree(ctx, repository, m.baseRevision)
	if err != nil {
		return nil, ErrInvalidConfiguration
	}
	if err := os.MkdirAll(filepath.Join(root, ownershipDirectory), 0o700); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) BindGraph(graph executiongraph.Graph) error {
	if !executiongraph.ValidGraph(graph) {
		return ErrInvalidConfiguration
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if graph.Parent.ExecutionID != m.graph.Parent.ExecutionID || graph.Parent.GraphRevision != m.graph.Parent.GraphRevision || len(graph.Children) != len(m.graph.Children) {
		return ErrInvalidConfiguration
	}
	for _, original := range m.graph.Children {
		updated, ok := childByID(graph, original.ExecutionID)
		if !ok || executiongraph.EnvelopeDigest(updated.Envelope) != executiongraph.EnvelopeDigest(original.Envelope) {
			return ErrInvalidConfiguration
		}
	}
	m.graph = graph
	return nil
}

func (m *Manager) Prepare(ctx context.Context) ([]WorkspaceObservation, error) {
	m.mu.RLock()
	graph := m.graph
	m.mu.RUnlock()
	observations := make([]WorkspaceObservation, 0, len(graph.Children))
	for _, child := range graph.Children {
		observation, err := m.create(ctx, child)
		if err != nil {
			return observations, err
		}
		observations = append(observations, observation)
	}
	sort.Slice(observations, func(i, j int) bool { return observations[i].ChildID < observations[j].ChildID })
	return observations, nil
}

func (m *Manager) ValidateWorkspace(ctx context.Context, child executiongraph.ChildExecution) error {
	_, err := m.inspect(ctx, child, true)
	return err
}

func (m *Manager) InspectChild(ctx context.Context, child executiongraph.ChildExecution) (executiongraph.ChildResult, WorkspaceObservation, error) {
	if len(child.Attempts) == 0 || child.Attempts[len(child.Attempts)-1].Status != executiongraph.AttemptSucceeded {
		return executiongraph.ChildResult{}, WorkspaceObservation{}, ErrNoResult
	}
	observation, err := m.inspect(ctx, child, false)
	if err != nil {
		return executiongraph.ChildResult{}, WorkspaceObservation{}, err
	}
	if err := validateChangedPaths(observation.ChangedPaths, child.Envelope.AllowedEffects, "repository-write"); err != nil {
		return executiongraph.ChildResult{}, observation, err
	}
	resultTree, err := m.writeWorkspaceTree(ctx, child.Envelope.Workspace)
	if err != nil {
		return executiongraph.ChildResult{}, observation, err
	}
	observation.ResultTree = resultTree
	attempt := child.Attempts[len(child.Attempts)-1]
	result := executiongraph.ChildResult{
		ParentID: child.ParentID, ChildID: child.ExecutionID, AttemptID: attempt.AttemptID,
		GraphRevision: child.GraphRevision, EnvelopeDigest: executiongraph.EnvelopeDigest(child.Envelope),
		BaseRevision: m.baseRevision, ResultTree: resultTree,
		Effects: append([]executiongraph.Effect(nil), child.Envelope.AllowedEffects...),
	}
	return result, observation, nil
}

func (m *Manager) ObserveIntegration(ctx context.Context, waivers map[string]string) (executiongraph.IntegrationObservation, []executiongraph.ChildResult, []WorkspaceObservation, error) {
	m.mu.RLock()
	graph := m.graph
	m.mu.RUnlock()
	integration, ok := childByID(graph, graph.Parent.IntegrationChild)
	if !ok {
		return executiongraph.IntegrationObservation{}, nil, nil, ErrInvalidConfiguration
	}
	target, err := m.inspect(ctx, integration, true)
	if err != nil {
		return executiongraph.IntegrationObservation{}, nil, nil, err
	}
	results := make([]executiongraph.ChildResult, 0, len(graph.Children)-1)
	observations := []WorkspaceObservation{target}
	for _, child := range graph.Children {
		if child.Envelope.IntegrationOwner {
			continue
		}
		result, observation, inspectErr := m.InspectChild(ctx, child)
		if errors.Is(inspectErr, ErrNoResult) {
			continue
		}
		if inspectErr != nil {
			return executiongraph.IntegrationObservation{}, nil, observations, inspectErr
		}
		results = append(results, result)
		observations = append(observations, observation)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].ChildID < results[j].ChildID })
	conflicts := []string{}
	if _, err := m.previewResultTree(ctx, results); err != nil {
		if errors.Is(err, ErrIntegrationConflict) {
			conflicts = []string{"child_result_conflict"}
		} else {
			return executiongraph.IntegrationObservation{}, nil, observations, err
		}
	}
	return executiongraph.IntegrationObservation{
		TargetRevision: m.baseRevision, TargetTree: m.baseTree,
		Conflicts: conflicts, OptionalWaivers: cloneMap(waivers),
	}, results, observations, nil
}

func (m *Manager) Apply(ctx context.Context, integration executiongraph.ChildExecution, preview executiongraph.IntegrationPreview) (executiongraph.AppliedIntegration, error) {
	m.mu.RLock()
	graph := m.graph
	m.mu.RUnlock()
	expected, ok := childByID(graph, graph.Parent.IntegrationChild)
	if !ok || integration.ExecutionID != expected.ExecutionID || executiongraph.EnvelopeDigest(integration.Envelope) != executiongraph.EnvelopeDigest(expected.Envelope) || preview.ParentID != graph.Parent.ExecutionID || preview.IntegrationChildID != integration.ExecutionID || preview.GraphRevision != graph.Parent.GraphRevision || preview.TargetRevision != m.baseRevision || preview.TargetTree != m.baseTree {
		return executiongraph.AppliedIntegration{}, ErrWorkspaceInvalid
	}
	if _, err := m.inspect(ctx, integration, true); err != nil {
		return executiongraph.AppliedIntegration{}, err
	}
	actual := make([]executiongraph.ChildResult, 0, len(preview.Sources))
	for _, source := range preview.Sources {
		child, exists := childByID(graph, source.ChildID)
		if !exists {
			return executiongraph.AppliedIntegration{}, ErrWorkspaceInvalid
		}
		result, _, err := m.InspectChild(ctx, child)
		if err != nil || !sameChildResult(result, source) {
			return executiongraph.AppliedIntegration{}, ErrWorkspaceInvalid
		}
		actual = append(actual, result)
	}
	expectedTree, err := m.previewResultTree(ctx, actual)
	if err != nil {
		return executiongraph.AppliedIntegration{}, err
	}
	for _, source := range actual {
		patch, err := m.diffTrees(ctx, source.BaseRevision, source.ResultTree)
		if err != nil {
			return executiongraph.AppliedIntegration{}, err
		}
		if len(patch) == 0 {
			continue
		}
		if _, err := m.gitOutput(ctx, integration.Envelope.Workspace, patch, nil, "apply", "--index", "--whitespace=nowarn", "-"); err != nil {
			return executiongraph.AppliedIntegration{References: []string{"git-workspace:" + integration.ExecutionID}}, ErrIntegrationConflict
		}
	}
	resultTree, err := m.gitObject(ctx, integration.Envelope.Workspace, "write-tree")
	if err != nil || resultTree != expectedTree {
		return executiongraph.AppliedIntegration{ResultTree: resultTree, References: []string{"git-workspace:" + integration.ExecutionID}}, ErrForeignMutation
	}
	changed, err := m.changedPaths(ctx, integration.Envelope.Workspace)
	if err != nil || validateChangedPaths(changed, integration.Envelope.AllowedEffects, "integration") != nil {
		return executiongraph.AppliedIntegration{ResultTree: resultTree, References: []string{"git-workspace:" + integration.ExecutionID}}, ErrForeignMutation
	}
	return executiongraph.AppliedIntegration{
		Confirmed: true, ResultTree: resultTree,
		AppliedEffects: append([]executiongraph.Effect(nil), preview.Effects...),
		References:     []string{"git-workspace:" + integration.ExecutionID, "git-tree:" + resultTree},
	}, nil
}

func (m *Manager) ValidateResultTree(ctx context.Context, integration executiongraph.ChildExecution, resultTree string) error {
	observation, err := m.inspect(ctx, integration, false)
	if err != nil {
		return err
	}
	if validateChangedPaths(observation.ChangedPaths, integration.Envelope.AllowedEffects, "integration") != nil {
		return ErrForeignMutation
	}
	actual, err := m.writeWorkspaceTree(ctx, integration.Envelope.Workspace)
	if err != nil || actual != resultTree {
		return ErrForeignMutation
	}
	return nil
}

func (m *Manager) create(ctx context.Context, child executiongraph.ChildExecution) (WorkspaceObservation, error) {
	if err := m.validateChildBinding(child); err != nil {
		return WorkspaceObservation{}, err
	}
	if err := m.validateControlPaths(child.ExecutionID, false); err != nil {
		return WorkspaceObservation{}, err
	}
	workspace, err := m.confinedWorkspace(child.Envelope.Workspace, false)
	if err != nil {
		return WorkspaceObservation{}, err
	}
	if _, err := os.Lstat(workspace); !errors.Is(err, os.ErrNotExist) {
		return WorkspaceObservation{}, ErrWorkspaceCollision
	}
	if _, err := os.Lstat(m.ownerPath(child.ExecutionID)); !errors.Is(err, os.ErrNotExist) {
		return WorkspaceObservation{}, ErrWorkspaceCollision
	}
	if _, err := m.gitOutput(ctx, m.repository, nil, nil, "worktree", "add", "--detach", workspace, m.baseRevision); err != nil {
		return WorkspaceObservation{}, fmt.Errorf("%w: %v", ErrWorkspaceCollision, err)
	}
	record := ownerRecord{FormatVersion: 1, Repository: m.repository, CommonDirectory: m.commonDirectory, ParentID: child.ParentID, ChildID: child.ExecutionID, GraphRevision: child.GraphRevision, EnvelopeDigest: executiongraph.EnvelopeDigest(child.Envelope), Workspace: workspace, BaseRevision: m.baseRevision, BaseTree: m.baseTree}
	if err := writeExclusiveJSON(m.ownerPath(child.ExecutionID), record); err != nil {
		return WorkspaceObservation{}, err
	}
	return m.inspect(ctx, child, true)
}

func (m *Manager) inspect(ctx context.Context, child executiongraph.ChildExecution, requireClean bool) (WorkspaceObservation, error) {
	if err := m.validateChildBinding(child); err != nil {
		return WorkspaceObservation{}, err
	}
	if err := m.validateControlPaths(child.ExecutionID, true); err != nil {
		return WorkspaceObservation{}, err
	}
	workspace, err := m.confinedWorkspace(child.Envelope.Workspace, true)
	if err != nil {
		return WorkspaceObservation{}, err
	}
	var owner ownerRecord
	if err := readStrictJSON(m.ownerPath(child.ExecutionID), &owner); err != nil || owner != (ownerRecord{FormatVersion: 1, Repository: m.repository, CommonDirectory: m.commonDirectory, ParentID: child.ParentID, ChildID: child.ExecutionID, GraphRevision: child.GraphRevision, EnvelopeDigest: executiongraph.EnvelopeDigest(child.Envelope), Workspace: workspace, BaseRevision: m.baseRevision, BaseTree: m.baseTree}) {
		return WorkspaceObservation{}, ErrWorkspaceInvalid
	}
	top, err := m.gitPath(ctx, workspace, "rev-parse", "--show-toplevel")
	if err != nil || !samePath(top, workspace) {
		return WorkspaceObservation{}, ErrWorkspaceInvalid
	}
	common, err := m.gitPath(ctx, workspace, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || !samePath(common, m.commonDirectory) {
		return WorkspaceObservation{}, ErrWorkspaceInvalid
	}
	head, err := m.revision(ctx, workspace, "HEAD")
	if err != nil || head != m.baseRevision {
		return WorkspaceObservation{}, ErrWorkspaceInvalid
	}
	if !m.registeredWorktree(ctx, workspace, head) {
		return WorkspaceObservation{}, ErrWorkspaceInvalid
	}
	headTree, err := m.tree(ctx, workspace, "HEAD")
	if err != nil || headTree != m.baseTree {
		return WorkspaceObservation{}, ErrWorkspaceInvalid
	}
	changed, err := m.changedPaths(ctx, workspace)
	if err != nil {
		return WorkspaceObservation{}, ErrWorkspaceInvalid
	}
	if requireClean && len(changed) != 0 {
		return WorkspaceObservation{}, ErrForeignMutation
	}
	return WorkspaceObservation{Repository: m.repository, CommonDirectory: m.commonDirectory, ParentID: child.ParentID, ChildID: child.ExecutionID, GraphRevision: child.GraphRevision, EnvelopeDigest: executiongraph.EnvelopeDigest(child.Envelope), Workspace: workspace, BaseRevision: m.baseRevision, HeadRevision: head, HeadTree: headTree, ChangedPaths: changed, Clean: len(changed) == 0}, nil
}

func (m *Manager) validateChildBinding(child executiongraph.ChildExecution) error {
	m.mu.RLock()
	expected, ok := childByID(m.graph, child.ExecutionID)
	m.mu.RUnlock()
	if !ok || child.ParentID != expected.ParentID || child.GraphRevision != expected.GraphRevision || executiongraph.EnvelopeDigest(child.Envelope) != executiongraph.EnvelopeDigest(expected.Envelope) {
		return ErrWorkspaceInvalid
	}
	return nil
}

func (m *Manager) confinedWorkspace(value string, mustExist bool) (string, error) {
	if !filepath.IsAbs(value) || filepath.Clean(value) != value || strings.ContainsAny(value, "\x00\r\n") {
		return "", ErrWorkspaceInvalid
	}
	relative, err := filepath.Rel(m.root, value)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", ErrWorkspaceInvalid
	}
	current := m.root
	parts := strings.Split(relative, string(filepath.Separator))
	limit := len(parts)
	if !mustExist {
		limit--
	}
	for _, part := range parts[:limit] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", ErrWorkspaceInvalid
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", ErrWorkspaceInvalid
		}
	}
	if mustExist {
		resolved, err := filepath.EvalSymlinks(value)
		if err != nil || !samePath(resolved, value) || !isWithin(m.root, resolved) {
			return "", ErrWorkspaceInvalid
		}
	}
	return value, nil
}

func (m *Manager) validateControlPaths(childID string, ownerMustExist bool) error {
	for _, directory := range []string{m.root, filepath.Join(m.root, ownershipDirectory)} {
		info, err := os.Lstat(directory)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return ErrWorkspaceInvalid
		}
	}
	owner := m.ownerPath(childID)
	info, err := os.Lstat(owner)
	if !ownerMustExist && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return ErrWorkspaceInvalid
	}
	return nil
}

func (m *Manager) registeredWorktree(ctx context.Context, workspace, head string) bool {
	output, err := m.gitOutput(ctx, m.repository, nil, nil, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return false
	}
	var listedPath, listedHead string
	detached := false
	flush := func() bool {
		return samePath(listedPath, workspace) && listedHead == head && detached
	}
	for _, field := range splitNUL(output) {
		switch {
		case strings.HasPrefix(field, "worktree "):
			if listedPath != "" && flush() {
				return true
			}
			listedPath = strings.TrimPrefix(field, "worktree ")
			listedHead = ""
			detached = false
		case strings.HasPrefix(field, "HEAD "):
			listedHead = strings.TrimPrefix(field, "HEAD ")
		case field == "detached":
			detached = true
		}
	}
	return listedPath != "" && flush()
}

func (m *Manager) changedPaths(ctx context.Context, workspace string) ([]string, error) {
	tracked, err := m.gitOutput(ctx, workspace, nil, nil, "diff", "--name-only", "-z", "HEAD", "--")
	if err != nil {
		return nil, err
	}
	untracked, err := m.gitOutput(ctx, workspace, nil, nil, "ls-files", "--others", "--exclude-standard", "-z", "--")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, value := range append(splitNUL(tracked), splitNUL(untracked)...) {
		value = path.Clean(filepath.ToSlash(value))
		if value == "." || strings.HasPrefix(value, "../") || path.IsAbs(value) {
			return nil, ErrForeignMutation
		}
		seen[value] = true
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func (m *Manager) writeWorkspaceTree(ctx context.Context, workspace string) (string, error) {
	index, err := os.CreateTemp(m.root, ".axiom-index-*")
	if err != nil {
		return "", err
	}
	indexPath := index.Name()
	if err := index.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(indexPath); err != nil {
		return "", err
	}
	defer os.Remove(indexPath)
	extra := []string{"GIT_INDEX_FILE=" + indexPath}
	if _, err := m.gitOutput(ctx, workspace, nil, extra, "read-tree", m.baseRevision); err != nil {
		return "", err
	}
	if _, err := m.gitOutput(ctx, workspace, nil, extra, "add", "-A", "--", "."); err != nil {
		return "", err
	}
	return m.gitObjectWithEnv(ctx, workspace, extra, "write-tree")
}

func (m *Manager) previewResultTree(ctx context.Context, sources []executiongraph.ChildResult) (string, error) {
	index, err := os.CreateTemp(m.root, ".axiom-preview-index-*")
	if err != nil {
		return "", err
	}
	indexPath := index.Name()
	if err := index.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(indexPath); err != nil {
		return "", err
	}
	defer os.Remove(indexPath)
	extra := []string{"GIT_INDEX_FILE=" + indexPath}
	if _, err := m.gitOutput(ctx, m.repository, nil, extra, "read-tree", m.baseRevision); err != nil {
		return "", err
	}
	ordered := append([]executiongraph.ChildResult(nil), sources...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ChildID < ordered[j].ChildID })
	for _, source := range ordered {
		patch, err := m.diffTrees(ctx, source.BaseRevision, source.ResultTree)
		if err != nil {
			return "", err
		}
		if len(patch) == 0 {
			continue
		}
		if _, err := m.gitOutput(ctx, m.repository, patch, extra, "apply", "--cached", "--check", "--whitespace=nowarn", "-"); err != nil {
			return "", ErrIntegrationConflict
		}
		if _, err := m.gitOutput(ctx, m.repository, patch, extra, "apply", "--cached", "--whitespace=nowarn", "-"); err != nil {
			return "", ErrIntegrationConflict
		}
	}
	return m.gitObjectWithEnv(ctx, m.repository, extra, "write-tree")
}

func (m *Manager) diffTrees(ctx context.Context, base, result string) ([]byte, error) {
	if !validObjectID(base) || !validObjectID(result) {
		return nil, ErrWorkspaceInvalid
	}
	return m.gitOutput(ctx, m.repository, nil, nil, "diff", "--binary", "--full-index", base, result, "--")
}

func (m *Manager) revision(ctx context.Context, cwd, revision string) (string, error) {
	return m.gitObject(ctx, cwd, "rev-parse", "--verify", revision+"^{commit}")
}

func (m *Manager) tree(ctx context.Context, cwd, revision string) (string, error) {
	return m.gitObject(ctx, cwd, "rev-parse", "--verify", revision+"^{tree}")
}

func (m *Manager) gitObject(ctx context.Context, cwd string, args ...string) (string, error) {
	return m.gitObjectWithEnv(ctx, cwd, nil, args...)
}

func (m *Manager) gitObjectWithEnv(ctx context.Context, cwd string, env []string, args ...string) (string, error) {
	output, err := m.gitOutput(ctx, cwd, nil, env, args...)
	value := strings.ToLower(strings.TrimSpace(string(output)))
	if err != nil || !validObjectID(value) {
		return "", ErrWorkspaceInvalid
	}
	return value, nil
}

func (m *Manager) gitPath(ctx context.Context, cwd string, args ...string) (string, error) {
	output, err := m.gitOutput(ctx, cwd, nil, nil, args...)
	if err != nil {
		return "", err
	}
	value := filepath.Clean(strings.TrimSpace(string(output)))
	if !filepath.IsAbs(value) {
		value = filepath.Join(cwd, value)
	}
	return filepath.EvalSymlinks(value)
}

func (m *Manager) gitOutput(ctx context.Context, cwd string, stdin []byte, extraEnv []string, args ...string) ([]byte, error) {
	baseArgs := []string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "credential.helper="}
	command := exec.CommandContext(ctx, m.git, append(baseArgs, args...)...)
	command.Dir = cwd
	command.Env = append([]string{"LC_ALL=C", "LANG=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0"}, extraEnv...)
	command.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > 512 {
			message = message[:512]
		}
		return nil, fmt.Errorf("git %s failed: %w: %s", args[0], err, message)
	}
	return stdout.Bytes(), nil
}

func (m *Manager) ownerPath(childID string) string {
	return filepath.Join(m.root, ownershipDirectory, childID+".json")
}

func validateChangedPaths(changed []string, effects []executiongraph.Effect, requiredKind string) error {
	allowed := make([]string, 0, len(effects))
	for _, effect := range effects {
		if effect.Kind == requiredKind {
			target := path.Clean(filepath.ToSlash(effect.Target))
			if target == "." || target == ".." || strings.HasPrefix(target, "../") || path.IsAbs(target) {
				return ErrForeignMutation
			}
			allowed = append(allowed, target)
		}
	}
	for _, changedPath := range changed {
		matched := false
		for _, target := range allowed {
			if changedPath == target || strings.HasPrefix(changedPath, target+"/") {
				matched = true
				break
			}
		}
		if !matched {
			return ErrForeignMutation
		}
	}
	return nil
}

func childByID(graph executiongraph.Graph, childID string) (executiongraph.ChildExecution, bool) {
	for _, child := range graph.Children {
		if child.ExecutionID == childID {
			return child, true
		}
	}
	return executiongraph.ChildExecution{}, false
}

func sameChildResult(left, right executiongraph.ChildResult) bool {
	leftWire, _ := json.Marshal(left)
	rightWire, _ := json.Marshal(right)
	return bytes.Equal(leftWire, rightWire)
}

func validObjectID(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && (len(decoded) == 20 || len(decoded) == sha256.Size)
}

func cleanExistingDirectory(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil || filepath.Clean(value) != absolute {
		return "", ErrInvalidConfiguration
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", ErrInvalidConfiguration
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", ErrInvalidConfiguration
	}
	return resolved, nil
}

func cleanAuthorizedRoot(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil || filepath.Clean(value) != absolute {
		return "", ErrInvalidConfiguration
	}
	for current := absolute; ; current = filepath.Dir(current) {
		info, statErr := os.Lstat(current)
		if statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return "", ErrInvalidConfiguration
			}
			break
		}
		if !errors.Is(statErr, os.ErrNotExist) || filepath.Dir(current) == current {
			return "", ErrInvalidConfiguration
		}
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil || !samePath(resolved, absolute) {
		return "", ErrInvalidConfiguration
	}
	return absolute, nil
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func isWithin(root, value string) bool {
	relative, err := filepath.Rel(root, value)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func writeExclusiveJSON(name string, value any) error {
	wire, err := json.Marshal(value)
	if err != nil {
		return err
	}
	wire = append(wire, '\n')
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(wire); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func readStrictJSON(name string, value any) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrWorkspaceInvalid
	}
	return nil
}

func splitNUL(value []byte) []string {
	parts := bytes.Split(value, []byte{0})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 0 {
			result = append(result, string(part))
		}
	}
	return result
}

func cloneMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
