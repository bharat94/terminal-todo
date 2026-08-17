// Command terminal-todo is the module-root entry point.
//
// It exists so that `go install github.com/bharat94/terminal-todo@latest`
// keeps working for anyone already using it. That path installs the binary
// under the module name, terminal-todo, because Go derives the name from the
// import path. Most users want it called todo, which is what cmd/todo
// provides.
package main

import "github.com/bharat94/terminal-todo/internal/cli"

func main() { cli.Execute() }
