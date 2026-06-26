#!/bin/sh
# Fleet Terminal test fabric — jump host entrypoint.
#
# Brings up the userspace WireGuard hub (wg0) then starts sshd in the
# foreground. WireGuard public keys are exchanged with the managed hosts
# through the shared /wgkeys volume.
set -e

WG_DIR=/wgkeys
WG_IFACE=wg0
WG_PORT=51820
WG_ADDR="${WG_ADDRESS:-10.100.0.1/24}"
SELF_NAME="${WG_NAME:-jumphost}"

# Managed-host spokes: name:wg-ip pairs the hub must peer with.
PEERS="${WG_PEERS:-host-ubuntu:10.100.0.21 host-rocky:10.100.0.22}"

mkdir -p "$WG_DIR" /etc/wireguard /run/sshd
umask 077

# Generate a persistent keypair on first boot.
if [ ! -f /etc/wireguard/privatekey ]; then
  wg genkey > /etc/wireguard/privatekey
  wg pubkey < /etc/wireguard/privatekey > /etc/wireguard/publickey
fi

# Publish our public key so the spokes can discover the hub.
cp /etc/wireguard/publickey "$WG_DIR/${SELF_NAME}.pub"

# Start userspace WireGuard (daemonizes; uses /dev/net/tun).
echo "[jumphost] starting wireguard-go on ${WG_IFACE}"
wireguard-go "$WG_IFACE"

# Wait for the userspace control socket to be ready.
i=0
while [ "$i" -lt 15 ]; do
  if wg show "$WG_IFACE" >/dev/null 2>&1; then break; fi
  i=$((i + 1))
  sleep 1
done

wg set "$WG_IFACE" listen-port "$WG_PORT" private-key /etc/wireguard/privatekey
ip address add "$WG_ADDR" dev "$WG_IFACE"
ip link set "$WG_IFACE" up

# Add every managed-host peer once its public key appears.
for entry in $PEERS; do
  name="${entry%%:*}"
  wgip="${entry##*:}"
  echo "[jumphost] waiting for ${name}.pub ..."
  while [ ! -s "$WG_DIR/${name}.pub" ]; do sleep 1; done
  pub="$(cat "$WG_DIR/${name}.pub")"
  wg set "$WG_IFACE" peer "$pub" \
    allowed-ips "${wgip}/32" \
    endpoint "${name}:${WG_PORT}" \
    persistent-keepalive 25
  echo "[jumphost] peered with ${name} (${wgip})"
done

# Ensure host keys exist, then run sshd in the foreground.
ssh-keygen -A
echo "[jumphost] starting sshd"
exec /usr/sbin/sshd -D -e
