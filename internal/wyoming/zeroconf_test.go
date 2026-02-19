package wyoming

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterZeroconf_DefaultHostname(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := RegisterZeroconf(ZeroconfConfig{Port: 10300}, logger)
	require.NoError(t, err)
	require.NotNil(t, svc)
	defer svc.Shutdown()
}

func TestRegisterZeroconf_CustomName(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := RegisterZeroconf(ZeroconfConfig{
		ServiceName: "my-meadowlark",
		Port:        10300,
	}, logger)
	require.NoError(t, err)
	require.NotNil(t, svc)
	defer svc.Shutdown()
}

func TestRegisterZeroconf_NilLogger(t *testing.T) {
	svc, err := RegisterZeroconf(ZeroconfConfig{
		ServiceName: "test-service",
		Port:        10300,
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, svc)
	defer svc.Shutdown()
}

func TestZeroconfService_Shutdown_Idempotent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := RegisterZeroconf(ZeroconfConfig{
		ServiceName: "test-idempotent",
		Port:        10300,
	}, logger)
	require.NoError(t, err)

	// Multiple shutdowns should not panic.
	svc.Shutdown()
	svc.Shutdown()
}

func TestZeroconfService_Shutdown_NilServer(t *testing.T) {
	// Should not panic.
	svc := &ZeroconfService{logger: slog.Default()}
	assert.NotPanics(t, func() {
		svc.Shutdown()
	})
}
