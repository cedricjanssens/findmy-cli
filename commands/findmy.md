---
description: Query Find My — people, devices, ring phones. Supports aliases and 41 languages.
argument-hint: "[people | person <name> | devices | phone <device> | ring <device>]"
allowed-tools: Bash(${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh:*)
---

# /findmy

Query Find My via the bundled `findmy` CLI.

## Run

Based on `$ARGUMENTS`:

- Empty or `people`:
  ```bash
  "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" people --json
  ```

- `person <name>`:
  ```bash
  "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" person "<name>" --json
  ```

- `devices`:
  ```bash
  "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" devices --json
  ```

- `phone <device>` (rings immediately):
  ```bash
  "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" phone "<device>"
  ```

- `ring <device>` (dry-run, add --confirm to ring):
  ```bash
  "${CLAUDE_PLUGIN_ROOT}/scripts/findmy.sh" ring "<device>"
  ```

If `$ARGUMENTS` is a single name that doesn't match a subcommand, treat
it as `person <name>`.

## How to report results

- Lead with **name**, **location**, **distance**.
- `staleness: "Paused"` → say so up front, location is last known.
- `phone` → confirm the device rang or report the error.
- For multi-person/device output, sort by name.

## Caveats

- `phone` and `ring --confirm` play a loud sound on the target device.
- Focus steal during ring/phone commands.
- Back-to-back calls within ~5s can fail — space them out.
