# findmy-cli

Query Apple's Find My from the command line via UI scraping.

Apple does not expose a public API for friend or device locations and the
on-disk caches are encrypted with keychain-bound keys, so this tool drives the
GUI: it activates FindMy.app, switches to the appropriate tab, screenshots the
window, and runs Vision OCR on the result.

Supports **41 macOS display languages** out of the box (English, French, German,
Japanese, etc.) — the locale layer auto-detects your system language and maps
menu names accordingly.

## Why a Go CLI plus a Swift helper

The macOS APIs we need (Vision, CoreGraphics window list, CGEvent click) have no
Go binding. We bundle a tiny Swift binary `findmy-helper` that exposes them as
JSON-emitting subcommands, and a Go CLI `findmy` that orchestrates.

## Install

Pick the channel that matches how you'll use it:

| Goal | Channel | Command |
|---|---|---|
| Use `findmy` CLI from terminal | Homebrew | `brew install omarshahine/tap/findmy-cli` |
| Use as OpenClaw plugin (chat tools) | ClawHub | `clawhub install findmy-cli` |
| Use as Claude Code plugin | OpenClaw | `openclaw plugins install --link ~/GitHub/findmy-cli` |
| Use as a Node library | NPM | `npm install findmy-cli` |
| Hack on the code | Source | `git clone … && make` |

### Homebrew (CLI)

```bash
brew install omarshahine/tap/findmy-cli
```

