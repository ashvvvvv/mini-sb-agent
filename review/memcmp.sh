#!/bin/bash
# Memory usage comparison: GitHub latest (2b57186, baseline) vs local HEAD.
#
# Both binaries are built with identical flags (default build) from the same
# worktree path, so only the source differs. Each run starts an idle agent
# (empty inbounds, stats API disabled) with N local users applied by the
# panel sync loop, samples the Go heap via -debug-runtime-log and the Windows
# working set, then kills the process.
#
# Methodology notes (learned the hard way):
#   - The FIRST execution of a freshly built exe shows an inflated working
#     set (page touching / AV scanning) that decays within one run; every
#     binary is warmed up once before measured runs.
#   - Run order alternates between the two binaries so time-based machine
#     state cannot favor one side.
#   - default   variant: production-like (no forced GC)
#     scavenge  variant: -scavenge-every 2s forces GC + page return, giving
#     a tight live-heap comparison.
#
# Metrics per run: working set (bytes), goroutines, heap_alloc, heap_sys
# (mean of the last 5 CSV samples at 500ms interval).
set -u

TMP="$LOCALAPPDATA/Temp"
OUT="$TMP/mini-sb-memcmp"
BASE="$TMP/mini-sb-agent-baseline.exe"
FIXED="$TMP/mini-sb-agent-fixed.exe"
SETTLE=12

run_once() { # bin label n trial variant
  local bin=$1 label=$2 n=$3 trial=$4 variant=$5
  local csv="$OUT/${label}-${n}-${trial}-${variant}.csv"
  rm -f "$csv"
  local users=""
  [ "$n" != "0" ] && users="$OUT/users-$n.json"
  local args=(-config "$OUT/config.json" -api "" -panel-every 2s -debug-runtime-log "$csv" -debug-runtime-every 500ms)
  [ -n "$users" ] && args+=(-users "$users")
  [ "$variant" = "scavenge" ] && args+=(-scavenge-every 2s)
  "$bin" "${args[@]}" &
  local pid=$!
  sleep "$SETTLE"
  local wpid
  wpid=$(cat "/proc/$pid/winpid" 2>/dev/null || true)
  local ws=0
  if [ -n "$wpid" ]; then
    ws=$(powershell -NoProfile -Command "(Get-Process -Id $wpid).WorkingSet64" 2>/dev/null | tr -d '\r' || true)
    taskkill //PID "$wpid" //F >/dev/null 2>&1 || true
  fi
  wait "$pid" 2>/dev/null
  tail -5 "$csv" | awk -F, -v ws="$ws" -v tag="$label-$variant" -v n="$n" -v trial="$trial" '
    {g+=$15; ha+=$16; hs+=$17}
    END {
      if (NR == 0) { printf "%s %s %s %d EMPTY\n", tag, n, trial, ws }
      else { printf "%s %s %s %d %.1f %.0f %.0f\n", tag, n, trial, ws, g/NR, ha/NR, hs/NR }
    }' >> "$OUT/results.txt"
}

mkdir -p "$OUT"

# Warm-up: one short unmeasured run per binary (first-execution page effects).
for bin in "$BASE" "$FIXED"; do
  "$bin" -config "$OUT/config.json" -api "" -debug-runtime-log "$OUT/warmup.csv" &
  local_pid=$!
  sleep 3
  taskkill //PID "$(cat /proc/$local_pid/winpid)" //F >/dev/null 2>&1 || true
  wait $local_pid 2>/dev/null
done

: > "$OUT/results.txt"

for variant in default scavenge; do
  for n in 0 1000 10000; do
    if [ "$variant" = "scavenge" ] && [ "$n" != "10000" ] && [ "$n" != "0" ]; then
      continue # scavenge only at the extremes
    fi
    for trial in 1 2 3; do
      if [ $((trial % 2)) -eq 1 ]; then
        run_once "$BASE" baseline "$n" "$trial" "$variant"
        run_once "$FIXED" fixed "$n" "$trial" "$variant"
      else
        run_once "$FIXED" fixed "$n" "$trial" "$variant"
        run_once "$BASE" baseline "$n" "$trial" "$variant"
      fi
    done
  done
done

echo "=== raw rows: tag n trial ws_bytes goroutines heap_alloc heap_sys ==="
cat "$OUT/results.txt"
echo
echo "=== means per tag/n ==="
awk '{key=$1" n="$2; ws[key]+=$4; g[key]+=$5; ha[key]+=$6; hs[key]+=$7; cnt[key]++}
  END {for (k in ws) printf "%-22s ws=%8.2f MiB  goroutines=%5.1f  heap_alloc=%7.2f MiB  heap_sys=%7.2f MiB\n",
    k, ws[k]/cnt[k]/1048576, g[k]/cnt[k], ha[k]/cnt[k]/1048576, hs[k]/cnt[k]/1048576}' "$OUT/results.txt" | sort
