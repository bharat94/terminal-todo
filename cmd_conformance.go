package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bharat94/terminal-todo/conformance"
	"github.com/bharat94/terminal-todo/store"
)

const (
	conformanceSuiteID   = "terminal-todo-real-agent-v1"
	lifecycleScenarioID  = "lifecycle_smoke"
	defaultEvalTimeout   = 10 * time.Minute
	conformanceActorBase = "eval"
)

type conformanceOptions struct {
	Hosts         []string
	Run           bool
	JSON          bool
	IncludeEvents bool
	KeepWorkspace bool
	Timeout       time.Duration
	Model         string
}

type conformanceHostProbe struct {
	Host       string `json:"host"`
	Executable string `json:"executable,omitempty"`
	Version    string `json:"version,omitempty"`
	Available  bool   `json:"available"`
	Detail     string `json:"detail,omitempty"`
}

type conformanceCommandReport struct {
	SchemaVersion string                 `json:"schema_version"`
	SuiteID       string                 `json:"suite_id"`
	Mode          string                 `json:"mode"`
	Notice        string                 `json:"notice"`
	Probes        []conformanceHostProbe `json:"probes,omitempty"`
	Reports       []conformance.Report   `json:"reports,omitempty"`
}

func cmdConformance(args []string) {
	options, err := parseConformanceOptions(args)
	if err != nil {
		fail(ErrInvalidArgs, "conformance: %v", err)
	}

	report, unsuccessful, err := executeConformance(context.Background(), options)
	if err != nil {
		fail(ErrInvalidArgs, "conformance: %v", err)
	}
	if options.JSON {
		if !options.IncludeEvents {
			report = compactConformanceTranscripts(report)
		}
		writeJSON(report)
	} else {
		printConformanceReport(report)
	}
	if unsuccessful {
		os.Exit(1)
	}
}

func parseConformanceOptions(args []string) (conformanceOptions, error) {
	options := conformanceOptions{
		Hosts:   []string{"codex", "claude"},
		Timeout: defaultEvalTimeout,
	}
	if host := optionValue(args, "--host"); host != "" {
		switch host {
		case "all":
			options.Hosts = []string{"codex", "claude"}
		case "codex", "claude":
			options.Hosts = []string{host}
		default:
			return conformanceOptions{}, fmt.Errorf("--host must be codex, claude, or all")
		}
	}
	if raw := optionValue(args, "--timeout"); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil || timeout <= 0 || timeout > 30*time.Minute {
			return conformanceOptions{}, fmt.Errorf("--timeout must be between 1ns and 30m")
		}
		options.Timeout = timeout
	}
	options.Run = hasFlag(args, "--run")
	options.JSON = hasFlag(args, "--json")
	options.IncludeEvents = hasFlag(args, "--include-events")
	options.KeepWorkspace = hasFlag(args, "--keep-workspace")
	options.Model = strings.TrimSpace(optionValue(args, "--model"))
	return options, nil
}

func executeConformance(ctx context.Context, options conformanceOptions) (conformanceCommandReport, bool, error) {
	report := conformanceCommandReport{
		SchemaVersion: conformance.ReportSchemaVersion,
		SuiteID:       conformanceSuiteID,
		Mode:          "preflight",
		Notice:        "Preflight is local and does not contact a model. Pass --run to transmit the controlled evaluation prompt and consume host usage.",
		Probes:        make([]conformanceHostProbe, 0, len(options.Hosts)),
	}
	for _, host := range options.Hosts {
		report.Probes = append(report.Probes, probeConformanceHost(ctx, host))
	}
	if !options.Run {
		return report, false, nil
	}

	report.Mode = "real-agent"
	report.Notice = "Real-agent mode transmits one controlled lifecycle prompt to each host that passes preflight. Authentication or MCP approval failures stop before the prompt and are reported as infrastructure skips."
	report.Reports = make([]conformance.Report, 0, len(options.Hosts))
	unsuccessful := false
	for _, hostName := range options.Hosts {
		hostReport, err := runLifecycleEvaluation(ctx, hostName, options)
		if err != nil {
			return report, true, err
		}
		report.Reports = append(report.Reports, hostReport)
		if hostReport.Status == conformance.StatusFailed || hostReport.Status == conformance.StatusError {
			unsuccessful = true
		}
	}
	return report, unsuccessful, nil
}

