#!/usr/bin/env sh
# SeedGrid — Seed Node bootstrap (zero-touch onboarding, ADR-017).
# Runs on a new Node (e.g.: Raspberry Pi, clean 64-bit OS). The Control Plane assigns the
# BitTorrent port, the WebUI password is generated locally and the Node registers itself.
#
# Usage (the CP serves this script and the Agent binary, by architecture):
#   curl -fsSL http://<IP_DO_CP>/node/bootstrap-node.sh | CP_URL=http://<IP_DO_CP> JOIN_TOKEN=<token> sh
#
# JOIN_TOKEN is generated in the CP:  POST /api/v1/enroll-tokens  (operator) — one-time, expires in 1h.
#
# Storage (optional): by default detects the largest USB disk and mounts the existing
# filesystem on /data (non-destructive). Overrides:
#   STORAGE_DEVICE=/dev/sda2   points to the partition exactly
#   STORAGE_FORMAT=1           formats ext4 if the partition has no filesystem (destructive)
#
# Dedicated disk, or system disk (#221): the script WRITES to /etc/seedgrid-agent.env whether the Agent
# should require /data to be a mountpoint (SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT). It is declared here,
# during install, and never inferred later: an unmounted disk looks like a local folder.
#   true    /data is a dedicated disk (mounted now, or already mounted): storage is only sane if it's a mountpoint.
#   false   /data ended up on the system disk (VM, mini PC, single-disk NAS): the Agent only checks
#           writes; free space follows the NODE_DISK_FULL alert. To add a disk later,
#           use --storage-only.
# A 2nd pass never downgrades a Node declared with a disk: with no disk NOW, but with the previous env set to
# true (or /data in fstab), the value remains true. Overrides:
#   STORAGE_REQUIRE_MOUNT=true    declares dedicated disk even without a disk now (disk arrives later):
#                                 the Node stays out of the Scheduler until it mounts, instead of writing to the system
#   STORAGE_REQUIRE_MOUNT=false   reverts a Node to the system disk, on purpose
#
# IPv6 Network (ADR-034, ADR-038): the Node needs ONE stable IPv6 address, which the operator allows
# in the firewall. The script disables privacy extensions and fixes the lower half of the address to
# ::5eed:<N> (N = Node index, returned by the Control Plane on enroll); the /64 prefix still
# comes from Router Advertisement. Overrides:
#   IPV6_SUFFIX=off            does not fix the suffix (keeps the address chosen by the network manager)
#   NODE_IFACE=eth0            interface to configure (default: the one with the default route)
#
# Onboarding saves a copy of this script at /opt/seedgrid-node/bootstrap-node.sh: that's where the
# modes below are run from later, and it's the path the script messages refer to. On a
# Node installed before this, the copy doesn't exist: download it again, with the mode after `sh -s --`
# (curl -fsSL http://<IP_DO_CP>/node/bootstrap-node.sh | sudo sh -s -- --network-only).
#
# Attach storage LATER (Node already registered) — the disk arrived late, or was replaced:
#   sh /opt/seedgrid-node/bootstrap-node.sh --storage-only
# Mounts the disk, installs hotplug auto-recovery, makes the Engine see the new /data and
# writes SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true (the Node now has a dedicated disk, and the Agent
# requires it). Idempotent: running again with the disk already mounted does nothing wrong.
#
# Apply only the network on an existing Node (privacy extensions + IPv6 suffix):
#   sh /opt/seedgrid-node/bootstrap-node.sh --network-only
# The index comes from the Node's own BitTorrent port (/etc/seedgrid-agent.env); IPV6_NODE_INDEX=<N> sets it
# manually. Idempotent: with the address already up it changes nothing. After a new address,
# restart the Engine (it needs to reopen the socket) and re-announce to the trackers.
set -eu

MODE=full
for arg in "$@"; do
  case "$arg" in
    --storage-only) MODE=storage ;;
    --network-only) MODE=network ;;
    -h|--help)
      sed -n '2,51p' "$0" | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *) echo "!! unknown argument: $arg (use --storage-only, --network-only or --help)"; exit 2 ;;
  esac
done

# require_root MODE: everything this script does (sysctl, network, /etc, /opt, Docker, systemd)
# requires root. Without the check it would continue as a regular user and fail piecemeal, with
# "Permission denied", after already having talked to the Control Plane (#21). It tells the right
# command for the requested mode, because what the Portal shows doesn't include sudo. It is called after
# validating the arguments (usage error exits 2, with or without root) and before any side effects.
require_root() {
  [ "$(id -u)" -eq 0 ] && return 0
  echo "!! this script must run as root (you are $(id -un)). Nothing was changed." >&2
  case "$1" in
    storage) echo "   Use: sudo sh /opt/seedgrid-node/bootstrap-node.sh --storage-only" >&2 ;;
    network) echo "   Use: sudo sh /opt/seedgrid-node/bootstrap-node.sh --network-only" >&2 ;;
    *) echo "   Use: curl -fsSL ${CP_URL:-http://<control-plane>}/node/bootstrap-node.sh | sudo CP_URL=${CP_URL:-http://<control-plane>} JOIN_TOKEN=<the same token> sh" >&2 ;;
  esac
  return 1
}

# STORAGE_REQUIRE_MOUNT is an operator declaration: only true or false. A typo cannot
# silently turn into the value that was NOT requested — and --storage-only attaches a disk, which
# doesn't mix with declaring there isn't one.
case "${STORAGE_REQUIRE_MOUNT:-}" in
  ''|true|false) ;;
  *) echo "!! STORAGE_REQUIRE_MOUNT must be true or false (got '$STORAGE_REQUIRE_MOUNT')"; exit 2 ;;
esac
if [ "$MODE" = storage ] && [ "${STORAGE_REQUIRE_MOUNT:-}" = false ]; then
  echo "!! --storage-only attaches a dedicated disk: incompatible with STORAGE_REQUIRE_MOUNT=false"
  exit 2
fi

INSTALL_DIR=/opt/seedgrid-node
AGENT_ENV=/etc/seedgrid-agent.env
AGENT_UNIT=/etc/systemd/system/seedgrid-agent.service
# The fstab where setup_storage writes the /data mount. Variable only so the test can
# point it to a disposable file (like NETWORKD_DIR).
FSTAB=/etc/fstab
# The Agent env key that says whether /data must be a dedicated disk (#221).
STORAGE_MOUNT_KEY=SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT
QBT_API=http://127.0.0.1:8080
# Torrent Engine container name: the `container_name` from the compose served at /node/ and
# the Agent's default (SEEDGRID_ENGINE_CONTAINER). All three need to be the same name — the
# Agent uses it to report the Engine uptime, and a mismatched name leaves
# `engine_started_at` NULL without any error (#146). An Agent test checks the artifacts.
ENGINE_CONTAINER=seedgrid-engine
# SeedGrid sysctl for IPv6 (ADR-034). Dedicated file, read after the 10-ipv6-privacy.conf
# that Ubuntu ships with — lexical order is what makes our value take effect.
SYSCTL_CONF=/etc/sysctl.d/99-seedgrid-ipv6.conf

# In --storage-only the Node is already registered: there is no enroll or token, and the CP comes from the
# Agent env — so the operator doesn't need to remember anything to attach a disk.
if [ "$MODE" = storage ]; then
  CP_URL="${CP_URL:-$(sed -n 's/^SEEDGRID_CONTROL_PLANE_URL=//p' "$AGENT_ENV" 2>/dev/null | head -1)}"
  : "${CP_URL:?SEEDGRID_CONTROL_PLANE_URL not found in $AGENT_ENV — pass CP_URL=http://<cp-ip>}"
  # The container name for this Node comes from the Agent env: a Node prior to #146 still has the
  # container with the old name, and that's the one that needs to be inspected/recreated.
  ENGINE_CONTAINER=$(sed -n 's/^SEEDGRID_ENGINE_CONTAINER=//p' "$AGENT_ENV" 2>/dev/null | head -1)
  ENGINE_CONTAINER="${ENGINE_CONTAINER:-seedgrid-engine}"
