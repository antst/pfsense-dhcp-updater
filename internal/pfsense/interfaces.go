// Package pfsense provides pfSense REST API client functionality.
package pfsense

import (
	"context"
	"encoding/json"
	"fmt"
)

// InterfaceInfo represents a pfSense network interface.
type InterfaceInfo struct {
	ID     string `json:"id"`     // Logical name (e.g., "lan", "opt1")
	If     string `json:"if"`     // Physical interface (e.g., "iavf1", "iavf1.101")
	Descr  string `json:"descr"`  // Description
	Enable bool   `json:"enable"` // Whether interface is enabled
}

// GetInterfaces retrieves all configured interfaces from pfSense.
// Returns a map of physical interface name -> logical interface ID.
func (c *Client) GetInterfaces(ctx context.Context) (map[string]string, error) {
	allInterfaces, err := c.getInterfaceDetails(ctx)
	if err != nil {
		return nil, err
	}

	// Build mapping: physical interface -> logical ID
	mapping := make(map[string]string, len(allInterfaces))
	for _, iface := range allInterfaces {
		if ifName, ok := iface["if"].(string); ok && ifName != "" {
			if id, ok := iface["id"].(string); ok && id != "" {
				mapping[ifName] = id
			}
		}
	}

	c.logger.Debug("Interface mapping retrieved",
		"count", len(mapping),
		"interfaces", mapping,
	)

	return mapping, nil
}

// getInterfaceDetails retrieves full interface details from pfSense.
// Returns the raw data array for further processing.
func (c *Client) getInterfaceDetails(ctx context.Context) ([]map[string]interface{}, error) {
	endpoint := "/api/v2/interfaces"

	body, err := c.Get(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get interfaces: %w", err)
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

	return apiResp.Data, nil
}

// ResolveInterfaceID resolves a physical interface name to its logical ID.
// First checks the provided manual mapping, then falls back to API discovery.
//
// VLAN Interface Naming Convention:
//   - VLAN 0 (base interface): physical interface name only (e.g., "iavf1" → "lan")
//   - VLAN N (tagged interface): physical interface name with VLAN tag suffix (e.g., "iavf1.101" → "opt7")
//
// The physical interface name is constructed as:
//   - Base interface (VLAN 0): baseInterface
//   - Tagged interface (VLAN > 0): baseInterface.vlanTag
//
// Examples:
//   - ResolveInterfaceID(ctx, "iavf1", 0, nil)   → "lan" (base interface)
//   - ResolveInterfaceID(ctx, "iavf1", 101, nil) → "opt7" (VLAN 101)
//   - ResolveInterfaceID(ctx, "iavf1", 10, map[string]string{"iavf1.10": "custom"}) → "custom" (manual mapping)
//
// The function returns an error if the interface is not found in either the manual mapping
// or the pfSense API response. The error includes a list of available interfaces.
func (c *Client) ResolveInterfaceID(ctx context.Context, physicalInterface string, vlanTag int, manualMapping map[string]string) (string, error) {
	// Construct the physical interface name with VLAN tag if needed
	physicalName := physicalInterface
	if vlanTag > 0 {
		physicalName = fmt.Sprintf("%s.%d", physicalInterface, vlanTag)
	}

	// Check manual mapping first
	if manualMapping != nil {
		if logicalID, ok := manualMapping[physicalName]; ok {
			c.logger.Debug("Using manual interface mapping",
				"physical", physicalName,
				"logical", logicalID,
			)
			return logicalID, nil
		}
	}

	// Fall back to API discovery
	c.logger.Debug("Auto-discovering interface mapping via API",
		"physical", physicalName,
	)

	interfaceMap, err := c.GetInterfaces(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to discover interfaces: %w", err)
	}

	logicalID, ok := interfaceMap[physicalName]
	if !ok {
		return "", fmt.Errorf("interface %s not found in pfSense (available: %v)", physicalName, interfaceMap)
	}

	c.logger.Info("Interface resolved",
		"physical", physicalName,
		"logical", logicalID,
	)

	return logicalID, nil
}
