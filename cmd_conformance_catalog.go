package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bharat94/terminal-todo/conformance"
	"github.com/bharat94/terminal-todo/store"
)

type catalogFixtureRuntime struct {
	Fixture    conformance.Fixture
	ActorNames map[string]string
	TaskIDs    map[string]string
}

func materializeCatalogFixture(scenario conformance.CatalogScenario, todoExecutable string) (catalogFixtureRuntime, error) {
	runtime := catalogFixtureRuntime{
		ActorNames: make(map[string]string, len(scenario.Actors)),
		TaskIDs:    make(map[string]string, len(scenario.Project.Tasks)),
	}
	for _, actor := range scenario.Actors {
		runtime.ActorNames["actor:"+actor.Ref] = actor.Name
	}

	files, err := catalogIntegrationFiles(todoExecutable, scenario.SkillPolicy == "project_integration")
	if err != nil {
		return catalogFixtureRuntime{}, err
	}
	files = append(files, conformance.ClockFixture(scenario.InitialTime))
	if scenario.Project.Initialized {
		storeFile, agentFile, taskIDs, err := catalogProjectFiles(scenario, runtime.ActorNames)
		if err != nil {
			return catalogFixtureRuntime{}, err
		}
		files = append(files, storeFile, agentFile)
		runtime.TaskIDs = taskIDs
	}
	runtime.Fixture = conformance.Fixture{Files: files}
	return runtime, nil
}

func catalogIntegrationFiles(todoExecutable string, includeSkills bool) ([]conformance.FixtureFile, error) {
	staging, err := os.MkdirTemp("", "terminal-todo-catalog-integration-*")
	if err != nil {
		return nil, fmt.Errorf("create catalog integration staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	if includeSkills {
		prepared, err := prepareIntegration(staging, []integrationTarget{integrateCodex, integrateClaude}, todoExecutable, true)
		if err != nil {
			return nil, fmt.Errorf("prepare catalog integration: %w", err)
		}
		files := make([]conformance.FixtureFile, 0, len(prepared))
		for _, file := range prepared {
			relative, err := filepath.Rel(staging, file.path)
			if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("resolve catalog integration path %q", file.path)
			}
			files = append(files, conformance.FixtureFile{Path: filepath.ToSlash(relative), Content: file.content, Mode: 0o600})
		}
		return files, nil
	}

	codexConfig, err := mergeCodexMCPConfig(nil, todoExecutable, true)
	if err != nil {
		return nil, err
	}
	claudeConfig, err := mergeClaudeMCPConfig(nil, todoExecutable, true)
	if err != nil {
		return nil, err
	}
	return []conformance.FixtureFile{
		{Path: conformance.CodexProjectConfigFile, Content: codexConfig, Mode: 0o600},
		{Path: conformance.ClaudeProjectMCPConfigFile, Content: claudeConfig, Mode: 0o600},
	}, nil
}

