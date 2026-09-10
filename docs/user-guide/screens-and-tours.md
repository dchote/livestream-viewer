# Build screens and tours

Open **Display Strategy** to decide what the wall shows. Only administrators can change this page.

## Choose the right feature

- Use a **grid screen** to show one or more sources at the same time.
- Use a **sequence** to rotate sources inside one tile while the rest of a grid stays in place.
- Use a **transition screen** to show a full-screen playlist of sources.
- Use a **tour** to move between complete screens.

The dwell time is the time an item stays visible. It does not include the time used by a transition.

## Create a grid screen

1. Under **Screens**, select **Add Screen**.
2. Enter a name, choose **Grid**, and select **Create**.
3. Open the new screen and choose a layout.
4. Select a tile in the diagram. On a phone, select it from the tile list.
5. Set **Assignment**:
   - **Empty** leaves the tile blank.
   - **Source** shows one source.
   - **Sequence** cycles through several sources in this tile.
6. Choose the source and its **Fit**.
7. Repeat for the other tiles, then select **Save**.

When you switch layouts, assignments outside the new layout are removed. Check every tile before saving.

## Create a sequence inside a tile

1. Open a grid screen and select a tile.
2. Set **Assignment** to **Sequence**.
3. Select each source and enter its **Dwell time (seconds)**.
4. Use **Add sequence item** for more sources.
5. Select **Save** for the screen.

This changes only the selected tile. Other tiles and the current screen remain in place.

## Create a full-screen playlist

1. Under **Screens**, select **Add Screen**.
2. Enter a name, choose **Transition**, and select **Create**.
3. Open the new screen.
4. Add playlist items. For each item, choose a source, dwell time, and fit.
5. Use the arrow buttons to change the order.
6. Turn on **Loop playlist** if it should return to the first item.
7. Choose the transition used between items.
8. Select **Save**.

## Choose a transition

- **cut** changes immediately and is the least demanding.
- **fade** blends pictures or fades through a color.
- **barWipe**, **boxWipe**, and **barnDoorWipe** reveal the next picture with different shapes.
- **pushWipe** moves both the old and new pictures.
- **slideWipe** moves the new picture over the old one.

For anything other than a cut, set the duration and easing. Start with the default easing and a short duration. Long or elaborate transitions can be distracting on an operational camera wall.

## Build a tour

A tour steps through whole screens.

1. Create and save the screens you need.
2. Under **Tour**, select **Add Entry**.
3. Choose a screen and its dwell time.
4. Choose the transition used when entering that screen.
5. Add and order the remaining entries.
6. Turn on **Enabled**.
7. Turn on **Loop** if the tour should restart after its final entry.
8. Select **Save** at the bottom of the Tour card.

One tour entry creates a static wall. Add at least two entries if you want the wall to change.

## Unsaved changes

The **Save** button becomes available after a change. Closing an expanded screen without saving discards that screen’s unsaved edits. Screens and the tour have separate Save buttons.

## Delete a screen

Select the trash button on the screen. If the screen is part of the tour, remove its tour entries and save the tour before deleting it.

## Check your work

Open **Preview** after every significant change. Check the image, active screen, tile statuses, and dwell countdown. See [Operate and monitor the wall](operation.md).
