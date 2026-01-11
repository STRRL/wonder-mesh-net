#!/bin/bash
# Entrypoint script for systemd-enabled Kubernetes worker container

set -e

# Load kernel modules from host (if available)
modprobe br_netfilter 2>/dev/null || true
modprobe overlay 2>/dev/null || true
modprobe ip_tables 2>/dev/null || true
modprobe ip_vs 2>/dev/null || true
modprobe ip_vs_rr 2>/dev/null || true
modprobe ip_vs_wrr 2>/dev/null || true
modprobe ip_vs_sh 2>/dev/null || true
modprobe nf_conntrack 2>/dev/null || true

# Apply sysctl settings
sysctl -w net.bridge.bridge-nf-call-iptables=1 2>/dev/null || true
sysctl -w net.bridge.bridge-nf-call-ip6tables=1 2>/dev/null || true
sysctl -w net.ipv4.ip_forward=1 2>/dev/null || true

# Generate machine-id if missing
if [ ! -s /etc/machine-id ]; then
    systemd-machine-id-setup 2>/dev/null || dbus-uuidgen --ensure=/etc/machine-id
fi

# Ensure dbus machine-id exists
if [ ! -s /var/lib/dbus/machine-id ]; then
    mkdir -p /var/lib/dbus
    cp /etc/machine-id /var/lib/dbus/machine-id 2>/dev/null || true
fi

# Create required directories
mkdir -p /run/systemd/system
mkdir -p /var/log/journal

# Start init (systemd)
exec "$@"
