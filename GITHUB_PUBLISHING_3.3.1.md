# Publishing File Browser 3.3.1 to Kosztyk/filebrowser

**As packaged, GitHub master is NOT yet updated.** The connected GitHub integration was able to read the repository but rejected branch/blob writes with HTTP 403 (`Resource not accessible by integration`). This package provides the complete source and an independent authenticated Git push script.

## On macOS, Linux, or a git-capable workstation

Download `FileBrowser_3.3.1_GitHub_Ready.zip` and run:

```bash
cd ~/Downloads
unzip -q FileBrowser_3.3.1_GitHub_Ready.zip
cd filebrowser-3.3.1
bash publish-3.3.1.sh
```

The script checks the version and source tree, clones `https://github.com/Kosztyk/filebrowser.git` into an **isolated temporary directory**, checks the original `master` SHA, creates/updates **`release/3.3.1`** (not `master`), mirrors the complete 3.3.1 source (excluding regenerable frontend `dist` and bundled native executable files), commits and pushes. The normal GitHub commit history is preserved. A proposed pull request URL is printed. It never force-pushes.

After CI and a staging upgrade, open https://github.com/Kosztyk/filebrowser/compare/master...release/3.3.1?expand=1 and merge the PR through GitHub. Until merged, `master` remains the old version.

## Authentication without `gh` or Homebrew

Git is sufficient. On macOS, run `git --version`; if the developer tools are missing, install the Xcode Command Line Tools (`xcode-select --install`). For HTTPS, configure a GitHub credential manager or Keychain, then authenticate with a **fine-grained personal access token** that has `Contents: Read and write` for **this repository** when prompted by Git. GitHub account passwords cannot be used for HTTPS Git pushes. Keep the token in your credential manager, not in this repository or command history.

If you have configured an SSH key on GitHub, push using SSH instead:

```bash
FILEBROWSER_GIT_REMOTE=git@github.com:Kosztyk/filebrowser.git bash publish-3.3.1.sh
```

The script fails safely if remote `master` no longer matches the SHA checked when this package was prepared. Review the newer changes first; only after review may you rerun with `ALLOW_MASTER_ADVANCE=1`.

## Validation and release

Run the CI workflows and test actual integrations. The `native-install` prebuilt binaries and `frontend/dist` are included in the distribution ZIP for offline/local use; they are intentionally **not** committed as generated artifacts by the publishing script. To build a published source checkout, run frontend build and Go build per [README.md](README.md). GitHub Releases and Docker Hub tags are **not** created by this script.
