import { useEffect, useMemo, useState } from 'react'
import { Events } from '@wailsio/runtime'
import {
  ApproveInstallPlan,
  Pause,
  NeedsOnboarding,
  ResolveConflict,
  Resume,
  SaveSettings,
  SetStartAtLogin,
  Snapshot as loadSnapshot,
  StartAtLogin,
  TriggerSync,
} from '../bindings/github.com/qinqingxu/acsync/internal/desktop/wailsservice'
import {
  type Agent,
  type CustomResourceInput,
  type Progress,
  type ResourceCategory,
  type ResourceIssue,
  type SettingsInput,
  type Snapshot,
} from '../bindings/github.com/qinqingxu/acsync/internal/desktop/models'
import './style.css'
import { BrandMark } from './BrandMark'
import Onboarding from './onboarding/Onboarding'
import { ConflictPanel } from './resources/ConflictPanel'
import { CustomResourceEditor } from './resources/CustomResourceEditor'
import { InstallPlanPanel } from './resources/InstallPlanPanel'
import { ResourceSettings, type CategorySettings } from './resources/ResourceSettings'
import { RestorePreview } from './resources/RestorePreview'
import { ResultSummary } from './resources/ResultSummary'

type AppAgent = Omit<Agent, 'exclude' | 'resources'> & {
  exclude: string[]
  resources: ResourceCategory[]
}
type AppPreview = Omit<Snapshot['preview'], 'resources' | 'issues'> & {
  resources: ResourceCategory[]
  issues: ResourceIssue[]
}
type AppSnapshot = Omit<Snapshot, 'agents' | 'preview' | 'conflicts' | 'customResources'> & {
  agents: AppAgent[]
  preview: AppPreview
  conflicts: NonNullable<Snapshot['conflicts']>
  customResources: CustomResourceInput[]
}

const stateLabels: Record<string, string> = {
  idle: 'Ready',
  updating: 'Updating',
  done: 'Sync complete',
  paused: 'Paused',
  error: 'Needs attention',
}

const progressStages = ['pulling', 'scanning', 'comparing', 'applying', 'uploading']

function formatTime(value: string) {
  const date = new Date(value)
  if (!value || Number.isNaN(date.getTime()) || date.getUTCFullYear() <= 1) return 'Never'
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date)
}

function formatSyncFrequency(minutes: number) {
  return `Every ${minutes} ${minutes === 1 ? 'minute' : 'minutes'}`
}

