import { useEffect, useRef, useState } from 'react'
import { Browser, Events } from '@wailsio/runtime'
import {
  CheckForUpdates,
  RestartToUpdate,
  SetAutomaticUpdates,
  UpdateStatus,
} from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice'
import type { Status } from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/updater/models'

type UpdateAction = 'check' | 'automatic' | 'restart' | 'release'

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

function statusMessage(status: Status | null) {
  const version = status?.latestVersion ? `Version ${status.latestVersion}` : 'An update'
  switch (status?.phase) {
    case 'checking': return 'Checking for updates…'
    case 'downloading': return 'Downloading and verifying update…'
    case 'ready': return status.supported ? `${version} is ready to install.` : `${version} is available.`
    case 'available': return `${version} is available.`
    case 'upToDate': return "You're up to date."
    case 'error': return 'The last update attempt failed.'
    default: return 'Check for the latest release of SyncHub for Agents.'
  }
}

function checkedTime(value: string | undefined) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || date.getUTCFullYear() <= 1) return ''
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date)
}

export function UpdatePanel({ syncBusy }: { syncBusy: boolean }) {
  const [status, setStatus] = useState<Status | null>(null)
  const [loading, setLoading] = useState(true)
  const [pending, setPending] = useState<UpdateAction | null>(null)
  const [error, setError] = useState('')
  const mounted = useRef(false)
  const statusRevision = useRef(0)

  useEffect(() => {
    mounted.current = true
    let active = true
    const revision = statusRevision.current
    const unsubscribe = Events.On('updater:status', (event) => {
      if (!active) return
      statusRevision.current += 1
      setStatus(event.data as Status)
      setError('')
      setLoading(false)
    })
    void UpdateStatus().then((latest) => {
      if (active && statusRevision.current === revision) {
        setStatus(latest)
        setError('')
      }
    }).catch((cause) => {
      if (active && statusRevision.current === revision) setError(errorMessage(cause))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => {
      active = false
      mounted.current = false
      unsubscribe()
    }
  }, [])

  const run = async (action: UpdateAction, request: () => Promise<Status | void>) => {
    if (pending) return
    setPending(action)
    setError('')
    const revision = statusRevision.current
    try {
      const latest = await request()
      if (!mounted.current) return
      if (latest) {
        // Live events can advance a download before its initiating request returns.
        if (statusRevision.current === revision) {
          setStatus(latest)
        } else if (action === 'automatic') {
          setStatus((current) => current ? { ...current, automatic: latest.automatic } : latest)
        }
      }
    } catch (cause) {
      if (mounted.current) setError(errorMessage(cause))
    } finally {
      if (mounted.current) setPending(null)
    }
  }

  const development = status?.currentVersion === 'dev'
  const updating = status?.phase === 'checking' || status?.phase === 'downloading'
  const ready = status?.phase === 'ready' && status.supported && !development
  const lastChecked = checkedTime(status?.lastChecked)
  const visibleError = error || status?.error || (status?.phase === 'error' ? 'Please try checking for updates again.' : '')

  return (
    <fieldset className="update-panel">
      <legend>Application updates</legend>
      <div className="update-version">
        Current version <strong>{status?.currentVersion || (loading ? 'Loading…' : 'Unavailable')}</strong>
      </div>
      <label className="toggle-row">
        <span>
          <strong>Automatic updates</strong>
          <small id="automatic-updates-help">Saved immediately on this device, separately from sync settings.</small>
        </span>
        <input
          aria-describedby="automatic-updates-help"
          aria-label="Automatic updates"
          checked={status?.automatic ?? true}
          disabled={!status || development || pending !== null}
          onChange={(event) => {
            const enabled = event.target.checked
            void run('automatic', () => SetAutomaticUpdates(enabled))
          }}
          type="checkbox"
        />
      </label>
      {development ? (
        <p>Updates are disabled for developer builds. Install a release build to enable update checks.</p>
      ) : (
        <>
          {status && (
            <p>
              {status.supported
                ? 'Automatic updates download and verify releases. Ready updates install when you choose Quit from the tray or Restart to update. No forced background restarts.'
                : 'Automatic updates check for releases and notify you. Automatic installation is not supported on this platform; download and install releases manually.'}
            </p>
          )}
          <div aria-live="polite" role="status">
            {loading ? 'Loading update settings…' : pending === 'check' && !updating ? 'Checking for updates…' : statusMessage(status)}
            {lastChecked && <small>Last checked {lastChecked}</small>}
          </div>
        </>
      )}
      {visibleError && <div className="inline-error" role="alert">{visibleError}</div>}
      <div className="inline-actions">
        <button
          className="secondary"
          disabled={loading || development || updating || pending !== null}
          onClick={() => void run('check', CheckForUpdates)}
          type="button"
        >
          Check now
        </button>
        {ready && (
          <button
            className="primary"
            disabled={syncBusy || pending !== null}
            onClick={() => {
              if (!syncBusy) void run('restart', RestartToUpdate)
            }}
            type="button"
          >
            {pending === 'restart' ? 'Restarting…' : 'Restart to update'}
          </button>
        )}
        {!development && status?.releaseURL && (
          <button
            className="secondary"
            disabled={pending !== null}
            onClick={() => void run('release', () => Browser.OpenURL(status.releaseURL))}
            type="button"
          >
            {status.supported ? 'View release' : 'Download release'}
          </button>
        )}
      </div>
      {ready && syncBusy && <small>Wait for synchronization or settings changes to finish before restarting.</small>}
    </fieldset>
  )
}
