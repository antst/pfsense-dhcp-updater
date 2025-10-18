# pfSense DHCP Static Mapping Updater

CLI tool that automatically creates, updates, and deletes DHCP static mappings in pfSense from Proxmox pre-start hooks using the pfSense REST API v2.

## Features

- ✅ **Lifecycle Management**: Create, update, and delete DHCP static mappings
- ✅ **Automatic IP Assignment**: Intelligently selects available IPs from configured ranges
- ✅ **Multi-VLAN Support**: Handles base interface and VLAN-tagged interfaces
- ✅ **Interface Auto-Discovery**: Automatically maps physical interfaces to logical IDs
- ✅ **Automatic Change Application**: Applies pending DHCP changes immediately
- ✅ **Idempotent Operations**: Safe to run multiple times without side effects
- ✅ **Hostname Conflict Detection**: Prevents DNS ambiguity by detecting duplicate hostnames
- ✅ **Concurrent IP Conflict Handling**: Automatic retry with next available IP (up to 3 attempts)
- ✅ **Dry-Run Mode**: Test operations without making changes
- ✅ **JSON & Human-Readable Output**: Supports both automation and manual use
- ✅ **Audit Trail Logging**: Comprehensive operation logging for compliance (SEC-008)
- ✅ **Security Warnings**: Config permission validation with actionable recommendations (SEC-009)
- ✅ **Robust Error Handling**: Clear, actionable error messages with specific exit codes
- ✅ **Statically Linked Binary**: Single binary with no runtime dependencies (<10MB)

## API Version

This tool is designed for **pfSense REST API v2**.
## Quick Start

### Prerequisites

