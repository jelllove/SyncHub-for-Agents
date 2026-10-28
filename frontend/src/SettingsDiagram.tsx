import { useI18n } from './i18n'

type Copy = [string, string]
type Diagram = { title: Copy; description: Copy; caption: Copy }
export type SettingsDiagramKind = 'repository' | 'sync' | 'resources' | 'library' | 'desktop' | 'advanced'

const diagrams: Record<SettingsDiagramKind, Diagram> = {
  repository: {
    title: ['Repository', '同步仓库'],
    description: ['Connect a private Git repository to share resources across computers.', '连接私有 Git 仓库，让不同电脑共享资源。'],
    caption: ['The local clone is a staging area, not your agent’s own resource folder. Only selected, supported resources are synchronized—not your entire home folder.', '本地仓库副本是中转站，不是各个工具自己的资源目录。只同步选定且受支持的资源，不会上传整个用户目录。'],
  },
  sync: {
    title: ['Sync and recovery', '同步与恢复'],
    description: ['Choose how often to sync and how long deleted files remain recoverable.', '设置自动同步频率，以及删除后可以恢复多久。'],
    caption: ['A sync pulls remote changes, checks and applies safe changes, then pushes when needed. Deleted files stay recoverable during retention; expired archives are cleaned after successful sync.', '同步会先拉取远端，检查并应用安全变更，再按需推送。删除的文件在保留期内仍可恢复；过期归档在成功同步后的清理阶段移除。'],
  },
  resources: {
    title: ['Sync content', '同步内容'],
    description: ['Choose which tools and resource types are included in sync.', '选择需要同步的工具和资源类型。'],
    caption: ['Safety filters still exclude credentials and caches. This page controls cross-computer sync; Library controls how compatible local tools use a shared source.', '安全过滤仍会排除凭据和缓存。这里管理跨电脑同步；共享资源页管理本机多个兼容工具如何使用一份资源。'],
  },
  library: {
    title: ['Shared library', '共享资源'],
    description: ['Maintain one source for skills or instructions, then choose where to use it.', '维护一份技能或指令，再选择哪些兼容工具使用。'],
    caption: ['Scanning does not change files. This version shares skills and Claude global instructions—not plugin or configuration conversions.', '扫描不会改动文件。当前共享技能和 Claude 全局指令，不进行插件或配置格式转换。'],
  },
  desktop: {
    title: ['Startup and notifications', '启动与通知'],
    description: ['Control startup and how SyncHub notifies you while working in the background.', '管理开机启动，以及后台工作时的提醒方式。'],
    caption: ['When enabled, signing in starts SyncHub in the tray. Windows activity tips hide after 5 seconds, or 10 seconds for errors. Hiding tips does not stop synchronization or remove the tray icon.', '启用自动启动后，登录时在托盘启动 SyncHub。Windows 普通活动提示 5 秒后隐藏，错误提示 10 秒后隐藏。关闭提示不会停止同步，也不会关闭托盘图标。'],
  },
  advanced: {
    title: ['Advanced settings', '高级设置'],
    description: ['Manage updates or start over with local setup when needed.', '管理应用更新，或在需要时重新设置本机。'],
    caption: ['Reset is not a normal sync action. Review its actual impact and explicitly confirm before setting up your connection again.', '重置不是日常同步操作。先查看实际影响范围，明确确认后，再重新设置连接。'],
  },
}

export function SettingsCategoryIcon({ kind }: { kind: SettingsDiagramKind }) {
  const paths: Record<SettingsDiagramKind, React.ReactNode> = {
    repository: <><path d="M9 28h23a7 7 0 0 0 1-14 12 12 0 0 0-23-2A8 8 0 0 0 9 28z" /><rect x="16" y="18" width="10" height="8" rx="2" /><path d="M18 18v-3a3 3 0 0 1 6 0v3" /></>,
    sync: <><path d="M7 12a14 14 0 1 1-1 12M7 5v7H1M20 9v10l7 4" /></>,
    resources: <><rect x="7" y="7" width="24" height="26" rx="3" /><path d="M13 7V3h23v25h-5M12 15l2 2 4-4m5 3h3M12 24l2 2 4-4m5 3h3" /></>,
    library: <><path d="M4 8V4a2 2 0 0 1 2-2h9l4 5h15a2 2 0 0 1 2 2v13H4zM4 11h32M20 22v5M10 27h20M10 27v3m20-3v3" /><rect x="4" y="30" width="12" height="5" rx="1" /><rect x="24" y="30" width="12" height="5" rx="1" /></>,
    desktop: <><rect x="3" y="3" width="34" height="28" rx="3" /><path d="M3 24h34m-10 4h2m3 0h2" /><rect x="17" y="11" width="16" height="10" rx="2" /></>,
    advanced: <><path d="m20 3 14 5v10c0 8-14 15-14 15S6 26 6 18V8z" /><path d="m13 18 5 5 9-11" /></>,
  }
  return <svg viewBox="0 0 40 36" aria-hidden="true" focusable="false">{paths[kind]}</svg>
}

export function SettingsDiagram({ kind, compact = false }: { kind: SettingsDiagramKind; compact?: boolean }) {
  const { t } = useI18n()
  const diagram = diagrams[kind]
  const illustration = (
    <div className="settings-category-illustration" role="img" aria-label={t(...diagram.title)}>
      <SettingsCategoryIcon kind={kind} />
    </div>
  )
  if (compact) return illustration
  return (
    <div className="settings-explainer">
      <div className="settings-category-heading">
        {illustration}
        <div>
          <h3>{t(...diagram.title)}</h3>
          <p>{t(...diagram.description)}</p>
        </div>
      </div>
      <details className="settings-explainer-help">
        <summary>{t('How this works', '查看原理说明')}</summary>
        <p>{t(...diagram.caption)}</p>
      </details>
    </div>
  )
}
