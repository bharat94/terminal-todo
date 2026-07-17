package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bharat94/terminal-todo/dag"

	"github.com/bharat94/terminal-todo/store"
)

func cmdDoctor(args []string) {
	ttDir := filepath.Join(projectRoot, ".terminal-todo")
	fix := hasFlag(args, "--fix")

	fmt.Printf("Running diagnostics on %s\n\n", ttDir)

	checkFile := func(name, path string, mustExist bool) string {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			if mustExist {
				return "MISSING"
			}
			return "absent (ok)"
		}
		if err != nil {
			return fmt.Sprintf("ERROR: %v", err)
		}
		if info.Size() == 0 {
			return "empty"
		}
		return fmt.Sprintf("ok (%s)", formatSize(info.Size()))
	}

	// Check essential files
	tasksBin := filepath.Join(ttDir, "tasks.bin")
	reposJSON := filepath.Join(ttDir, "repositories.json")
	configJSON := filepath.Join(ttDir, "config.json")
	agentsJSON := filepath.Join(ttDir, "agents.json")

	fmt.Printf("  tasks.bin         %s\n", checkFile("tasks.bin", tasksBin, true))
	fmt.Printf("  repositories.json %s\n", checkFile("repos.json", reposJSON, false))
	fmt.Printf("  config.json       %s\n", checkFile("config.json", configJSON, false))
	fmt.Printf("  agents.json       %s\n", checkFile("agents.json", agentsJSON, false))

	entries, err := os.ReadDir(ttDir)
	if err != nil {
		fmt.Printf("  ERROR reading .terminal-todo: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("Local privacy permissions:")
	checkMode := func(path string, want os.FileMode) {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			fmt.Printf("  %s ERROR: %v\n", filepath.Base(path), err)
			return
		}
		actual := info.Mode().Perm()
		if actual == want {
			fmt.Printf("  %-20s %04o (ok)\n", filepath.Base(path), actual)
			return
		}
		fmt.Printf("  %-20s %04o (expected %04o)", filepath.Base(path), actual, want)
		if fix {
			if err := os.Chmod(path, want); err != nil {
				fmt.Printf(" ERROR: %v\n", err)
			} else {
				fmt.Print(" fixed\n")
			}
		} else {
			fmt.Println()
		}
	}
	checkMode(ttDir, 0700)
	for _, entry := range entries {
		path := filepath.Join(ttDir, entry.Name())
		if !entry.IsDir() && (filepath.Ext(entry.Name()) == ".bin" ||
			filepath.Ext(entry.Name()) == ".json" ||
			filepath.Ext(entry.Name()) == ".lock") {
			checkMode(path, 0600)
		}
	}

	// Lock sidecars are stable synchronization inodes and intentionally persist.
	fmt.Println()
	fmt.Println("Lock files (persistent):")
	now := time.Now()
	foundLocks := false
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".lock" {
			fmt.Printf("  %s (ok)\n", entry.Name())
			foundLocks = true
		}
	}
	if !foundLocks {
		fmt.Println("  none found")
	}

	// Check for orphaned temp files
	fmt.Println()
	fmt.Println("Orphaned temp files:")
	foundTemp := false
	for _, entry := range entries {
		if len(entry.Name()) > 4 && entry.Name()[0] == '.' &&
			(len(entry.Name()) > 5 && entry.Name()[len(entry.Name())-4:] == ".tmp") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			age := now.Sub(info.ModTime())
			fmt.Printf("  %s (%s old)\n", entry.Name(), formatDuration(age))
			if age > 1*time.Hour && fix {
				path := filepath.Join(ttDir, entry.Name())
				if err := os.Remove(path); err != nil {
					fmt.Printf("    ERROR removing: %v\n", err)
				} else {
					fmt.Printf("    removed\n")
				}
			}
			foundTemp = true
		}
	}
	if !foundTemp {
		fmt.Println("  none found")
	}

	// Try loading the store
	fmt.Println()
	fmt.Println("Store integrity:")
	s, err := store.Load(tasksBin)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
	} else {
		fmt.Printf("  %d tasks, next ID: %d\n", len(s.Tasks), s.NextID)
		problems := storeInvariantProblems(s)
		if len(problems) > 0 {
			fmt.Printf("  WARNING: %d invariant problem(s)\n", len(problems))
			for _, problem := range problems {
				fmt.Printf("    - %s\n", problem)
			}
		} else {
			fmt.Println("  task graph invariants ok")
		}
	}

	if !fix {
		fmt.Println()
		fmt.Println("Run with --fix to remove orphaned temp files.")
	}
}

func storeInvariantProblems(s *store.TaskStore) []string {
	var problems []string
	var maxID uint64

	for key, task := range s.Tasks {
		if task == nil {
			problems = append(problems, fmt.Sprintf("task map entry %d is nil", key))
			continue
		}
		if key != task.ID {
			problems = append(problems, fmt.Sprintf("task map key %d contains task ID %d", key, task.ID))
		}
		if task.ID > maxID {
			maxID = task.ID
		}
		if task.Status > store.StatusBlocked {
			problems = append(problems, fmt.Sprintf("task %d has invalid status %d", task.ID, task.Status))
		}
		if !validPriority32(task.Priority) {
			problems = append(problems, fmt.Sprintf("task %d has invalid priority %v", task.ID, task.Priority))
		}
		if task.Status == store.StatusInProgress {
			if task.Owner == "" || task.LeaseExpires == 0 {
				problems = append(problems, fmt.Sprintf("task %d is in progress without a complete ownership lease", task.ID))
			}
		} else if task.Owner != "" || task.LeaseExpires != 0 {
			problems = append(problems, fmt.Sprintf("task %d has ownership metadata while not in progress", task.ID))
		}
		if task.Status == store.StatusCompleted && task.Completed == 0 {
			problems = append(problems, fmt.Sprintf("task %d is completed without a completion timestamp", task.ID))
		}
		if task.Status != store.StatusCompleted && task.Completed != 0 {
			problems = append(problems, fmt.Sprintf("task %d has a completion timestamp while not completed", task.ID))
		}

		for _, dep := range task.Depends {
			repository, depID, err := dag.ParseTaskURI(dep)
			if err != nil {
				if localID, local := dag.ParseLocalID(dep); local {
					depID = localID
					repository = "local"
				} else {
					problems = append(problems, fmt.Sprintf("task %d has invalid dependency %q", task.ID, dep))
					continue
				}
			}
			if repository == "local" {
				if _, ok := s.Tasks[depID]; !ok {
					problems = append(problems, fmt.Sprintf("task %d references missing local task %d", task.ID, depID))
				}
			}
		}
	}

	if s.NextID == 0 || s.NextID <= maxID {
		problems = append(problems, fmt.Sprintf("next ID %d must be greater than maximum task ID %d", s.NextID, maxID))
	}

	graph := dag.NewDAG()
	graph.BuildFromTasks(s.Tasks)
	if err := graph.DetectCycle(nil, 0); err != nil {
		problems = append(problems, fmt.Sprintf("local dependency graph: %v", err))
	}

	return problems
}

func formatSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}
