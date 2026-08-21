# Erugo large-file sharing

File Browser streams regular files directly to Erugo with a known
`Content-Length`; it does not create another full-size multipart copy in `/tmp`.

For payloads larger than 1,900,000,000 bytes File Browser:

- uses `Expect: 100-continue` so Erugo can reject invalid headers before the
  complete body is sent;
- logs `clamav_bypass_expected=true`;
- expects Erugo to make the antivirus decision from the actual stored file size.

File Browser also skips its own automatic upload-time ClamAV scan for files
larger than 1,900,000,000 bytes. Explicit/manual scans are unchanged.

HTTP 413 responses from Erugo are returned to the browser as 413 with a clear
message about the Caddy and PHP limits instead of being flattened into a generic
500 error.

Erugo must separately raise its Caddy/PHP multipart limits and implement the
server-side ClamAV size bypass. See the matching Erugo deployment bundle.
