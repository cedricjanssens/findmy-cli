---
name: findmy
description: |
  Query Find My locations and ring devices on macOS via the findmy-cli plugin.
  People, devices, phone ring, aliases. Supports 41 macOS display languages.
license: MIT
metadata:
  author: Cedric Janssens
  version: 0.2.0
  openclaw:
    emoji: pushpin
    os: [darwin]
    homepage: https://github.com/cedricjanssens/findmy-cli
    requires:
      bins: [findmy, findmy-helper]
    install:
      - kind: brew
        id: findmy-cli
        label: "Install findmy and findmy-helper via Homebrew"
        formula: omarshahine/tap/findmy-cli
        bins: [findmy, findmy-helper]
---

# Find My Skill

Five tools available, all shell out to the `findmy` binary which drives
FindMy.app via screen capture and Vision OCR.

## When to use

- "Where is Omar?"              → `findmy_person` with `name: "Omar"`
- "Where is everyone?"          → `findmy_people`
- "List my devices"             → `findmy_devices`
- "Find Christel's phone"       → `findmy_phone` with `device: "Christel"`
- "Ring my iPhone"              → `findmy_phone` with `device: "iPhone14PM Cedric"`
- "Where am I?"                 → `findmy_person` with `name: "Me"`

## Tools

| Tool | Action | Mutating? |
|------|--------|-----------|
| `findmy_people` | List all friends (People tab) | No |
| `findmy_person` | Look up one friend by name | No |
| `findmy_devices` | List all devices (Devices tab) | No |
| `findmy_phone` | Find device + ring immediately | **Yes** — plays loud sound |
| `findmy_ring` | Find device, dry-run by default | Only with `confirm: true` |

## Aliases

Devices can have short aliases (e.g. "Christel" → "iPhone14PM Christel").
Both `findmy_phone` and `findmy_ring` resolve aliases automatically.
Aliases are managed via the CLI: `findmy alias <name> <device>`.

## Using location to drive other actions

Before chaining a location result into a mutating action (booking, sending
a message, triggering a routine), **ask the user for explicit approval**.
Read-only queries never need approval.

## Output shape

### People / Person
```json
{
  "name": "Omar Shahine",
  "location": "Redmond, WA",
  "staleness": "Paused",
  "distance": "7 mi"
}
```

### Devices
```json
{
  "name": "iPhone14PM Christel",
  "location": "Rue de la Martinière, Bièvres",
  "status": "Maintenant",
  "group": "Appareils de Christel"
}
```

## Caveats

- **`staleness: "Paused"`** — friend paused sharing, location is last known.
- **Focus steal**: `findmy_phone` and `findmy_ring` raise FindMy and move the cursor.
- **Back-to-back races**: space calls by ~5s when iterating.
- **Display must be awake**: `screencapture` returns blank if display is sleeping.

## ClawScan note

This skill drives FindMy.app by screen capture + Vision OCR. The `phone` and
`ring` tools additionally click inside the app (CGEvent). No network traffic
from this plugin. All data stays on-device.
