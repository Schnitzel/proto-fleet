// Package mujina provides a client and types for the Mujina miner REST API.
//
// API reference: https://github.com/256foundation/mujina/blob/main/docs/api.md
// All endpoints live under /api/v0/. The API is unauthenticated.
package mujina

// MinerTelemetry is the full miner state snapshot returned by GET /api/v0/miner.
type MinerTelemetry struct {
	UptimeSecs      uint64          `json:"uptime_secs"`
	Hashrate        uint64          `json:"hashrate"` // hashes per second
	SharesSubmitted uint64          `json:"shares_submitted"`
	Paused          bool            `json:"paused"`
	Boards          []BoardTelemetry `json:"boards"`
	Sources         []SourceTelemetry `json:"sources"`
}

// BoardTelemetry is the per-board snapshot returned by GET /api/v0/boards or /boards/{name}.
type BoardTelemetry struct {
	Name         string              `json:"name"`
	Model        string              `json:"model"`
	Serial       *string             `json:"serial,omitempty"`
	Fans         []Fan               `json:"fans"`
	Temperatures []TemperatureSensor `json:"temperatures"`
	Powers       []PowerMeasurement  `json:"powers"`
	Threads      []ThreadTelemetry   `json:"threads"`
}

// Fan describes a single cooling fan on a board.
type Fan struct {
	Name          string  `json:"name"`
	RPM           *uint32 `json:"rpm,omitempty"`
	Percent       *uint8  `json:"percent,omitempty"`
	TargetPercent *uint8  `json:"target_percent,omitempty"`
}

// TemperatureSensor describes a temperature reading from a named sensor.
// TemperatureC is null when the hardware read failed or the sensor is absent.
type TemperatureSensor struct {
	Name          string   `json:"name"`
	TemperatureC  *float32 `json:"temperature_c,omitempty"`
}

// PowerMeasurement holds voltage, current and power from a single measurement point.
type PowerMeasurement struct {
	Name      string   `json:"name"`
	VoltageV  *float32 `json:"voltage_v,omitempty"`
	CurrentA  *float32 `json:"current_a,omitempty"`
	PowerW    *float32 `json:"power_w,omitempty"`
}

// ThreadTelemetry describes per-thread (ASIC thread) metrics.
type ThreadTelemetry struct {
	Name     string `json:"name"`
	Hashrate uint64 `json:"hashrate"` // hashes per second
	IsActive bool   `json:"is_active"`
}

// SourceTelemetry describes a job source (e.g. a Stratum pool connection).
type SourceTelemetry struct {
	Name       string   `json:"name"`
	URL        *string  `json:"url,omitempty"`
	Difficulty *float64 `json:"difficulty,omitempty"`
}

// MinerPatchRequest is the body for PATCH /api/v0/miner.
// Only fields present in the JSON are applied; omitted fields are left unchanged.
type MinerPatchRequest struct {
	Paused *bool `json:"paused,omitempty"`
}
