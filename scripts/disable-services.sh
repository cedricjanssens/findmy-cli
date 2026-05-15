#!/usr/bin/env bash
# disable-services.sh — disable heavy services for the running user.
# Persistent (survives logout) + immediate (bootout active session).
#
# IMPORTANT: run as the user whose RAM you want to minimize.
# DO NOT run on your main user account unless you want a leaner setup
# for it too — these disables affect Spotlight, Photos, Siri, Mail, etc.

set -u

UID_=$(id -u)
USER_=$(whoami)

read -p "Disable heavy services for user '$USER_' (UID=$UID_)? [y/N] " ans
if [[ ! "$ans" =~ ^[yY]$ ]]; then
    echo "Aborted."
    exit 0
fi

# Services to disable (matches the KILL list in check-services.sh)
KILL_LIST=(
  com.apple.bird
  com.apple.cloudd
  com.apple.cloudphotod
  com.apple.photoanalysisd
  com.apple.photolibraryd
  com.apple.appplaceholdersyncd
  com.apple.itunescloudd
  com.apple.iCloudNotificationAgent
  com.apple.iCloudUserNotifications
  com.apple.icloudmailagent
  com.apple.email.maild
  com.apple.calendar.CalendarAgent
  com.apple.exchange.exchangesyncd
  com.apple.notes.exchangenotesd
  com.apple.AddressBook.SourceSync
  com.apple.AddressBook.AssistantService
  com.apple.AddressBook.abd
  com.apple.CallHistoryPluginHelper
  com.apple.CallHistorySyncHelper
  com.apple.facetimemessagestored
  com.apple.SafariBookmarksSyncAgent
  com.apple.SafariHistoryServiceAgent
  com.apple.Safari.SafeBrowsing.Service
  com.apple.Safari.PasswordBreachAgent
  com.apple.SafariNotificationAgent
  com.apple.Spotlight
  com.apple.corespotlightd
  com.apple.corespotlightservice
  com.apple.spotlightknowledged
  com.apple.parsec-fbf
  com.apple.ModelCatalogAgent
  com.apple.Siri.agent
  com.apple.SiriTTSTrainingAgent
  com.apple.assistantd
  com.apple.assistant_service
  com.apple.voicebankingd
  com.apple.voicememod
  com.apple.DictationIM
  com.apple.Maps.mapssyncd
  com.apple.Maps.pushdaemon
  com.apple.maps.destinationd
  com.apple.newsd
  com.apple.AMPArtworkAgent
  com.apple.AMPDeviceDiscoveryAgent
  com.apple.AMPDevicesAgent
  com.apple.AMPLibraryAgent
  com.apple.AMPSystemPlayerAgent
  com.apple.NowPlayingTouchUI
  com.apple.PIPAgent
  com.apple.AirPlayUIAgent
  com.apple.familycircled
  com.apple.familynotificationd
  com.apple.familycontrols.useragent
  com.apple.FamilyControlsAgent
  com.apple.ScreenTimeAgent
  com.apple.biomeAgent
  com.apple.biomesyncd
  com.apple.cmfsyncagent
  com.apple.syncdefaultsd
  com.apple.RemoteDesktop
  com.apple.RemoteManagementAgent
  com.apple.TMHelperAgent
  com.apple.GameController.gamecontrolleragentd
  com.apple.SoftwareUpdateNotificationManager
)

echo "Disabling ${#KILL_LIST[@]} services..."
disabled=0
errors=0

for svc in "${KILL_LIST[@]}"; do
    # disable = persistent across logout
    if launchctl disable "user/${UID_}/${svc}" 2>/dev/null; then
        disabled=$((disabled + 1))
    fi
    # bootout = stop the running instance now
    launchctl bootout "gui/${UID_}/${svc}" 2>/dev/null || true
done

echo "Disabled: $disabled / ${#KILL_LIST[@]}"
echo
echo "Note: some services may not exist on your macOS version (skipped silently)."
echo "Persistent: survives logout via launchctl disable."
echo "To re-enable: launchctl enable user/\$(id -u)/<label>"
echo
echo "Verify with: bash scripts/check-services.sh"
