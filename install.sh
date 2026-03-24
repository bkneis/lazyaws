#!/usr/bin/env bash
set -euo pipefail

REPO="bkneis/lazyaws"
BINARY="lazyaws"

# --- helpers -----------------------------------------------------------------

info()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
ok()    { printf '\033[1;32m  ✓\033[0m %s\n' "$*"; }
die()   { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# --- detect OS ---------------------------------------------------------------

case "$(uname -s)" in
  Linux)  OS="linux"  ;;
  Darwin) OS="darwin" ;;
  *)      die "Unsupported OS: $(uname -s). Install manually from https://github.com/${REPO}/releases" ;;
esac

# --- detect arch -------------------------------------------------------------

case "$(uname -m)" in
  x86_64)          ARCH="amd64" ;;
  aarch64|arm64)   ARCH="arm64" ;;
  *)               die "Unsupported architecture: $(uname -m). Install manually from https://github.com/${REPO}/releases" ;;
esac

# --- resolve latest release --------------------------------------------------

info "Fetching latest release from github.com/${REPO} ..."

TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' \
  | head -1 \
  | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

[[ -n "$TAG" ]] || die "Could not determine latest release tag."

VERSION="${TAG#v}"   # strip leading 'v' for the filename

FILENAME="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"

ok "Latest release: ${TAG}  (${OS}/${ARCH})"

# --- download ----------------------------------------------------------------

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

info "Downloading ${FILENAME} ..."
curl -fsSL "${BASE_URL}/${FILENAME}"    -o "${TMPDIR}/${FILENAME}"
curl -fsSL "${BASE_URL}/checksums.txt"  -o "${TMPDIR}/checksums.txt"

# --- verify checksum ---------------------------------------------------------

info "Verifying SHA256 checksum ..."
cd "$TMPDIR"

if command -v sha256sum &>/dev/null; then
  grep "${FILENAME}" checksums.txt | sha256sum --check --status \
    || die "Checksum verification failed — aborting."
elif command -v shasum &>/dev/null; then
  grep "${FILENAME}" checksums.txt | shasum -a 256 --check --status \
    || die "Checksum verification failed — aborting."
else
  printf '\033[1;33mwarning:\033[0m sha256sum/shasum not found — skipping checksum verification.\n'
fi

ok "Checksum verified."

# --- extract -----------------------------------------------------------------

tar -xzf "${FILENAME}"

# --- install -----------------------------------------------------------------

INSTALL_DIR=""
if [[ -w "/usr/local/bin" ]]; then
  INSTALL_DIR="/usr/local/bin"
elif sudo -n true 2>/dev/null; then
  INSTALL_DIR="/usr/local/bin"
  SUDO="sudo"
else
  INSTALL_DIR="${HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

${SUDO:-} install -m 755 "${BINARY}" "${INSTALL_DIR}/${BINARY}"
ok "Installed to ${INSTALL_DIR}/${BINARY}"

# --- PATH reminder -----------------------------------------------------------

if [[ "$INSTALL_DIR" == "${HOME}/.local/bin" ]]; then
  case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *) printf '\n\033[1;33mnote:\033[0m Add %s to your PATH:\n  export PATH="%s:$PATH"\n\n' \
         "$INSTALL_DIR" "$INSTALL_DIR" ;;
  esac
fi

ok "lazyaws ${TAG} ready — run: lazyaws"
