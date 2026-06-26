#!/bin/sh
# Fleet Terminal test fabric — jump host entrypoint.
#
# Brings up the userspace WireGuard hub (wg0) with a stable keypair, then starts
# sshd. Managed-host *peers are NOT configured here* — the Fleet Terminal
# enrollment flow adds each peer dynamically (wg set) when a host is enrolled.
set -e

WG_IFACE="${WG_INTERFACE:-wg0}"
WG_PORT="${WG_PORT:-51820}"
WG_ADDR="${WG_ADDRESS:-10.100.0.1/24}"

mkdir -p /etc/wireguard /run/sshd
umask 077

# Generate a persistent keypair on first boot; the backend reads the public key
# over SSH (/etc/wireguard/publickey) during enrollment.
if [ ! -f /etc/wireguard/privatekey ]; then
  wg genkey > /etc/wireguard/privatekey
  wg pubkey < /etc/wireguard/privatekey > /etc/wireguard/publickey
fi

echo "[jumphost] starting userspace WireGuard hub on ${WG_IFACE}"
wireguard-go "$WG_IFACE"

# Wait for the userspace control socket.
i=0
while [ "$i" -lt 15 ]; do
  if wg show "$WG_IFACE" >/dev/null 2>&1; then break; fi
  i=$((i + 1)); sleep 1
done

wg set "$WG_IFACE" listen-port "$WG_PORT" private-key /etc/wireguard/privatekey
ip address add "$WG_ADDR" dev "$WG_IFACE" 2>/dev/null || true
ip link set "$WG_IFACE" up
echo "[jumphost] wg0 up at ${WG_ADDR}; peers added on demand by enrollment"

# Ensure host keys exist, then run sshd in the foreground.
ssh-keygen -A >/dev/null 2>&1 || true
echo "[jumphost] starting sshd"
exec /usr/sbin/sshd -D -e