**pfSense Setup:**
1. pfSense 2.6.0 or later recommended
2. Install pfSense API package (see https://pfrest.org)
3. Generate API key (System → API → Settings → Generate)
4. Ensure DHCP server enabled on target interfaces


**System Requirements:**
- Linux x86_64
- <50MB disk space
- HTTPS connectivity to pfSense

### Installation

```bash
# Download latest release
VERSION=v1.0.0
wget https://github.com/antst/pfsense-dhcp-updater/releases/download/${VERSION}/pfsense-dhcp-updater-linux-amd64

# Install
sudo mv pfsense-dhcp-updater-linux-amd64 /usr/local/bin/pfsense-dhcp-updater
sudo chmod +x /usr/local/bin/pfsense-dhcp-updater

# Verify
pfsense-dhcp-updater --version
```

**Or build from source:**

```bash
git clone https://github.com/antst/pfsense-dhcp-updater.git
cd pfsense-dhcp-updater
./scripts/build.sh
sudo cp bin/pfsense-dhcp-updater /usr/local/bin/
```

### Configuration

Create configuration directory:

```bash
mkdir -p ~/.config/pfsense-dhcp-updater
```

Create `~/.config/pfsense-dhcp-updater/config.yaml`:

```yaml
# pfSense API Configuration
pfsense:
  # API endpoint (use HTTPS in production)
  endpoint: "https://pfsense.example.com"
  
  # API key (use environment variable for better security)
  api_key: "${PFSENSE_API_KEY}"
  
  # Base physical interface name (e.g., em0, igb0, iavf1, vtnet0)
  base_interface: "em0"
  
  # TLS verification (set to false only for testing with self-signed certs)
  insecure_skip_verify: false
  
  # Optional: Manual interface mapping (overrides auto-discovery)
  # interface_mapping:
  #   "em0": "lan"         # Physical -> Logical
  #   "em0.10": "opt1"     # VLAN -> Logical

# VLAN-specific IP range overrides (optional)
# If not specified, tool uses full interface netmask range minus exclusions
vlan_ip_ranges:
  0:   # Base interface (no VLAN tag)
    - "10.0.0.100-10.0.0.200"
  10:  # VLAN 10
    - "10.0.10.0/24"
  20:  # VLAN 20
    - "10.0.20.50-10.0.20.150"

# Network timeouts (optional, defaults shown)
timeouts:
  api_request: 5s
  total_execution: 10s

# Logging configuration (optional)
logging:
  level: "INFO"     # ERROR, WARN, INFO, DEBUG
  format: "text"    # text or json
```

Set API key as environment variable (recommended):

```bash
# Add to ~/.bashrc or /etc/environment for persistence
export PFSENSE_API_KEY="your-api-key-here"

# Or for system-wide Proxmox hookscripts
echo 'export PFSENSE_API_KEY="your-api-key-here"' | sudo tee -a /etc/environment
```

Verify configuration:

```bash
# Test connectivity with dry-run
pfsense-dhcp-updater --dry-run 00:11:22:33:44:55 test-vm 10

# Expected output should show successful API query and calculated IP
```

### Usage

**Command Syntax:**

```bash
# Create/Update operations
pfsense-dhcp-updater [OPTIONS] <MAC_ADDRESS> <HOSTNAME> [VLAN_TAG]

# Delete operations
pfsense-dhcp-updater --delete [OPTIONS] --mac <MAC_ADDRESS> [--hostname <HOSTNAME>]
pfsense-dhcp-updater --delete [OPTIONS] --hostname <HOSTNAME> [--vlan <VLAN_TAG>]
```

**Options:**

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--config-file` | - | Path to config file | `~/.config/pfsense-dhcp-updater/config.yaml` |
| `--delete` | - | Delete mapping mode | `false` |
| `--mac` | - | MAC address (for deletion by MAC) | - |
| `--hostname` | - | Hostname (for deletion by hostname) | - |
| `--vlan` | - | VLAN tag (for hostname-based deletion scoping) | - |
| `--dry-run` | `-d` | Simulate without making changes | `false` |
| `--json` | `-j` | Output in JSON format | `false` |
| `--verbose` | `-v` | Increase verbosity (`-v` = INFO, `-vv` = DEBUG) | 0 (WARN) |
| `--insecure` | - | Skip TLS certificate verification | `false` |
| `--version` | - | Display version and exit | - |
| `--help` | `-h` | Display help message | - |

**Common Examples:**

```bash
# Create mapping for VM on base interface (VLAN 0)
pfsense-dhcp-updater 00:11:22:33:44:55 web-server-01

# Create mapping for VM on VLAN 10
pfsense-dhcp-updater AA:BB:CC:DD:EE:FF db-server-01 10

# Update hostname for existing MAC
pfsense-dhcp-updater AA:BB:CC:DD:EE:FF db-server-01-renamed 10

# Delete mapping by MAC address
pfsense-dhcp-updater --delete --mac 00:11:22:33:44:55

# Delete mapping by hostname (searches all VLANs)
pfsense-dhcp-updater --delete --hostname web-server-01

# Delete by hostname with VLAN scoping
pfsense-dhcp-updater --delete --hostname db-server-01 --vlan 10

# Delete with validation (both MAC and hostname must match same mapping)
pfsense-dhcp-updater --delete --mac AA:BB:CC:DD:EE:FF --hostname db-server-01

# Dry-run mode (test without changes)
pfsense-dhcp-updater --dry-run 00:11:22:33:44:55 test-vm 10

# JSON output (for automation)
pfsense-dhcp-updater --json 00:11:22:33:44:55 app-server 10

# Verbose logging (troubleshooting)
pfsense-dhcp-updater -v 00:11:22:33:44:55 debug-vm 10  # INFO level
pfsense-dhcp-updater -vv 00:11:22:33:44:55 debug-vm 10  # DEBUG level
```

gt ## Troubleshooting

### Common Issues

**Authentication Failure (HTTP 401):**
```bash
# Verify API key
echo $PFSENSE_API_KEY
# Regenerate API key in pfSense UI if needed
```

**Connection Timeout:**
```bash
# Check connectivity
ping pfsense.example.com
curl -k https://pfsense.example.com/api/v2/

# Increase timeout in config.yaml
timeouts:
  api_request: 10s
```

**TLS Certificate Verification Failure:**
```bash
# Temporary (testing only)
pfsense-dhcp-updater --insecure 00:11:22:33:44:55 test-vm 10

# Production: Install CA certificate
sudo cp /path/to/pfsense-ca.crt /usr/local/share/ca-certificates/
sudo update-ca-certificates
```

**Invalid MAC Address Format:**
```bash
# Use colon or hyphen format
pfsense-dhcp-updater 00:11:22:33:44:55 test-vm 10  # ✅ Correct
pfsense-dhcp-updater 00-11-22-33-44-55 test-vm 10  # ✅ Correct
pfsense-dhcp-updater 0011.2233.4455 test-vm 10     # ❌ Not supported
```

**No Available IPs:**
```bash
# Expand IP range in config.yaml
vlan_ip_ranges:
  10: ["10.0.10.0/23"]  # Expand from /24 to /23
  
# Or remove unused mappings in pfSense UI
```

**DHCP Server Disabled:**
```bash
# Enable DHCP in pfSense UI:
# Services → DHCP Server → Select VLAN → Enable DHCP server
```

### Debug Mode

Enable detailed logging:

```bash
# INFO level
pfsense-dhcp-updater -v 00:11:22:33:44:55 test-vm 10

# DEBUG level (very detailed)
pfsense-dhcp-updater -vv 00:11:22:33:44:55 test-vm 10 2>&1 | tee debug.log
```

## Proxmox Integration

Automatically register and deregister VMs in pfSense DHCP using Proxmox hookscripts:

### Hookscript Installation

Create `/var/lib/vz/snippets/pfsense-dhcp-hook.sh`:

```bash
#!/bin/bash
# Proxmox VM Hookscript for pfSense DHCP Static Mapping Updater
# Automatically creates/deletes DHCP mappings on VM start/stop

set -e

VMID=$1
PHASE=$2

# Query VM configuration
VM_CONF="/etc/pve/qemu-server/${VMID}.conf"
if [ ! -f "$VM_CONF" ]; then
    echo "[WARN] VM config not found: $VM_CONF" >&2
    exit 0
fi

# Extract VM details
HOSTNAME=$(grep "^name:" "$VM_CONF" | awk '{print $2}')
MAC=$(grep "^net0:" "$VM_CONF" | grep -oP '([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}')
VLAN=$(grep "^net0:" "$VM_CONF" | grep -oP 'tag=\K[0-9]+' || echo "0")

# Validate extracted values
if [ -z "$HOSTNAME" ] || [ -z "$MAC" ]; then
    echo "[WARN] Missing hostname or MAC for VM $VMID" >&2
    exit 0
fi

case "$PHASE" in
    pre-start)
        # Create or update DHCP mapping
        echo "[INFO] Creating DHCP mapping for VM $VMID: $HOSTNAME ($MAC) on VLAN $VLAN"
        /usr/local/bin/pfsense-dhcp-updater "$MAC" "$HOSTNAME" "$VLAN"
        EXIT_CODE=$?
        
        if [ $EXIT_CODE -eq 0 ]; then
            echo "[SUCCESS] DHCP mapping ready for $HOSTNAME"
        elif [ $EXIT_CODE -eq 2 ]; then
            echo "[WARN] Hostname conflict for $HOSTNAME - VM start will continue" >&2
            exit 0  # Don't block VM start
        else
            echo "[ERROR] DHCP update failed with exit code $EXIT_CODE" >&2
            exit 0  # Don't block VM start on DHCP failures
        fi
        ;;
        
    post-stop)
        # Delete DHCP mapping
        echo "[INFO] Removing DHCP mapping for VM $VMID: $HOSTNAME ($MAC)"
        /usr/local/bin/pfsense-dhcp-updater --delete --mac "$MAC" --hostname "$HOSTNAME"
        EXIT_CODE=$?
        
        if [ $EXIT_CODE -eq 0 ]; then
            echo "[SUCCESS] DHCP mapping removed for $HOSTNAME"
        else
            echo "[WARN] DHCP deletion returned exit code $EXIT_CODE" >&2
        fi
        ;;
        
    *)
        # Ignore other phases (post-start, pre-stop)
        exit 0
        ;;
