#!/bin/bash
# Run findmy ring in dry-run mode 20 times, measure success rate.
# NEVER rings — dry-run only.

echo "=== Ring reliability test — $(date) ==="
echo "Target: christel (dry-run, NO ring)"
echo ""

successes=0
failures=0
skips=0
total=20

for i in $(seq 1 $total); do
    START=$(python3 -c "import time; print(time.time())")

    OUTPUT=$(./bin/findmy ring "christel" --keep 2>&1)
    EXIT=$?

    END=$(python3 -c "import time; print(time.time())")
    DURATION=$(python3 -c "print(f'{$END - $START:.1f}s')")

    if echo "$OUTPUT" | grep -q "would click"; then
        COORDS=$(echo "$OUTPUT" | grep "would click" | grep -o "([0-9]*, [0-9]*)")
        echo "  Run $i: SUCCESS ($DURATION) — button at $COORDS"
        successes=$((successes + 1))
    elif echo "$OUTPUT" | grep -q "no device matching"; then
        echo "  Run $i: SKIP ($DURATION) — device not found in scan"
        skips=$((skips + 1))
    else
        REASON=$(echo "$OUTPUT" | tail -1)
        echo "  Run $i: FAIL ($DURATION) — $REASON"
        failures=$((failures + 1))
    fi
done

echo ""
echo "=== Results ==="
echo "  Success: $successes/$total"
echo "  Fail:    $failures/$total"
echo "  Skip:    $skips/$total"
echo "  Rate:    $(python3 -c "print(f'{$successes/$total*100:.0f}%')")"
