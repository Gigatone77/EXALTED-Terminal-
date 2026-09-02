#!/usr/bin/env bash
# Build a portable Linux AppImage for EXALTED Terminal (GUI edition).
# Needs: the GUI binary (scripts/build-gui.sh) and appimagetool.
set -eu
cd "$(dirname "$0")/.."

GUI_BIN="bin/exalted-gui"
OUT="dist/EXALTED_Terminal-x86_64.AppImage"
APPDIR="dist/AppDir"
APPIMAGETOOL="${APPIMAGETOOL:-/var/home/Gigatone/tools/appimagetool}"

[ -x "$GUI_BIN" ] || { echo "run scripts/build-gui.sh first"; exit 1; }
[ -x "$APPIMAGETOOL" ] || { echo "appimagetool not found at $APPIMAGETOOL"; exit 1; }

rm -rf "$APPDIR" dist/EXALTED_Terminal-x86_64.AppImage
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/lib" \
  "$APPDIR/usr/share/applications" \
  "$APPDIR/usr/share/icons/hicolor/256x256/apps"

cp "$GUI_BIN" "$APPDIR/usr/bin/exalted-gui"

cat > "$APPDIR/usr/share/applications/exalted-gui.desktop" <<'D'
[Desktop Entry]
Name=EXALTED Terminal
Comment=Offline Bible learning & study tool
Exec=exalted-gui
Icon=exalted-gui
Terminal=false
Type=Application
Categories=Education;Science;Utility;
StartupNotify=false
D
cp "$APPDIR/usr/share/applications/exalted-gui.desktop" "$APPDIR/exalted-gui.desktop"
cp cmd/exalted-gui/icon-512.png "$APPDIR/exalted-gui.png"
cp cmd/exalted-gui/icon-256.png "$APPDIR/usr/share/icons/hicolor/256x256/apps/exalted-gui.png"

# AppRun so the bundled libs are found regardless of the host.
cat > "$APPDIR/AppRun" <<'R'
#!/bin/sh
HERE="$(dirname "$(readlink -f "${0}")")"
export LD_LIBRARY_PATH="${HERE}/usr/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
exec "${HERE}/usr/bin/exalted-gui" "$@"
R
chmod +x "$APPDIR/AppRun"

# Bundle the GL/X11/Wayland runtime libs for portability (skip core glibc).
LDCONF="$(ldconfig -p 2>/dev/null || true)"
LIBS="libGL.so.1 libGLX.so.0 libGLdispatch.so.0 \
  libwayland-client.so.0 libwayland-cursor.so.0 libwayland-egl.so.1 \
  libX11.so.6 libxcb.so.1 libXau.so.6 libXext.so.6 libXfixes.so.3 \
  libXinerama.so.1 libXi.so.6 libXrandr.so.2 libXrender.so.1 \
  libXcursor.so.1 libXxf86vm.so.1 libxkbcommon.so.0 libffi.so.8"
for lib in $LIBS; do
  # Resolve to a real file via ldconfig (best-effort) or common search paths.
  src=$(printf '%s\n' "$LDCONF" | awk -v L="$lib" '$1==L {print $NF; exit}')
  if [ -z "$src" ] || [ ! -f "$src" ]; then
    for c in /usr/lib64 /usr/lib /lib64 /lib; do
      if [ -f "$c/$lib" ]; then src="$c/$lib"; break; fi
    done
  fi
  if [ -n "$src" ] && [ -f "$src" ]; then
    cp -L "$src" "$APPDIR/usr/lib/$lib"
  else
    echo "warn: could not locate $lib"
  fi
done

"$APPIMAGETOOL" "$APPDIR" >/dev/null
mv -f "EXALTED_Terminal-x86_64.AppImage" "$OUT"
echo "built $OUT"