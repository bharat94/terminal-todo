package main

import "fmt"

func cmdUnlink(args []string) {
	if len(args) != 1 {
		fail(ErrInvalidArgs, "usage: todo unlink <alias>")
	}
	alias := args[0]
	if alias == "local" {
		fail(ErrInvalidArgs, "repository alias local is reserved")
	}

	var found bool
	if err := updateRepositoryRegistry(func(registry *repositoryRegistry) error {
		if _, ok := registry.Repositories[alias]; !ok {
			return fmt.Errorf("repository alias %s not found", alias)
		}
		delete(registry.Repositories, alias)
		found = true
		return nil
	}); err != nil {
		if !found {
			fail(ErrInvalidArgs, "%v", err)
		}
		fail(ErrStoreCorrupted, "saving repository registry: %v", err)
	}
	fmt.Printf("Unlinked %s\n", alias)
}
