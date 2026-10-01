//go:build ignore

// This standalone verification tool inspects application sources outside application tests.
// Run from the repository root: go run ./scripts/check-projectapp.go
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Restrict qualified standard-library symbols to the pure operations consumed.
// Any expansion requires an explicit review of its I/O behavior.
var allowed = map[string]string{
	"context":         "Context Background WithCancel Canceled",
	"crypto/sha256":   "Sum256",
	"encoding/binary": "BigEndian",
	"encoding/hex":    "EncodeToString",
	"encoding/json":   "Marshal",
	"errors":          "Is New",
	"sort":            "Slice",
	"strings":         "ContainsAny HasPrefix Split Contains",
	"sync":            "Mutex",
	"sync/atomic":     "Bool",
	"path/filepath":   "Clean IsAbs",
	"time":            "Time Unix",
	"reflect":         "DeepEqual",
	"testing":         "T",
	"unicode/utf8":    "ValidString",
	"github.com/rgomids/axiom/internal/project":  "Project State Issue New Configured BusinessContext Declaration Repository Provider Runtime Integration ModelProfile CredentialReference Intent Set ValidSlug Absent NotConfigured Unconfigured",
	"github.com/rgomids/axiom/internal/manifest": "Codec",
}

func main() {
	entries, err := os.ReadDir("internal/projectapp")
	if err != nil {
		fail("cannot inspect application directory")
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			fail("unexpected application file type")
		}
		check(filepath.Join("internal/projectapp", entry.Name()))
		count++
	}
	if count == 0 {
		fail("no application sources checked")
	}
	fmt.Printf("PASS: %d application source/test files; pure import/symbol allowlist; no test I/O helpers or compiler directives\n", count)
}

func check(path string) {
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		fail("cannot parse application source")
	}
	imports := map[string]string{}
	for _, imp := range f.Imports {
		name, _ := strconv.Unquote(imp.Path.Value)
		if imp.Name != nil {
			fail("application import aliases require review")
		}
		if name == "github.com/rgomids/axiom/internal/projectapp" && strings.HasSuffix(path, "_test.go") {
			continue
		}
		if _, ok := allowed[name]; !ok {
			fail("non-allowlisted application import: " + name)
		}
		if (name == "testing" || name == "reflect" || name == "sync" || name == "github.com/rgomids/axiom/internal/manifest") && !strings.HasSuffix(path, "_test.go") {
			fail("test-only helper imported by production application")
		}
		imports[filepath.Base(name)] = name
	}
	for _, group := range f.Comments {
		for _, comment := range group.List {
			if strings.Contains(comment.Text, "go:") {
				fail("application compiler directive requires review")
			}
		}
	}
	ast.Inspect(f, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		for _, forbidden := range []string{"TempDir", "Setenv", "Chdir", "Output", "Attr"} {
			if selector.Sel.Name == forbidden {
				fail("application test I/O helper forbidden")
			}
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		name, imported := imports[identifier.Name]
		if imported && !strings.Contains(" "+allowed[name]+" ", " "+selector.Sel.Name+" ") {
			fail("non-allowlisted application symbol: " + name + "." + selector.Sel.Name)
		}
		return true
	})
}

func fail(message string) { fmt.Fprintln(os.Stderr, "FAIL: "+message); os.Exit(1) }
