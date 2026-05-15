#!/bin/bash
# Ring benchmark v2 — tests with FindMy on real display
# NEVER clicks "Émettre un son". Only checks if it appears in OCR.

HELPER="./bin/findmy-helper"
RESULTS_DIR="/tmp/findmy-cli/benchmark2"
mkdir -p "$RESULTS_DIR"

get_window_id() {
    $HELPER window --owner "Localiser" 2>/dev/null | python3 -c "
import sys, json
for w in json.load(sys.stdin):
    if w['height'] > 100:
        print(f'{w[\"windowID\"]} {w[\"x\"]} {w[\"y\"]} {w[\"width\"]} {w[\"height\"]} {w[\"onScreen\"]}')
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
    if 'mettre un son' in lower or 'emettre' in lower:
        print(f'BUTTON_FOUND x={l[\"x\"]} y={l[\"y\"]} w={l[\"width\"]}')
        sys.exit(0)
print('BUTTON_NOT_FOUND')
" 2>/dev/null
}

move_to_real() {
    osascript -e '
    tell application "FindMy" to activate
    delay 0.5
    tell application "System Events"
        tell process "FindMy"
            set position of window 1 to {100, 100}
            set size of window 1 to {1400, 900}
        end tell
    end tell
    ' 2>/dev/null
    sleep 1
}

move_to_virtual() {
    osascript -e '
    tell application "System Events"
        tell process "FindMy"
            set position of window 1 to {2560, 0}
            set size of window 1 to {1920, 1080}
        end tell
    end tell
    ' 2>/dev/null
    sleep 0.5
}

reset_state() {
    osascript -e '
    tell application "System Events"
        tell process "FindMy"
            click menu item "Personnes" of menu "Présentation" of menu bar 1
        end tell
    end tell
    ' 2>/dev/null
    sleep 1
}

echo "=== Ring Benchmark v2 — $(date) ==="
echo "Strategy: move FindMy to real display for clicks"
echo ""

