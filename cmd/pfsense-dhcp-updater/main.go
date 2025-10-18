// pfSense DHCP Static Mapping Updater
// Automatically creates and updates DHCP static mappings in pfSense from Proxmox pre-start hooks.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/antst/pfsense-dhcp-updater/internal/config"
	"github.com/antst/pfsense-dhcp-updater/internal/errors"
	"github.com/antst/pfsense-dhcp-updater/internal/logger"
	"github.com/antst/pfsense-dhcp-updater/internal/models"
	"github.com/antst/pfsense-dhcp-updater/internal/pfsense"
	"github.com/antst/pfsense-dhcp-updater/internal/validator"
)

var (
	// Version information (set via ldflags during build)
	Version    = "dev"
	BuildDate  = "unknown"
	CommitHash = "unknown"
)

func main() {
	// Define CLI flags
	configFile := flag.String("config-file", config.DefaultConfigPath(), "Path to configuration file")
	dryRun := flag.Bool("dry-run", false, "Simulate operation without making changes")
	dryRunShort := flag.Bool("d", false, "Alias for --dry-run")
	jsonOutput := flag.Bool("json", false, "Output in JSON format")
	jsonShort := flag.Bool("j", false, "Alias for --json")
	verbose := flag.Int("verbose", 0, "Verbosity level (0=WARN, 1=INFO, 2=DEBUG)")
	verboseShort := flag.Int("v", 0, "Alias for --verbose")
	insecure := flag.Bool("insecure", false, "Skip TLS certificate verification (testing only)")
	version := flag.Bool("version", false, "Show version information and exit")

	// Delete mode flags (US6)
	deleteMode := flag.Bool("delete", false, "Delete existing static mapping")
	macFlag := flag.String("mac", "", "MAC address for deletion (requires --delete)")
	hostnameFlag := flag.String("hostname", "", "Hostname for deletion (requires --delete)")

	// Custom usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "pfSense DHCP Static Mapping Updater v%s\n\n", Version)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  Create/Update: %s [OPTIONS] <MAC_ADDRESS> <HOSTNAME> [VLAN_TAG]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  Delete:        %s [OPTIONS] --delete [--mac <MAC>] [--hostname <NAME>] [VLAN_TAG]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Automatically creates, updates, or deletes DHCP static mappings in pfSense.\n\n")
		fmt.Fprintf(os.Stderr, "Positional Arguments (Create/Update Mode):\n")
		fmt.Fprintf(os.Stderr, "  MAC_ADDRESS  VM's MAC address (format: XX:XX:XX:XX:XX:XX or XX-XX-XX-XX-XX-XX)\n")
		fmt.Fprintf(os.Stderr, "  HOSTNAME     VM's hostname (RFC 1123 compliant)\n")
		fmt.Fprintf(os.Stderr, "  VLAN_TAG     VLAN tag (0-4094, default: 0 for base interface)\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Create mapping on base interface\n")
		fmt.Fprintf(os.Stderr, "  %s 00:11:22:33:44:55 web-server-01\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Create mapping on VLAN 10\n")
		fmt.Fprintf(os.Stderr, "  %s AA:BB:CC:DD:EE:FF db-server-01 10\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Delete mapping by MAC address\n")
		fmt.Fprintf(os.Stderr, "  %s --delete --mac 00:11:22:33:44:55\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Delete mapping by hostname\n")
		fmt.Fprintf(os.Stderr, "  %s --delete --hostname web-server-01\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Delete mapping with validation (both MAC and hostname)\n")
		fmt.Fprintf(os.Stderr, "  %s --delete --mac 00:11:22:33:44:55 --hostname web-server-01 101\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Dry-run with verbose logging\n")
		fmt.Fprintf(os.Stderr, "  %s --dry-run -v 00:11:22:33:44:55 test-vm 10\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # JSON output for automation\n")
		fmt.Fprintf(os.Stderr, "  %s --json 00:11:22:33:44:55 app-server 10\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Exit Codes:\n")
		fmt.Fprintf(os.Stderr, "  0 - Success or noop (mapping already exists)\n")
		fmt.Fprintf(os.Stderr, "  1 - Validation error (invalid input)\n")
		fmt.Fprintf(os.Stderr, "  2 - Hostname conflict (hostname already in use)\n")
		fmt.Fprintf(os.Stderr, "  3 - API error (pfSense communication failure)\n")
		fmt.Fprintf(os.Stderr, "  4 - Configuration error (config file issue)\n")
		fmt.Fprintf(os.Stderr, "  5 - Network timeout\n\n")
		fmt.Fprintf(os.Stderr, "For more information, see: https://github.com/antst/pfsense-dhcp-updater\n")
	}

	flag.Parse()

	// Handle version flag
	if *version {
		fmt.Printf("pfSense DHCP Static Mapping Updater\n")
		fmt.Printf("Version:    %s\n", Version)
		fmt.Printf("Build Date: %s\n", BuildDate)
		fmt.Printf("Commit:     %s\n", CommitHash)
		os.Exit(0)
	}

	// Merge short and long flag values
	if *dryRunShort {
		*dryRun = true
	}
	if *jsonShort {
		*jsonOutput = true
	}
	if *verboseShort > *verbose {
		*verbose = *verboseShort
	}

	// Parse positional arguments
	args := flag.Args()

	// Determine operation mode based on --delete flag
	if *deleteMode {
		// DELETE MODE: Validate delete-specific arguments
		// Supports 0 or 1 positional argument (VLAN tag)
		// Requires at least one of --mac or --hostname flags

		if len(args) > 1 {
			fmt.Fprintf(os.Stderr, "Error: Delete mode accepts at most one positional argument (VLAN tag)\n\n")
			flag.Usage()
			os.Exit(errors.ExitValidationError)
		}

		if *macFlag == "" && *hostnameFlag == "" {
			fmt.Fprintf(os.Stderr, "Error: Delete mode requires at least one identifier (--mac or --hostname)\n\n")
			flag.Usage()
			os.Exit(errors.ExitValidationError)
		}

		// Optional VLAN tag
		vlanTag := 0
		if len(args) == 1 {
			var err error
			vlanTag, err = strconv.Atoi(args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Invalid VLAN tag '%s': must be an integer\n", args[0])
				os.Exit(errors.ExitValidationError)
			}
		}

		// Setup logger
		logLevel := logger.LevelFromVerbosity(*verbose)
		log := logger.New(logLevel, *jsonOutput, os.Stderr)

		log.Info("Starting pfSense DHCP Static Mapping Updater (DELETE mode)",
			"version", Version,
			"mac", *macFlag,
			"hostname", *hostnameFlag,
			"vlan", vlanTag,
			"dry_run", *dryRun,
		)

		// Load configuration
		log.Info("Loading configuration", "config_file", *configFile)
		cfg, err := config.Load(*configFile)
		if err != nil {
			appErr := errors.NewConfigError("Failed to load configuration", err)
			fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
			os.Exit(appErr.Code)
		}

		// Override insecure setting if flag is set
		if *insecure {
			cfg.PfSense.InsecureSkipVerify = true
			log.Warn("TLS certificate verification disabled (--insecure flag)")
		}

		log.Info("Configuration loaded successfully",
			"endpoint", cfg.PfSense.Endpoint,
			"base_interface", cfg.PfSense.BaseInterface,
		)

		// Validate identifiers
		var normalizedMAC, normalizedHostname string
		if *macFlag != "" {
			normalizedMAC, err = validator.ValidateMAC(*macFlag)
			if err != nil {
				appErr := errors.NewValidationError("Invalid MAC address", err)
				log.Error("Validation failed", "error", err)
				fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
				os.Exit(appErr.Code)
			}
			log.Debug("MAC validated", "mac", normalizedMAC)
		}

		if *hostnameFlag != "" {
			normalizedHostname, err = validator.ValidateHostname(*hostnameFlag)
			if err != nil {
				appErr := errors.NewValidationError("Invalid hostname", err)
				log.Error("Validation failed", "error", err)
				fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
				os.Exit(appErr.Code)
			}
			log.Debug("Hostname validated", "hostname", normalizedHostname)
		}

		_, err = validator.ValidateVLAN(vlanTag)
		if err != nil {
			appErr := errors.NewValidationError("Invalid VLAN tag", err)
			log.Error("Validation failed", "error", err)
			fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
			os.Exit(appErr.Code)
		}

		// Create pfSense API client
		pfClient := pfsense.NewClient(pfsense.ClientConfig{
			BaseURL:            cfg.PfSense.Endpoint,
			APIKey:             cfg.PfSense.APIKey,
			Timeout:            cfg.Timeouts.APIRequest,
			InsecureSkipVerify: cfg.PfSense.InsecureSkipVerify,
			Logger:             log,
		})

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeouts.TotalExecution)
		defer cancel()

		// Resolve physical interface name to logical ID
		log.Info("Resolving interface mapping", "physical_base", cfg.PfSense.BaseInterface, "vlan", vlanTag)
		interfaceID, err := pfClient.ResolveInterfaceID(ctx, cfg.PfSense.BaseInterface, vlanTag, cfg.PfSense.InterfaceMapping)
		if err != nil {
			appErr := errors.NewConfigError("Interface resolution failed", err)
			log.Error("Failed to resolve interface", "error", err)
			fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
			os.Exit(appErr.Code)
		}
		log.Info("Interface resolved", "logical_id", interfaceID, "vlan", vlanTag)

		// Query interface configuration
		log.Info("Querying DHCP configuration", "interface", interfaceID)
		iface, err := pfClient.GetDHCPConfig(ctx, interfaceID)
		if err != nil {
			appErr := errors.NewAPIError("Failed to query DHCP configuration", err)
			log.Error("API request failed", "error", err, "interface", interfaceID)
			fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
			os.Exit(appErr.Code)
		}

		// Validate dual identifiers if both provided (US6 consistency check)
		if normalizedMAC != "" && normalizedHostname != "" {
			log.Debug("Validating dual identifiers", "mac", normalizedMAC, "hostname", normalizedHostname)
			if err := pfsense.ValidateDualIdentifiers(iface, normalizedMAC, normalizedHostname); err != nil {
				appErr := errors.NewValidationError("Identifier validation failed", err)
				log.Error("Dual identifier validation failed", "error", err)
				fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
				os.Exit(appErr.Code)
			}
			log.Debug("Dual identifiers validated successfully")
		}

		// Find the mapping to delete
		var mappingToDelete *models.StaticMapping
		var lookupKey string

		if normalizedMAC != "" {
			mappingToDelete = pfsense.FindMappingByMAC(iface, normalizedMAC)
			lookupKey = fmt.Sprintf("MAC %s", normalizedMAC)
		} else {
			mappingToDelete = pfsense.FindMappingByHostname(iface, normalizedHostname)
			lookupKey = fmt.Sprintf("hostname %s", normalizedHostname)
		}

		if mappingToDelete == nil {
			// Idempotent deletion: mapping doesn't exist = success (US6 requirement)
			result := map[string]interface{}{
				"status":   "noop",
				"message":  "Static mapping does not exist",
				"mac":      normalizedMAC,
				"hostname": normalizedHostname,
				"vlan":     vlanTag,
			}

			log.Info("Static mapping not found (idempotent success)", "lookup", lookupKey)

			if *jsonOutput {
				if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
					log.Error("Failed to encode JSON output", "error", err)
				}
			} else {
				fmt.Printf("No action needed: mapping does not exist\n")
				if normalizedMAC != "" {
					fmt.Printf("  MAC:      %s\n", normalizedMAC)
				}
				if normalizedHostname != "" {
					fmt.Printf("  Hostname: %s\n", normalizedHostname)
				}
				fmt.Printf("  VLAN:     %d\n", vlanTag)
			}

			os.Exit(0)
		}

		log.Info("Mapping found for deletion",
			"mac", mappingToDelete.MACAddress,
			"hostname", mappingToDelete.Hostname,
			"ip", mappingToDelete.IPAddress,
			"mapping_id", mappingToDelete.ID,
		)

		// Dry-run mode: preview deletion without making changes
		if *dryRun {
			result := map[string]interface{}{
				"status":    "dry_run",
				"operation": "delete",
				"message":   "Would delete static mapping (dry-run mode)",
				"mac":       mappingToDelete.MACAddress,
				"hostname":  mappingToDelete.Hostname,
				"ip":        mappingToDelete.IPAddress.String(),
				"vlan":      vlanTag,
			}

			log.Info("Dry-run mode: would delete mapping",
				"mac", mappingToDelete.MACAddress,
				"hostname", mappingToDelete.Hostname,
				"ip", mappingToDelete.IPAddress,
			)

			if *jsonOutput {
				if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
					log.Error("Failed to encode JSON output", "error", err)
				}
			} else {
				fmt.Printf("Dry-run: Would delete static mapping\n")
				fmt.Printf("  MAC:      %s\n", mappingToDelete.MACAddress)
				fmt.Printf("  Hostname: %s\n", mappingToDelete.Hostname)
				fmt.Printf("  IP:       %s\n", mappingToDelete.IPAddress)
				fmt.Printf("  VLAN:     %d\n", vlanTag)
			}

			os.Exit(0)
		}

		// Delete the mapping
		log.Info("Deleting static mapping", "mapping_id", mappingToDelete.ID, "interface_id", interfaceID)
		if err := pfClient.DeleteStaticMapping(ctx, interfaceID, mappingToDelete.ID); err != nil {
			appErr := errors.NewAPIError("Failed to delete static mapping", err)
			log.Error("Deletion failed", "error", err)
			fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
			os.Exit(appErr.Code)
		}

		// Apply pending DHCP configuration changes
		log.Info("Applying DHCP configuration changes")
		if err := pfClient.ApplyDHCPChanges(ctx); err != nil {
			appErr := errors.NewAPIError("Failed to apply DHCP changes", err)
			log.Error("DHCP apply failed", "error", err)
			fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
			// Don't exit - mapping was deleted, just warn user
			log.Warn("Static mapping deleted but changes not applied - manual apply required in pfSense GUI")
		} else {
			log.Info("DHCP configuration changes applied successfully")
		}

		// Success
		result := map[string]interface{}{
			"status":    "deleted",
			"operation": "delete",
			"message":   "Static mapping deleted successfully",
			"mac":       mappingToDelete.MACAddress,
			"hostname":  mappingToDelete.Hostname,
			"ip":        mappingToDelete.IPAddress.String(),
			"vlan":      vlanTag,
		}

		log.Info("Static mapping deleted successfully",
			"mac", mappingToDelete.MACAddress,
			"hostname", mappingToDelete.Hostname,
			"ip", mappingToDelete.IPAddress,
		)

		if *jsonOutput {
			if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
				log.Error("Failed to encode JSON output", "error", err)
			}
		} else {
			fmt.Printf("Success: Static mapping deleted\n")
			fmt.Printf("  MAC:      %s\n", mappingToDelete.MACAddress)
			fmt.Printf("  Hostname: %s\n", mappingToDelete.Hostname)
			fmt.Printf("  IP:       %s\n", mappingToDelete.IPAddress)
			fmt.Printf("  VLAN:     %d\n", vlanTag)
		}

		os.Exit(0)
	}

	// CREATE/UPDATE MODE: Original behavior (requires MAC and hostname as positional args)
	if len(args) < 2 || len(args) > 3 {
		fmt.Fprintf(os.Stderr, "Error: Invalid number of arguments\n\n")
		flag.Usage()
		os.Exit(errors.ExitValidationError)
	}

	macAddr := args[0]
	hostname := args[1]
	vlanTag := 0
	if len(args) == 3 {
		var err error
		vlanTag, err = strconv.Atoi(args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Invalid VLAN tag '%s': must be an integer\n", args[2])
			os.Exit(errors.ExitValidationError)
		}
	}

	// Setup logger
	logLevel := logger.LevelFromVerbosity(*verbose)
	log := logger.New(logLevel, *jsonOutput, os.Stderr)

	log.Info("Starting pfSense DHCP Static Mapping Updater",
		"version", Version,
		"mac", macAddr,
		"hostname", hostname,
		"vlan", vlanTag,
		"dry_run", *dryRun,
	)

	// Load configuration
	log.Info("Loading configuration", "config_file", *configFile)
	cfg, err := config.Load(*configFile)
	if err != nil {
		appErr := errors.NewConfigError("Failed to load configuration", err)
		fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
		os.Exit(appErr.Code)
	}

	// Override insecure setting if flag is set
	if *insecure {
		cfg.PfSense.InsecureSkipVerify = true
		log.Warn("TLS certificate verification disabled (--insecure flag)")
	}

	log.Info("Configuration loaded successfully",
		"endpoint", cfg.PfSense.Endpoint,
		"base_interface", cfg.PfSense.BaseInterface,
	)

	// Validate input
	log.Debug("Validating input parameters")
	normalizedMAC, err := validator.ValidateMAC(macAddr)
	if err != nil {
		appErr := errors.NewValidationError("Invalid MAC address", err)
		log.Error("Validation failed", "error", err)
		fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
		os.Exit(appErr.Code)
	}

	normalizedHostname, err := validator.ValidateHostname(hostname)
	if err != nil {
		appErr := errors.NewValidationError("Invalid hostname", err)
		log.Error("Validation failed", "error", err)
		fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
		os.Exit(appErr.Code)
	}

	_, err = validator.ValidateVLAN(vlanTag)
	if err != nil {
		appErr := errors.NewValidationError("Invalid VLAN tag", err)
		log.Error("Validation failed", "error", err)
		fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
		os.Exit(appErr.Code)
	}

	log.Debug("Input validated", "mac", normalizedMAC, "hostname", normalizedHostname)

	// Create pfSense API client
	pfClient := pfsense.NewClient(pfsense.ClientConfig{
		BaseURL:            cfg.PfSense.Endpoint,
		APIKey:             cfg.PfSense.APIKey,
		Timeout:            cfg.Timeouts.APIRequest,
		InsecureSkipVerify: cfg.PfSense.InsecureSkipVerify,
		Logger:             log,
	})

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeouts.TotalExecution)
	defer cancel()

	// Resolve physical interface name to logical ID (e.g., iavf1 -> lan, iavf1.101 -> opt7)
	log.Info("Resolving interface mapping", "physical_base", cfg.PfSense.BaseInterface, "vlan", vlanTag)
	interfaceID, err := pfClient.ResolveInterfaceID(ctx, cfg.PfSense.BaseInterface, vlanTag, cfg.PfSense.InterfaceMapping)
	if err != nil {
		appErr := errors.NewConfigError("Interface resolution failed", err)
		log.Error("Failed to resolve interface", "error", err)
		fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
		os.Exit(appErr.Code)
	}
	log.Info("Interface resolved", "logical_id", interfaceID, "vlan", vlanTag)

	// Query interface configuration
	log.Info("Querying DHCP configuration", "interface", interfaceID)
	iface, err := pfClient.GetDHCPConfig(ctx, interfaceID)
	if err != nil {
		appErr := errors.NewAPIError("Failed to query DHCP configuration", err)
		log.Error("API request failed", "error", err, "interface", interfaceID)
		fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
		os.Exit(appErr.Code)
	}

	// Set VLAN tag in interface (API doesn't return it)
	iface.VLANTag = vlanTag

	// Validate interface state
	if err := iface.Validate(); err != nil {
		appErr := errors.NewConfigError("Interface not properly configured for DHCP", err)
		log.Error("Interface validation failed", "error", err)
		fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
		os.Exit(appErr.Code)
	}

	log.Info("Interface configuration retrieved",
		"dhcp_enabled", iface.DHCPEnabled,
		"ip", iface.IPAddress,
		"subnet", iface.Subnet,
		"mask", iface.SubnetMask,
		"static_mappings", len(iface.StaticMappings),
		"dynamic_leases", len(iface.DynamicLeases),
	)

	// Check for existing mapping with same MAC (idempotency check)
	for _, existing := range iface.StaticMappings {
		if existing.MACAddress == normalizedMAC {
			log.Info("Static mapping already exists for MAC address",
				"mac", normalizedMAC,
				"existing_hostname", existing.Hostname,
				"existing_ip", existing.IPAddress,
				"requested_hostname", normalizedHostname,
			)

			// Save old hostname for update tracking (US2)
			oldHostname := existing.Hostname

			// Check if hostname matches (case-insensitive comparison)
			existingHostnameNorm := strings.ToLower(strings.TrimSpace(existing.Hostname))
			if existingHostnameNorm == normalizedHostname {
				// Perfect match: noop (same MAC and hostname)
				result := map[string]interface{}{
					"status":   "noop",
					"message":  "Static mapping already exists with same hostname",
					"mac":      normalizedMAC,
					"hostname": normalizedHostname,
					"ip":       existing.IPAddress.String(),
					"vlan":     vlanTag,
				}

				if *jsonOutput {
					if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
						log.Error("Failed to encode JSON output", "error", err)
					}
				} else {
					fmt.Printf("No action needed: mapping already exists\n")
					fmt.Printf("  MAC:      %s\n", normalizedMAC)
					fmt.Printf("  Hostname: %s\n", normalizedHostname)
					fmt.Printf("  IP:       %s\n", existing.IPAddress)
					fmt.Printf("  VLAN:     %d\n", vlanTag)
				}

				os.Exit(0)
			} else {
				// Hostname mismatch - this is an update scenario (US2: Update Hostname for Existing MAC)
				// This handles the case where a VM is renamed but keeps the same MAC address.
				// We update the hostname in the existing mapping while preserving the IP address.
				// This prevents duplicate mappings and maintains IP stability for renamed VMs.
				log.Info("MAC address exists with different hostname, updating hostname",
					"mac", normalizedMAC,
					"old_hostname", oldHostname,
					"new_hostname", normalizedHostname,
					"ip", existing.IPAddress,
				)

				// Update the existing mapping with new hostname
				existing.Hostname = normalizedHostname
				existing.Description = fmt.Sprintf("Updated by pfsense-dhcp-updater on %s", time.Now().Format(time.RFC3339))

				// Dry-run mode: Preview the hostname update without making actual changes
				// This allows operators to verify the update before applying it to production
				if *dryRun {
					result := map[string]interface{}{
						"status":       "dry_run",
						"operation":    "update",
						"message":      "Would update hostname (dry-run mode)",
						"mac":          normalizedMAC,
						"old_hostname": oldHostname,
						"new_hostname": normalizedHostname,
						"ip":           existing.IPAddress.String(),
						"vlan":         vlanTag,
					}

					log.Info("Dry-run mode: would update hostname",
						"mac", normalizedMAC,
						"old_hostname", oldHostname,
						"new_hostname", normalizedHostname,
						"ip", existing.IPAddress,
					)

					if *jsonOutput {
						if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
							log.Error("Failed to encode JSON output", "error", err)
						}
					} else {
						fmt.Printf("Dry-run: Would update hostname\n")
						fmt.Printf("  MAC:          %s\n", normalizedMAC)
						fmt.Printf("  Old Hostname: %s\n", oldHostname)
						fmt.Printf("  New Hostname: %s\n", normalizedHostname)
						fmt.Printf("  IP:           %s\n", existing.IPAddress)
						fmt.Printf("  VLAN:         %d\n", vlanTag)
					}

					os.Exit(0)
				}
			}

			// Update the hostname in pfSense using PATCH API call
			// This modifies the existing mapping in place while preserving the IP address
			if err := pfClient.UpdateStaticMapping(ctx, &existing); err != nil {
				appErr := errors.NewAPIError("Failed to update static mapping hostname", err)
				log.Error("Hostname update failed", "error", err)
				fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
				os.Exit(appErr.Code)
			}

			// Apply the DHCP configuration changes to make the update active
			// The PATCH call updates the config, but changes aren't live until applied
			log.Info("Applying DHCP configuration changes")
			if err := pfClient.ApplyDHCPChanges(ctx); err != nil {
				appErr := errors.NewAPIError("Failed to apply DHCP changes", err)
				log.Error("Failed to apply DHCP changes", "error", err)
				fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
				os.Exit(appErr.Code)
			}

			// Success output
			result := map[string]interface{}{
				"status":       "success",
				"operation":    "update",
				"message":      "Hostname updated successfully",
				"mac":          normalizedMAC,
				"old_hostname": oldHostname,
				"new_hostname": normalizedHostname,
				"ip":           existing.IPAddress.String(),
				"vlan":         vlanTag,
			}

			log.Info("Hostname updated successfully",
				"mac", normalizedMAC,
				"new_hostname", normalizedHostname,
				"ip", existing.IPAddress,
			)

			if *jsonOutput {
				if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
					log.Error("Failed to encode JSON output", "error", err)
				}
			} else {
				fmt.Printf("Success: Hostname updated\n")
				fmt.Printf("  MAC:          %s\n", normalizedMAC)
				fmt.Printf("  Old Hostname: %s\n", oldHostname)
				fmt.Printf("  New Hostname: %s\n", normalizedHostname)
				fmt.Printf("  IP:           %s\n", existing.IPAddress)
				fmt.Printf("  VLAN:         %d\n", vlanTag)
			}

			os.Exit(0)
		}
	}
	// Check for hostname conflict with different MAC (case-insensitive)
	if err := pfsense.CheckHostnameConflict(iface, normalizedHostname, normalizedMAC); err != nil {
		appErr := errors.NewHostnameConflictError(normalizedHostname, "")
		log.Error("Hostname conflict detected",
			"hostname", normalizedHostname,
			"error", err,
		)
		// The error message from CheckHostnameConflict includes the MAC and IP
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(appErr.Code)
	}

	// Get configured IP range for this VLAN (if any)
	var ipRangeConfig *models.IPRangeConfig
	if vlanRanges, ok := cfg.VLANIPRanges[vlanTag]; ok && len(vlanRanges) > 0 {
		// Use the first configured range
		rangeStr := vlanRanges[0]
		log.Debug("Using configured IP range", "vlan", vlanTag, "range", rangeStr)

		startIP, endIP, err := models.ParseIPRange(rangeStr)
		if err != nil {
			appErr := errors.NewConfigError("Invalid IP range format", err)
			log.Error("Failed to parse IP range", "error", err, "range", rangeStr)
			fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
			os.Exit(appErr.Code)
		}

		ipRangeConfig = &models.IPRangeConfig{
			StartIP: startIP,
			EndIP:   endIP,
		}

		log.Info("IP range configured", "start", startIP, "end", endIP)
	}

	// Calculate available IP pool
	log.Info("Calculating IP pool", "interface", interfaceID)
	pool, err := models.CalculateIPPool(iface, ipRangeConfig)
	if err != nil {
		appErr := errors.NewAPIError("Failed to calculate IP pool", err)
		log.Error("IP pool calculation failed", "error", err)
		fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
		os.Exit(appErr.Code)
	}

	log.Info("IP pool calculated",
		"total_capacity", pool.TotalCapacity,
		"available_count", pool.AvailableCount,
		"exclusions", len(pool.Exclusions),
	)

	// Get next available IP
	nextIP, err := pool.GetNextAvailableIP()
	if err != nil {
		appErr := errors.NewAPIError("No available IP addresses in pool", err)
		log.Error("IP pool exhausted", "error", err)
		fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
		os.Exit(appErr.Code)
	}

	log.Info("Next available IP selected", "ip", nextIP)

	// Create static mapping entity
	mapping := &models.StaticMapping{
		MACAddress:  normalizedMAC,
		IPAddress:   nextIP,
		Hostname:    normalizedHostname,
		Interface:   interfaceID,
		Description: fmt.Sprintf("Created by pfsense-dhcp-updater on %s", time.Now().Format(time.RFC3339)),
	}

	// Dry-run mode: stop before making changes
	if *dryRun {
		result := map[string]interface{}{
			"status":   "dry_run",
			"message":  "Would create static mapping (dry-run mode)",
			"mac":      normalizedMAC,
			"hostname": normalizedHostname,
			"ip":       nextIP.String(),
			"vlan":     vlanTag,
		}

		log.Info("Dry-run mode: would create mapping",
			"mac", normalizedMAC,
			"hostname", normalizedHostname,
			"ip", nextIP,
			"vlan", vlanTag,
		)

		if *jsonOutput {
			if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
				log.Error("Failed to encode JSON output", "error", err)
			}
		} else {
			fmt.Printf("Dry-run: Would create static mapping\n")
			fmt.Printf("  MAC:      %s\n", normalizedMAC)
			fmt.Printf("  Hostname: %s\n", normalizedHostname)
			fmt.Printf("  IP:       %s\n", nextIP)
			fmt.Printf("  VLAN:     %d\n", vlanTag)
			fmt.Printf("  Interface: %s\n", interfaceID)
		}

		os.Exit(0)
	}

	// Create static mapping in pfSense with retry logic for concurrent IP conflicts
	// Retry up to 3 times if IP assignment fails due to concurrent assignment by another process
	log.Info("Creating static mapping", "mac", normalizedMAC, "hostname", normalizedHostname, "ip", nextIP)
	if err := pfClient.CreateStaticMappingWithRetry(ctx, mapping, pool, 3); err != nil {
		appErr := errors.NewAPIError("Failed to create static mapping", err)
		log.Error("Static mapping creation failed", "error", err)
		fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
		os.Exit(appErr.Code)
	}

	// Log the final IP address (may have changed due to retry with different IP)
	log.Info("Static mapping created", "final_ip", mapping.IPAddress.String())

	// Apply pending DHCP configuration changes
	log.Info("Applying DHCP configuration changes")
	if err := pfClient.ApplyDHCPChanges(ctx); err != nil {
		appErr := errors.NewAPIError("Failed to apply DHCP changes", err)
		log.Error("DHCP apply failed", "error", err)
		fmt.Fprintln(os.Stderr, appErr.ErrorWithHint())
		// Don't exit - mapping was created, just warn user
		log.Warn("Static mapping created but changes not applied - manual apply required in pfSense GUI")
	} else {
		log.Info("DHCP configuration changes applied successfully")
	}

	// Success - use mapping.IPAddress since it may have been updated by retry logic
	result := map[string]interface{}{
		"status":   "created",
		"message":  "Static mapping created successfully",
		"mac":      normalizedMAC,
		"hostname": normalizedHostname,
		"ip":       mapping.IPAddress.String(),
		"vlan":     vlanTag,
	}

	log.Info("Static mapping created successfully",
		"mac", normalizedMAC,
		"hostname", normalizedHostname,
		"ip", mapping.IPAddress.String(),
		"vlan", vlanTag,
	)

	if *jsonOutput {
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			log.Error("Failed to encode JSON output", "error", err)
		}
	} else {
		fmt.Printf("Success: Static mapping created\n")
		fmt.Printf("  MAC:      %s\n", normalizedMAC)
		fmt.Printf("  Hostname: %s\n", normalizedHostname)
		fmt.Printf("  IP:       %s\n", mapping.IPAddress.String())
		fmt.Printf("  VLAN:     %d\n", vlanTag)
		fmt.Printf("  Interface: %s\n", interfaceID)
	}

	os.Exit(0)
}
