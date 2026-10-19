#!/usr/bin/env bash
# ctrl32 WireGuard modem — turns a Raspberry Pi (or any Debian/Ubuntu machine with systemd) in a site's LAN into the
# site's VPN link to a ctrl32 telemetry server, so the server's connectors (Siemens S7, Modbus TCP, OPC UA) reach PLCs
# there. The Pi connects out (UDP to the server's WireGuard port); nothing is opened towards the internet.
#
#   sudo bash install-wg-modem.sh --conf ctrl32.conf --forward 102=192.168.0.7:102
#
# ctrl32.conf is the configuration shown once when the peer was added on the server (System → VPN → Add peer).
#
# Two modes:
#   --forward PORT=HOST:PORT (repeatable; a "device" peer, no licence needed)
#       The server reaches only the listed LAN services, at this Pi's VPN address: the connector of the PLC above uses
#       host <VPN address of the Pi>, port 102. Everything else arriving through the tunnel is dropped; the Pi does
#       not become a router for the LAN.
#   --site (a "site" peer with routed subnets, needs the server's licence)
#       The server's VPN range reaches the LAN subnets routed to this peer (the server's access rules decide what);
#       traffic into the LAN is masqueraded behind the Pi, so LAN devices need no route back.
#
# What it does (idempotent, re-run to change the configuration):
#   1. installs wireguard-tools and nftables (and iproute2 if missing)
#   2. copies the configuration to /etc/wireguard/ctrl32.conf (0600), keeps PersistentKeepalive = 25
#   3. writes the nftables table inet ctrl32_modem (/etc/ctrl32-modem/modem.nft), loaded when the tunnel comes up
#   4. enables IPv4 forwarding and the service wg-quick@ctrl32 (starts at boot, restarts with the tunnel)
#   5. checks: handshake with the server, every forwarded LAN service answers on TCP
#
# Options:
#   --conf FILE            WireGuard configuration from the server (required on the first run)
#   --forward P=HOST:PORT  forward TCP port P of the Pi's VPN address to a LAN service (repeatable)
#   --site                 route the VPN range into the LAN (site peer) instead of forwarding single ports
#   --lan IFACE            LAN interface (default: the interface of the default route, e.g. eth0)
#   --endpoint HOST:PORT   override the server's WireGuard address in the configuration
#   --ssh                  allow SSH to the Pi from the server's VPN range (default: only ping)
#   --check                only run the checks
#   --uninstall            stop the tunnel and remove the service, the table and the configuration
set -euo pipefail

IFACE=ctrl32
WGCONF=/etc/wireguard/$IFACE.conf
DIR=/etc/ctrl32-modem
NFT=$DIR/modem.nft
conf="" site=0 lan="" endpoint="" ssh=0 check=0 uninstall=0
forwards=()

die() { echo "error: $*" >&2; exit 1; }
log() { echo "· $*"; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --conf) conf="${2:-}"; shift 2 ;;
    --forward) forwards+=("${2:-}"); shift 2 ;;
    --site) site=1; shift ;;
    --lan) lan="${2:-}"; shift 2 ;;
    --endpoint) endpoint="${2:-}"; shift 2 ;;
    --ssh) ssh=1; shift ;;
    --check) check=1; shift ;;
    --uninstall) uninstall=1; shift ;;
    -h|--help) sed -n '2,40p' "$0"; exit 0 ;;
    *) die "unknown option $1 (see --help)" ;;
  esac
done
[[ $EUID -eq 0 ]] || die "run as root (sudo)"

if [[ $uninstall -eq 1 ]]; then
  systemctl disable --now "wg-quick@$IFACE" 2>/dev/null || true
  nft delete table inet ctrl32_modem 2>/dev/null || true
  rm -f "$WGCONF" /etc/sysctl.d/90-ctrl32-modem.conf
  rm -rf "$DIR"
  log "removed (wireguard-tools and nftables stay installed)"
  exit 0
fi