Installs `findmy` and `findmy-helper` to `/opt/homebrew/bin/`. macOS only.
Tap source: [omarshahine/homebrew-tap](https://github.com/omarshahine/homebrew-tap).
First run will prompt for **Screen Recording** permission.

### ClawHub (OpenClaw plugin)

```bash
clawhub install findmy-cli
```

Registers `findmy_people` and `findmy_person` as OpenClaw tools. Shells out
to the `findmy` binary — install that via Homebrew first.
Listing: [`clawhub.com/p/findmy-cli`](https://clawhub.com/p/findmy-cli) ·
NPM package: [`findmy-cli`](https://www.npmjs.com/package/findmy-cli).

### Source build

```bash
make
```

Outputs `bin/findmy` and `bin/findmy-helper`.

Requirements:
- macOS (tested on 15+; FindMy.app is a Catalyst app)
- Go 1.22+
- Xcode Command Line Tools (`swiftc`)

## Usage

```bash
# List people in the sidebar with coarse location, staleness, distance.
findmy people
findmy people --json

# Click a row and OCR the detail pane (precise address).
findmy person "cedric.janssens@gmail.com" --zoom --json

# List all devices (yours + family sharing).
findmy devices --json

# Ring a device immediately (finds it and plays a sound).
findmy phone "iPhone14PM Christel"

# Or use an alias for convenience:
findmy alias Christel "iPhone14PM Christel"
findmy phone Christel

# Ring with dry-run first (locates the button without clicking).
findmy ring "iPhone14PM Christel"
findmy ring "iPhone14PM Christel" --confirm

# Manage aliases.
findmy alias                          # list all
findmy alias Maman "iPhone14PM Christel"  # add/update
findmy alias --delete Maman           # remove
```

Aliases are stored in `~/.config/findmy-cli/aliases.json` and work with both
`phone` and `ring` commands.

### Environment check

```bash
findmy-helper setup-check
```

Verifies TCC permissions (Screen Recording, Accessibility), display status,
and FindMy.app availability.

## Required macOS permissions

Grant to the terminal emulator (or to `findmy` once installed system-wide):

- **Screen Recording** — for `screencapture`
- **Accessibility** — for `osascript` menu clicks

Settings → Privacy & Security → Screen Recording / Accessibility.

After granting, **fully quit and relaunch the host process** — TCC is read once
at process start.

## Running on a headless Mac

FindMy.app needs WindowServer compositing to render its window. WindowServer
only runs when macOS sees a display, so a Mac mini / Studio / Pro with nothing
plugged into HDMI or USB-C will return a 99 KB all-black PNG every time you
call `screencapture`, even though the process itself runs fine.

To make findmy work headless:

1. **Plug in a dummy display.** A 4K HDMI/USB-C "headless adapter" (~$10 on
   Amazon, search "4K HDMI dummy plug" or "USB-C dummy display"). macOS sees
   it as a real 4K@60Hz monitor and starts WindowServer normally.

2. **Disable display sleep** so WindowServer stays compositing:

   ```bash
   sudo pmset -a displaysleep 0
   sudo pmset -a sleep 0          # optional: also disable system sleep
   ```

   Or, for a per-session keep-awake without changing global power settings:

   ```bash
   caffeinate -d &
   ```

   Run `caffeinate` as a LaunchAgent if you want it to start at login.

3. **Verify WindowServer can see FindMy.app:**

   ```bash
   open -a FindMy
   findmy-helper window --owner FindMy
   ```

   Should return JSON with non-zero `width`/`height` and `onScreen: true`. If
   `width`/`height` are 0 or the array is empty, WindowServer isn't
   compositing — re-check display sleep and that the dummy plug is seated.

4. **Grant Screen Recording to the host process** that will call `findmy`
   (your SSH session's shell, the LaunchAgent, the OpenClaw gateway, etc.).
   TCC is per-binary path; the brew-installed `/opt/homebrew/bin/findmy-helper`
   is what needs the grant.

If you're hitting black screenshots even with a dummy plug, the CLI's
`findmy-helper permissions` output (`screenRecording: false`) is the
diagnostic — TCC denied is more common than missing display.

## Limitations

- **The display must be awake and unlocked.** WindowServer stops compositing
  when the display sleeps, so `screencapture` returns a 99 KB all-black PNG.
  The CLI detects this and tells you to wake the keyboard. There is no
  software-only path to wake a sleeping display from userland — Apple gates
  `IODisplayWranglerWakeup` behind real HID hardware. See [Running on a
  headless Mac](#running-on-a-headless-mac) above for the dummy-plug fix.
- The MapKit map area does not always render into the captured bitmap (Catalyst
  quirk). This tool only reads the sidebar and detail pane text; map pins are
  not extracted.
- The FindMy.app window must be openable on this Mac (you must be signed in to
  iCloud and have at least one friend sharing).
- Window position is re-queried on every run; the app does not need to be at a
  fixed location.
- This brings FindMy.app to the foreground and steals focus during a click.
  Using a virtual display (see below) avoids visual disruption for read-only
  commands; `phone` and `ring` always need to click and will move the cursor.
- Apple's TOS may consider GUI scraping out of scope. Use at your own risk.

## Locale support

The CLI auto-detects your macOS display language via `defaults read .GlobalPreferences AppleLocale` and maps localized menu names (View, People, Devices, etc.) accordingly. 41 languages are supported.

To override auto-detection:

```bash
export FINDMY_LANG=fr   # force French
findmy people --json
```

To discover menu names for a new locale:

```bash
osascript -e 'tell application "System Events" to tell process "FindMy" to get name of every menu bar item of menu bar 1'
```

Then add an entry to `localeTable` in `internal/findmy/locale.go`.

## Layout

```
cmd/findmy/                       Go CLI (people, person, devices, phone, ring, alias)
internal/findmy/findmy.go         Orchestration, sidebar parser, ring logic
internal/findmy/locale.go         Locale detection + 41-language string table
internal/findmy/aliases.go        Device alias resolution (~/.config/findmy-cli/aliases.json)
helpers/findmy-helper/main.swift  window + ocr + click + setup-check subcommands
bin/                              Build outputs
scripts/                          Benchmarks, service optimization scripts
.claude-plugin/plugin.json        Claude Code / OpenClaw plugin manifest
commands/findmy.md                /findmy slash command
skills/findmy/SKILL.md            Auto-triggering skill
scripts/findmy.sh                 Plugin wrapper (auto-builds on first use)
```

## Plugin surfaces

All four distribution channels in one repo:

| Surface | Source of truth | Auto-published |
|---|---|---|
| Homebrew formula | [`omarshahine/homebrew-tap`](https://github.com/omarshahine/homebrew-tap) `Formula/findmy-cli.rb` | manual on tag |
| NPM package | `openclaw/package.json` | GH Actions on tag push |
| ClawHub package | same as NPM, source-linked to commit | GH Actions on tag push |
| Claude Code plugin | `.claude-plugin/plugin.json` (bundle format) | manual linked install |

CI workflows under `.github/workflows/` handle NPM and ClawHub on every
`v*` tag push (OIDC trusted publishing for NPM, `CLAWHUB_TOKEN` for
ClawHub). Homebrew formula bump is still manual.

The Claude Code wrapper (`scripts/findmy.sh`) builds `bin/findmy` and
`bin/findmy-helper` on first invocation via `make`. No binaries are
committed.
