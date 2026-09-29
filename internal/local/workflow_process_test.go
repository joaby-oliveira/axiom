package local

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/rgomids/axiom/internal/workflow"
)

func TestExecutionStoreTwoProcessBarrierHasOneWinningRevision(t *testing.T) {
	root := privateTestRoot(t)
	store, _ := NewWorkflowStore(root)
	state := validExecution()
	if err := store.Create(context.Background(), state); err != nil {
		t.Fatal(err)
	}

	first := startExecutionProcess(t, root, "advance")
	second := startExecutionProcess(t, root, "interrupt")
	first.release(t)
	second.release(t)
	results := []string{first.wait(t), second.wait(t)}
	if !containsProcessResult(results, "saved") || !containsProcessResult(results, "conflict") {
		t.Fatalf("process results = %v", results)
	}
	loaded, err := store.Load(context.Background(), state.ProjectID, state.RepositoryKey, state.WorkItem)
	if err != nil || loaded.Revision != 2 || len(loaded.Transitions) != 1 {
		t.Fatalf("authoritative state = %#v, %v", loaded, err)
	}
}

func TestExecutionStoreProcessCrashBoundariesFailClosed(t *testing.T) {
	for _, point := range []string{"stage", "committed"} {
		t.Run(point, func(t *testing.T) {
			root := privateTestRoot(t)
			store, _ := NewWorkflowStore(root)
			state := validExecution()
			if err := store.Create(context.Background(), state); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(os.Args[0], "-test.run=^TestExecutionProcessHelper$")
			command.Env = append(os.Environ(), "AXIOM_EXECUTION_HELPER="+point, "AXIOM_EXECUTION_ROOT="+root)
			input, _ := command.StdinPipe()
			defer input.Close()
			output, _ := command.StdoutPipe()
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			line, err := bufio.NewReader(output).ReadString('\n')
			if err != nil || strings.TrimSpace(line) != point {
				t.Fatalf("barrier = %q, %v", line, err)
			}
			if err := command.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = command.Wait()
			if _, err := store.Load(context.Background(), state.ProjectID, state.RepositoryKey, state.WorkItem); !errors.Is(err, workflow.ErrRecoveryRequired) {
				t.Fatalf("crash accepted = %v", err)
			}
		})
	}
}

type executionProcess struct {
	command *exec.Cmd
	input   io.WriteCloser
	output  *bufio.Reader
}

func startExecutionProcess(t *testing.T, root, action string) executionProcess {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestExecutionProcessHelper$")
	command.Env = append(os.Environ(), "AXIOM_EXECUTION_HELPER=race", "AXIOM_EXECUTION_ACTION="+action, "AXIOM_EXECUTION_ROOT="+root)
	input, _ := command.StdinPipe()
	output, _ := command.StdoutPipe()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(output)
	line, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "ready" {
		t.Fatalf("process ready = %q, %v", line, err)
	}
	return executionProcess{command: command, input: input, output: reader}
}

func (p executionProcess) release(t *testing.T) {
	t.Helper()
	if _, err := p.input.Write([]byte{'x'}); err != nil {
		t.Fatal(err)
	}
	_ = p.input.Close()
}
func (p executionProcess) wait(t *testing.T) string {
	t.Helper()
	line, _ := p.output.ReadString('\n')
	if err := p.command.Wait(); err != nil {
		t.Fatalf("helper: %v", err)
	}
	return strings.TrimSpace(line)
}
func containsProcessResult(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestExecutionProcessHelper(t *testing.T) {
	point := os.Getenv("AXIOM_EXECUTION_HELPER")
	if point == "" {
		return
	}
	store, err := NewWorkflowStore(os.Getenv("AXIOM_EXECUTION_ROOT"))
	if err != nil {
		t.Fatal(err)
	}
	state := validExecution()
	loaded, err := store.Load(context.Background(), state.ProjectID, state.RepositoryKey, state.WorkItem)
	if err != nil {
		t.Fatal(err)
	}
	if point == "race" {
		fmt.Fprintln(os.Stdout, "ready")
		if _, err := io.ReadAll(io.LimitReader(os.Stdin, 1)); err != nil {
			t.Fatal(err)
		}
		if os.Getenv("AXIOM_EXECUTION_ACTION") == "advance" {
			loaded = advancedExecution(loaded)
		} else {
			loaded = interruptedExecution(loaded)
		}
		if err := store.Save(context.Background(), loaded); errors.Is(err, workflow.ErrConflict) {
			fmt.Fprintln(os.Stdout, "conflict")
			return
		} else if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(os.Stdout, "saved")
		return
	}
	loaded = advancedExecution(loaded)
	store.hooks.afterStage = func() {
		if point == "stage" {
			fmt.Fprintln(os.Stdout, point)
			_, _ = io.Copy(io.Discard, os.Stdin)
		}
	}
	store.hooks.afterCommit = func() {
		if point == "committed" {
			fmt.Fprintln(os.Stdout, point)
			_, _ = io.Copy(io.Discard, os.Stdin)
		}
	}
	if err := store.Save(context.Background(), loaded); err != nil {
		t.Fatal(err)
	}
}
