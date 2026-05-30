# Session Manager

[简体中文](README_zh.md)

**Session Manager is a CLI and Terminal UI for browsing, filtering, forking, deleting, and recovering local Codex session JSONL files across projects and model providers.**

Codex stores conversation rollouts under `~/.codex/sessions`. When you use multiple model providers or accounts, the native resume flow can make it hard to inspect sessions from another provider or project. Session Manager scans those rollout files directly, exposes automation-friendly CLI APIs, and provides a three-column TUI for human browsing.

Core features:

- Browse sessions by project, provider, source, query, and session id.
- Hide subagent/internal sessions by default, with `-include-subagents` for debugging.
- Inspect session metadata and recent user messages.
- Fork/copy a session into another model provider through `codex fork -c model_provider=<provider>`.
- Reversibly delete sessions by moving rollout JSONL files to a trash directory.
- Export machine-readable JSON for agent and script automation.

## Usage

### Installation

#### Download a release binary

Download the archive for your platform from GitHub Releases, unpack it, and place the binary on your `PATH`.

```sh
tar -xzf session-manager_<version>_darwin_arm64.tar.gz
chmod +x session-manager
mv session-manager /usr/local/bin/
```

Available release targets are built by GitHub Actions:

- `darwin/arm64`
- `linux/amd64`

#### Install with Go

Go 1.24.7 or newer is recommended.

```sh
go install github.com/lulafun/session-manager@latest
```

The installed command is:

```sh
session-manager
```

### Command Line API

The CLI is designed for scripts and AI agents. Most read commands support `-json`; commands that modify files are explicit.

List configured and discovered providers:

```sh
session-manager providers
session-manager providers -json
```

List projects and project-level provider counts:

```sh
session-manager projects
session-manager projects -json
session-manager projects sessions "~/Code/project" -json
session-manager projects providers "~/Code/project" -json
```

List and filter sessions:

```sh
session-manager list -limit 20
session-manager list -provider openai -json
session-manager list -query "deployment" -json
session-manager list -include-subagents
```

Inspect one session:

```sh
session-manager inspect <session-id-or-rollout-path>
session-manager inspect -json <session-id-or-rollout-path>
```

Fork/copy a session to another model provider:

```sh
session-manager fork -to-provider zaixiao_s2a -dry-run <session-id-or-rollout-path>
session-manager fork -to-provider zaixiao_s2a <session-id-or-rollout-path>
session-manager fork -to-provider zaixiao_s2a -C /Users/lula/code/Tools/session-manager <session-id-or-rollout-path>
session-manager fork -to-provider zaixiao_s2a --last -C /Users/lula/code/Tools/session-manager
```

`fork` can pass a target working directory to Codex with `-C`, `--cd`, or `--dir`. Use `--last` when you want Codex to fork the most recent session without resolving the session id yourself. These directory controls are for the CLI path; the TUI launches Codex's own interactive fork flow.

Delete and recover sessions:

```sh
session-manager trash -dry-run <session-id-or-rollout-path>
session-manager trash <session-id-or-rollout-path>
session-manager recover <session-id-or-trashed-rollout-path>
```

Deletion is reversible. The tool moves rollout files from `~/.codex/sessions` to `~/.codex/session-manager-trash/sessions/<relative path>` and does not rewrite the rollout JSONL file or edit Codex's SQLite state database.

Useful flags:

- `-root`: Codex sessions directory, default `~/.codex/sessions`
- `-config`: Codex `config.toml`, default `~/.codex/config.toml`
- `-provider`: filter by `model_provider`
- `-cwd`: filter by recorded working directory
- `-source`: filter by session source, such as `cli` or `vscode`
- `-query`: substring search over id, path, cwd, provider, branch, and preview
- `-limit`: maximum rows to print
- `-json`: machine-readable output
- `-include-subagents`: include subagent/internal non-root sessions
- `-to-provider`: target provider for fork/copy
- `-model`: optional model override for the forked session
- `-C`, `--cd`, `--dir`: target working directory for a forked session
- `--last`: fork the most recent Codex session
- `-dry-run`: print the operation without running it
- `-trash-root`: reversible delete directory, default `~/.codex/session-manager-trash`

### Terminal UI

Start the interactive browser:

```sh
session-manager ui
```

The main view is a three-column browser:

- Project
- Provider
- Session

Important keys:

- `h` / `l` or left / right: move between columns
- `j` / `k` or up / down: move within the focused column
- `enter`: open session details
- `f` / `F`: fork/copy the selected session to another provider
- `d` / `D`: delete the selected session after confirmation
- `/`: search
- `c`: clear cwd filter
- `esc`, left, `h`, or `q`: leave detail/copy views
- `q`: quit from the main view

Debug TUI states without entering the interactive app:

```sh
session-manager render -provider zaixiao_s2a -width 120 -height 28
session-manager render -query "deployment" -width 100 -height 24
session-manager render -view detail
session-manager render -view copy
session-manager render -view delete
```

### Agent Skill Usage

This repository includes an Agent Skill at:

```text
.agents/skills/session-manager/SKILL.md
```

Copy or symlink `.agents/skills/session-manager` into an agent's Skill folder so the agent knows how to operate the CLI safely. The skill teaches agents to prefer JSON output, avoid modifying `~/.codex/sessions` except through explicit `trash` or `recover` commands, and use `-dry-run` before fork/delete operations when appropriate.

## License

Apache License 2.0. See [LICENSE](LICENSE).
