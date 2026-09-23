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

## Tasks (mirror — bd epic `gort-qxl` is the source of truth; never hand-ticked)

- [ ] `TASK-7.1` **Premise corrected 2026-09-22 — the four flash addresses this task was written
  around do not exist in this port's execution OR its data flow.** `0x9FC75F2F`, `0x9FC75F7B`,
  `0x9FC762FD` and `0x9FC77D41` never execute (6,488,065 instructions during the draw, every one in
  RAM) and are never read either — the grid is interpreted OpenTV o-code, so the reconciliation that
  an interpreter fetches bytecode as DATA was tested too, and the draw's 40,980 flash reads across
  50 pages include none of them. They are the browser oracle's instrumentation. **Find instead where
  the grid gets its channel list**, starting from what it demonstrably walks: the table-shaped
  regions `0x8045D000` (10,752 reads over 512 distinct addresses — a whole page) and `0x80494000`
  (17,025 over 325), and the o-code itself at `0x9FC5D000` (6,188 over 381). Ruled out by
  measurement and not to be re-run: the box holds six service records matching the broadcast, the
  grid reads none of them, and it never handles our channel numbers at all → `/go-engineer` [TC-7.1]
- [ ] `TASK-7.2` Determine whether the loop runs and draws nothing, or never runs. Different faults, same blank screen; the widget count per iteration separates them → `/go-engineer` [TC-7.1]
- [x] `TASK-7.3` **Premise corrected 2026-09-21 — do not "feed PID `0x52`".** The record already
  settles it: `0x52` is transient, opens in response to our own NIT and closes again, and is the
  box's OWN subtable registration for network `0x20`; the earlier "nothing feeds PID `0x52`"
  reading is named in the record as an artefact of a join. The live question is instead: **signal
  an application in the programme stream and find how this firmware expects to see it.** OpenTV 1.x
  auto-downloads a flow's directory module "when an application is signalled in the programme
  stream" (LTU 2000:075 §3.3–3.6), and we signal none. PAT/PMT is the cheap standard-DVB first
  attempt, NOT a diagnosis — the record mentions PAT and PMT zero times and no observation shows a
  PID `0x00` filter. Pass/fail is `0x800BECF0` executing once → `/go-engineer` [TC-7.2]
- [ ] `TASK-7.4` Establish what makes the box ask for a carousel at all. Every carousel-side function is currently cold; `0x800BECF0`, the Huffman decompressor, **has never executed once** and is the cleanest marker that a module was accepted → `/go-engineer` [TC-7.3]
- [ ] `TASK-7.5` Once it asks: the module format, read off the parser the way the `0xB1` entry layout
  was read. **Not DSM-CC — corrected 2026-09-21.** OpenTV 1.x uses module "flows" with a directory
  module, not a DSM-CC object carousel (LTU 2000:075). Building to DSM-CC would be the guessed
  fifth format this phase's own rule forbids → `/go-engineer` [TC-7.4]
- [ ] `TASK-7.6` Establish whether the firmware drives audio at all — a measurement, not an implementation task → `/go-engineer` [TC-7.5]
- [ ] `TASK-7.7` Implement whatever 7.1–7.6 prove is needed; the shape cannot honestly be planned before they run → `/go-engineer` [TC-7.1, TC-7.2, TC-7.3]
- [ ] `TASK-7.8` ⫘ Playwright: navigate sky → TV GUIDE → ALL CHANNELS and assert channels with programmes → `/qa-test-engineer` [TC-7.6]
- [x] `TASK-7.10` **Answered 2026-09-22: SIX, matching the broadcast exactly.** The box's own
  18-byte service records, found by content, are one per announced service — so the restored line-up
  and the broadcast line-up do NOT disagree and the twelve-against-six hypothesis is dead. "Twelve"
  is the oracle's number in the oracle's address space. The grid does not iterate them in any case:
  zero reads of those records in 488,551 during the draw → `/go-engineer` [TC-7.7]
- [ ] `TASK-7.11` Broadcast the genre index so the TV GUIDE's eight category screens fill. The A-Z
  index ships and works; the SAME table feeds the genre screens and they are unfed. Table `0xC1` on
  PID `0x52`, fourth dispatch arm `0x0100..0x01CF`, slot `(ext & 0x0F) * 4 + ((ext & 0xC0) >> 6)` —
  sixteen categories by four six-hour blocks, boundaries measured on the box. **Which number is
  which genre is NOT established and must not be guessed from the menu's order**: open a genre
  screen and watch which slot it READS, the way the A-Z screen was solved in one run rather than
  ninety-two sweeps → `/go-engineer` [no-test: the acceptance IS a firmware probe + screenshot]
- [ ] `TASK-7.9` ⫘ Security audit → `/security-reviewer` [no-test: audit produces its own report]

## Closing gates

The phase epic carries **GATE: every test-plan case in phase 7 proved** and a security audit.

**No `/ux-review`, accessibility or mobile gate, deliberately.** The ALL CHANNELS grid is the
*firmware's* 1998 interface — we render it, we do not author it, and redesigning it would be the
opposite of the product. Those three gates live on phase 5, which owns the only surface this project
actually writes. Recorded here rather than silently omitted, so a later reader finds a decision
instead of a gap.

## Risk, stated plainly
These have no guaranteed completion date. If they prove unbounded the honest move is to ship the rest
and reopen them — **not** to redefine them as done. `/plan-reconcile` should treat a silently
narrowed FR-13/14/15 as drift.

## Custom Feature: the three unsolved screens

**Purpose:** The work the predecessor could not finish. **This is research before it is code**, and
the deliverable of tasks 7.1–7.6 is evidence, not an implementation.

**What is already ruled out, by measurement — do not re-test these:**

| Hypothesis | Verdict |
|---|---|
| The grid has no listings to show | Ruled out — there are no rows to fill |
| The line-up is incomplete | Ruled out — twelve services with numbers and flags |
| The `0xB1` flag bits are wrong | Ruled out |
| The grid uses the notification model | Ruled out — it registers no slot |
| The 18-byte service record array feeds it | Ruled out — nothing reads it while drawing |
| `DS+0x02E20C` is a channel count | **Wrong reading, withdrawn** — it feeds `add -12; switch` and is a view selector |

**Interfaces:** none can honestly be named before 7.1–7.6 report. TASK-7.7 exists to hold the work
they prove is needed and **must be decomposed into real tasks when they do** — a task that says
"implement whatever is needed" is a placeholder, and closing it as-is would be the silent narrowing
this phase's risk note forbids.

**Key patterns:**
- **Do not guess a format.** Four section formats were built, fed and measured; a fifth guessed one
  is how the predecessor's last day would have been wasted. Let the box name what it wants.
- **`0x800BECF0` — the Huffman decompressor — has never executed once in this project's history.**
  It is the cleanest single marker that a carousel module was accepted.
- **A census showing everything cold is a valid result** and must be reported as one, with a live
  control proving the census can see anything at all.

**Test checklist:**
- [ ] The grid's row loop is named by address, with evidence of whether its body executes
- [ ] PID `0x52`'s consumer is identified by the firmware, with a working control
- [ ] `carouselAcquire` runs with its arguments captured
- [ ] Whether audio is driven at all is answered — a no is an acceptable answer
