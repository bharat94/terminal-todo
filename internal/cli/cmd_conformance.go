package cli

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
	Suite         string
}

type conformanceHostProbe struct {
	Host       string `json:"host"`
	Executable string `json:"executable,omitempty"`
	Version    string `json:"version,omitempty"`
	Available  bool   `json:"available"`
	Detail     string `json:"detail,omitempty"`
}

type conformanceCommandReport struct {
	SchemaVersion string                         `json:"schema_version"`
	SuiteID       string                         `json:"suite_id"`
	Mode          string                         `json:"mode"`
	Notice        string                         `json:"notice"`
	Probes        []conformanceHostProbe         `json:"probes,omitempty"`
	Results       []conformanceCatalogHostReport `json:"host_results,omitempty"`
}

type conformanceCatalogHostReport struct {
	Host                   string                              `json:"host"`
	HostVersion            string                              `json:"host_version,omitempty"`
	Model                  string                              `json:"model,omitempty"`
	IntegrationVersion     string                              `json:"integration_version"`
	Transport              string                              `json:"transport"`
	ScenarioResults        []conformance.Report                `json:"scenario_results"`
	Scored                 bool                                `json:"scored"`
	RawScore               float64                             `json:"raw_score"`
	CappedScore            float64                             `json:"capped_score"`
	Level                  string                              `json:"level,omitempty"`
	Criteria               []conformance.CriterionResult       `json:"criteria"`
	HardGateFailures       []string                            `json:"hard_gate_failures"`
	InfrastructureFailures []conformance.InfrastructureFailure `json:"infrastructure_failures"`
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
		Suite:   "v2",
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
	if suite := strings.TrimSpace(optionValue(args, "--suite")); suite != "" {
		if suite != "v1" && suite != "v2" {
			return conformanceOptions{}, fmt.Errorf("--suite must be v1 or v2")
		}
		options.Suite = suite
	}
	return options, nil
}

