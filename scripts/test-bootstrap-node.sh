#!/usr/bin/env sh
# SeedGrid — tests for the bootstrap-node.sh functions, without touching the host.
#
# The bootstrap runs as root on a real Node and tweaks fstab, sysctl, network and Docker;
# there is no way to "run it" in a test. What can be proven is the LOGIC of the functions that decide
# what to do. This script extracts each bootstrap function by name (without editing the
# bootstrap, without test hooks in it) and runs it with the external commands (findmnt, ip,
# nmcli, ...) replaced by stubs that return the recorded output of a real host.
#
# Usage:  sh scripts/test-bootstrap-node.sh

# Stub bodies go in single quotes ON PURPOSE: they are the fake command code,
# which expands inside it, not here.
# shellcheck disable=SC2016
set -u

ROOT=$(cd "$(dirname "$0")/.." && pwd)
SCRIPT="$ROOT/scripts/bootstrap-node.sh"
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT INT TERM
STUBS="$WORK/stubs"
mkdir -p "$STUBS"
PATH="$STUBS:$PATH"

pass=0
fail=0

# extract NAME prints the bootstrap NAME function (from the line `NAME() {` to the `}` at column 0).
extract() {
  awk -v f="$1" '$0 ~ ("^" f "\\(\\) \\{") { p = 1 } p { print } p && /^}/ { exit }' "$SCRIPT"
}

# load NAME... loads the functions into the test shell; fails loudly if any is missing.
load() {
  for fn in "$@"; do
    src=$(extract "$fn")
    if [ -z "$src" ]; then
      echo "ERROR: function $fn not found in $SCRIPT"
      exit 2
    fi
    eval "$src"
  done
}

# stub NAME 'body' creates a fake NAME executable in the test PATH.
stub() {
  printf '#!/bin/sh\n%s\n' "$2" > "$STUBS/$1"
  chmod +x "$STUBS/$1"
}

assert_eq() { # description expected got
  if [ "$2" = "$3" ]; then
    pass=$((pass + 1))
  else
    fail=$((fail + 1))
    echo "FAILED: $1"
    echo "   expected: [$2]"
    echo "   got:      [$3]"
  fi
}

# ---------------------------------------------------------------------------
# data_propagation — Docker refuses rslave bind over a `private` mount; the bootstrap only
# requests rslave when the mount containing /data is shared or slave (#143).
# ---------------------------------------------------------------------------
load data_propagation
stub findmnt 'echo "$FAKE_PROPAGATION"'

for tc in "shared:rslave" "slave:rslave" "shared,slave:rslave" "private:rprivate" "unbindable:rprivate" ":rprivate"; do
  FAKE_PROPAGATION=${tc%%:*}
  export FAKE_PROPAGATION
  assert_eq "data_propagation with findmnt='${tc%%:*}'" "${tc#*:}" "$(data_propagation /data)"
done

# findmnt missing (or failing): there's no way to know, so it falls back to the old behavior.
stub findmnt 'exit 1'
assert_eq "data_propagation with findmnt failing" "rprivate" "$(data_propagation /data)"

# ---------------------------------------------------------------------------
# set_env_kv — creates, appends and replaces without duplicating.
# ---------------------------------------------------------------------------
load set_env_kv
F="$WORK/compose.env"
set_env_kv SEEDGRID_DATA_PROPAGATION rslave "$F"
assert_eq "set_env_kv creates the file" "SEEDGRID_DATA_PROPAGATION=rslave" "$(cat "$F")"
printf 'SEEDGRID_ENGINE_BT_PORT=6884\n' > "$F"
set_env_kv SEEDGRID_DATA_PROPAGATION rprivate "$F"
assert_eq "set_env_kv adds while preserving the other keys" \
  "SEEDGRID_ENGINE_BT_PORT=6884 SEEDGRID_DATA_PROPAGATION=rprivate" "$(tr '\n' ' ' < "$F" | sed 's/ $//')"
set_env_kv SEEDGRID_DATA_PROPAGATION rslave "$F"
assert_eq "set_env_kv replaces existing value without duplicating" \
  "SEEDGRID_ENGINE_BT_PORT=6884 SEEDGRID_DATA_PROPAGATION=rslave" "$(tr '\n' ' ' < "$F" | sed 's/ $//')"

# ---------------------------------------------------------------------------
# IPv6 privacy extensions (#142).
#
# The simulated `ip` outputs use documentation addresses (2001:db8::/32) and the real
# format of `ip -o`; what matters for the test is the format, not the address.
# ---------------------------------------------------------------------------
load primary_iface sysctl_iface_key set_sysctl_kv disable_ipv6_privacy

stub ip '
case "$*" in
  "-6 route show default") printf "%s" "${FAKE_ROUTE6:-}" ;;
  "-4 route show default") printf "%s" "${FAKE_ROUTE4:-}" ;;
  "-6 -o addr show dev "*) printf "%s" "${FAKE_ADDRS:-}" ;;
  "-6 addr del "*) echo "$*" >> "$IP_LOG" ;;
esac
exit 0'

# primary_iface: the default IPv6 route rules; without it, the IPv4 one; NODE_IFACE wins overall.
R6='default via fe80::1 dev eth0 proto ra metric 600 pref medium
'
R4='default via 192.0.2.1 dev wlan0 proto dhcp src 192.0.2.50 metric 600
'
export FAKE_ROUTE6="$R6" FAKE_ROUTE4="$R4"
assert_eq "primary_iface prefers the IPv6 default route" "eth0" "$(primary_iface)"
export FAKE_ROUTE6=""
assert_eq "primary_iface falls back to IPv4 route when IPv6 hasn't arrived yet" "wlan0" "$(primary_iface)"
assert_eq "primary_iface respects NODE_IFACE" "eth9" "$(NODE_IFACE=eth9 primary_iface)"
export FAKE_ROUTE4=""
primary_iface >/dev/null 2>&1
assert_eq "primary_iface fails with no default route" "1" "$?"

assert_eq "sysctl_iface_key uses dots in a simple name" "net.ipv6.conf.eth0.use_tempaddr" "$(sysctl_iface_key eth0 use_tempaddr)"
assert_eq "sysctl_iface_key uses slashes in VLAN interface" "net/ipv6/conf/eth0.100/use_tempaddr" "$(sysctl_iface_key eth0.100 use_tempaddr)"

# disable_ipv6_privacy
SYSCTL_CONF="$WORK/99-seedgrid-ipv6.conf"
SYSCTL_LOG="$WORK/sysctl.log"
IP_LOG="$WORK/ip.log"
export IP_LOG SYSCTL_LOG
stub sysctl 'echo "$*" >> "$SYSCTL_LOG"; exit 0'
# One stable and one temporary address (`temporary` flag) on the same interface.
export FAKE_ADDRS='2: eth0    inet6 2001:db8:1:2::5eed:4/64 scope global dynamic noprefixroute \       valid_lft 7100sec preferred_lft 3500sec
2: eth0    inet6 2001:db8:1:2:b8c1:7f3a:9e21:4d05/64 scope global temporary dynamic \       valid_lft 80000sec preferred_lft 3600sec
'
: > "$SYSCTL_LOG"; : > "$IP_LOG"; rm -f "$SYSCTL_CONF"
(set -e; disable_ipv6_privacy eth0 >/dev/null)
assert_eq "sysctl.d receives the three keys, at 0" \
  "net.ipv6.conf.all.use_tempaddr = 0|net.ipv6.conf.default.use_tempaddr = 0|net.ipv6.conf.eth0.use_tempaddr = 0" \
  "$(grep -v '^#' "$SYSCTL_CONF" | tr '\n' '|' | sed 's/|$//')"