# ---- checks -----------------------------------------------------------------------------------------------------
checks() {
  local ok=0 hs now
  [[ -f $WGCONF ]] || die "$WGCONF missing — install with --conf first"
  systemctl is-active --quiet "wg-quick@$IFACE" || die "wg-quick@$IFACE is not running (journalctl -u wg-quick@$IFACE)"
  for _ in $(seq 1 15); do
    hs=$(wg show "$IFACE" latest-handshakes | awk '{print $2}' | sort -n | tail -1)
    now=$(date +%s)
    if [[ -n $hs && $hs -gt 0 && $((now - hs)) -lt 180 ]]; then ok=1; break; fi
    sleep 2
  done
  if [[ $ok -eq 1 ]]; then
    log "tunnel: handshake with the server $((now - hs)) s ago"
  else
    echo "! tunnel: no handshake with the server yet — is outgoing UDP to $(awk -F' *= *' '/^Endpoint/{print $2}' "$WGCONF") allowed in this network?"
  fi
  local f t host port bad=0
  if [[ -f $DIR/forwards ]]; then
    while read -r f; do
      [[ -n $f ]] || continue
      t=${f#*=}; host=${t%:*}; port=${t##*:}
      if timeout 3 bash -c "exec 3<>/dev/tcp/$host/$port" 2>/dev/null; then
        log "LAN service $t answers (VPN port ${f%%=*})"
      else
        echo "! LAN service $t does not answer on TCP — address, port, cable?"; bad=1
      fi
    done < "$DIR/forwards"
  fi
  [[ $ok -eq 1 && $bad -eq 0 ]] && log "ready" || return 1
}
if [[ $check -eq 1 ]]; then checks; exit $?; fi

# ---- install ----------------------------------------------------------------------------------------------------
if [[ -z $conf ]]; then
  [[ -f $WGCONF ]] || die "--conf FILE is required on the first run"
else
  [[ -f $conf ]] || die "$conf not found"
  grep -q '^\[Interface\]' "$conf" && grep -q '^PrivateKey' "$conf" && grep -q '^Endpoint' "$conf" ||
    die "$conf is not a WireGuard configuration from the server (needs [Interface], PrivateKey and Endpoint)"
fi
if [[ $site -eq 0 && ${#forwards[@]} -eq 0 && ! -f $DIR/forwards ]]; then
  die "choose --forward PORT=HOST:PORT (one or more) or --site"
fi
for f in "${forwards[@]}"; do
  [[ $f =~ ^([0-9]{1,5})=([0-9]{1,3}(\.[0-9]{1,3}){3}):([0-9]{1,5})$ ]] || die "--forward $f: use PORT=HOST:PORT, e.g. 102=192.168.0.7:102"
  [[ ${BASH_REMATCH[1]} -ge 1 && ${BASH_REMATCH[1]} -le 65535 && ${BASH_REMATCH[4]} -ge 1 && ${BASH_REMATCH[4]} -le 65535 ]] || die "--forward $f: port out of range"
  [[ ${BASH_REMATCH[1]} -ne 22 || $ssh -eq 0 ]] || die "--forward $f: port 22 is used by --ssh"
done
missing=()
command -v ip >/dev/null || missing+=(iproute2)
command -v wg >/dev/null || missing+=(wireguard-tools)
command -v nft >/dev/null || missing+=(nftables)
if [[ ${#missing[@]} -gt 0 ]]; then
  log "installing ${missing[*]}"
  apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends "${missing[@]}" >/dev/null
fi
if [[ -z $lan ]]; then
  lan=$(ip -4 route show default | awk '{for(i=1;i<NF;i++) if($i=="dev"){print $(i+1); exit}}')
  [[ -n $lan ]] || die "no default route — give the LAN interface with --lan"
fi
ip link show "$lan" >/dev/null 2>&1 || die "interface $lan not found"


install -d -m 0750 "$DIR"
if [[ ${#forwards[@]} -gt 0 ]]; then printf '%s\n' "${forwards[@]}" > "$DIR/forwards"; elif [[ $site -eq 1 ]]; then : > "$DIR/forwards"; fi
echo "$site" > "$DIR/site"; echo "$lan" > "$DIR/lan"; echo "$ssh" > "$DIR/ssh"

if [[ -n $conf ]]; then
  tmp=$(mktemp); trap 'rm -f "$tmp"' EXIT
  # drop PostUp/PostDown of a router template (this script manages the firewall) and keep the keepalive
  grep -v -E '^#?\s*(PostUp|PostDown|Table)\s*=' "$conf" | tr -d '\r' > "$tmp"
  grep -q '^PersistentKeepalive' "$tmp" || sed -i '/^\[Peer\]/a PersistentKeepalive = 25' "$tmp"
  [[ -z $endpoint ]] || sed -i "s|^Endpoint *=.*|Endpoint = $endpoint|" "$tmp"
  install -m 0600 "$tmp" "$WGCONF"
fi
sed -i '/^PostUp *= *nft -f/d;/^PostDown *= *nft delete table inet ctrl32_modem/d' "$WGCONF"
sed -i "/^\[Interface\]/a PostUp = nft -f $NFT\nPostDown = nft delete table inet ctrl32_modem" "$WGCONF"

vpnaddr=$(awk -F' *= *' '/^Address/{print $2}' "$WGCONF" | cut -d, -f1 | cut -d/ -f1)
[[ $vpnaddr =~ ^[0-9.]+$ ]] || die "no IPv4 Address in $WGCONF"

# ---- firewall ---------------------------------------------------------------------------------------------------
{
  echo "# ctrl32 WireGuard modem — generated by install-wg-modem.sh, re-run it to change. Do not edit."
  echo "table inet ctrl32_modem"
  echo "delete table inet ctrl32_modem"
  echo "table inet ctrl32_modem {"
  echo "  chain input {"
  echo "    type filter hook input priority filter - 10; policy accept;"
  echo "    iifname \"$IFACE\" ct state established,related accept"
  echo "    iifname \"$IFACE\" icmp type echo-request accept"
  [[ $ssh -eq 1 ]] && echo "    iifname \"$IFACE\" tcp dport 22 accept"
  echo "    iifname \"$IFACE\" drop comment \"nothing on the Pi itself is reachable through the tunnel\""
  echo "  }"
  echo "  chain prerouting {"
  echo "    type nat hook prerouting priority dstnat; policy accept;"
  if [[ -s $DIR/forwards ]]; then
    while read -r f; do
      [[ -n $f ]] || continue
      echo "    iifname \"$IFACE\" ip daddr $vpnaddr tcp dport ${f%%=*} dnat ip to ${f#*=}"
    done < "$DIR/forwards"
  fi
  echo "  }"
  echo "  chain forward {"
  echo "    type filter hook forward priority filter - 10; policy accept;"
  echo "    ct state established,related accept"
  if [[ $site -eq 1 ]]; then
    echo "    iifname \"$IFACE\" oifname \"$lan\" accept"
  else
    echo "    iifname \"$IFACE\" oifname \"$lan\" ct status dnat accept"
  fi
  echo "    iifname \"$IFACE\" drop"
  echo "    oifname \"$IFACE\" drop comment \"the LAN cannot open connections into the VPN\""
  echo "  }"
  echo "  chain postrouting {"
  echo "    type nat hook postrouting priority srcnat; policy accept;"
  echo "    iifname \"$IFACE\" oifname \"$lan\" masquerade comment \"LAN devices answer through the Pi\""
  echo "  }"
  echo "}"
} > "$NFT.new"
nft -c -f "$NFT.new" || die "generated firewall rejected by nft -c ($NFT.new)"
mv "$NFT.new" "$NFT"

echo net.ipv4.ip_forward=1 > /etc/sysctl.d/90-ctrl32-modem.conf
echo 1 > /proc/sys/net/ipv4/ip_forward  # now; the sysctl.d file keeps it after a reboot

systemctl enable -q "wg-quick@$IFACE"
systemctl restart "wg-quick@$IFACE"
log "tunnel $IFACE up, VPN address $vpnaddr, LAN $lan, mode $([[ $site -eq 1 ]] && echo site || echo forward)"
if [[ $site -eq 0 ]]; then
  while read -r f; do [[ -n $f ]] && log "server → $vpnaddr:${f%%=*} → ${f#*=}"; done < "$DIR/forwards"
fi
checks || { echo "installed, but a check failed — fix it and run: sudo bash $0 --check"; exit 1; }
