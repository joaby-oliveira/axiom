// Package executiongraph owns proposal, validation and durable semantics for
// bounded parent/child Execution graphs.
package executiongraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"sort"
	"strings"
	"time"
)

const (
	ProposalFormatVersion = 1
	GraphFormatVersion    = 1
	maxNodes              = 32
	maxListItems          = 64
	maxTextBytes          = 512
)

var (
	ErrInvalidPlan     = errors.New("invalid approved plan")
	ErrInvalidGraph    = errors.New("invalid execution graph")
	ErrCycle           = errors.New("execution graph contains cycle")
	ErrUnsafeOverlap   = errors.New("unsafe overlapping effects")
	ErrUnresolvable    = errors.New("unresolvable capability request")
	ErrMissingOwner    = errors.New("missing integration owner")
	ErrAuthoritySubset = errors.New("child authority exceeds parent authority")
	ErrStaleGraph      = errors.New("stale execution graph authority")
)

type CapabilityRequest struct {
	Role         string   `json:"role"`
	Complexity   string   `json:"complexity"`
	Capabilities []string `json:"capabilities"`
}

type Scope struct {
	ProjectID     string   `json:"projectId"`
	RepositoryKey string   `json:"repositoryKey"`
	Paths         []string `json:"paths"`
}

type Effect struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

type ExecutionControls struct {
	Timeout         time.Duration `json:"timeout"`
	MaximumAttempts uint32        `json:"maximumAttempts"`
}

type WorkUnit struct {
	Key              string            `json:"key"`
	Capability       CapabilityRequest `json:"capability"`
	Dependencies     []string          `json:"dependencies"`
	Inputs           []string          `json:"inputs"`
	Outputs          []string          `json:"outputs"`
	Scope            Scope             `json:"scope"`
	Effects          []Effect          `json:"effects"`
	ValidationOwner  bool              `json:"validationOwner"`
	IntegrationOwner bool              `json:"integrationOwner"`
	Optional         bool              `json:"optional"`
	Controls         ExecutionControls `json:"controls"`
}

type ApprovedPlan struct {
	Approved     bool       `json:"approved"`
	PlanRevision string     `json:"planRevision"`
	PlanDigest   string     `json:"planDigest"`
	MaximumNodes int        `json:"maximumNodes"`
	Work         []WorkUnit `json:"work"`
}

type ProposedNode struct {
	Key              string            `json:"key"`
	Capability       CapabilityRequest `json:"capability"`
	Dependencies     []string          `json:"dependencies"`
	Inputs           []string          `json:"inputs"`
	Outputs          []string          `json:"outputs"`
	Scope            Scope             `json:"scope"`
	Effects          []Effect          `json:"effects"`
	ValidationOwner  bool              `json:"validationOwner"`
	IntegrationOwner bool              `json:"integrationOwner"`
	Optional         bool              `json:"optional"`
	Controls         ExecutionControls `json:"controls"`
}

type Proposal struct {
	FormatVersion int            `json:"formatVersion"`
	PlanRevision  string         `json:"planRevision"`
	PlanDigest    string         `json:"planDigest"`
	Nodes         []ProposedNode `json:"nodes"`
	Digest        string         `json:"digest"`
}

type CapabilityValidator interface {
	ValidateCapability(context.Context, CapabilityRequest) error
}

type Planner struct{ capabilities CapabilityValidator }

func NewPlanner(capabilities CapabilityValidator) Planner { return Planner{capabilities: capabilities} }

