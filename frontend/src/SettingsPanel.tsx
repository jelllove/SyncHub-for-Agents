import { useEffect, useState } from 'react'
import { StartAtLogin } from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice'
import type {
  CustomResourceInput,
  SettingsInput,
} from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import type { AppSnapshot } from './desktopState'
import { hasGeneratedPreview } from './desktopState'
import { CustomResourceEditor } from './resources/CustomResourceEditor'
import { ResourceSettings, type CategorySettings } from './resources/ResourceSettings'
import { RestorePreview } from './resources/RestorePreview'
import { UpdatePanel } from './UpdatePanel'

export type SettingsPanelProps = {
  snapshot: AppSnapshot
  busy: boolean
  previewLoading: boolean
  close: () => void
  refreshPreview: () => Promise<void>
  runSyncNow: () => Promise<unknown>
  save: (
    input: SettingsInput,
    startAtLogin: boolean,
    startAtLoginChanged: boolean,
  ) => Promise<unknown>
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

function previewTimestamp(value: string) {
  if (!hasGeneratedPreview(value)) return 'No preview generated yet'
  return `Last refreshed ${new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))}`
}

export function SettingsPanel({
  snapshot,
  busy,
  previewLoading,
  close,
  refreshPreview,
  runSyncNow,
  save,
}: SettingsPanelProps) {
  const [repositoryUrl, setRepositoryUrl] = useState(snapshot.repositoryUrl)
  const [intervalMinutes, setIntervalMinutes] = useState(snapshot.intervalMinutes)
  const [trashGraceDays, setTrashGraceDays] = useState(snapshot.trashGraceDays)
  const [repositoryDir, setRepositoryDir] = useState(snapshot.repoPath)
  const [repoPathMode, setRepoPathMode] = useState<'reclone' | 'migrate'>('reclone')
  const [agents, setAgents] = useState<Record<string, boolean>>(
    Object.fromEntries(snapshot.agents.map((agent) => [agent.name, agent.enabled])),
  )
  const [categories, setCategories] = useState<CategorySettings>(
    Object.fromEntries(snapshot.agents.map((agent) => [
      agent.name,
      Object.fromEntries(agent.resources.map((resource) => [resource.category, resource.enabled])),
    ])),
  )
  const [customResources, setCustomResources] = useState<CustomResourceInput[]>(snapshot.customResources)
  const [startAtLogin, setStartAtLogin] = useState(false)
  const [initialStartAtLogin, setInitialStartAtLogin] = useState(false)
  const [startAtLoginError, setStartAtLoginError] = useState('')

  useEffect(() => {
    void StartAtLogin().then((enabled) => {
      setStartAtLogin(enabled)
      setInitialStartAtLogin(enabled)
    }).catch((cause) => setStartAtLoginError(errorMessage(cause)))
  }, [])

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    const saved = await save(
      {
        repositoryUrl,
        repositoryDir,
        repoPathMode,
        intervalMinutes,
        trashGraceDays,
        agents,
        categories,
        customResources,
      },
      startAtLogin,
      startAtLogin !== initialStartAtLogin,
    )
    if (saved) close()
  }

  const title = snapshot.configured ? 'Sync settings' : 'Connect your repository'

  return (
    <div className="drawer-backdrop" onMouseDown={close}>
      <aside
        aria-labelledby="settings-title"
        aria-modal="true"
        className="drawer"
        onMouseDown={(event) => event.stopPropagation()}
        role="dialog"
      >
        <div className="drawer-title">
          <div>
            <span className="eyebrow">{snapshot.configured ? 'PREFERENCES' : 'GET STARTED'}</span>
            <h2 id="settings-title">{title}</h2>
          </div>
          <button className="icon-button" onClick={close} aria-label="Close settings">×</button>
        </div>
        <form onSubmit={(event) => void submit(event)}>
          <label>
            Private Git repository
            <input
              required
              value={repositoryUrl}
              onChange={(event) => setRepositoryUrl(event.target.value)}
              placeholder="git@github.com:your-name/agent-sync.git"
            />
            <small>SSH and HTTPS repositories are supported.</small>
          </label>
          <label>
            Local repository directory
            <input
              required
              value={repositoryDir}
              onChange={(event) => setRepositoryDir(event.target.value)}
              placeholder={snapshot.repoPath}
            />
            <small>Where SyncHub keeps the local clone.</small>
          </label>
          <fieldset>
            <legend>When directory changes</legend>
            <label className="toggle-row">
              <span>
                <strong>Keep old folder, clone into new</strong>
              </span>
              <input
                type="radio"
                checked={repoPathMode === 'reclone'}
                onChange={() => setRepoPathMode('reclone')}
              />
            </label>
            <label className="toggle-row">
              <span>
                <strong>Move existing repo to new folder</strong>
              </span>
              <input
                type="radio"
                checked={repoPathMode === 'migrate'}
                onChange={() => setRepoPathMode('migrate')}
              />
            </label>
          </fieldset>
          <div className="field-grid">
            <label>
              Sync frequency (minutes)
              <input
                required
                type="number"
                min={1}
                max={1440}
                step={1}
                value={intervalMinutes}
                onChange={(event) => setIntervalMinutes(Number(event.target.value))}
              />
              <small>Runs every 1–1440 minutes.</small>
            </label>
            <label>
              Archive retention (days)
              <input
                required
                type="number"
                min={1}
                max={365}
                step={1}
                value={trashGraceDays}
                onChange={(event) => setTrashGraceDays(Number(event.target.value))}
              />
              <small>Deleted files remain recoverable for 1–365 days.</small>
            </label>
          </div>
          <fieldset>
            <legend>Agents to synchronize</legend>
            {snapshot.agents.map((agent) => (
              <label className="toggle-row" key={agent.name}>
                <span>
                  <strong>{agent.name}</strong>
                  <small>{agent.exclude.length} safety exclusions</small>
                </span>
                <input
                  type="checkbox"
                  checked={agents[agent.name] ?? false}
                  onChange={(event) => setAgents({ ...agents, [agent.name]: event.target.checked })}
                />
              </label>
            ))}
          </fieldset>
          <fieldset>
            <legend>Resource categories</legend>
            <ResourceSettings agents={snapshot.agents} categories={categories} onChange={setCategories} />
          </fieldset>
          <section className="preview-refresh" aria-live="polite">
            <div>
              <strong>Resource preview</strong>
              <small>{previewTimestamp(snapshot.preview.generatedAt)}</small>
            </div>
            <button
              className="secondary"
              disabled={previewLoading || !snapshot.configured}
              onClick={() => void refreshPreview()}
              type="button"
            >
              {previewLoading ? 'Refreshing…' : 'Refresh preview'}
            </button>
          </section>
          <section className="preview-refresh">
            <div>
              <strong>Synchronization</strong>
              <small>Run an immediate one-time synchronization now.</small>
            </div>
            <button
              className="secondary"
              disabled={busy || !snapshot.configured}
              onClick={() => void runSyncNow()}
              type="button"
            >
              Run sync now
            </button>
          </section>
          <RestorePreview preview={snapshot.preview} />
          <CustomResourceEditor
            platform={snapshot.platform}
            resources={customResources}
            onChange={setCustomResources}
          />
          <fieldset>
            <legend>Desktop application</legend>
            <label className="toggle-row">
              <span>
                <strong>Start at login</strong>
                <small>Starts SyncHub for Agents the next time you sign in. It does not restart the app now.</small>
              </span>
              <input
                type="checkbox"
                checked={startAtLogin}
                onChange={(event) => setStartAtLogin(event.target.checked)}
              />
            </label>
            {startAtLoginError && <div className="inline-error">{startAtLoginError}</div>}
          </fieldset>
          <div className="form-actions">
            <button type="button" className="secondary" onClick={close}>Cancel</button>
            <button type="submit" className="primary" disabled={busy}>
              {snapshot.configured ? 'Save settings' : 'Continue'}
            </button>
          </div>
        </form>
        <UpdatePanel syncBusy={busy || snapshot.state === 'updating'} />
      </aside>
    </div>
  )
}
