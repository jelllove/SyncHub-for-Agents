import { useState } from 'react'
import { useI18n } from './i18n'
import { LocalizedError, localizedErrorMessage } from './localizedError'
import {
  ResetLocalSetup,
  ResetPreview,
} from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice'

export function ResetPanel({ busy, complete, workingChanged }: {
  busy: boolean
  complete: () => void
  workingChanged?: (working: boolean) => void
}) {
  const { t } = useI18n()
  const [repoPath, setRepoPath] = useState<string>()
  const [confirmation, setConfirmation] = useState('')
  const [working, setWorking] = useState(false)
  const [error, setError] = useState<string | LocalizedError>('')

  const review = async () => {
    setWorking(true)
    workingChanged?.(true)
    setError('')
    try {
      const preview = await ResetPreview()
      setConfirmation('')
      setRepoPath(preview.repoPath)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setWorking(false)
      workingChanged?.(false)
    }
  }

  const reset = async () => {
    if (repoPath === undefined) {
      setError(new LocalizedError('Review the local repository path before resetting.', '重置前请审查本地仓库路径。'))
      return
    }
    setWorking(true)
    workingChanged?.(true)
    setError('')
    try {
      await ResetLocalSetup(confirmation, repoPath)
      complete()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setWorking(false)
      workingChanged?.(false)
    }
  }

  return (
    <section className="reset-panel">
      <h3>{t('Reset local setup', '重置本地设置')}</h3>
      <p>{t('Remove SyncHub settings, sync history, pending conflicts and installs, saved OAuth login, and the local clone. This cannot be undone.', '删除 SyncHub 设置、同步历史、待处理冲突和安装、已保存的 OAuth 登录以及本地克隆。此操作无法撤销。')}</p>
      <p>{t('Your remote repository, agent files, and SSH keys/configuration will not be changed.', '远程仓库、智能体文件和 SSH 密钥及配置不会被更改。')}</p>
      {repoPath === undefined ? (
        <button className="secondary" disabled={busy || working} onClick={() => void review()} type="button">
          {t('Reset and start over', '重置并重新开始')}
        </button>
      ) : (
        <>
          <p>{t('Local clone to delete:', '将删除的本地克隆：')}</p>
          <code>{repoPath}</code>
          <label>
            {t('Type RESET to confirm', '输入 RESET 以确认')}
            <input value={confirmation} disabled={busy || working} onChange={(event) => setConfirmation(event.target.value)} autoComplete="off" />
          </label>
          <div className="inline-actions">
            <button className="secondary" disabled={working} onClick={() => { setRepoPath(undefined); setError('') }} type="button">
              {t('Cancel reset', '取消重置')}
            </button>
            <button className="primary" disabled={busy || working || confirmation !== 'RESET'} onClick={() => void reset()} type="button">
              {working ? t('Resetting…', '正在重置…') : t('Delete local setup', '删除本地设置')}
            </button>
          </div>
        </>
      )}
      {error && <div className="inline-error" role="alert">{localizedErrorMessage(error, t)}</div>}
    </section>
  )
}