esac

exit 0
```

Make executable:

```bash
sudo chmod +x /var/lib/vz/snippets/pfsense-dhcp-hook.sh
```

### Assign to VMs

```bash
# Via CLI (for VM 100)
pvesh set /nodes/$(hostname)/qemu/100/config --hookscript local:snippets/pfsense-dhcp-hook.sh

# Or via Proxmox web UI:
# VM → Options → Hookscript → local:snippets/pfsense-dhcp-hook.sh
```

### Test Hookscript

```bash
# Start VM and check logs
qm start 100
journalctl -u pve-guests --since "1 minute ago" | grep pfsense-dhcp

# Stop VM and verify cleanup
qm stop 100
journalctl -u pve-guests --since "1 minute ago" | grep "Removing DHCP mapping"
```

## Exit Codes

| Code | Meaning | Description |
|------|---------|-------------|
| 0 | Success | Mapping created, updated, deleted, or already exists (noop) |
| 1 | Validation Error | Invalid MAC, hostname, VLAN, or configuration |
| 2 | Hostname Conflict | Hostname already mapped to different MAC |
| 3 | API Error | pfSense API request failed (network, HTTP error) |
| 4 | Configuration Error | Config file missing, malformed, or invalid |
| 5 | Network Timeout | API request exceeded timeout threshold |

**Script Integration Example:**

```bash
#!/bin/bash

