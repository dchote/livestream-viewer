# Get started with livestream-viewer

This guide creates a simple wall with one source and one screen. Installation steps for Home Assistant are in the [add-on README](../../addon/README.md).

## Open the management page

In Home Assistant, open **Settings → Add-ons → livestream-viewer**, make sure the add-on is running, and select **Open Web UI**.

If your organization installed livestream-viewer another way, use the address provided by your administrator.

## Sign in for the first time

1. Enter username `admin` and password `admin`.
2. Choose a new password when prompted. It must contain at least eight characters.
3. Store the new password in your organization’s approved password manager.

Do not leave the default password in place.

## Add your first source

1. Open **Stream Sources**.
2. Select **Add Source**.
3. Enter a clear name, such as `Front entrance` or `Town Hall livestream`.
4. Choose the kind of source and enter its address. See [Add and manage sources](sources.md) for the available kinds.
5. Leave **Enabled** on and select **Create**.
6. Select **Probe** beside the new source.

A **Hardware**, **Software**, or **Probed** status means the source was opened. An error means the address, credentials, access, or stream availability needs attention.

## Create your first screen

1. Open **Display Strategy**.
2. Under **Screens**, select **Add Screen**.
3. Give it a name, choose **Grid**, and select **Create**.
4. Open the new screen. The starting layout has one tile.
5. Select the tile, set **Assignment** to **Source**, and choose the source you added.
6. Choose a **Fit** option:
   - **Contain** shows the entire picture and may add black bars.
   - **Cover** fills the tile and may crop the edges.
   - **Fill** stretches the picture to fit.
7. Select **Save**.

## Put the screen on the wall

1. Under **Tour**, select **Add Entry**.
2. Choose the screen you just created.
3. Turn on **Enabled**.
4. Select the Tour card’s **Save** button.

A tour with one entry is a static wall. You do not need to turn on **Loop** until the tour has more than one entry.

## Check the result

Open **Preview**. You should see the source and the name of the active screen. The same image should appear on the attached display.

If the browser preview works but the attached display is blank, go to [The browser preview works, but the attached display is blank](troubleshooting.md#the-browser-preview-works-but-the-attached-display-is-blank).

## Add more screens

You can now:

- Add more sources and choose a multi-tile grid.
- Put several sources in one tile as a sequence.
- Create a full-screen playlist with a transition screen.
- Add screens to a tour so the wall changes automatically.

See [Build screens and tours](screens-and-tours.md).
