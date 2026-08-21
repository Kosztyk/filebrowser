# Erugo bandwidth limits from File Browser

This build extends the existing optional Erugo integration without changing File Browser's native sharing model.

## Normal Erugo shares

The **Share with Erugo** dialog contains a **Maximum aggregate download speed** selector. The selected value is sent to Erugo as `download_speed_limit_kbps`.

- `0` or **Unlimited** omits the field and preserves the previous Erugo behavior.
- Positive values are expressed in kilobits per second.
- File Browser accepts values from `0` through `1000000` Kbps.
- Erugo enforces the limit across simultaneous downloads of that share.

## Erugo reverse shares

When Erugo is enabled, the file-list toolbar contains **Request files with Erugo**. This opens a form for:

- recipient name
- recipient email
- optional message
- maximum upload speed

File Browser sends the request through its backend so the Erugo API token is never exposed to the browser. The default Erugo endpoint is:

`/api/integrations/filebrowser/reverse-share`

The endpoint can be changed in **Settings → Global Settings → Erugo share integration** or with `FB_ERUGO_REVERSE_SHARE_ENDPOINT`.

## Compatibility

- Native File Browser shares retain their original request and persistence models.
- Existing Erugo settings load with the new reverse-share endpoint default automatically.
- Existing Erugo shares remain unlimited unless a limit is explicitly selected.
- Collabora, archive, ConvertX, ClamAV, large-file Erugo upload, and native-share fallback paths are unchanged.
