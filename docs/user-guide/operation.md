# Operate and monitor the wall

Open **Preview** for day-to-day monitoring. Closing the browser does not stop the physical display or its tour.

## What Preview shows

- The composed video being sent to the attached display.
- The active screen and the next screen in the tour.
- Whether the tour is running or paused.
- The dwell time remaining.
- Each tile’s source, fit, decoder, and current error.

When the display engine is disabled or unavailable, Preview shows the planned layout and source names instead of live composed video.

## Control the tour

Administrators can use:

- **Previous** — Go to the previous tour entry.
- **Next** — Go to the next tour entry.
- **Pause** — Hold the current state.
- **Resume** — Continue automatic movement.
- **Go to screen** — Immediately show a selected screen and hold it.

If **Go to screen** is used, select **Resume** when you want the normal tour to continue.

## Read common Preview messages

- **Display engine is not running** — The management page is available, but livestream-viewer is not driving a physical display. Check the add-on’s **Display engine** option and restart it.
- **Preview stream unavailable** — Too many browser preview tabs may be open. Close the other tabs, wait a few seconds, and select **Retry**.
- **No active screen** — Create and save a screen. Add it to an enabled tour, or choose it with **Go to screen**.
- **Tour paused** — Select **Resume** if the wall should advance automatically.
- **Event stream idle** — The browser is not receiving live status updates. Refresh the page. If it returns, restart the add-on.
- **YouTube sign-in or bot message** — Upload browser cookies as described in [Add a YouTube livestream](sources.md#add-a-youtube-livestream).

## Routine checks

At the start of a shift or event:

1. Confirm the attached display is on and using the correct input.
2. Open **Preview** and compare it with the physical display.
3. Confirm the intended screen or tour is active.
4. Check for warning or error messages.
5. Watch long enough to see each important source and at least one tour change.

## When playback is not smooth

Start with changes that reduce work:

1. Use lower-resolution camera substreams for small tiles.
2. Show fewer simultaneous sources.
3. Prefer sources marked **Hardware** over **Software**.
4. Increase the buffer slightly for YouTube, HLS, or DASH feeds.
5. Use **cut** or a short transition.

Change one thing at a time and check Preview again. More checks are in [Troubleshooting](troubleshooting.md).
