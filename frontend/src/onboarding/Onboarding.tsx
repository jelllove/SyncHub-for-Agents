import { useCallback, useEffect, useState } from 'react'
import { Browser, Events } from '@wailsio/runtime'
import {
  CancelOnboarding,
  CompleteOnboarding,
  OnboardingState,
  ReturnToRepository,
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
import { ResetPanel } from '../ResetPanel'
import { useI18n } from '../i18n'

type WizardState = Omit<State, 'agents'> & { agents: Agent[] }

function normalize(state: State): WizardState {
  return { ...state, agents: state.agents ?? [] }
}

function message(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

export default function Onboarding({ complete }: { complete: () => void }) {
  const { t } = useI18n()
  const [state, setState] = useState<WizardState>()
  const [showRepository, setShowRepository] = useState(false)
  const [repository, setRepository] = useState('')
  const [enabled, setEnabled] = useState<Record<string, boolean>>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const applyState = useCallback((next: WizardState) => {
    setState(next)
    if (next.step === Step.Repository) {
      setRepository(next.repositoryUrl)
    }
    if (next.step === Step.Agents) {
      setEnabled(Object.fromEntries(next.agents.map((agent) => [agent.name, agent.enabled])))
    }
  }, [])

  useEffect(() => {
    void OnboardingState().then((value) => applyState(normalize(value))).catch((cause) => setError(message(cause)))
    return Events.On('onboarding:state', (event) => {
      applyState(normalize(event.data as State))
    })
  }, [applyState])

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
      applyState(normalize(await OnboardingState()))
    })
  }

  const startLogin = async () => {
    await run(async () => {
      const started = normalize(await StartGitHubLogin())
      applyState(started)
      void WaitGitHubLogin()
        .then((next) => {
          applyState(normalize(next))
        })
        .catch((cause) => setError(message(cause)))
    })
  }

  const verifySSH = async () => {
    await run(async () => {
      applyState(normalize(await VerifySSH()))
    })
  }

  const editRepository = async () => {
    await run(async () => {
      applyState(normalize(await ReturnToRepository()))
    })
  }

  const finish = async () => {
    await run(async () => {
      await CompleteOnboarding(enabled)
      complete()
    })
  }

  const resetComplete = () => {
    setShowRepository(false)
    setRepository('')
    setEnabled({})
    setError('')
    void OnboardingState().then((value) => applyState(normalize(value))).catch((cause) => setError(message(cause)))
  }

  if (!state) {
    return <main className="loading"><BrandMark label="SyncHub for Agents" /><p>{error || t('Preparing setup…', '正在准备设置…')}</p></main>
  }

  if (state.step === Step.Welcome && !showRepository) {
    return (
      <div className="onboarding-shell">
        <section className="onboarding-card welcome-card">
          <BrandMark className="large" label="SyncHub for Agents" />
          <span className="eyebrow">{t('WELCOME TO SYNCHUB FOR AGENTS', '欢迎使用 SYNCHUB FOR AGENTS')}</span>
          <h1>{t('One workspace.', '一个工作空间。')}<br />{t('Every computer.', '每一台电脑。')}</h1>
          <p>{t('Synchronize Claude, Copilot, Gemini, Cursor, and their sessions through a private repository you control.', '通过您掌控的私有仓库，同步 Claude、Copilot、Gemini、Cursor 及其会话。')}</p>
          <button className="primary wide" onClick={() => setShowRepository(true)}>{t('Get started', '开始使用')}</button>
          <div className="privacy-note">{t('Your credentials stay in your operating system keyring.', '凭据仅保存在操作系统密钥环中。')}</div>
        </section>
      </div>
    )
  }

  if (state.step === Step.Welcome || state.step === Step.Repository) {
    return (
      <WizardFrame step={1} title={t('Connect your private repository', '连接您的私有仓库')} subtitle={t('SyncHub for Agents uses this repository as an encrypted-in-transit bridge between your computers.', 'SyncHub for Agents 通过此仓库连接您的电脑，传输过程经过加密。')} error={error} busy={busy} resetComplete={resetComplete} resetBusyChanged={setBusy}>
        <form className="wizard-form" onSubmit={(event) => void submitRepository(event)}>
          <label>
            {t('GitHub repository URL', 'GitHub 仓库地址')}
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
            <span><strong>{t('SSH alias', 'SSH 别名')}</strong> git@github-work:you/sync.git</span>
            <span><strong>HTTPS</strong> https://github.com/you/sync.git</span>
          </div>
          <button className="primary wide" disabled={busy}>{t('Continue', '继续')}</button>
        </form>
      </WizardFrame>
    )
  }

  if (state.step === Step.Authentication || state.step === Step.Verification) {
    const isSSH = state.authMode === 'ssh'
    return (
      <WizardFrame step={2} title={isSSH ? t('Verify SSH access', '验证 SSH 访问') : t('Sign in with GitHub', '使用 GitHub 登录')} subtitle={isSSH ? t('We will check your SSH agent and verify access to the actual repository without opening a terminal.', '我们将检查 SSH 代理并验证目标仓库的访问权限，无需打开终端。') : t('Authorize SyncHub for Agents using GitHub Device Flow. Your token is stored only in the system keyring.', '通过 GitHub 设备授权流程授权 SyncHub for Agents。令牌仅保存在系统密钥环中。')} error={error} busy={busy} resetComplete={resetComplete} resetBusyChanged={setBusy}>
        <div className="auth-summary">
          <span>{t('Repository', '仓库')}</span>
          <strong>{state.repositoryUrl}</strong>
        </div>
        {isSSH ? (
          <>
            <div className="security-points">
              <span>{t('✓ Uses BatchMode with no terminal prompts', '✓ 使用 BatchMode，不显示终端提示')}</span>
              <span>{t('✓ Requires strict GitHub host-key verification', '✓ 严格验证 GitHub 主机密钥')}</span>
              <span>{t('✓ Tests repository access, not just key presence', '✓ 验证仓库访问权限，而非仅检查密钥是否存在')}</span>
            </div>
            <button className="primary wide" disabled={busy} onClick={() => void verifySSH()}>
              {busy ? t('Verifying…', '正在验证…') : t('Verify SSH access', '验证 SSH 访问')}
            </button>
            {error && (
              <button className="secondary wide" disabled={busy} onClick={() => void editRepository()} type="button">
                {t('Edit repository URL', '编辑仓库地址')}
              </button>
            )}
          </>
        ) : state.userCode ? (
          <div className="device-flow">
            <span className="eyebrow">{t('YOUR ONE-TIME CODE', '您的一次性代码')}</span>
            <button className="device-code" onClick={() => navigator.clipboard.writeText(state.userCode ?? '')}>{state.userCode}</button>
            <button className="primary wide" onClick={() => void Browser.OpenURL(state.verificationUri ?? 'https://github.com/login/device')}>{t('Open GitHub', '打开 GitHub')}</button>
            <p className="waiting"><i /> {state.message || t('Waiting for authorization', '等待授权')}</p>
          </div>
        ) : (
          <button className="primary wide" disabled={busy} onClick={() => void startLogin()}>
            {busy ? t('Starting…', '正在启动…') : t('Continue with GitHub', '使用 GitHub 继续')}
          </button>
        )}
      </WizardFrame>
    )
  }

  if (state.step === Step.Agents) {
    return (
      <WizardFrame step={3} title={t('Choose what to synchronize', '选择同步内容')} subtitle={t('All detected agents are enabled by default. You can change this later in Settings.', '默认启用所有检测到的智能体。您可以稍后在设置中修改。')} error={error} busy={busy} resetComplete={resetComplete} resetBusyChanged={setBusy}>
        <div className="wizard-agent-list">
          {state.agents.map((agent) => (
            <label className="wizard-agent" key={agent.name}>
              <span className="agent-avatar">{agent.name.slice(0, 1).toUpperCase()}</span>
              <span><strong>{agent.name}</strong><small>{t(`${agent.exclude?.length ?? 0} sensitive-path exclusions`, `${agent.exclude?.length ?? 0} 项敏感路径排除规则`)}</small></span>
              <input type="checkbox" checked={enabled[agent.name] ?? true} onChange={(event) => setEnabled({ ...enabled, [agent.name]: event.target.checked })} />
            </label>
          ))}
        </div>
        <button className="primary wide" disabled={busy} onClick={() => void finish()}>
          {busy ? t('Finishing setup…', '正在完成设置…') : t('Start synchronizing', '开始同步')}
        </button>
      </WizardFrame>
    )
  }

  return <main className="loading"><BrandMark label="SyncHub for Agents" /><p>{t('Setup complete', '设置完成')}</p></main>
}

function WizardFrame({ step, title, subtitle, error, busy, resetComplete, resetBusyChanged, children }: { step: number; title: string; subtitle: string; error: string; busy: boolean; resetComplete: () => void; resetBusyChanged: (working: boolean) => void; children: React.ReactNode }) {
  const cancel = () => void CancelOnboarding()
  return (
    <div className="onboarding-shell">
      <button className="onboarding-brand" disabled={busy} onClick={cancel}><BrandMark /> SyncHub for Agents</button>
      <section className="onboarding-card">
        <div className="stepper"><span className={step >= 1 ? 'done' : ''}>1</span><i /><span className={step >= 2 ? 'done' : ''}>2</span><i /><span className={step >= 3 ? 'done' : ''}>3</span></div>
        <h1>{title}</h1>
        <p>{subtitle}</p>
        {children}
        {error && <div className="inline-error" role="alert">{error}</div>}
        <ResetPanel busy={busy} complete={resetComplete} workingChanged={resetBusyChanged} />
      </section>
    </div>
  )
}
