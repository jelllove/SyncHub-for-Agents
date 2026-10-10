import { useEffect, useId, useRef, useState } from 'react'
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
import { ResetPanel } from './ResetPanel'

const settingsTabs = [
  { id: 'repository', label: 'Repository' },
  { id: 'sync', label: 'Sync' },
  { id: 'resources', label: 'Resources' },
  { id: 'desktop', label: 'Desktop' },
  { id: 'advanced', label: 'Advanced' },
] as const

type SettingsTab = typeof settingsTabs[number]['id']

export type SettingsPanelProps = {
  snapshot: AppSnapshot
  busy: boolean
  previewLoading: boolean
  close: () => void
  refreshPreview: () => Promise<void>
  runSyncNow: () => Promise<unknown>
  resetComplete?: () => void
  resetBusyChanged?: (working: boolean) => void
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
  resetComplete,
  resetBusyChanged,
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
  const [startAtLoginLoading, setStartAtLoginLoading] = useState(true)
  const [startupRevision, setStartupRevision] = useState(0)
  const [activeTab, setActiveTab] = useState<SettingsTab>(snapshot.configured ? 'sync' : 'repository')
  const settingsId = useId()
  const formId = `${settingsId}-form`
  const tabButtons = useRef<(HTMLButtonElement | null)[]>([])
  const invalidSetting = useRef<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement | null>(null)

  useEffect(() => {
    let active = true
    setStartAtLoginLoading(true)
    setStartAtLoginError('')
    void StartAtLogin().then((enabled) => {
      if (!active) return
      setStartAtLogin(enabled)
      setInitialStartAtLogin(enabled)
    }).catch((cause) => {
      if (active) setStartAtLoginError(errorMessage(cause))
    }).finally(() => {
      if (active) setStartAtLoginLoading(false)
    })
    return () => { active = false }
  }, [startupRevision])

  useEffect(() => {
    if (invalidSetting.current) {
      invalidSetting.current.focus()
      invalidSetting.current.reportValidity()
      invalidSetting.current = null
    }
  }, [activeTab])

  const navigateTab = (event: React.KeyboardEvent<HTMLButtonElement>, index: number) => {
    let next: number
    switch (event.key) {
      case 'ArrowRight': next = (index + 1) % settingsTabs.length; break
      case 'ArrowLeft': next = (index - 1 + settingsTabs.length) % settingsTabs.length; break
      case 'Home': next = 0; break
      case 'End': next = settingsTabs.length - 1; break
      default: return
    }
    event.preventDefault()
    setActiveTab(settingsTabs[next].id)
    tabButtons.current[next]?.focus()
  }

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const invalid = event.currentTarget.querySelector<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>(
      'input:invalid, select:invalid, textarea:invalid',
    )
    if (invalid) {
      const panel = invalid.closest<HTMLElement>('[data-settings-tab]')
      const tab = settingsTabs.find((candidate) => candidate.id === panel?.dataset.settingsTab)
      if (tab && tab.id !== activeTab) {
        invalidSetting.current = invalid
        setActiveTab(tab.id)
      } else {
        invalid.focus()
        invalid.reportValidity()
      }
      return
    }
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
        className="drawer settings-drawer"
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
        <div className="settings-tabs" role="tablist" aria-label="Settings categories">
          {settingsTabs.map((tab, index) => (
            <button
              aria-controls={`${settingsId}-${tab.id}-panel`}
              aria-description={tab.id === 'desktop' && startAtLoginError ? 'Startup needs attention' : undefined}
              aria-label={tab.label}
              aria-selected={activeTab === tab.id}
              id={`${settingsId}-${tab.id}-tab`}
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              onKeyDown={(event) => navigateTab(event, index)}
              ref={(button) => { tabButtons.current[index] = button }}
              role="tab"
              tabIndex={activeTab === tab.id ? 0 : -1}
              type="button"
            >
              {tab.label}
              {tab.id === 'desktop' && startAtLoginError && <span className="settings-tab-error" aria-label="Startup needs attention">!</span>}
            </button>
          ))}
        </div>
        <div className="settings-content">
        <form id={formId} noValidate onSubmit={(event) => void submit(event)}>
          <section
            aria-labelledby={`${settingsId}-repository-tab`}
            className="settings-panel"
            data-settings-tab="repository"
            hidden={activeTab !== 'repository'}
            id={`${settingsId}-repository-panel`}
            role="tabpanel"
            tabIndex={0}
          >
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
          </section>
          <section
            aria-labelledby={`${settingsId}-sync-tab`}
            className="settings-panel"
            data-settings-tab="sync"
            hidden={activeTab !== 'sync'}
            id={`${settingsId}-sync-panel`}
            role="tabpanel"
            tabIndex={0}
          >
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
          </section>
          <section
            aria-labelledby={`${settingsId}-resources-tab`}
            className="settings-panel"
            data-settings-tab="resources"
            hidden={activeTab !== 'resources'}
            id={`${settingsId}-resources-panel`}
            role="tabpanel"
            tabIndex={0}
          >
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
          <RestorePreview preview={snapshot.preview} />
          <CustomResourceEditor
            platform={snapshot.platform}
            resources={customResources}
            onChange={setCustomResources}
          />
          </section>
        </form>
        <section
          aria-labelledby={`${settingsId}-desktop-tab`}
          className="settings-panel"
          hidden={activeTab !== 'desktop'}
          id={`${settingsId}-desktop-panel`}
          role="tabpanel"
          tabIndex={0}
        >
          <fieldset>
            <legend>Desktop application</legend>
            <label className="toggle-row">
              <span>
                <strong>{snapshot.platform === 'windows' ? 'Start with Windows' : 'Start at login'}</strong>
                <small>
                  {snapshot.platform === 'windows'
                    ? 'Enabled by default in Windows release builds. Starts in the tray when you sign in; you can turn it off here.'
                    : 'Starts SyncHub for Agents the next time you sign in. It does not restart the app now.'}
                </small>
              </span>
              <input
                type="checkbox"
                checked={startAtLogin}
                disabled={startAtLoginLoading || !!startAtLoginError}
                onChange={(event) => setStartAtLogin(event.target.checked)}
              />
            </label>
            {startAtLoginLoading && <small role="status">Checking startup registration…</small>}
            {startAtLoginError && (
              <>
                <div className="inline-error" role="alert">{startAtLoginError}</div>
                <button className="secondary" onClick={() => setStartupRevision((revision) => revision + 1)} type="button">
                  Retry startup status
                </button>
              </>
            )}
          </fieldset>
          <UpdatePanel syncBusy={busy || snapshot.state === 'updating'} />
        </section>
        <section
          aria-labelledby={`${settingsId}-advanced-tab`}
          className="settings-panel"
          hidden={activeTab !== 'advanced'}
          id={`${settingsId}-advanced-panel`}
          role="tabpanel"
          tabIndex={0}
        >
          <ResetPanel busy={busy || previewLoading || snapshot.state === 'updating'} complete={resetComplete ?? close} workingChanged={resetBusyChanged} />
        </section>
        </div>
        <div className="form-actions settings-actions">
          <button type="button" className="secondary" onClick={close}>Cancel</button>
          <button type="submit" form={formId} className="primary" disabled={busy}>
            {snapshot.configured ? 'Save settings' : 'Continue'}
          </button>
        </div>
      </aside>
    </div>
  )
}
