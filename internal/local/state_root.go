package local

import "path/filepath"

// NativeStateRoot resolves the approved platform directory without filesystem
// effects. XDG_STATE_HOME is used only when it is an absolute Linux path.
func NativeStateRoot(goos, home, xdgStateHome string) (string, error) {
	if !filepath.IsAbs(home) {
		return "", ErrUnsafe
	}
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Lingo"), nil
	case "linux":
		if filepath.IsAbs(xdgStateHome) {
			return filepath.Join(xdgStateHome, "lingo"), nil
		}
		return filepath.Join(home, ".local", "state", "lingo"), nil
	case "windows":
		return filepath.Join(home, "AppData", "Local", "Axiom", "state"), nil
	default:
		return "", ErrUnsafe
	}
}

// WindowsStateRoot honors redirected LocalAppData explicitly, without storing
// machine paths in portable Project manifests.
func WindowsStateRoot(home, localAppData string) (string, error) {
	if localAppData != "" {
		if !filepath.IsAbs(localAppData) {
			return "", ErrUnsafe
		}
		return filepath.Join(localAppData, "Axiom", "state"), nil
	}
	return NativeStateRoot("windows", home, "")
}
