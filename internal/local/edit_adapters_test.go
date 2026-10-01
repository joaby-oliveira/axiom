package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const recordedSourceManifest = "schemaVersion: 1\nproject:\n  id: 123e4567-e89b-42d3-a456-426614174000\n  slug: sample\n  name: Sample\n"

// Issue #132: EDIT reads the exact source a protected record names, whether it
// is the configured <root>/<slug> or another absolute source installed with
// project install, through the existing safe loading rules only.
func TestInspectRecordedSourceLoadsConfiguredAndArbitrarySafeSources(t *testing.T) {
	root := filepath.Join(privateTestRoot(t), "projects")
	store, err := NewPortableStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), "sample", []byte(recordedSourceManifest)); err != nil {
		t.Fatal(err)
	}
	configured, err := store.InspectRecordedSource(context.Background(), filepath.Join(root, "sample"), "sample")
	expected, inspectErr := store.Inspect(context.Background(), "sample")
	if err != nil || inspectErr != nil || !configured.Exists || configured.Revision != expected.Revision {
		t.Fatalf("configured source = %+v %v; store = %+v %v", configured, err, expected, inspectErr)
	}

	external := privateTestRoot(t)
	if err := os.WriteFile(filepath.Join(external, manifestName), []byte(recordedSourceManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	arbitrary, err := store.InspectRecordedSource(context.Background(), external, "sample")
	if err != nil || !arbitrary.Exists || arbitrary.Revision != expected.Revision || arbitrary.Snapshot.Project().State().Slug != "sample" {
		t.Fatalf("arbitrary recorded source = %+v %v", arbitrary, err)
	}

	// A stage left by an interrupted portable update under the configured root
	// keeps the store's recovery classification.
	if err := os.WriteFile(filepath.Join(root, "sample", ".lingo-manifest-interrupted"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectRecordedSource(context.Background(), filepath.Join(root, "sample"), "sample"); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("configured source with interrupted stage = %v", err)
	}
}

func TestInspectRecordedSourceRejectsUnsafeOrMissingSources(t *testing.T) {
	store, err := NewPortableStore(filepath.Join(privateTestRoot(t), "projects"))
	if err != nil {
		t.Fatal(err)
	}
	valid := privateTestRoot(t)
	if err := os.WriteFile(filepath.Join(valid, manifestName), []byte(recordedSourceManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(privateTestRoot(t), "link")
	if err := os.Symlink(valid, link); err != nil {
		t.Fatal(err)
	}
	ancestorLink := filepath.Join(privateTestRoot(t), "ancestor")
	if err := os.Symlink(filepath.Dir(valid), ancestorLink); err != nil {
		t.Fatal(err)
	}
	shared := privateTestRoot(t)
	if err := os.WriteFile(filepath.Join(shared, manifestName), []byte(recordedSourceManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	extra := privateTestRoot(t)
	for _, name := range []string{manifestName, "notes.txt"} {
		if err := os.WriteFile(filepath.Join(extra, name), []byte(recordedSourceManifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for name, source := range map[string]string{
		"relative source":        "projects/sample",
		"unclean source":         valid + "/../" + filepath.Base(valid),
		"symlinked source":       link,
		"symlinked ancestor":     filepath.Join(ancestorLink, filepath.Base(valid)),
		"shared permissions":     shared,
		"unexpected source file": extra,
	} {
		t.Run(name, func(t *testing.T) {
			if observation, err := store.InspectRecordedSource(context.Background(), source, "sample"); !errors.Is(err, ErrUnsafe) || observation.Exists {
				t.Fatalf("observation=%+v err=%v", observation, err)
			}
		})
	}
	if _, err := store.InspectRecordedSource(context.Background(), valid, "Not A Slug"); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("invalid slug = %v", err)
	}
	missing, err := store.InspectRecordedSource(context.Background(), filepath.Join(valid, "absent"), "sample")
	if err != nil || missing.Exists {
		t.Fatalf("missing source = %+v %v", missing, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.InspectRecordedSource(ctx, valid, "sample"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled inspection = %v", err)
	}
}