elif [ "$MODE" = network ]; then
  : # only this Node's network: there is no enroll, token or Control Plane to contact.
else
  : "${CP_URL:?set CP_URL=http://<cp-ip>}"   # port 80 (the Portal nginx); the backend 8080 is local to the CP
  : "${JOIN_TOKEN:?set JOIN_TOKEN=<join token issued by the Control Plane>}"
fi
require_root "$MODE" || exit 1

# Base of the artifacts served by the Portal (nginx on port 80 of the CP).
ARTIFACTS="${ARTIFACTS:-$(printf '%s' "${CP_URL:-}" | sed -E 's#:[0-9]+$##')}"

# ---------------------------------------------------------------------------
# Storage — in functions because the disk does not always arrive together with the Node.
#
# The original bootstrap decided the storage ONCE and moved on: if the disk was not
# present at that moment, the Node was born with /data on the card and there was no way
# back — nor was the hotplug auto-recovery installed, precisely on the Node that needs
# it the most. These functions are the same step, reinvokable via --storage-only.
# ---------------------------------------------------------------------------

# save_self keeps a copy of this script in $INSTALL_DIR. The onboarding runs via `curl | sh`,
# so the script doesn't stay on disk — and its messages (and the docs) tell you to run it again with
# --storage-only / --network-only. Without the copy, the instruction pointed to a file that did
# not exist on the Node (#26). It downloads to a temporary file and only replaces it if the download succeeded.
save_self() {
  mkdir -p "$INSTALL_DIR"
  if curl -fsSL "$ARTIFACTS/node/bootstrap-node.sh" -o "$INSTALL_DIR/bootstrap-node.sh.tmp"; then
    chmod 755 "$INSTALL_DIR/bootstrap-node.sh.tmp"
    mv "$INSTALL_DIR/bootstrap-node.sh.tmp" "$INSTALL_DIR/bootstrap-node.sh"
  else
    rm -f "$INSTALL_DIR/bootstrap-node.sh.tmp"
    return 1
  fi
}

# Downloads any missing storage artifacts to $INSTALL_DIR. On an older Node, whose
# bootstrap ran without a disk, they weren't even downloaded in the first place.
fetch_storage_artifacts() {
  mkdir -p "$INSTALL_DIR"
  # Pre-#26 Node: takes the opportunity to leave a copy of the script (best effort).
  [ -f "$INSTALL_DIR/bootstrap-node.sh" ] || save_self || true
  for f in seedgrid-storage-reattach.sh seedgrid-storage-reattach.service 99-seedgrid-storage.rules; do
    [ -f "$INSTALL_DIR/$f" ] && continue
    curl -fsSL "$ARTIFACTS/node/$f" -o "$INSTALL_DIR/$f"
  done
}

# setup_storage mounts the data disk at /data (the Torrent Engine writes here).
# Non-destructive by default (mounts the existing filesystem); formats ext4 only with
# STORAGE_FORMAT=1. Idempotent: if /data is already mounted, it doesn't touch anything.
# Exits 0 when /data is a real disk; 1 when it falls back to the card/rootfs. It is this exit code — and
# only it — that decides what gets written to SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT (storage_require_mount).
setup_storage() {
  echo "==> configuring storage (/data)"
  if mountpoint -q /data 2>/dev/null; then
    echo "   /data is already mounted — keeping it ($(df -h /data 2>/dev/null | awk 'NR==2{print $2" total, "$4" free"}'))"
    return 0
  fi
  DATA_DEV="${STORAGE_DEVICE:-}"
  if [ -z "$DATA_DEV" ]; then
    BOOTDISK=$(lsblk -no pkname "$(findmnt -no SOURCE / 2>/dev/null)" 2>/dev/null | head -1)
    BEST=0
    for part in $(lsblk -rpno NAME,TYPE 2>/dev/null | awk '$2=="part"{print $1}'); do
      pk=$(lsblk -no pkname "$part" 2>/dev/null | head -1)
      [ -n "$pk" ] && [ "$pk" = "$BOOTDISK" ] && continue
      [ "$(lsblk -dno tran "/dev/$pk" 2>/dev/null | head -1)" = "usb" ] || continue
      sz=$(lsblk -bdno size "$part" 2>/dev/null | head -1)
      [ -n "$sz" ] && [ "$sz" -gt "$BEST" ] && { BEST=$sz; DATA_DEV=$part; }
    done
  fi
  if [ -z "$DATA_DEV" ]; then
    echo "   no USB disk detected — /data will be on the card (SD/eMMC)."
    mkdir -p /data
    return 1
  fi
  FST=$(lsblk -no fstype "$DATA_DEV" 2>/dev/null | head -1)
  if [ -z "$FST" ]; then
    if [ "${STORAGE_FORMAT:-0}" = "1" ]; then
      echo "   $DATA_DEV has no filesystem — formatting as ext4"
      mkfs.ext4 -F -L seedgrid-data "$DATA_DEV" >/dev/null 2>&1; FST=ext4
    else
      echo "   !! $DATA_DEV has no filesystem — pass STORAGE_FORMAT=1 to format. Using the card."
      mkdir -p /data
      return 1
    fi
  fi
  # nofail + device-timeout: if the HDD is not present at boot, the system
  # boots up anyway (it doesn't hang waiting for the disk).
  MT="$FST"; OPTS="defaults,noatime,nofail,x-systemd.device-timeout=10s"
  [ "$FST" = "ntfs" ] && { MT="ntfs3"; OPTS="rw,uid=0,gid=0,umask=022,nofail,x-systemd.device-timeout=10s"; }
  UUID=$(blkid -s UUID -o value "$DATA_DEV")
  mkdir -p /data
  grep -q "$UUID" "$FSTAB" 2>/dev/null || printf 'UUID=%s /data %s %s 0 2\n' "$UUID" "$MT" "$OPTS" >> "$FSTAB"
  systemctl daemon-reload 2>/dev/null || true
  if ! { mount /data 2>/dev/null || mount -t "$MT" -o "$OPTS" "$DATA_DEV" /data; }; then
    # There is a dedicated disk, but it didn't mount: it remains a Node WITH a declared disk
    # (return 0). The Agent will report storage_ok=false until it mounts — the safe side; falling back to the
    # system disk here is exactly what the mountpoint requirement exists to prevent.
    echo "   !! failed to mount $DATA_DEV on /data — the Node reports storage_ok=false until the disk is mounted."
    return 0
  fi
  echo "   $DATA_DEV ($FST) mounted on /data — $(df -h /data 2>/dev/null | awk 'NR==2{print $2" total, "$4" free"}')"
  return 0
}

# install_storage_recovery installs the hotplug auto-recovery for /data: if the disk drops
# and comes back (re-enumerating as another /dev), a udev rule remounts /data and restarts the
# Torrent Engine to reopen the files and recheck torrents in error. It only makes sense if
# /data is a real disk (with a filesystem UUID).
install_storage_recovery() {
  DATA_UUID=$(findmnt -no UUID /data 2>/dev/null || true)
  if [ -z "$DATA_UUID" ]; then
    echo "   /data has no UUID (card/no dedicated disk) — hotplug auto-recovery not installed"
    return 0
  fi
  fetch_storage_artifacts
  install -m 0755 "$INSTALL_DIR/seedgrid-storage-reattach.sh" /usr/local/bin/seedgrid-storage-reattach.sh
  install -m 0644 "$INSTALL_DIR/seedgrid-storage-reattach.service" /etc/systemd/system/seedgrid-storage-reattach.service
  sed "s/__DATA_UUID__/$DATA_UUID/" "$INSTALL_DIR/99-seedgrid-storage.rules" > /etc/udev/rules.d/99-seedgrid-storage.rules
  systemctl daemon-reload 2>/dev/null || true
  udevadm control --reload-rules 2>/dev/null || true
  echo "==> /data hotplug auto-recovery installed (udev, UUID=$DATA_UUID)"
}

