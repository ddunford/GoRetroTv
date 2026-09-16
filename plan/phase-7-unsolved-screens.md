# Phase 7: The unsolved screens

## Outcome
The ALL CHANNELS grid lists channels and programmes; the box accepts an object carousel; menu audio
plays. The three things the predecessor could not finish.

## Overview
**This phase is research before it is code**, and it is scheduled last deliberately: it needs phase
4's snapshots and debugger, without which each hypothesis costs six minutes to test. The predecessor
had three hypotheses killed on the grid alone.

Everything already established is in `SPEC.md` §13 and `docs/reference/digibox-emulation.md`. The
rule for this phase: **do not guess a format** — let the box name what it wants.

## Tasks (mirror — bd epic `gort-7` is the source of truth; never hand-ticked)

- [ ] `TASK-7.1` Find the grid's row loop: disassemble outward from the executed reads of `DS+0x02E20C` at `0x9FC75F7B` and `0x9FC762FD` — **not** the one at `0x9FC77D41`, which feeds `add -12; switch` and is a view selector rather than a channel count → `/go-engineer` [TC-7.1]
- [ ] `TASK-7.2` Determine whether the loop runs and draws nothing, or never runs. Different faults, same blank screen; the widget count per iteration separates them → `/go-engineer` [TC-7.1]
- [ ] `TASK-7.3` Feed PID `0x52` — the one PID the box arms that nobody has ever fed — and identify what consumes it → `/go-engineer` [TC-7.2]
- [ ] `TASK-7.4` Establish what makes the box ask for a carousel at all. Every carousel-side function is currently cold; `0x800BECF0`, the Huffman decompressor, **has never executed once** and is the cleanest marker that a module was accepted → `/go-engineer` [TC-7.3]
- [ ] `TASK-7.5` Once it asks: the DSM-CC module format, read off the parser the way the `0xB1` entry layout was read → `/go-engineer` [TC-7.4]
- [ ] `TASK-7.6` Establish whether the firmware drives audio at all — a measurement, not an implementation task → `/go-engineer` [TC-7.5]
- [ ] `TASK-7.7` Implement whatever 7.1–7.6 prove is needed; the shape cannot honestly be planned before they run → `/go-engineer` [TC-7.1, TC-7.2, TC-7.3]
- [ ] `TASK-7.8` ⫘ Playwright: navigate sky → TV GUIDE → ALL CHANNELS and assert channels with programmes → `/qa-test-engineer` [TC-7.6]
- [ ] `TASK-7.9` ⫘ Security audit → `/security-reviewer` [no-test: audit produces its own report]

## Risk, stated plainly
These have no guaranteed completion date. If they prove unbounded the honest move is to ship the rest
and reopen them — **not** to redefine them as done. `/plan-reconcile` should treat a silently
narrowed FR-13/14/15 as drift.
