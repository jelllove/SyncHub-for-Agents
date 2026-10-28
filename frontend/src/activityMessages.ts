import type { Translator } from './i18n'

const messages: Record<string, string> = {
  Ready: '就绪', Syncing: '正在同步', Paused: '已暂停', Pulling: '正在拉取',
  Scanning: '正在扫描', Comparing: '正在比较', Applying: '正在应用', Pushing: '正在推送',
  'Sync complete': '同步完成', 'Sync error': '同步错误', 'Needs attention': '需要处理',
  'SyncHub for Agents is running in the tray.': 'SyncHub for Agents 正在托盘中运行。',
  'Preparing to synchronize selected resources.': '正在准备同步所选资源。',
  'Automatic synchronization is paused.': '自动同步已暂停。',
  'Retrieving the latest changes from your sync repository.': '正在从同步仓库获取最新更改。',
  'Finding portable resources and applying safety exclusions.': '正在查找可移植资源并应用安全排除规则。',
  'Comparing local and remote resources.': '正在比较本地和远程资源。',
  'Applying synchronized resources on this computer.': '正在此电脑上应用同步资源。',
  'Uploading synchronized changes to your sync repository.': '正在将同步更改上传至同步仓库。',
  'Your selected resources have been synchronized.': '所选资源已同步。',
  'Synchronization failed. Open SyncHub for Agents to review the details.': '同步失败。请打开 SyncHub for Agents 查看详情。',
  'Review conflicts, blocked files or pending installations in the app.': '请在应用中审查冲突、拦截的文件或待安装项目。',
  'Synchronization notices have been reviewed.': '同步提示已审阅。',
  'Pulling latest repository changes': '正在拉取仓库最新更改',
  'Scanning portable resources': '正在扫描可移植资源',
  'Comparing local and remote resources': '正在比较本地和远程资源',
  'Applying synchronized resources': '正在应用同步资源',
  'Uploading synchronized changes': '正在上传同步更改',
  'Synchronization complete': '同步完成',
  'Synchronization needs attention': '同步需要处理',
}

/** Translate only exact, known backend activity messages, never diagnostic substrings. */
export function activityMessage(value: string, t: Translator) {
  return Object.prototype.hasOwnProperty.call(messages, value) ? t(value, messages[value]) : value
}
