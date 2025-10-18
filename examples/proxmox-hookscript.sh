#!/bin/bash
# Proxmox VM Hookscript for pfSense DHCP Static Mapping Updater
#
# INSTALLATION:
# 1. Copy to: /var/lib/vz/snippets/pfsense-dhcp-hook.sh
# 2. Make executable: chmod +x /var/lib/vz/snippets/pfsense-dhcp-hook.sh
# 3. Assign to VM: pvesh set /nodes/$(hostname)/qemu/<VMID>/config \
#                    --hookscript local:snippets/pfsense-dhcp-hook.sh
#
# FEATURES:
# - Automatically creates DHCP mappings when VMs start (pre-start phase)
# - Automatically deletes DHCP mappings when VMs stop (post-stop phase)
# - Full lifecycle management (create on start, delete on stop)
# - Handles missing VM configuration gracefully
# - Does not block VM operations on DHCP failures
# - Comprehensive logging for troubleshooting
#
# ENVIRONMENT:
# - PFSENSE_API_KEY: Set in /etc/environment for system-wide access
#   Example: echo 'export PFSENSE_API_KEY="your-key"' | sudo tee -a /etc/environment
#
# PROXMOX HOOK PHASES:
# - pre-start:  VM about to start (BEFORE qemu process)
# - post-start: VM has started (AFTER qemu process)
# - pre-stop:   VM about to stop (BEFORE shutdown)
# - post-stop:  VM has stopped (AFTER shutdown)

set -e

# Proxmox provides these arguments
VMID=$1
PHASE=$2

# Configuration
UPDATER_BIN="/usr/local/bin/pfsense-dhcp-updater"
VM_CONF="/etc/pve/qemu-server/${VMID}.conf"

# Logging helper
log() {
    local level=$1
    shift
    echo "[$level] [VM $VMID] [${PHASE}] $*" >&2
}

# Check if VM configuration exists
if [ ! -f "$VM_CONF" ]; then
    log "WARN" "VM config not found: $VM_CONF - skipping DHCP update"
    exit 0
fi

# Extract VM details from configuration
HOSTNAME=$(grep "^name:" "$VM_CONF" | awk '{print $2}' | head -n1)
MAC=$(grep "^net0:" "$VM_CONF" | grep -oP '([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}' | head -n1)
VLAN=$(grep "^net0:" "$VM_CONF" | grep -oP 'tag=\K[0-9]+' | head -n1)

# Default VLAN to 0 if not specified
VLAN=${VLAN:-0}

# Validate extracted values
if [ -z "$HOSTNAME" ]; then
    log "WARN" "Hostname not found in VM config - skipping DHCP update"
    exit 0
fi

if [ -z "$MAC" ]; then
    log "WARN" "MAC address not found in VM config - skipping DHCP update"
    exit 0
fi

# Check if updater binary exists
if [ ! -x "$UPDATER_BIN" ]; then
    log "ERROR" "pfSense DHCP updater not found or not executable: $UPDATER_BIN"
    log "INFO" "Install from: https://github.com/antst/pfsense-dhcp-updater/releases"
    exit 0  # Don't block VM operations
fi

# Process based on hook phase
case "$PHASE" in
    pre-start)
        # Create or update DHCP static mapping BEFORE VM starts
        log "INFO" "Creating DHCP mapping for: $HOSTNAME (MAC: $MAC, VLAN: $VLAN)"
        
        # Execute updater
        if "$UPDATER_BIN" "$MAC" "$HOSTNAME" "$VLAN"; then
            log "SUCCESS" "DHCP mapping ready for $HOSTNAME"
            exit 0
        else
            EXIT_CODE=$?
            
            case $EXIT_CODE in
                0)
                    log "SUCCESS" "DHCP mapping already exists (no-op)"
                    exit 0
                    ;;
                2)
                    log "WARN" "Hostname conflict detected for $HOSTNAME - VM will start anyway"
                    # Don't block VM start on hostname conflicts
                    exit 0
                    ;;
                3|5)
                    log "ERROR" "Network/API error (exit code $EXIT_CODE) - VM will start anyway"
                    # Don't block VM start on network issues
                    exit 0
                    ;;
                *)
                    log "ERROR" "DHCP update failed with exit code $EXIT_CODE - VM will start anyway"
                    exit 0
                    ;;
            esac
        fi
        ;;
        
    post-stop)
        # Delete DHCP static mapping AFTER VM stops
        log "INFO" "Removing DHCP mapping for: $HOSTNAME (MAC: $MAC)"
        
        # Execute updater in delete mode
        # Use both MAC and hostname for validation
        if "$UPDATER_BIN" --delete --mac "$MAC" --hostname "$HOSTNAME"; then
            log "SUCCESS" "DHCP mapping removed for $HOSTNAME"
            exit 0
        else
            EXIT_CODE=$?
            
            case $EXIT_CODE in
                0)
                    log "INFO" "DHCP mapping already removed or doesn't exist (no-op)"
                    exit 0
                    ;;
                1)
                    log "WARN" "Validation error during deletion (exit code $EXIT_CODE)"
                    # MAC/hostname mismatch or not found - not critical
                    exit 0
                    ;;
                3|5)
                    log "ERROR" "Network/API error during deletion (exit code $EXIT_CODE)"
                    # Network issues - log but don't block
                    exit 0
                    ;;
                *)
                    log "ERROR" "DHCP deletion failed with exit code $EXIT_CODE"
                    exit 0
                    ;;
            esac
        fi
        ;;
        
    post-start)
        # Optional: Post-start actions
        # Currently not used (DHCP mapping already created in pre-start)
        exit 0
        ;;
        
    pre-stop)
        # Optional: Pre-stop actions
        # Currently not used (DHCP deletion happens in post-stop)
        exit 0
        ;;
        
    *)
        # Unknown phase - ignore
        log "WARN" "Unknown hook phase: $PHASE"
        exit 0
        ;;
esac

# Should never reach here
exit 0
