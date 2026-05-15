#!/usr/bin/env bash
# optimize-main.sh — phase 1 optimization for Cedric's main session.
# Disables only services where he explicitly has no use case (Ollama replaces
# Apple Intelligence/Siri, no Exchange, no Apple Remote Desktop, etc.).
#
# KEPT (per user request):
#   - AirPlay / NowPlayingTouchUI / PIPAgent
#   - SoftwareUpdateNotificationManager
#   - TMHelperAgent (Time Machine)
#
# To restore a service:
#   launchctl enable user/$(id -u)/<label>
#   then logout/login (or `launchctl bootstrap gui/$(id -u) <plist>`)

set -u

UID_=$(id -u)
USER_=$(whoami)

echo "=== Phase 1 optimization for $USER_ (UID=$UID_) ==="
echo

if [ "$USER_" = "root" ]; then
    echo "ERROR: run as your normal user, not root."
    exit 1
fi

# Measure RAM before
ram_before=$(ps -axo rss,user | awk -v u="$USER_" '$2==u {s+=$1} END {printf "%.0f", s/1024}')
echo "RAM used by $USER_ before: ${ram_before} MB"
echo

# Services to disable (phase 1, approved)
KILL_LIST=(
  # Apple Intelligence / Siri (Ollama covers this)
  com.apple.ModelCatalogAgent
  com.apple.Siri.agent
  com.apple.SiriTTSTrainingAgent
  com.apple.assistantd
  com.apple.assistant_service
  com.apple.assistant_cdmd
  com.apple.voicebankingd
  com.apple.voicememod
  com.apple.DictationIM

  # Spotlight knowledge / suggestions (main Spotlight kept)
  com.apple.parsec-fbf
  com.apple.spotlightknowledged
  com.apple.spotlightknowledged.importer
  com.apple.spotlightknowledged.updater
  com.apple.managedcorespotlightd

  # Apple Music sync (artwork + devices, library kept for now)
  com.apple.AMPArtworkAgent
  com.apple.AMPDeviceDiscoveryAgent
  com.apple.AMPDevicesAgent
  com.apple.AMPSystemPlayerAgent

  # Exchange (none)
  com.apple.exchange.exchangesyncd
  com.apple.notes.exchangenotesd
  com.apple.calendar.CalendarAgentBookmarkMigrationService

  # Maps sync (Maps usable without)
  com.apple.Maps.mapssyncd
  com.apple.Maps.pushdaemon
  com.apple.maps.destinationd

  # News
  com.apple.newsd

  # Apple Remote Desktop / MDM
  com.apple.RemoteDesktop
  com.apple.RemoteManagementAgent

  # Game Controller (keyboard+mouse only)
  com.apple.GameController.gamecontrolleragentd

  # === Phase 2 — confirmed disabled ===
  # Mail (not used — Gmail web)
  com.apple.icloudmailagent
  com.apple.email.maild
  com.apple.mdworker.mail

  # Apple Music (not used)
  com.apple.AMPLibraryAgent

  # Safari (not used — uses Chrome/Firefox/Arc)
  com.apple.SafariBookmarksSyncAgent
  com.apple.SafariHistoryServiceAgent
  com.apple.Safari.SafeBrowsing.Service
  com.apple.Safari.PasswordBreachAgent
  com.apple.SafariNotificationAgent
)

echo "Disabling ${#KILL_LIST[@]} services..."
disabled=0
not_found=0

for svc in "${KILL_LIST[@]}"; do
    if launchctl disable "user/${UID_}/${svc}" 2>/dev/null; then
        disabled=$((disabled + 1))
    else
        not_found=$((not_found + 1))
    fi
    launchctl bootout "gui/${UID_}/${svc}" 2>/dev/null || true
done

echo "  Persistently disabled: $disabled"
echo "  Not found on this macOS: $not_found"
echo

# Defaults optimizations (no service kill, just settings)
echo "Applying defaults optimizations..."

# Animations
defaults write NSGlobalDomain NSAutomaticWindowAnimationsEnabled -bool false
defaults write -g NSWindowResizeTime -float 0.001
defaults write -g NSScrollAnimationEnabled -bool false
defaults write com.apple.dock launchanim -bool false
defaults write com.apple.dock expose-animation-duration -float 0.1

# Reduce motion / transparency (GPU savings on M4 Pro)
defaults write com.apple.universalaccess reduceMotion -bool true
defaults write com.apple.universalaccess reduceTransparency -bool true

# Recent items off
defaults write -g NSNavRecentPlacesLimit -int 0
defaults write com.apple.dock show-recents -bool false

# Game Mode off (M4 Pro auto-enables it, slows down Ollama/Claude)
defaults write com.apple.GameController GameModeEnabled -bool false

# Continuity Handoff off (per phase 3 request)
defaults -currentHost write com.apple.coreservices.useractivityd ActivityAdvertisingAllowed -bool false
defaults -currentHost write com.apple.coreservices.useractivityd ActivityReceivingAllowed -bool false

# Spotlight: disable indexing on external volumes (main / kept)
if [ -d /Volumes ] && [ "$(ls /Volumes 2>/dev/null | wc -l)" -gt 0 ]; then
    echo "  Disabling Spotlight on external volumes (needs sudo)..."
    sudo mdutil -i off /Volumes 2>/dev/null || echo "  (skipped — sudo declined)"
fi

echo "  defaults applied."
echo

# Apply UI changes
killall Dock 2>/dev/null
killall SystemUIServer 2>/dev/null
killall Finder 2>/dev/null

echo "UI processes restarted."
echo
echo "Waiting 5s for processes to settle..."
sleep 5

# Measure RAM after
ram_after=$(ps -axo rss,user | awk -v u="$USER_" '$2==u {s+=$1} END {printf "%.0f", s/1024}')
saved=$((ram_before - ram_after))

echo
echo "=== Results ==="
echo "  RAM before: ${ram_before} MB"
echo "  RAM after:  ${ram_after} MB"
if [ "$saved" -gt 0 ]; then
    echo "  Saved:      ${saved} MB"
else
    echo "  Note: services restart on demand. Actual savings appear after a reboot."
fi
echo
echo "To verify: bash scripts/check-services.sh"
echo "To restore a service: launchctl enable user/\$(id -u)/<label>"
echo
echo "RECOMMENDED: also disable Apple Intelligence in"
echo "  System Settings → Apple Intelligence & Siri → OFF"
echo "  System Settings → Spotlight → uncheck unused categories"
echo
echo "Reboot to fully apply all changes."
