import { Browser } from '@wailsio/runtime'
import {
  CheckForUpdates,
  RestartToUpdate,
  SetAutomaticUpdates,
} from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice'
import type { Status } from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/updater/models'
import { useI18n, type Translator } from './i18n'
import { useUpdates } from './updates'

function statusMessage(status: Status | null, t: Translator) {
  const version = status?.latestVersion ? t(`Version ${status.latestVersion}`, `版本 ${status.latestVersion}`) : t('An update', '更新')
  switch (status?.phase) {
    case 'checking': return t('Checking for updates…', '正在检查更新…')
    case 'downloading': return t('Downloading and verifying update…', '正在下载并验证更新…')
    case 'ready': return status.supported ? t(`${version} is ready to install.`, `${version} 已准备好安装。`) : t(`${version} is available.`, `${version} 可用。`)
    case 'available': return t(`${version} is available.`, `${version} 可用。`)
    case 'upToDate': return t("You're up to date.", '当前已是最新版本。')
    case 'error': return t('The last update attempt failed.', '上次更新尝试失败。')
    default: return t('Check for the latest release of SyncHub for Agents.', '检查 SyncHub for Agents 的最新版本。')
  }
}

function checkedTime(value: string | undefined, locale: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || date.getUTCFullYear() <= 1) return ''
  return new Intl.DateTimeFormat(locale, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date)
}

export function UpdatePanel({ syncBusy }: { syncBusy: boolean }) {
  const { locale, t } = useI18n()
  const { status, loading, pending, error, run } = useUpdates()
  const development = status?.currentVersion === 'dev'
  const updating = status?.phase === 'checking' || status?.phase === 'downloading'
  const ready = status?.phase === 'ready' && status.supported && !development
  const lastChecked = checkedTime(status?.lastChecked, locale)

  return (
    <fieldset className="update-panel">
      <legend>{t('Application updates', '应用更新')}</legend>
      <div className="update-version">
        {t('Current version', '当前版本')} <strong>{status?.currentVersion || (loading ? t('Loading…', '正在加载…') : t('Unavailable', '不可用'))}</strong>
      </div>
      <label className="toggle-row">
        <span>
          <strong>{t('Automatic updates', '自动更新')}</strong>
          <small id="automatic-updates-help">{t('Saved immediately on this device, separately from sync settings.', '立即保存在此设备上，与同步设置分开保存。')}</small>
        </span>
        <input
          aria-describedby="automatic-updates-help"
          aria-label={t('Automatic updates', '自动更新')}
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
        <p>{t('Updates are disabled for developer builds. Install a release build to enable update checks.', '开发版本已禁用更新。请安装正式版本以启用更新检查。')}</p>
      ) : (
        <>
          {status && (
            <p>
              {status.supported
                ? t('Automatic updates download and verify releases. Ready updates install when you choose Quit from the tray or Restart to update. No forced background restarts.', '自动更新会下载并验证发布版本。选择托盘中的退出或重启以更新时，将安装准备好的更新。不会在后台强制重启。')
                : t('Automatic updates check for releases and notify you. Automatic installation is not supported on this platform; download and install releases manually.', '自动更新会检查新版本并通知您。此平台不支持自动安装；请手动下载并安装发布版本。')}
            </p>
          )}
          <div aria-live="polite" role="status">
            {loading ? t('Loading update settings…', '正在加载更新设置…') : pending === 'check' && !updating ? t('Checking for updates…', '正在检查更新…') : statusMessage(status, t)}
            {lastChecked && <small>{t('Last checked ', '上次检查 ')}{lastChecked}</small>}
          </div>
        </>
      )}
      {error && <div className="inline-error" role="alert">{error}</div>}
      <div className="inline-actions">
        <button
          className="secondary"
          disabled={loading || development || updating || pending !== null}
          onClick={() => void run('check', CheckForUpdates)}
          type="button"
        >
          {t('Check now', '立即检查')}
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
            {pending === 'restart' ? t('Restarting…', '正在重启…') : t('Restart to update', '重启以更新')}
          </button>
        )}
        {!development && status?.releaseURL && (
          <button
            className="secondary"
            disabled={pending !== null}
            onClick={() => void run('release', () => Browser.OpenURL(status.releaseURL))}
            type="button"
          >
            {status.supported ? t('View release', '查看发布版本') : t('Download release', '下载发布版本')}
          </button>
        )}
      </div>
      {ready && syncBusy && <small>{t('Wait for synchronization or settings changes to finish before restarting.', '请等待同步或设置更改完成后再重启。')}</small>}
    </fieldset>
  )
}
