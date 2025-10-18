// Package pfsense provides pfSense REST API client functionality.
package pfsense

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/antst/pfsense-dhcp-updater/internal/models"
)

// GetDHCPConfig retrieves the DHCP configuration for the specified interface.
// API v2: GET /api/v2/services/dhcp_server?id={interface}
// Note: Query parameters work correctly when Content-Type header is NOT set on GET requests
func (c *Client) GetDHCPConfig(ctx context.Context, interfaceID string) (*models.Interface, error) {
	// Use singular endpoint with query parameter
	endpoint := fmt.Sprintf("/api/v2/services/dhcp_server?id=%s", interfaceID)

	body, err := c.Get(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get DHCP config: %w", err)
	}

	var apiResp struct {
		Status     string                 `json:"status"`
		Code       int                    `json:"code"`
		ResponseID string                 `json:"response_id"`
		Message    string                 `json:"message,omitempty"`
		Data       map[string]interface{} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if apiResp.Status != "ok" {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	dataMap := apiResp.Data

	iface := &models.Interface{
		ID: interfaceID,
	}

	// Parse enable flag
	if enable, ok := dataMap["enable"].(bool); ok {
		iface.DHCPEnabled = enable
	}

	// Get interface details (IP, subnet, etc.) from interfaces endpoint
	// The DHCP server config doesn't include network details
	// Need to fetch full interface data, not just the mapping
	allInterfaces, err := c.getInterfaceDetails(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get interface details: %w", err)
	}

	// Find the interface details by matching logical ID
	for _, ifData := range allInterfaces {
		if id, ok := ifData["id"].(string); ok && id == interfaceID {
			if ipaddr, ok := ifData["ipaddr"].(string); ok && ipaddr != "" {
				ip := net.ParseIP(ipaddr)
				iface.IPAddress = ip

				// Calculate subnet address from IP and mask
				if subnet, ok := ifData["subnet"].(float64); ok {
					iface.SubnetMask = int(subnet)
					// Create CIDR and extract network address
					mask := net.CIDRMask(iface.SubnetMask, 32)
					ipNet := &net.IPNet{IP: ip.To4(), Mask: mask}
					iface.Subnet = ipNet.IP.Mask(mask)
				}
			}
			break
		}
	}

	// Parse DHCP range
	if rangeFrom, ok := dataMap["range_from"].(string); ok && rangeFrom != "" {
		if rangeTo, ok := dataMap["range_to"].(string); ok && rangeTo != "" {
			iface.DHCPPools = []models.DHCPPool{{
				StartIP: net.ParseIP(rangeFrom),
				EndIP:   net.ParseIP(rangeTo),
			}}
		}
	} // Parse static mappings
	// API v2: staticmap array contains objects with parent_id and numeric id
	if staticmap, ok := dataMap["staticmap"].([]interface{}); ok {
		for _, item := range staticmap {
			if mapping, ok := item.(map[string]interface{}); ok {
				sm := models.StaticMapping{
					Interface: interfaceID,
				}

				if mac, ok := mapping["mac"].(string); ok {
					sm.MACAddress = mac
				}
				if ipaddr, ok := mapping["ipaddr"].(string); ok {
					sm.IPAddress = net.ParseIP(ipaddr)
				}
				if hostname, ok := mapping["hostname"].(string); ok {
					sm.Hostname = hostname
				}
				if descr, ok := mapping["descr"].(string); ok {
					sm.Description = descr
				}
				// API v2: id is numeric (float64 from JSON)
				if id, ok := mapping["id"].(float64); ok {
					sm.ID = fmt.Sprintf("%d", int(id))
				} else if idStr, ok := mapping["id"].(string); ok {
					sm.ID = idStr
				}

				iface.StaticMappings = append(iface.StaticMappings, sm)
			}
		}
	}

	// Parse dynamic leases (if available)
	if leases, ok := dataMap["leases"].([]interface{}); ok {
		for _, item := range leases {
			if lease, ok := item.(map[string]interface{}); ok {
				dl := models.DynamicLease{}

				if mac, ok := lease["mac"].(string); ok {
					dl.MACAddress = mac
				}
				if ipaddr, ok := lease["ip"].(string); ok {
					dl.IPAddress = net.ParseIP(ipaddr)
				}
				if hostname, ok := lease["hostname"].(string); ok {
					dl.Hostname = hostname
				}

				iface.DynamicLeases = append(iface.DynamicLeases, dl)
			}
		}
	}

	// Parse DHCP pools
	// API v2: Check both pool array and top-level range_from/range_to
	if pools, ok := dataMap["pool"].([]interface{}); ok && len(pools) > 0 {
		for _, item := range pools {
			if pool, ok := item.(map[string]interface{}); ok {
				dp := models.DHCPPool{}

				if start, ok := pool["range_from"].(string); ok {
					dp.StartIP = net.ParseIP(start)
				}
				if end, ok := pool["range_to"].(string); ok {
					dp.EndIP = net.ParseIP(end)
				}

				iface.DHCPPools = append(iface.DHCPPools, dp)
			}
		}
	} else {
		// API v2: Main DHCP range from top-level fields
		rangeFrom, hasFrom := dataMap["range_from"].(string)
		rangeTo, hasTo := dataMap["range_to"].(string)
		if hasFrom && hasTo && rangeFrom != "" && rangeTo != "" {
			iface.DHCPPools = append(iface.DHCPPools, models.DHCPPool{
				StartIP: net.ParseIP(rangeFrom),
				EndIP:   net.ParseIP(rangeTo),
			})
		}
	}

	// Fetch ALL current DHCP leases from the system
	// This is critical because:
	// 1. The DHCP config endpoint may not include all leases
	// 2. Leases may exist outside the current pool range (e.g., after reconfiguration)
	// 3. We must exclude ALL leased IPs to prevent conflicts
	c.logger.Debug("Fetching all DHCP leases", "interface", interfaceID)
	allLeases, err := c.GetDHCPLeases(ctx)
	if err != nil {
		// Log warning but don't fail - we'll work with whatever leases we have
		c.logger.Warn("Failed to fetch DHCP leases, using only leases from config endpoint",
			"error", err,
			"leases_from_config", len(iface.DynamicLeases),
		)
	} else {
		// Merge leases from config endpoint with leases from status endpoint
		// Use a map to deduplicate by IP address
		leaseMap := make(map[string]models.DynamicLease)

		// Add leases from config endpoint
		for _, lease := range iface.DynamicLeases {
			if lease.IPAddress != nil {
				leaseMap[lease.IPAddress.String()] = lease
			}
		}

		// Add leases from status endpoint (overwrites if duplicate)
		for _, lease := range allLeases {
			if lease.IPAddress != nil {
				leaseMap[lease.IPAddress.String()] = lease
			}
		}

		// Convert map back to slice
		iface.DynamicLeases = make([]models.DynamicLease, 0, len(leaseMap))
		for _, lease := range leaseMap {
			iface.DynamicLeases = append(iface.DynamicLeases, lease)
		}

		c.logger.Info("Merged DHCP leases",
			"total_leases", len(iface.DynamicLeases),
			"from_config", len(iface.DynamicLeases)-len(allLeases),
			"from_status", len(allLeases),
		)
	}

	return iface, nil
}

// CreateStaticMapping creates a new DHCP static mapping.
// API v2: POST /api/v2/services/dhcp_server/static_mapping
func (c *Client) CreateStaticMapping(ctx context.Context, mapping *models.StaticMapping) error {
	if mapping == nil {
		return fmt.Errorf("mapping is required")
	}

	if err := mapping.Validate(); err != nil {
		return fmt.Errorf("invalid mapping: %w", err)
	}

	endpoint := "/api/v2/services/dhcp_server/static_mapping"

	// Build request payload
	// API v2: requires parent_id (interface name as string)
	payload := map[string]interface{}{
		"parent_id": mapping.Interface, // Interface name (e.g., "lan", "opt1")
		"mac":       mapping.MACAddress,
		"ipaddr":    mapping.IPAddress.String(),
		"hostname":  mapping.Hostname,
	}

	if mapping.Description != "" {
		payload["descr"] = mapping.Description
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	body, err := c.Post(ctx, endpoint, jsonData)
	if err != nil {
		return fmt.Errorf("failed to create static mapping: %w", err)
	}

	var apiResp struct {
		Status     string `json:"status"`
		Code       int    `json:"code"`
		ResponseID string `json:"response_id"`
		Message    string `json:"message,omitempty"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if apiResp.Status != "ok" {
		return fmt.Errorf("API error: %s", apiResp.Message)
	}

	return nil
}

// CreateStaticMappingWithRetry creates a DHCP static mapping with retry logic for concurrent IP conflicts.
// If the IP assignment fails due to a conflict (e.g., another process assigned the same IP concurrently),
// this function will retry up to maxAttempts times, selecting the next available IP from the pool on each retry.
//
// This handles the race condition where multiple VMs start simultaneously and compete for IP addresses.
//
// Parameters:
//   - ctx: Context for request cancellation and timeouts
//   - mapping: The static mapping to create (IP address may be modified on retry)
//   - pool: IP pool to select alternative IPs from on conflict
//   - maxAttempts: Maximum number of attempts (typically 3)
//
// Returns:
//   - error: nil on success, or an error if all attempts fail
//
// Example:
//
//	pool := models.CalculateIPPool(iface, configRange)
//	mapping := &models.StaticMapping{...}
//	err := client.CreateStaticMappingWithRetry(ctx, mapping, pool, 3)
func (c *Client) CreateStaticMappingWithRetry(ctx context.Context, mapping *models.StaticMapping, pool *models.IPPool, maxAttempts int) error {
	if mapping == nil {
		return fmt.Errorf("mapping is required")
	}
	if pool == nil {
		return fmt.Errorf("IP pool is required")
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	attemptedIPs := make(map[string]bool)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Track attempted IP to avoid retrying with the same one
		attemptedIPs[mapping.IPAddress.String()] = true

		c.logger.Info("Attempting to create static mapping",
			"attempt", attempt,
			"max_attempts", maxAttempts,
			"mac", mapping.MACAddress,
			"ip", mapping.IPAddress.String(),
		)

		// Attempt to create the mapping
		err := c.CreateStaticMapping(ctx, mapping)
		if err == nil {
			// Success!
			if attempt > 1 {
				c.logger.Info("Static mapping created successfully after retry",
					"attempts", attempt,
					"final_ip", mapping.IPAddress.String(),
				)
			}
			return nil
		}

		lastErr = err

		// Check if this is an IP conflict error that we can retry
		// Common pfSense error messages for IP conflicts:
		// - "IP address already in use"
		// - "A static mapping with this IP address already exists"
		// - "ipaddr is already in use"
		isIPConflict := containsAny(err.Error(), []string{
			"ip address already in use",
			"ip already in use",
			"ipaddr is already in use",
			"static mapping with this ip",
			"duplicate ip",
		})

		if !isIPConflict {
			// Not an IP conflict - no point retrying with a different IP
			c.logger.Warn("Static mapping creation failed (non-conflict error)",
				"error", err,
				"attempt", attempt,
			)
			return fmt.Errorf("failed to create static mapping: %w", err)
		}

		// IP conflict detected - try to get next available IP
		if attempt >= maxAttempts {
			c.logger.Error("Static mapping creation failed after all retry attempts",
				"attempts", maxAttempts,
				"last_error", err,
			)
			return fmt.Errorf("failed to create static mapping after %d attempts (IP conflicts): %w", maxAttempts, lastErr)
		}

		c.logger.Warn("IP conflict detected, retrying with next available IP",
			"conflicted_ip", mapping.IPAddress.String(),
			"attempt", attempt,
			"max_attempts", maxAttempts,
		)

		// Find next available IP that we haven't tried yet
		nextIP, err := pool.GetNextAvailableIPExcluding(attemptedIPs)
		if err != nil {
			return fmt.Errorf("no more available IPs to retry with: %w", err)
		}

		// Update mapping with new IP for next attempt
		mapping.IPAddress = nextIP
		c.logger.Info("Selected next available IP for retry", "ip", nextIP.String())
	}

	return fmt.Errorf("failed to create static mapping after %d attempts: %w", maxAttempts, lastErr)
}

// UpdateStaticMapping updates an existing DHCP static mapping in pfSense.
// This is typically used when a VM is renamed but keeps the same MAC address,
// allowing the hostname to be updated while preserving the existing IP address.
//
// The function uses the pfSense API v2 PATCH endpoint to modify the mapping in place.
// The IP address is preserved during hostname updates unless explicitly changed.
//
// API v2: PATCH /api/v2/services/dhcp_server/static_mapping?parent_id={parent_id}&id={id}
//
// Parameters:
//   - ctx: Context for request cancellation and timeouts
//   - mapping: The static mapping to update. Must include ID, Interface, MACAddress, IPAddress, and Hostname.
//
// Returns:
//   - error: nil on success, or an error describing what went wrong
//
// Example:
//
//	// Update hostname for existing mapping
//	mapping := &models.StaticMapping{
//	    ID:          "59",
//	    Interface:   "lan",
//	    MACAddress:  "ff:ee:dd:cc:bb:00",
//	    IPAddress:   netip.MustParseAddr("192.168.2.181"),
//	    Hostname:    "new-hostname",
//	    Description: "Updated by automation",
//	}
//	err := client.UpdateStaticMapping(ctx, mapping)
//
// Note: After updating, you must call ApplyDHCPChanges() to apply the configuration.
func (c *Client) UpdateStaticMapping(ctx context.Context, mapping *models.StaticMapping) error {
	if mapping == nil {
		return fmt.Errorf("mapping is required")
	}

	if mapping.ID == "" {
		return fmt.Errorf("mapping ID is required for update")
	}

	if err := mapping.Validate(); err != nil {
		return fmt.Errorf("invalid mapping: %w", err)
	}

	// API v2: uses query parameters for parent_id and id
	endpoint := fmt.Sprintf("/api/v2/services/dhcp_server/static_mapping?parent_id=%s&id=%s",
		mapping.Interface, mapping.ID)

	// Build request payload
	// API v2: also requires parent_id and id in body
	payload := map[string]interface{}{
		"parent_id": mapping.Interface,
		"id":        mapping.ID, // Will be converted to integer by API
		"mac":       mapping.MACAddress,
		"ipaddr":    mapping.IPAddress.String(),
		"hostname":  mapping.Hostname,
	}

	if mapping.Description != "" {
		payload["descr"] = mapping.Description
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	body, err := c.Patch(ctx, endpoint, jsonData)
	if err != nil {
		return fmt.Errorf("failed to update static mapping: %w", err)
	}

	var apiResp struct {
		Status     string `json:"status"`
		Code       int    `json:"code"`
		ResponseID string `json:"response_id"`
		Message    string `json:"message,omitempty"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if apiResp.Status != "ok" {
		return fmt.Errorf("API error: %s", apiResp.Message)
	}

	return nil
}

// DeleteStaticMapping deletes an existing DHCP static mapping from pfSense.
// This is typically used when a VM is permanently deleted or decommissioned,
// freeing up the IP address for reuse and maintaining clean DNS records.
//
// The function uses the pfSense API v2 DELETE endpoint to remove the mapping.
//
// API v2: DELETE /api/v2/services/dhcp_server/static_mapping?parent_id={parent_id}&id={id}
//
// Parameters:
//   - ctx: Context for request cancellation and timeouts
//   - interfaceID: The interface ID (e.g., "lan", "opt1")
//   - mappingID: The static mapping ID to delete
//
// Returns:
//   - error: nil on success, or an error describing what went wrong
//
// Example:
//
//	// Delete a static mapping
//	err := client.DeleteStaticMapping(ctx, "lan", "59")
//
// Note: After deleting, you must call ApplyDHCPChanges() to apply the configuration.
func (c *Client) DeleteStaticMapping(ctx context.Context, interfaceID, mappingID string) error {
	if interfaceID == "" {
		return fmt.Errorf("interface ID is required")
	}
	if mappingID == "" {
		return fmt.Errorf("mapping ID is required")
	}

	// API v2: uses query parameters for parent_id and id
	endpoint := fmt.Sprintf("/api/v2/services/dhcp_server/static_mapping?parent_id=%s&id=%s",
		interfaceID, mappingID)

	body, err := c.Delete(ctx, endpoint)
	if err != nil {
		return fmt.Errorf("failed to delete static mapping: %w", err)
	}

	var apiResp struct {
		Status     string `json:"status"`
		Code       int    `json:"code"`
		ResponseID string `json:"response_id"`
		Message    string `json:"message,omitempty"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if apiResp.Status != "ok" {
		return fmt.Errorf("API error: %s", apiResp.Message)
	}

	c.logger.Info("Static mapping deleted",
		"interface", interfaceID,
		"id", mappingID,
	)

	return nil
}

// FindMappingByMAC finds a static mapping by MAC address within an interface.
// Returns the mapping if found, or nil if not found.
func FindMappingByMAC(iface *models.Interface, macAddress string) *models.StaticMapping {
	for _, mapping := range iface.StaticMappings {
		if mapping.MACAddress == macAddress {
			// Return a copy of the mapping
			mappingCopy := mapping
			return &mappingCopy
		}
	}
	return nil
}

// FindMappingByHostname finds a static mapping by hostname within an interface.
// Hostname comparison is case-insensitive.
// Returns the mapping if found, or nil if not found.
func FindMappingByHostname(iface *models.Interface, hostname string) *models.StaticMapping {
	hostnameNormalized := NormalizeHostname(hostname)

	for _, mapping := range iface.StaticMappings {
		mappingHostnameNormalized := NormalizeHostname(mapping.Hostname)
		if mappingHostnameNormalized == hostnameNormalized {
			// Return a copy of the mapping
			mappingCopy := mapping
			return &mappingCopy
		}
	}
	return nil
}

// ValidateDualIdentifiers validates that both MAC address and hostname reference the same mapping.
// This is used when both identifiers are provided for deletion to ensure they are consistent.
// Returns an error if:
//   - MAC address is not found
//   - Hostname is not found
//   - MAC and hostname reference different mappings
//
// Returns nil if both identifiers reference the same mapping.
func ValidateDualIdentifiers(iface *models.Interface, macAddress, hostname string) error {
	macMapping := FindMappingByMAC(iface, macAddress)
	if macMapping == nil {
		return fmt.Errorf("MAC address %s not found", macAddress)
	}

	hostnameMapping := FindMappingByHostname(iface, hostname)
	if hostnameMapping == nil {
		return fmt.Errorf("hostname '%s' not found", hostname)
	}

	// Check if they reference the same mapping by comparing IDs
	if macMapping.ID != hostnameMapping.ID {
		return fmt.Errorf("MAC address %s is associated with hostname '%s', not '%s'",
			macAddress, macMapping.Hostname, hostname)
	}

	return nil
}

// ApplyDHCPChanges applies pending DHCP server configuration changes.
// API v2: POST /api/v2/services/dhcp_server/apply
// This must be called after creating or updating DHCP static mappings for changes to take effect.
func (c *Client) ApplyDHCPChanges(ctx context.Context) error {
	endpoint := "/api/v2/services/dhcp_server/apply"

	// POST with empty body
	body, err := c.Post(ctx, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to apply DHCP changes: %w", err)
	}

	var apiResp struct {
		Status     string `json:"status"`
		Code       int    `json:"code"`
		ResponseID string `json:"response_id"`
		Message    string `json:"message,omitempty"`
		Data       struct {
			Applied bool `json:"applied"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if apiResp.Status != "ok" {
		return fmt.Errorf("API error: %s", apiResp.Message)
	}

	c.logger.Debug("DHCP changes applied", "applied", apiResp.Data.Applied)

	return nil
}

// CheckPendingDHCPChanges checks if there are pending DHCP configuration changes.
// API v2: GET /api/v2/services/dhcp_server/apply
// Returns true if changes have been applied (no pending changes), false if changes are pending.
func (c *Client) CheckPendingDHCPChanges(ctx context.Context) (bool, error) {
	endpoint := "/api/v2/services/dhcp_server/apply"

	body, err := c.Get(ctx, endpoint)
	if err != nil {
		return false, fmt.Errorf("failed to check pending changes: %w", err)
	}

	var apiResp struct {
		Status     string `json:"status"`
		Code       int    `json:"code"`
		ResponseID string `json:"response_id"`
		Message    string `json:"message,omitempty"`
		Data       struct {
			Applied bool `json:"applied"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return false, fmt.Errorf("failed to decode response: %w", err)
	}

	if apiResp.Status != "ok" {
		return false, fmt.Errorf("API error: %s", apiResp.Message)
	}

	// applied=true means no pending changes
	// applied=false means changes are pending
	return apiResp.Data.Applied, nil
}

// GetDHCPLeases retrieves ALL current DHCP leases from pfSense.
// API v2: GET /api/v2/status/dhcp_server/leases
//
// This is critical for preventing IP conflicts because:
// 1. Leases may exist outside the current DHCP pool range (e.g., after pool reconfiguration)
// 2. The DHCP server config endpoint may not always include lease data
// 3. We must exclude ALL leased IPs, not just those in the current pool
//
// Example scenario requiring this:
// - Original pool: 192.168.1.100-200
// - VM gets lease: 192.168.1.150
// - Admin changes pool to: 192.168.1.50-100 (shrinks range)
// - Lease 192.168.1.150 still exists but is now OUTSIDE the pool
// - Without this check, we might assign static IP 192.168.1.150 → CONFLICT!
func (c *Client) GetDHCPLeases(ctx context.Context) ([]models.DynamicLease, error) {
	endpoint := "/api/v2/status/dhcp_server/leases"

	c.logger.Debug("Fetching DHCP leases", "endpoint", endpoint)

	body, err := c.Get(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get DHCP leases: %w", err)
	}

	var apiResp struct {
		Status     string                   `json:"status"`
		Code       int                      `json:"code"`
		ResponseID string                   `json:"response_id"`
		Message    string                   `json:"message,omitempty"`
		Data       []map[string]interface{} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if apiResp.Status != "ok" {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	var leases []models.DynamicLease
	for _, item := range apiResp.Data {
		lease := models.DynamicLease{}

		// Parse MAC address
		if mac, ok := item["mac"].(string); ok {
			lease.MACAddress = mac
		}

		// Parse IP address (field might be "ip" or "ipaddr")
		if ip, ok := item["ip"].(string); ok && ip != "" {
			lease.IPAddress = net.ParseIP(ip)
		} else if ipaddr, ok := item["ipaddr"].(string); ok && ipaddr != "" {
			lease.IPAddress = net.ParseIP(ipaddr)
		}

		// Parse hostname
		if hostname, ok := item["hostname"].(string); ok {
			lease.Hostname = hostname
		}

		// Only add leases with valid IP addresses
		if lease.IPAddress != nil {
			leases = append(leases, lease)
		}
	}

	c.logger.Info("DHCP leases retrieved",
		"count", len(leases),
	)

	return leases, nil
}

// CheckHostnameConflict checks if the hostname is already in use by a different MAC address.
// Returns an error if a conflict is detected.
// Returns nil if:
// - No mapping exists with this hostname
// - A mapping exists with the same hostname AND same MAC (idempotent case)
func CheckHostnameConflict(iface *models.Interface, hostname, macAddress string) error {
	// Normalize hostname for case-insensitive comparison
	hostnameNormalized := normalizeHostname(hostname)

	for _, mapping := range iface.StaticMappings {
		mappingHostnameNormalized := normalizeHostname(mapping.Hostname)

		// Check if hostname matches (case-insensitive)
		if mappingHostnameNormalized == hostnameNormalized {
			// If MAC addresses are different, it's a conflict
			if mapping.MACAddress != macAddress {
				return fmt.Errorf("hostname '%s' is already in use by MAC address %s (IP: %s)",
					hostname, mapping.MACAddress, mapping.IPAddress)
			}
			// Same hostname with same MAC - idempotent, no conflict
			return nil
		}
	}

	// No conflict found
	return nil
}

// NormalizeHostname normalizes a hostname to lowercase for comparison.
// This function is exported for use in tests and other packages.
func NormalizeHostname(hostname string) string {
	// Use strings package for lowercase conversion
	result := ""
	for _, ch := range hostname {
		if ch >= 'A' && ch <= 'Z' {
			result += string(ch + 32) // Convert to lowercase
		} else {
			result += string(ch)
		}
	}
	return result
}

// normalizeHostname is a deprecated alias for NormalizeHostname.
// Use NormalizeHostname instead.
func normalizeHostname(hostname string) string {
	return NormalizeHostname(hostname)
}

// containsAny checks if a string contains any of the given substrings (case-insensitive).
func containsAny(s string, substrings []string) bool {
	lowerS := strings.ToLower(s)
	for _, substr := range substrings {
		if strings.Contains(lowerS, strings.ToLower(substr)) {
			return true
		}
	}
	return false
}
