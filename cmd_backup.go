package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"terminal-todo/store"
)

func cmdBackup(args []string) {
	output := optionValue(args, "--output")
	if output == "" {
		output = filepath.Join(projectRoot, ".terminal-todo", fmt.Sprintf("backup-%d.bin", time.Now().UnixMilli()))
	}

	s := loadStore()

	dir := filepath.Dir(output)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fail(ErrStoreCorrupted, "creating backup directory: %v", err)
	}

	if err := s.Save(output); err != nil {
		fail(ErrStoreCorrupted, "saving backup: %v", err)
	}

	fmt.Printf("Backup saved to %s (%d tasks)\n", output, len(s.Tasks))
}

func cmdRestore(args []string) {
	if len(args) == 0 || args[0] == "" || args[0][0] == '-' {
		fail(ErrInvalidArgs, "usage: todo restore <backup-file>")
	}

	backupPath := args[0]

	s, err := store.Load(backupPath)
	if err != nil {
		fail(ErrStoreCorrupted, "loading backup: %v", err)
	}

	updateStore(func(existing *store.TaskStore) error {
		existing.Tasks = s.Tasks
		existing.NextID = s.NextID
		return nil
	})

	fmt.Printf("Restored %d tasks from %s\n", len(s.Tasks), backupPath)
}
