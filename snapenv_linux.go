package main

import (
	"os"
	"strings"
	"syscall"
)

const snapOrigSuffix = "_VSCODE_SNAP_ORIG"

// fixSnapEnv re-executes the app without the GTK environment injected by the
// VS Code snap terminal. That terminal points GTK/GIO at the snap's bundled
// schemas and modules, which crashes native GTK apps on startup. VS Code keeps
// each original value in <NAME>_VSCODE_SNAP_ORIG, so restore those.
func fixSnapEnv() {
	env := os.Environ()
	restore := map[string]string{}
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		if strings.HasSuffix(name, snapOrigSuffix) {
			restore[strings.TrimSuffix(name, snapOrigSuffix)] = value
		}
	}
	if len(restore) == 0 {
		return
	}

	clean := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if _, ok := restore[name]; ok || strings.HasSuffix(name, snapOrigSuffix) {
			continue
		}
		clean = append(clean, kv)
	}
	for name, value := range restore {
		if value != "" {
			clean = append(clean, name+"="+value)
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return
	}
	// On success Exec does not return; on failure keep going with the current env.
	_ = syscall.Exec(exe, os.Args, clean)
}
