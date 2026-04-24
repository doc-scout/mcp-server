// Copyright 2026 Leonan Carvalho
// SPDX-License-Identifier: AGPL-3.0-only

package mcp

import "strings"

// KnownServerRegistry maps lowercase MCP server names to their tool observation strings.
// Matched servers get these observations merged in addition to transport/command metadata.
type KnownServerRegistry map[string][]string

// DefaultKnownServers returns the built-in registry of well-known public MCP servers.
func DefaultKnownServers() KnownServerRegistry {
	return KnownServerRegistry{
		"github": {
			"tool:search_repositories: Search GitHub repositories by query",
			"tool:get_file_contents: Read the contents of a file from a GitHub repo",
			"tool:create_issue: Create a new GitHub issue",
			"tool:list_pull_requests: List pull requests for a repository",
		},
		"filesystem": {
			"tool:read_file: Read the contents of a file",
			"tool:write_file: Write content to a file",
			"tool:list_directory: List the contents of a directory",
			"tool:delete_file: Delete a file",
		},
		"postgres": {
			"tool:query: Execute a read-only SQL query",
			"tool:list_tables: List all tables in the database",
			"tool:describe_table: Describe the schema of a specific table",
		},
		"fetch": {
			"tool:fetch: Fetch a URL and return its content as text or markdown",
		},
		"brave-search": {
			"tool:brave_web_search: Run a web search using the Brave Search API",
		},
		"slack": {
			"tool:send_message: Send a message to a Slack channel",
			"tool:list_channels: List available Slack channels",
			"tool:get_channel_history: Retrieve message history from a channel",
		},
	}
}

// Lookup returns the tool observations for a given server name (case-insensitive).
// Returns nil if the server is not in the registry.
func (r KnownServerRegistry) Lookup(name string) []string {
	return r[strings.ToLower(name)]
}
