#!/usr/bin/env bash
# check-services.sh — list which heavy services are currently loaded
# for the running user, color-coded by category and impact.
#
# Run as the user whose RAM you want to minimize (e.g. findmy-bot).

UID_=$(id -u)
USER_=$(whoami)
echo "Services loaded for $USER_ (UID=$UID_):"
echo

# Services to check, grouped by category (label, est-mem-savings-MB)
declare -a SERVICES=(
  # FindMy dependencies — KEEP
  "KEEP|com.apple.findmy.findmylocateagent|FindMy locate agent"
  "KEEP|com.apple.icloud.findmydeviced.findmydevice-user-agent|FindMy device agent"
  "KEEP|com.apple.icloud.searchpartyuseragent|Search Party agent"
  "KEEP|com.apple.CoreLocationAgent|Location services"
  "KEEP|com.apple.akd|Apple ID authentication"

  # Cloud sync heavy — KILL
  "KILL|com.apple.bird|iCloud Drive sync"
  "KILL|com.apple.cloudd|CloudKit daemon"
  "KILL|com.apple.cloudphotod|Photos cloud sync"
  "KILL|com.apple.photoanalysisd|Photos ML analysis (BIG)"
  "KILL|com.apple.photolibraryd|Photos library"
  "KILL|com.apple.appplaceholdersyncd|App Store sync"
  "KILL|com.apple.itunescloudd|Music cloud sync"
  "KILL|com.apple.iCloudNotificationAgent|iCloud notifications"
  "KILL|com.apple.icloudmailagent|iCloud Mail"
  "KILL|com.apple.email.maild|Mail daemon"
  "KILL|com.apple.calendar.CalendarAgent|Calendar agent"
  "KILL|com.apple.exchange.exchangesyncd|Exchange sync"
  "KILL|com.apple.notes.exchangenotesd|Notes Exchange"
  "KILL|com.apple.AddressBook.SourceSync|Contacts sync"
  "KILL|com.apple.AddressBook.abd|Contacts daemon"
  "KILL|com.apple.CallHistorySyncHelper|Call history sync"
  "KILL|com.apple.SafariBookmarksSyncAgent|Safari bookmarks sync"
  "KILL|com.apple.SafariHistoryServiceAgent|Safari history sync"

  # Spotlight — KILL
  "KILL|com.apple.Spotlight|Spotlight UI"
  "KILL|com.apple.corespotlightd|Core Spotlight"
  "KILL|com.apple.spotlightknowledged|Spotlight knowledge"
  "KILL|com.apple.parsec-fbf|Siri suggestions"

  # Apple Intelligence / Siri — KILL (BIG on M4)
  "KILL|com.apple.ModelCatalogAgent|Apple Intelligence models (HUGE)"
  "KILL|com.apple.Siri.agent|Siri"
  "KILL|com.apple.assistantd|Siri assistant"
  "KILL|com.apple.assistant_service|Assistant service"
  "KILL|com.apple.voicebankingd|Voice banking"
  "KILL|com.apple.voicememod|Voice memos"
  "KILL|com.apple.DictationIM|Dictation"

  # Maps / News / Media — KILL
  "KILL|com.apple.Maps.mapssyncd|Maps sync"
  "KILL|com.apple.Maps.pushdaemon|Maps push"
  "KILL|com.apple.newsd|News"
  "KILL|com.apple.AMPLibraryAgent|Music library"
  "KILL|com.apple.AMPArtworkAgent|Music artwork"
  "KILL|com.apple.AMPDevicesAgent|Music devices"
  "KILL|com.apple.NowPlayingTouchUI|Now playing"
  "KILL|com.apple.AirPlayUIAgent|AirPlay UI"

  # Family / ScreenTime — KILL
  "KILL|com.apple.familycircled|Family Sharing"
  "KILL|com.apple.familynotificationd|Family notifications"
  "KILL|com.apple.FamilyControlsAgent|Family controls"
  "KILL|com.apple.ScreenTimeAgent|Screen Time"

  # Background sync — KILL
  "KILL|com.apple.biomeAgent|Biome (behavior tracking)"
  "KILL|com.apple.biomesyncd|Biome sync"
  "KILL|com.apple.cmfsyncagent|Continuity sync"
  "KILL|com.apple.syncdefaultsd|Defaults sync"
  "KILL|com.apple.RemoteDesktop|Remote Desktop"
  "KILL|com.apple.RemoteManagementAgent|MDM agent"
  "KILL|com.apple.TMHelperAgent|Time Machine helper"
  "KILL|com.apple.GameController.gamecontrolleragentd|Game controller"
  "KILL|com.apple.SoftwareUpdateNotificationManager|Software update"
)

# Get all loaded services for this user.
# launchctl print output has lines like:  "   94283   (pe)  com.apple.bird"
# We extract the last whitespace-separated field (the service label).
LOADED=$(launchctl print "gui/${UID_}" 2>/dev/null | awk '/^[[:space:]]+[0-9]+/ {print $NF}')

# Get list of explicitly disabled services
DISABLED=$(launchctl print-disabled "user/${UID_}" 2>/dev/null | grep '=> true' | awk '{print $1}' | sed 's/"//g')

# Color codes
GREEN=$'\e[32m'
RED=$'\e[31m'
YELLOW=$'\e[33m'
GRAY=$'\e[90m'
RESET=$'\e[0m'

active_count=0
kill_active_count=0

for entry in "${SERVICES[@]}"; do
    IFS='|' read -r category label description <<< "$entry"

    is_loaded=false
    is_disabled=false

    if echo "$LOADED" | grep -qF "$label"; then
        is_loaded=true
    fi
    if echo "$DISABLED" | grep -qF "$label"; then
        is_disabled=true
    fi

    if $is_loaded; then
        active_count=$((active_count + 1))
        if [ "$category" = "KILL" ]; then
            kill_active_count=$((kill_active_count + 1))
            printf "%s[ACTIVE]%s %-60s %s\n" "$RED" "$RESET" "$label" "$description"
        else
            printf "%s[KEEP] %s  %-60s %s\n" "$GREEN" "$RESET" "$label" "$description"
        fi
    elif $is_disabled; then
        printf "%s[DISABLED]%s %-58s %s\n" "$GRAY" "$RESET" "$label" "$description"
    else
        printf "%s[notload]%s %-60s %s\n" "$YELLOW" "$RESET" "$label" "$description"
    fi
done

echo
echo "Summary:"
echo "  Active services tracked:  $active_count"
echo "  Active services in KILL list: ${RED}${kill_active_count}${RESET}"
echo
echo "To disable all KILL-listed services, run:"
echo "  bash scripts/disable-services.sh"
