// Package driver implements the Fleet SDK Driver interface for Mujina miners.
//
// Mujina (https://github.com/256foundation/mujina) is an open-source Bitcoin
// mining firmware that exposes an unauthenticated REST API on port 7785 by
// default.  This driver discovers and manages Mujina devices through that API.
package driver

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"github.com/block/proto-fleet/plugin/mujina/internal/device"
	"github.com/block/proto-fleet/plugin/mujina/pkg/mujina"
	sdk "github.com/block/proto-fleet/server/sdk/v1"
)

const (
	driverName  = "mujina"
	apiVersion  = "v1"
	manufacturer = "256 Foundation / Mujina"
)

var _ sdk.Driver = (*Driver)(nil)
var _ sdk.DiscoveryPortsProvider = (*Driver)(nil)
var _ sdk.DefaultCredentialsProvider = (*Driver)(nil)

// Driver implements the Fleet SDK Driver interface for Mujina devices.
type Driver struct {
	devices map[string]sdk.Device
	mutex   sync.RWMutex
}

// New creates a new Mujina driver.
func New() *Driver {
	return &Driver{
		devices: make(map[string]sdk.Device),
	}
}

// Handshake implements sdk.Driver.
func (d *Driver) Handshake(_ context.Context) (sdk.DriverIdentifier, error) {
	return sdk.DriverIdentifier{
		DriverName: driverName,
		APIVersion: apiVersion,
	}, nil
}

// DescribeDriver implements sdk.Driver.
func (d *Driver) DescribeDriver(_ context.Context) (sdk.DriverIdentifier, sdk.Capabilities, error) {
	id := sdk.DriverIdentifier{
		DriverName: driverName,
		APIVersion: apiVersion,
	}
	caps := sdk.Capabilities{
		sdk.CapabilityPollingHost: true,
		sdk.CapabilityDiscovery:   true,
		sdk.CapabilityPairing:     true,

		sdk.CapabilityMiningStart: true,
		sdk.CapabilityMiningStop:  true,

		sdk.CapabilityRealtimeTelemetry: true,
		sdk.CapabilityHashrateReported:  true,
		sdk.CapabilityUptime:            true,
		sdk.CapabilityTemperature:       true,
		sdk.CapabilityFanSpeed:          true,
		sdk.CapabilityPowerUsage:        true,
		sdk.CapabilityMinerStatus:       true,
		sdk.CapabilityPoolStats:         true,
		sdk.CapabilityPerBoardStats:     true,
	}
	return id, caps, nil
}

// GetDiscoveryPorts implements sdk.DiscoveryPortsProvider.
func (d *Driver) GetDiscoveryPorts(_ context.Context) []string {
	return []string{strconv.Itoa(mujina.DefaultPort)}
}

// DiscoverDevice implements sdk.Driver. It probes the given address and port for
// a Mujina API and returns device information if found.
func (d *Driver) DiscoverDevice(ctx context.Context, ipAddress, port string) (sdk.DeviceInfo, error) {
	apiPort, err := parsePort(port)
	if err != nil {
		apiPort = mujina.DefaultPort
	}

	c := mujina.NewClient(ipAddress, apiPort)

	if err := c.Health(ctx); err != nil {
		return sdk.DeviceInfo{}, fmt.Errorf("mujina health check failed: %w", err)
	}

	info, err := deviceInfoFromAPI(ctx, c, ipAddress, apiPort)
	if err != nil {
		return sdk.DeviceInfo{}, fmt.Errorf("failed to read device info: %w", err)
	}

	slog.Info("Discovered Mujina device", "host", ipAddress, "port", apiPort, "model", info.Model)
	return info, nil
}

// PairDevice implements sdk.Driver. Mujina has no authentication, so pairing
// simply verifies connectivity and returns updated device info.
func (d *Driver) PairDevice(ctx context.Context, deviceInfo sdk.DeviceInfo, _ sdk.SecretBundle) (sdk.DeviceInfo, error) {
	apiPort := int(deviceInfo.Port)
	if apiPort == 0 {
		apiPort = mujina.DefaultPort
	}

	c := mujina.NewClient(deviceInfo.Host, apiPort)

	if err := c.Health(ctx); err != nil {
		return sdk.DeviceInfo{}, fmt.Errorf("mujina unreachable during pairing: %w", err)
	}

	updated, err := deviceInfoFromAPI(ctx, c, deviceInfo.Host, apiPort)
	if err != nil {
		// Non-fatal: return original info if we can't refresh.
		slog.Warn("Could not refresh device info during pairing", "host", deviceInfo.Host, "error", err)
		return deviceInfo, nil
	}

	slog.Info("Paired Mujina device", "host", deviceInfo.Host, "model", updated.Model)
	return updated, nil
}

// NewDevice implements sdk.Driver. It creates a new Device instance.
func (d *Driver) NewDevice(ctx context.Context, deviceID string, deviceInfo sdk.DeviceInfo, _ sdk.SecretBundle) (sdk.NewDeviceResult, error) {
	apiPort := int(deviceInfo.Port)
	if apiPort == 0 {
		apiPort = mujina.DefaultPort
	}

	dev, err := device.New(deviceID, deviceInfo, mujina.NewClient(deviceInfo.Host, apiPort))
	if err != nil {
		return sdk.NewDeviceResult{}, fmt.Errorf("failed to create mujina device: %w", err)
	}

	d.mutex.Lock()
	d.devices[deviceID] = dev
	d.mutex.Unlock()

	slog.Info("Created Mujina device", "deviceID", deviceID, "host", deviceInfo.Host)
	return sdk.NewDeviceResult{Device: dev}, nil
}

// GetDefaultCredentials implements sdk.DefaultCredentialsProvider.
// Mujina has no authentication; returning a single empty credential causes
// the Fleet server to attempt pairing immediately (auto-pair on discovery)
// rather than waiting for the user to enter credentials.
func (d *Driver) GetDefaultCredentials(_ context.Context, _, _ string) []sdk.UsernamePassword {
	return []sdk.UsernamePassword{{Username: "", Password: ""}}
}

// ---- helpers ----------------------------------------------------------------

// deviceInfoFromAPI builds a sdk.DeviceInfo by querying the Mujina API.
func deviceInfoFromAPI(ctx context.Context, c *mujina.Client, host string, port int) (sdk.DeviceInfo, error) {
	boards, err := c.GetBoards(ctx)
	if err != nil {
		return sdk.DeviceInfo{}, err
	}

	model := "Mujina Miner"
	var serial string
	if len(boards) > 0 {
		model = boards[0].Model
		if boards[0].Serial != nil {
			serial = *boards[0].Serial
		}
	}

	return sdk.DeviceInfo{
		Host:         host,
		Port:         int32(port),
		URLScheme:    "http",
		Model:        model,
		Manufacturer: manufacturer,
		SerialNumber: serial,
	}, nil
}

func parsePort(s string) (int, error) {
	if s == "" || s == "0" {
		return 0, fmt.Errorf("empty port")
	}
	p, err := strconv.Atoi(s)
	if err != nil || p <= 0 || p > 65535 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return p, nil
}