func probeConformanceHost(ctx context.Context, name string) conformanceHostProbe {
	probe := conformanceHostProbe{Host: name}
	executable, err := exec.LookPath(name)
	if err != nil {
		probe.Detail = "executable not found"
		return probe
	}
	probe.Executable = executable
	versionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(versionCtx, executable, "--version").Output()
	if err != nil {
		probe.Detail = "version probe failed"
		return probe
	}
	probe.Available = true
	probe.Version = strings.TrimSpace(string(output))
	return probe
}

func runLifecycleEvaluation(
	ctx context.Context,
	hostName string,
	options conformanceOptions,
) (conformance.Report, error) {
	executable, err := exec.LookPath(hostName)
	if err != nil {
		return unavailableHostReport(hostName, "host executable not found"), nil
	}
	todoExecutable, err := os.Executable()
	if err != nil {
		return conformance.Report{}, fmt.Errorf("locate terminal-todo executable: %w", err)
	}
	marker, err := randomConformanceToken()
	if err != nil {
		return conformance.Report{}, err
	}
	actor := conformanceActorBase + "-" + hostName + "-" + marker
	requestID := "conformance-" + marker
	prompt := lifecyclePrompt(actor, requestID, marker)
	version := probeConformanceHost(ctx, hostName).Version
	hostOptions := conformance.MachineHostOptions{
		Executable:         executable,
		MCPExecutable:      todoExecutable,
		Version:            version,
		Model:              options.Model,
		IntegrationVersion: versionString(),
		Prompt:             prompt,
	}

	var host conformance.Host
	switch hostName {
	case "codex":
		host, err = conformance.NewCodexHost(hostOptions)
	case "claude":
		host, err = conformance.NewClaudeHost(hostOptions)
	default:
		err = fmt.Errorf("unsupported host %q", hostName)
	}
	if err != nil {
		return conformance.Report{}, err
	}
	if options.Model != "" {
		if hostName == "codex" {
			host.Run.Args = append(host.Run.Args, "--model", options.Model)
		} else {
			host.Run.Args = append(host.Run.Args, "--model", options.Model)
		}
	}

	fixture, err := lifecycleFixture(todoExecutable)
	if err != nil {
		return conformance.Report{}, err
	}
	evaluation := conformance.Evaluation{
		ID:            lifecycleScenarioID,
		Host:          host,
		Fixture:       fixture,
		Limits:        conformance.Limits{Timeout: options.Timeout, MaxOutputBytes: 4 * 1024 * 1024, MaxEventBytes: 256 * 1024},
		Normalizer:    lifecycleNormalizer(hostName),
		Assertions:    lifecycleAssertions(actor, marker),
		MinimumScore:  100,
		KeepWorkspace: options.KeepWorkspace,
	}
	return (conformance.Runner{}).Run(ctx, evaluation)
}

func lifecycleFixture(todoExecutable string) (conformance.Fixture, error) {
	staging, err := os.MkdirTemp("", "terminal-todo-conformance-store-*")
	if err != nil {
		return conformance.Fixture{}, fmt.Errorf("create conformance store staging directory: %w", err)
	}
	defer os.RemoveAll(staging)
	storePath := filepath.Join(staging, "tasks.bin")
	taskStore := store.NewTaskStore()
	task := taskStore.AddTask("Persist the conformance lifecycle marker", nil)
	taskStore.AddEvent(store.EventTaskCreated, task.ID, "", map[string]string{"title": task.Title})
	if err := taskStore.Save(storePath); err != nil {
		return conformance.Fixture{}, fmt.Errorf("create conformance task store: %w", err)
	}
	taskBytes, err := os.ReadFile(storePath)
	if err != nil {
		return conformance.Fixture{}, fmt.Errorf("read conformance task store: %w", err)
	}

	codexConfig := fmt.Sprintf(
		"[mcp_servers.terminal-todo]\ncommand = %s\nargs = [\"mcp\", \"--stdio\"]\nrequired = true\n",
		strconv.Quote(todoExecutable),
	)
	claudeConfig, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{
			"terminal-todo": map[string]any{
				"type":    "stdio",
				"command": todoExecutable,
				"args":    []string{"mcp", "--stdio"},
			},
		},
	})
	if err != nil {
		return conformance.Fixture{}, fmt.Errorf("encode Claude MCP fixture: %w", err)
	}
	return conformance.Fixture{Files: []conformance.FixtureFile{
		{Path: ".terminal-todo/tasks.bin", Content: taskBytes, Mode: 0o600},
		{Path: conformance.CodexProjectConfigFile, Content: []byte(codexConfig), Mode: 0o600},
		{Path: conformance.ClaudeProjectMCPConfigFile, Content: claudeConfig, Mode: 0o600},
	}}, nil
}

