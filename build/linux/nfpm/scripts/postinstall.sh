#!/bin/sh
# Refresh the desktop database so ViPlay appears in application menus right after installation.
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q /usr/share/applications || true
fi
