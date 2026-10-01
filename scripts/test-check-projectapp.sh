#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd -P)"
TEST_ROOT="$(mktemp -d)"
trap 'rm -rf "$TEST_ROOT"' EXIT
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
go build -o "$TEST_ROOT/check-app" "$ROOT/scripts/check-projectapp.go"
mkdir -p "$TEST_ROOT/internal/projectapp"

check_case() {
  local expected="$1"
  local source="$2"
  printf '%s\n' "$source" > "$TEST_ROOT/internal/projectapp/fixture_test.go"
  if (cd "$TEST_ROOT" && ./check-app > result.txt 2>&1); then
    [[ "$expected" == "pass" ]] || { printf 'FAIL: forbidden fixture accepted\n' >&2; exit 1; }
    return
  fi
  [[ "$expected" == "fail" ]] || { printf 'FAIL: pure fixture rejected\n' >&2; exit 1; }
}

check_case pass 'package projectapp_test; import "github.com/rgomids/axiom/internal/project"; var p project.Project'
check_case pass 'package projectapp_test; import "unicode/utf8"; func pure() { _ = utf8.ValidString("context/api�legacy.md") }'
check_case fail 'package projectapp_test; import "unicode/utf8"; func unreviewed() { _ = utf8.RuneCountInString("text") }'
check_case fail 'package projectapp_test; import "os"; func impure() { _ = os.Getenv("SYNTHETIC") }'
check_case fail 'package projectapp_test; import "net/http"; func impure() { _, _ = http.Get("https://example.invalid") }'
check_case fail 'package projectapp_test; import "os/exec"; func impure() { _ = exec.Command("synthetic") }'
check_case fail 'package projectapp_test; import "os"; func impure() { _ = os.WriteFile("synthetic", nil, 0600) }'
check_case fail 'package projectapp_test; import "testing"; func TestImpure(t *testing.T) { _ = t.TempDir() }'
check_case fail 'package projectapp_test; import "time"; func impure() { _ = time.Now() }'
check_case fail 'package projectapp_test; import "path/filepath"; func impure() { _, _ = filepath.Abs("synthetic") }'
check_case fail 'package projectapp_test; import "encoding/json"; func unreviewed() { _ = json.Valid(nil) }'
check_case fail 'package projectapp_test; import "reflect"; func impure() { _ = reflect.ValueOf(1) }'
check_case fail 'package projectapp_test; import alias "strings"; func pure() { _ = alias.TrimSpace("text") }'
check_case fail 'package projectapp_test; import "github.com/rgomids/axiom/internal/local"; var _ = local.Store'
check_case fail 'package projectapp_test; import "unsafe"; var _ unsafe.Pointer'
check_case fail 'package projectapp_test; //go:linkname escape runtime.escape'

printf 'PASS: application checker accepts inward domain dependency and pure UTF-8 validation; rejects fourteen unreviewed symbol, ambient I/O, outward dependency and bypass fixtures without executing them\n'
