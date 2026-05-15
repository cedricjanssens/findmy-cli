---
name: findmy
description: |
  Query Find My locations and ring devices on macOS. People, devices, phone
  ring, aliases. Supports 41 macOS display languages. Requires Screen
  Recording + Accessibility permissions.
---

# Find My

Wraps the `findmy` CLI bundled with this plugin. Drives FindMy.app via
screen capture, Vision OCR, and CGEvent clicks.

## When to use

- "Where is Omar?" / "Where is everyone?" → people/person lookup
- "List my devices" → devices listing
- "Find Christel's phone" / "Ring my iPhone" → phone ring

## Run

```bash
# People
bash "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" people --json
bash "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" person "Omar" --json

# Devices
bash "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" devices --json

# Ring a device immediately (alias or full name)
bash "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" phone Christel

# Ring with dry-run safety
bash "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" ring "iPhone14PM Christel"
bash "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" ring "iPhone14PM Christel" --confirm
```

Auto-builds on first run via `make` (Go 1.22+ + Xcode CLI Tools).

## Caveats

- **`staleness: "Paused"`** — friend paused sharing, location is last known.
- **`phone` and `ring --confirm`** play a loud sound on the target device.
- **Focus steal**: ring commands raise FindMy and move the cursor.
- **Back-to-back races**: space calls by ~5s.
- **Display must be awake** for screencapture to work.

## Permissions (one-time)

- **Screen Recording** — for screencapture
- **Accessibility** — for CGEvent clicks (ring, zoom)

Grant in System Settings → Privacy & Security, then restart the host process.

```bash
"${CLAUDE_PLUGIN_ROOT}/bin/findmy-helper" permissions
# → {"accessibility":true,"screenRecording":true}
```
