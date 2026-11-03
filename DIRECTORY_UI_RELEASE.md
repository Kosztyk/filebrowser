# Filebrowser 3.3.1 — directory layout and behavior correction

This patch compares the complete 3.3.0 application with the preceding 3.2.0 archive and restores the original directory renderer. It keeps the 3.3.0 backend enhancements, package versions and server-side directory pages. Native Linux amd64 and arm64 executables are included.

## Restored and corrected

| Area | Finding in 3.3.0 | Behavior in 3.3.1 |
| --- | --- | --- |
| Folder/file sections | The virtual renderer bypassed the original Folders and Files sections. Reversing the server index also put files before folders. | Folders appear first, then Files. Descending orders reverse entries within each section. Sections continue across bounded pages. |
| Cards and view modes | The virtual renderer forced every card's width, height and margin, overriding the existing mosaic/gallery/list styling. | Original card spacing, shadows, responsive columns, thumbnails and all three view modes use their existing styles. |
| Right-click actions | New paged items bypassed the wrapper that opened the existing context menu. | The original permission-aware menu and its actions work on paged items. Right-click keeps an existing multi-selection and does not count as a double-click/open. |
| Filtering | Plain browser controls required Apply and could ignore input while busy. | A compact themed field filters the current folder as you type after 250 ms. Enter runs immediately. Clear filter and Escape in the field remove the filter. Superseded requests are aborted and late responses ignored. |
| Sorting | Header reloads could lose the active filter. Name directions differed from the preceding version. | The compact sort selector and list column headers load immediately, preserve the filter and save the preference. Automatic refreshes use the latest controls and cannot overwrite them with an older response. Name order matches existing saved preferences. Column headers also support Enter/Space. |
| Selection | Clear selection was always shown, even at zero, and its name could be mistaken for clearing the filter. | Clear selection appears with an actual selection and clears visible/off-page selection and pending preselection. It is separate from Clear filter. Changing the filter clears selection; paging/sorting retains it. |
| Keyboard behavior | Global file shortcuts intercepted typing in the new filter. | Editing a filter or another input does not select, rename or delete files. Existing file shortcuts remain available outside editable fields. |
| Navigation | Returning from a file on a later page could reset the listing to page one and lose the selected item. | The previous folder query/page is reused, the item is reselected and scrolled into view. Reload after an action retains filter/sort and refreshes the index; an invalid page falls back to the first page. |
| Drag and drop | Folder drops still used a Vue 2 DOM instance lookup. Drops without a filesystem-entry API could hang. | Folder targets use their actual item URL. Internal drag/move and native file drops work; file-only drops use getAsFile, inaccessible entries report an error, and empty drops settle safely. Folder readers still consume all batches. |
| Info totals | Directory counts showed only the current server page. | Info uses matching totals from the complete directory snapshot, rather than a page's counts. |

Paging remains bounded at 200 entries by default (server maximum 500). The original renderer progressively reveals entries within the page and retains selections across pages. This replaces the separate virtual renderer; it does not load an entire large directory into the browser.

The original `.filebrowser-staging` visibility is deliberately not restored. Private upload staging, previous contents and incomplete files remain protected. Archive browsing and read-only restrictions, user permissions, upload recovery, jobs, sessions, WOPI coordination, integrations, prepared downloads and bounded previews remain in place. There are no dependency upgrades in this patch.

## Native upgrade

On the already working **the staging host**, log in as **the service operator**, finish active transfers, copy `FileBrowser_3.3.1_GitHub_Ready.zip` to `$HOME/filebrowser-migration`, and run:

```bash
sudo -v
cd "$HOME/filebrowser-migration" &&
python3 -m zipfile -e FileBrowser_3.3.1_GitHub_Ready.zip . &&
cd filebrowser-3.3.1 &&
sudo bash install-native-test.sh upgrade &&
/usr/local/bin/filebrowser version &&
sudo systemctl status filebrowser --no-pager
```

Expected version: `File Browser v3.3.1/directory-fix`. The upgrade helper keeps the configuration, environment, systemd overrides and `/mnt/data` mounts, and saves rollback copies. It restarts an already running service and leaves a stopped service stopped. Hard-reload the browser to load the new UI. See NATIVE_INSTALL.md for details and DIRECTORY_UI_VALIDATION.md for the checks performed.
