package proxy

import (
	"context"
	"fmt"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// CacheEmptyRequestHTTP ...
type CacheEmptyRequestHTTP struct {
	Channel string `json:"channel"`
}

// HTTPCacheEmptyProxy ...
type HTTPCacheEmptyProxy struct {
	config     Config
	httpCaller HTTPCaller
}

var _ CacheEmptyProxy = (*HTTPCacheEmptyProxy)(nil)

// NewHTTPCacheEmptyProxy ...
func NewHTTPCacheEmptyProxy(p Config) (*HTTPCacheEmptyProxy, error) {
	httpClient, err := proxyHTTPClient(p, "cache_empty_proxy")
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %w", err)
	}
	return &HTTPCacheEmptyProxy{
		httpCaller: NewHTTPCaller(httpClient),
		config:     p,
	}, nil
}

// ProxyCacheEmpty proxies NotifyCacheEmpty to application backend.
func (p *HTTPCacheEmptyProxy) ProxyCacheEmpty(ctx context.Context, req *proxyproto.NotifyCacheEmptyRequest) (*proxyproto.NotifyCacheEmptyResponse, error) {
	data, err := httpEncoder.EncodeNotifyCacheEmptyRequest(req)
	if err != nil {
		return nil, err
	}
	respData, err := p.httpCaller.CallHTTP(ctx, p.config.Endpoint, httpRequestHeaders(ctx, p.config), data)
	if err != nil {
		return transformCacheEmptyResponse(err, p.config.HTTP.StatusToCodeTransforms)
	}
	return httpDecoder.DecodeNotifyCacheEmptyResponse(respData)
}

// Protocol ...
func (p *HTTPCacheEmptyProxy) Protocol() string {
	return "http"
}

// UseBase64 ...
func (p *HTTPCacheEmptyProxy) UseBase64() bool {
	return p.config.BinaryEncoding
}

// IncludeMeta ...
func (p *HTTPCacheEmptyProxy) IncludeMeta() bool {
	return p.config.IncludeConnectionMeta
}
