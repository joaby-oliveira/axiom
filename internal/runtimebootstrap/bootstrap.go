// Package runtimebootstrap discovers supported local Runtimes by executable
// and converges Axiom's user-global integration for every Runtime found.
//
// A Runtime is present only when its executable resolves from the current
// process environment; a configuration directory alone never counts.
// Discovery only resolves executables and never runs them, and no Runtime,
// credential, secret or subscription is installed or changed.
package runtimebootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/rgomids/axiom/internal/codexruntime"
)

// LookPath resolves an executable name; production uses exec.LookPath.
type LookPath func(string) (string, error)

// Integration converges one Runtime's Axiom-owned user-global integration.
type Integration interface {
	Install(context.Context) codexruntime.Result
}

// Runtime is one supported Runtime. Integration is resolved only when the
// Runtime is present, so an absent Runtime's roots are never touched.
type Runtime struct {
	ID          string
	Executable  string
	Integration func() (Integration, error)
	// ConfigurationRoot is only checked for existence, as a diagnostic.
	ConfigurationRoot string
}

type State string

const (
	Absent            State = "absent"
	Configured        State = "configured"
	AlreadyConfigured State = "already_configured"
	Failed            State = "failed"
)

type RuntimeReport struct {
	ID         string
	Executable string
	Present    bool
	// ConfigurationWithoutExecutable marks a stale or foreign configuration
	// directory for an absent Runtime. It never makes the Runtime available.
	ConfigurationWithoutExecutable bool
	State                          State
	Category                       string
	Result                         *codexruntime.Result
}

type Report struct {
	Runtimes []RuntimeReport
}

// Detected counts Runtimes whose executable resolved.
func (r Report) Detected() int {
	count := 0
	for _, runtime := range r.Runtimes {
		if runtime.Present {
			count++
		}
	}
	return count
}

// Failed counts detected Runtimes whose integration did not converge.
func (r Report) Failed() int {
	count := 0
	for _, runtime := range r.Runtimes {
		if runtime.State == Failed {
			count++
		}
	}
	return count
}

// Run discovers every Runtime and converges each present one independently,
// in the given order. A failure is reported for that Runtime only; results
// already confirmed for another Runtime are kept and nothing is rolled back.
func Run(ctx context.Context, lookPath LookPath, runtimes []Runtime) Report {
	report := Report{Runtimes: make([]RuntimeReport, 0, len(runtimes))}
	for _, runtime := range runtimes {
		entry := RuntimeReport{ID: runtime.ID, Executable: runtime.Executable, State: Absent, Category: runtime.ID + "_absent"}
		if !present(lookPath, runtime.Executable) {
			entry.ConfigurationWithoutExecutable = directoryExists(runtime.ConfigurationRoot)
			report.Runtimes = append(report.Runtimes, entry)
			continue
		}
		entry.Present = true
		entry.State, entry.Category = Failed, runtime.ID+"_skill_root_unavailable"
		if err := ctx.Err(); err != nil {
			entry.Category = "cancelled"
			report.Runtimes = append(report.Runtimes, entry)
			continue
		}
		if runtime.Integration == nil {
			report.Runtimes = append(report.Runtimes, entry)
			continue
		}
		integration, err := runtime.Integration()
		if err != nil || integration == nil {
			report.Runtimes = append(report.Runtimes, entry)
			continue
		}
		result := integration.Install(ctx)
		entry.Result, entry.Category = &result, result.Category
		switch result.Status {
		case codexruntime.Applied:
			entry.State = Configured
		case codexruntime.Unchanged:
			entry.State = AlreadyConfigured
		}
		report.Runtimes = append(report.Runtimes, entry)
	}
	return report
}

// present reports whether name resolves to an executable. Any resolution
// error, including a match found only through a relative PATH entry, is
// absence.
func present(lookPath LookPath, name string) bool {
	if lookPath == nil || name == "" {
		return false
	}
	path, err := lookPath(name)
	return err == nil && filepath.IsAbs(path)
}

func directoryExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

// ErrUnsafeConfigurationRoot rejects a Claude configuration directory that is
// not an absolute single-line path.
var ErrUnsafeConfigurationRoot = errors.New("unsafe Claude configuration directory")

// ClaudeConfigurationRoot returns Claude Code's effective configuration
// directory: CLAUDE_CONFIG_DIR when set (documented by Claude Code as the
// override of the default), otherwise ~/.claude.
func ClaudeConfigurationRoot(getenv func(string) string, home string) (string, error) {
	if override := getenv("CLAUDE_CONFIG_DIR"); override != "" {
		return safeRoot(override)
	}
	if home == "" {
		return "", ErrUnsafeConfigurationRoot
	}
	return safeRoot(filepath.Join(home, ".claude"))
}

// ClaudeSkillsRoot is the Claude user-global skill root,
// <configuration directory>/skills, holding skills/<name>/SKILL.md.
func ClaudeSkillsRoot(getenv func(string) string, home string) (string, error) {
	root, err := ClaudeConfigurationRoot(getenv, home)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "skills"), nil
}

func safeRoot(path string) (string, error) {
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "\n\r") || filepath.Clean(path) == string(filepath.Separator) {
		return "", ErrUnsafeConfigurationRoot
	}
	return filepath.Clean(path), nil
}
