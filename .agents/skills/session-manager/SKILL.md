---
name: session-manager
description: Operate the session-manager CLI for local Codex sessions. Use when listing projects, providers, sessions, inspecting rollout JSONL metadata, forking sessions across model providers, or safely trashing and recovering Codex sessions.
license: Apache-2.0
compatibility: Requires the session-manager binary and read access to the local Codex sessions directory, usually ~/.codex/sessions.
---

# Session Manager

Use `session-manager` to inspect and manage local Codex session rollout files. Prefer JSON output for automation and avoid direct filesystem edits under `~/.codex/sessions`.

## Codex session model

Codex stores local conversation history as rollout JSONL files under:

```text
~/.codex/sessions/YYYY/MM/DD/rollout-<timestamp>-<session-id>.jsonl
```

Each rollout file is append-only session history. The first `session_meta` record is the identity metadata for the file and includes fields such as session id, working directory, source, thread source, and `model_provider`. A forked rollout may contain older historical metadata later in the file, so tools must treat the first `session_meta` as authoritative for listing and provider attribution.

Important implications:

- A session belongs to a provider because its rollout metadata says so; provider is not just a UI filter.
- Native Codex resume behavior may filter by current provider/account, so sessions from other providers can be hard to discover through the normal UI.
- Cross-provider copying should use `codex fork -c model_provider=<provider> <session-id>` through `session-manager fork`, because that lets Codex create a valid new session instead of manually editing JSONL metadata.
- Deleting should move the rollout file out of `~/.codex/sessions` while preserving its relative path. Do not remove files directly and do not edit JSONL in place.
- Codex may keep separate local state/index data. `session-manager` does not edit that database; it operates on rollout files and relies on Codex to validate/repair stale rollout paths when sessions are restored or discovered again.
- Some rollout files are subagent/internal child sessions. They are useful for debugging but should usually be hidden from user-facing parent-session views.

## Agent operating prompt

When this skill is active, follow this prompt:

```text
You manage local Codex session data through the session-manager CLI. Use JSON output for discovery and parsing. Never edit, delete, or move files under ~/.codex/sessions directly. For destructive or state-changing actions, run the matching -dry-run command first unless the user explicitly asked to execute immediately. Explain what will change before running fork, trash, or recover. Prefer project/provider/session ids from session-manager output over guessing paths.
```

If the binary is missing, tell the user the tool is not installed and suggest:

```sh
go install github.com/lulafun/session-manager@latest
```

## Safety rules

- Treat `~/.codex/sessions` as user data.
- Do not delete, rewrite, or move rollout files with shell commands.
- Use `session-manager trash` and `session-manager recover` for reversible changes.
- Use `-dry-run` before `fork` or `trash` unless the user explicitly asked you to perform the change.
- Default behavior hides subagent/internal sessions. Add `-include-subagents` only when the user asks to debug child sessions.
- When the user asks to "delete", use `trash`, not `rm`.
- When the user asks to "copy", "fork", "move to another provider", or "resume under another provider", use `fork`.
- If a project selector is ambiguous, ask for the exact project label or cwd shown by `projects -json`.

## Decision guide

- User asks "what projects exist?" -> run `session-manager projects -json`.
- User asks "what providers exist?" -> run `session-manager providers -json`.
- User asks "sessions in this repo/project" -> run `session-manager projects sessions <project> -json`.
- User asks "find a session" -> run `session-manager list -query <text> -json`.
- User asks "show details" -> run `session-manager inspect -json <session>`.
- User asks "copy/fork to provider X" -> run `session-manager fork -to-provider X -dry-run <session>` first, then execute if confirmed or explicitly requested.
- User asks "delete/remove a session" -> run `session-manager trash -dry-run <session>` first, then execute if confirmed or explicitly requested.
- User asks "restore/recover" -> run `session-manager recover -dry-run <session>` first when possible.

## Common workflow

1. Discover projects:

   ```sh
   session-manager projects -json
   ```

2. Inspect providers:

   ```sh
   session-manager providers -json
   session-manager projects providers "~/Code/project" -json
   ```

3. List sessions:

   ```sh
   session-manager list -json -limit 20
   session-manager projects sessions "~/Code/project" -json
   session-manager list -provider openai -query "deployment" -json
   ```

4. Inspect a session:

   ```sh
   session-manager inspect -json <session-id-or-rollout-path>
   ```

5. Fork a session to another provider:

   ```sh
   session-manager fork -to-provider <provider> -dry-run <session-id-or-rollout-path>
   session-manager fork -to-provider <provider> <session-id-or-rollout-path>
   ```

6. Trash or recover a session:

   ```sh
   session-manager trash -dry-run <session-id-or-rollout-path>
   session-manager trash <session-id-or-rollout-path>
   session-manager recover <session-id-or-trashed-rollout-path>
   ```

## Interaction patterns

Use these response patterns when operating the tool for a user.

For read-only discovery:

```text
I will inspect the local session index with session-manager and use JSON output so the result is machine-readable.
```

For fork/copy:

```text
I found session <id> under provider <source-provider>. I will dry-run a fork to <target-provider> first, then run the real fork only after the command shape is correct.
```

For delete:

```text
I will use session-manager trash, which moves the rollout file to the recoverable trash directory and preserves its relative path. I will not remove files directly.
```

For ambiguous project matches:

```text
The project selector matches multiple projects. I need the exact label or cwd from the project list before acting.
```

## Command reference

- `providers -json`: returns configured and discovered model providers.
- `projects -json`: returns project cwd, `~` label, session count, provider counts, and latest update time.
- `projects sessions <project> -json`: returns sessions under one project. The project can be an absolute cwd, a `~`-relative label, or an unambiguous substring.
- `projects providers <project> -json`: returns provider counts under one project.
- `list -json`: lists sessions with filters such as `-provider`, `-cwd`, `-source`, `-query`, and `-limit`.
- `inspect -json <session>`: returns full metadata for one session.
- `fork`: calls `codex fork -c model_provider=<provider> <session-id>`.
- `trash`: moves a rollout file to the configured trash root while preserving relative path.
- `recover`: restores a trashed rollout file.

## TUI

Use the TUI only when the user wants an interactive browser:

```sh
session-manager ui
```

For non-interactive visual debugging, render a static view:

```sh
session-manager render -view detail -width 120 -height 28
```
