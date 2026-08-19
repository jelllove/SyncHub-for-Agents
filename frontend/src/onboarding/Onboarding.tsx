import { useEffect, useState } from 'react'
import { Browser, Events } from '@wailsio/runtime'
import {
  CancelOnboarding,
  CompleteOnboarding,
  OnboardingState,
  SetRepository,
  StartGitHubLogin,
  VerifySSH,
  WaitGitHubLogin,
} from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice'
import {
  Step,
  type Agent,
  type State,
} from '../../bindings/github.com/qinqingxu/synchub-for-agents/internal/onboarding/models'
import { BrandMark } from '../BrandMark'

type WizardState = Omit<State, 'agents'> & { agents: Agent[] }

function normalize(state: State): WizardState {
  return { ...state, agents: state.agents ?? [] }
}

function message(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

export default function Onboarding({ complete }: { complete: () => void }) {
  const [state, setState] = useState<WizardState>()
  const [showRepository, setShowRepository] = useState(false)
  const [repository, setRepository] = useState('')
  const [enabled, setEnabled] = useState<Record<string, boolean>>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    void OnboardingState().then((value) => setState(normalize(value))).catch((cause) => setError(message(cause)))
    return Events.On('onboarding:state', (event) => {
      const next = normalize(event.data as State)
      setState(next)
      if (next.step === Step.Agents) {
        setEnabled(Object.fromEntries(next.agents.map((agent) => [agent.name, agent.enabled])))
      }
    })
  }, [])

  const run = async (action: () => Promise<void>) => {
    setBusy(true)
    setError('')
    try {
      await action()
    } catch (cause) {
      setError(message(cause))
    } finally {
      setBusy(false)
    }
  }

  const submitRepository = async (event: React.FormEvent) => {
    event.preventDefault()
    await run(async () => {
      await SetRepository(repository)
      setState(normalize(await OnboardingState()))
    })
  }

  const startLogin = async () => {
    await run(async () => {
      const started = normalize(await StartGitHubLogin())
      setState(started)
      void WaitGitHubLogin()
        .then((next) => {
          const normalized = normalize(next)
          setState(normalized)
          setEnabled(Object.fromEntries(normalized.agents.map((agent) => [agent.name, agent.enabled])))
        })
        .catch((cause) => setError(message(cause)))
    })
  }

  const verifySSH = async () => {
    await run(async () => {
      const next = normalize(await VerifySSH())
      setState(next)
      setEnabled(Object.fromEntries(next.agents.map((agent) => [agent.name, agent.enabled])))
    })
  }

  const finish = async () => {
    await run(async () => {
      await CompleteOnboarding(enabled)
      complete()
    })
  }

  if (!state) {
    return <main className="loading"><BrandMark label="SyncHub for Agents" /><p>{error || 'Preparing setup…'}</p></main>
  }

  if (state.step === Step.Welcome && !showRepository) {
    return (
      <div className="onboarding-shell">
        <section className="onboarding-card welcome-card">
          <BrandMark className="large" label="SyncHub for Agents" />
          <span className="eyebrow">WELCOME TO SYNCHUB FOR AGENTS</span>
          <h1>One workspace.<br />Every computer.</h1>
          <p>Synchronize Claude, Copilot, Gemini, Cursor, and their sessions through a private repository you control.</p>
          <button className="primary wide" onClick={() => setShowRepository(true)}>Get started</button>
          <div className="privacy-note">Your credentials stay in your operating system keyring.</div>
        </section>
      </div>
    )
  }

  if (state.step === Step.Welcome || state.step === Step.Repository) {
    return (
      <WizardFrame step={1} title="Connect your private repository" subtitle="SyncHub for Agents uses this repository as an encrypted-in-transit bridge between your computers." error={error}>
        <form className="wizard-form" onSubmit={(event) => void submitRepository(event)}>
          <label>
            GitHub repository URL
            <input
              autoFocus
              required
              value={repository}
              onChange={(event) => setRepository(event.target.value)}
              placeholder="git@github.com:your-name/agent-sync.git"
            />
          </label>
          <div className="url-examples">
            <span><strong>SSH</strong> git@github.com:you/sync.git</span>
            <span><strong>HTTPS</strong> https://github.com/you/sync.git</span>
          </div>
          <button className="primary wide" disabled={busy}>Continue</button>
        </form>
      </WizardFrame>
    )
  }

  if (state.step === Step.Authentication || state.step === Step.Verification) {
    const isSSH = state.authMode === 'ssh'
    return (
      <WizardFrame step={2} title={isSSH ? 'Verify SSH access' : 'Sign in with GitHub'} subtitle={isSSH ? 'We will check your SSH agent and verify access to the actual repository without opening a terminal.' : 'Authorize SyncHub for Agents using GitHub Device Flow. Your token is stored only in the system keyring.'} error={error}>
        <div className="auth-summary">
          <span>Repository</span>
          <strong>{state.repositoryUrl}</strong>
        </div>
        {isSSH ? (
          <>
            <div className="security-points">
              <span>✓ Uses BatchMode with no terminal prompts</span>
              <span>✓ Requires strict GitHub host-key verification</span>
              <span>✓ Tests repository access, not just key presence</span>
            </div>
            <button className="primary wide" disabled={busy} onClick={() => void verifySSH()}>
              {busy ? 'Verifying…' : 'Verify SSH access'}
            </button>
          </>
        ) : state.userCode ? (
          <div className="device-flow">
            <span className="eyebrow">YOUR ONE-TIME CODE</span>
            <button className="device-code" onClick={() => navigator.clipboard.writeText(state.userCode ?? '')}>{state.userCode}</button>
            <button className="primary wide" onClick={() => void Browser.OpenURL(state.verificationUri ?? 'https://github.com/login/device')}>Open GitHub</button>
            <p className="waiting"><i /> {state.message || 'Waiting for authorization'}</p>
          </div>
        ) : (
          <button className="primary wide" disabled={busy} onClick={() => void startLogin()}>
            {busy ? 'Starting…' : 'Continue with GitHub'}
          </button>
        )}
      </WizardFrame>
    )
  }

  if (state.step === Step.Agents) {
    return (
      <WizardFrame step={3} title="Choose what to synchronize" subtitle="All detected agents are enabled by default. You can change this later in Settings." error={error}>
        <div className="wizard-agent-list">
          {state.agents.map((agent) => (
            <label className="wizard-agent" key={agent.name}>
              <span className="agent-avatar">{agent.name.slice(0, 1).toUpperCase()}</span>
              <span><strong>{agent.name}</strong><small>{agent.exclude?.length ?? 0} sensitive-path exclusions</small></span>
              <input type="checkbox" checked={enabled[agent.name] ?? true} onChange={(event) => setEnabled({ ...enabled, [agent.name]: event.target.checked })} />
            </label>
          ))}
        </div>
        <button className="primary wide" disabled={busy} onClick={() => void finish()}>
          {busy ? 'Finishing setup…' : 'Start synchronizing'}
        </button>
      </WizardFrame>
    )
  }

  return <main className="loading"><BrandMark label="SyncHub for Agents" /><p>Setup complete</p></main>
}

function WizardFrame({ step, title, subtitle, error, children }: { step: number; title: string; subtitle: string; error: string; children: React.ReactNode }) {
  const cancel = () => void CancelOnboarding()
  return (
    <div className="onboarding-shell">
      <button className="onboarding-brand" onClick={cancel}><BrandMark /> SyncHub for Agents</button>
      <section className="onboarding-card">
        <div className="stepper"><span className={step >= 1 ? 'done' : ''}>1</span><i /><span className={step >= 2 ? 'done' : ''}>2</span><i /><span className={step >= 3 ? 'done' : ''}>3</span></div>
        <h1>{title}</h1>
        <p>{subtitle}</p>
        {children}
        {error && <div className="inline-error" role="alert">{error}</div>}
      </section>
    </div>
  )
}
