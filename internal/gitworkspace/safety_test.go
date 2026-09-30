package gitworkspace_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/rgomids/axiom/internal/testfs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rgomids/axiom/internal/gitworkspace"
)

// filterCommand is executed by Git through its own shell when a driver is
// live. It only creates the sentinel, so its presence proves execution.
func filterCommand(sentinel string) string {
	return "touch '" + sentinel + "'"
}

func assertNoSentinel(t *testing.T, sentinel string) {
	t.Helper()
	if _, err := os.Lstat(sentinel); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("filter helper executed: sentinel stat err=%v", err)
	}
}

func assertSentinel(t *testing.T, sentinel string) {
	t.Helper()
	if _, err := os.Lstat(sentinel); err != nil {
		t.Fatalf("control filter was not live: %v", err)
	}
}

// filteredRepository commits a tracked file routed through the "sentinel"
// filter by attribute and configures the given driver variable.
func filteredRepository(t *testing.T, directory, variable, command string) repositoryFixture {
	t.Helper()
	repository := initRepository(t, directory)
	writeFile(t, filepath.Join(directory, ".gitattributes"), "*.txt filter=sentinel\n")
	git(t, directory, "add", ".gitattributes")
	git(t, directory, "-c", "user.name=Axiom Test", "-c", "user.email=axiom@example.invalid", "commit", "-m", "attributes")
	git(t, directory, "config", "filter.sentinel."+variable, command)
	repository.revision = git(t, directory, "rev-parse", "HEAD")
	repository.tree = git(t, directory, "rev-parse", "HEAD^{tree}")
	return repository
}

