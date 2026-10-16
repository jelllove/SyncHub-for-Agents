import { useI18n } from './i18n'

type Copy = [string, string]
type Icon = 'computer' | 'folder' | 'tools' | 'repository' | 'shield' | 'clock' | 'tray'
type Node = { icon: Icon; label: Copy; hint: Copy }
type Diagram = { title: Copy; description: Copy; summary: Copy; nodes: [Node, Node, Node]; caption: Copy; bidirectional?: boolean }
export type SettingsDiagramKind = 'repository' | 'sync' | 'resources' | 'library' | 'desktop' | 'advanced'

const diagrams: Record<SettingsDiagramKind, Diagram> = {
  repository: {
    title: ['Connect your computers through your private repository', '用你的私有仓库连接电脑'],
    description: ['The local clone is a staging area, not your agent’s own resource folder.', '本地仓库副本是中转站，不是各个工具自己的资源目录。'],
    summary: ['This computer exchanges selected resources with a private Git repository; another computer connects to the same repository.', '这台电脑与私有 Git 仓库交换选定资源，另一台电脑连接同一个仓库。'],
    nodes: [
      { icon: 'computer', label: ['This computer', '这台电脑'], hint: ['Local repository clone', '本地仓库副本'] },
      { icon: 'repository', label: ['Private Git repository', '私有 Git 仓库'], hint: ['Owned by you', '由你拥有和控制'] },
      { icon: 'computer', label: ['Another computer', '另一台电脑'], hint: ['Same repository', '连接同一个仓库'] },
    ],
    caption: ['Only selected, supported resources are synchronized—not your entire home folder.', '只同步选定且受支持的资源，不会上传整个用户目录。'],
    bidirectional: true,
  },
  sync: {
    title: ['Choose when to sync and how long to keep deleted files', '选择何时同步，以及删除后保留多久'],
    description: ['Automatic checks keep resources up to date. The archive gives you time to recover deleted files.', '自动检查保持资源更新；可恢复归档为误删留出恢复时间。'],
    summary: ['A sync pulls remote changes, checks and applies safe changes, then pushes when there are changes to publish.', '一次同步先拉取远端，检查并应用安全变更，再按需推送本地变更。'],
    nodes: [
      { icon: 'repository', label: ['Pull changes', '拉取远端'], hint: ['Read the latest source', '获取最新内容'] },
      { icon: 'shield', label: ['Check and apply', '检查与应用'], hint: ['Conflicts need review', '冲突需要你处理'] },
      { icon: 'repository', label: ['Push if needed', '按需推送'], hint: ['Only new changes', '没有改动就不推送'] },
    ],
    caption: ['Deleted files stay recoverable during retention. Expired archives are cleaned after successful sync.', '删除的文件在保留期内仍可恢复；过期归档在成功同步后的清理阶段移除。'],
  },
  resources: {
    title: ['Sync only the resources you choose', '只同步你选择的内容'],
    description: ['Choose tools and resource categories. Safety filters still apply to every selected resource.', '先选择工具和资源类型，所有选定资源仍需通过安全过滤。'],
    summary: ['Selected tools and resource types pass safety filters before entering the private repository. Excluded files stay local.', '选定工具和资源类型通过安全过滤后进入私有仓库，被排除的文件留在本机。'],
    nodes: [
      { icon: 'tools', label: ['Your selection', '选择工具与类型'], hint: ['Tools and categories', '不是整目录复制'] },
      { icon: 'shield', label: ['Safety filters', '安全过滤'], hint: ['Exclude credentials and caches', '排除凭据与缓存'] },
      { icon: 'repository', label: ['Private repository', '私有仓库'], hint: ['Review the file preview', '可在下方查看预览'] },
    ],
    caption: ['This controls cross-computer sync. Library controls how compatible local tools use a shared source.', '这里管理跨电脑同步；共享资源页管理本机多个兼容工具如何使用一份资源。'],
  },
  library: {
    title: ['One source, compatible tools', '一份资源，多个兼容工具使用'],
    description: ['Add an existing skill or global instruction, then choose where to use it. Each write needs your approval.', '先加入已有技能或全局指令，再选择使用位置。每次写入都需要你确认。'],
    summary: ['Local resources enter your shared library after approval, then reach compatible tools through a separately approved copy.', '本机已有资源经确认后加入共享库，再经单独确认提供给兼容工具使用。'],
    nodes: [
      { icon: 'computer', label: ['Local resources', '本机已有资源'], hint: ['Claude / Copilot / common', 'Claude / Copilot / 公共目录'] },
      { icon: 'folder', label: ['Your shared library', '你的共享库'], hint: ['Maintain the source here', '以后在这里维护源文件'] },
      { icon: 'tools', label: ['Compatible tools', '兼容工具'], hint: ['Choose a supported location', '选择受支持的使用位置'] },
    ],
    caption: ['Scanning does not change files. This version shares skills and Claude global instructions—not plugin or configuration conversions.', '扫描不会改动文件。当前共享技能和 Claude 全局指令，不进行插件或配置格式转换。'],
  },
  desktop: {
    title: ['Work in the background, show tips when needed', '在后台工作，需要时再提醒'],
    description: ['Startup, the tray icon, and activity tips are separate controls.', '开机启动、托盘图标和活动提示各有不同作用。'],
    summary: ['When enabled, signing in starts SyncHub in the tray. Windows activity tips appear briefly without replacing background synchronization.', '启用自动启动后，登录时在托盘启动 SyncHub。Windows 活动提示短暂显示，不影响后台同步。'],
    nodes: [
      { icon: 'computer', label: ['Sign in', '登录系统'], hint: ['Start automatically if enabled', '按设置自动启动'] },
      { icon: 'tray', label: ['Run in the tray', '托盘中运行'], hint: ['Keep working in the background', '后台继续工作'] },
      { icon: 'clock', label: ['Activity tips', '活动提示'], hint: ['Windows: 5s / errors: 10s', 'Windows：5 秒 / 错误 10 秒'] },
    ],
    caption: ['Hiding activity tips does not stop synchronization or remove the tray icon.', '关闭活动提示不会停止同步，也不会关闭托盘图标。'],
  },
  advanced: {
    title: ['Start over only when you need to', '需要重新设置时，再来这里'],
    description: ['Reset is not a normal sync action. Read what will change before confirming.', '重置不是日常同步操作。先查看影响范围，再明确确认。'],
    summary: ['Review the reset impact, explicitly confirm, then complete connection setup again.', '先查看重置影响范围，明确确认后，再重新设置连接。'],
    nodes: [
      { icon: 'shield', label: ['Review the impact', '查看影响范围'], hint: ['What changes, what stays', '哪些会变，哪些保留'] },
      { icon: 'folder', label: ['Confirm explicitly', '明确确认'], hint: ['No automatic reset', '不会默认执行重置'] },
      { icon: 'computer', label: ['Set up again', '重新设置连接'], hint: ['Follow the setup steps', '跟随引导完成'] },
    ],
    caption: ['The confirmation below describes the actual impact. This diagram is an explanation, not a running operation.', '下方确认区域说明真实影响；示意图仅用于解释，不代表操作正在执行。'],
  },
}

