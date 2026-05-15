#!/usr/bin/env bash
# optimize-main-sip-off.sh — RUN ONLY WITH SIP TEMPORARILY DISABLED.
#
# Combines phases 1+2+3 per Cedric's explicit feedback:
#
#   KILL (Cedric approved):
#     - Apple Intelligence + Siri + assistants + voice/dictation
#     - Spotlight knowledge (NOT main Spotlight)
#     - Apple Music (Library + Artwork + Devices)
#     - Exchange + Notes Exchange + Calendar migration
#     - Maps sync (Maps app still works)
#     - News
#     - Remote Desktop / MDM agent
#     - Game Controller
#     - Mail (iCloud + maild + Spotlight Mail worker)
#     - Safari sync (Bookmarks, History, SafeBrowsing, PasswordBreach, Notification)
#
#   KEEP (Cedric explicitly):
#     - AirPlay / NowPlaying / PIPAgent
#     - SoftwareUpdateNotificationManager
#     - TMHelperAgent (Time Machine)
#
#   NOT TOUCHED (no decision yet):
#     - iCloud Drive (bird, cloudd) — keep for file sync
#     - Photos (cloudphotod, photoanalysisd, photolibraryd)
#     - Contacts (AddressBook), Calendar agent
#     - Family Sharing services
#     - Main Spotlight + corespotlightd
#     - Continuity (cmfsyncagent, biomesyncd, syncdefaultsd)
#     - Call History sync
#
# After running: REBOOT and RE-ENABLE SIP.

set -u

UID_=$(id -u)
USER_=$(whoami)

echo "=== optimize-main-sip-off.sh — for $USER_ (UID=$UID_) ==="
echo

# 1. Check SIP status
sip_status=$(csrutil status 2>&1)
echo "SIP status: $sip_status"
if echo "$sip_status" | grep -q "enabled"; then
    echo
    echo "ERROR: SIP is enabled. This script needs SIP temporarily disabled to bootout"
    echo "       Apple's system LaunchAgents. Boot into Recovery, run 'csrutil disable',"
    echo "       reboot, then re-run this script. Re-enable SIP with 'csrutil enable'"
    echo "       in Recovery after."
    exit 1
fi
echo

# 2. Measure RAM before
ram_before=$(ps -axo rss,user | awk -v u="$USER_" '$2==u {s+=$1} END {printf "%.0f", s/1024}')
echo "RAM used by $USER_ before: ${ram_before} MB"
echo

# 3. KILL list — per Cedric's explicit approval (phases 1 + 2)
KILL_LIST=(
  # Apple Intelligence (BIG on M4 Pro — Ollama covers AI)
  com.apple.ModelCatalogAgent

  # Siri + assistants (Cedric uses Claude)
  com.apple.Siri.agent
  com.apple.SiriTTSTrainingAgent
  com.apple.assistantd
  com.apple.assistant_service
  com.apple.assistant_cdmd
  com.apple.voicebankingd
  com.apple.voicememod
  com.apple.DictationIM

  # Spotlight knowledge (main Spotlight kept)
  com.apple.parsec-fbf
  com.apple.spotlightknowledged
  com.apple.spotlightknowledged.importer
  com.apple.spotlightknowledged.updater
  com.apple.managedcorespotlightd

  # Apple Music (phase 2 — not used)
  com.apple.AMPArtworkAgent
  com.apple.AMPDeviceDiscoveryAgent
  com.apple.AMPDevicesAgent
  com.apple.AMPSystemPlayerAgent
  com.apple.AMPLibraryAgent

  # Exchange + Calendar migration (none)
  com.apple.exchange.exchangesyncd
  com.apple.notes.exchangenotesd
  com.apple.calendar.CalendarAgentBookmarkMigrationService

  # Maps sync (Maps still usable)
  com.apple.Maps.mapssyncd
  com.apple.Maps.pushdaemon
  com.apple.maps.destinationd

  # News (never opened)
  com.apple.newsd

  # Remote Desktop / MDM
  com.apple.RemoteDesktop.agent
  com.apple.RemoteManagementAgent

  # Game Controller (keyboard+mouse only)
  com.apple.GameController.gamecontrolleragentd

  # Mail (phase 2 — Gmail web)
  com.apple.icloudmailagent
  com.apple.email.maild
  com.apple.mdworker.mail

  # Safari sync (phase 2 — not used as primary browser)
  com.apple.SafariBookmarksSyncAgent
  com.apple.SafariHistoryServiceAgent
  com.apple.Safari.SafeBrowsing.Service
  com.apple.Safari.PasswordBreachAgent
  com.apple.SafariNotificationAgent
)

