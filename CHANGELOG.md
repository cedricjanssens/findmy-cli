# Changelog

## face-detect v0.5.3 — 2026-05-16

### Added

- **FAQ.md** — troubleshooting guide: FIFO deadlock patterns, ANE/Ollama contention, integration tips, model selection.

### Fixed

- **Zombie test false positive** — test 11 ("zero zombies") no longer fails when a pre-existing face-detect daemon (e.g. archiviste) is running.
- **README: `request_id` → `id`** — watch protocol docs now match actual JSON field name.

## v0.2.0 — 2026-05-15

Full i18n support, device management, and ring-to-find workflow.

### Added

- **Locale support for 41 macOS display languages** — auto-detects system language, maps localized menu names (View, People, Devices, time suffixes). Override with `FINDMY_LANG=xx`.
- **`findmy devices`** — list all devices (yours + family sharing) via sidebar OCR.
- **`findmy phone <device|alias>`** — find a device and ring it immediately, no confirmation step.
- **`findmy ring <device|alias>`** — ring with dry-run safety (requires `--confirm` to actually play sound).
- **`findmy alias`** — register short names for devices (`findmy alias Christel "iPhone14PM Christel"`). Stored in `~/.config/findmy-cli/aliases.json`.
- **`findmy person --zoom`** — click a person's row to OCR the precise street address from the detail pane.
- **`findmy-helper setup-check`** — verify TCC permissions, display status, and FindMy availability.
- **Auto Space switching** — switches to FindMy's Space for capture, returns to user's Space after.
- **Virtual display support** — zero visual disruption when FindMy runs on a BetterDisplay dummy screen. Scale-aware coordinate mapping for non-integer pixel ratios.

### Fixed

- **Ring reliability** — 95% success rate with progressive retry delays (5 attempts with increasing waits).
- **Scale-aware top margin** — correct sidebar parsing on virtual displays with non-2x scale factors.

## v0.1.2

Upstream release — English-only, people listing.

## v0.1.1

Upstream release.

## v0.1.0

Initial release by [@omarshahine](https://github.com/omarshahine).
