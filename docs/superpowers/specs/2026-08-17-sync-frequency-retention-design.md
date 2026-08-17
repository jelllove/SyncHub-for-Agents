# Configurable Sync Frequency and Archive Retention Design

## Goal

Let users configure how often AgentConfigSync runs and how long deleted files remain recoverable, while showing both settings on the main dashboard.

## User Experience

- Sync frequency is a whole number from 1 through 1440 minutes.
- Archive retention is a whole number from 1 through 365 days.
- New configurations default to a 10-minute sync frequency and 30-day archive retention.
- The settings drawer uses numeric inputs rather than fixed presets.
- The main dashboard keeps four metrics:
  - Last sync
  - Next sync
  - Sync frequency, formatted as `Every 10 minutes`
  - Archive retention, formatted as `30 days`
- Sync frequency and archive retention replace the existing Pending changes and Protected agents metrics.

## Architecture and Data Flow

The existing `SyncIntervalMinutes` and `TrashGraceDays` configuration fields remain the source of truth. No configuration migration or schema version change is required because both fields already exist.

When settings are saved:

1. The React settings drawer validates the HTML numeric input bounds.
2. The desktop service validates both values before writing configuration.
3. The saved sync interval is applied immediately to the running scheduler through `SetInterval`.
4. The archive retention value is consumed by the existing trash cleanup behavior on subsequent synchronization cycles.
5. A refreshed desktop snapshot returns both values, and the dashboard formats them for display.

## Validation and Errors

- Sync frequency below 1 or above 1440 returns a clear validation error.
- Archive retention below 1 or above 365 returns a clear validation error.
- Invalid values are not persisted and do not change the running scheduler.
- Existing configurations keep their saved values. Defaults apply only when creating a new configuration.

## Compatibility

- Existing repository and local configuration formats remain unchanged.
- Existing scheduler pause, resume, manual trigger, and Done-state behavior remain unchanged.
- Onboarding continues to create configurations with the 10-minute and 30-day defaults.

## Testing

- Desktop service tests cover all four lower/upper boundary failures and valid boundary values.
- Tests confirm invalid settings leave both the saved configuration and live scheduler interval unchanged.
- Tests confirm valid settings update the persisted values and scheduler interval.
- Frontend production build verifies numeric input types, bounds, snapshot field usage, and dashboard rendering compile correctly.
- Full Go tests and `go vet ./...` provide regression coverage.
