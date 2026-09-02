#!/usr/bin/env bash
# Source this file to set the Fyne/Go build environment (Homebrew GL/X11 deps).
export PATH=/var/home/Gigatone/.local/go/bin:/home/linuxbrew/.linuxbrew/bin:$PATH
export GOPATH=/home/Gigatone/go

B=/home/linuxbrew/.linuxbrew
PKGS=""
for d in "$B"/opt/*/lib/pkgconfig "$B"/opt/*/share/pkgconfig; do
  [ -d "$d" ] && PKGS="$PKGS:$d"
done
export PKG_CONFIG_PATH="${PKGS#:}"

CFL=""
LDF=""
for d in "$B"/opt/*; do
  [ -d "$d/include" ] && CFL="$CFL -I$d/include"
  [ -d "$d/lib" ] && LDF="$LDF -L$d/lib"
done
export CGO_CFLAGS="$CFL"
export CGO_CPPFLAGS="$CFL"
export CGO_CXXFLAGS="$CFL"
export CGO_LDFLAGS="$LDF"