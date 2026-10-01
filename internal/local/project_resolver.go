package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

type ResolutionStatus string

const (
	ResolutionFound  ResolutionStatus = "found"
	ResolutionFailed ResolutionStatus = "failed"
)

type ResolvedProject struct {
	ID, Slug, Source string
	Repositories     []ResolvedRepository
}

type ResolvedRepository struct {
	Key, Path, CanonicalIdentity string
}

type ResolutionResult struct {
	Status   ResolutionStatus
	Category string
	Project  ResolvedProject
}

// Resolve selects one installed Project by exact canonical ID or observed slug.
// It reads only protected ID-addressed local records and never consults caller CWD.
func (s InstallationStore) Resolve(ctx context.Context, selector string) ResolutionResult {
	result := s.Select(ctx, selector)
	if result.Status != ResolutionFound {
		return result
	}
	if category := validateResolvedLocations(result.Project); category != "" {
		return failedResolution(category)
	}
	return result
}

// Select performs the same exact UUID/slug selection as Resolve without
// checking per-binding availability, so an edit can repair or remove a broken
// binding. Callers remain responsible for source confinement and safe loading.
func (s InstallationStore) Select(ctx context.Context, selector string) ResolutionResult {
	if selector == "" {
		return failedResolution("invalid_project_selector")
	}
	if err := ctx.Err(); err != nil {
		return failedResolution("cancelled")
	}
	root, err := existingPrivateRoot(s.root)
	if errors.Is(err, ErrNotFound) {
		return failedResolution("project_not_found")
	}
	if err != nil {
		return failedResolution("invalid_existing_local_state")
	}
	defer root.Close()
	projects, err := existingPrivateChild(root, "projects")
	if errors.Is(err, ErrNotFound) {
		return failedResolution("project_not_found")
	}
	if err != nil {
		return failedResolution("invalid_existing_local_state")
	}
	defer projects.Close()
	names, err := childNames(projects)
	if err != nil {
		return failedResolution("invalid_existing_local_state")
	}
	matches := make([]ResolvedProject, 0, 1)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return failedResolution("cancelled")
		}
		candidate, category := readResolvedProject(projects, name)
		if category != "" {
			return failedResolution(category)
		}
		if candidate.ID == selector || candidate.Slug == selector {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return failedResolution("project_not_found")
	}
	if len(matches) > 1 {
		return failedResolution("project_ambiguous")
	}
	return ResolutionResult{Status: ResolutionFound, Category: "project_resolved", Project: matches[0]}
}

func childNames(root *os.Root) ([]string, error) {
	names, err := readDirectoryNamesBounded(root, maxLocalDirectoryEntries)
	sort.Strings(names)
	return names, err
}

func readResolvedProject(projects *os.Root, name string) (ResolvedProject, string) {
	target, err := existingPrivateChild(projects, name)
	if err != nil {
		return ResolvedProject{}, "invalid_existing_local_state"
	}
	defer target.Close()
	if category := installationDirectoryIssue(target); category != "" {
		return ResolvedProject{}, category
	}
	wire, err := readPrivateFile(target, "installation.json")
	if err != nil {
		return ResolvedProject{}, "invalid_existing_local_state"
	}
	record, _, issues := DecodeObservedRecord(wire, true)
	if len(issues) != 0 {
		return ResolvedProject{}, "invalid_existing_local_state"
	}
	state := record.State()
	if name != state.ProjectID {
		return ResolvedProject{}, "invalid_existing_local_state"
	}
	result := ResolvedProject{ID: state.ProjectID, Slug: state.ObservedSlug, Source: state.SourceLocation}
	result.Repositories = make([]ResolvedRepository, 0, len(state.Repositories))
	for _, binding := range state.Repositories {
		result.Repositories = append(result.Repositories, ResolvedRepository{Key: binding.RepositoryKey, Path: binding.ExplicitPath, CanonicalIdentity: binding.CanonicalIdentity})
	}
	return result, ""
}

func validateResolvedLocations(project ResolvedProject) string {
	if !availableDirectory(project.Source) {
		return "project_source_unavailable"
	}
	for _, repository := range project.Repositories {
		if !availableDirectory(repository.Path) {
			return "repository_unavailable"
		}
		if repository.CanonicalIdentity != "" {
			identity, err := DirectoryIdentity(repository.Path)
			if err != nil || identity != repository.CanonicalIdentity {
				return "repository_unavailable"
			}
		}
	}
	return ""
}

func availableDirectory(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

func failedResolution(category string) ResolutionResult {
	return ResolutionResult{Status: ResolutionFailed, Category: category}
}
