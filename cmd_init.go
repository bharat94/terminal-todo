package main

import (
	"fmt"
	"os"
	"path/filepath"

	"terminal-todo/store"
)

func cmdInit(args []string) {
	ttDir := filepath.Join(projectRoot, ".terminal-todo")
	if err := os.MkdirAll(ttDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		os.Exit(1)
	}

	s := store.NewTaskStore()
	if err := s.Save(filepath.Join(ttDir, "tasks.bin")); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating tasks.bin: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Initialized .terminal-todo/ in", projectRoot)
}
