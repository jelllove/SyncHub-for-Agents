# AgentConfigSync (`acsync`) — 设计文档

- **日期**: 2026-07-23
- **状态**: 已确认设计，待编写实现计划
- **作者**: 与用户协作头脑风暴产出

## 1. 目标与背景

用户在多台电脑（Windows / macOS / Linux 混合）上使用多个 AI agent 工具（Claude Code、GitHub Copilot CLI、Gemini CLI、Cursor 等）。希望有一个工具把这些 agent 的**配置文件**与 **session/历史对话文件**在多台机器间自动同步，使用**私有 GitHub 仓库**作为中转。

工具名暂定 **`acsync`**（AgentConfigSync），用 **Go** 实现，编译为跨平台单二进制，免运行时依赖。

## 2. 已确认的需求

| 项 | 决定 |
|----|------|
| 支持的 agent | 通用可扩展框架，内置 Claude / Copilot / Gemini / Cursor，用户可通过 YAML 扩展 |
| 平台 | Windows + macOS + Linux 混合 |
| 敏感信息 | **自动识别并排除**密钥文件/字段，敏感内容绝不上传 |
| session 同步 | 全量镜像（各机器求并集，每台都能看到全部历史） |
| 触发方式 | **定时**（默认 10 分钟，可配置）+ 手动"立即同步"；不做实时监听 |
| 删除处理 | **删除传播**：一台删除，其他机器同步删除 |
| 删除保护 | **回收站 + 30 天宽限期**（防误删、防 agent 自动清理误传播） |
| 冲突处理 | 最后写入胜出（LWW，按 mtime）；被覆盖版本留底 |
| 中转方式 | **私有 GitHub 仓库**（git 作为中转，自带版本历史/回滚） |
| 网络 | 各地分散、都能访问公网 |
| 实现语言 | **Go**（单二进制、跨平台分发） |
| 常驻/交互 | 开机自启、常驻后台；**Windows 系统托盘**，图标区分状态 |
| 设置界面 | 默认纳入所有已知 agent 目录；提供 Settings UI 可取消勾选（默认全选） |

## 3. 整体架构

本地维护一个"同步工作区"（一个私有 GitHub repo 的本地克隆），作为各 agent 目录与远端仓库之间的桥梁。

```
 ┌─────────────┐   收集/过滤    ┌──────────────────┐  git push/pull  ┌──────────────┐
 │ Agent 目录   │ ────────────▶ │ 本地同步工作区     │ ◀─────────────▶ │ 私有 GitHub   │
 │ ~/.claude    │ ◀──────────── │ ~/.acsync/repo   │                 │ repo (中转)   │
 │ ~/.copilot   │   应用/还原    └──────────────────┘                 └──────────────┘
 │ ~/.gemini …  │                        ▲
 └─────────────┘                         │ 每 10 分钟: pull → 合并 → push
```

同步周期内的完整流程：`git pull --rebase` → 三向合并（应用远端变化到本地 agent 目录）→ 收集本地变化（对比快照）→ 脱敏过滤 → commit → `git push`（失败则 pull-rebase 重试）。

## 4. 组件清单

1. **provider** — agent 声明式适配器 + 内置定义（YAML 驱动）
2. **pathresolver** — 跨平台路径解析（`~` / `%USERPROFILE%`）与换行符处理
3. **secret** — 敏感文件/字段扫描与拦截
4. **state** — 本地快照（上次成功同步时每个文件的 hash / mtime）
5. **syncengine** — 三向合并、删除传播、软删除、LWW、push 重试
6. **gitclient** — 封装 clone / pull --rebase / commit / push
7. **scheduler** — 10 分钟定时 + 手动触发
8. **tray** — 系统托盘 + 状态图标
9. **settings** — 本地网页设置页 + 配置读写
10. **cli** — `init / sync / status / daemon / install / restore` 等命令
11. **autostart** — 开机自启（Windows 注册表 Run / macOS LaunchAgent / Linux systemd user service）

## 5. 关键设计

### 5.1 Provider（可扩展适配器）

每个 agent 用一份声明式 YAML 描述。内置若干，用户可在 `~/.acsync/providers/*.yaml` 中新增，无需改代码。

```yaml
# providers/claude.yaml
name: claude
config:
  paths:
    windows: "%USERPROFILE%\\.claude"
    darwin:  "~/.claude"
    linux:   "~/.claude"
  include:
    - "settings.json"
    - "CLAUDE.md"
  sessions:
    - "projects/**/*.jsonl"
  exclude:
    - "**/.credentials.json"
    - "**/*token*"
    - "**/shell-snapshots/**"
secrets:
  key_patterns: ["apiKey","token","secret","password","oauth","refresh_token"]
```

### 5.2 敏感信息处理（secret）

- 文件名命中 `exclude` 规则 → 直接跳过，不进入仓库。
- JSON 文件命中 `key_patterns` → **整文件拦截**（不上传），并在日志/托盘提示。
- 目标：新机器可能需要重新登录/授权一次，这是用安全换便利的可接受折中。

### 5.3 删除传播（基于快照的三向同步）

不使用简单并集，而是像 git/Syncthing 那样比对**上次同步快照**：

- 本地 `state.json` 记录上次成功同步时每个文件的 `hash + mtime`。
- 每次同步三方比较：**本地现状** vs **上次快照** vs **远端（pull 后）**：
  - 快照有、本地现在没了 → 判定**本地删除** → 从仓库删除并传播。
  - 快照无、本地新增 → 上传。
  - 远端删除、本地未改 → 本地同步删除。
