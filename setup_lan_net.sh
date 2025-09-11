#!/bin/bash
set -e

PARENT_IFACE="ens160"
MACVLAN_IFACE="macvlan0"
HOST_IP="192.168.10.250/32"

CONTAINER_IPS=(
  "192.168.10.210"
  "192.168.10.211"
  "192.168.10.212"
  "192.168.10.213"
  "192.168.10.214"
  "192.168.10.215"
)

setup() {
  echo "--- Setting up macvlan bridge for host access ---"

  echo "[1/8] Removing existing ${MACVLAN_IFACE} if any..."
  sudo ip link del ${MACVLAN_IFACE} 2>/dev/null || true

  echo "[2/8] Creating virtual interface ${MACVLAN_IFACE}..."
  sudo ip link add ${MACVLAN_IFACE} link ${PARENT_IFACE} type macvlan mode bridge

  echo "[3/8] Assigning IP ${HOST_IP} to ${MACVLAN_IFACE}..."
  sudo ip addr flush dev ${MACVLAN_IFACE} || true
  sudo ip addr add ${HOST_IP} dev ${MACVLAN_IFACE}

  echo "[4/8] Activating interface ${MACVLAN_IFACE}..."
  sudo ip link set ${MACVLAN_IFACE} up

  echo "[5/8] Adding route for subnet 192.168.10.0/24 via ${PARENT_IFACE}..."
  sudo ip route replace 192.168.10.0/24 dev ${PARENT_IFACE}

  echo "[6/8] Adding routes for containers via ${MACVLAN_IFACE}..."
  for ip in "${CONTAINER_IPS[@]}"; do
    echo "     -> Routing ${ip} via ${MACVLAN_IFACE}"
    sudo ip route replace ${ip}/32 dev ${MACVLAN_IFACE}
  done

  echo "[7/8] Waiting for default route via ${MACVLAN_IFACE} to appear (timeout 15s)..."
  WAIT_TIME=15
  PASSED=0
  while [ $PASSED -lt $WAIT_TIME ]; do
    if ip route show default dev ${MACVLAN_IFACE} > /dev/null 2>&1; then
      echo "     -> Default route found. Removing..."
      if sudo ip route del default dev ${MACVLAN_IFACE}; then
        echo "     -> Default route removed."
        break
      else
        echo "     -> Failed to remove default route, retrying..."
      fi
    else
      echo "     -> Default route not found yet. Waiting..."
    fi
    sleep 1
    PASSED=$((PASSED + 1))
  done

  if ip route show default dev ${MACVLAN_IFACE} > /dev/null 2>&1; then
    echo "Warning: default route via ${MACVLAN_IFACE} still exists after timeout."
  else
    echo "No default route via ${MACVLAN_IFACE} exists anymore."
  fi

  echo "[8/8] Setup complete."
}

teardown() {
  echo "--- Tearing down macvlan bridge ---"
  sudo ip link del ${MACVLAN_IFACE} 2>/dev/null || echo "Interface ${MACVLAN_IFACE} already removed."
  echo "--- Teardown complete. ---"
}

case "$1" in
  up)
    setup
    ;;
  down)
    teardown
    ;;
  *)
    echo "Usage: $0 {up|down}"
    exit 1
    ;;
esac

exit 0
