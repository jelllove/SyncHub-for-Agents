import { Browser } from '@wailsio/runtime'
import { RestartToUpdate } from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice'
import { useUpdates } from './updates'
import { useI18n } from './i18n'

export function UpdateButton({ syncBusy }: { syncBusy: boolean }) {
  const { t } = useI18n()
  const { status, pending, run } = useUpdates()
  if (!status || status.currentVersion === 'dev') return null

  const downloading = status.supported && status.phase === 'downloading'
  const ready = status.supported && status.phase === 'ready'
  const manual = !status.supported && Boolean(status.releaseURL)
    && (status.phase === 'available' || status.phase === 'ready')
  if (!downloading && !ready && !manual) return null

  const version = status.latestVersion || t('the latest version', '最新版本')
  const restarting = pending === 'restart'
  const label = restarting ? t('Restarting…', '正在重启…') : downloading ? t('Downloading…', '正在下载…') : manual ? t('Download update', '下载更新') : t('Update', '更新')
  const title = downloading
    ? t(`Downloading and verifying ${version}`, `正在下载并验证 ${version}`)
    : manual
      ? t(`Download ${version} from the release page`, `从发布页面下载 ${version}`)
      : syncBusy
        ? t('Wait for synchronization or settings changes to finish before updating.', '请等待同步或设置更改完成后再更新。')
        : t(`Update to ${version} and restart`, `更新到 ${version} 并重启`)

  return (
    <button
      aria-busy={downloading || restarting}
      className="primary update-button"
      disabled={downloading || pending !== null || (ready && syncBusy)}
      onClick={() => {
        if (manual) {
          void run('release', () => Browser.OpenURL(status.releaseURL))
        } else if (ready && !syncBusy) {
          void run('restart', RestartToUpdate)
        }
      }}
      title={title}
      type="button"
    >
      <svg aria-hidden="true" viewBox="0 0 20 20">
        <path d="M10 15V5m-4 4 4-4 4 4M4 16h12" />
      </svg>
      {label}
    </button>
  )
}