- 由此可区分"B 还没拉到 A 新建的文件"（不删）与"A 真的删了文件"（传播删除）。

### 5.4 删除保护（回收站 + 宽限期）

- 删除不立即物理清除，先移入仓库 `.trash/`，保留 **30 天**（可配置）后由后台清理任务真正删除。
- 作用：防误删；防某些 agent 自动清理旧 session 的删除被错误传播导致历史丢失。
- 30 天宽限期内可从 `.trash/` 或 git 历史恢复。

### 5.5 定时调度（scheduler）

- 后台每 **10 分钟**（可配置）执行一次完整同步。
- 托盘提供"立即同步"；两次定时之间可手动触发。
- 同步前对"最近几秒内 mtime 变化"的文件本轮跳过，避免同步到写了一半的文件。

### 5.6 并发编辑与 push 竞争

- 两台机器各改**不同 session** → 不同文件，三向合并天然无冲突，双方都同步且互不影响。
- 两台几乎同时 push → `git push` 非快进被拒 → `pull --rebase` 后重试（上限 N 次）。
- 同一共享配置文件双改 → 按 mtime **LWW**，被覆盖版本留在 `.trash/`（并有 git 历史）。

### 5.7 系统托盘与状态图标（tray）

- 使用 `fyne.io/systray` 实现跨平台托盘（**Windows 为主**；macOS 支持；Linux 需 appindicator 库）。
- 图标状态：
  - 🟢 **Idle** — 空闲
  - 🔄 **Updating** — 同步中
  - 🔴 **Error** — 出错（认证失败 / 冲突 / 无网络）
  - ⚪ **Paused** — 用户暂停
- 右键菜单：`立即同步` / `暂停·恢复` / `设置…` / `打开日志` / `退出`；并列出各 agent 的可勾选启用项。

### 5.8 设置界面（settings）

- **托盘菜单**：列出检测到的所有 agent，逐个可勾选（默认全选），取消即不同步该 agent。
- **完整设置页**：点"设置…"打开本地网页设置页（内嵌 HTTP + 浏览器，保持单二进制、无需原生 GUI 库）。可配置：
  - 每个 agent 的启用/禁用、要同步的子目录（config / sessions）
  - 同步间隔、GitHub repo 地址、删除保护开关与宽限期
  - 敏感排除规则查看

### 5.9 开机自启（autostart）

- Windows：注册表 `HKCU\...\Run` 项或启动目录。
- macOS：LaunchAgent（`~/Library/LaunchAgents`）。
- Linux：systemd user service。
- `acsync install` 一键配置自启，`acsync uninstall` 移除。

## 6. 仓库布局（远端 GitHub repo）

```
manifest.json                       # 机器注册表、全局元数据
agents/
  claude/
    config/
      settings.json                 # 共享配置，同名冲突走 LWW
    sessions/
      <project>/<uuid>.jsonl         # session 直接镜像（UUID 唯一，天然并集）
  copilot/ …
  gemini/ …
.trash/                             # 软删除区，超期真正清除
```

- 换行符统一以 LF 存仓库，检出到本地时按平台/需要还原。
- session 文件以 UUID 命名，跨机器求并集不会撞名；删除通过快照比对识别并传播。

## 7. 本地布局

```
~/.acsync/
  repo/            # 私有 GitHub repo 的本地克隆（同步工作区）
  state.json       # 上次同步快照（file → hash/mtime）
  config.yaml      # 用户设置（启用的 agent、间隔、repo 地址等）
  providers/       # 用户自定义 agent 适配器 YAML
  logs/            # 运行日志
```

## 8. CLI 命令

| 命令 | 作用 |
|------|------|
| `acsync init` | 首次配置：绑定私有 repo、克隆、检测 agent、写默认配置 |
| `acsync sync` | 手动执行一次完整同步 |
| `acsync status` | 显示当前状态、上次同步时间、待同步变化 |
| `acsync daemon` | 前台/后台启动守护进程（含托盘） |
| `acsync install` / `uninstall` | 配置/移除开机自启 |
| `acsync restore <path>` | 从回收站或 git 历史恢复文件 |

## 9. 错误处理

- **认证失败 / 无网络**：托盘转 🔴，记录日志，下个周期自动重试，不中断守护进程。
- **push 竞争**：`pull --rebase` 后重试，上限 N 次。
- **合并冲突**：LWW（mtime），旧版本进 `.trash/`。
- **文件正在写入**：静默期跳过本轮。
- **仓库损坏 / 首次为空**：`init` 幂等，自愈式重新克隆。

## 10. 测试策略

- **单元测试**：pathresolver（跨平台映射）、secret（正/负样本）、state 三向 diff（新增/修改/删除/删除传播）、LWW 决策。
- **集成测试**：用本地裸 git 仓库当"假 GitHub"，模拟两台机器（两个工作目录）跑完整 push/pull，验证：不同 session 并发→都同步；删除→传播；软删除→超期清除。
- **CI**：GitHub Actions 跑 `go test` + `go vet` + 三平台交叉编译。

## 11. 非目标（YAGNI）

- 不做实时文件监听（改为定时）。
- 不做 P2P / 局域网直连（机器分散且都能上公网，走 GitHub 中转即可）。
- 不做端到端加密存储（敏感内容采用"排除不上传"策略，而非加密）。
- 不做字段级智能三路合并（冲突用 LWW + 留底即可）。
