package wyoming

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/grandcat/zeroconf"
)

// ZeroconfService manages mDNS/Zeroconf registration for Wyoming service discovery.
type ZeroconfService struct {
	server *zeroconf.Server
	logger *slog.Logger
}

// ZeroconfConfig holds configuration for Zeroconf registration.
type ZeroconfConfig struct {
	// ServiceName is the mDNS instance name. Defaults to the hostname.
	ServiceName string
	// Port is the Wyoming TCP port to advertise.
	Port int
}

// RegisterZeroconf registers the Wyoming service via mDNS/Zeroconf.
// The service type is "_wyoming._tcp" on the "local." domain.
// Call Shutdown on the returned ZeroconfService to deregister.
func RegisterZeroconf(cfg ZeroconfConfig, logger *slog.Logger) (*ZeroconfService, error) {
	if logger == nil {
		logger = slog.Default()
	}

	name := cfg.ServiceName
	if name == "" {
		var err error
		name, err = os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("zeroconf: get hostname: %w", err)
		}
	}

	server, err := zeroconf.Register(name, "_wyoming._tcp", "local.", cfg.Port, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("zeroconf: register service: %w", err)
	}

	logger.Info("zeroconf registered", "name", name, "service", "_wyoming._tcp.local.", "port", cfg.Port)

	return &ZeroconfService{
		server: server,
		logger: logger,
	}, nil
}

// Shutdown deregisters the Zeroconf service.
func (z *ZeroconfService) Shutdown() {
	if z.server != nil {
		z.server.Shutdown()
		z.logger.Info("zeroconf deregistered")
	}
}
