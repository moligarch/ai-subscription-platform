#!/bin/bash
set -e

# --- Configuration ---
# The physical network interface your Docker network is attached to.
PARENT_IFACE="ens160"

# The name for the new virtual interface on your host.
MACVLAN_IFACE="macvlan0"

# A STATIC IP for your host machine on the macvlan network.
# IMPORTANT: This IP must be on the same subnet as your containers
# but must NOT be used by any other device or container.
HOST_IP="192.168.10.250/24"

# The list of all container IPs defined in your docker-compose.yml file.
CONTAINER_IPS=(
  "192.168.10.210" # app
  "192.168.10.211" # postgres
  "192.168.10.212" # redis
  "192.168.10.213" # prometheus
  "192.168.10.214" # loki
  "192.168.10.215" # grafana
)
# --- End Configuration ---

# Function to set up the routes
setup() {
  echo "--- Setting up macvlan bridge for host access ---"
  
  # 1. Create the virtual interface linked to the parent
  echo "[1/4] Creating virtual interface ${MACVLAN_IFACE}..."
  sudo ip link add ${MACVLAN_IFACE} link ${PARENT_IFACE} type macvlan mode bridge

  # 2. Assign the static IP to the new virtual interface
  echo "[2/4] Assigning IP ${HOST_IP} to ${MACVLAN_IFACE}..."
  sudo ip addr add ${HOST_IP} dev ${MACVLAN_IFACE}

  # 3. Bring the new interface up
  echo "[3/4] Activating interface ${MACVLAN_IFACE}..."
  sudo ip link set ${MACVLAN_IFACE} up

  # 4. Add specific routes for each container via the new interface
  echo "[4/4] Adding routes for containers..."
  for ip in "${CONTAINER_IPS[@]}"; do
    echo "      -> Routing ${ip} via ${MACVLAN_IFACE}"
    sudo ip route replace ${ip}/32 dev ${MACVLAN_IFACE}
  done

  echo "--- Setup complete. Host can now reach containers. ---"
}

# Function to tear down the routes
teardown() {
  echo "--- Tearing down macvlan bridge ---"
  sudo ip link del ${MACVLAN_IFACE} 2>/dev/null || echo "Interface ${MACVLAN_IFACE} already removed."
  echo "--- Teardown complete. ---"
}

# Main script logic
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