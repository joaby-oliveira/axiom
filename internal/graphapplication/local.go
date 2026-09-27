// Package graphapplication composes the concrete local S8 execution path.
package graphapplication

import (
	"context"
	"errors"
	"sync"

	"github.com/rgomids/axiom/internal/coordination"
	"github.com/rgomids/axiom/internal/executiongraph"
	"github.com/rgomids/axiom/internal/gitworkspace"
	"github.com/rgomids/axiom/internal/runtimeadapter"
)

var ErrInvalidComposition = errors.New("invalid local graph execution composition")

type LocalConfiguration struct {
	Repository, WorkspaceRoot, BaseRevision string
	Graph                                   executiongraph.Graph
	GraphStore                              executiongraph.GraphAttemptStore
	RuntimeProfiles                         []runtimeadapter.CommandProfile
	Credentials                             runtimeadapter.CredentialResolver
	Validators                              []gitworkspace.ValidationCommand
	AllocateAttemptID                       func() (string, error)
}

// LocalService is the application boundary used by the T36 operator. It keeps
// graph state, scheduling, Git ownership, integration, validation and Evidence
// on one concrete path without moving policy into a CLI.
type LocalService struct {
	mu           sync.Mutex
	graph        executiongraph.Graph
	workspaces   *gitworkspace.Manager
	scheduler    executiongraph.Scheduler
	integration  executiongraph.IntegrationService
	coordination coordination.AcceptanceVerifier
}

func NewLocalService(ctx context.Context, configuration LocalConfiguration) (*LocalService, error) {
	invocations, err := runtimeadapter.NewInvocationResolver(configuration.RuntimeProfiles, configuration.Credentials)
	if err != nil {
		return nil, err
	}
	return newLocalService(ctx, configuration, invocations)
}

func newLocalService(ctx context.Context, configuration LocalConfiguration, invocations executiongraph.InvocationResolver) (*LocalService, error) {
	if invocations == nil || configuration.GraphStore == nil || len(configuration.Validators) == 0 || !executiongraph.ValidGraph(configuration.Graph) {
		return nil, ErrInvalidComposition
	}
	workspaces, err := gitworkspace.NewManager(ctx, configuration.Repository, configuration.WorkspaceRoot, configuration.BaseRevision, configuration.Graph)
	if err != nil {
		return nil, err
	}
	validator, err := gitworkspace.NewCombinedValidator(workspaces, configuration.Validators)
	if err != nil {
		return nil, err
	}
	return &LocalService{
		graph:        configuration.Graph,
		workspaces:   workspaces,
		scheduler:    executiongraph.NewScheduler(invocations, executiongraph.OSProcessRunner{}, workspaces, configuration.GraphStore, configuration.AllocateAttemptID, nil),
		integration:  executiongraph.NewIntegrationService(workspaces, validator, configuration.GraphStore, configuration.AllocateAttemptID, nil),
		coordination: coordination.AcceptanceVerifier{},
	}, nil
}

func (s *LocalService) PrepareWorkspaces(ctx context.Context) ([]gitworkspace.WorkspaceObservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.workspaces.Prepare(ctx)
}

func (s *LocalService) DispatchReady(ctx context.Context, retryChildIDs map[string]bool, cancelRequested bool) (executiongraph.DispatchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.scheduler.DispatchReady(ctx, executiongraph.DispatchRequest{Graph: s.graph, RetryChildIDs: retryChildIDs, CancelRequested: cancelRequested})
	if err != nil {
		return result, err
	}
	if err := s.workspaces.BindGraph(result.Graph); err != nil {
		return result, err
	}
	s.graph = result.Graph
	return result, nil
}

func (s *LocalService) PreviewIntegration(ctx context.Context, optionalWaivers map[string]string) (executiongraph.IntegrationPreview, []gitworkspace.WorkspaceObservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	observation, results, workspaces, err := s.workspaces.ObserveIntegration(ctx, optionalWaivers)
	if err != nil {
		return executiongraph.IntegrationPreview{}, workspaces, err
	}
	preview, err := s.integration.Preview(s.graph, observation, results)
	return preview, workspaces, err
}

func (s *LocalService) ExecuteIntegration(ctx context.Context, preview executiongraph.IntegrationPreview, authority executiongraph.IntegrationAuthority) (executiongraph.ParentRollup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// The service always returns the last persisted graph, including a running
	// or failed Integration attempt, so local state follows canonical state.
	result, err := s.integration.Execute(ctx, s.graph, preview, authority)
	if bindErr := s.workspaces.BindGraph(result.Graph); bindErr != nil {
		result.Rollup.Status = "recovery_required"
		return result.Rollup, errors.Join(err, bindErr)
	}
	s.graph = result.Graph
	return result.Rollup, err
}

func (s *LocalService) BuildEvidence(evidence executiongraph.Evidence) (executiongraph.Evidence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return executiongraph.BuildEvidence(s.graph, evidence, s.coordination)
}

func (s *LocalService) Graph() executiongraph.Graph {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.graph
}
