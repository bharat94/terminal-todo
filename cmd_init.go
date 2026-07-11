package main

import (
	"fmt"
	"os"
	"path/filepath"

	"terminal-todo/store"
)

func cmdInit(args []string) {
	ttDir := filepath.Join(projectRoot, ".terminal-todo")
	storePath := filepath.Join(ttDir, "tasks.bin")
	if _, err := os.Stat(storePath); err == nil {
		fmt.Println("Already initialized .terminal-todo/ in", projectRoot)
		return
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error checking existing store: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(ttDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		os.Exit(1)
	}

	s := store.NewTaskStore()
	if err := s.Save(storePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating tasks.bin: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Initialized .terminal-todo/ in", projectRoot)
}