function FlowIcon({ icon }: { icon: Icon }) {
  const paths: Record<Icon, React.ReactNode> = {
    computer: <><rect x="5" y="3" width="30" height="23" rx="3" /><path d="M1 31h38M16 26v5m8-5v5M11 10h12m-12 5h18m-18 5h7" /></>,
    folder: <><path d="M3 10V6a3 3 0 0 1 3-3h10l4 5h14a3 3 0 0 1 3 3v19H3z" /><path d="M3 12h34M14 21h12m-6-6v12" /></>,
    tools: <><rect x="3" y="3" width="14" height="14" rx="3" /><rect x="23" y="3" width="14" height="14" rx="3" /><rect x="13" y="23" width="14" height="10" rx="3" /><path d="M10 17v4h20v-4m-10 4v2" /></>,
    repository: <><path d="M9 28h23a7 7 0 0 0 1-14 12 12 0 0 0-23-2A8 8 0 0 0 9 28z" /><rect x="16" y="18" width="10" height="8" rx="2" /><path d="M18 18v-3a3 3 0 0 1 6 0v3" /></>,
    shield: <><path d="m20 3 14 5v10c0 8-14 15-14 15S6 26 6 18V8z" /><path d="m13 18 5 5 9-11" /></>,
    clock: <><circle cx="20" cy="18" r="14" /><path d="M20 9v10l7 4" /></>,
    tray: <><rect x="3" y="3" width="34" height="28" rx="3" /><path d="M3 24h34m-10 4h2m3 0h2" /><rect x="17" y="11" width="16" height="10" rx="2" /></>,
  }
  return <svg viewBox="0 0 40 36" aria-hidden="true" focusable="false">{paths[icon]}</svg>
}

export function SettingsCategoryIcon({ kind }: { kind: SettingsDiagramKind }) {
  const icons: Record<SettingsDiagramKind, Icon> = {
    repository: 'repository', sync: 'clock', resources: 'tools',
    library: 'folder', desktop: 'tray', advanced: 'shield',
  }
  return <FlowIcon icon={icons[kind]} />
}

export function SettingsDiagram({ kind, compact = false }: { kind: SettingsDiagramKind; compact?: boolean }) {
  const { t } = useI18n()
  const diagram = diagrams[kind]
  return (
    <div className={`settings-explainer${compact ? ' compact' : ''}`}>
      {!compact && <><h3>{t(...diagram.title)}</h3><p>{t(...diagram.description)}</p></>}
      <figure className="settings-flow" role="img" aria-label={t(...diagram.summary)}>
        <div className="settings-flow-nodes" aria-hidden="true">
          {diagram.nodes.map((node, index) => (
            <div className="settings-flow-step" key={node.label[0]}>
              {index > 0 && <span className="settings-flow-arrow">{diagram.bidirectional ? '↔' : '→'}</span>}
              <div className={`settings-flow-node${index === 1 ? ' source' : ''}`}>
                <FlowIcon icon={node.icon} />
                <strong>{t(...node.label)}</strong>
                <small>{t(...node.hint)}</small>
              </div>
            </div>
          ))}
        </div>
      </figure>
      {!compact && <details className="settings-flow-help">
        <summary>{t('How this works', '查看原理说明')}</summary>
        <p className="settings-flow-caption">{t(...diagram.caption)}</p>
      </details>}
    </div>
  )
}