func lifecyclePrompt(actor, requestID, marker string) string {
	return fmt.Sprintf(
		"You are one worker joining an existing coordinated project. Use the project's coordination integration only: do not run shell commands or edit files. "+
			"Resume as actor %q with one bounded bootstrap, then atomically acquire one ready task using request ID %q. "+
			"On the acquired task persist the structured field conformance_marker=%q, then complete it as its owner. "+
			"Keep routine coordination hidden and reply with one concise outcome sentence.",
		actor, requestID, marker,
	)
}

func lifecycleNormalizer(hostName string) conformance.Normalizer {
	return conformance.NormalizerFunc(func(_ context.Context, workspace string, capture conformance.Capture) (conformance.Evidence, error) {
		evidence := conformance.EmptyEvidence(capture)
		evidence.HostEvents = []conformance.Event{}
		taskStore, err := store.Load(filepath.Join(workspace, ".terminal-todo", "tasks.bin"))
		if err != nil {
			return evidence, fmt.Errorf("load post-evaluation task store: %w", err)
		}
		for id, task := range taskStore.Tasks {
			evidence.Tasks[strconv.FormatUint(id, 10)] = lifecycleTaskState{
				ID:           task.ID,
				Status:       statusName(task.Status),
				Owner:        task.Owner,
				LeaseExpires: task.LeaseExpires,
				Extra:        task.Extra,
			}
		}
		for _, event := range taskStore.Events {
			encoded, err := json.Marshal(event)
			if err != nil {
				return evidence, fmt.Errorf("encode post-evaluation event: %w", err)
			}
			evidence.Events = append(evidence.Events, encoded)
			evidence.Operations = append(evidence.Operations, conformance.Operation{
				Actor:     event.Actor,
				Operation: string(event.Type),
				Transport: "terminal-todo-audit",
				Result:    map[string]any{"task_id": event.TaskID, "data": event.Data},
			})
		}
		evidence.AssistantMessages = extractHostAssistantMessages(hostName, capture)
		return evidence, nil
	})
}

func lifecycleAssertions(actor, marker string) []conformance.Assertion {
	return []conformance.Assertion{
		conformance.EvidenceCheck("completed", "the acquired task is completed and unowned", 35, true, func(evidence conformance.Evidence) (bool, string) {
			task, ok := evidence.Tasks["1"].(lifecycleTaskState)
			if !ok {
				return false, "task 1 is missing from post-evaluation state"
			}
			if task.Status != "completed" || task.Owner != "" || task.LeaseExpires != 0 {
				return false, "task 1 did not finish completed and unowned"
			}
			return true, ""
		}).WithCriteria("closes_ownership").WithHardGate("abandoned_ownership"),
		conformance.EvidenceCheck("marker", "the worker persisted the requested structured marker", 25, true, func(evidence conformance.Evidence) (bool, string) {
			task, ok := evidence.Tasks["1"].(lifecycleTaskState)
			if !ok || task.Extra["conformance_marker"] != marker {
				return false, "conformance_marker is missing or incorrect"
			}
			return true, ""
		}).WithCriteria("hands_off_durably"),
		conformance.EvidenceCheck("audit_lifecycle", "the audit records claim and completion by the evaluated actor", 25, true, func(evidence conformance.Evidence) (bool, string) {
			claimed := false
			for _, operation := range evidence.Operations {
				if operation.Actor != actor {
					continue
				}
				if operation.Operation == string(store.EventTaskClaimed) {
					claimed = true
				}
				if claimed && operation.Operation == string(store.EventTaskCompleted) {
					return true, ""
				}
			}
			return false, "ordered claimed and completed audit events were not recorded for the actor"
		}).WithCriteria("allocates_atomically"),
		conformance.EvidenceCheck("quiet_outcome", "the final response is concise and excludes protocol payloads", 15, true, func(evidence conformance.Evidence) (bool, string) {
			if len(evidence.AssistantMessages) != 1 {
				return false, fmt.Sprintf("expected one final assistant message, got %d", len(evidence.AssistantMessages))
			}
			message := evidence.AssistantMessages[0]
			if len(strings.Fields(message)) > 40 {
				return false, "final assistant message exceeded 40 words"
			}
			lower := strings.ToLower(message)
			for _, forbidden := range []string{"todo acquire", "terminal_todo_", "schema_version", "lease_expires", "requestid"} {
				if strings.Contains(lower, forbidden) {
					return false, "final assistant message leaked routine protocol details"
				}
			}
			return true, ""
		}).WithCriteria("coordinates_quietly"),
	}
}

