#!/usr/bin/env bash
# Runs a command with the VS Code snap's GTK environment undone.
# The snap terminal points GTK/GIO at its own bundled libraries and schemas,
# which crashes native GTK apps (e.g. "Settings schema ... does not contain a key").
# VS Code saves each original value in <NAME>_VSCODE_SNAP_ORIG; restore those.
while IFS='=' read -r name _; do
  var="${name%_VSCODE_SNAP_ORIG}"
  orig="${!name}"
  if [ -n "$orig" ]; then
    export "$var=$orig"
  else
    unset "$var"
  fi
  unset "$name"
done < <(env | grep -E '^[A-Z_]+_VSCODE_SNAP_ORIG=')
exec "$@"
