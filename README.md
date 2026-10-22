# skillsync

**Git-based skill sync & conflict reconciliation for AI coding agents.**

Keep your agent skills (Claude Code, Codex, OpenCode, Cursor, Trae, Antigravity…)
identical across every machine you work on — with safe automatic merges and
explicit human control when two machines edited the same skill.

> 中文：把你在各 AI 编码工具里的 skill 统一放进一个 git 仓，多台电脑各自 `sync`：
> 单边改动自动合并推送，两边改了同一个 skill 会停住、写报告、弹通知，等你选边。

## Why

There is no native way to sync skills across machines
([anthropics/claude-code#36693](https://github.com/anthropics/claude-code/issues/36693)).
Installers (Vercel `skills` CLI) handle *getting* skills; dotfiles handle *copying* them;
nobody handles the hard part: **two laptops, same skill, different edits, what now?**

skillsync does a three-way reconciliation (local vs remote vs common base), classifies
every skill, merges what is safe, and refuses to guess when it isn't.

## Install

Grab a binary from [Releases](../../releases):

| Platform | Binary |
|---|---|
| macOS (Apple Silicon / Intel) | `skillsync-darwin-arm64` / `skillsync-darwin-amd64` |
| Windows | `skillsync-windows-amd64.exe` |
| Linux | `skillsync-linux-amd64` |
| Menu-bar app (macOS / Windows) | `skillsync-tray-*` |

Or build: `go build ./cmd/skillsync`.

## Quick start

```bash
# machine 1 — create an empty private/public repo first, then:
skillsync init --repo https://github.com/you/skills.git
# clones into ~/.skillsync/skills, writes config, symlinks skills into detected tool dirs

# edit skills as usual, then:
skillsync sync        # snapshot local, reconcile, merge safe changes, push

# machine 2 — same init, then:
skillsync sync        # pulls everything, pushes your own changes
```

## Commands

| Command | What it does |
|---|---|
| `init` | clone repo, write config, auto-detect tool dirs, link skills |
| `status` | three-way report: per-skill kind + advice |
| `sync` | unattended: snapshot → reconcile → merge → verify → push |
| `merge` | start a merge for manual reconciliation |
| `resolve` | `--keep-local a,b --keep-remote c` per-skill conflict choices |
| `verify` | pre-commit checks: secrets, size limits, SKILL.md frontmatter |
| `link` | (re)symlink skills into tool directories |
| `watch` | run `sync` on an interval |

## How `sync` decides

| Situation | Behaviour |
|---|---|
| one side added/changed/deleted a skill | merged and pushed automatically |
| both sides changed the same skill | **stops**, writes `pending-*.md`, sends a notification |
| uncommitted local changes | snapshotted into a commit first (never lost) |
| merged tree fails `verify` | rolled back, nothing pushed |
| `--force` | never. history is the shared contract |

Exit codes: `0` synced · `1` error · `2` needs a human.

## Skill for your agent

`skill/SKILL.md` teaches your coding agent to run skillsync for you. Install it:

```bash
ln -s "$(pwd)/skill" ~/.claude/skills/skillsync
```

Then just tell your agent: *"sync my skills"* or *"check skill conflicts"*.

## Config

`~/.config/skillsync/config.toml` (Windows: `%AppData%\skillsync\config.toml`):

```toml
repo = "https://github.com/you/skills.git"
branch = "main"
skills_dir = "~/.skillsync/skills"
interval_minutes = 60
targets = ["~/.claude/skills", "~/.codex/skills", "~/.config/opencode/skills"]
ignore = ["node_modules", "__pycache__", ".DS_Store"]
max_file_mb = 10
# optional; auto-detected as scripts/pii-check.{sh,py} when empty
pre_sync_cmd = ""
```

## Pre-sync gate

If the repo contains `scripts/pii-check.sh` (or `.py`), every `sync` runs it
after snapshotting and after merging — **before any push**. A failing gate
aborts the merge, blocks the push, writes a `pending-*.md` report and sends a
notification. Override the command with `pre_sync_cmd`.

## Security

`verify` blocks commits containing likely secrets (`sk-…`, `ghp_…`, `AKIA…`,
inline api keys), oversized files, and ignored paths. Unattended sync runs the
same gate before every push.

## Roadmap

- [x] pre-sync gate (PII / custom checks) — v0.1.2
- [ ] `skillsync ui` — local management dashboard
- [ ] `skillsync vendor` — lockfile + reinstall for third-party skills
- [ ] brew tap
- [ ] team mode (shared repo, per-member branches)

## License

MIT
