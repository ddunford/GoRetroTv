# `.codex/` — what Codex CLI reads here

Verified against **codex-cli 0.154.0**. Version matters more than usual: 0.71.0 supported none of
this, and the two versions disagree about nearly everything below. Re-check after a Codex update
rather than trusting this page — the commands to do so are at the bottom.

## `hooks.json` — live, after you approve it once

`bd setup codex` generated it. It wires `bd codex-hook` to `SessionStart`, `UserPromptSubmit`,
`PreCompact` and `PostCompact`, which is how a Codex session gets the same tracker priming a Claude
session gets from its own `SessionStart` hook.

**It will not run until you approve it.** Codex has a startup hooks review: on your first
interactive session in this repo it shows the hooks a project wants to run and asks. That is a
security boundary worth having — a repo's config asking to execute commands is exactly the thing
that should need a human — so approve it knowingly rather than reflexively. Until then, and in
non-interactive `codex exec`, the hooks are silently inert.

Evidence: `codex features list` shows `hooks  stable  true`; the binary carries `hooks.json`, all
five event names and a `tui/src/startup_hooks_review.rs` module. A non-interactive
`codex exec` in a fresh untrusted directory did **not** fire a marker-writing `SessionStart` hook,
which is the approval boundary doing its job rather than a defect.

`bd codex-hook SessionStart` on its own works and emits ~25 KB — the ready frontier, the blocked
list and the `bd remember` notes.

## `config.toml` — inert, and harmless

**Codex reads config from `~/.codex/config.toml` only; a project-level one is ignored.** Proved by
putting `[features] hooks = false` in a project `.codex/config.toml` and seeing `codex features list`
still report `true`, and again with a different flag on the older version.

Its `[features] hooks = true` is therefore doing nothing — and needs to do nothing, since `hooks` is
already `stable` and on by default. Left in place because it costs nothing and is what bd writes.

## What else reaches a Codex session

- **`AGENTS.md`** — a symlink to `CLAUDE.md`, so Codex and Claude read the same instructions. This is
  the channel that works with no approval step, and it is why the symlink matters rather than being
  tidiness. `./ctl.sh agents` asserts it.
- **Skills**, from `.agents/skills/` and `~/.codex/skills`. Confirmed loading: a run reported
  "Skill descriptions were shortened to fit the skills context budget", so `.agents/skills/beads/`
  is visible to Codex.
- **The `bd` CLI**, via the shell, like any other command.

## Re-testing after a Codex update

```bash
codex --version
codex features list | grep -E '^hooks '     # want: hooks  stable  true
codex doctor                                # config load, auth, sandbox, MCP
```

To confirm hooks actually fire end to end, add a `SessionStart` hook that appends to a marker file,
start an interactive session, approve the review, and check the marker. Do not infer it from the
feature flag alone: the flag says the mechanism exists, not that this project's hooks were approved.
