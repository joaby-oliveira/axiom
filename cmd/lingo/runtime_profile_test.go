package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rgomids/axiom/internal/cli"
	"github.com/rgomids/axiom/internal/local"
	"github.com/rgomids/axiom/internal/runtimeprofile"
)

func TestRuntimeProfileValidateConfiguration(t *testing.T) {
	for _, scenario := range []string{"valid", "missing-root", "missing-configuration", "malformed", "invalid", "unsafe-root"} {
		for _, mode := range []string{"--json", "--human"} {
			t.Run(scenario+mode, func(t *testing.T) {
				base := t.TempDir()
				state := filepath.Join(base, "state")
				t.Setenv("LINGO_STATE_ROOT", state)
				t.Setenv("LINGO_PROJECTS_ROOT", filepath.Join(base, "projects"))
				t.Setenv("AXIOM_CODEX_SKILLS_ROOT", filepath.Join(base, "skills"))
				// No executable can be found; validation must only inspect local state.
				t.Setenv("PATH", filepath.Join(base, "no-executables"))
				if scenario == "unsafe-root" {
					if err := os.WriteFile(state, []byte("private-state-marker"), 0600); err != nil {
						t.Fatal(err)
					}
				} else if scenario != "missing-root" {
					store, err := local.NewRuntimeProfileStore(state)
					if err != nil {
						t.Fatal(err)
					}
					cfg := runtimeprofile.Configuration{
						FormatVersion: runtimeprofile.FormatVersion, Revision: 1,
						Runtimes:      []runtimeprofile.Runtime{{ID: "codex", Adapter: "codex", Enabled: true, AllowlistedProfileIDs: []string{"default"}, CredentialReference: "private-credential-reference"}},
						ModelProfiles: []runtimeprofile.ModelProfile{{ID: "default", RuntimeID: "codex", Model: "private-model-name", Capabilities: []string{"code"}}},
					}
					if err := store.Create(context.Background(), cfg); err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(state, "runtime-profiles", "v1", "configuration.json")
					switch scenario {
					case "missing-configuration":
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					case "malformed":
						if err := os.WriteFile(path, []byte("private-invalid-content"), 0600); err != nil {
							t.Fatal(err)
						}
					case "invalid":
						cfg.ModelProfiles[0].RuntimeID = "private-unknown-runtime"
						wire, err := json.Marshal(cfg)
						if err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(path, wire, 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
				before := runtimeProfileSnapshot(t, base)
				var output, prompts bytes.Buffer
				code := cli.RunInteractive(context.Background(), []string{mode, "runtime", "profile", "validate"}, compose(), currentProvenance(), nil, &output, &prompts)
				wantCode, wantStatus, wantMessage := cli.ExitFailure, "validation_failure", "Runtime profile configuration is missing or invalid"
				if scenario == "valid" {
					wantCode, wantStatus, wantMessage = cli.ExitSuccess, "success", "Runtime profile configuration is valid"
				}
				if code != wantCode || prompts.Len() != 0 {
					t.Fatalf("code=%d output=%s prompts=%s", code, &output, &prompts)
				}
				if mode == "--json" {
					var event canonicalEvent
					if err := json.Unmarshal(output.Bytes(), &event); err != nil || event.Status != wantStatus || event.Result != wantMessage {
						t.Fatalf("event=%+v err=%v output=%s", event, err, &output)
					}
				} else if !strings.Contains(output.String(), "status: "+wantStatus) || !strings.Contains(output.String(), "result: "+wantMessage) {
					t.Fatalf("output=%s", &output)
				}
				for _, hidden := range []string{base, "private-"} {
					if strings.Contains(output.String(), hidden) {
						t.Fatalf("output leaked %q: %s", hidden, &output)
					}
				}
				if after := runtimeProfileSnapshot(t, base); !reflect.DeepEqual(before, after) {
					t.Fatalf("validation changed local files: before=%v after=%v", before, after)
				}
			})
		}
	}
}

func runtimeProfileSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := info.Mode().String() + info.ModTime().String()
		if !entry.IsDir() {
			wire, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += string(wire)
		}
		snapshot[path] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
