# Add and manage sources

A source is any camera, livestream, or video file that livestream-viewer can display. Add each source once, then reuse it on any number of screens.

Only administrators can add, change, probe, or delete sources.

## Add a source

1. Open **Stream Sources**.
2. Select **Add Source**.
3. Enter a recognizable **Name**.
4. Choose the **Kind** and complete the fields for that source.
5. Leave **Enabled** on and select **Create**.
6. Select **Probe** to test it before adding it to a screen.

## Choose the source kind

- **YouTube** — Paste the normal YouTube video or livestream address.
- **RTSP** — Use for most IP cameras and NVR camera feeds. Enter the camera address and, if needed, its username and password. Start with **TCP** transport; try **UDP** only if your camera or network requires it.
- **HLS** — Use for an HLS stream, usually an address ending in `.m3u8`.
- **DASH** — Use for a DASH stream, usually an address ending in `.mpd`.
- **HTTP** — Use for another video feed available over HTTP or HTTPS.
- **SRT** or **RTMP** — Use when the person supplying the feed identifies it by that name.
- **File** — Upload a video from your computer. Uploaded video is useful for an idle message or fallback image in motion.

If you are unsure which kind to choose, ask the camera or stream owner. Do not guess by changing a working address.

## Understand the options

- **Buffer (seconds)** — Adds a small delay to make an internet stream smoother. YouTube, HLS, and DASH start at four seconds. Increase it if playback pauses or jumps; reduce it when being close to live matters more.
- **Enabled** — An off source remains saved but will not play.
- **Force software decode** — Leave this off. Support may ask you to turn it on when the host’s video hardware or driver cannot play a particular feed.
- **Verify TLS certificate (RTSPS)** — Use only for secure RTSP addresses. Turn it on when the camera has a certificate your organization trusts.

## Read the probe status

Select **Probe** after adding or changing a source.

- **Hardware** — The host can use dedicated video hardware. This is preferred, especially for several streams at once.
- **Software** — The source works, but it uses more processor capacity.
- **Probed** — The source opened, but there is no specific hardware result.
- **Connecting** or **Reconnecting** — The wall is opening the stream or retrying after an interruption.
- **Offline** or **Failed** — The wall cannot currently play the source.
- **Unavailable** — The source exists but cannot currently be opened, such as a YouTube event that has not started.
- **Disabled** — The source is saved but its Enabled switch is off.
- **Idle** — The source is not currently playing and has no saved probe result.

Probe tests one source. Preview is the better test of whether all sources on a busy screen can run together.

## Add a camera

1. Confirm the camera or NVR feed works in its usual application.
2. Add it as **RTSP**.
3. Paste the RTSP or RTSPS address.
4. Enter the feed username and password when required.
5. Leave transport at **TCP** and buffer at `0` for the first test.
6. Create the source and select **Probe**.

If the camera offers a lower-resolution “substream,” use it for small grid tiles. It reduces load and often makes a multi-camera wall more reliable.

## Add a YouTube livestream

1. Add a **YouTube** source and paste its normal watch-page address.
2. Create it and select **Probe**.
3. If it works, no other setup is required.

If YouTube asks you to sign in or says the host looks like a bot:

1. Use a browser account that can watch the stream.
2. Follow the linked export instructions in the **YouTube → Configuration** section to create a Netscape-format `cookies.txt`.
3. Upload that file under **Stream Sources → YouTube → Configuration**.
4. Probe the source again.

Cookies are also needed for private, members-only, and age-restricted videos. They grant the add-on access as that browser account, so use a limited-purpose account where possible. Automated playback can cause YouTube to restrict an account.

The **Token provider** normally shows **Running** in the Home Assistant add-on. A running provider helps public playback but does not replace cookies when YouTube requires a signed-in session.

## Edit or delete a source

Use the pencil button to edit a source. For an RTSP password, leave the password field blank to keep the saved password.

Use the trash button to delete a source. A source that is still assigned to a screen cannot be deleted; remove it from those screens first. Deleting a source does not delete the camera or original stream.

## Next step

Add the source to a layout, playlist, or sequence in [Build screens and tours](screens-and-tours.md).
