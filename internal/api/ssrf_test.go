package api

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

// staticResolver returns a fixed set of IPs for any host.
type staticResolver struct {
	addrs []net.IPAddr
	err   error
}

func (r *staticResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	return r.addrs, r.err
}

func publicResolver() *staticResolver {
	return &staticResolver{addrs: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}}
}

func privateResolver(ip string) *staticResolver {
	return &staticResolver{addrs: []net.IPAddr{{IP: net.ParseIP(ip)}}}
}

func TestValidateProbeURL_AllowsPublicHTTPS(t *testing.T) {
	err := validateProbeURL(context.Background(), "https://api.openai.com/v1", publicResolver())
	assert.NoError(t, err)
}

func TestValidateProbeURL_AllowsPublicHTTP(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://api.example.com/v1", publicResolver())
	assert.NoError(t, err)
}

func TestValidateProbeURL_RejectsInvalidURL(t *testing.T) {
	err := validateProbeURL(context.Background(), "not a url", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid URL")
}

func TestValidateProbeURL_RejectsFTPScheme(t *testing.T) {
	err := validateProbeURL(context.Background(), "ftp://files.example.com/data", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestValidateProbeURL_RejectsFileScheme(t *testing.T) {
	err := validateProbeURL(context.Background(), "file:///etc/passwd", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestValidateProbeURL_RejectsGopherScheme(t *testing.T) {
	err := validateProbeURL(context.Background(), "gopher://evil.com", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestValidateProbeURL_RejectsLoopbackIP(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://127.0.0.1:8080/v1", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsLoopbackIPVariant(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://127.0.0.2:8080/v1", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_Rejects10xRange(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://10.0.0.1/v1", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_Rejects172Range(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://172.16.0.1/v1", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")

	err = validateProbeURL(context.Background(), "http://172.31.255.255/v1", publicResolver())
	assert.Error(t, err)
}

func TestValidateProbeURL_Rejects192168Range(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://192.168.1.1/v1", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsLinkLocal(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://169.254.169.254/latest/meta-data/", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsIPv6Loopback(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://[::1]:8080/v1", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsIPv6UniqueLocal(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://[fd00::1]:8080/v1", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsIPv6LinkLocal(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://[fe80::1]:8080/v1", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsDNSRebindToLoopback(t *testing.T) {
	err := validateProbeURL(context.Background(), "https://evil.example.com/v1", privateResolver("127.0.0.1"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsDNSRebindTo10x(t *testing.T) {
	err := validateProbeURL(context.Background(), "https://evil.example.com/v1", privateResolver("10.0.0.5"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsDNSRebindTo192168(t *testing.T) {
	err := validateProbeURL(context.Background(), "https://evil.example.com/v1", privateResolver("192.168.0.1"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsDNSRebindToLinkLocal(t *testing.T) {
	err := validateProbeURL(context.Background(), "https://evil.example.com/v1", privateResolver("169.254.169.254"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_Allows172OutsidePrivateRange(t *testing.T) {
	// 172.32.0.1 is outside the 172.16.0.0/12 private range.
	r := &staticResolver{addrs: []net.IPAddr{{IP: net.ParseIP("172.32.0.1")}}}
	err := validateProbeURL(context.Background(), "https://public.example.com/v1", r)
	assert.NoError(t, err)
}

func TestValidateProbeURL_AllowsPublicIPLiteral(t *testing.T) {
	err := validateProbeURL(context.Background(), "https://93.184.216.34/v1", publicResolver())
	assert.NoError(t, err)
}

func TestValidateProbeURL_RejectsBroadcastAddress(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://255.255.255.255/v1", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsIPv4Multicast(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://224.0.0.1/v1", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsIPv6Multicast(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://[ff02::1]:8080/v1", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsIPv4MappedIPv6(t *testing.T) {
	// ::ffff:127.0.0.1 is an IPv4-mapped IPv6 address that should be blocked.
	err := validateProbeURL(context.Background(), "http://[::ffff:127.0.0.1]:8080/v1", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsIPv4MappedIPv6_10x(t *testing.T) {
	err := validateProbeURL(context.Background(), "http://[::ffff:10.0.0.1]:8080/v1", publicResolver())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsDNSRebindToIPv4MappedIPv6(t *testing.T) {
	r := &staticResolver{addrs: []net.IPAddr{{IP: net.ParseIP("::ffff:192.168.1.1")}}}
	err := validateProbeURL(context.Background(), "https://evil.example.com/v1", r)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private/internal")
}

func TestValidateProbeURL_RejectsDNSResolutionFailure(t *testing.T) {
	r := &staticResolver{err: &net.DNSError{Err: "no such host", Name: "nonexistent.invalid"}}
	err := validateProbeURL(context.Background(), "https://nonexistent.invalid/v1", r)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve")
}
