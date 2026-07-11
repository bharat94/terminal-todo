package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"terminal-todo/dag"
	"terminal-todo/store"
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

	fmt.Printf("  tasks.bin         %s\n", checkFile("tasks.bin", tasksBin, true))
	fmt.Printf("  repositories.json %s\n", checkFile("repos.json", reposJSON, false))
	fmt.Printf("  config.json       %s\n", checkFile("config.json", configJSON, false))

	// Check for stale lock files
	fmt.Println()
	fmt.Println("Stale lock files:")
	entries, err := os.ReadDir(ttDir)
	if err != nil {
		fmt.Printf("  ERROR reading .terminal-todo: %v\n", err)
		return
	}

	now := time.Now()
	foundStale := false
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".lock" {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			age := now.Sub(info.ModTime())
			fmt.Printf("  %s (%s old)\n", entry.Name(), formatDuration(age))
			if age > 5*time.Minute && fix {
				path := filepath.Join(ttDir, entry.Name())
				if err := os.Remove(path); err != nil {
					fmt.Printf("    ERROR removing: %v\n", err)
				} else {
					fmt.Printf("    removed\n")
				}
			}
			foundStale = true
		}
	}
	if !foundStale {
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
		// Check for orphaned dep references
		orphaned := 0
		for _, t := range s.Tasks {
			for _, dep := range t.Depends {
				depID, local := dag.ParseLocalID(dep)
				if local {
					if _, ok := s.Tasks[depID]; !ok {
						orphaned++
					}
				}
			}
		}
		if orphaned > 0 {
			fmt.Printf("  WARNING: %d orphaned dependency reference(s)\n", orphaned)
		} else {
			fmt.Println("  no orphaned dependencies")
		}
	}

	if !fix {
		fmt.Println()
		fmt.Println("Run with --fix to remove stale locks and orphaned temp files.")
	}
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
