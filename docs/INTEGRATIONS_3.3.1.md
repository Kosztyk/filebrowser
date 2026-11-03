# File Browser 3.3.1 — integrations and container examples

File Browser has **optional** integrations. Each is controlled in **Settings → Global Settings** (the admin UI) or configured through supported env defaults; saved GUI settings can take precedence over environment fallbacks. All example hostnames and credentials below are placeholders; no home-network IPs, API tokens, passwords or private domains are committed.

[Complete sample Compose stack](../deploy/examples/compose.integrations.yaml) · [Sample variables](../deploy/examples/.env.example) · [Install/upgrade](INSTALL_3.3.1.md)

## The service topology

| Container | Image | Used for | Endpoint from File Browser |
|---|---|---|---|
| **Erugo** | `kosztyk/erugo:5th` (custom image provided by the project owner) | Shared-link uploads and reverse-share requests | `http://erugo:80` |
| **ConvertX** | `kosztyk/convertx-clamav:v0.20.0` (custom image provided by the project owner) | File conversion; optional Erugo and AV integration | `http://convertx:3000` |
| **ClamAV REST API** | `kosztyk/clamav-rest-api:renovate` (custom image provided by the project owner) | HTTP multipart scanner bridge | `http://clamav-rest-api:3000/api/v1/scan` |
| **ClamAV daemon** | `clamav/clamav:stable` (official image) | Scan engine for the REST API, not an HTTP replacement | `clamav:3310` (TCP) |
| **Collabora CODE** | `collabora/code:latest` (official image) | Office editor accessible by browsers, WOPI callbacks to File Browser | `https://office.example.com` public editor URL |

**Before deployment:** confirm that your custom image tags still exist and expose the expected REST routes; exact tags above are the ones supplied with the original deployment, not claims that they are latest. Pin image digests once validated. Protect service-to-service APIs behind a trusted Docker network and put all browser-facing endpoints behind TLS and authentication. `clamav/clamav` is not the `/api/v1/scan` HTTP service; the REST API container is required for this integration.

## ClamAV: official daemon + custom REST bridge

```yaml
services:
  clamav:
    image: clamav/clamav:stable
    restart: unless-stopped
    volumes:
      - clamav-signatures:/var/lib/clamav
    # No 3310 host port: other containers reach clamav:3310.
  clamav-rest-api:
    image: kosztyk/clamav-rest-api:renovate
    restart: unless-stopped
    environment:
      NODE_ENV: production
      APP_FORM_KEY: FILES
      CLAMD_IP: clamav
      CLAMD_PORT: "3310"
      CLAMD_TIMEOUT: "3600000"
      APP_MAX_FILE_SIZE: "190" # image-specific setting; validate real limit
      APP_MAX_FILES_NUMBER: "5"
      APP_TEMP_FILE_DIR: /tmp/clamav-uploads
      APP_UPLOAD_TIMEOUT: "0"
      APP_MAX_CONCURRENT_SCANS: "1"
    volumes:
      - clamav-upload-tmp:/tmp/clamav-uploads
volumes:
  clamav-signatures:
  clamav-upload-tmp:
```

File Browser's ClamAV URL must be the **HTTP** URL `http://clamav-rest-api:3000/api/v1/scan` (or the host-accessible equivalent). The request uses multipart field **`FILES`**. An infected response blocks publishing the upload; a broken/unknown scan cannot be treated as a clean result. Manual scans are separate from automatic upload scans. The legacy automatic skip threshold is 1,900,000,000 bytes, subject to configurable newer oversize-policy controls — validate with your security policy; do not assume oversized files were checked. Signature updates, clamd `StreamMaxLength`/`MaxFileSize`/archive limits and proxy upload limits must be sized consistently; changing the wrapper `APP_MAX_FILE_SIZE` alone will not override clamd limits.

## Erugo: outgoing shares AND reverse file requests

```yaml
services:
  erugo:
    image: kosztyk/erugo:5th
    env_file: ./erugo.env  # Erugo/Laravel app configuration, not committed
    restart: unless-stopped
    ports: ["127.0.0.1:9998:80"]
    environment:
      CLAMAV_URL: http://clamav-rest-api:3000/api/v1/scan
      CLAMAV_MAX_FILE_SIZE_BYTES: "1900000000"
    volumes:
      - erugo-storage:/var/www/html/storage
volumes:
  erugo-storage:
```

File Browser: enable Erugo, set URL to `http://erugo:80`, supply its **Erugo integration API token**, share endpoint `/api/integrations/convertx/share`, reverse request endpoint `/api/integrations/filebrowser/reverse-share`, expiry (default 168 h), timeout and upload timeout. The Share menu routes through Erugo when enabled; **Request files with Erugo** creates reverse invites and supports an upload-speed limit. Existing native File Browser shares are still supported, and the UI falls back when Erugo is disabled/unavailable. A prepared folder ZIP is durable; the request to Erugo itself is an ordinary stream, **not** a resumable byte transfer. Avoid blindly retrying an ambiguous upstream submission; inspect the job and Erugo state first.

Erugo's own `erugo.env` depends on the Erugo build. Its application key, mail settings, database path and service token must be configured separately. The Erugo REST share contract must be present in the custom image; stock upstream images may not provide it.

## ConvertX: converting files from File Browser

