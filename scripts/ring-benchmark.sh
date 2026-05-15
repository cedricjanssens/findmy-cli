#!/bin/bash
# Ring benchmark — tests different approaches to reach "Émettre un son"
# NEVER clicks the actual button. Measures if it's found in OCR.

HELPER="./bin/findmy-helper"
WINDOW_X=2560
WINDOW_Y=0
SIDEBAR_X=$((WINDOW_X + 150))
SIDEBAR_Y=$((WINDOW_Y + 540))
RESULTS_DIR="/tmp/findmy-cli/benchmark"
mkdir -p "$RESULTS_DIR"

get_window_id() {
    $HELPER window --owner "Localiser" 2>/dev/null | python3 -c "
import sys, json
for w in json.load(sys.stdin):
    if w['height'] > 100 and w['onScreen']:
        print(w['windowID']); break
" 2>/dev/null
}

reset_state() {
    # Switch to People tab to reset
    osascript -e '
    tell application "System Events"
        tell process "FindMy"
            click menu item "Personnes" of menu "Présentation" of menu bar 1
        end tell
    end tell
    ' 2>/dev/null
    sleep 1
}

switch_to_devices() {
    osascript -e '
    tell application "System Events"
        tell process "FindMy"
            click menu item "Appareils" of menu "Présentation" of menu bar 1
        end tell
    end tell
    ' 2>/dev/null
    sleep 1.5
}

scroll_to_top() {
    for i in $(seq 1 15); do
        $HELPER scroll $SIDEBAR_X $SIDEBAR_Y 5 2>/dev/null
        sleep 0.15
    done
    sleep 0.5
}

scroll_down() {
    local n=${1:-3}
    for i in $(seq 1 $n); do
        $HELPER scroll $SIDEBAR_X $SIDEBAR_Y -3 2>/dev/null
        sleep 0.2
    done
    sleep 0.5
}

find_christel_y() {
    local wid=$1
    /usr/sbin/screencapture -x -l "$wid" -t png /tmp/findmy-cli/bench-scan.png 2>/dev/null
    $HELPER ocr /tmp/findmy-cli/bench-scan.png 2>/dev/null | python3 -c "
import sys, json
for l in json.load(sys.stdin):
    if 'iPhone14PM Christel' in l['text'] or 'iPhone14PM christel' in l['text'].lower():
        print(f'{l[\"x\"]} {l[\"y\"]}')
        break
" 2>/dev/null
}

check_play_sound() {
    local wid=$1
    local shot="$2"
    /usr/sbin/screencapture -x -l "$wid" -t png "$shot" 2>/dev/null
    $HELPER ocr "$shot" 2>/dev/null | python3 -c "
import sys, json
for l in json.load(sys.stdin):
    lower = l['text'].lower().strip()
    if 'mettre un son' in lower or 'emettre un son' in lower:
        print(f'FOUND x={l[\"x\"]} y={l[\"y\"]} w={l[\"width\"]} h={l[\"height\"]}')
        break
else:
    print('NOT_FOUND')
" 2>/dev/null
}

# ========== APPROACH A: click sidebar → double-click map center ==========
approach_a() {
    local wid=$1
    local cx=$2
    local cy=$3

    # Click device in sidebar
    $HELPER click $((WINDOW_X + cx)) $((WINDOW_Y + cy + 8)) 2>/dev/null
    sleep 3

    # Double-click map center
    local map_cx=$((WINDOW_X + 340 + (1920-340)/2))
    local map_cy=$((WINDOW_Y + 540))
    $HELPER click $map_cx $map_cy 2>/dev/null
    sleep 1
    $HELPER click $map_cx $map_cy 2>/dev/null
    sleep 2
}

