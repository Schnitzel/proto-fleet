package driver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/block/proto-fleet/plugin/mujina/internal/driver"
	"github.com/block/proto-fleet/plugin/mujina/pkg/mujina"
	sdk "github.com/block/proto-fleet/server/sdk/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeMujina starts an httptest server that mimics the Mujina API.
func fakeMujina(t *testing.T) (host string, port string, close func()) {
	t.Helper()
	serial := "CPU-TEST-001"
	tel := mujina.MinerTelemetry{
		UptimeSecs: 60,
		Hashrate:   500_000,
		Boards: []mujina.BoardTelemetry{
			{Name: "cpu-0", Model: "cpu", Serial: &serial},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v0/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case "/api/v0/miner":
			_ = json.NewEncoder(w).Encode(tel)
		case "/api/v0/boards":
			_ = json.NewEncoder(w).Encode(tel.Boards)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	h, p, _ := net.SplitHostPort(srv.Listener.Addr().String())
	return h, p, srv.Close
}

func TestDriver_Handshake(t *testing.T) {
	d := driver.New()
	id, err := d.Handshake(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "mujina", id.DriverName)
	assert.NotEmpty(t, id.APIVersion)
}

func TestDriver_DescribeDriver(t *testing.T) {
	d := driver.New()
	id, caps, err := d.DescribeDriver(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "mujina", id.DriverName)
	assert.True(t, caps[sdk.CapabilityDiscovery])
	assert.True(t, caps[sdk.CapabilityPairing])
	assert.True(t, caps[sdk.CapabilityMiningStart])
	assert.True(t, caps[sdk.CapabilityMiningStop])
}

func TestDriver_GetDiscoveryPorts(t *testing.T) {
	d := driver.New()
	ports := d.GetDiscoveryPorts(context.Background())
	require.Len(t, ports, 1)
	assert.Equal(t, fmt.Sprintf("%d", mujina.DefaultPort), ports[0])
}

func TestDriver_DiscoverDevice(t *testing.T) {
	host, port, close := fakeMujina(t)
	defer close()

	d := driver.New()
	info, err := d.DiscoverDevice(context.Background(), host, port)
	require.NoError(t, err)
	assert.Equal(t, host, info.Host)
	assert.Equal(t, "cpu", info.Model)
	assert.Equal(t, "256 Foundation / Mujina", info.Manufacturer)
	assert.Equal(t, "CPU-TEST-001", info.SerialNumber)
	assert.Equal(t, "http", info.URLScheme)
}

func TestDriver_DiscoverDevice_NotMujina(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h, p, _ := net.SplitHostPort(srv.Listener.Addr().String())
	d := driver.New()
	_, err := d.DiscoverDevice(context.Background(), h, p)
	require.Error(t, err)
}

func TestDriver_DiscoverDevice_DefaultPort(t *testing.T) {
	host, port, close := fakeMujina(t)
	defer close()

	d := driver.New()
	// Passing the actual port — driver should use it.
	info, err := d.DiscoverDevice(context.Background(), host, port)
	require.NoError(t, err)
	assert.Equal(t, host, info.Host)
}

func TestDriver_PairDevice(t *testing.T) {
	host, port, close := fakeMujina(t)
	defer close()

	// First discover to get proper port.
	d := driver.New()
	info, err := d.DiscoverDevice(context.Background(), host, port)
	require.NoError(t, err)

	paired, err := d.PairDevice(context.Background(), info, sdk.SecretBundle{})
	require.NoError(t, err)
	assert.Equal(t, host, paired.Host)
	assert.Equal(t, "cpu", paired.Model)
}

func TestDriver_GetDefaultCredentials(t *testing.T) {
	d := driver.New()
	creds := d.GetDefaultCredentials(context.Background(), "256 Foundation / Mujina", "")
	require.Len(t, creds, 1)
	assert.Empty(t, creds[0].Username)
	assert.Empty(t, creds[0].Password)
}

func TestDriver_NewDevice(t *testing.T) {
	host, port, close := fakeMujina(t)
	defer close()

	d := driver.New()
	info, err := d.DiscoverDevice(context.Background(), host, port)
	require.NoError(t, err)

	result, err := d.NewDevice(context.Background(), "dev-001", info, sdk.SecretBundle{})
	require.NoError(t, err)
	require.NotNil(t, result.Device)
	assert.Equal(t, "dev-001", result.Device.ID())
}
