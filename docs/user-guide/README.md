# livestream-viewer user guide

livestream-viewer turns a display connected to a small computer or Home Assistant host into a dedicated video wall. It can show cameras, YouTube livestreams, other web streams, and uploaded video files. The Home Assistant add-on is confirmed working on Raspberry Pi.

You manage the wall from a web browser. The browser is only the control panel: closing it does not stop the attached display.

## What you can do

- **Preview** — See what the attached display is showing, check each tile, and control a tour.
- **Stream Sources** — Add and test cameras, livestreams, and video files.
- **Display Strategy** — Build screens, choose layouts, and arrange a tour.
- **Users** — Give other people access. This page is available to administrators.

## How the pieces fit together

1. A **source** is a camera, livestream, or video file.
2. A **tile** is one area of the display.
3. A **screen** is one complete arrangement. A grid screen can show several sources; a transition screen shows a full-screen playlist.
4. A **sequence** changes sources inside one tile.
5. A **tour** changes from one whole screen to another.
6. **Dwell time** is how long an item or screen remains visible before moving on.

## Start here

- [Getting started](getting-started.md) — Sign in and create your first working display.
- [Add and manage sources](sources.md) — Cameras, YouTube, web streams, and files.
- [Build screens and tours](screens-and-tours.md) — Layouts, playlists, sequences, and transitions.
- [Operate and monitor the wall](operation.md) — Preview, manual controls, and status information.
- [Manage users](users.md) — Accounts, roles, and passwords.
- [Troubleshooting](troubleshooting.md) — Start with the symptom you see and collect useful diagnostics.

## Before you begin

You need:

- A host with livestream-viewer installed. Raspberry Pi running the Home Assistant add-on is a confirmed working host.
- A display connected to that host if you want physical display output.
- A computer or tablet that can open the management page.
- At least one stream address, camera address, or video file.

The product displays video only. It does not play audio, record cameras, detect motion, or show protected subscription services.
