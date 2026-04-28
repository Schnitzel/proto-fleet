package mujina_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/block/proto-fleet/plugin/mujina/pkg/mujina"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient builds a mujina.Client backed by the given httptest.Server.
func newTestClient(t *testing.T, srv *httptest.Server) *mujina.Client {
	t.Helper()
	return mujina.NewTestClient(srv.URL, srv.Client())
}

func TestHealth_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v0/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	err := newTestClient(t, srv).Health(context.Background())
	require.NoError(t, err)
}

func TestHealth_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	err := newTestClient(t, srv).Health(context.Background())
	require.Error(t, err)
}

func TestGetMiner(t *testing.T) {
	paused := false
	serial := "CPU-001"
	url := "stratum+tcp://pool.example.com:3333"
	difficulty := 2048.0

	payload := mujina.MinerTelemetry{
		UptimeSecs:      120,
		Hashrate:        1234567,
		SharesSubmitted: 42,
		Paused:          paused,
		Boards: []mujina.BoardTelemetry{
			{
				Name:   "cpu-0",
				Model:  "cpu",
				Serial: &serial,
				Fans:   []mujina.Fan{},
				Temperatures: []mujina.TemperatureSensor{
					{Name: "cpu", TemperatureC: ptr32(45.5)},
				},
				Powers:  []mujina.PowerMeasurement{},
				Threads: []mujina.ThreadTelemetry{{Name: "t0", Hashrate: 1234567, IsActive: true}},
			},
		},
		Sources: []mujina.SourceTelemetry{
			{Name: "stratum-0", URL: &url, Difficulty: &difficulty},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v0/miner", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	got, err := newTestClient(t, srv).GetMiner(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, uint64(120), got.UptimeSecs)
	assert.Equal(t, uint64(1234567), got.Hashrate)
	assert.Equal(t, uint64(42), got.SharesSubmitted)
	assert.False(t, got.Paused)
	assert.Len(t, got.Boards, 1)
	assert.Equal(t, "cpu-0", got.Boards[0].Name)
	assert.Equal(t, "cpu", got.Boards[0].Model)
	assert.Equal(t, &serial, got.Boards[0].Serial)
	assert.Len(t, got.Sources, 1)
	assert.Equal(t, &url, got.Sources[0].URL)
	assert.Equal(t, &difficulty, got.Sources[0].Difficulty)
}

func TestGetMiner_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv).GetMiner(context.Background())
	require.Error(t, err)
}

func TestPatchMiner_Pause(t *testing.T) {
	paused := true
	var gotBody mujina.MinerPatchRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v0/miner", r.URL.Path)
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := newTestClient(t, srv).PatchMiner(context.Background(), mujina.MinerPatchRequest{Paused: &paused})
	require.NoError(t, err)
	require.NotNil(t, gotBody.Paused)
	assert.True(t, *gotBody.Paused)
}

func TestPatchMiner_Resume(t *testing.T) {
	paused := false
	var gotBody mujina.MinerPatchRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := newTestClient(t, srv).PatchMiner(context.Background(), mujina.MinerPatchRequest{Paused: &paused})
	require.NoError(t, err)
	require.NotNil(t, gotBody.Paused)
	assert.False(t, *gotBody.Paused)
}

func TestGetBoards(t *testing.T) {
	boards := []mujina.BoardTelemetry{
		{Name: "board-0", Model: "bitaxe-gamma"},
		{Name: "board-1", Model: "bitaxe-gamma"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v0/boards", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(boards)
	}))
	defer srv.Close()

	got, err := newTestClient(t, srv).GetBoards(context.Background())
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Equal(t, "board-0", got[0].Name)
}

func TestGetSources(t *testing.T) {
	url := "stratum+tcp://pool.example.com:3333"
	sources := []mujina.SourceTelemetry{
		{Name: "stratum-0", URL: &url},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v0/sources", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sources)
	}))
	defer srv.Close()

	got, err := newTestClient(t, srv).GetSources(context.Background())
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Equal(t, &url, got[0].URL)
}

func ptr32(v float32) *float32 { return &v }
