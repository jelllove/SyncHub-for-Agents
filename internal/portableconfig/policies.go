package portableconfig

func BuiltinRegistry() *Registry {
	registry := NewRegistry()
	registry.Register("claude-settings", Policy{
		Portable:     []string{"**"},
		Sensitive:    []string{"**.*token*", "**.*secret*", "**.*password*", "**.*oauth*"},
		MachineLocal: []string{"installationId", "recentWorkspaces", "cache.**"},
		PathFields:   []string{"**.*Path", "**.*Directory"},
	})
	registry.Register("copilot-settings", Policy{
		Portable:     []string{"**"},
		Sensitive:    []string{"**.*token*", "**.*secret*", "**.*password*", "**.*oauth*", "hosts.**"},
		MachineLocal: []string{"installationId", "recentRepositories", "cache.**", "session.**"},
		PathFields:   []string{"**.*Path", "**.*Directory"},
	})
	registry.Register("gemini-settings", Policy{
		Portable:     []string{"theme", "model", "tools.**", "mcpServers.**", "context.**"},
		Sensitive:    []string{"**.*token*", "**.*secret*", "**.*password*", "**.*oauth*", "accounts.**"},
		MachineLocal: []string{"installationId", "trustedFolders", "recentProjects", "state.**"},
		PathFields:   []string{"context.fileName"},
	})
	registry.Register("vscode-settings", Policy{
		Portable:     []string{"**"},
		Sensitive:    []string{"**.*token*", "**.*secret*", "**.*password*", "**.*oauth*"},
		MachineLocal: []string{"window.restoreWindows", "workbench.localHistory.**", "recentlyOpened.**"},
		PathFields:   []string{"**.*Path", "**.*Directory"},
	})
	registry.Register("vscode-mcp", Policy{
		Portable:   []string{"servers.**.type", "servers.**.command", "servers.**.args", "servers.**.url"},
		Sensitive:  []string{"servers.**.env.**", "servers.**.headers.**", "**.*token*", "**.*secret*", "**.*password*"},
		PathFields: []string{"servers.**.command", "servers.**.args"},
	})
	registry.Register("cursor-settings", Policy{
		Portable:     []string{"**"},
		Sensitive:    []string{"**.*token*", "**.*secret*", "**.*password*", "**.*oauth*"},
		MachineLocal: []string{"installationId", "recentWorkspaces", "cache.**"},
		PathFields:   []string{"**.*Path", "**.*Directory"},
	})
	registry.Register("common-skill-lock", Policy{
		Portable:  []string{"skills.**.name", "skills.**.source", "skills.**.version", "skills.**.revision"},
		Sensitive: []string{"**.*token*", "**.*secret*", "**.*password*", "**.*oauth*"},
	})
	return registry
}
