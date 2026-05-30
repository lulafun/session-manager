# Session Manager

**Session Manager 是一个用于跨项目、跨 model provider 浏览、筛选、复制 fork、删除与恢复本地 Codex session JSONL 文件的 CLI 与 Terminal UI 工具。**

Codex 默认把会话 rollout 文件保存在 `~/.codex/sessions`。当你使用多个 model provider 或不同账户时，原生 resume 流程不方便查看另一个 provider 或项目下的 session。Session Manager 会直接扫描这些 rollout 文件，提供适合自动化调用的 CLI API，也提供适合人工浏览的三栏 Terminal UI。

核心能力：

- 按项目、provider、source、query、session id 浏览 session。
- 默认隐藏 subagent/internal session，调试时可用 `-include-subagents` 打开。
- 查看 session 元数据和最近用户消息。
- 通过 `codex fork -c model_provider=<provider>` 将 session 复制 fork 到另一个 provider。
- 通过移动 JSONL 文件实现可恢复删除。
- 为脚本和 AI agent 提供 JSON 输出。

## 使用方法

### 安装方法

#### 从 Release 下载二进制文件

从 GitHub Releases 下载对应平台的压缩包，解压后把二进制文件放到 `PATH` 中。

```sh
tar -xzf session-manager_<version>_darwin_arm64.tar.gz
chmod +x session-manager
mv session-manager /usr/local/bin/
```

当前 Release 工作流会构建：

- `darwin/arm64`
- `linux/amd64`

#### 使用 Go 原生方式安装

建议使用 Go 1.24.7 或更新版本。

```sh
go install github.com/lulafun/session-manager@latest
```

安装后的命令是：

```sh
session-manager
```

### 命令行 API 使用方式

CLI 面向脚本和 AI agent 设计。大多数读取命令支持 `-json`；会修改文件的命令都是显式命令。

查看配置和 session 中发现的 provider：

```sh
session-manager providers
session-manager providers -json
```

按项目查看 session 和 provider 分布：

```sh
session-manager projects
session-manager projects -json
session-manager projects sessions "~/Code/project" -json
session-manager projects providers "~/Code/project" -json
```

列出并筛选 session：

```sh
session-manager list -limit 20
session-manager list -provider openai -json
session-manager list -query "deployment" -json
session-manager list -include-subagents
```

查看某个 session：

```sh
session-manager inspect <session-id-or-rollout-path>
session-manager inspect -json <session-id-or-rollout-path>
```

把 session 复制 fork 到另一个 model provider：

```sh
session-manager fork -to-provider zaixiao_s2a -dry-run <session-id-or-rollout-path>
session-manager fork -to-provider zaixiao_s2a <session-id-or-rollout-path>
session-manager fork -to-provider zaixiao_s2a -C /Users/lula/code/Tools/session-manager <session-id-or-rollout-path>
session-manager fork -to-provider zaixiao_s2a --last -C /Users/lula/code/Tools/session-manager
```

`fork` 可以通过 `-C`、`--cd` 或 `--dir` 把目标工作目录传给 Codex。需要复制最近一个 session 时可以使用 `--last`，不必先手动解析 session id。这些目录控制能力只面向 CLI；TUI 会进入 Codex 自己的交互 fork 流程。

删除和恢复 session：

```sh
session-manager trash -dry-run <session-id-or-rollout-path>
session-manager trash <session-id-or-rollout-path>
session-manager recover <session-id-or-trashed-rollout-path>
```

删除是可恢复的。工具会把 rollout 文件从 `~/.codex/sessions` 移动到 `~/.codex/session-manager-trash/sessions/<relative path>`，不会重写 JSONL 文件，也不会编辑 Codex 的 SQLite state 数据库。

常用参数：

- `-root`：Codex sessions 目录，默认 `~/.codex/sessions`
- `-config`：Codex `config.toml`，默认 `~/.codex/config.toml`
- `-provider`：按 `model_provider` 筛选
- `-cwd`：按 session 记录的工作目录筛选
- `-source`：按 session source 筛选，例如 `cli` 或 `vscode`
- `-query`：在 id、path、cwd、provider、branch、preview 中做子串搜索
- `-limit`：限制输出行数
- `-json`：机器可读 JSON 输出
- `-include-subagents`：包含 subagent/internal 非根 session
- `-to-provider`：fork/copy 的目标 provider
- `-model`：fork session 时可选的 model override
- `-C`、`--cd`、`--dir`：fork 后的目标工作目录
- `--last`：fork 最近一个 Codex session
- `-dry-run`：只打印操作，不真正执行
- `-trash-root`：可恢复删除目录，默认 `~/.codex/session-manager-trash`

### Terminal UI 使用方式

启动交互式浏览器：

```sh
session-manager ui
```

主界面是三栏结构：

- Project
- Provider
- Session

常用快捷键：

- `h` / `l` 或左 / 右方向键：在栏目之间移动
- `j` / `k` 或上 / 下方向键：在当前栏目内移动
- `enter`：打开 session 详情
- `f` / `F`：把选中的 session 复制 fork 到另一个 provider
- `d` / `D`：确认后删除选中的 session
- `/`：搜索
- `c`：清除 cwd 筛选
- `esc`、左方向键、`h` 或 `q`：从详情/copy 视图回退
- `q`：在主界面退出

不进入交互界面也可以渲染 TUI 状态，方便调试：

```sh
session-manager render -provider zaixiao_s2a -width 120 -height 28
session-manager render -query "deployment" -width 100 -height 24
session-manager render -view detail
session-manager render -view copy
session-manager render -view delete
```

### 基于 Skill 的使用方式

本仓库提供了 Agent Skill：

```text
.agents/skills/session-manager/SKILL.md
```

可以把 `.agents/skills/session-manager` 复制或软链接到 agent 的 Skill 文件夹中。这样 AI agent 就会知道如何安全地操作这个 CLI：优先使用 JSON 输出，除显式 `trash` 或 `recover` 命令外不修改 `~/.codex/sessions`，并在 fork/delete 等操作前优先使用 `-dry-run` 验证。

## License

Apache License 2.0。详见 [LICENSE](LICENSE)。
