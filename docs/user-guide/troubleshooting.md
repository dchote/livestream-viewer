# Troubleshoot livestream-viewer

Start with what you see. Check one source by itself before changing a whole wall, and change one setting at a time.

## The add-on will not install

1. Confirm the Home Assistant host can reach the internet.
2. Confirm the repository address is exactly `https://github.com/dchote/livestream-viewer`.
3. Refresh the Add-on store and try again.
4. Open the Home Assistant Supervisor or system logs and record the installation error.

Do not repeatedly uninstall a previously working add-on without first recording its settings. Uninstalling may remove its saved data.

## Open Web UI is missing or does not load

1. Open **Settings → Add-ons → livestream-viewer** and confirm its state is **Running**.
2. If it is stopped, select **Start** and wait about 30 seconds.
3. Refresh Home Assistant, then select **Open Web UI** again.
4. If the page still does not load, restart the add-on once and check its log.

If the add-on stops again by itself, do not keep restarting it. Copy the final error messages from the log.

## You cannot sign in

1. For a new installation, use username `admin` and password `admin`, then set a new password.
2. Check Caps Lock and make sure your password manager selected the livestream-viewer entry.
3. Try a private browser window to rule out an old saved session.
4. Ask an administrator to confirm your account still exists.

After several failed attempts, sign-in is temporarily slowed for that computer. Wait at least 20 seconds before trying the verified password again.

There is no self-service password reset. If no administrator can sign in, contact the system owner before reinstalling; reinstalling may remove the display configuration.

## The browser preview works, but the attached display is blank

1. Confirm the display has power and is set to the correct HDMI or DisplayPort input.
2. Connect and turn on the display before restarting the add-on.
3. In the add-on **Configuration**, confirm **Display engine** is enabled.
4. Restart the add-on and wait 30 seconds.
5. Check the add-on log for a display error.
6. If the host normally shows another desktop, console, or kiosk on this display, stop that display application or reboot the host. Only one application may be able to control the display.

Useful log clues include:

- A message saying no connected panel was found — check the cable, input, and power.
- A message saying the display device is missing or cannot be opened — the installation does not have access to the host’s display hardware; contact the system owner.
- A message saying another client may hold the display — stop the other display application or reboot.

The management page can continue working even when physical display output fails.

## The attached display works, but Preview has no video

1. Close other livestream-viewer Preview tabs.
2. Wait a few seconds and select **Retry**.
3. Refresh the page.
4. Confirm the add-on is still running.

The add-on limits simultaneous browser previews so the wall itself remains responsive. A layout diagram in the browser does not mean the physical display stopped.

## A source does not play

1. Open **Stream Sources** and select **Probe** beside that source.
2. Read the full status message.
3. Test the same address in the camera vendor’s application or the stream owner’s recommended player.
4. Recheck the address, username, and password.
5. Confirm **Enabled** is on.
6. Put the source alone on a one-tile screen. If it works alone, the original layout may be asking too much of the host.

livestream-viewer retries interrupted feeds automatically. A brief network interruption normally does not require a restart.

## A camera will not connect

1. Confirm the camera is online in its normal application.
2. Confirm you used the camera’s stream address, not the address of its settings webpage.
3. Start with **RTSP** and **TCP** transport.
4. Re-enter the camera username and password.
5. If the address starts with `rtsps://`, ask the camera owner whether **Verify TLS certificate** should be enabled.
6. Confirm the camera and livestream-viewer host can reach each other on the same network.

If the camera has main-stream and substream addresses, try the lower-resolution substream.

## YouTube asks you to sign in or confirms you are not a bot

1. Confirm the livestream plays in a normal browser.
2. Under **Stream Sources**, find the **YouTube** card and open **Configuration**.
3. Export a Netscape-format `cookies.txt` from a browser account that can watch the stream.
4. Upload the file and probe the source again.

Private, members-only, and age-restricted videos require cookies. A **Running** token provider does not guarantee that YouTube will allow the request.

If uploaded cookies stop working, export a fresh file. Do not share cookie files in a support ticket; they can provide access to the browser account.

## YouTube says the video is unavailable

Open the address in a browser and check that:

- The event is currently live.
- The video has not been removed.
- The account used for cookies has permission to watch.
- The video is available in the host’s location.

A scheduled premiere or event that has not started is not yet a playable livestream.

## Video pauses, jumps, or falls behind

1. Check whether the source is marked **Hardware** or **Software**.
2. Use lower-resolution camera substreams in small tiles.
3. Reduce the number of sources visible at the same time.
4. For YouTube, HLS, or DASH, increase **Buffer (seconds)** a little.
5. For a local camera, check network stability before adding buffer.
6. Use shorter transitions or **cut**.
7. Avoid probing several sources while the wall is in critical use.

Software status is not itself an error, but several high-resolution software-decoded feeds may exceed a small host’s capacity.

## The tour does not move

1. On **Preview**, check whether it says **Tour paused**. Select **Resume**.
2. Open **Display Strategy** and confirm the tour is **Enabled**.
3. Confirm it has at least two entries.
4. Check each entry’s dwell time.
5. Turn on **Loop** if it should restart after the final entry.
6. Select the Tour card’s **Save** button.

Using **Go to screen** holds that screen. Select **Resume** to return to the tour.

## A tile is empty or shows the wrong source

1. Open the screen under **Display Strategy**.
2. Select the tile and check its **Assignment**.
3. If it is a sequence, check every sequence item and dwell time.
4. Confirm the selected sources are enabled.
5. Select the screen’s **Save** button.

Changing to a smaller layout removes assignments that no longer fit.

## A source or screen cannot be deleted

- A source must be removed from every screen before it can be deleted.
- A screen must be removed from the tour, and the tour saved, before the screen can be deleted.

The message in the delete window identifies where the item is still in use.

## Collect useful diagnostic information

Before contacting support:

1. Write down what you expected and what happened.
2. Record when the problem occurred and whether it affects one source, one screen, or the whole wall.
3. Take a screenshot of **Preview** and any visible error.
4. Record the source kind, but remove passwords, private addresses, and YouTube cookies.
5. In Home Assistant, open **Settings → Add-ons → livestream-viewer → Log** and copy messages from the time of the problem.
6. Record the add-on version and host model.

For an intermittent issue, an administrator can temporarily change **Log level** to `debug`:

1. Change the option to `debug` and restart the add-on.
2. Reproduce the problem once.
3. Copy the relevant log section.
4. Change the option back to `info` and restart.

Debug logs can contain source addresses and usernames. Review them before sharing. Never share passwords, access tokens, or a `cookies.txt` file.

## If the issue is unresolved

Include the collected information when opening an issue in the [GitHub repository](https://github.com/dchote/livestream-viewer/issues). State whether the browser Preview and attached display show the same result; that distinction greatly narrows the cause.
