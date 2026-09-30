package install

import (
	"github.com/rgomids/axiom/internal/windowsfs"
	"golang.org/x/sys/windows"
)

const binaryName = "axiom.exe"

func supportedWindowsHost() bool {
	version := windows.RtlGetVersion()
	return version.MajorVersion >= 10 && version.BuildNumber >= 17763
}

func statfsAvailable(path string) (uint64, error) { return windowsfs.Available(path) }
