import type { Translator } from './i18n'

const names: Record<string, string> = {
  idle: '空闲', ready: '就绪', updating: '正在更新', done: '完成', complete: '完成',
  completed: '已完成', pending: '待处理', queued: '已排队', running: '正在运行',
  failed: '失败', error: '错误', warning: '警告', paused: '已暂停',
  syncing: '正在同步', pulling: '正在拉取', scanning: '正在扫描',
  comparing: '正在比较', applying: '正在应用', uploading: '正在上传', pushing: '正在推送',
  skipped: '已跳过', blocked: '已拦截', install: '安装', update: '更新',
  reinstall: '重新安装', remove: '移除', dependencies: '依赖项', sessions: '会话', config: '配置',
  instructions: '指令', skills: '技能', plugins: '插件', mcp: 'MCP',
  'file-tree': '文件树', 'text-tree': '文本树', 'structured-merge': '结构化合并',
  'source-tree': '源码树',
}

/** Only known protocol labels are translated; unknown values remain inspectable. */
export function statusName(value: string, t: Translator) {
  return Object.prototype.hasOwnProperty.call(names, value) ? t(value, names[value]) : value
}
