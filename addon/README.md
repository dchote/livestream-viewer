# livestream-viewer Home Assistant Add-on

Turn a display connected to your Home Assistant host into a dedicated video wall for cameras, YouTube livestreams, web streams, and uploaded video files.

The add-on is confirmed working on Raspberry Pi Home Assistant hosts with an attached display.

Use **Open Web UI** to add your video sources, choose what appears on the display, and monitor the wall from another computer or tablet.

## Installation

1. In Home Assistant, go to **Settings** → **Add-ons** → **Add-on store**.
2. Click the three dots (⋮) → **Repositories**.
3. Add this repository URL: `https://github.com/dchote/livestream-viewer`
4. Find **livestream-viewer** in the add-on list and click **Install**.
5. Leave the default options in place unless you know you need to change them.
6. Select **Start**, then **Open Web UI**.
7. Sign in with username `admin` and password `admin`. You will be asked to choose a new password.

For a wall-mounted installation, connect and turn on the display before starting the add-on.

## Configuration

- **Log level** — Leave this at `info` for normal use. If support asks for more detail, change it to `debug`, restart the add-on, reproduce the problem, and copy the relevant log messages. Change it back to `info` afterward.
- **Display engine** — Leave this enabled when a display is connected to the Home Assistant host. Disable it only when you want to manage and preview a wall without driving a physical display.

Your settings, uploaded files, and accounts are kept when the add-on restarts or updates. Uninstalling the add-on may remove that data.

## First steps

1. Open **Stream Sources** and add a camera, livestream, or video file.
2. Select **Probe** beside the source to check that it can be opened.
3. Open **Display Strategy** and create a screen.
4. Assign your source to a tile, then select **Save**.
5. Add the screen to the **Tour**, turn on **Enabled**, then save the tour.
6. Open **Preview** to confirm what is being sent to the attached display.

## YouTube

Most public YouTube livestreams work without extra setup. If the Preview page says YouTube needs you to sign in or is treating the device as a bot:

1. On a computer that can play the stream, export a Netscape-format `cookies.txt` file.
2. In livestream-viewer, open **Stream Sources**.
3. In the **YouTube** section, open **Configuration** and upload the file.

Private, members-only, and age-restricted videos also require cookies. Use an account with only the access this display needs; automated playback can cause YouTube to restrict an account.

## Support

- [User guide](../docs/user-guide/README.md)
- [Troubleshooting](../docs/user-guide/troubleshooting.md)
- [GitHub repository](https://github.com/dchote/livestream-viewer)
