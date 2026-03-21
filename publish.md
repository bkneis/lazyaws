# Publishing lazyaws

## Phase 1: Automated (zero ongoing maintenance)

These channels are driven entirely by GoReleaser on each release push. No manual steps required after the one-time secret setup below.

### Required GitHub repo secrets

| Secret | How to obtain |
|---|---|
| `DOCKERHUB_USERNAME` | DockerHub account name |
| `DOCKERHUB_TOKEN` | DockerHub → Account Settings → Security → New Access Token |
| `HOMEBREW_TAP_TOKEN` | GitHub PAT (classic), `repo` scope, write access to `bryanl/homebrew-lazyaws` |
| `SCOOP_BUCKET_TOKEN` | GitHub PAT (classic), `repo` scope, write access to `bryanl/scoop-lazyaws` |

### One-time companion repo setup

1. Create `bryanl/homebrew-lazyaws` — public, empty. GoReleaser will push `Formula/lazyaws.rb`.
2. Create `bryanl/scoop-lazyaws` — public, empty. GoReleaser will push `bucket/lazyaws.json`.

### What GoReleaser produces automatically

- Homebrew formula in `bryanl/homebrew-lazyaws`
- Scoop manifest in `bryanl/scoop-lazyaws`
- `.deb` and `.rpm` packages attached to the GitHub release
- Multi-arch Docker images (`linux/amd64`, `linux/arm64`) on DockerHub
- `.snap` build artifact (not published until classic confinement is approved — see Snap section below)

---

## Phase 2: Submit-once channels

### Snap store (classic confinement)

Classic confinement is required because lazyaws reads `~/.aws/` credentials.

1. Create a Snapcraft account at https://snapcraft.io
2. `snapcraft login`
3. `snapcraft register lazyaws`
4. Submit a classic confinement request at https://forum.snapcraft.io/t/request-classic-confinement — explain that the snap needs filesystem access to `~/.aws/` for AWS credentials
5. Once approved:
   - Flip `publish: false` → `publish: true` in the `snapcrafts:` block of `.goreleaser.yml`
   - Obtain export credentials: `snapcraft export-login --snaps lazyaws --acls package_upload -`
   - Add the output as GitHub secret `SNAPCRAFT_STORE_CREDENTIALS`

### pkgx (pantry)

pkgx auto-tracks GitHub releases after the initial PR. No further PRs needed once merged.

1. Fork https://github.com/pkgxdev/pantry
2. Add `projects/github.com/bryanl/lazyaws/package.yml` (file is at `packaging/pkgx/package.yml` in this repo)
3. Open a PR — maintainers typically merge within a few days

### AUR (`lazyaws-bin`)

1. Create an account at https://aur.archlinux.org and add your SSH public key
2. Clone the (initially empty) AUR package repo:
   ```bash
   git clone ssh://aur@aur.archlinux.org/lazyaws-bin.git
   ```
3. Copy `packaging/aur/PKGBUILD` into the clone
4. Fill in real checksums from the release `checksums.txt`
5. Generate `.SRCINFO` (mandatory):
   ```bash
   makepkg --printsrcinfo > .SRCINFO
   ```
6. Commit and push:
   ```bash
   git add PKGBUILD .SRCINFO
   git commit -m "Initial import: lazyaws-bin 0.1.0"
   git push
   ```

---

## Phase 3: Ongoing-light channels

### MacPorts

Requires a PR to `macports/macports-ports` for the initial submission, then a version-bump PR per release.

**Initial submission:**
1. Install `go2port`: `go install github.com/amake/go2port@latest`
2. Generate the `go.vendors` block: `go2port go.sum` — paste output into `packaging/macports/Portfile`
3. Fill in `checksums` (rmd160, sha256, size) from the source tarball
4. Fork https://github.com/macports/macports-ports, add `sysutils/lazyaws/Portfile`, open PR

**Version bump:**
Update `go.setup` version, regenerate checksums and `go.vendors`, open a new PR.

### Chocolatey

**Initial submission:**
1. Create account at https://community.chocolatey.org
2. Copy `packaging/chocolatey/` locally
3. Replace `REPLACE_WITH_SHA256` in `tools/chocolateyInstall.ps1` with the SHA256 of the Windows amd64 zip from `checksums.txt`
4. Pack and push:
   ```powershell
   choco pack lazyaws.nuspec
   choco push lazyaws.0.1.0.nupkg --source https://push.chocolatey.org
   ```
5. First submission goes through moderation (1–3 days)

**Version bump:**
Update `<version>` in `lazyaws.nuspec`, `$version` and `checksum64` in `tools/chocolateyInstall.ps1`, then re-pack and re-push.

---

## Verification checklist

```
[ ] goreleaser release --snapshot --clean   # local dry-run
[ ] brew tap bryanl/lazyaws && brew install lazyaws && lazyaws --version
[ ] scoop bucket add lazyaws https://github.com/bryanl/scoop-lazyaws && scoop install lazyaws
[ ] docker run --rm bryanl/lazyaws --version
[ ] dpkg -i lazyaws_*.deb && lazyaws --version
[ ] rpm -i lazyaws_*.rpm && lazyaws --version
[ ] yay -S lazyaws-bin   (Arch)
```
