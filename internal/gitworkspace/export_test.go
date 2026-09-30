package gitworkspace

import (
	"github.com/rgomids/axiom/internal/testfs"
	"testing"
)

// SetEffectiveUID makes every control path appear owned by another identity,
// which is otherwise only reproducible with privileges to chown.
func SetEffectiveUID(t *testing.T, uid int) {
	testfs.POSIXModes(t)
	t.Helper()
	previous := effectiveUID
	effectiveUID = func() int { return uid }
	t.Cleanup(func() { effectiveUID = previous })
}
