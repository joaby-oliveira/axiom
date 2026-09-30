// Package runtimeadapter contains the two concrete S8 Runtime boundaries.
package runtimeadapter

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rgomids/axiom/internal/executiongraph"
	"github.com/rgomids/axiom/internal/runtimeprofile"
)

var ErrInvalidAdapterConfiguration = errors.New("invalid runtime adapter configuration")

type Inventory struct {
	observations map[string]runtimeprofile.Observation
}

func NewInventory(observations []runtimeprofile.Observation) (Inventory, error) {
	result := Inventory{observations: make(map[string]runtimeprofile.Observation, len(observations))}
	for _, observation := range observations {
		if observation.RuntimeID != "codex" && observation.RuntimeID != "claude" || observation.Adapter != observation.RuntimeID || observation.Revision == 0 || observation.ObservedAt.IsZero() {
			return Inventory{}, ErrInvalidAdapterConfiguration
		}
		if _, exists := result.observations[observation.RuntimeID]; exists {
			return Inventory{}, ErrInvalidAdapterConfiguration
		}
		copyObservation := observation
		copyObservation.CapabilityStatus = make(map[string]runtimeprofile.CapabilityStatus, len(observation.CapabilityStatus))
		for capability, status := range observation.CapabilityStatus {
			copyObservation.CapabilityStatus[capability] = status
		}
		result.observations[observation.RuntimeID] = copyObservation
	}
	return result, nil
}

func (i Inventory) Observe(_ context.Context, runtimeID string) (runtimeprofile.Observation, error) {
	observation, exists := i.observations[runtimeID]
	if !exists {
		return runtimeprofile.Observation{}, ErrInvalidAdapterConfiguration
	}
	return observation, nil
}

type CommandProfile struct {
	RuntimeID, ModelProfileID string
	Executable                string
	Model                     string
	Arguments                 []string
	Environment               []string
	CredentialReference       string
	OutputMax                 int
}

type CredentialResolver interface {
	ResolveEnvironment(context.Context, string) ([]string, error)
}

type InvocationResolver struct {
	profiles    map[string]CommandProfile
	credentials CredentialResolver
}

func NewInvocationResolver(profiles []CommandProfile, credentials CredentialResolver) (InvocationResolver, error) {
	resolver := InvocationResolver{profiles: make(map[string]CommandProfile, len(profiles)), credentials: credentials}
	for _, profile := range profiles {
		if !validCommandProfile(profile) {
			return InvocationResolver{}, ErrInvalidAdapterConfiguration
		}
		key := profile.RuntimeID + "\x00" + profile.ModelProfileID
		if _, exists := resolver.profiles[key]; exists {
			return InvocationResolver{}, ErrInvalidAdapterConfiguration
		}
		profile.Arguments = append([]string(nil), profile.Arguments...)
		profile.Environment = append([]string(nil), profile.Environment...)
		resolver.profiles[key] = profile
	}
	return resolver, nil
}

func (r InvocationResolver) ResolveInvocation(ctx context.Context, child executiongraph.ChildExecution) (executiongraph.Invocation, error) {
	resolution := child.Envelope.Resolution
	profile, exists := r.profiles[resolution.RuntimeID+"\x00"+resolution.ModelProfileID]
	if !exists {
		return executiongraph.Invocation{}, ErrInvalidAdapterConfiguration
	}
	argv := []string{profile.Executable}
	switch profile.RuntimeID {
	case "codex":
		argv = append(argv, "exec", "--model", profile.Model)
	case "claude":
		argv = append(argv, "--print", "--model", profile.Model)
	default:
		return executiongraph.Invocation{}, ErrInvalidAdapterConfiguration
	}
	argv = append(argv, profile.Arguments...)
	environment := append([]string(nil), profile.Environment...)
	if profile.CredentialReference != "" {
		if r.credentials == nil {
			return executiongraph.Invocation{}, ErrInvalidAdapterConfiguration
		}
		credentialEnvironment, err := r.credentials.ResolveEnvironment(ctx, profile.CredentialReference)
		if err != nil {
			return executiongraph.Invocation{}, err
		}
		environment = append(environment, credentialEnvironment...)
	}
	sort.Strings(environment)
	return executiongraph.Invocation{RuntimeID: profile.RuntimeID, Argv: argv, CWD: child.Envelope.Workspace, Env: environment, OutputMax: profile.OutputMax}, nil
}

func validCommandProfile(profile CommandProfile) bool {
	if profile.RuntimeID != "codex" && profile.RuntimeID != "claude" || profile.ModelProfileID == "" || profile.Model == "" || !filepath.IsAbs(profile.Executable) || profile.OutputMax <= 0 || profile.OutputMax > executiongraph.MaxCapturedOutputBytes || len(profile.Arguments) > 32 || len(profile.Environment) > 32 {
		return false
	}
	if strings.TrimSuffix(strings.ToLower(filepath.Base(profile.Executable)), ".exe") != profile.RuntimeID {
		return false
	}
	for _, argument := range profile.Arguments {
		if argument == "" || strings.ContainsRune(argument, '\x00') {
			return false
		}
	}
	for _, environment := range profile.Environment {
		if !strings.Contains(environment, "=") || strings.ContainsRune(environment, '\x00') {
			return false
		}
	}
	return profile.CredentialReference == "" || (!strings.ContainsAny(profile.CredentialReference, "\x00\r\n=") && len(profile.CredentialReference) <= 256)
}
