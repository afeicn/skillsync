---
name: skillsync
description: "Manage local agent skills and keep them synced across machines with the skillsync CLI. Use when the user says sync skills / skill conflicts / 同步skill / skill对账 / 换电脑后skill不一致, or asks to check, merge, or push skill changes."
license: MIT
---

# skillsync (agent skill)

Drive the `skillsync` CLI to manage this machine's skills and keep them in sync
with the shared git repo.

## When to trigger

- "sync my skills" / "同步 skill" / "把 skill 推上去"
- "skill conflicts" / "skill 对账" / "另一台电脑改了 skill"
- after the user edits any SKILL.md and wants it everywhere

## Commands

```bash
skillsync status                      # three-way report (read-only, always safe)
skillsync sync                        # snapshot → reconcile → merge → verify → push
skillsync merge                       # start manual merge
skillsync resolve --keep-local A,B --keep-remote C
skillsync verify                      # secrets / size / frontmatter gate
```

## Rules

1. `status` first, then act. Never merge blind.
2. `sync` exit code 2 means a human must decide — show the user the
   `pending-*.md` report (path printed in output) and ask per skill:
   keep local / keep remote / hand-merge. Never pick a side yourself.
3. Never `git push --force`. Never commit `node_modules`, secrets, or >10MB files.
4. After a successful sync, tell the user which skills moved and which machine
   will receive them on next sync.