assert_eq "the three keys are applied at runtime" \
  "-qw net.ipv6.conf.all.use_tempaddr=0|-qw net.ipv6.conf.default.use_tempaddr=0|-qw net.ipv6.conf.eth0.use_tempaddr=0" \
  "$(tr '\n' '|' < "$SYSCTL_LOG" | sed 's/|$//')"
assert_eq "only the temporary address is removed (the stable one remains)" \
  "-6 addr del 2001:db8:1:2:b8c1:7f3a:9e21:4d05/64 dev eth0" "$(cat "$IP_LOG")"

# Idempotency: the second pass leaves the file byte-for-byte identical.
cp "$SYSCTL_CONF" "$WORK/sysctl.first"
(set -e; disable_ipv6_privacy eth0 >/dev/null)
assert_eq "second pass does not change sysctl.d" "" "$(diff "$WORK/sysctl.first" "$SYSCTL_CONF")"

# Pre-existing file from another source (e.g., hand-crafted accept_ra): preserves the foreign
# lines, replaces the wrong value and does not duplicate.
printf '# hand-crafted\nnet.ipv6.conf.default.accept_ra=2\nnet.ipv6.conf.eth0.accept_ra=2\nnet.ipv6.conf.all.use_tempaddr = 2\n' > "$SYSCTL_CONF"
(set -e; disable_ipv6_privacy eth0 >/dev/null)
assert_eq "unrelated lines preserved and use_tempaddr replaced without duplicating" \
  "net.ipv6.conf.default.accept_ra=2|net.ipv6.conf.eth0.accept_ra=2|net.ipv6.conf.all.use_tempaddr = 0|net.ipv6.conf.default.use_tempaddr = 0|net.ipv6.conf.eth0.use_tempaddr = 0" \
  "$(grep -v '^#' "$SYSCTL_CONF" | tr '\n' '|' | sed 's/|$//')"
assert_eq "foreign accept_ra is NOT reapplied at runtime" "0" \
  "$(grep -c accept_ra "$SYSCTL_LOG")"

# Minimal host without /etc/sysctl.d: the directory is created (it does not abort the bootstrap).
SYSCTL_CONF="$WORK/no-directory/sysctl.d/99-seedgrid-ipv6.conf"
(set -e; disable_ipv6_privacy eth0 >/dev/null)
assert_eq "without /etc/sysctl.d: creates the directory and writes" "1" "$(grep -c '^net.ipv6.conf.all.use_tempaddr = 0$' "$SYSCTL_CONF")"
SYSCTL_CONF="$WORK/99-seedgrid-ipv6.conf"

# VLAN interface: key with slashes.
rm -f "$SYSCTL_CONF"
(set -e; disable_ipv6_privacy eth0.100 >/dev/null)
assert_eq "dotted interface uses the slash form in sysctl.d" "1" \
  "$(grep -c '^net/ipv6/conf/eth0.100/use_tempaddr = 0$' "$SYSCTL_CONF")"

# ---------------------------------------------------------------------------
# Deterministic IPv6 suffix (ADR-038, #202).
# ---------------------------------------------------------------------------
load index_for_port ipv6_expand ipv6_tail ipv6_global_addrs ipv6_has_token_addr ipv6_token_addr \
  ipv6_token_valid_lft wait_ipv6_token_addr prune_stale_ipv6 networkd_manages apply_token_networkd \
  nm_manages nm_write_ipv6_token nm_loaded_token_ok apply_token_nm print_manual_ipv6_instructions \
  ipv6_dad_conflict configure_ipv6_suffix

# The mirrors of the Control Plane constants come from the script itself: if someone changes the value there
# without changing the Control Plane (or vice versa), the Go test (TestBootstrapMirrorsPortSequence)
# is the one that catches it; here only the real value is used.
eval "$(grep -E '^(BT_PORT_BASE|BT_PORT_SLOTS)=' "$SCRIPT")"

assert_eq "index_for_port: first port in sequence" "1" "$(index_for_port 6881)"
assert_eq "index_for_port: fourth port" "4" "$(index_for_port 6884)"
assert_eq "index_for_port: last port in sequence" "1000" "$(index_for_port 7880)"
for bad in 6880 7881 06881 abc "" 0 -5; do
  index_for_port "$bad" >/dev/null 2>&1
  assert_eq "index_for_port rejects '$bad'" "1" "$?"
done

# ipv6_expand: the same way to compare, regardless of the spelling.
assert_eq "ipv6_expand: regular address with /len" \
  "2001:0db8:0001:0002:0000:0000:5eed:0004" "$(ipv6_expand 2001:db8:1:2::5eed:4/64)"
assert_eq "ipv6_expand: ::5eed:4 and ::94.237.0.4 are the SAME address (0x5eed = 94.237)" \
  "$(ipv6_expand ::5eed:4)" "$(ipv6_expand ::94.237.0.4)"
assert_eq "ipv6_expand: dotted tail expands correctly" \
  "0000:0000:0000:0000:0000:0000:5eed:0004" "$(ipv6_expand ::94.237.0.4)"
assert_eq "ipv6_expand: the dotted form of Node 12 (0x0012 = 0.18)" \
  "$(ipv6_expand ::5eed:12)" "$(ipv6_expand ::94.237.0.18)"
assert_eq "ipv6_expand: uppercase" "$(ipv6_expand ::5eed:4)" "$(ipv6_expand ::5EED:4)"
assert_eq "ipv6_expand: ::" "0000:0000:0000:0000:0000:0000:0000:0000" "$(ipv6_expand ::)"
assert_eq "ipv6_expand: eight groups without ::" \
  "0001:0002:0003:0004:0005:0006:0007:0008" "$(ipv6_expand 1:2:3:4:5:6:7:8)"
assert_eq "ipv6_expand: :: in the middle" "2001:0db8:0000:0000:0000:0000:0000:0001" "$(ipv6_expand 2001:db8::1)"
for bad in 2001:db8::1::2 gggg::1 1:2:3 12345::1 not-an-address; do
  ipv6_expand "$bad" >/dev/null 2>&1
  assert_eq "ipv6_expand rejects '$bad'" "1" "$?"
done

assert_eq "ipv6_tail of Node 4" "0000:0000:5eed:0004" "$(ipv6_tail 4)"
assert_eq "Node 12 ipv6_tail uses decimal digits (::5eed:12)" "0000:0000:5eed:0012" "$(ipv6_tail 12)"

# Network command stubs. `ip` returns $FAKE_ADDRS until the $APPLIED_MARK marker exists (the
# simulated `networkctl reconfigure`/`nmcli device reapply` create it) and $FAKE_ADDRS_AFTER afterwards.
stub ip '
case "$*" in
  "-6 route show default") printf "%s" "${FAKE_ROUTE6:-}" ;;
  "-4 route show default") printf "%s" "${FAKE_ROUTE4:-}" ;;
  "-6 -o addr show dev "*)
    if [ -n "${APPLIED_MARK:-}" ] && [ -e "$APPLIED_MARK" ]; then printf "%s" "${FAKE_ADDRS_AFTER:-}"; else printf "%s" "${FAKE_ADDRS:-}"; fi ;;
  "-6 addr del "*) echo "$*" >> "$IP_LOG" ;;
esac
exit 0'
stub sleep 'exit 0'
# The test kernel log: empty, unless the case exports FAKE_DMESG.
stub dmesg 'printf "%s" "${FAKE_DMESG:-}"'
stub journalctl 'exit 1'
stub systemctl 'echo "$*" >> "${SYSTEMCTL_LOG:-/dev/null}"; exit 0'
stub networkctl '
case "$1" in
  status) printf "%s\n" "$FAKE_NETWORKCTL_STATUS" ;;
  reload) echo "reload" >> "$NET_LOG" ;;
  reconfigure) echo "reconfigure $2" >> "$NET_LOG"; [ -z "${APPLIED_MARK:-}" ] || : > "$APPLIED_MARK" ;;