# data_propagation returns the propagation (rslave|rprivate) with which compose must do the
# bind of /data in the Engine container (#143). rslave is what makes a disk mounted on the host
# AFTER the Engine starts appear inside the container. But Docker REFUSES to create a
# container with an rslave bind when the mount containing the directory is `private` ("is mounted
# on / but it is not a shared or slave mount") — it's not a no-op, it's a failure, and the Engine won't start.
# On a systemd host the root is already shared; only an init that doesn't make this adjustment falls back to
# the rprivate fallback (the previous behavior). Without findmnt there's no way to tell either, so
# it stays on the fallback: erring on the old side never crashes the Engine.
data_propagation() {
  case "$(findmnt -no PROPAGATION -T "${1:-/data}" 2>/dev/null | head -1)" in
    *shared*|*slave*) echo rslave ;;
    *) echo rprivate ;;
  esac
}

# set_env_kv KEY VALUE FILE writes KEY=VALUE to FILE (creates the file; replaces the line if it already exists).
set_env_kv() {
  touch "$3"
  if grep -q "^$1=" "$3"; then
    sed -i "s|^$1=.*|$1=$2|" "$3"
  else
    printf '%s=%s\n' "$1" "$2" >> "$3"
  fi
}

# refresh_engine_mount makes the Engine see a /data mounted AFTER the container started.
# With the current compose (rslave bind, #143) the new mount appears on its own; this step remains
# here for two reasons: the Node whose container was spawned from an old compose (rprivate bind:
# the mount done on the host DOES NOT propagate to the container, which would keep writing to the old
# directory, the system disk) and the Engine with files still open on the disk that was removed. In
# either case, recreating the container is what reconnects it to the host's current mount.
refresh_engine_mount() {
  command -v docker >/dev/null 2>&1 || return 0
  [ -f "$INSTALL_DIR/docker-compose.yml" ] || return 0
  set_env_kv SEEDGRID_DATA_PROPAGATION "$(data_propagation /data)" "$INSTALL_DIR/.env"
  echo "==> recreating the Torrent Engine to see the new /data"
  ( cd "$INSTALL_DIR" && docker compose up -d --force-recreate qbittorrent )
  prop=$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Propagation}}{{end}}{{end}}' "$ENGINE_CONTAINER" 2>/dev/null || true)
  case "$prop" in
    rslave|rshared) ;;
    *) echo "   warning: this Node's /data bind uses '${prop:-?}' propagation: a disk mounted later will only"
       echo "   reach the Engine by recreating it. Update this Node's docker-compose.yml (docs/19 - Deployment.md §50)." ;;
  esac
  # The Agent's ConfigureEngine only runs at STARTUP and needs the Engine to be up, otherwise it logs
  # "connection refused" and continues without reapplying the save_path. It waits for the WebUI to respond.
  i=0
  while [ "$i" -lt 30 ]; do
    curl -fsS -o /dev/null "$QBT_API/" 2>/dev/null && break
    i=$((i + 1)); sleep 2
  done
  if [ -f "$AGENT_UNIT" ]; then
    systemctl restart seedgrid-agent
    AGENT_RESTARTED=1
    echo "   Agent restarted — reapplies save_path=/data to Engine"
  fi
}

# ---------------------------------------------------------------------------
# Dedicated disk, or system disk (#221).
#
# Requiring /data to be a mountpoint only protects those who HAVE a dedicated disk: a disk that didn't
# mount leaves /data as an empty folder on the system disk, and the Engine would start writing there without
# anyone noticing. On the other hand, a single-disk Node (VM, mini PC, NAS) will never have a mountpoint, and the
# same requirement would keep it out of the cluster forever. Looking only at the CURRENT /data the two
# cases are indistinguishable — that's why the difference is DECLARED here, at installation, in
# SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT, and the Agent never infers it at runtime.
# ---------------------------------------------------------------------------

# fstab_declares_data: does fstab mount something at /data? It's the mark of a machine that once had a dedicated
# disk (setup_storage writes it), even if the disk is not present at this moment.
fstab_declares_data() {
  grep -Eq '^[[:space:]]*[^#[:space:]][^[:space:]]*[[:space:]]+/data([[:space:]]|$)' "$FSTAB" 2>/dev/null
}

# storage_require_mount DISK prints the value (true|false) of SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT.
# DISK is 1 when setup_storage found/mounted a dedicated disk at /data, 0 when /data ended up
# on the system disk. The decision, in order:
#   1. STORAGE_REQUIRE_MOUNT, if the operator passed it: the explicit declaration overrides everything;
#   2. disk found/mounted now: true;
#   3. no disk now: false ONLY if the machine never declared a dedicated disk. With the previous env set to
#      true, or /data in fstab, the disk that disappeared is a MISSING disk, not "single disk" — and a 2nd
#      bootstrap pass cannot downgrade the Node to the system disk because of this.
storage_require_mount() {
  case "${STORAGE_REQUIRE_MOUNT:-}" in
    true|false) echo "$STORAGE_REQUIRE_MOUNT"; return 0 ;;
  esac
  if [ "$1" = 1 ]; then
    echo true
    return 0
  fi
  prev=$(sed -n "s/^$STORAGE_MOUNT_KEY=//p" "$AGENT_ENV" 2>/dev/null | head -1)
  if [ "$prev" = true ] || fstab_declares_data; then echo true; else echo false; fi
}

# announce_storage_mode DISK VALUE tells the operator, in the terminal, what was declared — and,
# in the case of a single disk, what that means and how to attach a disk later.
announce_storage_mode() {
  if [ "$2" = true ] && [ "$1" = 1 ]; then
    echo "   /data is a dedicated disk — the Agent will require it to be a mountpoint ($STORAGE_MOUNT_KEY=true)."
  elif [ "$2" = true ]; then
    echo "   !! This Node is declared with a dedicated disk at /data ($STORAGE_MOUNT_KEY=true), but there is no"
    echo "      disk present right now. It stays out of the Scheduler (storage_ok=false) until the disk is mounted — this is the"
    echo "      protection against writing to the system disk. Connect the disk and run:"
    echo "        sh /opt/seedgrid-node/bootstrap-node.sh --storage-only"
    echo "      To use the system disk anyway: STORAGE_REQUIRE_MOUNT=false."
  elif [ "$1" = 1 ]; then
    echo "   !! STORAGE_REQUIRE_MOUNT=false: the Agent will NOT require a mountpoint, even though /data is on a disk."
    echo "      If this disk fails, the Engine will start writing to the system disk without an alarm."
  else
    echo "   !! /data is on the SYSTEM DISK: this Node will store torrents on it, without a dedicated disk."
    echo "      The Agent will not require a mountpoint ($STORAGE_MOUNT_KEY=false in $AGENT_ENV) and checks only"
    echo "      write access; free space is still monitored by the NODE_DISK_FULL alert."
    echo "      To add a disk later, connect it and run:  sh /opt/seedgrid-node/bootstrap-node.sh --storage-only"
    echo "      (mounts the disk, writes $STORAGE_MOUNT_KEY=true, and the Agent starts requiring it)."
    echo "      If the disk is already on the way and the Node must NOT receive torrents before it, install with"
    echo "      STORAGE_REQUIRE_MOUNT=true, or change this env line now and restart the Agent."
  fi
}

