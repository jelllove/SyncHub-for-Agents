// Package desktop exposes frontend-safe application state and operations.
package desktop

import "time"

// Agent is one provider shown in the desktop application.
type Agent struct {
	Name    string   `json:"name"`
	Enabled bool     `json:"enabled"`
	Exclude []string `json:"exclude"`
}

// Snapshot is the current desktop-visible sync state.
type Snapshot struct {
	Configured      bool      `json:"configured"`
	State           string    `json:"state"`
	RepositoryURL   string    `json:"repositoryUrl"`
	IntervalMinutes int       `json:"intervalMinutes"`
	TrashGraceDays  int       `json:"trashGraceDays"`
	Agents          []Agent   `json:"agents"`
	LastSync        time.Time `json:"lastSync"`
	NextSync        time.Time `json:"nextSync"`
	PendingActions  int       `json:"pendingActions"`
	BlockedFiles    int       `json:"blockedFiles"`
	LastError       string    `json:"lastError"`
}

// SettingsInput contains editable desktop settings.
type SettingsInput struct {
	RepositoryURL   string          `json:"repositoryUrl"`
	IntervalMinutes int             `json:"intervalMinutes"`
	TrashGraceDays  int             `json:"trashGraceDays"`
	Agents          map[string]bool `json:"agents"`
}
