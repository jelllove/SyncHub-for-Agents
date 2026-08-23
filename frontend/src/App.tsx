import { useEffect, useMemo, useRef, useState } from 'react'
import { Browser, Events } from '@wailsio/runtime'
import {
  ApproveInstallPlan,
  Pause,
  NeedsOnboarding,
  QueueConflictBatch,
  RetryInstallPlan,
  RetryConflictBatch,
  ResourcePreview,
  Resume,
  SaveSettings,
  SetStartAtLogin,
  Snapshot as loadSnapshot,
  TriggerSync,
} from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice'
import {
  type Progress,
  type Snapshot,
} from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/models'
import './style.css'
import { BrandMark } from './BrandMark'
import {
  type AppSnapshot,
  hasGeneratedPreview,
  normalizePreview,
  normalizeSnapshot,
} from './desktopState'
import Onboarding from './onboarding/Onboarding'
import { SettingsPanel } from './SettingsPanel'
import { ConflictPanel } from './resources/ConflictPanel'
import { InstallPlanPanel } from './resources/InstallPlanPanel'
import { ResultSummary } from './resources/ResultSummary'

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
  const [previewLoading, setPreviewLoading] = useState(false)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
  const previewRefresh = useRef<{
    request: ReturnType<typeof ResourcePreview>
    task: Promise<void>
  } | null>(null)

  const refresh = async () => {
    try {
      setSnapshot(normalizeSnapshot(await loadSnapshot()))
      setError('')
    } catch (cause) {
      setError(errorMessage(cause))
    }
  }

  const cancelPreviewRequest = () => {
    const active = previewRefresh.current
    previewRefresh.current = null
    active?.request.cancel()
  }

  const cancelPreviewRefresh = () => {
    cancelPreviewRequest()
    setPreviewLoading(false)
  }

  const refreshResourcePreview = (): Promise<void> => {
    const active = previewRefresh.current
    if (active) return active.task

    const request = ResourcePreview()
    setPreviewLoading(true)
    const task = (async () => {
      try {
        const preview = await request
        if (previewRefresh.current?.request !== request) return
        setSnapshot((current) => current ? {
          ...current,
          preview: normalizePreview(preview),
        } : current)
        setError('')
      } catch (cause) {
        if (previewRefresh.current?.request === request) {
          setError(errorMessage(cause))
        }
      } finally {
        if (previewRefresh.current?.request === request) {
          previewRefresh.current = null
          setPreviewLoading(false)
        }
      }
    })()
    previewRefresh.current = { request, task }
    return task
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

  useEffect(() => {
    if (!settingsOpen || !snapshot?.configured) {
      return
    }
    if (!hasGeneratedPreview(snapshot.preview.generatedAt)) {
      void refreshResourcePreview()
    }
    return cancelPreviewRefresh
  }, [settingsOpen, snapshot?.configured, snapshot?.preview.generatedAt])

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

  const closeSettings = () => {
    cancelPreviewRefresh()
    setSettingsOpen(false)
  }

  const saveFirstSyncStrategy = async (strategy: string) => perform(async () => {
    if (!snapshot) return
    await SaveSettings({
      repositoryUrl: snapshot.repositoryUrl,
      repositoryDir: snapshot.repoPath,
      repoPathMode: 'reclone',
      firstSyncStrategy: strategy,
      intervalMinutes: snapshot.intervalMinutes,
      trashGraceDays: snapshot.trashGraceDays,
      agents: Object.fromEntries(snapshot.agents.map((agent) => [agent.name, agent.enabled])),
      categories: Object.fromEntries(snapshot.agents.map((agent) => [
        agent.name,
        Object.fromEntries(agent.resources.map((resource) => [resource.category, resource.enabled])),
      ])),
      customResources: snapshot.customResources,
    })
    await TriggerSync()
  }, 'First sync strategy saved')

  if (needsOnboarding) {
    return <Onboarding complete={() => {
      setNeedsOnboarding(false)
      void refresh()
    }} />
  }

  if (needsOnboarding === undefined || !snapshot) {
    return (
      <main className="loading">
        <BrandMark label="SyncHub for Agents" />
        <p>{error || 'Opening SyncHub for Agents…'}</p>
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
          <BrandMark label="SyncHub for Agents" />
          <div>
            <strong>SyncHub for Agents</strong>
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
              Connect a private Git repository once, then SyncHub for Agents will safely synchronize
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

            {snapshot.firstSyncRequired && (
              <section className="attention-panel">
                <div className="attention-heading">
                  <h2>Choose first sync strategy</h2>
                </div>
                <p>Select how SyncHub should handle your first synchronization between local and cloud data.</p>
                <div className="inline-actions">
                  <button className="secondary" disabled={busy} onClick={() => void saveFirstSyncStrategy('use-cloud')}>
                    Use cloud
                  </button>
                  <button className="secondary" disabled={busy} onClick={() => void saveFirstSyncStrategy('merge-cloud-local')}>
                    Merge cloud + local
                  </button>
                  <button className="secondary" disabled={busy} onClick={() => void saveFirstSyncStrategy('use-local')}>
                    Use local
                  </button>
                </div>
              </section>
            )}

            {snapshot.syncDiagnostic && (
              <section className="attention-panel">
                <div className="attention-heading">
                  <h2>Fix sync issue</h2>
                </div>
                <p>{snapshot.syncDiagnostic.summary}</p>
                <code>{snapshot.syncDiagnostic.repoPath}</code>
                <div className="inline-actions">
                  <button
                    className="secondary"
                    type="button"
                    onClick={() => void Browser.OpenURL(toFileURL(snapshot.syncDiagnostic!.repoPath))}
                  >
                    Open repo folder
                  </button>
                </div>
                <div className="operation-list">
                  {snapshot.syncDiagnostic.steps.map((step) => (
                    <article key={step.title}>
                      <strong>{step.title}</strong>
                      <code>{step.command}</code>
                      {step.warning && <small className="warning">{step.warning}</small>}
                    </article>
                  ))}
                </div>
              </section>
            )}

            {snapshot.pendingInstallPlan && (
              <InstallPlanPanel
                plan={snapshot.pendingInstallPlan}
                busy={busy}
                approve={(id) => perform(
                  () => ApproveInstallPlan(id),
                  'Install plan approved; synchronization queued',
                )}
                retry={(id) => perform(
                  () => RetryInstallPlan(id),
                  'Install operation retry queued',
                )}
              />
            )}

            {(snapshot.conflicts.length > 0 || snapshot.conflictResolution) && (
              <ConflictPanel
                conflicts={snapshot.conflicts}
                resolution={snapshot.conflictResolution ?? null}
                busy={busy}
                applyBatch={(selections) => perform(
                  () => QueueConflictBatch(selections),
                  'Conflict resolution batch queued',
                )}
                retryBatch={(id) => perform(
                  () => RetryConflictBatch(id),
                  'Conflict resolution batch retried',
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
          previewLoading={previewLoading}
          close={closeSettings}
          refreshPreview={refreshResourcePreview}
          runSyncNow={() => perform(TriggerSync, 'Synchronization queued')}
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

function SettingsIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 15.4a3.4 3.4 0 1 0 0-6.8 3.4 3.4 0 0 0 0 6.8Zm7.1-2.5c.1-.6.1-1.2 0-1.8l2-1.5-2-3.4-2.5 1a8 8 0 0 0-1.5-.9L14.8 3h-4l-.4 3.3c-.5.2-1 .5-1.5.9l-2.5-1-2 3.4 2 1.5a7 7 0 0 0 0 1.8l-2 1.5 2 3.4 2.5-1c.5.4 1 .7 1.5.9l.4 3.3h4l.4-3.3c.5-.2 1-.5 1.5-.9l2.5 1 2-3.4-2.1-1.5Z" /></svg>
}

function SyncIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5m9.6-2A7 7 0 0 0 6.3 7.7L4 12m16 0-2.3 4.3A7 7 0 0 1 5.4 14" /></svg>
}

function toFileURL(path: string) {
  const normalized = path.replace(/\\/g, '/')
  if (/^[a-zA-Z]:\//.test(normalized)) {
    return `file:///${normalized}`
  }
  return `file://${normalized}`
}

export default App