# declare_storage_dedicated writes SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true to the Agent env of an
# existing Node — the --storage-only: it got a disk. Idempotent. It doesn't create the file: with the
# key missing the Agent already requires the mountpoint, and a single-line env would hide that the Node was never
# installed.
declare_storage_dedicated() {
  if [ ! -f "$AGENT_ENV" ]; then
    echo "   warning: $AGENT_ENV does not exist — $STORAGE_MOUNT_KEY not written (if absent, the Agent already requires the mountpoint)."
    return 0
  fi
  set_env_kv "$STORAGE_MOUNT_KEY" true "$AGENT_ENV"
  echo "   $STORAGE_MOUNT_KEY=true in $AGENT_ENV — the Agent now requires /data to be a mountpoint."
}

# restart_agent restarts the Agent, which only re-reads the env on startup. Without the unit (a Node installed in
# another way) there is nothing to restart.
restart_agent() {
  [ -f "$AGENT_UNIT" ] || return 0
  systemctl restart seedgrid-agent
  AGENT_RESTARTED=1
  echo "   Agent restarted — rereads $AGENT_ENV"
}

# write_agent_env writes the env for the Agent of a new Node (the join token serves as the registration token —
# one-time). REQUIRE_MOUNT is the value of storage_require_mount, decided AFTER the storage step.
write_agent_env() {
  cat > "$AGENT_ENV" <<ENV
SEEDGRID_CONTROL_PLANE_URL=$CP_URL
SEEDGRID_AGENT_REGISTRATION_TOKEN=$JOIN_TOKEN
SEEDGRID_ENGINE_QBT_URL=$QBT_API
SEEDGRID_ENGINE_QBT_USERNAME=admin
SEEDGRID_ENGINE_QBT_PASSWORD=$QBT_PASS
SEEDGRID_ENGINE_BT_PORT=$BT_PORT
SEEDGRID_ENGINE_CONTAINER=$ENGINE_CONTAINER
SEEDGRID_AGENT_STORAGE_PATH=/data
SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=$REQUIRE_MOUNT
ENV
  chmod 0600 "$AGENT_ENV"
}

# ---------------------------------------------------------------------------
# Network (IPv6) — the Node needs to announce to the tracker, and be reached at, ONE stable IPv6
# address: this is the address the operator opens on their firewall (ADR-034).
# ---------------------------------------------------------------------------

# primary_iface returns the interface of the default route — IPv6 first, IPv4 as a fallback
# (IPv6 might not have arrived yet at bootstrap time) —, or NODE_IFACE if the
# operator fixed it. Fails (return 1) if it doesn't find any.
primary_iface() {
  if [ -n "${NODE_IFACE:-}" ]; then
    echo "$NODE_IFACE"
    return 0
  fi
  for fam in -6 -4; do
    dev=$(ip "$fam" route show default 2>/dev/null | awk '{ for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit } }')
    if [ -n "$dev" ]; then
      echo "$dev"
      return 0
    fi
  done
  return 1
}

# sysctl_iface_key IFACE FIELD returns the sysctl key per interface. The dot of a
# VLAN interface (eth0.100) would become a separator, so it uses the slash form.
sysctl_iface_key() {
  case "$1" in
    *.*) printf 'net/ipv6/conf/%s/%s' "$1" "$2" ;;
    *)   printf 'net.ipv6.conf.%s.%s' "$1" "$2" ;;
  esac
}

# set_sysctl_kv KEY VALUE FILE writes `KEY = VALUE` to the sysctl.d file, replacing the
# existing line (and discarding duplicates) and preserving all others — the file
# may have lines from another origin, like the accept_ra from a previous manual configuration.
set_sysctl_kv() {
  touch "$3"
  awk -v k="$1" -v v="$2" '
    { l = $0; sub(/^[ \t]+/, "", l); split(l, a, /[ \t]*=/) }
    l !~ /^[#;]/ && a[1] == k { if (!done) { print k " = " v; done = 1 }; next }
    { print }
    END { if (!done) print k " = " v }
  ' "$3" > "$3.tmp.$$" && cat "$3.tmp.$$" > "$3"
  rm -f "$3.tmp.$$"
}

# disable_ipv6_privacy IFACE disables the privacy extensions (RFC 4941) and removes any
# existing temporary addresses (#142). With them enabled the Node gets, in addition to the stable address,
# a temporary one that rotates automatically — and this is what outbound traffic (and the announce to the
# tracker) uses, not the stable one the operator opened on the firewall. Idempotent.
#
# `all` and `default` apply to interfaces created after boot (a WiFi that comes up late, a
# VLAN); the one for the interface itself applies immediately. Only these keys are applied at runtime
# (sysctl -w), never the whole file: it may have accept_ra lines that shouldn't be
# reapplied over a networkd that already handles the RA.
disable_ipv6_privacy() {
  iface=$1
  mkdir -p "$(dirname "$SYSCTL_CONF")"   # a minimal host may not have /etc/sysctl.d
  [ -s "$SYSCTL_CONF" ] || printf '%s\n' \
    '# SeedGrid: stable IPv6 address (ADR-034). With privacy extensions, outgoing traffic' \
    '# uses a temporary address that rotates, and the tracker starts seeing an IP different from the' \
    '# one allowed in the firewall.' > "$SYSCTL_CONF"
  for key in net.ipv6.conf.all.use_tempaddr net.ipv6.conf.default.use_tempaddr "$(sysctl_iface_key "$iface" use_tempaddr)"; do
    set_sysctl_kv "$key" 0 "$SYSCTL_CONF"
    sysctl -qw "$key=0" 2>/dev/null || echo "   warning: could not apply $key now (the $SYSCTL_CONF file takes effect on next boot)"
  done
  ip -6 -o addr show dev "$iface" scope global 2>/dev/null | awk '/temporary/ { print $4 }' | while read -r tmp; do
    if ip -6 addr del "$tmp" dev "$iface" 2>/dev/null; then
      echo "   temporary IPv6 address removed: $tmp"
    fi
  done
  return 0
}

# ---------------------------------------------------------------------------
# Deterministic IPv6 suffix (ADR-038, #202).
#
# The default SLAAC address is EUI-64, derived from the MAC: recreating the Node, swapping the NIC or migrating
# hosts changes it, and the operator's firewall rule ends up pointing to nowhere. The Node
# pins the lower half of the address to `::5eed:<N>` (interface token) and lets the prefix
# /64 come from the Router Advertisement — a provider prefix change is still
# tracked. N is the index the Control Plane returns on enroll (`node_index`).
# ---------------------------------------------------------------------------

# Where each network manager stores the configuration. Variables (instead of literals in the functions)
# just so the test can point them to a disposable directory.
NETWORKD_DIR=/etc/systemd/network
NM_DIR=/etc/NetworkManager/system-connections
BACKUP_DIR=/var/backups/seedgrid-node
# Start of the Control Plane's BitTorrent port range (btPortBase in
# control-plane/internal/node/node.go). Only --network-only uses this mirror, because it doesn't do
# enroll; in normal onboarding the index comes ready in the CP response.
BT_PORT_BASE=6881
BT_PORT_SLOTS=1000

# index_for_port PORT prints the Node index (port 6881 → 1), mirroring node.IndexForPort
# from the Control Plane. Exits 1 for a port outside the range.
index_for_port() {
  case "$1" in
    ''|*[!0-9]*|0*) return 1 ;;
  esac
  [ "$1" -ge "$BT_PORT_BASE" ] && [ "$1" -lt $((BT_PORT_BASE + BT_PORT_SLOTS)) ] || return 1
  echo $(($1 - BT_PORT_BASE + 1))
}