// sharedCheckoutState observes the shared checkout without Git commands that
// could run a live filter: HEAD, tree, raw index entries, untracked names and
// the bytes of every tracked file read directly from disk.
func sharedCheckoutState(t *testing.T, repository string) string {
	t.Helper()
	state := git(t, repository, "rev-parse", "HEAD") + git(t, repository, "rev-parse", "HEAD^{tree}") + git(t, repository, "ls-files", "--stage") + git(t, repository, "ls-files", "--others")
	for _, name := range strings.Split(git(t, repository, "ls-files"), "\n") {
		content, err := os.ReadFile(filepath.Join(repository, name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(content)
		state += name + ":" + hex.EncodeToString(digest[:]) + "\n"
	}
	return state
}

func TestManagerRejectsExecutableFilterConfigurationBeforeCheckout(t *testing.T) {
	for _, variable := range []string{"clean", "smudge", "process"} {
		t.Run(variable, func(t *testing.T) {
			root := canonicalTempDir(t)

			controlSentinel := filepath.Join(root, "control-sentinel")
			control := filteredRepository(t, filepath.Join(root, "control"), variable, filterCommand(controlSentinel))
			if variable == "clean" {
				writeFile(t, filepath.Join(control.repository, "shared.txt"), "changed\n")
				_, _ = gitAllowFailure(control.repository, "add", "shared.txt")
			} else {
				_, _ = gitAllowFailure(control.repository, "worktree", "add", "--detach", filepath.Join(root, "control-worktree"), control.revision)
			}
			assertSentinel(t, controlSentinel)

			sentinel := filepath.Join(root, "sentinel")
			repository := filteredRepository(t, filepath.Join(root, "repository"), variable, filterCommand(sentinel))
			before := sharedCheckoutState(t, repository.repository)
			workspaceRoot := filepath.Join(root, "workspaces")
			graph := buildGraph(t, workspaceRoot, graphOptions{})
			_, err := gitworkspace.NewManager(context.Background(), repository.repository, workspaceRoot, repository.revision, graph)
			if !errors.Is(err, gitworkspace.ErrExecutableGitConfiguration) || !errors.Is(err, gitworkspace.ErrInvalidConfiguration) {
				t.Fatalf("err=%v", err)
			}
			assertNoSentinel(t, sentinel)
			if _, err := os.Lstat(graph.Children[0].Envelope.Workspace); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("worktree created: %v", err)
			}
			if after := sharedCheckoutState(t, repository.repository); after != before {
				t.Fatalf("shared checkout changed\nbefore=%s\nafter=%s", before, after)
			}
		})
	}
}

func TestManagerRejectsFilterConfigurationIntroducedAfterPreparation(t *testing.T) {
	t.Run("repository config before staging a child result", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{})
		fixture.prepare(t)
		graph := fixture.succeed(t, "a", "b")
		writeFile(t, filepath.Join(fixture.childFrom(graph, "a").Envelope.Workspace, "a.txt"), "a\n")
		writeFile(t, filepath.Join(fixture.childFrom(graph, "b").Envelope.Workspace, "b.txt"), "b\n")
		sharedBefore := sharedCheckoutState(t, fixture.repository)
		sentinel := filepath.Join(filepath.Dir(fixture.root), "sentinel")
		commonDirectory := git(t, fixture.repository, "rev-parse", "--path-format=absolute", "--git-common-dir")
		writeFile(t, filepath.Join(commonDirectory, "info", "attributes"), "*.txt filter=sentinel\n")
		// A child can reach the shared common configuration from its worktree.
		git(t, fixture.childFrom(graph, "a").Envelope.Workspace, "config", "filter.sentinel.clean", filterCommand(sentinel))

		if _, _, _, err := fixture.manager.ObserveIntegration(context.Background(), nil); !errors.Is(err, gitworkspace.ErrExecutableGitConfiguration) {
			t.Fatalf("observe err=%v", err)
		}
		if _, _, err := fixture.manager.InspectChild(context.Background(), fixture.childFrom(graph, "a")); !errors.Is(err, gitworkspace.ErrExecutableGitConfiguration) {
			t.Fatalf("inspect err=%v", err)
		}
		assertNoSentinel(t, sentinel)
		if after := sharedCheckoutState(t, fixture.repository); after != sharedBefore {
			t.Fatalf("shared checkout changed\nbefore=%s\nafter=%s", sharedBefore, after)
		}
	})

	t.Run("worktree-specific process driver", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{})
		fixture.prepare(t)
		sharedBefore := sharedCheckoutState(t, fixture.repository)
		sentinel := filepath.Join(filepath.Dir(fixture.root), "sentinel")
		workspace := fixture.child("a").Envelope.Workspace
		git(t, workspace, "config", "extensions.worktreeConfig", "true")
		git(t, workspace, "config", "--worktree", "filter.sentinel.process", filterCommand(sentinel))
		if err := fixture.manager.ValidateWorkspace(context.Background(), fixture.child("a")); !errors.Is(err, gitworkspace.ErrExecutableGitConfiguration) {
			t.Fatalf("err=%v", err)
		}
		assertNoSentinel(t, sentinel)
		if after := sharedCheckoutState(t, fixture.repository); after != sharedBefore {
			t.Fatalf("shared checkout changed\nbefore=%s\nafter=%s", sharedBefore, after)
		}
	})

	t.Run("external diff driver", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{})
		fixture.prepare(t)
		git(t, fixture.repository, "config", "diff.external", "/usr/bin/true")
		if err := fixture.manager.ValidateWorkspace(context.Background(), fixture.child("a")); !errors.Is(err, gitworkspace.ErrExecutableGitConfiguration) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("disabled driver value is not executable", func(t *testing.T) {
		fixture := newGitFixture(t, graphOptions{})
		fixture.prepare(t)
		git(t, fixture.repository, "config", "filter.lfs.smudge", "")
		if err := fixture.manager.ValidateWorkspace(context.Background(), fixture.child("a")); err != nil {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestManagerRequiresPrivateWorkspaceRootAndControlPaths(t *testing.T) {
	newManager := func(t *testing.T, prepare func(root, workspaceRoot string)) error {
		t.Helper()
		root := canonicalTempDir(t)
		repository := initRepository(t, filepath.Join(root, "repository"))
		workspaceRoot := filepath.Join(root, "workspaces")
		prepare(root, workspaceRoot)
		_, err := gitworkspace.NewManager(context.Background(), repository.repository, workspaceRoot, repository.revision, buildGraph(t, workspaceRoot, graphOptions{}))
		return err
	}
	mkdir := func(t *testing.T, name string, mode os.FileMode) {
		t.Helper()
		if err := os.Mkdir(name, mode); err != nil {
			t.Fatal(err)
		}
		if err := testfs.SharedMode(name, mode); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("safe preexisting root", func(t *testing.T) {
		if err := newManager(t, func(_, workspaceRoot string) { mkdir(t, workspaceRoot, 0o700) }); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	for _, mode := range []os.FileMode{0o755, 0o770, 0o701} {
		t.Run("permissive root "+mode.String(), func(t *testing.T) {
			if err := newManager(t, func(_, workspaceRoot string) { mkdir(t, workspaceRoot, mode) }); !errors.Is(err, gitworkspace.ErrInvalidConfiguration) {
				t.Fatalf("err=%v", err)
			}
		})
	}

	t.Run("permissive owner directory", func(t *testing.T) {
		err := newManager(t, func(_, workspaceRoot string) {
			mkdir(t, workspaceRoot, 0o700)
			mkdir(t, filepath.Join(workspaceRoot, ".axiom-workspace-owners"), 0o755)
		})
		if !errors.Is(err, gitworkspace.ErrInvalidConfiguration) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("foreign ownership", func(t *testing.T) {
		gitworkspace.SetEffectiveUID(t, os.Geteuid()+1)
		if err := newManager(t, func(_, workspaceRoot string) { mkdir(t, workspaceRoot, 0o700) }); !errors.Is(err, gitworkspace.ErrInvalidConfiguration) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("symlink root", func(t *testing.T) {
		err := newManager(t, func(root, workspaceRoot string) {
			target := filepath.Join(root, "target")
			mkdir(t, target, 0o700)
			if err := testfs.Symlink(t, target, workspaceRoot); err != nil {
				t.Fatal(err)
			}
		})
		if !errors.Is(err, gitworkspace.ErrInvalidConfiguration) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("symlink owner directory", func(t *testing.T) {
		err := newManager(t, func(root, workspaceRoot string) {
			mkdir(t, workspaceRoot, 0o700)
			target := filepath.Join(root, "owners-target")
			mkdir(t, target, 0o700)
			if err := testfs.Symlink(t, target, filepath.Join(workspaceRoot, ".axiom-workspace-owners")); err != nil {
				t.Fatal(err)
			}
		})
		if !errors.Is(err, gitworkspace.ErrInvalidConfiguration) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestManagerRejectsControlPathsWeakenedAfterPreparation(t *testing.T) {
	ownerRecord := func(fixture *gitFixture) string {
		return filepath.Join(fixture.root, ".axiom-workspace-owners", fixture.child("a").ExecutionID+".json")
	}
	tests := []struct {
		name   string
		weaken func(t *testing.T, fixture *gitFixture)
	}{
		{"permissive root", func(t *testing.T, fixture *gitFixture) { chmod(t, fixture.root, 0o755) }},
		{"permissive owner directory", func(t *testing.T, fixture *gitFixture) {
			chmod(t, filepath.Join(fixture.root, ".axiom-workspace-owners"), 0o750)
		}},
		{"permissive owner record", func(t *testing.T, fixture *gitFixture) { chmod(t, ownerRecord(fixture), 0o644) }},
		{"hard-linked owner record", func(t *testing.T, fixture *gitFixture) {
			if err := os.Link(ownerRecord(fixture), filepath.Join(fixture.root, "owner-alias")); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlinked owner record", func(t *testing.T, fixture *gitFixture) {
			record := ownerRecord(fixture)
			moved := filepath.Join(fixture.root, "moved-owner.json")
			if err := testfs.RenameOrSkipPinned(t, record, moved); err != nil {
				t.Fatal(err)
			}
			if err := testfs.Symlink(t, moved, record); err != nil {
				t.Fatal(err)
			}
		}},
		{"foreign ownership", func(t *testing.T, _ *gitFixture) { gitworkspace.SetEffectiveUID(t, os.Geteuid()+1) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGitFixture(t, graphOptions{})
			fixture.prepare(t)
			if err := fixture.manager.ValidateWorkspace(context.Background(), fixture.child("a")); err != nil {
				t.Fatalf("safe control paths rejected: %v", err)
			}
			test.weaken(t, fixture)
			if err := fixture.manager.ValidateWorkspace(context.Background(), fixture.child("a")); !errors.Is(err, gitworkspace.ErrWorkspaceInvalid) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func chmod(t *testing.T, name string, mode os.FileMode) {
	t.Helper()
	if err := testfs.SharedMode(name, mode); err != nil {
		t.Fatal(err)
	}
}
