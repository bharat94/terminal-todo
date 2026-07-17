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
		fail(ErrStoreCorrupted, "checking existing store: %v", err)
	}
	if err := os.MkdirAll(ttDir, 0700); err != nil {
		fail(ErrStoreCorrupted, "creating directory: %v", err)
	}

	s := store.NewTaskStore()
	if err := s.Save(storePath); err != nil {
		fail(ErrStoreCorrupted, "creating tasks.bin: %v", err)
	}

	fmt.Println("Initialized .terminal-todo/ in", projectRoot)
}
