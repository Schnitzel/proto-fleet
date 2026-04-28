//go:build live

package driver_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/block/proto-fleet/plugin/mujina/internal/driver"
	sdk "github.com/block/proto-fleet/server/sdk/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLive runs against a real Mujina instance on localhost:7785.
// Enable with: go test -tags live ./internal/driver/
func TestLive_DiscoverAndStatus(t *testing.T) {
	ctx := context.Background()
	d := driver.New()

	id, err := d.Handshake(ctx)
	require.NoError(t, err)
	fmt.Printf("Driver: %s v%s\n", id.DriverName, id.APIVersion)

	info, err := d.DiscoverDevice(ctx, "127.0.0.1", "7785")
	require.NoError(t, err)
	fmt.Printf("Discovered: model=%s serial=%q\n", info.Model, info.SerialNumber)

	res, err := d.NewDevice(ctx, "live-test-001", info, sdk.SecretBundle{})
	require.NoError(t, err)
	dev := res.Device

	metrics, err := dev.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, "live-test-001", metrics.DeviceID)
	fmt.Printf("Health: %v\n", metrics.Health)
	if len(metrics.SensorMetrics) > 0 {
		fmt.Printf("Uptime: %.0f secs\n", metrics.SensorMetrics[0].Value.Value)
	}

	pools, err := dev.GetMiningPools(ctx)
	require.NoError(t, err)
	fmt.Printf("Pools: %d\n", len(pools))
	for _, p := range pools {
		fmt.Printf("  [%d] url=%q user=%q\n", p.Priority, p.URL, p.Username)
	}

	errs, err := dev.GetErrors(ctx)
	require.NoError(t, err)
	fmt.Printf("Errors: %d\n", len(errs.Errors))
}
