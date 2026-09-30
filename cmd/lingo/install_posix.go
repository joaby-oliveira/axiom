//go:build !windows

package main

import "io"

func installReleaseCommand([]string, io.Writer, io.Writer) (bool, int) { return false, 0 }