esac
exit 0'
stub nmcli '
case "$*" in
  "-g GENERAL.STATE device show "*) echo "${FAKE_NM_STATE:-100 (connected)}" ;;
  "-g GENERAL.CON-UUID device show "*) echo "${FAKE_NM_UUID:-}" ;;
  "-g ipv6.addr-gen-mode,ipv6.token connection show "*) printf "%s\n" "${FAKE_NM_LOADED:-}" ;;
  "connection reload") echo "reload" >> "$NM_LOG"; exit "${FAKE_NM_RELOAD_RC:-0}" ;;
  "device reapply "*) echo "$*" >> "$NM_LOG"; [ -z "${APPLIED_MARK:-}" ] || : > "$APPLIED_MARK"; exit "${FAKE_NM_REAPPLY_RC:-0}" ;;
esac
exit 0'

NETWORKD_DIR="$WORK/net"
NM_DIR="$WORK/nm"
BACKUP_DIR="$WORK/bak"
APPLIED_MARK="$WORK/applied"
NET_LOG="$WORK/net.log"
NM_LOG="$WORK/nm.log"
SYSTEMCTL_LOG="$WORK/systemctl.log"
export APPLIED_MARK NET_LOG NM_LOG SYSTEMCTL_LOG IP_LOG
mkdir -p "$NETWORKD_DIR" "$NM_DIR"

# addr_line ADDRESS/LEN FLAGS VALID_LFT: a line in the real format of `ip -6 -o addr show`.
addr_line() {
  printf '2: eth0    inet6 %s scope global %s \\       valid_lft %s preferred_lft 3500sec\n' "$1" "$2" "$3"
}
TOKEN4=$(addr_line 2001:db8:1:2::5eed:4/64 'dynamic noprefixroute' 7100sec)
EUI64=$(addr_line 2001:db8:1:2:200:5eff:fe10:1/64 'dynamic mngtmpaddr noprefixroute' 7100sec)
TEMP=$(addr_line 2001:db8:1:2:b8c1:7f3a:9e21:4d05/64 'temporary dynamic' 80000sec)
STATIC=$(addr_line 2001:db8:1:2::99/64 'noprefixroute' forever)
OTHERPFX=$(addr_line 2001:db8:9:9:200:5eff:fe10:1/64 'dynamic' 7100sec)
TENTATIVE7=$(addr_line 2001:db8:1:2::5eed:7/64 'tentative dynamic' 7100sec)
fixture() { printf '%s\n' "$@"; }

# ipv6_has_token_addr / ipv6_token_addr / ipv6_token_valid_lft
FAKE_ADDRS=$(fixture "$TOKEN4" "$EUI64" "$TEMP" "$TENTATIVE7"); export FAKE_ADDRS
ipv6_has_token_addr eth0 4; assert_eq "has_token_addr: Node 4 is up" "0" "$?"
ipv6_has_token_addr eth0 40 >/dev/null 2>&1; assert_eq "has_token_addr: ::5eed:40 does NOT match ::5eed:4" "1" "$?"
ipv6_has_token_addr eth0 5 >/dev/null 2>&1; assert_eq "has_token_addr: Node 5 is not up" "1" "$?"
ipv6_has_token_addr eth0 7 >/dev/null 2>&1; assert_eq "has_token_addr: address in DAD (tentative) does not count" "1" "$?"
assert_eq "ipv6_token_addr returns the address without /len" "2001:db8:1:2::5eed:4" "$(ipv6_token_addr eth0 4)"
assert_eq "ipv6_token_valid_lft returns the seconds" "7100" "$(ipv6_token_valid_lft eth0 4)"

# prune_stale_ipv6: only the dynamic residue of the SAME /64, and only with the token address standing.
FAKE_ADDRS=$(fixture "$TOKEN4" "$EUI64" "$TEMP" "$STATIC" "$OTHERPFX"); export FAKE_ADDRS
: > "$IP_LOG"
(set -e; prune_stale_ipv6 eth0 4 >/dev/null)
assert_eq "prune deletes the EUI-64 and temporary from the token's /64; preserves the token, the static and the other prefix" \
  "-6 addr del 2001:db8:1:2:200:5eff:fe10:1/64 dev eth0|-6 addr del 2001:db8:1:2:b8c1:7f3a:9e21:4d05/64 dev eth0" \
  "$(tr '\n' '|' < "$IP_LOG" | sed 's/|$//')"
FAKE_ADDRS=$(fixture "$EUI64" "$TEMP"); export FAKE_ADDRS
: > "$IP_LOG"
(set -e; prune_stale_ipv6 eth0 4 >/dev/null)
assert_eq "prune deletes NOTHING if the token address is not up (never leaves the Node without an address)" "" "$(cat "$IP_LOG")"

# networkd_manages
FAKE_NETWORKCTL_STATUS='● 2: eth0
                     Link File: /usr/lib/systemd/network/99-default.link
                  Network File: /etc/systemd/network/eth0.network
                         State: routable (configured)'; export FAKE_NETWORKCTL_STATUS
networkd_manages eth0; assert_eq "networkd_manages: interface (configured)" "0" "$?"
FAKE_NETWORKCTL_STATUS='● 2: eth0
                  Network File: n/a
                         State: routable (unmanaged)'; export FAKE_NETWORKCTL_STATUS
networkd_manages eth0; assert_eq "networkd_manages: interface (unmanaged)" "1" "$?"

# apply_token_networkd: the drop-in goes next to the .network that networkd says it's using — including
# the one generated in /run by netplan —, and the link is reconfigured (not the whole service restarted).
FAKE_NETWORKCTL_STATUS='Network File: /etc/systemd/network/eth0.network
State: routable (configured)'; export FAKE_NETWORKCTL_STATUS
: > "$NET_LOG"; : > "$SYSTEMCTL_LOG"; rm -f "$APPLIED_MARK"
(set -e; apply_token_networkd eth0 4 >/dev/null)
DROPIN="$NETWORKD_DIR/eth0.network.d/99-seedgrid-ipv6-ra.conf"
assert_eq "networkd: drop-in with the token" "1" "$(grep -c '^IPv6Token=static:::5eed:4$' "$DROPIN")"
assert_eq "networkd: drop-in keeps IPv6AcceptRA=true (ADR-034)" "1" "$(grep -c '^IPv6AcceptRA=true$' "$DROPIN")"
assert_eq "networkd: reloads and reconfigures only the link" "reload|reconfigure eth0" "$(tr '\n' '|' < "$NET_LOG" | sed 's/|$//')"
assert_eq "networkd: does not restart the service when reconfigure exists" "0" "$(grep -c restart "$SYSTEMCTL_LOG")"
FAKE_NETWORKCTL_STATUS='Network File: /run/systemd/network/10-netplan-eth0.network
State: routable (configured)'; export FAKE_NETWORKCTL_STATUS
(set -e; apply_token_networkd eth0 12 >/dev/null)
assert_eq "networkd: file generated by netplan gets the drop-in in /etc, with the SAME name" "1" \
  "$(grep -c '^IPv6Token=static:::5eed:12$' "$NETWORKD_DIR/10-netplan-eth0.network.d/99-seedgrid-ipv6-ra.conf")"
FAKE_NETWORKCTL_STATUS='Network File: n/a
State: routable (configured)'; export FAKE_NETWORKCTL_STATUS
apply_token_networkd eth0 4 >/dev/null; assert_eq "networkd: no known .network file, refuses" "1" "$?"

# nm_manages
FAKE_NM_STATE='100 (connected)'; export FAKE_NM_STATE
nm_manages wlan0; assert_eq "nm_manages: device connected" "0" "$?"
FAKE_NM_STATE='10 (unmanaged)'; export FAKE_NM_STATE
nm_manages wlan0; assert_eq "nm_manages: device unmanaged" "1" "$?"