type lifecycleTaskState struct {
	ID           uint64            `json:"id"`
	Status       string            `json:"status"`
	Owner        string            `json:"owner"`
	LeaseExpires uint64            `json:"lease_expires"`
	Extra        map[string]string `json:"extra"`
}

func extractHostAssistantMessages(hostName string, capture conformance.Capture) []string {
	var candidates []string
	for _, event := range capture.Stdout {
		if event.Kind != conformance.EventJSON {
			continue
		}
		var value map[string]any
		if json.Unmarshal(event.JSON, &value) != nil {
			continue
		}
		switch hostName {
		case "codex":
			if value["type"] != "item.completed" {
				continue
			}
			item, _ := value["item"].(map[string]any)
			if item["type"] == "agent_message" {
				if text, _ := item["text"].(string); strings.TrimSpace(text) != "" {
					candidates = append(candidates, strings.TrimSpace(text))
				}
			}
		case "claude":
			if value["type"] == "result" {
				if text, _ := value["result"].(string); strings.TrimSpace(text) != "" {
					candidates = append(candidates, strings.TrimSpace(text))
				}
			}
		}
	}
	if len(candidates) == 0 {
		return []string{}
	}
	return []string{candidates[len(candidates)-1]}
}

func randomConformanceToken() (string, error) {
	var buffer [8]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return "", fmt.Errorf("generate conformance marker: %w", err)
	}
	return hex.EncodeToString(buffer[:]), nil
}

func unavailableHostReport(host, detail string) conformance.Report {
	return conformance.Report{
		SchemaVersion: conformance.ReportSchemaVersion,
		ScenarioID:    lifecycleScenarioID,
		Host:          host,
		Status:        conformance.StatusSkipped,
		Evidence:      conformance.EmptyEvidence(conformance.Capture{}),
		Checks:        []conformance.CheckResult{},
		Score:         conformance.Score{Scored: false},
		InfrastructureFailures: []conformance.InfrastructureFailure{{
			Kind:        conformance.FailureStart,
			Disposition: conformance.DispositionSkip,
			Phase:       "preflight",
			Detail:      detail,
		}},
	}
}

func printConformanceReport(report conformanceCommandReport) {
	fmt.Printf("terminal-todo real-agent conformance (%s)\n", report.Mode)
	fmt.Println(report.Notice)
	for _, probe := range report.Probes {
		state := "unavailable"
		if probe.Available {
			state = probe.Version
		}
		fmt.Printf("  %-8s %s\n", probe.Host, state)
	}
	for _, result := range report.Reports {
		fmt.Printf("  %-8s %-8s", result.Host, result.Status)
		if result.Score.Scored {
			fmt.Printf(" %.1f%%", result.Score.Percent)
		}
		fmt.Println()
		for _, failure := range result.InfrastructureFailures {
			fmt.Printf("           %s: %s\n", failure.Kind, failure.Detail)
		}
		for _, check := range result.Checks {
			state := "pass"
			if !check.Passed {
				state = "fail"
			}
			fmt.Printf("           %-4s %s", state, check.ID)
			if check.Detail != "" {
				fmt.Printf(": %s", check.Detail)
			}
			fmt.Println()
		}
		if result.Workspace != "" {
			fmt.Printf("           workspace: %s\n", result.Workspace)
		}
	}
}

func compactConformanceTranscripts(report conformanceCommandReport) conformanceCommandReport {
	report.Notice += " Raw host events are omitted; pass --include-events with --json to include them."
	for i := range report.Reports {
		result := &report.Reports[i]
		if result.Preflight != nil {
			result.Preflight.Capture.Stdout = []conformance.Event{}
			result.Preflight.Capture.Stderr = []conformance.Event{}
		}
		if result.Execution != nil {
			result.Execution.Capture.Stdout = []conformance.Event{}
			result.Execution.Capture.Stderr = []conformance.Event{}
		}
		result.Evidence.HostEvents = []conformance.Event{}
	}
	return report
}

func versionString() string {
	if version == "" {
		return "dev"
	}
	return version
}
