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
	if err := os.MkdirAll(dir, 0700); err != nil {
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

	taskCount, err := restoreBackup(backupPath)
	if err != nil {
		fail(ErrStoreCorrupted, "restoring backup: %v", err)
	}

	fmt.Printf("Restored %d tasks from %s\n", taskCount, backupPath)
}

func restoreBackup(backupPath string) (int, error) {
	snapshot, err := store.Load(backupPath)
	if err != nil {
		return 0, fmt.Errorf("loading backup: %w", err)
	}
	if _, err := store.Update(tasksBinPath(), func(existing *store.TaskStore) error {
		*existing = *snapshot
		return nil
	}); err != nil {
		return 0, err
	}
	return len(snapshot.Tasks), nil
}
