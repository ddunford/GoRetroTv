# Where the host deliberately intervenes, and why it always says so

The emulator's normal state is that the guest firmware runs and the host only models hardware. There
are a small number of places where that is not true — where the host reaches in and changes guest
state directly. Each one is deliberate, each is declared, and **each is reported explicitly rather
than folded quietly into a run.**

They are collected here because they are the thing most likely to mislead someone comparing two
runs: an intervention that fires in one run and not the other explains a divergence that otherwise
looks like a CPU or device fault. Individually they are documented at their call sites; what is
nowhere else is the list, and what each one costs.

## 1. Application handoff — always on

The box's own bootloader runs to completion and then goes idle, waiting on something a real box gets
from hardware this port does not model yet. Once it has been idle long enough and the flash header
matches the ROM magic, the host sets **three CPU registers only** — PC, ISA and RA — so execution
lands on the application's entry stub. The guest then decompresses and runs its own application.

**What it is not:** a host-performed ROM transfer. The guest does the decompression. Describing it
as anything else overstates what the host did.

**The cost:** none to the comparison — the oracle declares the same policy, so both sides do it.

**The trap already paid for:** the idle threshold counts *covered instructions*, not calls. Counting
calls turned a 200,000-instruction gate into a 3.2-million-instruction one, because the device pump
is batched.

<!-- anchor: internal/machine/handoff.go#^func \(h \*Handoff\) Tick -->
<!-- fingerprint: sha256:e2ea04d9d2a203715d1278e185b0b98926d4e232fd3607057b627622fbd5b15f @ 2026-09-22 -->

## 2. The Sky menu gates — optional, off by default

After the guest has created forty-two tasks, this policy answers two checks the application makes,
by changing **one RAM word and one flash byte**. With it, the Sky menus present; without it, a clean
hardware run stops short of them.

**The cost, and why it is a flag:** it is a presentation policy, not hardware. A clean hardware
oracle run must be able to exclude it, so it is opt-in (`-sky-gates`) and its effect is reported
rather than assumed.

<!-- anchor: internal/machine/sky_gates.go#^func \(g \*SkyGates\) Tick -->
<!-- fingerprint: sha256:614888b32c5cd15e55a296d057b0cf29f71167cdf61e5541a8a1522eef0ad8fd @ 2026-09-22 -->

## 3. Card acknowledgement — on, and divergent from the oracle by design

The box sends two command codes on every key press. Leaving them unanswered starves the task that
drains its event queue, and the box stops taking input at all. The modelled smartcard therefore
answers what the box asks for.

**This is the one place the port deliberately differs from the oracle**, because the browser
emulator is silent on those codes too. The comparison is run in the oracle's declared condition
(`-ack-oracle`), exactly as it is already run without `-sky-gates`.

**The rule that makes this safe:** the oracle file is never edited to agree with the port. The
divergence is declared on the comparison's side instead.

## What must stay true

- **Every intervention is reported explicitly in a run's output.** A silent one is indistinguishable
  from a device model that happens to behave that way — which is exactly the confusion these notes
  exist to prevent.
- **A clean hardware run can exclude every optional policy.** If a policy cannot be turned off, the
  oracle comparison has lost its meaning.
- **The oracle is never edited to agree.** Where the two disagree, the measured record decides; where
  the record is silent, measure.
- **Interventions are the first thing to check when two runs diverge** — before suspecting the CPU.

## Open

- Handoff exists because the bootloader waits on hardware this port does not model yet. Whether that
  hardware will eventually be modelled — retiring the intervention — is not recorded anywhere, and
  is a question for whoever owns the device roadmap rather than a gap in this document.
