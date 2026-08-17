// Package integrations embeds the agent integration assets that `todo
// integrate` installs.
//
// The assets live at the repository root rather than beside the CLI because
// README.md and CONTRIBUTING.md document this path as the place to copy a
// skill from by hand. go:embed cannot reach outside its own package
// directory, so the embed lives here and the CLI imports it.
package integrations

import _ "embed"

// Skill is the canonical terminal-todo agent skill.
//
//go:embed skills/terminal-todo/SKILL.md
var Skill []byte

// OpenAIAgentMetadata is the Codex agent descriptor that accompanies the skill.
//
//go:embed skills/terminal-todo/agents/openai.yaml
var OpenAIAgentMetadata []byte