func executeConformance(ctx context.Context, options conformanceOptions) (conformanceCommandReport, bool, error) {
	catalog, err := conformance.LoadCatalogVersion(options.Suite)
	if err != nil {
		return conformanceCommandReport{}, true, err
	}
	report := conformanceCommandReport{
		SchemaVersion: conformance.ReportSchemaVersion,
		SuiteID:       catalog.SuiteID,
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
	report.Notice = fmt.Sprintf("Real-agent mode executes all %d isolated %s catalog scenarios for each host that passes preflight. Authentication or MCP approval failures stop that host before behavioral scoring.", len(catalog.Scenarios), options.Suite)
	report.Results = make([]conformanceCatalogHostReport, 0, len(options.Hosts))
	unsuccessful := false
	for _, hostName := range options.Hosts {
		hostReport, err := runCatalogEvaluation(ctx, hostName, options, catalog)
		if err != nil {
			return report, true, err
		}
		report.Results = append(report.Results, hostReport)
		if hostReport.Scored && hostReport.Level != "conformant" {
			unsuccessful = true
		}
		for _, failure := range hostReport.InfrastructureFailures {
			if failure.Disposition == conformance.DispositionFail {
				unsuccessful = true
			}
		}
	}
	return report, unsuccessful, nil
}

func runCatalogEvaluation(
	ctx context.Context,
	hostName string,
	options conformanceOptions,
	catalog conformance.Catalog,
) (conformanceCatalogHostReport, error) {
	result := conformanceCatalogHostReport{
		Host: hostName, IntegrationVersion: versionString(), Transport: "mcp",
		ScenarioResults: []conformance.Report{}, Criteria: []conformance.CriterionResult{},
		HardGateFailures: []string{}, InfrastructureFailures: []conformance.InfrastructureFailure{},
	}
	executable, err := exec.LookPath(hostName)
	if err != nil {
		for _, scenario := range catalog.Scenarios {
			result.ScenarioResults = append(result.ScenarioResults, unavailableScenarioReport(hostName, scenario.ID, "host executable not found"))
		}
		result.InfrastructureFailures = append(result.InfrastructureFailures, conformance.InfrastructureFailure{
			Kind: conformance.FailureStart, Disposition: conformance.DispositionSkip, Phase: "preflight", Detail: "host executable not found",
		})
		return result, nil
	}
	todoExecutable, err := os.Executable()
	if err != nil {
		return result, fmt.Errorf("locate terminal-todo executable: %w", err)
	}
	result.HostVersion = probeConformanceHost(ctx, hostName).Version
	hostOptions := conformance.MachineHostOptions{
		Executable: executable, MCPExecutable: todoExecutable, Version: result.HostVersion,
		Model: options.Model, IntegrationVersion: versionString(), Prompt: "Run the selected conformance scenario.",
		PersistentSessions: true,
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
		return result, err
	}
	host = applyConformanceModel(host, options.Model)
	host.Run.Env = cloneStringMap(host.Run.Env)
	host.Run.Env[conformance.ConformanceTraceEnvironment] = filepath.ToSlash(filepath.Join(conformance.ConformanceWorkspacePlaceholder, conformance.ConformanceTraceFile))
	result.Model = host.Model

	allChecks := []conformance.CheckResult{}
	infrastructureBlocked := false
	for _, scenario := range catalog.Scenarios {
		if infrastructureBlocked {
			skipped := unavailableScenarioReport(hostName, scenario.ID, "host preflight did not permit catalog execution")
			result.ScenarioResults = append(result.ScenarioResults, skipped)
			continue
		}
		runtime, err := materializeCatalogFixture(scenario, todoExecutable)
		if err != nil {
			return result, err
		}
		steps, err := catalogSequenceSteps(scenario, runtime)
		if err != nil {
			return result, err
		}
		assertions, err := compileCatalogAssertions(scenario, runtime)
		if err != nil {
			return result, err
		}
		timeout := time.Duration(scenario.TimeoutSeconds) * time.Second
		if options.Timeout < timeout {
			timeout = options.Timeout
		}
		evaluation := conformance.SequenceEvaluation{
			ID: scenario.ID, Host: host, Fixture: runtime.Fixture, Steps: steps,
			Limits:     conformance.Limits{Timeout: timeout, MaxOutputBytes: 4 * 1024 * 1024, MaxEventBytes: 256 * 1024},
			Normalizer: catalogNormalizer(hostName, runtime), Assertions: assertions, MinimumScore: 100,
			KeepWorkspace: options.KeepWorkspace,
		}
		scenarioReport, err := (conformance.Runner{}).RunSequence(ctx, evaluation)
		if err != nil {
			return result, fmt.Errorf("execute scenario %q for %s: %w", scenario.ID, hostName, err)
		}
		result.ScenarioResults = append(result.ScenarioResults, scenarioReport)
		allChecks = append(allChecks, scenarioReport.Checks...)
		result.InfrastructureFailures = append(result.InfrastructureFailures, scenarioReport.InfrastructureFailures...)
		if scenarioReport.Status == conformance.StatusSkipped {
			infrastructureBlocked = true
		}
	}
	if len(result.InfrastructureFailures) > 0 {
		return result, nil
	}
	score, err := conformance.Grade(catalog.ScoringModel, allChecks)
	if err != nil {
		return result, err
	}
	result.Scored = true
	result.RawScore = score.RawScore
	result.CappedScore = score.CappedScore
	result.Level = score.Level
	result.Criteria = score.Criteria
	result.HardGateFailures = score.HardGateFailures
	return result, nil
}

func applyConformanceModel(host conformance.Host, model string) conformance.Host {
	if strings.TrimSpace(model) == "" {
		return host
	}
	host.Run.Args = insertConformanceModelArgs(host.Name, host.Run.Args, model)
	if host.Resume != nil {
		resume := host.Resume
		host.Resume = func(sessionID, prompt string) (conformance.Command, error) {
			command, err := resume(sessionID, prompt)
			if err == nil {
				command.Args = insertConformanceModelArgs(host.Name, command.Args, model)
			}
			return command, err
		}
	}
	return host
}

func insertConformanceModelArgs(hostName string, arguments []string, model string) []string {
	if hostName != "codex" {
		return append(append([]string(nil), arguments...), "--model", model)
	}
	insertAt := 0
	for index, argument := range arguments {
		if argument == "exec" || argument == "resume" {
			insertAt = index + 1
		}
	}
	result := make([]string, 0, len(arguments)+2)
	result = append(result, arguments[:insertAt]...)
	result = append(result, "--model", model)
	result = append(result, arguments[insertAt:]...)
	return result
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
	host = applyConformanceModel(host, options.Model)

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
		conformance.ClockFixture(time.Now().UTC()),
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
	return unavailableScenarioReport(host, lifecycleScenarioID, detail)
}

func unavailableScenarioReport(host, scenarioID, detail string) conformance.Report {
	return conformance.Report{
		SchemaVersion: conformance.ReportSchemaVersion,
		ScenarioID:    scenarioID,
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
	for _, result := range report.Results {
		state := "unscored"
		if result.Scored {
			state = result.Level
		}
		fmt.Printf("  %-8s %-14s %.1f/100\n", result.Host, state, result.CappedScore)
		for _, failure := range result.InfrastructureFailures {
			fmt.Printf("           %s: %s\n", failure.Kind, failure.Detail)
		}
		for _, scenario := range result.ScenarioResults {
			fmt.Printf("           %-18s %s", scenario.ScenarioID, scenario.Status)
			if scenario.Score.Scored {
				fmt.Printf(" %.1f%%", scenario.Score.Percent)
			}
			fmt.Println()
			if scenario.Workspace != "" {
				fmt.Printf("             workspace: %s\n", scenario.Workspace)
			}
		}
	}
}

func compactConformanceTranscripts(report conformanceCommandReport) conformanceCommandReport {
	report.Notice += " Raw host events are omitted; pass --include-events with --json to include them."
	for hostIndex := range report.Results {
		for scenarioIndex := range report.Results[hostIndex].ScenarioResults {
			result := &report.Results[hostIndex].ScenarioResults[scenarioIndex]
			if result.Preflight != nil {
				result.Preflight.Capture.Stdout = []conformance.Event{}
				result.Preflight.Capture.Stderr = []conformance.Event{}
			}
			if result.Execution != nil {
				result.Execution.Capture.Stdout = []conformance.Event{}
				result.Execution.Capture.Stderr = []conformance.Event{}
			}
			for turnIndex := range result.Turns {
				result.Turns[turnIndex].Execution.Capture.Stdout = []conformance.Event{}
				result.Turns[turnIndex].Execution.Capture.Stderr = []conformance.Event{}
			}
			result.Evidence.HostEvents = []conformance.Event{}
		}
	}
	return report
}

func versionString() string {
	if version == "" {
		return "dev"
	}
	return version
}
