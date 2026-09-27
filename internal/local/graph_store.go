package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/rgomids/axiom/internal/executiongraph"
)

type GraphStore struct {
	root  string
	hooks publicationHooks
}

func NewGraphStore(root string) (GraphStore, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) == string(filepath.Separator) {
		return GraphStore{}, ErrUnsafe
	}
	canonical, err := trustedCanonical(root)
	if err != nil {
		return GraphStore{}, err
	}
	return GraphStore{root: canonical}, nil
}

func (s GraphStore) Create(ctx context.Context, graph executiongraph.Graph) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	wire, err := executiongraph.EncodeGraph(graph)
	if err != nil {
		return err
	}
	root, graphs, version, project, err := s.openProject(graph.Children[0].Envelope.Scope.ProjectID, true)
	if err != nil {
		return err
	}
	defer root.Close()
	defer graphs.Close()
	defer version.Close()
	defer project.Close()
	locks, err := lockRoots(true, root, graphs, version, project)
	if err != nil {
		return err
	}
	defer closeFiles(locks)
	return publishFile(ctx, project, graphName(graph.Parent.ExecutionID), nil, wire, true, s.hooks)
}

func (s GraphStore) Save(ctx context.Context, graph executiongraph.Graph) (executiongraph.Graph, error) {
	if err := ctx.Err(); err != nil {
		return graph, err
	}
	wire, err := executiongraph.EncodeGraph(graph)
	if err != nil || graph.StorageRevision == ([sha256.Size]byte{}) {
		return graph, executiongraph.ErrInvalidGraph
	}
	root, graphs, version, project, err := s.openProject(graph.Children[0].Envelope.Scope.ProjectID, false)
	if err != nil {
		return graph, err
	}
	defer root.Close()
	defer graphs.Close()
	defer version.Close()
	defer project.Close()
	locks, err := lockRoots(true, root, graphs, version, project)
	if err != nil {
		return graph, err
	}
	defer closeFiles(locks)
	name := graphName(graph.Parent.ExecutionID)
	expected, err := readPublishedFile(project, name)
	if err != nil {
		return graph, err
	}
	if sha256.Sum256(expected) != graph.StorageRevision {
		return graph, ErrConflict
	}
	if err := publishFile(ctx, project, name, expected, wire, false, s.hooks); err != nil {
		return graph, err
	}
	graph.StorageRevision = sha256.Sum256(wire)
	return graph, nil
}

func (s GraphStore) Load(ctx context.Context, projectID, parentID string) (executiongraph.Graph, error) {
	if err := ctx.Err(); err != nil {
		return executiongraph.Graph{}, err
	}
	if !validGraphAddress(projectID, parentID) {
		return executiongraph.Graph{}, ErrUnsafe
	}
	root, graphs, version, project, err := s.openProject(projectID, false)
	if err != nil {
		return executiongraph.Graph{}, err
	}
	defer root.Close()
	defer graphs.Close()
	defer version.Close()
	defer project.Close()
	locks, err := lockRoots(false, root, graphs, version, project)
	if err != nil {
		return executiongraph.Graph{}, err
	}
	defer closeFiles(locks)
	wire, err := readPublishedFile(project, graphName(parentID))
	if err != nil {
		return executiongraph.Graph{}, err
	}
	graph, err := executiongraph.DecodeGraph(wire)
	if err != nil || graph.Parent.ExecutionID != parentID || graph.Children[0].Envelope.Scope.ProjectID != projectID {
		return executiongraph.Graph{}, ErrUnsafe
	}
	graph.StorageRevision = sha256.Sum256(wire)
	return graph, nil
}

func (s GraphStore) openProject(projectID string, create bool) (*os.Root, *os.Root, *os.Root, *os.Root, error) {
	openRoot := existingPrivateRoot
	openChild := existingPrivateChild
	if create {
		openRoot = privateRoot
		openChild = privateChild
	}
	root, err := openRoot(s.root)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	graphs, err := openChild(root, "graphs")
	if err != nil {
		root.Close()
		return nil, nil, nil, nil, err
	}
	version, err := openChild(graphs, "v1")
	if err != nil {
		graphs.Close()
		root.Close()
		return nil, nil, nil, nil, err
	}
	project, err := openChild(version, projectID)
	if err != nil {
		version.Close()
		graphs.Close()
		root.Close()
		return nil, nil, nil, nil, err
	}
	return root, graphs, version, project, nil
}

func graphName(parentID string) string {
	digest := sha256.Sum256([]byte(parentID))
	return hex.EncodeToString(digest[:]) + ".json"
}

func validGraphAddress(projectID, parentID string) bool {
	return validGraphToken(projectID) && len(parentID) == 36
}

func validGraphToken(value string) bool {
	if value == "" || len(value) > 512 {
		return false
	}
	for _, char := range value {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' || char == ':') {
			return false
		}
	}
	return true
}

func graphStoreError(err error) error {
	if errors.Is(err, ErrRecoveryRequired) {
		return errors.Join(err, executiongraph.ErrInvalidGraph)
	}
	return err
}
