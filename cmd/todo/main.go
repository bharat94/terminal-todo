// Command todo is the coordination CLI.
//
// This is the supported install path:
//
//	go install github.com/bharat94/terminal-todo/cmd/todo@latest
//
// Go names an installed binary after the last element of its import path, so
// installing this package produces `todo` rather than `terminal-todo`.
package main

import "github.com/bharat94/terminal-todo/internal/cli"

func main() { cli.Execute() }
