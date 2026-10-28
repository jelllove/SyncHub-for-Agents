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
import { TrayTipSettings } from './TrayTipSettings'
import { SharedLibraryPanel } from './SharedLibraryPanel'
import { SettingsCategoryIcon, SettingsDiagram } from './SettingsDiagram'
import { LanguageSwitcher } from './LanguageSwitcher'
import { useI18n, type Locale, type Translator } from './i18n'

const settingsTabs = [
  { id: 'repository', label: 'Repository', chinese: '同步仓库' },
  { id: 'sync', label: 'Sync', chinese: '同步与恢复' },
  { id: 'resources', label: 'Resources', chinese: '同步内容' },
  { id: 'library', label: 'Library', chinese: '共享资源' },
  { id: 'desktop', label: 'Desktop', chinese: '启动与通知' },
  { id: 'advanced', label: 'Advanced', chinese: '高级设置' },
] as const

type SettingsTab = typeof settingsTabs[number]['id']

export type SettingsPanelProps = {
  initialTab?: SettingsTab
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

function previewTimestamp(value: string, locale: Locale, t: Translator) {
  if (!hasGeneratedPreview(value)) return t('No preview generated yet', '尚未生成预览')
  const timestamp = new Intl.DateTimeFormat(locale, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
  return t(`Last refreshed ${timestamp}`, `上次刷新：${timestamp}`)
}

export function SettingsPanel({
  initialTab,
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
  const { t, locale } = useI18n()
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
  const [activeTab, setActiveTab] = useState<SettingsTab>(initialTab ?? (snapshot.configured ? 'sync' : 'repository'))
  const settingsId = useId()
  const formId = `${settingsId}-form`
  const tabButtons = useRef<(HTMLButtonElement | null)[]>([])
  const content = useRef<HTMLDivElement>(null)
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
    if (content.current) content.current.scrollTop = 0
    if (invalidSetting.current) {
      invalidSetting.current.focus()
      invalidSetting.current.reportValidity()
      invalidSetting.current = null
    }
  }, [activeTab])

  const navigateTab = (event: React.KeyboardEvent<HTMLButtonElement>, index: number) => {
    let next: number
    switch (event.key) {
      case 'ArrowDown':
      case 'ArrowRight': next = (index + 1) % settingsTabs.length; break
      case 'ArrowUp':
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
    if (activeTab === 'library' || activeTab === 'advanced') return
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

  const title = snapshot.configured ? t('Sync settings', '同步设置') : t('Connect your repository', '连接你的仓库')
  const separateConfirmation = activeTab === 'library' || activeTab === 'advanced'

  return (
    <div className="drawer-backdrop settings-backdrop" onMouseDown={close}>
      <aside
        aria-labelledby="settings-title"
        aria-modal="true"
        className="drawer settings-drawer"
        onMouseDown={(event) => event.stopPropagation()}
        role="dialog"
      >
        <div className="drawer-title">
          <div>
            <span className="eyebrow">{snapshot.configured ? t('PREFERENCES', '偏好设置') : t('GET STARTED', '开始使用')}</span>
            <h2 id="settings-title">{title}</h2>
          </div>
          <div className="settings-heading-actions">
            <LanguageSwitcher />
            <button type="button" className="icon-button" onClick={close} aria-label={t('Close settings', '关闭设置')}>×</button>
          </div>
        </div>
        <div className="settings-tabs" role="tablist" aria-orientation="vertical" aria-label={t('Settings categories', '设置分类')}>
          {settingsTabs.map((tab, index) => (
            <button
              aria-controls={`${settingsId}-${tab.id}-panel`}
              aria-description={tab.id === 'desktop' && startAtLoginError ? t('Startup needs attention', '启动设置需要处理') : undefined}
              aria-label={t(tab.label, tab.chinese)}
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
              <SettingsCategoryIcon kind={tab.id} />
              <span>{t(tab.label, tab.chinese)}</span>
              {tab.id === 'desktop' && startAtLoginError && <span className="settings-tab-error" aria-label={t('Startup needs attention', '启动设置需要处理')}>!</span>}
            </button>
          ))}
        </div>
        <div className="settings-content" ref={content}>
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
          <SettingsDiagram kind="repository" />
          <label>
            {t('Private Git repository', '私有 Git 仓库')}
            <input
              required
              value={repositoryUrl}
              onChange={(event) => setRepositoryUrl(event.target.value)}
              placeholder="git@github.com:your-name/agent-sync.git"
            />
            <small>{t('SSH and HTTPS repositories are supported.', '支持 SSH 和 HTTPS 仓库。')}</small>
          </label>
          <label>
            {t('Local repository directory', '本机仓库副本目录')}
            <input
              required
              value={repositoryDir}
              onChange={(event) => setRepositoryDir(event.target.value)}
              placeholder={snapshot.repoPath}
            />
            <small>{t('Where SyncHub keeps the local clone.', 'SyncHub 存放本地仓库副本的位置，不是工具的技能目录。')}</small>
          </label>
          <fieldset>
            <legend>{t('When directory changes', '更换目录时')}</legend>
            <label className="toggle-row">
              <span>
                <strong>{t('Keep old folder, clone into new', '保留旧目录，重新克隆到新目录')}</strong>
              </span>
              <input
                type="radio"
                checked={repoPathMode === 'reclone'}
                onChange={() => setRepoPathMode('reclone')}
              />
            </label>
            <label className="toggle-row">
              <span>
                <strong>{t('Move existing repo to new folder', '将现有仓库迁移到新目录')}</strong>
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
          <SettingsDiagram kind="sync" />
          <div className="field-grid">
            <label>
              {t('Sync frequency (minutes)', '自动同步间隔（分钟）')}
              <input
                required
                type="number"
                min={1}
                max={1440}
                step={1}
                value={intervalMinutes}
                onChange={(event) => setIntervalMinutes(Number(event.target.value))}
              />
              <small>{t('Runs every 1–1440 minutes.', '可设置为每 1–1440 分钟运行一次。')}</small>
            </label>
            <label>
              {t('Archive retention (days)', '删除后的恢复窗口（天）')}
              <input
                required
                type="number"
                min={1}
                max={365}
                step={1}
                value={trashGraceDays}
                onChange={(event) => setTrashGraceDays(Number(event.target.value))}
              />
              <small>{t('Deleted files remain recoverable for 1–365 days.', '已删除文件可保留 1–365 天用于恢复。')}</small>
            </label>
          </div>
          <section className="preview-refresh">
            <div>
              <strong>{t('Synchronization', '立即同步')}</strong>
              <small>{t('Run an immediate one-time synchronization now.', '立即运行一次同步，不改变自动同步间隔。')}</small>
            </div>
            <button
              className="secondary"
              disabled={busy || !snapshot.configured}
              onClick={() => void runSyncNow()}
              type="button"
            >
              {t('Run sync now', '立即同步一次')}
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
          <SettingsDiagram kind="resources" />
          <fieldset>
            <legend>{t('Agents to synchronize', '需要同步的工具')}</legend>
            {snapshot.agents.map((agent) => (
              <label className="toggle-row" key={agent.name}>
                <span>
                  <strong>{agent.name}</strong>
                  <small>{t(`${agent.exclude.length} safety exclusions`, `${agent.exclude.length} 条安全排除规则`)}</small>
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
            <legend>{t('Resource categories', '资源类型')}</legend>
            <ResourceSettings agents={snapshot.agents} categories={categories} onChange={setCategories} />
          </fieldset>
          <section className="preview-refresh" aria-live="polite">
            <div>
              <strong>{t('Resource preview', '资源预览')}</strong>
              <small>{previewTimestamp(snapshot.preview.generatedAt, locale, t)}</small>
            </div>
            <button
              className="secondary"
              disabled={previewLoading || !snapshot.configured}
              onClick={() => void refreshPreview()}
              type="button"
            >
              {previewLoading ? t('Refreshing…', '正在刷新…') : t('Refresh preview', '刷新预览')}
            </button>
          </section>
          <RestorePreview preview={snapshot.preview} />
          <CustomResourceEditor
            platform={snapshot.platform}
            resources={customResources}
            onChange={setCustomResources}
          />
          </section>
          <section
            aria-labelledby={`${settingsId}-library-tab`}
            className="settings-panel"
            data-settings-tab="library"
            hidden={activeTab !== 'library'}
            id={`${settingsId}-library-panel`}
            role="tabpanel"
            tabIndex={0}
          >
            {activeTab === 'library' && <SharedLibraryPanel busy={busy || snapshot.state === 'updating'} />}
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
          <SettingsDiagram kind="desktop" />
          <fieldset>
            <legend>{t('Desktop application', '桌面应用')}</legend>
            <label className="toggle-row">
              <span>
                <strong>{snapshot.platform === 'windows' ? t('Start with Windows', '登录 Windows 时启动') : t('Start at login', '登录系统时启动')}</strong>
                <small>
                  {snapshot.platform === 'windows'
                    ? t('Enabled by default in Windows release builds. Starts in the tray when you sign in; you can turn it off here.', 'Windows 发布版默认启用，登录后在托盘启动，可在此关闭。')
                    : t('Starts SyncHub for Agents the next time you sign in. It does not restart the app now.', '下次登录系统时启动 SyncHub for Agents，不会立即重启应用。')}
                </small>
              </span>
              <input
                type="checkbox"
                checked={startAtLogin}
                disabled={startAtLoginLoading || !!startAtLoginError}
                onChange={(event) => setStartAtLogin(event.target.checked)}
              />
            </label>
            {startAtLoginLoading && <small role="status">{t('Checking startup registration…', '正在检查自动启动设置…')}</small>}
            {startAtLoginError && (
              <>
                <div className="inline-error" role="alert">{startAtLoginError}</div>
                <button className="secondary" onClick={() => setStartupRevision((revision) => revision + 1)} type="button">
                  {t('Retry startup status', '重新检查启动状态')}
                </button>
              </>
            )}
          </fieldset>
          {snapshot.platform === 'windows' && <TrayTipSettings />}
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
          <SettingsDiagram kind="advanced" />
          <ResetPanel busy={busy || previewLoading || snapshot.state === 'updating'} complete={resetComplete ?? close} workingChanged={resetBusyChanged} />
        </section>
        </div>
        <div className="form-actions settings-actions">
          {separateConfirmation && <small className="settings-action-note">{activeTab === 'library'
            ? t('Library changes use their own preview and confirmation.', '共享资源通过各自的预览和确认生效。')
            : t('Reset requires a separate confirmation.', '重置需要单独确认。')}</small>}
          <button type="button" className="secondary" onClick={close}>{separateConfirmation ? t('Close', '关闭') : t('Cancel', '取消')}</button>
          {!separateConfirmation && <button type="submit" form={formId} className="primary" disabled={busy}>
            {snapshot.configured ? t('Save settings', '保存设置') : t('Continue', '继续')}
          </button>}
        </div>
      </aside>
    </div>
  )
}
