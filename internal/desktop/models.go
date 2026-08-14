// Package desktop exposes frontend-safe application state and operations.
package desktop

import "time"

// Agent is one provider shown in the desktop application.
type Agent struct {
	Name    string   `json:"name"`
	Enabled bool     `json:"enabled"`
	Exclude []string `json:"exclude"`
}

type Progress struct {
	Stage            string `json:"stage"`
	Label            string `json:"label"`
	Percentage       int    `json:"percentage"`
	CompletedActions int    `json:"completedActions"`
	TotalActions     int    `json:"totalActions"`
	BlockedFiles     int    `json:"blockedFiles"`
	Pushed           bool   `json:"pushed"`
	Restored         int    `json:"restored"`
	Reinstalled      int    `json:"reinstalled"`
	Skipped          int    `json:"skipped"`
	Conflicts        int    `json:"conflicts"`
	PendingInstalls  int    `json:"pendingInstalls"`
	NeedsAttention   bool   `json:"needsAttention"`
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
	Progress        Progress  `json:"progress"`
}

// SettingsInput contains editable desktop settings.
type SettingsInput struct {
	RepositoryURL   string          `json:"repositoryUrl"`
	IntervalMinutes int             `json:"intervalMinutes"`
	TrashGraceDays  int             `json:"trashGraceDays"`
	Agents          map[string]bool `json:"agents"`
}
