# Configuration Examples

This directory contains example configuration files for various deployment scenarios.

## Files

### Basic Configurations

- **`config-basic.yaml`**: Single interface (no VLANs)
  - Minimal configuration for simple networks
  - Single VLAN 0 (base interface)
  - Good starting point for testing

- **`config-multi-vlan.yaml`**: Multiple VLAN interfaces
  - Production multi-VLAN setup example
  - Shows VLAN 0, 10, 20, 30, 101
  - Includes optional manual interface mapping

- **`config-environment-vars.yaml`**: Environment variable usage
  - **RECOMMENDED for production**
  - All sensitive values from environment variables
  - Includes example /etc/environment entries
  - Most secure approach

- **`config-production.yaml`**: Production best practices
  - Security-hardened configuration
  - Proper timeouts and logging
  - Comprehensive security notes
  - Use as template for production deployments

### Hookscripts

- **`proxmox-hookscript.sh`**: Proxmox VM lifecycle integration
  - Automatically creates DHCP mappings on VM start (pre-start)
  - Automatically deletes DHCP mappings on VM stop (post-stop)
  - Full lifecycle management
  - Error handling and logging
  - Ready for production use

## Quick Start

### 1. Choose Configuration Template

```bash
# Copy appropriate template to config directory
mkdir -p ~/.config/pfsense-dhcp-updater

# For basic setup (single VLAN)
cp examples/config-basic.yaml ~/.config/pfsense-dhcp-updater/config.yaml

# For multi-VLAN production
cp examples/config-production.yaml ~/.config/pfsense-dhcp-updater/config.yaml
```

### 2. Customize Configuration

Edit the copied file:
- Replace `pfsense.example.com` with your pfSense hostname/IP
- Update `base_interface` to match your physical interface (check `Interfaces → Assignments` in pfSense)
- Define `vlan_ip_ranges` for your network topology
- Adjust timeouts if needed

### 3. Set Environment Variables

```bash
# Set API key (REQUIRED)
export PFSENSE_API_KEY="your-api-key-from-pfsense"

# For system-wide (Proxmox hookscripts)
echo 'export PFSENSE_API_KEY="your-api-key"' | sudo tee -a /etc/environment
```

### 4. Test Configuration

```bash
# Dry-run test
pfsense-dhcp-updater --dry-run 00:11:22:33:44:55 test-vm 10

# Verify no errors, API connection successful
```

### 5. Install Proxmox Hookscript (Optional)

```bash
# Copy hookscript
sudo cp examples/proxmox-hookscript.sh /var/lib/vz/snippets/pfsense-dhcp-hook.sh
sudo chmod +x /var/lib/vz/snippets/pfsense-dhcp-hook.sh

# Assign to VM (replace 100 with your VM ID)
pvesh set /nodes/$(hostname)/qemu/100/config \
  --hookscript local:snippets/pfsense-dhcp-hook.sh
```

## Common Customizations

### Different Physical Interfaces

```yaml
# Intel NIC (common in servers)
base_interface: "igb0"

# Broadcom NIC
base_interface: "bce0"

# VirtIO (virtualized pfSense)
base_interface: "vtnet0"

# SR-IOV Virtual Function
base_interface: "iavf1"
```

### IP Range Formats

```yaml
vlan_ip_ranges:
  10:
    # Range notation (start-end)
    - "10.0.10.100-10.0.10.200"
    
    # CIDR notation (entire subnet)
    - "10.0.10.0/24"
    
    # Multiple ranges (both formats)
    - "10.0.10.50-10.0.10.99"
    - "10.0.10.150-10.0.10.250"
```

### Manual Interface Mapping

Use if pfSense has custom logical interface names:

```yaml
pfsense:
  base_interface: "em0"
  interface_mapping:
    "em0": "lan"       # Base interface
    "em0.10": "opt1"   # VLAN 10
    "em0.20": "dmz"    # VLAN 20 (custom name)
```

### Timeouts for Slow Networks

```yaml
timeouts:
  api_request: 10s      # Increase for slow pfSense
  total_execution: 30s  # Increase for complex operations
```

### Debug Logging

```yaml
logging:
  level: "DEBUG"    # Very verbose (troubleshooting only)
  format: "json"    # Structured logs
```

## Security Best Practices

1. **File Permissions**: Protect config file
   ```bash
   chmod 600 ~/.config/pfsense-dhcp-updater/config.yaml
   ```

2. **Environment Variables**: Never hardcode API keys
   ```yaml
   api_key: "${PFSENSE_API_KEY}"  # ✅ Correct
   api_key: "actual-key-value"    # ❌ Insecure
   ```

3. **TLS Verification**: Always verify certificates in production
   ```yaml
   insecure_skip_verify: false    # ✅ Production
   insecure_skip_verify: true     # ❌ Testing only
   ```

4. **Restricted Ranges**: Define explicit IP ranges
   ```yaml
   vlan_ip_ranges:
     10: ["10.0.10.100-10.0.10.250"]  # ✅ Explicit
   # Don't rely on netmask alone in production
   ```

5. **Logging Level**: Use INFO/WARN in production
   ```yaml
   logging:
     level: "INFO"    # ✅ Production
     level: "DEBUG"   # ❌ May expose sensitive data
   ```

## Troubleshooting

### Configuration Not Found

```bash
# Check default location
ls -la ~/.config/pfsense-dhcp-updater/config.yaml

# Or specify path explicitly
pfsense-dhcp-updater --config-file /path/to/config.yaml <args>
```

### Environment Variable Not Expanding

```bash
# Verify variable is set
echo $PFSENSE_API_KEY

# Check syntax (must use ${VAR} not $VAR)
api_key: "${PFSENSE_API_KEY}"  # ✅ Correct
api_key: "$PFSENSE_API_KEY"    # ❌ May not work
```

### Interface Not Found

```bash
# Check physical interface names in pfSense
# Interfaces → Assignments → Network port

# Tool will show error if interface doesn't exist
# Error: Interface em0 not found in pfSense
```

## Support

- **Main README**: [../README.md](../README.md)
- **Quickstart Guide**: [../specs/001-pfsense-dhcp-updater/quickstart.md](../specs/001-pfsense-dhcp-updater/quickstart.md)
- **GitHub Issues**: https://github.com/antst/pfsense-dhcp-updater/issues
