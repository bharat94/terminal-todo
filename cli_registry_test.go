package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryNamesAndAliasesAreUnique(t *testing.T) {
	seen := make(map[string]string)
	for _, cmd := range commandRegistry() {
		for _, name := range append([]string{cmd.Name}, cmd.Aliases...) {
			if previous, clash := seen[name]; clash {
				t.Errorf("%q is declared by both %s and %s", name, previous, cmd.Name)
			}
			seen[name] = cmd.Name
		}
	}
}

func TestEveryRegistryCommandIsRunnableAndDescribed(t *testing.T) {
	for _, cmd := range commandRegistry() {
		t.Run(cmd.Name, func(t *testing.T) {
			assert.NotNil(t, cmd.Run, "no handler")
			assert.NotEmpty(t, cmd.Summary, "no summary")
			assert.NotEmpty(t, cmd.Usage, "no usage")
			assert.Contains(t, helpGroups, cmd.Group, "unknown help group")
			for _, flag := range cmd.Flags {
				assert.True(t, strings.HasPrefix(flag.Name, "--"), "%s is not a long flag", flag.Name)
				assert.NotEmpty(t, flag.Help, "%s has no help text", flag.Name)
			}
		})
	}
}

func TestRegistryFlagsAreDeclaredOncePerCommand(t *testing.T) {
	for _, cmd := range commandRegistry() {
		seen := make(map[string]bool, len(cmd.Flags))
		for _, flag := range cmd.Flags {
			assert.False(t, seen[flag.Name], "%s declares %s twice", cmd.Name, flag.Name)
			seen[flag.Name] = true
		}
	}
}

// The registry is the only flag table. Argument scanning derives its skip set
// from the same declaration that validation uses, which is what makes it
// impossible for a newly added value flag to be validated one way and read
// another.
func TestValueFlagsDeriveFromTheSameDeclarationAsValidation(t *testing.T) {
	for _, cmd := range commandRegistry() {
		values := cmd.valueFlags()
		for _, flag := range cmd.Flags {
			_, skipped := values[flag.Name]
			assert.Equal(t, flag.Kind.takesValue(), skipped,
				"%s %s: validation and scanning disagree about whether it takes a value",
				cmd.Name, flag.Name)
		}
	}
}

func TestValidateArgsRejectsUndeclaredAndIncompleteFlags(t *testing.T) {
	claim, ok := lookupCommand("claim")
	require.True(t, ok)

	assert.NoError(t, claim.validateArgs([]string{"1", "--as", "w1", "--ttl", "30m", "--json"}))

	err := claim.validateArgs([]string{"1", "--nope"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown option --nope")

	// A value flag followed by another flag is a forgotten value, not an actor
	// literally named "--json".
	err = claim.validateArgs([]string{"1", "--as", "--json"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--as requires a value")

	err = claim.validateArgs([]string{"1", "--as"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--as requires a value")
}

func TestAliasesResolveToTheSameCommand(t *testing.T) {
	canonical, ok := lookupCommand("what-if")
	require.True(t, ok)
	alias, ok := lookupCommand("whatif")
	require.True(t, ok)
	assert.Same(t, canonical, alias)
}

// A value flag's argument must never be mistaken for a task ID. This is the
// failure the five parallel tables made possible: a flag added to the
// validation table but not to the scanning table would have its value parsed
// as a positional ID.
func TestParseIDsSkipsEveryDeclaredValueFlag(t *testing.T) {
	for _, cmd := range commandRegistry() {
		values := cmd.valueFlags()
		if len(values) == 0 {
			continue
		}
		t.Run(cmd.Name, func(t *testing.T) {
			args := []string{"7"}
			for name := range values {
				// A numeric value is the case that would be misread.
				args = append(args, name, "42")
			}
			ids := parseIDsWithValueFlags(args, values)
			assert.Equal(t, []uint64{7}, ids,
				"a flag value was parsed as a task ID")
		})
	}
}

func TestParseIDsDeduplicatesAndIgnoresNonPositive(t *testing.T) {
	ids := parseIDsWithValueFlags([]string{"3", "3", "0", "-1", "not-an-id", "5"}, nil)
	assert.Equal(t, []uint64{3, 5}, ids)
}

func TestUsageListsEveryVisibleCommand(t *testing.T) {
	usage := renderUsage()
	for _, cmd := range commandRegistry() {
		if cmd.Hidden {
			continue
		}
		assert.Contains(t, usage, cmd.Usage, "%s is missing from help", cmd.Name)
		assert.Contains(t, usage, cmd.Summary, "%s summary is missing from help", cmd.Name)
	}
}

func TestCommandHelpListsEveryFlag(t *testing.T) {
	for _, cmd := range commandRegistry() {
		help := renderCommandHelp(cmd)
		for _, flag := range cmd.Flags {
			assert.Contains(t, help, flag.Name, "%s help omits %s", cmd.Name, flag.Name)
			assert.Contains(t, help, flag.Help, "%s help omits the description of %s", cmd.Name, flag.Name)
		}
	}
}

// Commands that must work before a store exists have to say so. Getting this
// wrong makes onboarding fail with "not in a project" at exactly the moment a
// user is trying to create one.
func TestCommandsUsableBeforeInitializationAreDeclared(t *testing.T) {
	expected := map[string]bool{
		"init": true, "serve": true, "mcp": true,
		"integrate": true, "conformance": true,
	}
	for _, cmd := range commandRegistry() {
		assert.Equal(t, expected[cmd.Name], cmd.AllowsUninitializedProject,
			"%s: unexpected uninitialized-project policy", cmd.Name)
	}
}

// The protocol reference specifically must cover every command that emits
// machine-readable output, because that is the surface automations bind to.
func TestJSONEmittingCommandsAppearInTheProtocolReference(t *testing.T) {
	reference, err := os.ReadFile("docs/agent-protocol.md")
	require.NoError(t, err)
	documented := string(reference)

	// conformance emits --json but is an evaluation tool rather than a
	// coordination operation. It is specified in docs/conformance.md, and
	// requiring it here would misrepresent the protocol surface.
	elsewhere := map[string]bool{"conformance": true}

	for _, cmd := range commandRegistry() {
		if cmd.Hidden || elsewhere[cmd.Name] {
			continue
		}
		if _, emitsJSON := cmd.accepts("--json"); !emitsJSON {
			continue
		}
		assert.Contains(t, documented, "`"+cmd.Name,
			"%s emits --json but is absent from docs/agent-protocol.md", cmd.Name)
	}
}