# ipv6_expand ADDRESS prints the 8 16-bit groups with 4 hexadecimal digits each,
# separated by ':' — the form that can be compared as text. Accepts `::`, leading
# zeros omitted, uppercase, /len suffix and the dotted tail that NetworkManager writes when the top 96
# bits are zero: `::94.237.0.4` is the SAME address as `::5eed:4` (0x5eed = 94.237). A
# comparator that only recognizes one of the forms takes the already applied token as not applied.
# Exits 1 if it's not a valid IPv6.
ipv6_expand() {
  printf '%s\n' "$1" | awk '
    function pad(h) {
      if (length(h) > 4 || h !~ /^[0-9a-f]+$/) bad = 1
      return substr("0000" h, length(h) + 1)
    }
    {
      a = tolower($0); sub(/\/.*$/, "", a)
      if (match(a, /[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$/)) {
        split(substr(a, RSTART), q, ".")
        a = substr(a, 1, RSTART - 1) sprintf("%x:%x", q[1] * 256 + q[2], q[3] * 256 + q[4])
      }
      n = index(a, "::")
      if (n) { l = substr(a, 1, n - 1); r = substr(a, n + 2) } else { l = a; r = "" }
      nl = (l == "") ? 0 : split(l, L, ":")
      nr = (r == "") ? 0 : split(r, R, ":")
      if ((n && nl + nr > 7) || (!n && nl != 8)) exit 1
      out = ""
      for (i = 1; i <= nl; i++) out = out (out == "" ? "" : ":") pad(L[i])
      if (n) for (i = 0; i < 8 - nl - nr; i++) out = out (out == "" ? "" : ":") "0000"
      for (i = 1; i <= nr; i++) out = out (out == "" ? "" : ":") pad(R[i])
      if (bad) exit 1
      print out
    }'
}

# ipv6_tail N prints the last 4 groups (64 bits) of Node N's address, already in the
# ipv6_expand format. N goes in DECIMAL digits in the last group (Node 12 → 0012, that is, `::5eed:12`):
# the operator reads the Node number straight from the address, without converting to hexadecimal.
ipv6_tail() {
  printf '0000:0000:5eed:%04d' "$1"
}

# ipv6_global_addrs IFACE lists the already usable global addresses (neither in DAD, nor with failed
# DAD) as `address/len dynamic|static`, one per line.
ipv6_global_addrs() {
  ip -6 -o addr show dev "$1" scope global 2>/dev/null \
    | awk '!/tentative|dadfailed/ { print $4, ($0 ~ /dynamic/ ? "dynamic" : "static") }'
}

# ipv6_has_token_addr IFACE N: does the interface already have Node N's address? It's the idempotency
# test — and the only one that matters: what matters is the address existing, not the configuration
# file having the right text (which the manager might have written in another form).
ipv6_has_token_addr() {
  # Collects everything before filtering: a `grep -q` (or `break`) at the end of a pipeline closes the pipe
  # with awk still writing, and awk complains about "Broken pipe" on bootstrap output.
  all=$(ipv6_global_addrs "$1" | while read -r a _; do ipv6_expand "$a"; done)
  printf '%s\n' "$all" | grep -q ":$(ipv6_tail "$2")\$"
}

# ipv6_token_addr IFACE N prints the address (without /len) of Node N on the interface, if any.
ipv6_token_addr() {
  want=$(ipv6_tail "$2")
  ipv6_global_addrs "$1" | while read -r a _; do
    case "$(ipv6_expand "$a")" in
      *:"$want") echo "${a%%/*}" ;;
    esac
  done | head -1
}

# ipv6_token_valid_lft IFACE N prints the valid_lft (seconds, or `forever`) of Node N's
# address. It's the number that proves the address RENEWS: after a Router Advertisement it
# goes back up. An address that exists but doesn't renew disappears on its own when the lifetime ends.
ipv6_token_valid_lft() {
  addr=$(ipv6_token_addr "$1" "$2")
  [ -n "$addr" ] || return 1
  ip -6 -o addr show dev "$1" scope global 2>/dev/null \
    | awk -v a="$addr/" 'index($4, a) == 1 { for (i = 1; i < NF; i++) if ($i == "valid_lft") { v = $(i + 1); sub(/sec$/, "", v); print v; exit } }'
}

# wait_ipv6_token_addr IFACE N [SECONDS] waits for Node N's address to appear (the Router
# Advertisement takes a few seconds to arrive after the configuration is reapplied).
wait_ipv6_token_addr() {
  i=0
  while [ "$i" -lt "${3:-60}" ]; do
    ipv6_has_token_addr "$1" "$2" && return 0
    i=$((i + 2))
    sleep 2
  done
  return 1
}

# ipv6_dad_conflict N: did the kernel reject Node N's address due to duplication (DAD)? Prints the
# MAC of whoever is already using it and returns 0; with no recorded rejection, returns 1.
#
# Only the kernel log knows: the network manager removes the rejected address from the interface, so
# `ip addr` only shows that it doesn't exist — the same it would show if the Router Advertisement
# hadn't arrived. These are different causes, with different solutions (#21).
ipv6_dad_conflict() {
  { dmesg 2>/dev/null || journalctl -k --no-pager 2>/dev/null || true; } \
    | sed -n "s/.*duplicate address [0-9a-fA-F:]*:5eed:$1 used by \\([0-9a-fA-F:]*\\) detected.*/\\1/p" \
    | tail -n 1 | grep .
}

# prune_stale_ipv6 IFACE N: with the token address already up, deletes what is left from the previous
# scheme on the same /64 — the EUI-64 (leftover from when the manager reapplies), the
# stable privacy address and the temporary ones. Only deletes DYNAMIC addresses and only after the
# token one exists: never leaves the Node without an address.
#
# Deleting the leftover is the right fix; disabling autoconf (`autoconf=0`) is the wrong one: the
# kernel/manager generates the token address from the Router Advertisement, and without
# autoconf the address stays up with its previous lifetime and EXPIRES in about two hours.
prune_stale_ipv6() {
  iface=$1
  want=$(ipv6_tail "$2")
  addrs=$(ipv6_global_addrs "$iface")
  prefixes=""
  while read -r a _; do
    [ -n "$a" ] || continue
    exp=$(ipv6_expand "$a") || continue
    case "$exp" in
      *:"$want") prefixes="$prefixes $(printf '%s' "$exp" | cut -d: -f1-4);" ;;
    esac
  done <<EOF
$addrs
EOF
  [ -n "$prefixes" ] || return 0
  while read -r a kind; do
    [ "$kind" = dynamic ] || continue
    exp=$(ipv6_expand "$a") || continue
    case "$exp" in *:"$want") continue ;; esac
    case "$prefixes" in
      *" $(printf '%s' "$exp" | cut -d: -f1-4);"*)
        if ip -6 addr del "$a" dev "$iface" 2>/dev/null; then
          echo "   previous scheme IPv6 address removed: $a"
        fi ;;
    esac
  done <<EOF
$addrs
EOF
  return 0
}

# networkd_manages IFACE: does systemd-networkd manage the interface? (`networkctl` shows
# `(configured)`; an interface outside it appears as `(unmanaged)`.)
networkd_manages() {
  command -v networkctl >/dev/null 2>&1 || return 1
  networkctl status "$1" 2>/dev/null | grep -Eq 'State:.*\(configured\)'
}

