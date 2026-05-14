# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Fork of [omarshahine/findmy-cli](https://github.com/omarshahine/findmy-cli) adapted for **non-English macOS locales**. The upstream code hardcodes English app/menu names which breaks on localized systems (French: "Localiser", German: "Wo ist?", etc.). Our changes add locale detection and a string mapping layer.

Goal: propose this as a PR to upstream so all locales benefit.

## Architecture

Go CLI (`cmd/findmy/`) orchestrates a Swift helper (`helpers/findmy-helper/main.swift`) to scrape the macOS FindMy.app GUI:

1. **Activate** FindMy via AppleScript (`tell application "FindMy"` — bundle name, locale-independent)
2. **Switch tab** to People via System Events menu click (menu names are localized)
3. **Screenshot** the window via `screencapture -l <windowID>`
4. **OCR** the screenshot via Vision framework (Swift helper)
5. **Parse** sidebar text into Person records (name, location, staleness, distance)

The Swift helper exposes 4 subcommands: `window`, `ocr`, `click`, `permissions`.

## Locale layer (`internal/findmy/locale.go`)

Handles 3 types of localized strings:

| What | English | French | Detection method |
|------|---------|--------|-----------------|
| Window owner (CGWindowList) | "Find My" | "Localiser" | `mdls` at runtime (universal, no map needed) |
| AppleScript `tell application` | "FindMy" | "FindMy" | Bundle name, always English — no change |
| System Events process name | "FindMy" | "FindMy" | Not localized — no change |
| View menu name | "View" | "Présentation" | Locale map in `localeTable` |
| People tab | "People" | "Personnes" | Locale map |
| Devices tab | "Devices" | "Appareils" | Locale map |
| Items tab | "Items" | "Objets" | Locale map |
| Time suffixes (OCR) | "min. ago" | "il y a", "min" | Locale map |

**Override**: set `FINDMY_LANG=fr` (or any key in `localeTable`) to bypass auto-detection.

**Adding a new locale**: add an entry to `localeTable` in `locale.go`. Window owner is auto-detected via `mdls`; you only need menu item names and time suffixes. Discover menu names with:
```bash
osascript -e 'tell application "System Events" to tell process "FindMy" to get name of every menu bar item of menu bar 1'
```

## Build & run

```bash
make              # Builds bin/findmy + bin/findmy-helper (needs Go 1.22+, swiftc)
make install      # Copies to /usr/local/bin/
make clean        # Removes bin/
```

Requirements: macOS 15+, Go 1.22+, Xcode Command Line Tools.

## macOS permissions needed

Grant to the **terminal emulator** (or to `findmy`/`findmy-helper`):
- **Screen Recording** — for `screencapture`
- **Accessibility** — for `osascript` menu clicks and CGEvent

After granting, fully quit and relaunch the terminal (TCC is read once at process start).

## Testing

No test suite. Validation is manual:
```bash
make
./bin/findmy-helper permissions                              # Check TCC grants
./bin/findmy people --json --keep                            # List all people
./bin/findmy person "cedric.janssens@gmail.com" --json --keep # Specific person
open /tmp/findmy-cli/people.png                              # Inspect screenshot
```

`--keep` preserves the screenshot in `/tmp/findmy-cli/` for debugging OCR issues.

## Known localized app display names (41 languages)

From `InfoPlist.loctable` in the FindMy.app bundle:

| Lang | Name | Lang | Name | Lang | Name |
|------|------|------|------|------|------|
| en | Find My | fr | Localiser | de | Wo ist? |
| es | Buscar | it | Dov'è | pt | Buscar |
| ja | 探す | ko | 나의 찾기 | zh_CN | 查找 |
| ru | Локатор | nl | Zoek mijn | sv | Hitta |
| ar | تحديد الموقع | da | Find | fi | Etsi |
| no | Hvor er | pl | Znajdź | tr | Bul |
