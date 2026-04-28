// Package device implements the Fleet SDK Device interface for a single Mujina miner.
package device

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/block/proto-fleet/plugin/mujina/pkg/mujina"
	sdk "github.com/block/proto-fleet/server/sdk/v1"
	sdkerrors "github.com/block/proto-fleet/server/sdk/v1/errors"
)

const (
	statusCacheTTL  = 5 * time.Second
	sensorTypeUptime = "uptime"
	unitSeconds      = "seconds"
)

var _ sdk.Device = (*Device)(nil)

// Device implements sdk.Device for a Mujina miner.
type Device struct {
	id         string
	deviceInfo sdk.DeviceInfo
	client     *mujina.Client

	mu          sync.Mutex
	lastMetrics *sdk.DeviceMetrics
	lastFetched time.Time
}

// New creates a new Mujina device instance.
func New(id string, info sdk.DeviceInfo, client *mujina.Client) (*Device, error) {
	return &Device{
		id:         id,
		deviceInfo: info,
		client:     client,
	}, nil
}

// ID implements sdk.DeviceCore.
func (d *Device) ID() string { return d.id }

// DescribeDevice implements sdk.DeviceCore.
func (d *Device) DescribeDevice(_ context.Context) (sdk.DeviceInfo, sdk.Capabilities, error) {
	caps := sdk.Capabilities{
		sdk.CapabilityPollingHost: true,

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
	return d.deviceInfo, caps, nil
}

// Status implements sdk.DeviceCore. It fetches the current miner snapshot and
// maps it to sdk.DeviceMetrics. Results are cached for statusCacheTTL.
func (d *Device) Status(ctx context.Context) (sdk.DeviceMetrics, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.lastMetrics != nil && time.Since(d.lastFetched) < statusCacheTTL {
		return *d.lastMetrics, nil
	}

	t, err := d.client.GetMiner(ctx)
	if err != nil {
		return sdk.DeviceMetrics{}, fmt.Errorf("GetMiner: %w", err)
	}

	metrics := telemetryToMetrics(d.id, t)
	d.lastMetrics = &metrics
	d.lastFetched = time.Now()
	return metrics, nil
}

// Close implements sdk.DeviceCore — Mujina uses stateless HTTP; nothing to close.
func (d *Device) Close(_ context.Context) error { return nil }

// StartMining implements sdk.DeviceControl.
func (d *Device) StartMining(ctx context.Context) error {
	paused := false
	if err := d.client.PatchMiner(ctx, mujina.MinerPatchRequest{Paused: &paused}); err != nil {
		return fmt.Errorf("start mining: %w", err)
	}
	d.invalidateCache()
	slog.Info("Started mining", "deviceID", d.id)
	return nil
}

// StopMining implements sdk.DeviceControl.
func (d *Device) StopMining(ctx context.Context) error {
	paused := true
	if err := d.client.PatchMiner(ctx, mujina.MinerPatchRequest{Paused: &paused}); err != nil {
		return fmt.Errorf("stop mining: %w", err)
	}
	d.invalidateCache()
	slog.Info("Stopped mining", "deviceID", d.id)
	return nil
}

// BlinkLED implements sdk.DeviceControl — not supported by Mujina.
func (d *Device) BlinkLED(_ context.Context) error {
	return sdk.NewErrUnsupportedCapability(sdk.CapabilityLEDBlink)
}

// Reboot implements sdk.DeviceControl — not supported by Mujina.
func (d *Device) Reboot(_ context.Context) error {
	return sdk.NewErrUnsupportedCapability(sdk.CapabilityReboot)
}

// SetCoolingMode implements sdk.DeviceConfiguration — not supported by Mujina.
func (d *Device) SetCoolingMode(_ context.Context, _ sdk.CoolingMode) error {
	return sdk.NewErrUnsupportedCapability(sdk.CapabilityCoolingModeAir)
}

// GetCoolingMode implements sdk.DeviceConfiguration — not supported by Mujina.
func (d *Device) GetCoolingMode(_ context.Context) (sdk.CoolingMode, error) {
	return sdk.CoolingModeUnspecified, sdk.NewErrUnsupportedCapability(sdk.CapabilityCoolingModeAir)
}

// SetPowerTarget implements sdk.DeviceConfiguration — not supported by Mujina.
func (d *Device) SetPowerTarget(_ context.Context, _ sdk.PerformanceMode) error {
	return sdk.NewErrUnsupportedCapability(sdk.CapabilityPowerModeEfficiency)
}

// UpdateMiningPools implements sdk.DeviceConfiguration — not supported by Mujina.
// Mujina pools are configured via environment variables, not the API.
func (d *Device) UpdateMiningPools(_ context.Context, _ []sdk.MiningPoolConfig) error {
	return sdk.NewErrUnsupportedCapability(sdk.CapabilityPoolConfig)
}

// GetMiningPools implements sdk.DeviceConfiguration.
// Returns pool info derived from the sources endpoint.
func (d *Device) GetMiningPools(ctx context.Context) ([]sdk.ConfiguredPool, error) {
	t, err := d.client.GetMiner(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetMiner: %w", err)
	}
	pools := make([]sdk.ConfiguredPool, 0, len(t.Sources))
	for i, s := range t.Sources {
		pool := sdk.ConfiguredPool{
			Priority: int32(i),
			Username: s.Name,
		}
		if s.URL != nil {
			pool.URL = *s.URL
		}
		pools = append(pools, pool)
	}
	return pools, nil
}

// UpdateMinerPassword implements sdk.DeviceConfiguration — not supported by Mujina.
func (d *Device) UpdateMinerPassword(_ context.Context, _, _ string) error {
	return sdk.NewErrUnsupportedCapability(sdk.CapabilityUpdateMinerPassword)
}

// DownloadLogs implements sdk.DeviceMaintenance — not supported by Mujina.
func (d *Device) DownloadLogs(_ context.Context, _ *time.Time, _ string) (string, bool, error) {
	return "", false, sdk.NewErrUnsupportedCapability(sdk.CapabilityLogsDownload)
}

// FirmwareUpdate implements sdk.DeviceMaintenance — not supported by Mujina.
func (d *Device) FirmwareUpdate(_ context.Context, _ sdk.FirmwareFile) error {
	return sdk.NewErrUnsupportedCapability(sdk.CapabilityFirmware)
}

// Unpair implements sdk.DeviceMaintenance — Mujina has no credentials to clear.
func (d *Device) Unpair(_ context.Context) error { return nil }

// GetErrors implements sdk.DeviceErrorReporting.
// Derives errors from telemetry heuristics (fan failures, high temperatures).
func (d *Device) GetErrors(ctx context.Context) (sdk.DeviceErrors, error) {
	t, err := d.client.GetMiner(ctx)
	if err != nil {
		return sdk.DeviceErrors{}, fmt.Errorf("GetMiner: %w", err)
	}
	return detectErrors(d.id, t), nil
}

// TryBatchStatus implements sdk.DeviceOptional.
func (d *Device) TryBatchStatus(_ context.Context, _ []string) (map[string]sdk.DeviceMetrics, bool, error) {
	return nil, false, nil
}

// TrySubscribe implements sdk.DeviceOptional.
func (d *Device) TrySubscribe(_ context.Context, _ []string) (<-chan sdk.DeviceMetrics, bool, error) {
	return nil, false, nil
}

// TryGetWebViewURL implements sdk.DeviceOptional.
func (d *Device) TryGetWebViewURL(_ context.Context) (string, bool, error) {
	return "", false, nil
}

// TryGetTimeSeriesData implements sdk.DeviceOptional.
func (d *Device) TryGetTimeSeriesData(_ context.Context, _ []string, _, _ time.Time, _ *time.Duration, _ int32, _ string) ([]sdk.DeviceMetrics, string, bool, error) {
	return nil, "", false, nil
}

// ---- telemetry mapping ------------------------------------------------------

func telemetryToMetrics(deviceID string, t *mujina.MinerTelemetry) sdk.DeviceMetrics {
	m := sdk.DeviceMetrics{
		DeviceID:  deviceID,
		Timestamp: time.Now(),
	}

	// Health: paused → inactive, otherwise active.
	if t.Paused {
		m.Health = sdk.HealthHealthyInactive
	} else {
		m.Health = sdk.HealthHealthyActive
	}

	// Device-level aggregates.
	if t.Hashrate > 0 {
		m.HashrateHS = gaugeMetric(float64(t.Hashrate))
	}

	// Uptime as a sensor metric.
	m.SensorMetrics = []sdk.SensorMetrics{
		{
			ComponentInfo: sdk.ComponentInfo{
				Index:  0,
				Name:   "uptime",
				Status: sdk.ComponentStatusHealthy,
			},
			Type:  sensorTypeUptime,
			Unit:  unitSeconds,
			Value: counterMetric(float64(t.UptimeSecs)),
		},
	}

	// Per-board metrics.
	m.HashBoards = make([]sdk.HashBoardMetrics, 0, len(t.Boards))
	var totalPowerW float64
	var highestTempC float64
	var highestFanRPM float64
	hasPower := false
	hasTemp := false
	hasFan := false

	for i, b := range t.Boards {
		hb := boardToHashBoard(i, b)
		m.HashBoards = append(m.HashBoards, hb)

		// Accumulate device-level aggregates from board data.
		for _, pw := range b.Powers {
			if pw.PowerW != nil {
				totalPowerW += float64(*pw.PowerW)
				hasPower = true
			}
		}
		for _, ts := range b.Temperatures {
			if ts.TemperatureC != nil && float64(*ts.TemperatureC) > highestTempC {
				highestTempC = float64(*ts.TemperatureC)
				hasTemp = true
			}
		}
		for _, f := range b.Fans {
			if f.RPM != nil && float64(*f.RPM) > highestFanRPM {
				highestFanRPM = float64(*f.RPM)
				hasFan = true
			}
		}
	}

	if hasPower {
		m.PowerW = gaugeMetric(totalPowerW)
	}
	if hasTemp {
		m.TempC = gaugeMetric(highestTempC)
	}
	if hasFan {
		m.FanRPM = gaugeMetric(highestFanRPM)
	}

	return m
}

func boardToHashBoard(idx int, b mujina.BoardTelemetry) sdk.HashBoardMetrics {
	hb := sdk.HashBoardMetrics{
		ComponentInfo: sdk.ComponentInfo{
			Index:  int32(idx),
			Name:   b.Name,
			Status: sdk.ComponentStatusHealthy,
		},
	}
	if b.Serial != nil {
		hb.SerialNumber = b.Serial
	}

	// Hashrate: sum of all active thread hashrates.
	var boardHashrate float64
	for _, th := range b.Threads {
		if th.IsActive {
			boardHashrate += float64(th.Hashrate)
		}
	}
	if boardHashrate > 0 {
		hb.HashRateHS = gaugeMetric(boardHashrate)
	}

	// Temperatures: use the highest reading for the board aggregate.
	var highestTemp float64
	hasTemp := false
	for _, ts := range b.Temperatures {
		if ts.TemperatureC != nil {
			if float64(*ts.TemperatureC) > highestTemp {
				highestTemp = float64(*ts.TemperatureC)
			}
			hasTemp = true
		}
	}
	if hasTemp {
		hb.TempC = gaugeMetric(highestTemp)
	}

	// Power.
	for _, pw := range b.Powers {
		if pw.VoltageV != nil {
			hb.VoltageV = gaugeMetric(float64(*pw.VoltageV))
		}
		if pw.CurrentA != nil {
			hb.CurrentA = gaugeMetric(float64(*pw.CurrentA))
		}
	}

	// Per-thread metrics mapped to ASIC slots.
	hb.ASICs = make([]sdk.ASICMetrics, 0, len(b.Threads))
	for j, th := range b.Threads {
		status := sdk.ComponentStatusDisabled
		if th.IsActive {
			status = sdk.ComponentStatusHealthy
		}
		asic := sdk.ASICMetrics{
			ComponentInfo: sdk.ComponentInfo{
				Index:  int32(j),
				Name:   th.Name,
				Status: status,
			},
		}
		if th.IsActive {
			asic.HashrateHS = gaugeMetric(float64(th.Hashrate))
		}
		hb.ASICs = append(hb.ASICs, asic)
	}

	// Fan metrics.
	hb.FanMetrics = make([]sdk.FanMetrics, 0, len(b.Fans))
	for k, f := range b.Fans {
		fm := sdk.FanMetrics{
			ComponentInfo: sdk.ComponentInfo{
				Index:  int32(k),
				Name:   f.Name,
				Status: sdk.ComponentStatusHealthy,
			},
		}
		if f.RPM != nil {
			fm.RPM = gaugeMetric(float64(*f.RPM))
			if *f.RPM == 0 {
				fm.Status = sdk.ComponentStatusWarning
			}
		}
		if f.Percent != nil {
			fm.Percent = gaugeMetric(float64(*f.Percent))
		}
		hb.FanMetrics = append(hb.FanMetrics, fm)
	}

	return hb
}

// detectErrors derives DeviceErrors from heuristics on the telemetry snapshot.
func detectErrors(deviceID string, t *mujina.MinerTelemetry) sdk.DeviceErrors {
	var errs []sdk.DeviceError
	now := time.Now()

	for _, b := range t.Boards {
		for _, f := range b.Fans {
			if f.RPM != nil && *f.RPM == 0 {
				errs = append(errs, sdk.DeviceError{
					MinerError:    sdkerrors.FanFailed,
					Severity:      sdkerrors.SeverityMajor,
					ComponentType: sdkerrors.ComponentTypeFan,
					CauseSummary:  fmt.Sprintf("fan %q on board %q reports 0 RPM", f.Name, b.Name),
					FirstSeenAt:   now,
					LastSeenAt:    now,
				})
			}
		}
		for _, ts := range b.Temperatures {
			if ts.TemperatureC != nil && float64(*ts.TemperatureC) > 95.0 {
				errs = append(errs, sdk.DeviceError{
					MinerError:    sdkerrors.DeviceOverTemperature,
					Severity:      sdkerrors.SeverityCritical,
					ComponentType: sdkerrors.ComponentTypeHashBoard,
					CauseSummary:  fmt.Sprintf("sensor %q on board %q: %.1f °C exceeds 95 °C threshold", ts.Name, b.Name, *ts.TemperatureC),
					FirstSeenAt:   now,
					LastSeenAt:    now,
				})
			}
		}
	}

	return sdk.DeviceErrors{
		DeviceID: deviceID,
		Errors:   errs,
	}
}

// ---- metric helpers ---------------------------------------------------------

func gaugeMetric(v float64) *sdk.MetricValue {
	return &sdk.MetricValue{Value: v, Kind: sdk.MetricKindGauge}
}

func counterMetric(v float64) *sdk.MetricValue {
	return &sdk.MetricValue{Value: v, Kind: sdk.MetricKindCounter}
}

// invalidateCache forces the next Status() call to fetch fresh data.
func (d *Device) invalidateCache() {
	d.mu.Lock()
	d.lastMetrics = nil
	d.mu.Unlock()
}
