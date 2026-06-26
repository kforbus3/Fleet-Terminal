#!/bin/sh
# Fleet Terminal test fabric — managed host entrypoint.
#
# Brings up a userspace WireGuard spoke (wg0) peered with the jump host,
# then starts sshd in the foreground. Public keys are exchanged through the
# shared /wgkeys volume.
set -e

WG_DIR=/wgkeys
WG_IFACE=wg0
WG_PORT=51820
WG_ADDR="${WG_ADDRESS:-10.100.0.21/24}"
SELF_NAME="${WG_NAME:-managed}"
JUMP_NAME="${JUMP_NAME:-jumphost}"
JUMP_WG_IP="${JUMP_WG_IP:-10.100.0.1}"

mkdir -p "$WG_DIR" /etc/wireguard /run/sshd
umask 077

# Generate a persistent keypair on first boot.
if [ ! -f /etc/wireguard/privatekey ]; then
  wg genkey > /etc/wireguard/privatekey
  wg pubkey < /etc/wireguard/privatekey > /etc/wireguard/publickey
fi

# Publish our public key so the hub can peer with us.
cp /etc/wireguard/publickey "$WG_DIR/${SELF_NAME}.pub"

# Start userspace WireGuard (daemonizes; uses /dev/net/tun).
echo "[${SELF_NAME}] starting wireguard-go on ${WG_IFACE}"
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

# Peer with the jump host once its public key appears.
echo "[${SELF_NAME}] waiting for ${JUMP_NAME}.pub ..."
while [ ! -s "$WG_DIR/${JUMP_NAME}.pub" ]; do sleep 1; done
JUMP_PUB="$(cat "$WG_DIR/${JUMP_NAME}.pub")"
wg set "$WG_IFACE" peer "$JUMP_PUB" \
  allowed-ips "${JUMP_WG_IP}/32" \
  endpoint "${JUMP_NAME}:${WG_PORT}" \
  persistent-keepalive 25
echo "[${SELF_NAME}] peered with ${JUMP_NAME} (${JUMP_WG_IP})"

# Ensure host keys exist, then run sshd in the foreground.
ssh-keygen -A
echo "[${SELF_NAME}] starting sshd"
exec /usr/sbin/sshd -D -e
