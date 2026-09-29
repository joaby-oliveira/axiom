package local

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/rgomids/axiom/internal/project"
	"github.com/rgomids/axiom/internal/workitem"
)

type WorkItemStore struct {
	root  string
	hooks publicationHooks
}

type workItemDTO struct {
	FormatVersion      int    `json:"formatVersion"`
	ProjectID          string `json:"projectId"`
	RepositoryKey      string `json:"repositoryKey"`
	ProviderRepository string `json:"providerRepository"`
	Number             int    `json:"number"`
	URL                string `json:"url"`
	State              string `json:"state"`
}

type createAttemptDTO struct {
	FormatVersion int    `json:"formatVersion"`
	ProjectID     string `json:"projectId"`
	RepositoryKey string `json:"repositoryKey"`
	Provider      string `json:"provider"`
	Resource      string `json:"resource"`
	Operation     string `json:"operation"`
	Correlation   string `json:"correlation"`
	PreviewDigest string `json:"previewDigest"`
	State         string `json:"state"`
	ExternalID    string `json:"externalId,omitempty"`
}

func NewWorkItemStore(root string) (WorkItemStore, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) == string(filepath.Separator) {
		return WorkItemStore{}, ErrUnsafe
	}
	canonical, err := trustedCanonical(root)
	if err != nil {
		return WorkItemStore{}, err
	}
	return WorkItemStore{root: canonical}, nil
}

func (s WorkItemStore) Save(ctx context.Context, link workitem.Link) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	number, valid := validV1WorkItemLink(link)
	if !valid {
		return ErrUnsafe
	}
	wire, err := json.Marshal(workItemDTO{1, link.ProjectID, link.RepositoryKey, link.Resource, number, link.URL, link.State})
	if err != nil {
		return err
	}
	wire = append(wire, '\n')
	root, items, projectRoot, err := s.openProject(link.ProjectID, true)
	if err != nil {
		return err
	}
	defer root.Close()
	defer items.Close()
	defer projectRoot.Close()
	locks, err := lockRoots(true, root, items, projectRoot)
	if err != nil {
		return err
	}
	defer closeFiles(locks)
	name, err := workItemSaveName(projectRoot, link)
	if err != nil {
		return workItemStoreError(err)
	}
	create := link.Revision == ([32]byte{})
	var expected []byte
	if !create {
		expected, err = readPublishedFile(projectRoot, name)
		if err != nil {
			return err
		}
		if sha256.Sum256(expected) != link.Revision {
			return workItemStoreError(ErrConflict)
		}
	}
	err = publishFile(ctx, projectRoot, name, expected, wire, create, s.hooks)
	return workItemStoreError(err)
}

func (s WorkItemStore) Load(ctx context.Context, projectID, repositoryKey, provider, resource, externalID string) (workitem.Link, error) {
	if err := ctx.Err(); err != nil {
		return workitem.Link{}, err
	}
	if len(project.ValidateIdentity(projectID, "work-item")) != 0 || !project.ValidSlug(repositoryKey) || !validV1GitHubNumber(externalID) {
		return workitem.Link{}, ErrUnsafe
	}
	if (provider == "") != (resource == "") || provider != "" && (provider != "github" || !validV1GitHubRepository(resource)) {
		return workitem.Link{}, ErrUnsafe
	}
	root, items, projectRoot, err := s.openProject(projectID, false)
	if err != nil {
		return workitem.Link{}, workItemStoreError(err)
	}
	defer root.Close()
	defer items.Close()
	defer projectRoot.Close()
	locks, err := lockRoots(false, root, items, projectRoot)
	if err != nil {
		return workitem.Link{}, workItemStoreError(err)
	}
	defer closeFiles(locks)
	if provider == "" {
		return findWorkItem(projectRoot, projectID, repositoryKey, externalID)
	}
	return loadExactWorkItem(projectRoot, projectID, repositoryKey, provider, resource, externalID)
}

