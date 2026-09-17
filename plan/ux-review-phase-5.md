# Phase 5 UX review — 2026-09-17

Scope: the public Digibox display and handset at desktop width and 320 px phone width. The
firmware-rendered menu is the guest's original interface; this review covers the authored page.

## Walk and finding

The desktop page gives the television and handset equal prominence, names the Sky action, and
reports the verified Ready state in plain English. Light and dark views were legible. At 320 px,
the screen and handset stacked cleanly with no horizontal overflow, but scrolling Select into the
viewport left the framebuffer wholly above it: screen bottom `-200px`, Select top `326px`. A
visitor could press a key without seeing what the firmware did. This was a **High** usability
finding: it breaks the page's main interaction on phones.

The fix keeps the television visible at the top of a phone viewport while the handset scrolls
under it. Keyboard scrolling reserves room for the display, so focused keys stay below it. Sticky
positioning is disabled in portrait viewports shorter than 400 px. Short landscape views place the display and
handset side by side, with a sticky display in its own column. Desktop layout and the single
original canvas remain the same.

## Pattern briefs

- **Display and control together:** the page's own purpose requires seeing the screen when a key
  acts. CSS sticky positioning can retain an element as its scrolling ancestor moves, provided
  its containing block spans the controls. [MDN position](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/position), checked 2026-09-17. The mobile grid now lets the television stick within the full experience rather than ending its sticky range at the viewer section.
- **Focus below persistent content:** authored sticky content can hide a focused control; W3C
  recommends scroll padding as one remedy. [W3C WCAG 2.2, Focus Not Obscured](https://www.w3.org/WAI/WCAG22/Understanding/focus-not-obscured-minimum), checked 2026-09-17. The page reserves space above keyboard targets and disables sticky placement in short viewports.

## Verification

`tests/e2e/handset.spec.ts` tabs through every key at 320 × 700 and checks that the screen remains
inside the viewport while each focused key lies below the television. It also taps Sky and Select
in a mobile touch context and checks that the screen stays above Select. The focused `0` key and
display were inspected together in the screenshot. The full Playwright suite passed 9/9;
TypeScript and CSS lint passed. The deployed page is checked again after the CSS build is live.

A second mobile pass found the display offscreen at 568 × 320 landscape. The local Playwright
landscape test now checks that the display and Select are both visible, side by side, without
horizontal overflow. The deployed URL was checked at 568 × 320 after the public rebuild; the
screen and Select were visible together with no horizontal overflow, and the screenshot was
inspected.