# apply_token_networkd IFACE N: systemd-networkd drop-in with the IPv6Token. Goes in a drop-in
# (not the base file) because the hypervisor/installer may regenerate the base file on every
# boot; the drop-in survives. Reuses the same file from the SLAAC step (ADR-034).
apply_token_networkd() {
  netfile=$(networkctl status "$1" 2>/dev/null | sed -n 's/^[[:space:]]*Network File:[[:space:]]*//p' | head -1)
  case "$netfile" in
    /*) ;;
    *) echo "   networkd did not report the .network file for $1 — token not configured."; return 1 ;;
  esac
  dropin="$NETWORKD_DIR/$(basename "$netfile").d"
  mkdir -p "$dropin"
  cat > "$dropin/99-seedgrid-ipv6-ra.conf" <<EOF
[Network]
IPv6AcceptRA=true
# SeedGrid (ADR-038): deterministic IPv6 suffix per Node. The /64 prefix still comes from the
# Router Advertisement, so a provider prefix change is followed; only the lower 64 bits
# are fixed.
IPv6Token=static:::5eed:$2
EOF
  echo "   drop-in $dropin/99-seedgrid-ipv6-ra.conf (IPv6Token=static:::5eed:$2)"
  networkctl reload 2>/dev/null || true
  # reconfigure (systemd >= 243) redoes only this link; without it, it restarts the service.
  networkctl reconfigure "$1" 2>/dev/null || systemctl restart systemd-networkd 2>/dev/null || true
  return 0
}

# nm_manages IFACE: does NetworkManager manage and have an active connection on the interface?
nm_manages() {
  command -v nmcli >/dev/null 2>&1 || return 1
  case "$(nmcli -g GENERAL.STATE device show "$1" 2>/dev/null | head -1)" in
    100*) return 0 ;;
  esac
  return 1
}

# nm_write_ipv6_token FILE TOKEN edits the NetworkManager keyfile: in the [ipv6] section,
# `addr-gen-mode=eui64` and `token=TOKEN` (both, always together), preserving everything else —
# a WiFi connection keyfile has the password. Creates the section if it doesn't exist.
nm_write_ipv6_token() {
  awk -v tok="$2" '
    function flush() {
      if (!mode) print "addr-gen-mode=eui64"
      if (!tk) print "token=" tok
    }
    /^\[/ {
      if (insec) flush()
      insec = ($0 == "[ipv6]"); if (insec) seen = 1
      print; next
    }
    insec && /^addr-gen-mode[ \t]*=/ { if (!mode) { print "addr-gen-mode=eui64"; mode = 1 }; next }
    insec && /^token[ \t]*=/ { if (!tk) { print "token=" tok; tk = 1 }; next }
    { print }
    END {
      if (insec) flush()
      if (!seen) { print ""; print "[ipv6]"; print "method=auto"; print "addr-gen-mode=eui64"; print "token=" tok }
    }
  ' "$1" > "$1.seedgrid.$$" && cat "$1.seedgrid.$$" > "$1"
  rm -f "$1.seedgrid.$$"
}

# nm_loaded_token_ok UUID N: did NetworkManager load the desired token? Reads what it
# HAS in memory (not the file): the same config can come back as `::5eed:4` or as
# `::94.237.0.4`, and `nmcli -g` escapes ':' with a backslash — that's why it goes through ipv6_expand.
nm_loaded_token_ok() {
  loaded=$(nmcli -g ipv6.addr-gen-mode,ipv6.token connection show "$1" 2>/dev/null | tr -d '\134')  # \134 = the backslash
  [ "$(printf '%s\n' "$loaded" | sed -n 1p)" = eui64 ] || return 1
  [ "$(ipv6_expand "$(printf '%s\n' "$loaded" | sed -n 2p)")" = "0000:0000:0000:0000:0000:0000:5eed:$(printf '%04d' "$2")" ]
}

# apply_token_nm IFACE N: token in the active connection's keyfile (NetworkManager).
#
# Writes the keyfile by hand because nmcli CANNOT do it: two validations exclude each other (NM requires
# addr-gen-mode=eui64 to accept `ipv6.token`; netplan, the write backend on Ubuntu,
# forbids the pair), and a failing `nmcli modify` STILL writes the keyfile, leaving disk and
# memory divergent — a reboot would apply the change on its own, without anyone editing the
# firewall rule. Hence: backup, edit, reload, check what NM loaded and, if anything
# fails, the file reverts to what it was.
apply_token_nm() {
  uuid=$(nmcli -g GENERAL.CON-UUID device show "$1" 2>/dev/null | head -1)
  [ -n "$uuid" ] || { echo "   NetworkManager did not report the active connection for $1 — token not configured."; return 1; }
  keyfile=$(grep -l "^uuid=$uuid\$" "$NM_DIR"/* 2>/dev/null | head -1)
  if [ -z "$keyfile" ] || [ ! -f "$keyfile" ]; then
    echo "   the active connection is not a keyfile in $NM_DIR (generated by netplan? it is in /run)."
    echo "      I do not modify what does not persist — configure the token at the source (netplan/YAML)."
    return 1
  fi
  method=$(awk '/^\[/ { s = ($0 == "[ipv6]") } s && /^method[ \t]*=/ { sub(/^[^=]*=[ \t]*/, ""); print; exit }' "$keyfile")
  case "${method:-auto}" in
    auto) ;;
    *) echo "   the connection has ipv6.method=$method (the token only works with auto) — I did not change it."; return 1 ;;
  esac
  ( umask 077; mkdir -p "$BACKUP_DIR" )
  backup="$BACKUP_DIR/$(basename "$keyfile").$(date +%Y%m%d%H%M%S)"
  ( umask 077; cp -p "$keyfile" "$backup" )
  nm_write_ipv6_token "$keyfile" "::5eed:$2"
  echo "   keyfile $keyfile updated (backup at $backup)"
  if nmcli connection reload && nm_loaded_token_ok "$uuid" "$2" && nmcli device reapply "$1" >/dev/null; then
    return 0
  fi
  cat "$backup" > "$keyfile"
  nmcli connection reload 2>/dev/null || true
  echo "   NetworkManager did not apply it (or loaded another token): keyfile restored from $backup."
  return 1
}

# print_manual_ipv6_instructions IFACE N: this host's network manager is not one that the script
# knows how to configure safely. Instead of guessing, it says what to do.
print_manual_ipv6_instructions() {
  echo "   I cannot safely configure this host's network manager. The Node works"
  echo "   anyway, only with the IPv6 address derived from the MAC. To get the stable address, set the half of"
  echo "   half at ::5eed:$2 (the /64 prefix still comes from the Router Advertisement):"
  echo "     systemd-networkd   no .network de $1:  [Network]  IPv6Token=static:::5eed:$2"
  echo "     NetworkManager     in the connection: addr-gen-mode=eui64 and token=::5eed:$2 (keyfile; nmcli cannot)"
  echo "   Then, check that the address's valid_lft INCREASES after a Router Advertisement"
  echo "   (docs/19 - Deployment.md §50.2). To avoid this warning: IPV6_SUFFIX=off."
}

