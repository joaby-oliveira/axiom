package main

import "runtime"

func testExecutableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func testRuntimeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".cmd"
	}
	return name
}

func testRuntimeScript(marker string) []byte {
	if runtime.GOOS == "windows" {
		return []byte("@echo called>\"" + marker + "\"\r\n")
	}
	return []byte("#!/bin/sh\ntouch '" + marker + "'\n")
}