func (s WorkItemStore) SaveCreateAttempt(ctx context.Context, attempt workitem.CreateAttempt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validCreateAttempt(attempt) {
		return ErrUnsafe
	}
	dto := createAttemptDTO{
		FormatVersion: 1, ProjectID: attempt.Target.ProjectID, RepositoryKey: attempt.Target.RepositoryKey,
		Provider: attempt.Target.Provider, Resource: attempt.Target.Resource, Operation: "create",
		Correlation: attempt.Correlation, PreviewDigest: attempt.PreviewDigest, State: string(attempt.State), ExternalID: attempt.ExternalID,
	}
	wire, err := json.Marshal(dto)
	if err != nil {
		return err
	}
	wire = append(wire, '\n')
	root, items, projectRoot, err := s.openProject(attempt.Target.ProjectID, true)
	if err != nil {
		return err
	}
	defer root.Close()
	defer items.Close()
	defer projectRoot.Close()
	locks, err := lockRoots(true, root, items, projectRoot)
	if err != nil {
		return err
	}
	defer closeFiles(locks)
	name := createAttemptName(attempt.Target)
	create := attempt.Revision == ([32]byte{})
	var expected []byte
	if !create {
		expected, err = readPublishedFile(projectRoot, name)
		if err != nil {
			return workItemStoreError(err)
		}
		if sha256.Sum256(expected) != attempt.Revision {
			return workItemStoreError(ErrConflict)
		}
	}
	return workItemStoreError(publishFile(ctx, projectRoot, name, expected, wire, create, s.hooks))
}

func (s WorkItemStore) LoadCreateAttempt(ctx context.Context, target workitem.DraftTarget) (workitem.CreateAttempt, error) {
	if err := ctx.Err(); err != nil {
		return workitem.CreateAttempt{}, err
	}
	if !validDraftTarget(target) {
		return workitem.CreateAttempt{}, ErrUnsafe
	}
	root, items, projectRoot, err := s.openProject(target.ProjectID, false)
	if err != nil {
		return workitem.CreateAttempt{}, workItemStoreError(err)
	}
	defer root.Close()
	defer items.Close()
	defer projectRoot.Close()
	locks, err := lockRoots(false, root, items, projectRoot)
	if err != nil {
		return workitem.CreateAttempt{}, workItemStoreError(err)
	}
	defer closeFiles(locks)
	wire, err := readPublishedFile(projectRoot, createAttemptName(target))
	if err != nil {
		return workitem.CreateAttempt{}, workItemStoreError(err)
	}
	attempt, err := decodeCreateAttempt(wire)
	if err != nil {
		return workitem.CreateAttempt{}, err
	}
	if attempt.Target != target {
		return workitem.CreateAttempt{}, ErrUnsafe
	}
	attempt.Revision = sha256.Sum256(wire)
	return attempt, nil
}

func workItemStoreError(err error) error {
	if errors.Is(err, ErrNotFound) || os.IsNotExist(err) {
		return errors.Join(err, workitem.ErrNotFound)
	}
	if errors.Is(err, ErrRecoveryRequired) {
		return errors.Join(err, workitem.ErrRecoveryRequired)
	}
	if errors.Is(err, ErrConflict) {
		return errors.Join(err, workitem.ErrConflict)
	}
	return err
}

func (s WorkItemStore) openProject(projectID string, create bool) (*os.Root, *os.Root, *os.Root, error) {
	openRoot := existingPrivateRoot
	openChild := existingPrivateChild
	if create {
		openRoot = privateRoot
		openChild = privateChild
	}
	root, err := openRoot(s.root)
	if err != nil {
		return nil, nil, nil, err
	}
	items, err := openChild(root, "work-items")
	if err != nil {
		root.Close()
		return nil, nil, nil, err
	}
	projectRoot, err := openChild(items, projectID)
	if err != nil {
		items.Close()
		root.Close()
		return nil, nil, nil, err
	}
	return root, items, projectRoot, nil
}