# configure_ipv6_suffix N applies the deterministic suffix ::5eed:<N> on the default route interface.
# Never breaks the bootstrap: the Node works with whichever address the network manager chooses,
# so any failure here becomes a warning and instruction, not an error. IPV6_SUFFIX=off disables it.
configure_ipv6_suffix() {
  idx=$1
  case "${IPV6_SUFFIX:-auto}" in
    off|OFF|0|no|false)
      echo "==> IPV6_SUFFIX=off: keeping the IPv6 address the network manager chooses."
      return 0 ;;
  esac
  case "$idx" in
    ''|*[!0-9]*|0*)
      echo "   warning: no valid Node index ('$idx') — IPv6 suffix not configured (IPV6_SUFFIX=off silences it)."
      return 0 ;;
  esac
  if [ "$idx" -gt 9999 ]; then
    echo "   warning: index $idx does not fit in the last group of the address (maximum 9999) — IPv6 suffix not configured."
    return 0
  fi
  if ! iface=$(primary_iface); then
    echo "   warning: could not find the default route interface — IPv6 suffix not configured (NODE_IFACE=<interface>)."
    return 0
  fi
  echo "==> deterministic IPv6 suffix ::5eed:$idx on interface $iface (ADR-038)"
  if ipv6_has_token_addr "$iface" "$idx"; then
    echo "   the address is already up — nothing to change."
  else
    if networkd_manages "$iface"; then
      echo "   manager: systemd-networkd"
      apply_token_networkd "$iface" "$idx" || true
    elif nm_manages "$iface"; then
      echo "   manager: NetworkManager"
      apply_token_nm "$iface" "$idx" || true
    else
      print_manual_ipv6_instructions "$iface" "$idx"
      return 0
    fi
    if wait_ipv6_token_addr "$iface" "$idx" 60; then
      echo "   address up: $(ipv6_token_addr "$iface" "$idx")"
    elif IPV6_CONFLICT_MAC=$(ipv6_dad_conflict "$idx"); then
      echo "   !! the address ::5eed:$idx IS ALREADY IN USE on this network by $IPV6_CONFLICT_MAC: IPv6 duplicate detection"
      echo "      rejected it and this Node was left without it. Another Node — or another SeedGrid Cluster"
      echo "      on the same network — has the same index. Reapply with a free index:"
      echo "        IPV6_NODE_INDEX=<N> sh /opt/seedgrid-node/bootstrap-node.sh --network-only"
      return 0
    else
      echo "   warning: the address ::5eed:$idx has not appeared on $iface yet (no Router Advertisement yet?)."
      echo "          The configuration is saved and will take effect when the RA arrives; check with 'ip -6 addr show dev $iface'."
      return 0
    fi
  fi
  prune_stale_ipv6 "$iface" "$idx"
  echo "   valid_lft de $(ipv6_token_addr "$iface" "$idx"): $(ipv6_token_valid_lft "$iface" "$idx")s — check that it INCREASES after the"
  echo "   next Router Advertisement (the interval depends on the router): this is what separates an address that renews from one that expires on its own."
  return 0
}

# --storage-only: attach a disk to an existing Node. Does not touch Docker, enroll,
# Agent binary or Engine password.
if [ "$MODE" = storage ]; then
  echo "==> SeedGrid bootstrap-node --storage-only — CP=$CP_URL"
  if ! setup_storage; then
    echo "!! no disk to attach. Connect the disk and run again, or specify it with"
    echo "   STORAGE_DEVICE=/dev/sdX1 sh $0 --storage-only"
    exit 1
  fi
  declare_storage_dedicated
  install_storage_recovery
  refresh_engine_mount
  # The Agent only re-reads env on restart. refresh_engine_mount already restarts it when recreating the
  # Engine; on a Node without the bootstrap compose it exits early, and the restart must be here —
  # otherwise the true written above would only take effect on the next boot.
  [ "${AGENT_RESTARTED:-0}" = 1 ] || restart_agent
  echo "==> done. /data: $(df -h /data | awk 'NR==2{print $1" — "$2" total, "$4" free"}')"
  exit 0
fi

# --network-only: the network (privacy extensions + IPv6 suffix) of an existing Node. Does not touch
# Docker, enroll, Engine or Agent. Without enroll, the Node's index comes from its BitTorrent
# port — the same Control Plane rule (node.IndexForPort) — unless IPV6_NODE_INDEX fixes it.
if [ "$MODE" = network ]; then
  echo "==> SeedGrid bootstrap-node --network-only"
  MAIN_IF=$(primary_iface) || { echo "!! could not find the default route interface — use NODE_IFACE=<interface>"; exit 1; }
  disable_ipv6_privacy "$MAIN_IF" || echo "   warning: could not write $SYSCTL_CONF — privacy extensions NOT disabled."
  NODE_INDEX="${IPV6_NODE_INDEX:-}"
  if [ -z "$NODE_INDEX" ]; then
    NODE_PORT=$(sed -n 's/^SEEDGRID_ENGINE_BT_PORT=//p' "$AGENT_ENV" 2>/dev/null | head -1)
    NODE_INDEX=$(index_for_port "$NODE_PORT") || NODE_INDEX=""
    if [ -z "$NODE_INDEX" ]; then
      echo "   warning: this Node's BitTorrent port ('${NODE_PORT:-?}' in $AGENT_ENV) is outside the sequence of the"
      echo "          Control Plane — without an index, the IPv6 suffix is not applied. Pass IPV6_NODE_INDEX=<N> to fix it."
    fi
  fi
  configure_ipv6_suffix "$NODE_INDEX"
  echo "==> done. If the IPv6 address changed, restart the Engine (it needs to reopen the socket) and"
  echo "    reannounce to the trackers; without reannouncing, the tracker keeps the verdict gathered from the old address."
  exit 0
fi

# 1) Docker
if ! command -v docker >/dev/null 2>&1; then
  echo "==> installing Docker"
  curl -fsSL https://get.docker.com | sh
fi

# 1b) IPv6 SLAAC (ADR-034): connectability under CGNAT depends on the Node forming a
#     global IPv6 address via Router Advertisement. There are environments (e.g., system
#     containers whose network is generated by the hypervisor) that generate
#     /etc/systemd/network/<iface>.network with IPv6AcceptRA=false (even with
#     accept_ra=2 in sysctl.d, networkd reapplies false on every boot and overwrites it).
#     Fixes it with a drop-in, which survives the base file regeneration.
if command -v networkctl >/dev/null 2>&1 && [ -d /etc/systemd/network ]; then
  for f in /etc/systemd/network/*.network; do
    [ -f "$f" ] || continue
    if grep -qi '^IPv6AcceptRA[[:space:]]*=[[:space:]]*false' "$f"; then
      IFACE=$(basename "$f" .network)
      echo "==> $f forces IPv6AcceptRA=false — installing drop-in (ADR-034)"
      mkdir -p "/etc/systemd/network/$IFACE.network.d"
      cat > "/etc/systemd/network/$IFACE.network.d/99-seedgrid-ipv6-ra.conf" <<EOF
[Network]
IPv6AcceptRA=true
EOF
      systemctl restart systemd-networkd 2>/dev/null || true
    fi
  done
fi

# 1c) Privacy extensions disabled (#142): the address the Node announces and uses to exit
#     must be the same one the operator allows in the firewall. See disable_ipv6_privacy.
if MAIN_IF=$(primary_iface); then
  echo "==> disabling IPv6 privacy extensions (interface $MAIN_IF)"
  disable_ipv6_privacy "$MAIN_IF" || echo "   warning: could not write $SYSCTL_CONF — privacy extensions NOT disabled."
else
  echo "!! could not find the default route interface — IPv6 privacy extensions NOT disabled."
  echo "   Repeat with NODE_IFACE=<interface> or see docs/19 - Deployment.md §50.2."
fi

# 2) enroll → the CP assigns the BitTorrent port (lowest free in the sequence)
echo "==> enroll no Control Plane"
ENROLL_JSON=$(curl -fsS -X POST "$CP_URL/api/v1/agent/enroll" \
  -H 'Content-Type: application/json' -d "{\"token\":\"$JOIN_TOKEN\"}") || ENROLL_JSON=""
BT_PORT=$(printf '%s' "$ENROLL_JSON" | sed -n 's/.*"bt_port":[[:space:]]*\([0-9]*\).*/\1/p')
[ -n "$BT_PORT" ] || { echo "!! enroll failed (invalid/expired join token?)"; exit 1; }
echo "   BitTorrent port assigned by CP: $BT_PORT"
# Node index in the port sequence (ADR-038): the origin of the IPv6 suffix ::5eed:<N>. Comes in the
# same enroll response, so it inherits the port's uniqueness. A CP older than the field does not
# return it; in that case (and only then) it uses the same rule from the port.
NODE_INDEX=$(printf '%s' "$ENROLL_JSON" | sed -n 's/.*"node_index":[[:space:]]*\([0-9]*\).*/\1/p')
[ -n "$NODE_INDEX" ] || NODE_INDEX=$(index_for_port "$BT_PORT") || NODE_INDEX=""
[ -z "$NODE_INDEX" ] || echo "   Node index: $NODE_INDEX"

