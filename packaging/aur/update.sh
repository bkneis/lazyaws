#!/usr/bin/env bash
# Usage: ./update.sh <version>  e.g.  ./update.sh 0.2.0
set -euo pipefail

VER=${1:?version required}
CHECKSUMS_URL="https://github.com/bkneis/lazyaws/releases/download/v${VER}/checksums.txt"

echo "Fetching checksums for v${VER}..."
CHECKSUMS=$(curl -fsSL "${CHECKSUMS_URL}")

SHA_AMD64=$(echo "${CHECKSUMS}" | grep "linux_amd64.tar.gz" | awk '{print $1}')
SHA_ARM64=$(echo "${CHECKSUMS}" | grep "linux_arm64.tar.gz" | awk '{print $1}')

sed -i \
  -e "s/^pkgver=.*/pkgver=${VER}/" \
  -e "s/^pkgrel=.*/pkgrel=1/" \
  -e "s/sha256sums_x86_64=('.*')/sha256sums_x86_64=('${SHA_AMD64}')/" \
  -e "s/sha256sums_aarch64=('.*')/sha256sums_aarch64=('${SHA_ARM64}')/" \
  PKGBUILD

makepkg --printsrcinfo > .SRCINFO

echo "Updated PKGBUILD and .SRCINFO for v${VER}"
echo "Review changes, then: git add PKGBUILD .SRCINFO && git commit -m \"upgpkg: lazyaws-bin ${VER}\" && git push"