# nm_write_ipv6_token: the keyfile of a WiFi connection has the password — nothing other than [ipv6] can change.
NM_UUID=00000000-0000-4000-8000-000000000001
write_keyfile() { # FILE IPV6_SECTION...
  f=$1; shift
  {
    printf '[connection]\nid=Net\nuuid=%s\ntype=wifi\ninterface-name=wlan0\n\n' "$NM_UUID"
    printf '[wifi]\nssid=Net\n\n[wifi-security]\nkey-mgmt=wpa-psk\npsk=not-a-real-secret\n\n'
    printf '[ipv4]\nmethod=auto\n\n'
    [ "$#" -eq 0 ] || { printf '[ipv6]\n'; printf '%s\n' "$@"; printf '\n[proxy]\n'; }
  } > "$f"
}
ipv6_section() { awk '/^\[/ { s = ($0 == "[ipv6]") } s' "$1" | grep -v '^$' | tr '\n' '|' | sed 's/|$//'; }

KF="$WORK/kf1"
write_keyfile "$KF" addr-gen-mode=stable-privacy method=auto
nm_write_ipv6_token "$KF" ::5eed:4
assert_eq "keyfile: changes addr-gen-mode and adds the token" \
  "[ipv6]|addr-gen-mode=eui64|method=auto|token=::5eed:4" "$(ipv6_section "$KF")"
assert_eq "keyfile: the password and other sections remain intact" "1" "$(grep -c '^psk=not-a-real-secret$' "$KF")"
assert_eq "keyfile: the next section ([proxy]) continues after [ipv6]" "1" "$(grep -c '^\[proxy\]$' "$KF")"
cp "$KF" "$WORK/kf1.first"
nm_write_ipv6_token "$KF" ::5eed:4
assert_eq "keyfile: idempotent" "" "$(diff "$WORK/kf1.first" "$KF")"
write_keyfile "$KF" addr-gen-mode=eui64 token=::94.237.0.9 method=auto
nm_write_ipv6_token "$KF" ::5eed:4
assert_eq "keyfile: replaces a token in colon-separated notation, without duplicating" \
  "[ipv6]|addr-gen-mode=eui64|token=::5eed:4|method=auto" "$(ipv6_section "$KF")"
write_keyfile "$KF"
nm_write_ipv6_token "$KF" ::5eed:4
assert_eq "keyfile without [ipv6]: the section is created" \
  "[ipv6]|method=auto|addr-gen-mode=eui64|token=::5eed:4" "$(ipv6_section "$KF")"

# nm_loaded_token_ok: what NetworkManager HAS loaded, in both token spellings.
FAKE_NM_LOADED='eui64
\:\:94.237.0.4'; export FAKE_NM_LOADED
nm_loaded_token_ok "$NM_UUID" 4; assert_eq "loaded: ::94.237.0.4 (colon-separated notation, ':' escaped) is Node 4" "0" "$?"
FAKE_NM_LOADED='eui64
\:\:5eed\:4'; export FAKE_NM_LOADED
nm_loaded_token_ok "$NM_UUID" 4; assert_eq "loaded: ::5eed:4 is also Node 4" "0" "$?"
nm_loaded_token_ok "$NM_UUID" 5 >/dev/null 2>&1; assert_eq "loaded: token from another Node fails" "1" "$?"
FAKE_NM_LOADED='stable-privacy
\:\:5eed\:4'; export FAKE_NM_LOADED
nm_loaded_token_ok "$NM_UUID" 4 >/dev/null 2>&1; assert_eq "loaded: token without addr-gen-mode=eui64 fails" "1" "$?"
FAKE_NM_LOADED=''; export FAKE_NM_LOADED
nm_loaded_token_ok "$NM_UUID" 4 >/dev/null 2>&1; assert_eq "loaded: nothing loaded fails" "1" "$?"

# apply_token_nm end-to-end: backup, edit, reload, check, reapply.
export FAKE_NM_UUID="$NM_UUID"
FAKE_NM_LOADED='eui64
\:\:94.237.0.4'; export FAKE_NM_LOADED
NMF="$NM_DIR/Net"   # keyfile without extension, which Ubuntu's NM also accepts
write_keyfile "$NMF" addr-gen-mode=stable-privacy method=auto
cp "$NMF" "$WORK/nm.orig"
: > "$NM_LOG"; rm -f "$APPLIED_MARK"
(set -e; apply_token_nm wlan0 4 >/dev/null); assert_eq "apply_token_nm: success" "0" "$?"
assert_eq "apply_token_nm: keyfile with the token" "[ipv6]|addr-gen-mode=eui64|method=auto|token=::5eed:4" "$(ipv6_section "$NMF")"
assert_eq "apply_token_nm: backup identical to the original" "" "$(diff "$WORK/nm.orig" "$BACKUP_DIR"/Net.*)"
assert_eq "apply_token_nm: reload BEFORE reapply" "reload|device reapply wlan0" "$(tr '\n' '|' < "$NM_LOG" | sed 's/|$//')"

# reapply failure: the keyfile reverts to what it was (disk and memory cannot diverge).
write_keyfile "$NMF" addr-gen-mode=stable-privacy method=auto
cp "$NMF" "$WORK/nm.orig"
export FAKE_NM_REAPPLY_RC=1
apply_token_nm wlan0 4 >/dev/null 2>&1; assert_eq "apply_token_nm: reapply failure is reported" "1" "$?"
assert_eq "apply_token_nm: a failed reapply restores the keyfile" "" "$(diff "$WORK/nm.orig" "$NMF")"
unset FAKE_NM_REAPPLY_RC
# NM loaded ANOTHER token (e.g., normalized differently than expected): it doesn't even reach the reapply.
FAKE_NM_LOADED='eui64
\:\:94.237.0.5'; export FAKE_NM_LOADED
: > "$NM_LOG"
apply_token_nm wlan0 4 >/dev/null 2>&1; assert_eq "apply_token_nm: loaded token different from requested is reported" "1" "$?"
assert_eq "apply_token_nm: ...and the keyfile is restored" "" "$(diff "$WORK/nm.orig" "$NMF")"
assert_eq "apply_token_nm: ...without reapplying" "0" "$(grep -c reapply "$NM_LOG")"
FAKE_NM_LOADED='eui64
\:\:94.237.0.4'; export FAKE_NM_LOADED
# Connection that is not a keyfile in /etc (netplan generates it in /run): changes nothing.
export FAKE_NM_UUID=00000000-0000-4000-8000-0000000000ff
apply_token_nm wlan0 4 >/dev/null 2>&1; assert_eq "apply_token_nm: connection that is not a keyfile in $NM_DIR is refused" "1" "$?"
export FAKE_NM_UUID="$NM_UUID"
# ipv6.method different from auto: the token is not valid.
write_keyfile "$NMF" addr-gen-mode=eui64 method=manual
cp "$NMF" "$WORK/nm.orig"
apply_token_nm wlan0 4 >/dev/null 2>&1; assert_eq "apply_token_nm: ipv6.method=manual is refused" "1" "$?"
assert_eq "apply_token_nm: ...and the keyfile remains untouched" "" "$(diff "$WORK/nm.orig" "$NMF")"

