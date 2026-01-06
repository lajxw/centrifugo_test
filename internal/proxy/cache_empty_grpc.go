package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"

	"google.golang.org/grpc"
)

// GRPCCacheEmptyProxy ...
type GRPCCacheEmptyProxy struct {
	config Config
	client proxyproto.CentrifugoProxyClient
}

var _ CacheEmptyProxy = (*GRPCCacheEmptyProxy)(nil)

// NewGRPCCacheEmptyProxy ...
func NewGRPCCacheEmptyProxy(name string, p Config) (*GRPCCacheEmptyProxy, error) {
	host, err := getGrpcHost(p.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("error getting grpc host: %v", err)
	}
	dialOpts, err := getDialOpts(name, p)
	if err != nil {
		return nil, fmt.Errorf("error creating GRPC dial options: %v", err)
	}
	conn, err := grpc.NewClient(host, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("error connecting to GRPC proxy server: %v", err)
	}
	return &GRPCCacheEmptyProxy{
		config: p,
		client: proxyproto.NewCentrifugoProxyClient(conn),
	}, nil
}

// ProxyCacheEmpty proxies NotifyCacheEmpty to application backend.
func (p *GRPCCacheEmptyProxy) ProxyCacheEmpty(ctx context.Context, req *proxyproto.NotifyCacheEmptyRequest) (*proxyproto.NotifyCacheEmptyResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout.ToDuration())
	defer cancel()
	return p.client.NotifyCacheEmpty(grpcRequestContext(ctx, p.config), req)
}

// Protocol ...
func (p *GRPCCacheEmptyProxy) Protocol() string {
	return "grpc"
}

// UseBase64 ...
func (p *GRPCCacheEmptyProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *GRPCCacheEmptyProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
