// Package desktop exposes frontend-safe application state and operations.
package desktop

import "time"

// Agent is one provider shown in the desktop application.
type Agent struct {
	Name      string             `json:"name"`
	Enabled   bool               `json:"enabled"`
	Exclude   []string           `json:"exclude"`
	Resources []ResourceCategory `json:"resources"`
}

type ResourceCategory struct {
	Provider      string `json:"provider"`
	ID            string `json:"id"`
	Category      string `json:"category"`
	Enabled       bool   `json:"enabled"`
	Supported     bool   `json:"supported"`
	Source        string `json:"source"`
	Target        string `json:"target"`
	FileCount     int    `json:"fileCount"`
	Bytes         int64  `json:"bytes"`
	ExcludedFiles int    `json:"excludedFiles"`
	ExcludedBytes int64  `json:"excludedBytes"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
}

type ResourceIssue struct {
	ResourceKey string `json:"resourceKey"`
	Path        string `json:"path"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	Bytes       int64  `json:"bytes"`
}

type ResourcePreview struct {
	GeneratedAt   time.Time          `json:"generatedAt"`
	Resources     []ResourceCategory `json:"resources"`
	Files         int                `json:"files"`
	Bytes         int64              `json:"bytes"`
	ExcludedFiles int                `json:"excludedFiles"`
	ExcludedBytes int64              `json:"excludedBytes"`
	Issues        []ResourceIssue    `json:"issues"`
}

type InstallOperation struct {
	ID         string   `json:"id"`
	Adapter    string   `json:"adapter"`
	Source     string   `json:"source"`
	Kind       string   `json:"kind"`
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	WorkingDir string   `json:"workingDir"`
	Error      string   `json:"error,omitempty"`
}

type InstallPlan struct {
	ID         string             `json:"id"`
	Approved   bool               `json:"approved"`
	Operations []InstallOperation `json:"operations"`
}

type ConflictSummary struct {
	ID          string    `json:"id"`
	Revision    string    `json:"revision"`
	ResourceKey string    `json:"resourceKey"`
	Path        string    `json:"path"`
	CreatedAt   time.Time `json:"createdAt"`
}

type ConflictResolutionStatus struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Selected int    `json:"selected"`
	Error    string `json:"error,omitempty"`
}

type ConflictResolution struct {
	ID      string `json:"id"`
	Choice  string `json:"choice"`
	Content string `json:"content,omitempty"`
}

type ConflictSelection struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Choice   string `json:"choice"`
	Content  string `json:"content,omitempty"`
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

type SyncFixStep struct {
	Title   string `json:"title"`
	Command string `json:"command"`
	Warning string `json:"warning,omitempty"`
}

type SyncDiagnostic struct {
	Code     string        `json:"code"`
	Summary  string        `json:"summary"`
	RepoPath string        `json:"repoPath"`
	Steps    []SyncFixStep `json:"steps"`
}

// Snapshot is the current desktop-visible sync state.
type Snapshot struct {
	Configured         bool                      `json:"configured"`
	State              string                    `json:"state"`
	RepositoryURL      string                    `json:"repositoryUrl"`
	Platform           string                    `json:"platform"`
	IntervalMinutes    int                       `json:"intervalMinutes"`
	TrashGraceDays     int                       `json:"trashGraceDays"`
	Agents             []Agent                   `json:"agents"`
	LastSync           time.Time                 `json:"lastSync"`
	NextSync           time.Time                 `json:"nextSync"`
	PendingActions     int                       `json:"pendingActions"`
	BlockedFiles       int                       `json:"blockedFiles"`
	LastError          string                    `json:"lastError"`
	RepoPath           string                    `json:"repoPath"`
	FirstSyncRequired  bool                      `json:"firstSyncRequired"`
	SyncDiagnostic     *SyncDiagnostic           `json:"syncDiagnostic,omitempty"`
	Progress           Progress                  `json:"progress"`
	Preview            ResourcePreview           `json:"preview"`
	CustomResources    []CustomResourceInput     `json:"customResources"`
	PendingInstallPlan *InstallPlan              `json:"pendingInstallPlan,omitempty"`
	Conflicts          []ConflictSummary         `json:"conflicts"`
	ConflictResolution *ConflictResolutionStatus `json:"conflictResolution,omitempty"`
}

// SettingsInput contains editable desktop settings.
type SettingsInput struct {
	RepositoryURL           string                     `json:"repositoryUrl"`
	RepositoryDir           string                     `json:"repositoryDir,omitempty"`
	RepoPathMode            string                     `json:"repoPathMode,omitempty"`
	FirstSyncStrategy       string                     `json:"firstSyncStrategy,omitempty"`
	FirstSyncChoiceRequired bool                       `json:"firstSyncChoiceRequired,omitempty"`
	IntervalMinutes         int                        `json:"intervalMinutes"`
	TrashGraceDays          int                        `json:"trashGraceDays"`
	Agents                  map[string]bool            `json:"agents"`
	Categories              map[string]map[string]bool `json:"categories"`
	CustomResources         []CustomResourceInput      `json:"customResources"`
}

type CustomResourceInput struct {
	ID       string            `json:"id"`
	Category string            `json:"category"`
	Paths    map[string]string `json:"paths"`
	Targets  map[string]string `json:"targets"`
	Include  []string          `json:"include"`
	Exclude  []string          `json:"exclude"`
	Strategy string            `json:"strategy"`
}
