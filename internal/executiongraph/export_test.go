package executiongraph

import "testing"

// Test-only exports for the external acceptance journey tests, which import
// the coordination package that owns the canonical record verifier.

var RollupGraph = rollupGraph

func MustGraph(t *testing.T) Graph { return mustGraph(t) }

func DigestOf(value string) string { return digestOf(value) }
