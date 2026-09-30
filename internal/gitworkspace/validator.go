package gitworkspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rgomids/axiom/internal/executiongraph"
)

type ValidationCommand struct {
	Reference string
	Argv      []string
	Env       []string
	OutputMax int
}

type CombinedValidator struct {
	workspaces *Manager
	commands   []ValidationCommand
}

func NewCombinedValidator(workspaces *Manager, commands []ValidationCommand) (*CombinedValidator, error) {
	if workspaces == nil || len(commands) == 0 || len(commands) > 64 {
		return nil, ErrInvalidConfiguration
	}
	result := &CombinedValidator{workspaces: workspaces, commands: make([]ValidationCommand, 0, len(commands))}
	seen := map[string]bool{}
	for _, command := range commands {
		if !validValidationCommand(command) || seen[command.Reference] {
			return nil, ErrInvalidConfiguration
		}
		seen[command.Reference] = true
		command.Argv = append([]string(nil), command.Argv...)
		command.Env = append([]string(nil), command.Env...)
		result.commands = append(result.commands, command)
	}
	return result, nil
}

func (v *CombinedValidator) ValidateCombined(ctx context.Context, integration executiongraph.ChildExecution, resultTree string) ([]executiongraph.ValidationResult, error) {
	if err := v.workspaces.ValidateResultTree(ctx, integration, resultTree); err != nil {
		return nil, err
	}
	results := make([]executiongraph.ValidationResult, 0, len(v.commands))
	for _, validation := range v.commands {
		command := exec.CommandContext(ctx, validation.Argv[0], validation.Argv[1:]...)
		command.Dir = integration.Envelope.Workspace
		command.Env = append([]string(nil), validation.Env...)
		output := &boundedBuffer{remaining: validation.OutputMax}
		command.Stdout, command.Stderr = output, output
		err := command.Run()
		exitCode := 0
		if err != nil {
			exitCode = -1
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				exitCode = exitError.ExitCode()
			}
		}
		digest := sha256.Sum256(output.Bytes())
		results = append(results, executiongraph.ValidationResult{CommandReference: validation.Reference, ExitCode: exitCode, OutputDigest: hex.EncodeToString(digest[:])})
		if err != nil {
			return results, err
		}
		if err := v.workspaces.ValidateResultTree(ctx, integration, resultTree); err != nil {
			return results, err
		}
	}
	return results, nil
}

func validValidationCommand(command ValidationCommand) bool {
	if command.Reference == "" || len(command.Reference) > 512 || strings.ContainsAny(command.Reference, "\x00\r\n") || len(command.Argv) == 0 || len(command.Argv) > 64 || !filepath.IsAbs(command.Argv[0]) || command.OutputMax <= 0 || command.OutputMax > executiongraph.MaxCapturedOutputBytes || len(command.Env) > 64 {
		return false
	}
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(command.Argv[0])), ".exe")
	if base == "sh" || base == "bash" || base == "zsh" || base == "fish" || base == "cmd" || base == "powershell" || base == "pwsh" {
		return false
	}
	for _, argument := range command.Argv {
		if argument == "" || strings.ContainsRune(argument, '\x00') {
			return false
		}
	}
	seen := map[string]bool{}
	for _, item := range command.Env {
		key, _, ok := strings.Cut(item, "=")
		if !ok || !validEnvironmentKey(key) || seen[key] || strings.ContainsRune(item, '\x00') {
			return false
		}
		seen[key] = true
	}
	return true
}

func validEnvironmentKey(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, char := range value {
		if !((char >= 'A' && char <= 'Z') || char == '_' || (index > 0 && char >= '0' && char <= '9')) {
			return false
		}
	}
	return true
}

type boundedBuffer struct {
	bytes.Buffer
	remaining int
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	if len(value) > b.remaining {
		value = value[:b.remaining]
	}
	if len(value) > 0 {
		_, _ = b.Buffer.Write(value)
		b.remaining -= len(value)
	}
	return original, nil
}