func catalogProjectFiles(
	scenario conformance.CatalogScenario,
	actorNames map[string]string,
) (conformance.FixtureFile, conformance.FixtureFile, map[string]string, error) {
	taskStore := store.NewTaskStore()
	createdAt := uint64(scenario.InitialTime.UnixMilli())
	taskIDs := make(map[string]string, len(scenario.Project.Tasks))
	for _, fixtureTask := range scenario.Project.Tasks {
		task := taskStore.AddTask(fixtureTask.Title, nil)
		task.Created = createdAt
		task.Capabilities = append([]string(nil), fixtureTask.Capabilities...)
		task.Extra = cloneStringMap(fixtureTask.Metadata)
		taskIDs["task:"+fixtureTask.Ref] = strconv.FormatUint(task.ID, 10)
	}
	for _, fixtureTask := range scenario.Project.Tasks {
		id, _ := strconv.ParseUint(taskIDs["task:"+fixtureTask.Ref], 10, 64)
		task := taskStore.Tasks[id]
		for _, dependency := range fixtureTask.DependsOn {
			task.Depends = append(task.Depends, "todo://local/"+taskIDs[dependency])
		}
		switch fixtureTask.Status {
		case "pending":
			task.Status = store.StatusPending
		case "in_progress":
			task.Status = store.StatusInProgress
		case "completed":
			task.Status = store.StatusCompleted
			task.Completed = createdAt
		case "blocked":
			task.Status = store.StatusBlocked
		}
		if fixtureTask.Owner != "" {
			task.Owner = actorNames[fixtureTask.Owner]
		}
		if fixtureTask.LeaseExpires != "" {
			expires, err := time.Parse(time.RFC3339, fixtureTask.LeaseExpires)
			if err != nil {
				return conformance.FixtureFile{}, conformance.FixtureFile{}, nil, fmt.Errorf("parse task %q lease: %w", fixtureTask.Ref, err)
			}
			task.LeaseExpires = uint64(expires.UnixMilli())
		}
		taskStore.AddEvent(store.EventTaskCreated, task.ID, "harness", map[string]string{"title": task.Title})
		taskStore.Events[len(taskStore.Events)-1].Timestamp = createdAt
	}
	taskStore.LastModified = createdAt

	staging, err := os.MkdirTemp("", "terminal-todo-catalog-store-*")
	if err != nil {
		return conformance.FixtureFile{}, conformance.FixtureFile{}, nil, fmt.Errorf("create catalog store staging directory: %w", err)
	}
	defer os.RemoveAll(staging)
	storePath := filepath.Join(staging, "tasks.bin")
	if err := taskStore.Save(storePath); err != nil {
		return conformance.FixtureFile{}, conformance.FixtureFile{}, nil, fmt.Errorf("save catalog task store: %w", err)
	}
	storeBytes, err := os.ReadFile(storePath)
	if err != nil {
		return conformance.FixtureFile{}, conformance.FixtureFile{}, nil, fmt.Errorf("read catalog task store: %w", err)
	}

	agents := AgentRegistry{SchemaVersion: "1", Agents: make(map[string]AgentCard, len(scenario.Actors))}
	for _, actor := range scenario.Actors {
		agents.Agents[actor.Name] = AgentCard{
			Name: actor.Name, Capabilities: append([]string(nil), actor.Capabilities...), MaxLoad: actor.MaxLoad,
			CreatedAt: scenario.InitialTime.UTC().Format(time.RFC3339Nano),
		}
	}
	agentBytes, err := json.MarshalIndent(agents, "", "  ")
	if err != nil {
		return conformance.FixtureFile{}, conformance.FixtureFile{}, nil, fmt.Errorf("encode catalog agent registry: %w", err)
	}
	agentBytes = append(agentBytes, '\n')
	return conformance.FixtureFile{Path: ".terminal-todo/tasks.bin", Content: storeBytes, Mode: 0o600},
		conformance.FixtureFile{Path: ".terminal-todo/agents.json", Content: agentBytes, Mode: 0o600}, taskIDs, nil
}

func catalogSequenceSteps(scenario conformance.CatalogScenario, runtime catalogFixtureRuntime) ([]conformance.SequenceStep, error) {
	steps := make([]conformance.SequenceStep, 0, len(scenario.Turns))
	for _, turn := range scenario.Turns {
		step := conformance.SequenceStep{ID: turn.ID}
		switch turn.Action {
		case "prompt":
			step.Action = conformance.SequencePrompt
			step.Actor = runtime.ActorNames[turn.By]
			step.Prompt = catalogActorPrompt(step.Actor, substituteCatalogSymbols(turn.Prompt, runtime))
		case "resume":
			step.Action = conformance.SequenceResume
			step.Actor = runtime.ActorNames[turn.By]
			step.Prompt = substituteCatalogSymbols(turn.Instruction, runtime)
		case "concurrent_turns":
			step.Action = conformance.SequenceConcurrent
			step.Prompt = catalogActorPrompt(conformance.ConformanceActorPlaceholder, substituteCatalogSymbols(turn.Prompt, runtime))
			for _, actor := range turn.Actors {
				step.Actors = append(step.Actors, runtime.ActorNames[actor])
			}
		case "advance_clock":
			step.Action = conformance.SequenceHarness
			advance := time.Duration(turn.Seconds) * time.Second
			step.Harness = func(_ context.Context, workspace string) error {
				_, err := conformance.AdvanceClock(workspace, advance)
				return err
			}
		case "checkpoint":
			step.Action = conformance.SequenceHarness
			step.Harness = func(ctx context.Context, _ string) error { return ctx.Err() }
		default:
			return nil, fmt.Errorf("compile scenario %q turn %q: unsupported action %q", scenario.ID, turn.ID, turn.Action)
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func catalogActorPrompt(actor, prompt string) string {
	return fmt.Sprintf(
		"Use coordination identity %q. Use the project coordination integration only; do not run shell commands or edit files. %s",
		actor, prompt,
	)
}

func substituteCatalogSymbols(value string, runtime catalogFixtureRuntime) string {
	for reference, actor := range runtime.ActorNames {
		value = strings.ReplaceAll(value, reference, actor)
	}
	for reference, taskID := range runtime.TaskIDs {
		value = strings.ReplaceAll(value, reference, taskID)
	}
	return value
}

func cloneStringMap(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