function formatArchiveRetention(days: number) {
  return `${days} ${days === 1 ? 'day' : 'days'}`
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

function normalizeSnapshot(snapshot: Snapshot): AppSnapshot {
  return {
    ...snapshot,
    agents: (snapshot.agents ?? []).map((agent) => ({
      ...agent,
      exclude: agent.exclude ?? [],
      resources: agent.resources ?? [],
    })),
    preview: {
      ...snapshot.preview,
      resources: snapshot.preview.resources ?? [],
      issues: snapshot.preview.issues ?? [],
    },
    conflicts: snapshot.conflicts ?? [],
    customResources: snapshot.customResources ?? [],
  }
}

function completionMessage(progress: Progress) {
  if (progress.needsAttention) return 'Synchronization completed with items that need attention'
  if (progress.restored + progress.reinstalled > 0) {
    return `Restored ${progress.restored} files and reinstalled ${progress.reinstalled} integrations`
  }
  if (progress.pushed) return `Uploaded ${progress.totalActions} change${progress.totalActions === 1 ? '' : 's'}`
  return 'Synchronization complete; no changes needed'
}

function App() {
  const [snapshot, setSnapshot] = useState<AppSnapshot>()
  const [needsOnboarding, setNeedsOnboarding] = useState<boolean>()
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')

  const refresh = async () => {
    try {
      setSnapshot(normalizeSnapshot(await loadSnapshot()))
      setError('')
    } catch (cause) {
      setError(errorMessage(cause))
    }
  }

  useEffect(() => {
    void NeedsOnboarding()
      .then((needed) => {
        setNeedsOnboarding(needed)
        if (!needed) void refresh()
      })
      .catch((cause) => setError(errorMessage(cause)))
    const unsubscribeSnapshot = Events.On('desktop:snapshot', (event) => {
      setSnapshot(normalizeSnapshot(event.data as Snapshot))
    })
    const unsubscribeProgress = Events.On('desktop:progress', (event) => {
      const progress = event.data as Progress
      setSnapshot((current) => current ? {
        ...current,
        progress,
        state: progress.stage === 'complete' ? current.state : 'updating',
      } : current)
      if (progress.stage === 'complete') setNotice(completionMessage(progress))
    })
    return () => {
      unsubscribeSnapshot()
      unsubscribeProgress()
    }
  }, [])

  const enabledAgents = useMemo(
    () => snapshot?.agents.filter((agent) => agent.enabled).length ?? 0,
    [snapshot],
  )

  const perform = async (action: () => Promise<void>, success = '') => {
    setBusy(true)
    setNotice('')
    setError('')
    try {
      await action()
      if (success) setNotice(success)
      await refresh()
      return true
    } catch (cause) {
      setError(errorMessage(cause))
      return false
    } finally {
      setBusy(false)
    }
  }

  if (needsOnboarding) {
    return <Onboarding complete={() => {
      setNeedsOnboarding(false)
      void refresh()
    }} />
  }

  if (needsOnboarding === undefined || !snapshot) {
    return (
      <main className="loading">
        <BrandMark label="AgentConfigSync" />
        <p>{error || 'Opening AgentConfigSync…'}</p>
      </main>
    )
  }

  const state = snapshot.state || 'idle'
  const paused = state === 'paused'
  const progress = snapshot.progress
  const statusMessage = state === 'updating'
    ? `${progress.label || 'Preparing synchronization'}${progress.totalActions > 0 ? ` · ${progress.completedActions} of ${progress.totalActions} changes` : ''}`
    : progress.stage === 'complete'
      ? completionMessage(progress)
      : `${enabledAgents} agents are protected across your connected computers.`

  return (
    <div className="app-shell">
      <header>
        <div className="brand">
          <BrandMark label="AgentConfigSync" />
          <div>
            <strong>AgentConfigSync</strong>
            <span>Desktop</span>
          </div>
        </div>
        <button className="icon-button" onClick={() => setSettingsOpen(true)} aria-label="Open settings">
          <SettingsIcon />
        </button>
      </header>

      <main>
        {!snapshot.configured ? (
          <section className="welcome">
            <span className="eyebrow">WELCOME</span>
            <h1>Keep every AI workspace in sync.</h1>
            <p>
              Connect a private Git repository once, then AgentConfigSync will safely synchronize
              your agent settings and sessions in the background.
            </p>
            <button className="primary" onClick={() => setSettingsOpen(true)}>
              Set up synchronization
            </button>
            <div className="welcome-agents">
              {snapshot.agents.map((agent) => <span key={agent.name}>{agent.name}</span>)}
            </div>
          </section>
        ) : (
          <>
            <section className={`hero state-${state}`}>
              <StatusMark state={state} />
              <div className="hero-copy">
                <span className="eyebrow">SYNC STATUS</span>
                <h1>{stateLabels[state] ?? state}</h1>
                <p>
                  {snapshot.lastError || statusMessage}
                </p>
              </div>
              <div className="hero-actions">
                <button
                  className="primary"
                  disabled={busy || state === 'updating'}
                  onClick={() => void perform(TriggerSync, 'Synchronization queued')}
                >
                  <SyncIcon /> {state === 'updating' ? 'Syncing…' : 'Sync now'}
                </button>
                <button
                  className="secondary"
                  disabled={busy}
                  onClick={() => void perform(paused ? Resume : Pause)}
                >
                  {paused ? 'Resume' : 'Pause'}
                </button>
              </div>
              {state === 'updating' && <SyncProgress progress={progress} />}
            </section>

            <section className="metrics" aria-label="Synchronization details">
              <Metric label="Last sync" value={formatTime(snapshot.lastSync)} />
              <Metric label="Next sync" value={paused ? 'Paused' : formatTime(snapshot.nextSync)} />
              <Metric label="Sync frequency" value={formatSyncFrequency(snapshot.intervalMinutes)} />
              <Metric label="Archive retention" value={formatArchiveRetention(snapshot.trashGraceDays)} />
            </section>

            <ResultSummary progress={progress} />

            {snapshot.pendingInstallPlan && (
              <InstallPlanPanel
                plan={snapshot.pendingInstallPlan}
                busy={busy}
                approve={(id) => perform(
                  () => ApproveInstallPlan(id),
                  'Install plan approved; synchronization queued',
                )}
              />
            )}

            {snapshot.conflicts.length > 0 && (
              <ConflictPanel
                conflicts={snapshot.conflicts}
                busy={busy}
                resolve={(input) => perform(
                  () => ResolveConflict(input),
                  'Conflict resolved; synchronization queued',
                )}
              />
            )}

            <section className="panel">
              <div className="panel-heading">
                <div>
                  <span className="eyebrow">CONNECTED AGENTS</span>
                  <h2>Your workspaces</h2>
                </div>
                <button className="text-button" onClick={() => setSettingsOpen(true)}>Manage</button>
              </div>
              <div className="agent-list">
                {snapshot.agents.map((agent) => (
                  <div className="agent-row" key={agent.name}>
                    <div className="agent-avatar">{agent.name.slice(0, 1).toUpperCase()}</div>
                    <div>
                      <strong>{agent.name}</strong>
                      <span>{agent.enabled ? 'Included in synchronization' : 'Not synchronized'}</span>
                    </div>
                    <span className={agent.enabled ? 'agent-state active' : 'agent-state'} />
                  </div>
                ))}
              </div>
            </section>
          </>
        )}

        {(notice || error) && (
          <div className={error ? 'toast error' : 'toast'} role="status">
            {error || notice}
          </div>
        )}
      </main>

      {settingsOpen && (
        <SettingsPanel
          snapshot={snapshot}
          busy={busy}
          close={() => setSettingsOpen(false)}
          save={(input, startAtLogin, startAtLoginChanged) => perform(async () => {
            await SaveSettings(input)
            if (startAtLoginChanged) await SetStartAtLogin(startAtLogin)
          }, 'Settings saved')}
        />
      )}
    </div>
  )
}

function SyncProgress({ progress }: { progress: Progress }) {
  const activeIndex = progressStages.indexOf(progress.stage)
  return (
    <div className="sync-progress" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={progress.percentage}>
      <div className="sync-progress-heading">
        <strong>{progress.label || 'Preparing synchronization'}</strong>
        <span>{progress.percentage}%</span>
      </div>
      <div className="sync-progress-track"><span style={{ width: `${progress.percentage}%` }} /></div>
      <div className="sync-progress-stages">
        {progressStages.map((stage, index) => (
          <span className={index < activeIndex ? 'done' : index === activeIndex ? 'active' : ''} key={stage}>
            {stage}
          </span>
        ))}
      </div>
    </div>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <article>
      <span>{label}</span>
      <strong>{value}</strong>
    </article>
  )
}

function StatusMark({ state }: { state: string }) {
  return (
    <div className="status-orb" aria-hidden="true">
      <svg viewBox="0 0 32 32">
        {state === 'updating' && (
          <>
            <path d="M7 15a9 9 0 0 1 15-6M25 17a9 9 0 0 1-15 6" />
            <path d="m20 6 4 3-4 3M12 26l-4-3 4-3" />
          </>
        )}
        {state === 'done' && <path d="m9 16 5 5 10-11" />}
        {state === 'error' && (
          <>
            <path d="M16 8v10" />
            <circle cx="16" cy="23" r="1.5" />
          </>
        )}
        {state === 'paused' && (
          <>
            <path d="M12 9v14" />
            <path d="M20 9v14" />
          </>
        )}
        {!['updating', 'done', 'error', 'paused'].includes(state) && (
          <>
            <path d="m16 9 7 4v8l-7 4-7-4v-8Z" />
            <circle cx="16" cy="17" r="2.5" />
          </>
        )}
      </svg>
    </div>
  )
}

function SettingsPanel({
  snapshot,
  busy,
  close,
  save,
}: {
  snapshot: AppSnapshot
  busy: boolean
  close: () => void
  save: (
    input: SettingsInput,
    startAtLogin: boolean,
    startAtLoginChanged: boolean,
  ) => Promise<boolean>
}) {
  const [repositoryUrl, setRepositoryUrl] = useState(snapshot.repositoryUrl)
  const [intervalMinutes, setIntervalMinutes] = useState(snapshot.intervalMinutes)
  const [trashGraceDays, setTrashGraceDays] = useState(snapshot.trashGraceDays)
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
      { repositoryUrl, intervalMinutes, trashGraceDays, agents, categories, customResources },
      startAtLogin,
      startAtLogin !== initialStartAtLogin,
    )
    if (saved) close()
  }

  return (
    <div className="drawer-backdrop" onMouseDown={close}>
      <aside className="drawer" onMouseDown={(event) => event.stopPropagation()}>
        <div className="drawer-title">
          <div>
            <span className="eyebrow">{snapshot.configured ? 'PREFERENCES' : 'GET STARTED'}</span>
            <h2>{snapshot.configured ? 'Sync settings' : 'Connect your repository'}</h2>
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
                <small>Starts AgentConfigSync the next time you sign in. It does not restart the app now.</small>
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
      </aside>
    </div>
  )
}

function SettingsIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 15.4a3.4 3.4 0 1 0 0-6.8 3.4 3.4 0 0 0 0 6.8Zm7.1-2.5c.1-.6.1-1.2 0-1.8l2-1.5-2-3.4-2.5 1a8 8 0 0 0-1.5-.9L14.8 3h-4l-.4 3.3c-.5.2-1 .5-1.5.9l-2.5-1-2 3.4 2 1.5a7 7 0 0 0 0 1.8l-2 1.5 2 3.4 2.5-1c.5.4 1 .7 1.5.9l.4 3.3h4l.4-3.3c.5-.2 1-.5 1.5-.9l2.5 1 2-3.4-2.1-1.5Z" /></svg>
}

function SyncIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5m9.6-2A7 7 0 0 0 6.3 7.7L4 12m16 0-2.3 4.3A7 7 0 0 1 5.4 14" /></svg>
}

export default App