func (p Planner) Propose(ctx context.Context, plan ApprovedPlan) (Proposal, error) {
	if !plan.Approved || !validToken(plan.PlanRevision) || !validDigest(plan.PlanDigest) || plan.MaximumNodes < 1 || plan.MaximumNodes > maxNodes || len(plan.Work) == 0 || len(plan.Work) > plan.MaximumNodes {
		return Proposal{}, ErrInvalidPlan
	}
	proposal := Proposal{FormatVersion: ProposalFormatVersion, PlanRevision: plan.PlanRevision, PlanDigest: plan.PlanDigest, Nodes: make([]ProposedNode, 0, len(plan.Work))}
	for _, unit := range plan.Work {
		proposal.Nodes = append(proposal.Nodes, ProposedNode(unit))
	}
	normalizeProposal(&proposal)
	if err := p.Validate(ctx, proposal, plan.MaximumNodes); err != nil {
		return Proposal{}, err
	}
	digest, err := proposalDigest(proposal)
	if err != nil {
		return Proposal{}, err
	}
	proposal.Digest = digest
	return proposal, nil
}

func (p Planner) Validate(ctx context.Context, proposal Proposal, maximumNodes int) error {
	if proposal.FormatVersion != ProposalFormatVersion || !validToken(proposal.PlanRevision) || !validDigest(proposal.PlanDigest) || maximumNodes < 1 || maximumNodes > maxNodes || len(proposal.Nodes) == 0 || len(proposal.Nodes) > maximumNodes {
		return ErrInvalidGraph
	}
	nodes := make(map[string]ProposedNode, len(proposal.Nodes))
	integrationKey := ""
	for _, node := range proposal.Nodes {
		if !validNode(node) {
			return ErrInvalidGraph
		}
		if _, exists := nodes[node.Key]; exists {
			return ErrInvalidGraph
		}
		nodes[node.Key] = node
		if node.IntegrationOwner {
			if integrationKey != "" {
				return ErrMissingOwner
			}
			integrationKey = node.Key
		}
	}
	if integrationKey == "" {
		return ErrMissingOwner
	}
	for _, node := range proposal.Nodes {
		for _, dependency := range node.Dependencies {
			if dependency == node.Key {
				return ErrCycle
			}
			if _, exists := nodes[dependency]; !exists {
				return ErrInvalidGraph
			}
		}
	}
	if hasCycle(nodes) {
		return ErrCycle
	}
	for key := range nodes {
		if key != integrationKey && !dependsTransitively(nodes, integrationKey, key) {
			return ErrInvalidGraph
		}
	}
	if !nodes[integrationKey].ValidationOwner {
		return ErrMissingOwner
	}
	for index, left := range proposal.Nodes {
		for _, right := range proposal.Nodes[index+1:] {
			if effectsConflict(left, right) && !dependsTransitively(nodes, left.Key, right.Key) && !dependsTransitively(nodes, right.Key, left.Key) {
				return ErrUnsafeOverlap
			}
		}
	}
	if p.capabilities == nil {
		return ErrUnresolvable
	}
	for _, node := range proposal.Nodes {
		if p.capabilities.ValidateCapability(ctx, node.Capability) != nil {
			return ErrUnresolvable
		}
	}
	return nil
}

func proposalDigest(proposal Proposal) (string, error) {
	proposal.Digest = ""
	wire, err := json.Marshal(proposal)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(wire)
	return hex.EncodeToString(digest[:]), nil
}

func normalizeProposal(proposal *Proposal) {
	sort.Slice(proposal.Nodes, func(i, j int) bool { return proposal.Nodes[i].Key < proposal.Nodes[j].Key })
	for index := range proposal.Nodes {
		node := &proposal.Nodes[index]
		node.Capability.Capabilities = sortedUnique(node.Capability.Capabilities)
		node.Dependencies = sortedUnique(node.Dependencies)
		node.Inputs = sortedUnique(node.Inputs)
		node.Outputs = sortedUnique(node.Outputs)
		node.Scope.Paths = sortedUnique(node.Scope.Paths)
		sort.Slice(node.Effects, func(i, j int) bool {
			return node.Effects[i].Kind+"\x00"+node.Effects[i].Target < node.Effects[j].Kind+"\x00"+node.Effects[j].Target
		})
	}
}