# 3) artifacts served by the CP (engine compose, systemd unit, Agent binary)
mkdir -p "$INSTALL_DIR"
for f in docker-compose.yml seedgrid-agent.service seedgrid-storage-reattach.sh seedgrid-storage-reattach.service 99-seedgrid-storage.rules; do
  curl -fsSL "$ARTIFACTS/node/$f" -o "$INSTALL_DIR/$f"
done
save_self   # the copy that --storage-only / --network-only use later

# The binary is published BY ARCHITECTURE (ADR-027). Previously there was a single /node/seedgrid-agent,
# cross-compiled ARM64 for the RPi — on an amd64 Node it downloaded but wouldn't run.
# Resolving the arch here is what makes onboarding correct in both worlds.
case "$(uname -m)" in
  x86_64|amd64)  NODE_ARCH=amd64 ;;
  aarch64|arm64) NODE_ARCH=arm64 ;;
  *) echo "!! unsupported architecture: $(uname -m) (the CP publishes amd64 and arm64)"; exit 1 ;;
esac
echo "==> downloading Agent for $NODE_ARCH"
curl -fsSL "$ARTIFACTS/node/seedgrid-agent-$NODE_ARCH" -o /usr/local/bin/seedgrid-agent
chmod 0755 /usr/local/bin/seedgrid-agent
# Proves the binary runs HERE before installing the service — same smoke-test that the
# self-update uses (ADR-027); failing early with a clear message is better than a systemd in
# a restart loop.
/usr/local/bin/seedgrid-agent --version >/dev/null 2>&1 || {
  echo "!! the downloaded binary does not run on this host (wrong architecture?)"; exit 1; }

# 4) storage: mounts the data disk at /data and installs hotplug auto-recovery.
#    Both live in functions (top of the script) because the disk doesn't always arrive with the
#    Node — `--storage-only` re-executes exactly these two steps later.
#    Without a disk the Node still comes up: /data stays on the system disk. What the script did here —
#    found/mounted a dedicated disk (1) or not (0) — becomes the declaration the Agent reads (#221).
if setup_storage; then DATA_DEDICATED=1; else DATA_DEDICATED=0; fi
REQUIRE_MOUNT=$(storage_require_mount "$DATA_DEDICATED")
announce_storage_mode "$DATA_DEDICATED" "$REQUIRE_MOUNT"
install_storage_recovery

# 4b) deterministic IPv6 suffix (ADR-038). Before bringing up the Engine, so it announces to the
#     tracker the final address from the very first announce — and doesn't need to re-announce.
configure_ipv6_suffix "$NODE_INDEX"

# 5) qBittorrent WebUI password — generated locally (never leaves the Node)
QBT_PASS=$(head -c 24 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | cut -c1-20)

# 6) brings up the Torrent Engine on the assigned port
DATA_PROPAGATION=$(data_propagation /data)
if [ "$DATA_PROPAGATION" != rslave ]; then
  echo "   !! the mount containing /data is 'private' (or findmnt does not exist): Docker would refuse the bind"
  echo "      rslave. Using rprivate — a disk mounted at /data after the Engine starts is only reachable"
  echo "      by recreating it (sh /opt/seedgrid-node/bootstrap-node.sh --storage-only does that). To get rslave,"
  echo "      share the root at boot ('mount --make-rshared /') and change SEEDGRID_DATA_PROPAGATION"
  echo "      in $INSTALL_DIR/.env."
fi
printf 'SEEDGRID_ENGINE_BT_PORT=%s\nSEEDGRID_DATA_PROPAGATION=%s\n' "$BT_PORT" "$DATA_PROPAGATION" > "$INSTALL_DIR/.env"
( cd "$INSTALL_DIR" && docker compose up -d )

# 6b) swaps the temporary qBittorrent password for the permanent one (via WebUI API)
echo "==> configuring qBittorrent WebUI password"
TMP_PASS=""
i=0
while [ "$i" -lt 30 ]; do
  TMP_PASS=$(docker logs "$ENGINE_CONTAINER" 2>&1 \
    | sed -n 's/.*temporary password[^:]*:[[:space:]]*\([^[:space:]]*\).*/\1/p' | tail -1)
  [ -n "$TMP_PASS" ] && break
  i=$((i + 1)); sleep 2
done
CJ=$(mktemp)
if [ -n "$TMP_PASS" ] && curl -fsS -c "$CJ" \
     --data-urlencode "username=admin" --data-urlencode "password=$TMP_PASS" \
     "$QBT_API/api/v2/auth/login" -H "Referer: $QBT_API" >/dev/null 2>&1; then
  curl -fsS -b "$CJ" "$QBT_API/api/v2/app/setPreferences" -H "Referer: $QBT_API" \
    --data-urlencode "json={\"web_ui_password\":\"$QBT_PASS\"}" >/dev/null 2>&1 \
    && echo "   WebUI password set" \
    || echo "   !! could not set the password via API — manual adjustment may be required"
else
  echo "   !! qBittorrent temporary password not found in the logs — set the password manually"
  echo "      and update SEEDGRID_ENGINE_QBT_PASSWORD in /etc/seedgrid-agent.env"
fi
rm -f "$CJ"

# 7) configures the Agent (the join token serves as the registration token — one-time) and brings up the service
write_agent_env
install -m0644 "$INSTALL_DIR/seedgrid-agent.service" "$AGENT_UNIT"
systemctl daemon-reload
systemctl enable --now seedgrid-agent

IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
# The address to allow is the token's, when it is up; otherwise, the first global one.
IP6=""
if [ -n "${NODE_INDEX:-}" ] && [ -n "${MAIN_IF:-}" ]; then IP6=$(ipv6_token_addr "$MAIN_IF" "$NODE_INDEX"); fi
[ -n "$IP6" ] || IP6=$(ip -6 addr show scope global 2>/dev/null | awk '/inet6/{print $2}' | cut -d/ -f1 | head -1)
echo "==> done. The Node registered with the Control Plane on BitTorrent port $BT_PORT."
if [ -n "$IP6" ]; then
  echo "    Node with a public IPv4: open the WAN PORT-FORWARD $BT_PORT -> ${IP:-<this-node-ip>}:$BT_PORT (TCP+UDP)."
  echo "    If behind CGNAT (no public IPv4 — ADR-034): the port-forward is ineffective. Instead,"
  echo "    allow INBOUND IPv6 on port $BT_PORT (TCP+UDP) on the router firewall to [$IP6]:$BT_PORT."
  if [ -n "${NODE_INDEX:-}" ] && ipv6_has_token_addr "${MAIN_IF:-}" "$NODE_INDEX"; then
    echo "    This address is stable: the suffix ::5eed:$NODE_INDEX does not depend on the MAC, so the rule remains valid"
    echo "    if the Node is recreated or changes host (only a new provider /64 prefix invalidates it)."
  fi
elif [ -n "${IPV6_CONFLICT_MAC:-}" ]; then
  echo "    !! this Node has NO global IPv6: the address ::5eed:$NODE_INDEX is already used by $IPV6_CONFLICT_MAC on this network."
  echo "       Under CGNAT (no public IPv4) this makes it UNREACHABLE. Reapply with a free index:"
  echo "         IPV6_NODE_INDEX=<N> sh /opt/seedgrid-node/bootstrap-node.sh --network-only"
else
  echo "    !! no global IPv6 detected on this Node. Under CGNAT (no public IPv4) this makes the Node"
  echo "       UNREACHABLE — check accept_ra/SLAAC (ADR-034) before allowing any firewall rule."
fi
if [ "$REQUIRE_MOUNT" = false ]; then
  echo "    Storage: this Node uses the SYSTEM DISK ($STORAGE_MOUNT_KEY=false). To get a dedicated disk"
  echo "    later: connect it and run  sh /opt/seedgrid-node/bootstrap-node.sh --storage-only"
fi
echo "    Check the Portal: $ARTIFACTS  (Health & Metrics)."
