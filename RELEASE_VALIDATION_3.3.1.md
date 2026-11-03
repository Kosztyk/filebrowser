# 3.3.1 GitHub-ready package validation

This is a **packaging and smoke-test report**, not a certification of all production integrations.

## Source provenance and diff

- Source: uploaded `Filebrowser_Upgraded_3.3.1(1).zip`, extracted at its `filebrowser-3.3.1/` root.
- Comparison baseline: uploaded `filebrowser-master(1).zip`, extracted at its `filebrowser-master/` root.
- Initial comparison: **493 baseline files vs 1,604 upgraded files**; 1,114 added, 84 changed, 3 removed, 406 unchanged, before documentation/packaging additions. The majority of additions are compiled frontend assets; native Linux amd64 and arm64 binaries are in the package.
- The v3.3.1 `VERSION`, frontend package version and prebuilt Linux amd64 version were checked. Native amd64 and arm64 binaries were verified against `native-install/binaries.json` SHA-256 hashes.
- Fork documentation and sanitized example Compose configuration have been prepared. The obsolete, master-targeting `deploy-filebrowser-v18.1-and-github.sh` was removed; `publish-3.3.1.sh` is the replacement (release branch only).

## Completed checks in this session

- Both input ZIP archives passed ZIP integrity validation.
- `python -m unittest discover -s native-install -p 'test_*.py' -v`: **30 tests passed** for native installer, rollback and upload-journal behavior.
- Packaged Linux amd64 executable: version reports `File Browser v3.3.1/directory-fix`.
- Isolated local startup with a temporary DB and test directory: **HTTP login 200, authorized `/api/resources/` 200 and `/api/raw/sample.txt` 200**. The app expects `X-Auth` for API requests (not an `Authorization: Bearer` header).
- `bash -n publish-3.3.1.sh`: passed.
- Python YAML syntax parse: both root `compose.yaml` and six-service `deploy/examples/compose.integrations.yaml` passed.
- Prepared new documentation and compose samples were checked for the example production secrets, private LAN addresses and domains provided by the user: no hits.

## Not verifiable in this environment

- Full Go unit/integration suite and frontend rebuild: local Go is **1.23.2**, below the module-required **1.27**; Node is **22.16.0**, below the required **26.13.0**, and pnpm 12 is unavailable. Source has not been rebuilt here; bundled native executable was smoke-tested instead.
- Docker Compose execution: Docker daemon not available. YAML parse is **not** a successful `docker compose config` or a container health check.
- Live Erugo, ConvertX, ClamAV REST/clamd and Collabora endpoints were not supplied as accessible test services; **no end-to-end integration checks** were performed. Their example image tags were supplied by the owner and must be verified/pulled on deployment.
- GitHub remote publishing was attempted with the connected GitHub integration but the server returned **HTTP 403 / Resource not accessible by integration** for branch and blob writes. The remote repository is **unchanged**. Use the provided guarded `publish-3.3.1.sh` after configuring GitHub push credentials. Do not claim that 3.3.1 is live on GitHub until that push succeeds.

## Staging checklist

On a staging copy of the real DB/files: run source build and test suites, `docker compose config` for the samples, smoke test file operations plus upload conflict/resume, inspect jobs and previous versions, verify Erugo forward/reverse shares, ConvertX conversion, ClamAV infected/clean/oversize policy and Collabora WOPI saving. Then review the GitHub release branch PR and deploy to production with cold backups and a rollback plan.
