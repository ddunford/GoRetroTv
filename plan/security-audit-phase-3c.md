# Phase 3c security audit

Date: 2026-09-17  
Scope: CSI handset and peripheral framing, smartcard UART, I²C and demodulator, EEPROM persistence, snapshot restore, trace runner inputs, firmware loading, and current HTTP exposure.

## Findings and disposition

An independent read-only reviewer found no remotely reachable Phase 3c exploit. The HTTP application currently registers only `/health` and optional pprof; it does not expose the emulator, handset input, firmware, NVRAM, or guest-memory probes. The trace runner validates key and address flags. CSI, smartcard, and demodulator buffers have explicit bounds, and snapshot restores reject oversized fields. The firmware loader verifies the manifest SHA-256 values; firmware files are gitignored and mounted read-only.

One low-severity local robustness issue was fixed: `eeprom.Store.Load` read the whole operator-selected `--nvram` file before checking its length. It now reads at most the 16 KiB chip capacity plus one byte and rejects oversized images without changing the chip. `TestLoadRejectsOversizedImageWithoutChangingChip` covers this case. EEPROM persistence uses a same-directory temporary file, mode 0600, sync, close, and rename; I²C reports persistence errors and the runner checks them.

## Verification and limit

The reviewer performed a read-only code audit and did not execute the firmware or tests. The bounded-read fix is in commit `f3db62e`; its test and the project gates are run as part of Phase 3c close-out. The HTTP integration in Phase 5 needs its own security review when remote handset and framebuffer routes exist. Firmware, guest-memory probes, snapshots, and pprof remain private at that boundary.
