#!/usr/bin/env sh
# SeedGrid — reattaches the storage and recovers the Torrent Engine when the /data disk
# reconnects (hotplug). A USB drive that drops from the bus and returns re-enumerates with a different
# /dev (e.g.: sda → sdb); fstab remounts by UUID, but the already running qBittorrent keeps
# handles to the old disk and the writing torrents crash with ERROR. This script
# remounts /data if needed, restarts the Engine (reopens files on the current mount) and
# rechecks errored torrents. Triggered by udev (99-seedgrid-storage.rules) via the
# seedgrid-storage-reattach.service unit. Idempotent, with debounce.
#
# Relation to compose mount propagation (#143): the /data bind uses
# `propagation: rslave`, so a disk mounted on the host AFTER the Engine comes up already
# appears inside the container without recreating it. This does NOT make the restart below
# dispensable. Measured: if the Engine has a file open on the old filesystem, it remains
# the owner of that mount even after the host remounts the disk — the new mount does not
# replace it inside the container, and the handles keep pointing to the device that
# disappeared. Only a restart drops these handles (and is enough: the container comes up with the
# host's current mount, no need to recreate). Propagation solves "the disk arrived later";
# this script solves "the disk dropped and came back".
set -u

TAG=seedgrid-storage
STAMP=/run/seedgrid-storage-reattach.stamp
log() { logger -t "$TAG" "$*" 2>/dev/null; echo "$TAG: $*"; }

# The Agent env provides the Engine container name (SEEDGRID_ENGINE_CONTAINER) and the
# WebUI credentials. On a Node whose container still has the old name, it is the one that tells
# which one to restart — the fallback applies to those who never defined the key.
[ -f /etc/seedgrid-agent.env ] && . /etc/seedgrid-agent.env 2>/dev/null || true
ENGINE_CONTAINER="${SEEDGRID_ENGINE_CONTAINER:-seedgrid-engine}"

# Debounce: udev fires multiple times (partition + disk). Ignores re-entries < 30s.
now=$(date +%s)
if [ -f "$STAMP" ]; then
  last=$(cat "$STAMP" 2>/dev/null || echo 0)
  [ $((now - last)) -lt 30 ] && exit 0
fi
echo "$now" > "$STAMP" 2>/dev/null || true

# 1) Ensures /data is mounted and writable (fstab defines the device by UUID).
if mountpoint -q /data 2>/dev/null; then
  if touch /data/.seedgrid-wtest 2>/dev/null; then
    rm -f /data/.seedgrid-wtest
  else
    log "/data mounted but not writable — remounting"
    umount -l /data 2>/dev/null || true
    mount /data 2>/dev/null || { log "failed to remount /data"; exit 1; }
  fi
else
  log "mounting /data"
  mount /data 2>/dev/null || { log "failed to mount /data"; exit 1; }
fi

# 2) Restarts the Engine to reopen files on the current mount (clears old disk handles).
if command -v docker >/dev/null 2>&1; then
  log "restarting Torrent Engine ($ENGINE_CONTAINER)"
  docker restart "$ENGINE_CONTAINER" >/dev/null 2>&1 || log "warning: restart of $ENGINE_CONTAINER failed (does the name match the compose container_name?)"
fi

# 3) Best-effort: rechecks torrents in ERROR after the restart (revalidates data on disk).
QURL="${SEEDGRID_ENGINE_QBT_URL:-http://127.0.0.1:8080}"
QUSER="${SEEDGRID_ENGINE_QBT_USERNAME:-admin}"
QPASS="${SEEDGRID_ENGINE_QBT_PASSWORD:-}"
if command -v curl >/dev/null 2>&1 && command -v python3 >/dev/null 2>&1; then
  CJ=$(mktemp)
  i=0; while [ $i -lt 30 ]; do curl -sf -o /dev/null "$QURL/api/v2/app/version" 2>/dev/null && break; i=$((i + 1)); sleep 2; done
  REF="Referer: $QURL"
  curl -s -c "$CJ" "$QURL/api/v2/auth/login" --data-urlencode "username=$QUSER" --data-urlencode "password=$QPASS" -H "$REF" >/dev/null 2>&1
  HASHES=$(curl -s -b "$CJ" "$QURL/api/v2/torrents/info" -H "$REF" 2>/dev/null | python3 -c '
import sys, json
try:
    ts = json.load(sys.stdin)
except Exception:
    sys.exit(0)
print("|".join(t["hash"] for t in ts if t.get("state") in ("error", "missingFiles")))
' 2>/dev/null)
  if [ -n "$HASHES" ]; then
    log "rechecking torrents in error"
    curl -s -b "$CJ" "$QURL/api/v2/torrents/recheck" --data-urlencode "hashes=$HASHES" -H "$REF" >/dev/null 2>&1
  fi
  rm -f "$CJ"
fi
log "storage reattachment completed"
