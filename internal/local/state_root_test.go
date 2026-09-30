package local

import (
	"github.com/rgomids/axiom/internal/testfs"
	"testing"
)

func TestNativeStateRoot(t *testing.T) {
	testfs.POSIXModes(t)
	tests := []struct {
		name, goos, home, xdg, want string
		invalid                     bool
	}{
		{"macOS", "darwin", "/users/test", "/xdg/ignored", "/users/test/Library/Application Support/Lingo", false},
		{"Linux XDG", "linux", "/home/test", "/state/custom", "/state/custom/lingo", false},
		{"Linux fallback", "linux", "/home/test", "", "/home/test/.local/state/lingo", false},
		{"Linux relative XDG", "linux", "/home/test", "relative/state", "/home/test/.local/state/lingo", false},
		{"Windows fallback", "windows", "/home/test", "", "/home/test/AppData/Local/Axiom/state", false},
		{"unsupported platform", "plan9", "/home/test", "", "", true},
		{"relative home", "linux", "relative/home", "", "", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NativeStateRoot(test.goos, test.home, test.xdg)
			if got != test.want || (err != nil) != test.invalid {
				t.Fatalf("NativeStateRoot(%q, %q, %q) = %q, %v", test.goos, test.home, test.xdg, got, err)
			}
		})
	}
}