func decodeWorkItem(wire []byte) (workitem.Link, error) {
	decoder := json.NewDecoder(io.LimitReader(bytes.NewReader(wire), MaxRecordBytes+1))
	decoder.DisallowUnknownFields()
	var dto workItemDTO
	if err := decoder.Decode(&dto); err != nil {
		return workitem.Link{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return workitem.Link{}, ErrUnsafe
	}
	link := workitem.Link{ProjectID: dto.ProjectID, RepositoryKey: dto.RepositoryKey, Provider: "github", Resource: dto.ProviderRepository, ExternalID: strconv.Itoa(dto.Number), URL: dto.URL, State: dto.State}
	if dto.FormatVersion != 1 {
		return workitem.Link{}, ErrUnsafe
	}
	if _, valid := validV1WorkItemLink(link); !valid {
		return workitem.Link{}, ErrUnsafe
	}
	return link, nil
}

func validV1WorkItemLink(link workitem.Link) (int, bool) {
	if len(project.ValidateIdentity(link.ProjectID, "work-item")) != 0 || !project.ValidSlug(link.RepositoryKey) || link.Provider != "github" || !validV1GitHubRepository(link.Resource) || !validV1GitHubNumber(link.ExternalID) {
		return 0, false
	}
	number, _ := strconv.Atoi(link.ExternalID)
	expectedURL := fmt.Sprintf("https://github.com/%s/issues/%d", link.Resource, number)
	return number, link.URL == expectedURL && (link.State == "OPEN" || link.State == "CLOSED")
}

func validV1GitHubNumber(value string) bool {
	number, err := strconv.Atoi(value)
	return err == nil && number > 0 && strconv.Itoa(number) == value
}

var v1GitHubSegment = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func validV1GitHubRepository(value string) bool {
	parts := strings.Split(value, "/")
	return len(parts) == 2 && parts[0] != "." && parts[0] != ".." && parts[1] != "." && parts[1] != ".." && v1GitHubSegment.MatchString(parts[0]) && v1GitHubSegment.MatchString(parts[1])
}

func loadExactWorkItem(root *os.Root, projectID, repositoryKey, provider, resource, externalID string) (workitem.Link, error) {
	exact := workItemName(repositoryKey, provider, resource, externalID)
	wire, err := readPublishedFile(root, exact)
	legacy := false
	if os.IsNotExist(err) {
		legacy = true
		wire, err = readPublishedFile(root, legacyWorkItemName(repositoryKey, externalID))
	}
	if err != nil {
		return workitem.Link{}, workItemStoreError(err)
	}
	link, err := decodeWorkItem(wire)
	if err != nil {
		return workitem.Link{}, err
	}
	if !sameWorkItemIdentity(link, projectID, repositoryKey, provider, resource, externalID) {
		if !legacy {
			return workitem.Link{}, ErrUnsafe
		}
		return workitem.Link{}, workItemStoreError(ErrNotFound)
	}
	link.Revision = sha256.Sum256(wire)
	return link, nil
}

func findWorkItem(root *os.Root, projectID, repositoryKey, externalID string) (workitem.Link, error) {
	if pending, err := protocolStatePresent(root); err != nil || pending {
		if err != nil {
			return workitem.Link{}, err
		}
		return workitem.Link{}, workItemStoreError(ErrRecoveryRequired)
	}
	names, err := readDirectoryNamesBounded(root, maxLocalDirectoryEntries)
	if err != nil {
		return workitem.Link{}, err
	}
	prefix := repositoryKey + "-"
	matches := make([]workitem.Link, 0, 1)
	for _, name := range names {
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		wire, readErr := readPrivateFileBounded(root, name, MaxRecordBytes)
		if readErr != nil {
			return workitem.Link{}, readErr
		}
		link, decodeErr := decodeWorkItem(wire)
		if decodeErr != nil {
			return workitem.Link{}, decodeErr
		}
		if link.ProjectID == projectID && link.RepositoryKey == repositoryKey && link.ExternalID == externalID {
			link.Revision = sha256.Sum256(wire)
			matches = append(matches, link)
		}
	}
	if len(matches) == 0 {
		return workitem.Link{}, workItemStoreError(ErrNotFound)
	}
	if len(matches) != 1 {
		return workitem.Link{}, workItemStoreError(ErrConflict)
	}
	return matches[0], nil
}

func workItemSaveName(root *os.Root, link workitem.Link) (string, error) {
	exact := workItemName(link.RepositoryKey, link.Provider, link.Resource, link.ExternalID)
	legacy := legacyWorkItemName(link.RepositoryKey, link.ExternalID)
	if link.Revision == ([32]byte{}) {
		wire, err := readPublishedFile(root, legacy)
		if err == nil {
			stored, decodeErr := decodeWorkItem(wire)
			if decodeErr != nil {
				return "", decodeErr
			}
			if sameWorkItemIdentity(stored, link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID) {
				return "", ErrConflict
			}
		} else if !os.IsNotExist(err) {
			return "", err
		}
		return exact, nil
	}
	for _, name := range []string{exact, legacy} {
		wire, err := readPublishedFile(root, name)
		if err == nil {
			stored, decodeErr := decodeWorkItem(wire)
			if decodeErr != nil {
				return "", decodeErr
			}
			if sameWorkItemIdentity(stored, link.ProjectID, link.RepositoryKey, link.Provider, link.Resource, link.ExternalID) && sha256.Sum256(wire) == link.Revision {
				return name, nil
			}
			continue
		}
		if !os.IsNotExist(err) {
			return "", err
		}
	}
	return exact, nil
}

func sameWorkItemIdentity(link workitem.Link, projectID, repositoryKey, provider, resource, externalID string) bool {
	return link.ProjectID == projectID && link.RepositoryKey == repositoryKey && link.Provider == provider && link.Resource == resource && link.ExternalID == externalID
}

func workItemName(repositoryKey, provider, resource, externalID string) string {
	digest := sha256.Sum256([]byte(provider + "\x00" + resource + "\x00" + externalID))
	return fmt.Sprintf("%s-%x.json", repositoryKey, digest)
}

func legacyWorkItemName(repositoryKey, externalID string) string {
	return fmt.Sprintf("%s-%s.json", repositoryKey, externalID)
}

func createAttemptName(target workitem.DraftTarget) string {
	digest := sha256.Sum256([]byte(target.RepositoryKey + "\x00" + target.Provider + "\x00" + target.Resource))
	return fmt.Sprintf(".axiom-create-%x.json", digest)
}

func validDraftTarget(target workitem.DraftTarget) bool {
	return len(project.ValidateIdentity(target.ProjectID, "work-item")) == 0 && project.ValidSlug(target.RepositoryKey) && target.Provider == "github" && validV1GitHubRepository(target.Resource)
}

func validCreateAttempt(attempt workitem.CreateAttempt) bool {
	if !validDraftTarget(attempt.Target) || !validDigest(attempt.Correlation) || !validDigest(attempt.PreviewDigest) {
		return false
	}
	switch attempt.State {
	case workitem.CreateAttemptPending, workitem.CreateAttemptRetryAllowed:
		return attempt.ExternalID == ""
	case workitem.CreateAttemptConfirmed:
		return validV1GitHubNumber(attempt.ExternalID)
	default:
		return false
	}
}

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validDigest(value string) bool { return digestPattern.MatchString(value) }

func decodeCreateAttempt(wire []byte) (workitem.CreateAttempt, error) {
	decoder := json.NewDecoder(io.LimitReader(bytes.NewReader(wire), MaxRecordBytes+1))
	decoder.DisallowUnknownFields()
	var dto createAttemptDTO
	if err := decoder.Decode(&dto); err != nil {
		return workitem.CreateAttempt{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return workitem.CreateAttempt{}, ErrUnsafe
	}
	attempt := workitem.CreateAttempt{
		Target:      workitem.DraftTarget{ProjectID: dto.ProjectID, RepositoryKey: dto.RepositoryKey, Provider: dto.Provider, Resource: dto.Resource},
		Correlation: dto.Correlation, PreviewDigest: dto.PreviewDigest, State: workitem.CreateAttemptState(dto.State), ExternalID: dto.ExternalID,
	}
	if dto.FormatVersion != 1 || dto.Operation != "create" || !validCreateAttempt(attempt) {
		return workitem.CreateAttempt{}, ErrUnsafe
	}
	return attempt, nil
}
