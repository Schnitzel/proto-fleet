package device_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/block/proto-fleet/plugin/mujina/internal/device"
	"github.com/block/proto-fleet/plugin/mujina/pkg/mujina"
	sdk "github.com/block/proto-fleet/server/sdk/v1"
	sdkerrors "github.com/block/proto-fleet/server/sdk/v1/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cpuMinerTelemetry builds a representative MinerTelemetry for a CPU miner.
func cpuMinerTelemetry(paused bool) mujina.MinerTelemetry {
	serial := "CPU-TEST-001"
	url := "stratum+tcp://pool.example.com:3333"
	diff := 1024.0
	temp := float32(42.5)
	rpm := uint32(1200)
	pct := uint8(60)
	powerW := float32(15.0)
	voltageV := float32(1.2)
	currentA := float32(12.5)

	return mujina.MinerTelemetry{
		UptimeSecs:      300,
		Hashrate:        2_000_000,
		SharesSubmitted: 5,
		Paused:          paused,
		Boards: []mujina.BoardTelemetry{
			{
				Name:   "cpu-0",
				Model:  "cpu",
				Serial: &serial,
				Fans: []mujina.Fan{
					{Name: "fan0", RPM: &rpm, Percent: &pct},
				},
				Temperatures: []mujina.TemperatureSensor{
					{Name: "cpu", TemperatureC: &temp},
				},
				Powers: []mujina.PowerMeasurement{
					{Name: "vdd", VoltageV: &voltageV, CurrentA: &currentA, PowerW: &powerW},
				},
				Threads: []mujina.ThreadTelemetry{
					{Name: "t0", Hashrate: 1_000_000, IsActive: true},
					{Name: "t1", Hashrate: 1_000_000, IsActive: true},
				},
			},
		},
		Sources: []mujina.SourceTelemetry{
			{Name: "stratum-0", URL: &url, Difficulty: &diff},
		},
	}
}

func newTestDevice(t *testing.T, handler http.HandlerFunc) *device.Device {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := mujina.NewTestClient(srv.URL, srv.Client())
	dev, err := device.New("test-device-id", sdk.DeviceInfo{
		Host:      "127.0.0.1",
		Port:      7785,
		URLScheme: "http",
		Model:     "cpu",
	}, client)
	require.NoError(t, err)
	return dev
}

func TestDevice_ID(t *testing.T) {
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {})
	assert.Equal(t, "test-device-id", dev.ID())
}

func TestDevice_DescribeDevice(t *testing.T) {
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {})
	info, caps, err := dev.DescribeDevice(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "cpu", info.Model)
	assert.True(t, caps[sdk.CapabilityPollingHost])
	assert.True(t, caps[sdk.CapabilityMiningStart])
	assert.True(t, caps[sdk.CapabilityMiningStop])
	assert.True(t, caps[sdk.CapabilityHashrateReported])
}

func TestDevice_Status_Active(t *testing.T) {
	tel := cpuMinerTelemetry(false)
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tel)
	})

	metrics, err := dev.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-device-id", metrics.DeviceID)
	assert.Equal(t, sdk.HealthHealthyActive, metrics.Health)
	require.NotNil(t, metrics.HashrateHS)
	assert.Equal(t, float64(2_000_000), metrics.HashrateHS.Value)
	require.NotNil(t, metrics.TempC)
	assert.InDelta(t, 42.5, metrics.TempC.Value, 0.01)
	require.NotNil(t, metrics.FanRPM)
	assert.Equal(t, float64(1200), metrics.FanRPM.Value)
	require.NotNil(t, metrics.PowerW)
	assert.InDelta(t, 15.0, metrics.PowerW.Value, 0.01)
	assert.Len(t, metrics.HashBoards, 1)
	assert.Equal(t, "cpu-0", metrics.HashBoards[0].Name)
	assert.Len(t, metrics.HashBoards[0].ASICs, 2)
}

func TestDevice_Status_Paused(t *testing.T) {
	tel := cpuMinerTelemetry(true)
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tel)
	})

	metrics, err := dev.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, sdk.HealthHealthyInactive, metrics.Health)
}

func TestDevice_Status_Cached(t *testing.T) {
	calls := 0
	tel := cpuMinerTelemetry(false)
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v0/miner" {
			calls++
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tel)
	})

	_, err := dev.Status(context.Background())
	require.NoError(t, err)
	_, err = dev.Status(context.Background())
	require.NoError(t, err)

	// Second call should use the cache — only one HTTP request.
	assert.Equal(t, 1, calls)
}

