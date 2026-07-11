package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"terminal-todo/dag"
	"terminal-todo/store"
)

type repositoryRegistry struct {
	Repositories map[string]string `json:"repositories"`
}

func registryPath() string {
	return filepath.Join(projectRoot, ".terminal-todo", "repositories.json")
}

func loadRepositoryRegistry() (*repositoryRegistry, error) {
	data, err := os.ReadFile(registryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &repositoryRegistry{Repositories: make(map[string]string)}, nil
		}
		return nil, err
	}
	var registry repositoryRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("invalid repository registry: %w", err)
	}
	if registry.Repositories == nil {
		registry.Repositories = make(map[string]string)
	}
	return &registry, nil
}

func saveRepositoryRegistry(registry *repositoryRegistry) error {
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(registryPath()), ".repositories-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, registryPath())
}

func dependencyResolver() dag.DependencyResolver {
	registry, err := loadRepositoryRegistry()
	if err != nil {
		return func(string) bool { return false }
	}
	cache := make(map[string]bool)
	return func(uri string) bool {
		if completed, ok := cache[uri]; ok {
			return completed
		}
		repository, id, err := dag.ParseTaskURI(uri)
		if err != nil || repository == "local" {
			return false
		}
		linkedPath, ok := registry.Repositories[repository]
		if !ok {
			return false
		}
		if !filepath.IsAbs(linkedPath) {
			linkedPath = filepath.Join(projectRoot, linkedPath)
		}
		s, err := store.Load(filepath.Join(filepath.Clean(linkedPath), ".terminal-todo", "tasks.bin"))
		if err != nil {
			return false
		}
		task, ok := s.GetTask(id)
		completed := ok && task.Status == store.StatusCompleted
		cache[uri] = completed
		return completed
	}
}

func snapshotDependencyResolver(tasks []*store.Task) dag.DependencyResolver {
	live := dependencyResolver()
	resolved := make(map[string]bool)
	for _, task := range tasks {
		for _, dependency := range task.Depends {
			if _, local := dag.ParseLocalID(dependency); !local {
				resolved[dependency] = live(dependency)
			}
		}
	}
	return func(uri string) bool { return resolved[uri] }
}
