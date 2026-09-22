
### Never `git reset --hard` to undo a test commit

<!-- anchor: none - a working-practice lesson; its subject is how we work, not a file -->

Undoing a throwaway commit with `git reset --hard HEAD~1` discards every *uncommitted* tracked
modification in the tree as well as the commit. I used it to clean up a two-line control commit and
destroyed forty minutes of unstaged work across seven files — registry rows, two harness checks, the
hook wiring and the ctl.sh verbs — none of which the command mentioned.

`--soft` moves the branch and keeps everything; `--mixed` (the default) unstages but keeps the
files. Neither touches the working tree. `--hard` is only correct when you have verified there is
nothing uncommitted you want, and in a tree another agent may be writing to it is never correct.

Better still, do not make the throwaway commit: to prove a pre-commit hook blocks or admits
something, stage the files and run the hook directly (`.githooks/pre-commit`) and read its exit code.

### Never background a compound command that contains a git commit

<!-- anchor: none - a working-practice lesson; its subject is how we work, not a file -->

A `git checkout -b … && mutate && commit && push` chain hit the 120s tool timeout at beads'
post-checkout hook and was moved to the background. I assumed it had stalled at the checkout, redid
the work by hand, returned to master — and then the background job woke up and ran its remaining
steps against whatever branch was checked out by then, committing a deliberate ARCH-PLAY-1 violation
onto master. The gate caught it (six rules UNPROVEN, the "clean corpus" check reporting the corpus
as violating), which is the only reason it did not get pushed.

Two rules from it: run git commits in the foreground, one command at a time, so a timeout leaves a
state you can read; and when a backgrounded command's output looks truncated, kill it before doing
the work again by hand — a half-finished chain is not a stalled one.

### Verify with an instrument before reporting, and never pipe its detail away

<!-- anchor: none - a working-practice lesson; its subject is how we work, not a file -->

I ran every conformance instrument over the whole estate for the first time, piped the stop-gate
through `tail -2` to keep the output short, got `STOP-GATE FAILED`, and had already written the
summary calling the estate green. The detail I had discarded was the only thing that said which rule
and why.

Two rules from it. Summarise a check's output only after you have read it in full — `tail` on a
verification is the same mistake as a checker that truncates a response. And when several
instruments exist, run them TOGETHER before believing any of them: the failure that run exposed was
two instruments disagreeing with each other, which neither could show alone.

### On a demo, a revert that buries an unexplained bug is worse than the bug

<!-- anchor: none - a working-practice lesson; its subject is how we work, not a file -->

I broke the live demo, guessed at the cause from a harness symptom ("the box never settled"),
invented a scaling story to fit it, and started reverting to the last known-good build. The owner
stopped me: *"no it's not, just fix it properly this is a demo."*

They were right twice over. The guess was wrong — the real fault was a function my own patch had
deleted, and the machine's error field said so verbatim — and reverting would have taken the
evidence off the host along with the fault, leaving a bug that would come back the next time
anyone touched that file.

So: a demo host has no users to protect, which is exactly what makes it the cheapest place to
leave a fault standing while you diagnose it. Revert when the cause is known and the fix is not
ready, not to make a symptom go away. And before proposing a revert at all, read the error the
machine recorded — not the one the harness reported.

### Verify against the URL the person is looking at, not the one your tooling defaults to

<!-- anchor: none - a working-practice lesson; its subject is how we work, not a file -->

Three times in a session I reported "verified end to end" from `localhost:5173` while the owner had
the demo host open. Every claim was true of the dev server and false of the thing with a URL.

The probe runner takes `--url`; the last check before telling anyone something works has to use it.
And watch for tooling that rewrites the URL — this runner appends `?si=0`, so passing the bare page
does not reproduce a visitor's experience either.
