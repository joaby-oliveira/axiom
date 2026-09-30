package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/rgomids/axiom/internal/compatibility"
	"github.com/rgomids/axiom/internal/install"
)

func installReleaseCommand(args []string, out, stderr io.Writer) (bool, int) {
	if len(args) == 0 || args[0] != "install-release" {
		return false, 0
	}
	fail := func(err error) (bool, int) { fmt.Fprintf(stderr, "install_error: %v\n", err); return true, 1 }
	values := map[string]string{}
	for i := 1; i < len(args); i += 2 {
		key := args[i]
		if i+1 >= len(args) || values[key] != "" || (key != "--archive" && key != "--checksums" && key != "--bin-dir" && key != "--receipt-dir") {
			return fail(fmt.Errorf("invalid or duplicate installation argument"))
		}
		value := args[i+1]
		if !filepath.IsAbs(value) || strings.ContainsAny(value, "\r\n=") {
			return fail(fmt.Errorf("absolute installation paths required"))
		}
		values[key] = filepath.Clean(value)
	}
	if len(values) != 4 {
		return fail(fmt.Errorf("archive, checksums, bin-dir and receipt-dir are required"))
	}
	candidate, err := install.LoadCandidate(values["--archive"], values["--checksums"])
	if err != nil {
		return fail(err)
	}
	projects, err := projectsRoot()
	if err != nil {
		return fail(err)
	}
	state, err := stateRoot()
	if err != nil {
		return fail(err)
	}
	skills := codexSkillsRoot()
	target := install.Target{BinaryDir: values["--bin-dir"], ReceiptDir: values["--receipt-dir"], SkillsRoot: skills, State: compatibility.Roots{Projects: projects, State: state, Skills: skills}}
	status, err := install.InstallRelease(context.Background(), target, candidate)
	if status != "" {
		fmt.Fprintf(out, "install_status=%s\n", status)
	}
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(out, "installed_binary=%s\npath_notice: add %s to your PATH, then run axiom first-run\n", filepath.Join(target.BinaryDir, "axiom.exe"), target.BinaryDir)
	return true, 0
}