func validNode(node ProposedNode) bool {
	if !validToken(node.Key) || !validCapability(node.Capability) || !validToken(node.Scope.ProjectID) || !validToken(node.Scope.RepositoryKey) || len(node.Dependencies) > maxListItems || len(node.Inputs) == 0 || len(node.Inputs) > maxListItems || len(node.Outputs) == 0 || len(node.Outputs) > maxListItems || len(node.Scope.Paths) == 0 || len(node.Scope.Paths) > maxListItems || len(node.Effects) > maxListItems {
		return false
	}
	if node.Controls.Timeout <= 0 || node.Controls.Timeout > 24*time.Hour || node.Controls.MaximumAttempts == 0 || node.Controls.MaximumAttempts > 10 {
		return false
	}
	for _, values := range [][]string{node.Dependencies, node.Inputs, node.Outputs} {
		for _, value := range values {
			if !validToken(value) {
				return false
			}
		}
	}
	for _, value := range node.Scope.Paths {
		if !validRelativePath(value) {
			return false
		}
	}
	for _, effect := range node.Effects {
		if effect.Kind != "read" && effect.Kind != "repository-write" && effect.Kind != "process" && effect.Kind != "artifact-publish" && effect.Kind != "integration" {
			return false
		}
		if !validTarget(effect.Target) {
			return false
		}
	}
	return true
}

func validCapability(request CapabilityRequest) bool {
	if !validToken(request.Role) || !validToken(request.Complexity) || len(request.Capabilities) == 0 || len(request.Capabilities) > maxListItems {
		return false
	}
	for _, capability := range request.Capabilities {
		if !validToken(capability) {
			return false
		}
	}
	return true
}

func hasCycle(nodes map[string]ProposedNode) bool {
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(key string) bool {
		if state[key] == 1 {
			return true
		}
		if state[key] == 2 {
			return false
		}
		state[key] = 1
		for _, dependency := range nodes[key].Dependencies {
			if visit(dependency) {
				return true
			}
		}
		state[key] = 2
		return false
	}
	for key := range nodes {
		if visit(key) {
			return true
		}
	}
	return false
}

func dependsTransitively(nodes map[string]ProposedNode, nodeKey, dependencyKey string) bool {
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(key string) bool {
		if seen[key] {
			return false
		}
		seen[key] = true
		for _, dependency := range nodes[key].Dependencies {
			if dependency == dependencyKey || visit(dependency) {
				return true
			}
		}
		return false
	}
	return visit(nodeKey)
}

func effectsConflict(left, right ProposedNode) bool {
	if left.Scope.RepositoryKey != right.Scope.RepositoryKey {
		return false
	}
	for _, leftEffect := range left.Effects {
		if leftEffect.Kind == "read" || leftEffect.Kind == "process" || leftEffect.Kind == "artifact-publish" {
			continue
		}
		for _, rightEffect := range right.Effects {
			if rightEffect.Kind == "read" || rightEffect.Kind == "process" || rightEffect.Kind == "artifact-publish" {
				continue
			}
			if pathsOverlap(leftEffect.Target, rightEffect.Target) {
				return true
			}
		}
	}
	return false
}

func pathsOverlap(left, right string) bool {
	left = path.Clean(left)
	right = path.Clean(right)
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func validRelativePath(value string) bool {
	return validTarget(value) && value != "." && !strings.HasPrefix(value, "/") && path.Clean(value) == value && !strings.HasPrefix(value, "../")
}

func validTarget(value string) bool {
	return value != "" && len(value) <= maxTextBytes && !strings.ContainsAny(value, "\x00\r\n") && strings.TrimSpace(value) == value
}

func validToken(value string) bool {
	if value == "" || len(value) > maxTextBytes {
		return false
	}
	for _, char := range value {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' || char == ':') {
			return false
		}
	}
	return true
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func sortedUnique(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	for index := 1; index < len(result); index++ {
		if result[index] == result[index-1] {
			return nil
		}
	}
	return result
}