echo "Killing ${#KILL_LIST[@]} services (bootout + disable)..."
killed=0
disabled=0
errors=0

for svc in "${KILL_LIST[@]}"; do
    # Persistent disable (survives logout/reboot)
    if launchctl disable "user/${UID_}/${svc}" 2>/dev/null; then
        disabled=$((disabled + 1))
    fi
    # Immediate bootout (kills running instance — needs SIP off)
    if launchctl bootout "gui/${UID_}/${svc}" 2>/dev/null; then
        killed=$((killed + 1))
    fi
done

echo "  Persistently disabled: $disabled"
echo "  Booted out now:        $killed"
echo

# 4. Apply defaults (phase 3) — idempotent, safe to re-run
echo "Applying defaults (animations + motion + Game Mode + Continuity + Recent)..."

defaults write NSGlobalDomain NSAutomaticWindowAnimationsEnabled -bool false
defaults write -g NSWindowResizeTime -float 0.001
defaults write -g NSScrollAnimationEnabled -bool false
defaults write com.apple.dock launchanim -bool false
defaults write com.apple.dock expose-animation-duration -float 0.1
defaults write com.apple.universalaccess reduceMotion -bool true
defaults write com.apple.universalaccess reduceTransparency -bool true
defaults write -g NSNavRecentPlacesLimit -int 0
defaults write com.apple.dock show-recents -bool false
# Game Mode KEPT — Cedric games on Steam (Civ, CK3, Clair Obscur)
# defaults write com.apple.GameController GameModeEnabled -bool false
defaults -currentHost write com.apple.coreservices.useractivityd ActivityAdvertisingAllowed -bool false
defaults -currentHost write com.apple.coreservices.useractivityd ActivityReceivingAllowed -bool false

echo "  defaults applied."
echo

# 5. Spotlight: stop indexing external volumes (keep main /)
if [ -d /Volumes ] && [ "$(ls /Volumes 2>/dev/null | wc -l)" -gt 0 ]; then
    echo "Disabling Spotlight on /Volumes (needs sudo)..."
    sudo mdutil -i off /Volumes 2>/dev/null && echo "  ok" || echo "  skipped"
fi
echo

# 6. Restart UI processes
killall Dock 2>/dev/null
killall SystemUIServer 2>/dev/null
killall Finder 2>/dev/null
echo "UI processes restarted."
sleep 3

# 7. RAM after
ram_after=$(ps -axo rss,user | awk -v u="$USER_" '$2==u {s+=$1} END {printf "%.0f", s/1024}')
saved=$((ram_before - ram_after))

echo
echo "=== Results ==="
echo "  RAM before: ${ram_before} MB"
echo "  RAM after:  ${ram_after} MB"
echo "  Saved:      ${saved} MB (some services may respawn — final gain visible after reboot)"
echo
echo "================================================================"
echo "  IMPORTANT: re-enable SIP NOW"
echo "  1. Reboot into Recovery (hold power button on Apple Silicon)"
echo "  2. Utilities → Terminal → csrutil enable"
echo "  3. Reboot normally"
echo "================================================================"
echo
echo "Then verify state: bash scripts/check-services.sh"