# configure_ipv6_suffix: the orchestration, with the network manager for each case.
R6='default via fe80::1 dev eth0 proto ra metric 600 pref medium
'
export FAKE_ROUTE6="$R6" FAKE_ROUTE4=""
reset_net() { : > "$NET_LOG"; : > "$NM_LOG"; : > "$IP_LOG"; : > "$SYSTEMCTL_LOG"; rm -f "$APPLIED_MARK"; rm -rf "${NETWORKD_DIR:?}"/*; }

# 1) opt-out
reset_net
FAKE_ADDRS=$(fixture "$EUI64"); export FAKE_ADDRS
out=$(set -e; IPV6_SUFFIX=off configure_ipv6_suffix 4)
assert_eq "IPV6_SUFFIX=off does not touch anything" "0" "$(cat "$NET_LOG" "$NM_LOG" "$IP_LOG" | wc -l | tr -d ' ')"
assert_eq "IPV6_SUFFIX=off warns" "1" "$(printf '%s\n' "$out" | grep -c 'IPV6_SUFFIX=off')"
assert_eq "IPV6_SUFFIX=off does not create drop-in" "0" "$(find "$NETWORKD_DIR" -type f | wc -l | tr -d ' ')"

# 2) invalid index: warning, never error
for bad in "" 0 abc 10000 07; do
  reset_net
  out=$(set -e; configure_ipv6_suffix "$bad"); rc=$?
  assert_eq "index '$bad': returns 0 (never crashes the bootstrap)" "0" "$rc"
  assert_eq "index '$bad': warns and does not configure anything" "0" "$(cat "$NET_LOG" "$NM_LOG" | wc -l | tr -d ' ')"
done

# 3) already applied: idempotent — does not reconfigure, and still cleans up the residue
reset_net
FAKE_ADDRS=$(fixture "$TOKEN4" "$EUI64"); export FAKE_ADDRS
out=$(set -e; configure_ipv6_suffix 4)
assert_eq "already applied: does not reconfigure the network" "0" "$(cat "$NET_LOG" "$NM_LOG" | wc -l | tr -d ' ')"
assert_eq "already applied: does not write drop-in" "0" "$(find "$NETWORKD_DIR" -type f | wc -l | tr -d ' ')"
assert_eq "already applied: says there is nothing to change" "1" "$(printf '%s\n' "$out" | grep -c 'nothing to change')"
assert_eq "already applied: still removes the residual EUI-64" "-6 addr del 2001:db8:1:2:200:5eff:fe10:1/64 dev eth0" "$(cat "$IP_LOG")"

# 4) networkd, end-to-end: the address only appears after the reconfigure
reset_net
FAKE_NETWORKCTL_STATUS='Network File: /etc/systemd/network/eth0.network
State: routable (configured)'; export FAKE_NETWORKCTL_STATUS
FAKE_ADDRS=$(fixture "$EUI64"); FAKE_ADDRS_AFTER=$(fixture "$TOKEN4" "$EUI64"); export FAKE_ADDRS FAKE_ADDRS_AFTER
out=$(set -e; configure_ipv6_suffix 4)
assert_eq "networkd: uses systemd-networkd" "1" "$(printf '%s\n' "$out" | grep -c 'manager: systemd-networkd')"
assert_eq "networkd: the drop-in was written" "1" "$(grep -c 'IPv6Token=static:::5eed:4' "$NETWORKD_DIR/eth0.network.d/99-seedgrid-ipv6-ra.conf")"
assert_eq "networkd: confirms the address is up" "1" "$(printf '%s\n' "$out" | grep -c 'address up: 2001:db8:1:2::5eed:4')"
assert_eq "networkd: remove o EUI-64 residual" "-6 addr del 2001:db8:1:2:200:5eff:fe10:1/64 dev eth0" "$(cat "$IP_LOG")"
assert_eq "networkd: asks to verify that valid_lft increases" "1" "$(printf '%s\n' "$out" | grep -c 'valid_lft.*7100s')"
unset FAKE_ADDRS_AFTER

# 5) NetworkManager, end-to-end
reset_net
FAKE_NETWORKCTL_STATUS='Network File: n/a
State: routable (unmanaged)'; export FAKE_NETWORKCTL_STATUS
FAKE_NM_STATE='100 (connected)'; export FAKE_NM_STATE
write_keyfile "$NMF" addr-gen-mode=stable-privacy method=auto
FAKE_ADDRS=$(fixture "$EUI64"); FAKE_ADDRS_AFTER=$(fixture "$TOKEN4" "$EUI64"); export FAKE_ADDRS FAKE_ADDRS_AFTER
out=$(set -e; configure_ipv6_suffix 4)
assert_eq "nm: uses NetworkManager" "1" "$(printf '%s\n' "$out" | grep -c 'manager: NetworkManager')"
assert_eq "nm: keyfile with the token" "[ipv6]|addr-gen-mode=eui64|method=auto|token=::5eed:4" "$(ipv6_section "$NMF")"
assert_eq "nm: remove o EUI-64 residual" "-6 addr del 2001:db8:1:2:200:5eff:fe10:1/64 dev eth0" "$(cat "$IP_LOG")"
unset FAKE_ADDRS_AFTER

# 6) unknown manager: instructions, nothing written
reset_net
FAKE_NM_STATE='10 (unmanaged)'; export FAKE_NM_STATE
FAKE_ADDRS=$(fixture "$EUI64"); export FAKE_ADDRS
out=$(set -e; configure_ipv6_suffix 4); rc=$?
assert_eq "unknown manager: returns 0" "0" "$rc"
assert_eq "unknown manager: prints the networkd instruction" "1" "$(printf '%s\n' "$out" | grep -c 'IPv6Token=static:::5eed:4')"
assert_eq "unknown manager: prints the NetworkManager instruction" "1" "$(printf '%s\n' "$out" | grep -c 'token=::5eed:4')"
assert_eq "unknown manager: writes nothing" "0" "$(find "$NETWORKD_DIR" -type f | wc -l | tr -d ' ')"

# 7) the RA never arrives: warns, keeps the saved configuration, deletes nothing
reset_net
FAKE_NETWORKCTL_STATUS='Network File: /etc/systemd/network/eth0.network
State: routable (configured)'; export FAKE_NETWORKCTL_STATUS
FAKE_ADDRS=$(fixture "$EUI64"); export FAKE_ADDRS
unset FAKE_ADDRS_AFTER
out=$(set -e; configure_ipv6_suffix 4); rc=$?
assert_eq "no RA: returns 0" "0" "$rc"
assert_eq "no RA: warns that the address has not appeared" "1" "$(printf '%s\n' "$out" | grep -c 'has not appeared on')"
assert_eq "no RA: does not delete the EUI-64 (it is the Node's only address)" "" "$(cat "$IP_LOG")"
assert_eq "no RA: does not mention address in use" "0" "$(printf '%s\n' "$out" | grep -c 'ALREADY IN USE')"

# 7b) the address doesn't appear because ANOTHER machine is already using it (DAD): says who and how to get out (#21)
reset_net
FAKE_ADDRS=$(fixture "$EUI64"); export FAKE_ADDRS
unset FAKE_ADDRS_AFTER
FAKE_DMESG='[  10.1] IPv6: eth0: IPv6 duplicate address 2001:db8:1:2::5eed:40 used by 02:00:00:00:00:40 detected!
[  12.3] IPv6: eth0: IPv6 duplicate address 2001:db8:1:2::5eed:4 used by 02:00:00:00:00:aa detected!
[  12.4] IPv6: eth0: IPv6 duplicate address fd00:1:2:3::5eed:4 used by 02:00:00:00:00:aa detected!
'; export FAKE_DMESG
assert_eq "ipv6_dad_conflict returns the MAC of whoever is using the address" "02:00:00:00:00:aa" "$(ipv6_dad_conflict 4)"
assert_eq "ipv6_dad_conflict does not confuse ::5eed:4 with ::5eed:40" "02:00:00:00:00:40" "$(ipv6_dad_conflict 40)"
ipv6_dad_conflict 7 >/dev/null 2>&1
assert_eq "ipv6_dad_conflict without a rejection for the index returns 1" "1" "$?"
out=$(set -e; configure_ipv6_suffix 4); rc=$?
assert_eq "address in use: returns 0" "0" "$rc"
assert_eq "address in use: prints the MAC" "1" "$(printf '%s\n' "$out" | grep -c 'IN USE on this network by 02:00:00:00:00:aa')"
assert_eq "address in use: shows how to reapply with another index" "1" "$(printf '%s\n' "$out" | grep -c 'IPV6_NODE_INDEX=<N> sh /opt/seedgrid-node/bootstrap-node.sh --network-only')"
assert_eq "address in use: does not blame Router Advertisement" "0" "$(printf '%s\n' "$out" | grep -c 'has not appeared on')"
assert_eq "address in use: does not delete the EUI-64" "" "$(cat "$IP_LOG")"
unset FAKE_DMESG

# 8) no default route: warning
reset_net
export FAKE_ROUTE6="" FAKE_ROUTE4=""
out=$(set -e; configure_ipv6_suffix 4); rc=$?
assert_eq "no interface: returns 0" "0" "$rc"
assert_eq "no interface: warns" "1" "$(printf '%s\n' "$out" | grep -c 'could not find the default route interface')"

# ---------------------------------------------------------------------------
# Dedicated disk, or system disk (#221).
#
# The bootstrap writes SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT according to what the storage step
# DID. The costly error is in both directions: writing `true` on a single-disk Node excludes it from the
# cluster forever; writing `false` on a Node that should have a disk removes the protection against the
# disk that didn't mount. The functions below are what decides and what writes.
# ---------------------------------------------------------------------------
load setup_storage fstab_declares_data storage_require_mount announce_storage_mode \
  declare_storage_dedicated restart_agent write_agent_env
eval "$(grep -E '^STORAGE_MOUNT_KEY=' "$SCRIPT")"
assert_eq "env key is the one the Agent reads" "SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT" "$STORAGE_MOUNT_KEY"

AGENT_ENV="$WORK/agent.env"
FSTAB="$WORK/fstab"
AGENT_UNIT="$WORK/agent.service"
unset STORAGE_REQUIRE_MOUNT STORAGE_DEVICE STORAGE_FORMAT AGENT_RESTARTED
# yes/no of what the test wants to know: the function itself can exit 1 (no match) or 2 (missing file).
yn() { if "$@"; then echo yes; else echo no; fi; }
exists() { [ -e "$1" ] && echo yes || echo no; }
reset_storage() { rm -f "$AGENT_ENV" "$FSTAB" "$AGENT_UNIT"; unset STORAGE_REQUIRE_MOUNT AGENT_RESTARTED; : > "$SYSTEMCTL_LOG"; }

# fstab_declares_data: only a real mount of /data counts.
reset_storage
assert_eq "missing fstab does not declare disk" "no" "$(yn fstab_declares_data)"
printf '%s\n' '# <file system> <mount point> <type> <options> <dump> <pass>' \
  'UUID=aaaa-1111 / ext4 defaults 0 1' '#UUID=bbbb-2222 /data ext4 defaults 0 2' \
  'UUID=cccc-3333 /database ext4 defaults 0 2' '/dev/sdb1 /mnt/data ext4 defaults 0 2' > "$FSTAB"
assert_eq "fstab without /data (commented, /database, /mnt/data) does not declare disk" "no" "$(yn fstab_declares_data)"
printf 'UUID=dddd-4444 /data ext4 defaults,noatime,nofail 0 2\n' >> "$FSTAB"
assert_eq "fstab with UUID=... /data declares disk" "yes" "$(yn fstab_declares_data)"
printf '/dev/sdb1\t/data\text4\tdefaults\t0\t2\n' > "$FSTAB"
assert_eq "fstab with tabs and /dev/... declares disk" "yes" "$(yn fstab_declares_data)"
printf '  UUID=eeee-5555   /data   ext4  defaults 0 2\n' > "$FSTAB"
assert_eq "fstab with leading spaces declares disk" "yes" "$(yn fstab_declares_data)"

# storage_require_mount: the decision, case by case.
rqm() { storage_require_mount "$1"; }
reset_storage
assert_eq "disk found/mounted, no previous state: true" "true" "$(rqm 1)"
assert_eq "no disk, no previous state (single disk): false" "false" "$(rqm 0)"
printf 'SEEDGRID_AGENT_STORAGE_PATH=/data\n' > "$AGENT_ENV"
assert_eq "no disk, previous env WITHOUT the key: false" "false" "$(rqm 0)"
printf 'SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=false\n' > "$AGENT_ENV"
assert_eq "no disk, previous env false: remains false" "false" "$(rqm 0)"
assert_eq "new disk, previous env false: becomes true (legitimate reason)" "true" "$(rqm 1)"
printf 'SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true\n' > "$AGENT_ENV"
assert_eq "2nd pass WITHOUT disk, previous env true: does NOT downgrade" "true" "$(rqm 0)"
assert_eq "2nd pass WITH disk, previous env true: remains true" "true" "$(rqm 1)"
rm -f "$AGENT_ENV"
printf 'UUID=dddd-4444 /data ext4 defaults,nofail 0 2\n' > "$FSTAB"
assert_eq "no disk, no env, but /data in fstab (Node pre-#221): does NOT downgrade" "true" "$(rqm 0)"
printf 'UUID=aaaa-1111 / ext4 defaults 0 1\n' > "$FSTAB"
assert_eq "no disk, fstab without /data: false" "false" "$(rqm 0)"
# What the operator declares overrides the inference, in both directions.
printf 'SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true\n' > "$AGENT_ENV"
assert_eq "STORAGE_REQUIRE_MOUNT=false intentionally downgrades (previous env true)" "false" "$(STORAGE_REQUIRE_MOUNT=false rqm 0)"
rm -f "$AGENT_ENV"
assert_eq "STORAGE_REQUIRE_MOUNT=true declares a disk that has not arrived yet" "true" "$(STORAGE_REQUIRE_MOUNT=true rqm 0)"
assert_eq "STORAGE_REQUIRE_MOUNT empty does not count as a declaration" "false" "$(STORAGE_REQUIRE_MOUNT='' rqm 0)"

# The bootstrap rejects what is not true/false and --storage-only combined with false — before
# touching anything. Running the real script: all of this exits at parse, with no side effects.
STORAGE_REQUIRE_MOUNT=maybe sh "$SCRIPT" >/dev/null 2>&1; assert_eq "STORAGE_REQUIRE_MOUNT=maybe: exits 2" "2" "$?"
STORAGE_REQUIRE_MOUNT=false sh "$SCRIPT" --storage-only >/dev/null 2>&1; assert_eq "--storage-only with STORAGE_REQUIRE_MOUNT=false: exits 2" "2" "$?"
help=$(sh "$SCRIPT" --help)
assert_eq "--help documents STORAGE_REQUIRE_MOUNT" "1" "$(printf '%s\n' "$help" | grep -c '^  STORAGE_REQUIRE_MOUNT=true')"
assert_eq "--help ends at the end of the header (does not leak script code)" "0" "$(printf '%s\n' "$help" | grep -c 'set -eu')"
assert_eq "--help shows the last paragraph of the header" "1" "$(printf '%s\n' "$help" | grep -c 're-announce to the trackers')"

# setup_storage: the exit code is what the bootstrap uses to declare the storage.
stub mountpoint '[ "${FAKE_MOUNTED:-0}" = 1 ]'
stub df 'echo "Filesystem Size Used Avail"; echo "x 100G 1G 99G"'
stub findmnt 'echo /dev/mmcblk0p2'
stub lsblk '
case "$*" in
  "-no pkname /dev/mmcblk0p2") echo mmcblk0 ;;
  "-no pkname /dev/sda1") echo sda ;;
  "-rpno NAME,TYPE") echo "/dev/mmcblk0p2 part"; [ "${FAKE_USB:-0}" != 1 ] || echo "/dev/sda1 part" ;;
  "-dno tran /dev/sda") echo usb ;;
  "-bdno size /dev/sda1") echo 1000 ;;
  "-no fstype /dev/sda1") printf "%s\n" "${FAKE_FSTYPE-ext4}" ;;
esac
exit 0'
stub blkid 'echo 1111-aaaa'
stub mount 'exit "${FAKE_MOUNT_RC:-0}"'
# setup_storage runs `mkdir -p /data`: in a test, as root, this WOULD CREATE /data on the host. A stub
# in PATH only logs the call; it steps aside right after the setup_storage tests, because the
# rest of the test needs the real mkdir. `hash -r` is essential: the shell caches where it found
# mkdir the first time it used it (up at the top), and without clearing this cache the stub is never
# called. (A function named mkdir would also work, but shellcheck rejects it due to SC2218, since
# there are calls to mkdir before it in the file.)
MKDIR_LOG="$WORK/mkdir.log"; : > "$MKDIR_LOG"
DATA_EXISTED=$(exists /data)
stub mkdir 'echo "$*" >> "$MKDIR_LOG"'
hash -r
export MKDIR_LOG
FAKE_MOUNTED=0 FAKE_USB=0 FAKE_FSTYPE=ext4 FAKE_MOUNT_RC=0
export FAKE_MOUNTED FAKE_USB FAKE_FSTYPE FAKE_MOUNT_RC

reset_storage; FAKE_MOUNTED=1 FAKE_USB=0
setup_storage >/dev/null; assert_eq "setup_storage: /data already mounted → 0 (dedicated disk)" "0" "$?"
assert_eq "setup_storage: /data already mounted does not write to fstab" "no" "$(exists "$FSTAB")"
FAKE_MOUNTED=0 FAKE_USB=0
out=$(setup_storage); rc=$?
assert_eq "setup_storage: no disk → 1 (system disk)" "1" "$rc"
assert_eq "setup_storage: no disk does not write to fstab" "no" "$(exists "$FSTAB")"
FAKE_USB=1 FAKE_FSTYPE=
out=$(setup_storage); rc=$?
assert_eq "setup_storage: disk without filesystem and without STORAGE_FORMAT → 1" "1" "$rc"
FAKE_USB=1 FAKE_FSTYPE=ext4 FAKE_MOUNT_RC=0
out=$(setup_storage); rc=$?
assert_eq "setup_storage: USB disk with ext4 → 0" "0" "$rc"
assert_eq "setup_storage: writes the mount by UUID to fstab" "1" "$(grep -c '^UUID=1111-aaaa /data ext4 defaults,noatime,nofail' "$FSTAB")"
out=$(setup_storage); rc=$?
assert_eq "setup_storage: 2nd pass → 0" "0" "$rc"
assert_eq "setup_storage: 2nd pass does not duplicate the fstab line" "1" "$(grep -c '/data' "$FSTAB")"
FAKE_MOUNT_RC=1
out=$(setup_storage); rc=$?
assert_eq "setup_storage: disk that failed to mount remains a dedicated disk (0)" "0" "$rc"
assert_eq "setup_storage: disk that failed to mount warns instead of saying 'mounted'" "1" "$(printf '%s\n' "$out" | grep -c 'failed to mount')"
assert_eq "setup_storage: disk that failed to mount does not say it mounted" "0" "$(printf '%s\n' "$out" | grep -c ' mounted on /data')"
assert_eq "setup_storage never called mkdir outside /data" "0" "$(grep -vc '^-p /data$' "$MKDIR_LOG")"
assert_eq "the mkdir stub was called (hash -r worked)" "yes" "$([ "$(grep -c '^-p /data$' "$MKDIR_LOG")" -ge 1 ] && echo yes || echo no)"
assert_eq "the setup_storage tests did not create /data on the host" "$DATA_EXISTED" "$(exists /data)"

# The step 4 bootstrap flow, end to end: setup_storage → DATA_DEDICATED → value.
decide() { if setup_storage >/dev/null; then d=1; else d=0; fi; storage_require_mount "$d"; }
reset_storage; FAKE_MOUNTED=0 FAKE_USB=0 FAKE_MOUNT_RC=0
assert_eq "installation without disk → false" "false" "$(decide)"
FAKE_USB=1
assert_eq "installation with disk → true" "true" "$(decide)"
# The same machine on a 2nd pass: the env was written with true and the disk is now gone.
export REQUIRE_MOUNT=true CP_URL=http://cp JOIN_TOKEN=tok QBT_API=http://127.0.0.1:8080 QBT_PASS=senha BT_PORT=6881 ENGINE_CONTAINER=seedgrid-engine
write_agent_env
FAKE_USB=0
assert_eq "2nd pass without the disk (previous fstab and env): remains true" "true" "$(decide)"
rm -f "$FSTAB"
assert_eq "2nd pass without the disk, only the previous env: remains true" "true" "$(decide)"
rm -f "$AGENT_ENV"
assert_eq "new single-disk machine: false" "false" "$(decide)"
rm -f "$STUBS/mkdir"
hash -r
assert_eq "the decision flow did not create /data on the host either" "$DATA_EXISTED" "$(exists /data)"

# announce_storage_mode: the single disk warning says what it is and how to attach a disk later.
out=$(announce_storage_mode 0 false)
assert_eq "single-disk warning: says that storage is the system disk" "1" "$(printf '%s\n' "$out" | grep -c 'SYSTEM DISK')"
assert_eq "single-disk warning: says how to attach a disk later" "1" "$(printf '%s\n' "$out" | grep -c -- '--storage-only')"
assert_eq "single-disk warning: says that only the write is checked" "1" "$(printf '%s\n' "$out" | grep -c 'checks only')"
assert_eq "single-disk warning: mentions NODE_DISK_FULL" "1" "$(printf '%s\n' "$out" | grep -c 'NODE_DISK_FULL')"
out=$(announce_storage_mode 1 true)
assert_eq "dedicated disk: no alarm" "0" "$(printf '%s\n' "$out" | grep -c '!!')"
out=$(announce_storage_mode 0 true)
assert_eq "declared with disk but without disk now: warns that the Node stays out of the Scheduler" "1" "$(printf '%s\n' "$out" | grep -c 'stays out of the Scheduler')"
out=$(announce_storage_mode 1 false)
assert_eq "forced false with mounted disk: warns about the risk" "1" "$(printf '%s\n' "$out" | grep -c 'without an alarm')"

# write_agent_env: the new Node's env gets the declaration, on both values, and remains 0600.
for v in true false; do
  rm -f "$AGENT_ENV"
  REQUIRE_MOUNT=$v write_agent_env
  assert_eq "write_agent_env writes ${v}" "1" "$(grep -c "^SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=$v\$" "$AGENT_ENV")"
  assert_eq "write_agent_env ($v): the key appears only once" "1" "$(grep -c '^SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=' "$AGENT_ENV")"
  assert_eq "write_agent_env ($v): o env segue 0600" "-rw-------" "$(ls -l "$AGENT_ENV" | cut -c1-10)"
  assert_eq "write_agent_env ($v): rest of env is there" "1" "$(grep -c '^SEEDGRID_AGENT_STORAGE_PATH=/data$' "$AGENT_ENV")"
done

# declare_storage_dedicated (--storage-only): false → true, without duplicating or touching the rest.
printf 'SEEDGRID_CONTROL_PLANE_URL=http://cp\nSEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=false\nSEEDGRID_AGENT_STORAGE_PATH=/data\n' > "$AGENT_ENV"
chmod 0600 "$AGENT_ENV"
declare_storage_dedicated >/dev/null
assert_eq "--storage-only replaces false with true in place" \
  "SEEDGRID_CONTROL_PLANE_URL=http://cp SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true SEEDGRID_AGENT_STORAGE_PATH=/data" "$(tr '\n' ' ' < "$AGENT_ENV" | sed 's/ $//')"
assert_eq "--storage-only keeps env at 0600" "-rw-------" "$(ls -l "$AGENT_ENV" | cut -c1-10)"
before=$(cat "$AGENT_ENV")
declare_storage_dedicated >/dev/null
assert_eq "--storage-only repeated leaves the env byte-for-byte identical" "$before" "$(cat "$AGENT_ENV")"
printf 'SEEDGRID_CONTROL_PLANE_URL=http://cp\n' > "$AGENT_ENV"
declare_storage_dedicated >/dev/null
assert_eq "--storage-only adds the key when the Node (old) does not have it" \
  "SEEDGRID_CONTROL_PLANE_URL=http://cp SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true" "$(tr '\n' ' ' < "$AGENT_ENV" | sed 's/ $//')"
rm -f "$AGENT_ENV"
out=$(declare_storage_dedicated)
assert_eq "--storage-only without Agent env does not create the file" "no" "$(exists "$AGENT_ENV")"
assert_eq "--storage-only without Agent env warns" "1" "$(printf '%s\n' "$out" | grep -c 'warning')"

# restart_agent: the Agent only re-reads the env upon restart.
reset_storage
restart_agent >/dev/null
assert_eq "restart_agent without the unit does nothing" "" "$(cat "$SYSTEMCTL_LOG")"
assert_eq "restart_agent without the unit does not mark as restarted" "" "${AGENT_RESTARTED:-}"
: > "$AGENT_UNIT"
restart_agent >/dev/null
assert_eq "restart_agent with the unit restarts the Agent" "restart seedgrid-agent" "$(cat "$SYSTEMCTL_LOG")"
assert_eq "restart_agent marks as restarted" "1" "${AGENT_RESTARTED:-}"

# The main flow uses these functions (its body is not extractable — runs as root, on a Node):
# if anyone reverts to `setup_storage || true` or removes the declaration, the test above would pass without
# the bootstrap writing it. Fixes the chain.
assert_eq "step 4 declares storage based on setup_storage result" "1" \
  "$(grep -c '^if setup_storage; then DATA_DEDICATED=1; else DATA_DEDICATED=0; fi$' "$SCRIPT")"
assert_eq "step 4 decides and announces" "1" \
  "$(grep -c '^REQUIRE_MOUNT=\$(storage_require_mount "\$DATA_DEDICATED")$' "$SCRIPT")"
assert_eq "step 7 writes the env via the tested function" "1" "$(grep -c '^write_agent_env$' "$SCRIPT")"
assert_eq "the --storage-only writes true" "1" "$(grep -c '^  declare_storage_dedicated$' "$SCRIPT")"
assert_eq "no leftover setup_storage || true that ignored the result" "0" "$(grep -c 'setup_storage || true' "$SCRIPT")"

# ---------------------------------------------------------------------------
# save_self — the Node saves the copy of the script that the instructions tell it to run (#26).
# ---------------------------------------------------------------------------
load save_self
INSTALL_DIR="$WORK/install"
# shellcheck disable=SC2034 # handled by save_self, loaded from the script
ARTIFACTS="http://controller.example"
stub curl 'out=""; url=""
while [ $# -gt 0 ]; do case "$1" in -o) out="$2"; shift ;; http*) url="$1" ;; esac; shift; done
echo "$url" >> "$CURL_LOG"
[ "${FAKE_CURL_FAIL:-0}" = 1 ] && exit 22
printf "#!/bin/sh\n# copy\n" > "$out"'
CURL_LOG="$WORK/curl.log"; export CURL_LOG; : > "$CURL_LOG"
save_self; rc=$?
assert_eq "save_self: returns 0" "0" "$rc"
assert_eq "save_self: download from /node/ of the Control Plane" "http://controller.example/node/bootstrap-node.sh" "$(cat "$CURL_LOG")"
assert_eq "save_self: writes to the path cited by messages" "1" "$([ -x "$INSTALL_DIR/bootstrap-node.sh" ] && echo 1 || echo 0)"
assert_eq "save_self: leaves no temporary file" "0" "$(find "$INSTALL_DIR" -name "*.tmp" | wc -l | tr -d " ")"
# Download failed: the good copy that already existed remains intact.
printf "good-copy\n" > "$INSTALL_DIR/bootstrap-node.sh"
export FAKE_CURL_FAIL=1
save_self; rc=$?
unset FAKE_CURL_FAIL
assert_eq "save_self with failed download: returns 1" "1" "$rc"
assert_eq "save_self with failed download: keeps the previous copy" "good-copy" "$(cat "$INSTALL_DIR/bootstrap-node.sh")"
assert_eq "save_self with failed download: leaves no temporary file" "0" "$(find "$INSTALL_DIR" -name "*.tmp" | wc -l | tr -d " ")"
assert_eq "onboarding calls save_self after downloading the artifacts" "1" "$(grep -c '^save_self   # ' "$SCRIPT")"
assert_eq "no instruction references the script without a path" "0" "$(grep -c 'sh bootstrap-node.sh --' "$SCRIPT")"
assert_eq "the path in the instructions is the script's INSTALL_DIR" "1" "$(grep -c '^INSTALL_DIR=/opt/seedgrid-node$' "$SCRIPT")"

# ---------------------------------------------------------------------------
# require_root — without root the script stops before touching anything (#21).
# ---------------------------------------------------------------------------
load require_root
stub id 'case "$1" in -u) echo "${FAKE_UID:-0}" ;; -un) echo "${FAKE_USER:-root}" ;; esac'
export FAKE_UID=0
out=$(require_root full 2>&1); rc=$?
assert_eq "require_root: root passes" "0" "$rc"
assert_eq "require_root: root passes silently" "" "$out"
export FAKE_UID=1000 FAKE_USER=ubuntu
out=$(CP_URL=http://controller.example require_root full 2>&1); rc=$?
assert_eq "require_root: regular user is blocked" "1" "$rc"
assert_eq "require_root: says who it is and that nothing changed" "1" "$(printf '%s\n' "$out" | grep -c 'you are ubuntu). Nothing was changed')"
assert_eq "require_root: shows the onboarding command with sudo and the CP URL" "1" \
  "$(printf '%s\n' "$out" | grep -c 'curl -fsSL http://controller.example/node/bootstrap-node.sh | sudo CP_URL=http://controller.example JOIN_TOKEN=')"
assert_eq "require_root: no --network-only shows the command this way" "1" \
  "$(require_root network 2>&1 | grep -c 'sudo sh /opt/seedgrid-node/bootstrap-node.sh --network-only')"
assert_eq "require_root: --storage-only shows the command this way" "1" \
  "$(require_root storage 2>&1 | grep -c 'sudo sh /opt/seedgrid-node/bootstrap-node.sh --storage-only')"
# The entire script, as a regular user: stops with exit 1 before talking to the Control Plane.
# (With `id` stubbed, the result doesn't depend on who runs the test — root on the bench, user in CI.)
out=$(CP_URL=http://controller.example JOIN_TOKEN=x sh "$SCRIPT" 2>&1); rc=$?
assert_eq "script without root: exits 1" "1" "$rc"
assert_eq "script without root: does not reach enroll" "0" "$(printf '%s
' "$out" | grep -c 'enroll no Control Plane')"
# Usage error still exits 2 even without root: validating input comes first.
STORAGE_REQUIRE_MOUNT=maybe CP_URL=http://controller.example JOIN_TOKEN=x sh "$SCRIPT" >/dev/null 2>&1
assert_eq "script without root and with invalid value: exits 2 (usage), not 1" "2" "$?"
unset FAKE_UID FAKE_USER
assert_eq "final message points to the existing screen" "1" "$(grep -c '(Health & Metrics)' "$SCRIPT")"

echo "== test-bootstrap-node: $pass ok, $fail failure(s)"
[ "$fail" -eq 0 ]