# ========== APPROACH F: real display, activate, click sidebar, double-click map ==========
echo "--- Approach F: real display + activate + sidebar + double-click map ---"
successes=0
total=5
for run in $(seq 1 $total); do
    START=$(python3 -c "import time; print(time.time())")

    reset_state
    move_to_real

    # Switch to Devices tab
    osascript -e '
    tell application "System Events"
        tell process "FindMy"
            click menu item "Appareils" of menu "Présentation" of menu bar 1
        end tell
    end tell
    ' 2>/dev/null
    sleep 1.5

    # Scroll to Christel
    INFO=$(get_window_id)
    WID=$(echo $INFO | cut -d' ' -f1)
    WX=$(echo $INFO | cut -d' ' -f2)
    WY=$(echo $INFO | cut -d' ' -f3)
    WW=$(echo $INFO | cut -d' ' -f4)
    WH=$(echo $INFO | cut -d' ' -f5)

    SX=$((WX + 150))
    SY=$((WY + WH/2))
    for i in $(seq 1 15); do $HELPER scroll $SX $SY -3 2>/dev/null; sleep 0.15; done
    sleep 0.5

    # Find Christel
    /usr/sbin/screencapture -x -l "$WID" -t png /tmp/findmy-cli/bench2-scan.png 2>/dev/null
    POS=$($HELPER ocr /tmp/findmy-cli/bench2-scan.png 2>/dev/null | python3 -c "
import sys, json
for l in json.load(sys.stdin):
    if 'iPhone14PM Christel' in l['text']:
        print(f'{l[\"x\"]} {l[\"y\"]}'); break
" 2>/dev/null)

    if [ -z "$POS" ]; then
        # Scroll more
        for i in $(seq 1 5); do $HELPER scroll $SX $SY -3 2>/dev/null; sleep 0.15; done
        sleep 0.5
        /usr/sbin/screencapture -x -l "$WID" -t png /tmp/findmy-cli/bench2-scan.png 2>/dev/null
        POS=$($HELPER ocr /tmp/findmy-cli/bench2-scan.png 2>/dev/null | python3 -c "
import sys, json
for l in json.load(sys.stdin):
    if 'iPhone14PM Christel' in l['text']:
        print(f'{l[\"x\"]} {l[\"y\"]}'); break
" 2>/dev/null)
    fi

    if [ -z "$POS" ]; then
        echo "  Run $run: SKIP (device not found)"
        continue
    fi

    # Compute scale
    IMG_W=$(python3 -c "
import struct
with open('/tmp/findmy-cli/bench2-scan.png','rb') as f:
    f.read(16)
    print(struct.unpack('>I',f.read(4))[0])
" 2>/dev/null)
    SCALE=$(python3 -c "print($IMG_W / $WW)" 2>/dev/null)

    CX=$(echo $POS | cut -d' ' -f1)
    CY=$(echo $POS | cut -d' ' -f2)

    # Click device in sidebar (screen coords)
    CLICK_X=$(python3 -c "print(int($WX + $CX / $SCALE))" 2>/dev/null)
    CLICK_Y=$(python3 -c "print(int($WY + $CY / $SCALE + 8))" 2>/dev/null)
    $HELPER click $CLICK_X $CLICK_Y 2>/dev/null
    sleep 3

    # Double-click map center
    SIDEBAR_PTS=$(python3 -c "print(int(340))" 2>/dev/null)
    MAP_CX=$((WX + SIDEBAR_PTS + (WW - SIDEBAR_PTS) / 2))
    MAP_CY=$((WY + WH / 2))
    $HELPER click $MAP_CX $MAP_CY 2>/dev/null
    sleep 1
    $HELPER click $MAP_CX $MAP_CY 2>/dev/null
    sleep 2

    END=$(python3 -c "import time; print(time.time())")
    DURATION=$(python3 -c "print(f'{$END - $START:.1f}s')")

    result=$(check_play_sound "$WID" "$RESULTS_DIR/f_run${run}.png")
    if echo "$result" | grep -q "BUTTON_FOUND"; then
        echo "  Run $run: SUCCESS ($DURATION) $result"
        successes=$((successes + 1))
    else
        echo "  Run $run: FAIL ($DURATION)"
    fi
done
echo "  Score: $successes/$total"
echo ""

# ========== APPROACH G: real display, activate, click sidebar, triple-click map, longer waits ==========
echo "--- Approach G: real display + longer waits + triple-click ---"
successes=0
for run in $(seq 1 $total); do
    START=$(python3 -c "import time; print(time.time())")

    reset_state
    move_to_real

    osascript -e '
    tell application "System Events"
        tell process "FindMy"
            click menu item "Appareils" of menu "Présentation" of menu bar 1
        end tell
    end tell
    ' 2>/dev/null
    sleep 2

    INFO=$(get_window_id)
    WID=$(echo $INFO | cut -d' ' -f1)
    WX=$(echo $INFO | cut -d' ' -f2)
    WY=$(echo $INFO | cut -d' ' -f3)
    WW=$(echo $INFO | cut -d' ' -f4)
    WH=$(echo $INFO | cut -d' ' -f5)

    SX=$((WX + 150))
    SY=$((WY + WH/2))
    for i in $(seq 1 20); do $HELPER scroll $SX $SY -3 2>/dev/null; sleep 0.12; done
    sleep 0.5

    /usr/sbin/screencapture -x -l "$WID" -t png /tmp/findmy-cli/bench2-scan.png 2>/dev/null
    POS=$($HELPER ocr /tmp/findmy-cli/bench2-scan.png 2>/dev/null | python3 -c "
import sys, json
for l in json.load(sys.stdin):
    if 'iPhone14PM Christel' in l['text']:
        print(f'{l[\"x\"]} {l[\"y\"]}'); break
" 2>/dev/null)

    if [ -z "$POS" ]; then
        echo "  Run $run: SKIP (device not found)"
        continue
    fi

    IMG_W=$(python3 -c "
import struct
with open('/tmp/findmy-cli/bench2-scan.png','rb') as f:
    f.read(16)
    print(struct.unpack('>I',f.read(4))[0])
" 2>/dev/null)
    SCALE=$(python3 -c "print($IMG_W / $WW)" 2>/dev/null)

    CX=$(echo $POS | cut -d' ' -f1)
    CY=$(echo $POS | cut -d' ' -f2)
    CLICK_X=$(python3 -c "print(int($WX + $CX / $SCALE))" 2>/dev/null)
    CLICK_Y=$(python3 -c "print(int($WY + $CY / $SCALE + 8))" 2>/dev/null)
    $HELPER click $CLICK_X $CLICK_Y 2>/dev/null
    sleep 4

    SIDEBAR_PTS=340
    MAP_CX=$((WX + SIDEBAR_PTS + (WW - SIDEBAR_PTS) / 2))
    MAP_CY=$((WY + WH / 2))
    $HELPER click $MAP_CX $MAP_CY 2>/dev/null
    sleep 1.5
    $HELPER click $MAP_CX $MAP_CY 2>/dev/null
    sleep 1.5
    $HELPER click $MAP_CX $MAP_CY 2>/dev/null
    sleep 2.5

    END=$(python3 -c "import time; print(time.time())")
    DURATION=$(python3 -c "print(f'{$END - $START:.1f}s')")

    result=$(check_play_sound "$WID" "$RESULTS_DIR/g_run${run}.png")
    if echo "$result" | grep -q "BUTTON_FOUND"; then
        echo "  Run $run: SUCCESS ($DURATION) $result"
        successes=$((successes + 1))
    else
        echo "  Run $run: FAIL ($DURATION)"
    fi
done
echo "  Score: $successes/$total"
echo ""

# ========== APPROACH H: real display, activate, click sidebar, find popup (i), click it ==========
echo "--- Approach H: real display + sidebar + find popup + click (i) ---"
successes=0
for run in $(seq 1 $total); do
    START=$(python3 -c "import time; print(time.time())")

    reset_state
    move_to_real

    osascript -e '
    tell application "System Events"
        tell process "FindMy"
            click menu item "Appareils" of menu "Présentation" of menu bar 1
        end tell
    end tell
    ' 2>/dev/null
    sleep 2

    INFO=$(get_window_id)
    WID=$(echo $INFO | cut -d' ' -f1)
    WX=$(echo $INFO | cut -d' ' -f2)
    WY=$(echo $INFO | cut -d' ' -f3)
    WW=$(echo $INFO | cut -d' ' -f4)
    WH=$(echo $INFO | cut -d' ' -f5)

    SX=$((WX + 150))
    SY=$((WY + WH/2))
    for i in $(seq 1 20); do $HELPER scroll $SX $SY -3 2>/dev/null; sleep 0.12; done
    sleep 0.5

    /usr/sbin/screencapture -x -l "$WID" -t png /tmp/findmy-cli/bench2-scan.png 2>/dev/null
    POS=$($HELPER ocr /tmp/findmy-cli/bench2-scan.png 2>/dev/null | python3 -c "
import sys, json
for l in json.load(sys.stdin):
    if 'iPhone14PM Christel' in l['text']:
        print(f'{l[\"x\"]} {l[\"y\"]}'); break
" 2>/dev/null)

    if [ -z "$POS" ]; then
        echo "  Run $run: SKIP (device not found)"
        continue
    fi

    IMG_W=$(python3 -c "
import struct
with open('/tmp/findmy-cli/bench2-scan.png','rb') as f:
    f.read(16)
    print(struct.unpack('>I',f.read(4))[0])
" 2>/dev/null)
    SCALE=$(python3 -c "print($IMG_W / $WW)" 2>/dev/null)

    CX=$(echo $POS | cut -d' ' -f1)
    CY=$(echo $POS | cut -d' ' -f2)
    CLICK_X=$(python3 -c "print(int($WX + $CX / $SCALE))" 2>/dev/null)
    CLICK_Y=$(python3 -c "print(int($WY + $CY / $SCALE + 8))" 2>/dev/null)
    $HELPER click $CLICK_X $CLICK_Y 2>/dev/null
    sleep 3

    # Single click map center → popup
    MAP_CX=$((WX + 340 + (WW - 340) / 2))
    MAP_CY=$((WY + WH / 2))
    $HELPER click $MAP_CX $MAP_CY 2>/dev/null
    sleep 2

    # OCR to find popup, click its (i)
    /usr/sbin/screencapture -x -l "$WID" -t png /tmp/findmy-cli/bench2-popup.png 2>/dev/null
    I_POS=$($HELPER ocr /tmp/findmy-cli/bench2-popup.png 2>/dev/null | python3 -c "
import sys, json
# Find rightmost non-label text in map area
best_right, best_y = 0, 0
for l in json.load(sys.stdin):
    if l['x'] < 340 * 2: continue  # scale-adjusted sidebar
    t = l['text'].strip()
    if len(t) < 3 or t in ('3D','N','+'): continue
    if t == t.upper() and len(t) > 5: continue  # map label
    right = l['x'] + l['width']
    if right > best_right:
        best_right = right
        best_y = l['y'] + l['height'] // 2
if best_right > 0:
    print(f'{best_right + 40} {best_y}')
" 2>/dev/null)

    if [ -n "$I_POS" ]; then
        IX=$(echo $I_POS | cut -d' ' -f1)
        IY=$(echo $I_POS | cut -d' ' -f2)
        $HELPER click $(python3 -c "print(int($WX + $IX / $SCALE))") $(python3 -c "print(int($WY + $IY / $SCALE))") 2>/dev/null
        sleep 2
    else
        # Fallback: double-click map center
        $HELPER click $MAP_CX $MAP_CY 2>/dev/null
        sleep 2
    fi

    END=$(python3 -c "import time; print(time.time())")
    DURATION=$(python3 -c "print(f'{$END - $START:.1f}s')")

    result=$(check_play_sound "$WID" "$RESULTS_DIR/h_run${run}.png")
    if echo "$result" | grep -q "BUTTON_FOUND"; then
        echo "  Run $run: SUCCESS ($DURATION) $result"
        successes=$((successes + 1))
    else
        echo "  Run $run: FAIL ($DURATION)"
    fi
done
echo "  Score: $successes/$total"
echo ""

# Move FindMy back to virtual display and restore People tab
move_to_virtual
reset_state

echo "=== Benchmark v2 complete ==="