func TestDevice_Status_UptimeSensor(t *testing.T) {
	tel := cpuMinerTelemetry(false)
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tel)
	})

	metrics, err := dev.Status(context.Background())
	require.NoError(t, err)
	require.Len(t, metrics.SensorMetrics, 1)
	assert.Equal(t, "uptime", metrics.SensorMetrics[0].Name)
	assert.Equal(t, float64(300), metrics.SensorMetrics[0].Value.Value)
}

func TestDevice_StartMining(t *testing.T) {
	var gotBody mujina.MinerPatchRequest
	var minerCalls int

	tel := cpuMinerTelemetry(true) // start paused
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v0/miner":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v0/miner":
			minerCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tel)
		}
	})

	err := dev.StartMining(context.Background())
	require.NoError(t, err)
	require.NotNil(t, gotBody.Paused)
	assert.False(t, *gotBody.Paused)
}

func TestDevice_StopMining(t *testing.T) {
	var gotBody mujina.MinerPatchRequest
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusOK)
		}
	})

	err := dev.StopMining(context.Background())
	require.NoError(t, err)
	require.NotNil(t, gotBody.Paused)
	assert.True(t, *gotBody.Paused)
}

func TestDevice_UnsupportedCapabilities(t *testing.T) {
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {})
	ctx := context.Background()

	assert.Error(t, dev.BlinkLED(ctx))
	assert.Error(t, dev.Reboot(ctx))
	assert.Error(t, dev.SetCoolingMode(ctx, sdk.CoolingModeAirCooled))
	_, err := dev.GetCoolingMode(ctx)
	assert.Error(t, err)
	assert.Error(t, dev.SetPowerTarget(ctx, sdk.PerformanceModeEfficiency))
	assert.Error(t, dev.UpdateMiningPools(ctx, nil))
	assert.Error(t, dev.UpdateMinerPassword(ctx, "old", "new"))
	_, _, err = dev.DownloadLogs(ctx, nil, "")
	assert.Error(t, err)
	assert.Error(t, dev.FirmwareUpdate(ctx, sdk.FirmwareFile{}))
}

func TestDevice_GetMiningPools(t *testing.T) {
	tel := cpuMinerTelemetry(false)
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tel)
	})

	pools, err := dev.GetMiningPools(context.Background())
	require.NoError(t, err)
	require.Len(t, pools, 1)
	assert.Equal(t, "stratum+tcp://pool.example.com:3333", pools[0].URL)
}

func TestDevice_GetErrors_ZeroRPMFan(t *testing.T) {
	rpm := uint32(0)
	tel := mujina.MinerTelemetry{
		Boards: []mujina.BoardTelemetry{
			{
				Name:  "cpu-0",
				Model: "cpu",
				Fans:  []mujina.Fan{{Name: "fan0", RPM: &rpm}},
			},
		},
	}
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tel)
	})

	errs, err := dev.GetErrors(context.Background())
	require.NoError(t, err)
	require.Len(t, errs.Errors, 1)
	assert.Equal(t, sdkerrors.FanFailed, errs.Errors[0].MinerError)
	assert.Equal(t, sdkerrors.SeverityMajor, errs.Errors[0].Severity)
}

func TestDevice_GetErrors_OverTemperature(t *testing.T) {
	temp := float32(98.0)
	tel := mujina.MinerTelemetry{
		Boards: []mujina.BoardTelemetry{
			{
				Name:  "cpu-0",
				Model: "cpu",
				Temperatures: []mujina.TemperatureSensor{
					{Name: "cpu", TemperatureC: &temp},
				},
			},
		},
	}
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tel)
	})

	errs, err := dev.GetErrors(context.Background())
	require.NoError(t, err)
	require.Len(t, errs.Errors, 1)
	assert.Equal(t, sdkerrors.DeviceOverTemperature, errs.Errors[0].MinerError)
	assert.Equal(t, sdkerrors.SeverityCritical, errs.Errors[0].Severity)
}

func TestDevice_GetErrors_NoErrors(t *testing.T) {
	tel := cpuMinerTelemetry(false)
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tel)
	})

	errs, err := dev.GetErrors(context.Background())
	require.NoError(t, err)
	assert.Empty(t, errs.Errors)
}

func TestDevice_Unpair(t *testing.T) {
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {})
	assert.NoError(t, dev.Unpair(context.Background()))
}

func TestDevice_Close(t *testing.T) {
	dev := newTestDevice(t, func(w http.ResponseWriter, r *http.Request) {})
	assert.NoError(t, dev.Close(context.Background()))
}