# ========== APPROACH B: click sidebar → single click map center → find (i) → click (i) ==========
approach_b() {
    local wid=$1
    local cx=$2
    local cy=$3

    # Click device in sidebar
    $HELPER click $((WINDOW_X + cx)) $((WINDOW_Y + cy + 8)) 2>/dev/null
    sleep 3

    # Single click map center to show popup
    local map_cx=$((WINDOW_X + 340 + (1920-340)/2))
    local map_cy=$((WINDOW_Y + 540))
    $HELPER click $map_cx $map_cy 2>/dev/null
    sleep 2

    # OCR to find popup, click right of it (i button)
    /usr/sbin/screencapture -x -l "$wid" -t png /tmp/findmy-cli/bench-popup.png 2>/dev/null
    local coords=$($HELPER ocr /tmp/findmy-cli/bench-popup.png 2>/dev/null | python3 -c "
import sys, json
best_right = 0
best_y = 0
for l in json.load(sys.stdin):
    if l['x'] > 340 and l['x'] < 1800 and len(l['text'].strip()) > 3:
        t = l['text'].strip()
        if t == t.upper() and len(t) > 5: continue  # skip map labels
        if t in ('3D', 'N', '+'): continue
        right = l['x'] + l['width']
        if right > best_right:
            best_right = right
            best_y = l['y'] + l['height'] // 2
if best_right > 0:
    print(f'{best_right + 25} {best_y}')
" 2>/dev/null)

    if [ -n "$coords" ]; then
        local ix=$(echo $coords | cut -d' ' -f1)
        local iy=$(echo $coords | cut -d' ' -f2)
        $HELPER click $((WINDOW_X + ix)) $((WINDOW_Y + iy)) 2>/dev/null
        sleep 2
    fi
}

# ========== APPROACH C: activate + click sidebar → double-click map center ==========
approach_c() {
    local wid=$1
    local cx=$2
    local cy=$3

    osascript -e 'tell application "FindMy" to activate' 2>/dev/null
    sleep 1

    # Click device in sidebar
    $HELPER click $((WINDOW_X + cx)) $((WINDOW_Y + cy + 8)) 2>/dev/null
    sleep 3

    # Double-click map center
    local map_cx=$((WINDOW_X + 340 + (1920-340)/2))
    local map_cy=$((WINDOW_Y + 540))
    $HELPER click $map_cx $map_cy 2>/dev/null
    sleep 1
    $HELPER click $map_cx $map_cy 2>/dev/null
    sleep 2
}

# ========== APPROACH D: activate + click sidebar → triple click map center (slower) ==========
approach_d() {
    local wid=$1
    local cx=$2
    local cy=$3

    osascript -e 'tell application "FindMy" to activate' 2>/dev/null
    sleep 1

    $HELPER click $((WINDOW_X + cx)) $((WINDOW_Y + cy + 8)) 2>/dev/null
    sleep 3

    local map_cx=$((WINDOW_X + 340 + (1920-340)/2))
    local map_cy=$((WINDOW_Y + 540))
    $HELPER click $map_cx $map_cy 2>/dev/null
    sleep 1.5
    $HELPER click $map_cx $map_cy 2>/dev/null
    sleep 1.5
    $HELPER click $map_cx $map_cy 2>/dev/null
    sleep 2
}

# ========== APPROACH E: activate + click sidebar → click popup name text directly ==========
approach_e() {
    local wid=$1
    local cx=$2
    local cy=$3

    osascript -e 'tell application "FindMy" to activate' 2>/dev/null
    sleep 1

    $HELPER click $((WINDOW_X + cx)) $((WINDOW_Y + cy + 8)) 2>/dev/null
    sleep 3

    # Single click map center to show popup
    local map_cx=$((WINDOW_X + 340 + (1920-340)/2))
    local map_cy=$((WINDOW_Y + 540))
    $HELPER click $map_cx $map_cy 2>/dev/null
    sleep 2

    # Find popup text and click ON it (not to the right)
    /usr/sbin/screencapture -x -l "$wid" -t png /tmp/findmy-cli/bench-popup.png 2>/dev/null
    local coords=$($HELPER ocr /tmp/findmy-cli/bench-popup.png 2>/dev/null | python3 -c "
import sys, json
for l in json.load(sys.stdin):
    if l['x'] > 340 and ('christel' in l['text'].lower() or 'iPhone14PM' in l['text']):
        print(f'{l[\"x\"] + l[\"width\"]//2} {l[\"y\"] + l[\"height\"]//2}')
        break
" 2>/dev/null)

    if [ -n "$coords" ]; then
        local tx=$(echo $coords | cut -d' ' -f1)
        local ty=$(echo $coords | cut -d' ' -f2)
        # Double-click on the popup text itself
        $HELPER click $((WINDOW_X + tx)) $((WINDOW_Y + ty)) 2>/dev/null
        sleep 0.5
        $HELPER click $((WINDOW_X + tx)) $((WINDOW_Y + ty)) 2>/dev/null
        sleep 2
    fi
}

# ========== RUN BENCHMARKS ==========
echo "=== Ring Benchmark — $(date) ==="
echo "Target: iPhone14PM Christel"
echo ""

WID=$(get_window_id)
echo "Window ID: $WID"

# First, find Christel's position once
switch_to_devices
scroll_to_top
scroll_down 12

POS=$(find_christel_y $WID)
if [ -z "$POS" ]; then
    echo "ERROR: Could not find iPhone14PM Christel in sidebar"
    # Try more scrolling
    scroll_down 5
    POS=$(find_christel_y $WID)
fi

if [ -z "$POS" ]; then
    echo "FATAL: iPhone14PM Christel not found after extended scroll"
    exit 1
fi

CX=$(echo $POS | cut -d' ' -f1)
CY=$(echo $POS | cut -d' ' -f2)
echo "Christel position: x=$CX y=$CY"
echo ""

# Run each approach 4 times
for approach in a b c d e; do
    echo "--- Approach $approach ---"
    successes=0
    for run in 1 2 3 4; do
        reset_state
        sleep 1
        switch_to_devices
        scroll_to_top
        scroll_down 12
        sleep 0.5

        # Re-find position (may shift after scroll)
        POS=$(find_christel_y $WID)
        if [ -z "$POS" ]; then
            scroll_down 3
            POS=$(find_christel_y $WID)
        fi
        if [ -z "$POS" ]; then
            echo "  Run $run: SKIP (device not visible)"
            continue
        fi
        CX=$(echo $POS | cut -d' ' -f1)
        CY=$(echo $POS | cut -d' ' -f2)

        START=$(python3 -c "import time; print(time.time())")

        approach_${approach} "$WID" "$CX" "$CY"

        END=$(python3 -c "import time; print(time.time())")
        DURATION=$(python3 -c "print(f'{$END - $START:.1f}s')")

        result=$(check_play_sound "$WID" "$RESULTS_DIR/approach_${approach}_run${run}.png")

        if echo "$result" | grep -q "FOUND"; then
            echo "  Run $run: OK ($DURATION) $result"
            successes=$((successes + 1))
        else
            echo "  Run $run: FAIL ($DURATION)"
        fi
    done
    echo "  Score: $successes/4"
    echo ""
done

echo "=== Benchmark complete ==="