```yaml
services:
  convertx:
    image: kosztyk/convertx-clamav:v0.20.0
    restart: unless-stopped
    ports: ["127.0.0.1:8081:3000"]
    environment:
      TZ: Europe/Bucharest
      HTTP_ALLOWED: "true" # only when intentionally using HTTP on trusted LAN
      JWT_SECRET: ${CONVERTX_JWT_SECRET:?set in .env}
      CONVERTX_API_TOKEN: ${CONVERTX_API_TOKEN:?set in .env}
      AUTO_DELETE_EVERY_N_HOURS: "720"
      CLAMAV_URL: http://clamav-rest-api:3000/api/v1/scan
      ERUGO_BASE_URL: http://erugo:80
      ERUGO_API_TOKEN: ${ERUGO_API_TOKEN:?set in .env}
      ERUGO_DEFAULT_EXPIRY_HOURS: "168"
    volumes:
      - convertx-data:/app/data
volumes:
  convertx-data:
```

File Browser's ConvertX endpoint is `http://convertx:3000` with API key matching `CONVERTX_API_TOKEN`; use the admin GUI or `FB_CONVERTX_URL`, `FB_CONVERTX_API_KEY`, `FB_CONVERTX_ENABLED=true` as **initial fallback values**. The required custom ConvertX REST contract includes `GET /api/health`, `GET /api/conversions?from=docx`, `POST /api/convert`, `GET /api/jobs/{id}` and output endpoints. A successful browser login page or HTTP 200 on `/` is **not** a valid File Browser API integration test. Server-side conversion saves the result next to the source with collision-aware naming; job IDs resume polling across restart when supported. See [CONVERTX_FILE_ACTION.md](../CONVERTX_FILE_ACTION.md).

## Collabora Online: WOPI office editing

```yaml
services:
  collabora:
    image: collabora/code:latest
    restart: unless-stopped
    ports: ["127.0.0.1:9980:9980"]
    environment:
      server_name: office.example.com
      aliasgroup1: 'https://files\.example\.com:443'
      aliasgroup2: 'https://files\.example\.com'
      aliasgroup3: 'http://filebrowser:80'
      username: ${COLLABORA_ADMIN_USER:?set in .env}
      password: ${COLLABORA_ADMIN_PASSWORD:?set in .env}
    command:
      - --o:ssl.enable=false
      - --o:ssl.termination=true # when HTTPS is terminated by a reverse proxy
    cap_add: [MKNOD] # omit if your runtime does not require this
```

Set File Browser **Collabora URL** to `https://office.example.com` (the URL the end user's browser can open), **public File Browser URL** to `https://files.example.com`, and optionally **internal File Browser URL** to `http://filebrowser:80`. Supply `FB_COLLABORA_WOPI_SECRET` (long random value), `FB_COLLABORA_TOKEN_TTL=2h`, and `FB_COLLABORA_ENABLED=true` or configure through the admin UI. All hostnames in Collabora `aliasgroup` must match the WOPI callbacks selected by File Browser; allow both internal and public addresses if both are used. `server_name` should match your actual **public Collabora host**. The proxy must forward websocket upgrades and `Host` headers correctly. Disable `ssl.termination` only if there is no HTTPS-terminating reverse proxy; the example assumes one. Do **not** add `SYS_ADMIN` just to make the example start. Check `/hosting/discovery`, `CheckFileInfo`, locks, and saving a `.docx`/`.xlsx`/`.pptx` document. See [COLLABORA_INTEGRATION.md](../COLLABORA_INTEGRATION.md).

## File Browser environment (optional defaults)

```yaml
services:
  filebrowser:
    build: {context: ., dockerfile: Dockerfile.source}
    image: filebrowser-custom:3.3.1
    environment:
      FB_ERUGO_ENABLED: "true"
      FB_ERUGO_URL: http://erugo:80
      FB_ERUGO_API_KEY: ${ERUGO_API_TOKEN:?set in .env}
      FB_ERUGO_REVERSE_SHARE_ENDPOINT: /api/integrations/filebrowser/reverse-share
      FB_CONVERTX_ENABLED: "true"
      FB_CONVERTX_URL: http://convertx:3000
      FB_CONVERTX_API_KEY: ${CONVERTX_API_TOKEN:?set in .env}
      CLAMAV_URL: http://clamav-rest-api:3000/api/v1/scan
      FB_COLLABORA_ENABLED: "true"
      FB_COLLABORA_URL: https://office.example.com
      FB_COLLABORA_PUBLIC_URL: https://files.example.com
      FB_COLLABORA_INTERNAL_URL: http://filebrowser:80
      FB_COLLABORA_WOPI_SECRET: ${WOPI_SECRET:?set in .env}
```

**Secret precedence:** if an integration was previously configured through the GUI, changing a fallback env var may not replace the saved database setting. Review and test the effective URL/token in the admin UI. The examples are intentionally vendor-hostname-agnostic and keep credentials in an untracked `.env` file. Never check `.env` into Git; rotate any previously exposed tokens.

## Troubleshooting checklist

1. Are all containers on the same user-defined Docker network? `localhost` from one container refers to **itself**, not another container.
2. Can File Browser reach the actual API route? Inspect **Settings → About / System → Integration diagnostics** (admin), and the backend logs; verify `/hosting/discovery` for Collabora and API JSON routes for ConvertX.
3. Does File Browser call a URL accessible from **the backend**, while the Collabora iframe URL is also reachable from **the browser**?
4. Does a reverse proxy enforce suitable upload/body/timeouts and forward websocket upgrades? Are WOPI `aliasgroup` host regexes correct?
5. Are AV results explicit (clean/infected), and are file size/timeout limits aligned across wrapper, daemon and proxies?
6. For a stalled conversion or Erugo request, inspect the persisted **Background jobs** entry and the corresponding upstream service before retrying, to prevent duplicate operations.