pfsense-dhcp-updater "$MAC" "$HOSTNAME" "$VLAN"
EXIT_CODE=$?

case $EXIT_CODE in
    0)
        echo "✅ Success: Mapping operation completed"
        ;;
    2)
        echo "⚠️ Hostname conflict - notify admin"
        # Send alert, log to monitoring system, etc.
        ;;
    3|5)
        echo "⚠️ Network issue - will retry later"
        # Implement retry logic or alert
        ;;
    *)
        echo "❌ Unexpected error: $EXIT_CODE"
        ;;
esac
```

## Development

### Prerequisites

- Go 1.24 or later
- golangci-lint (for linting)
- Make (optional, for convenience commands)

### Build

```bash
# Build static binary
./scripts/build.sh

# Output: bin/pfsense-dhcp-updater (Linux amd64, ~8.8MB)
```

### Testing

```bash
# Run all unit tests
go test -v ./...

# Run specific package tests
go test -v ./internal/models
go test -v ./internal/pfsense

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run contract tests (mock pfSense API)
go test -v ./tests/contract

# Run integration tests (requires live pfSense)
go test -v ./tests/integration -pfense-host https://pfsense.local
```

### Code Quality

```bash
# Run linter
golangci-lint run

# Format code
go fmt ./...
gofmt -s -w .

# Vet code
go vet ./...

# Check test coverage
go test -cover ./...
```

### Project Structure

```
cmd/pfsense-dhcp-updater/  # CLI entry point
internal/                  # Private packages
  ├── config/              # Configuration loading
  ├── models/              # Domain entities
  ├── pfsense/             # pfSense API client
  ├── validator/           # Input validation
  ├── logger/              # Structured logging
  └── errors/              # Error types
tests/                     # Integration and contract tests
scripts/                   # Build and deployment scripts
```

## License

MIT License - see [LICENSE](LICENSE) for details.


## Support

- **Issues**: [GitHub Issues](https://github.com/antst/pfsense-dhcp-updater/issues)
- **Discussions**: [GitHub Discussions](https://github.com/antst/pfsense-dhcp-updater/discussions)
