package proxy

import (
	"context"

	"github.com/centrifugal/centrifugo/v6/internal/proxyproto"
)

// CacheEmptyProxy allows to send NotifyCacheEmpty requests.
type CacheEmptyProxy interface {
	ProxyCacheEmpty(context.Context, *proxyproto.NotifyCacheEmptyRequest) (*proxyproto.NotifyCacheEmptyResponse, error)
	// Protocol for metrics and logging.
	Protocol() string
	// UseBase64 for bytes in requests from Centrifugo to application backend.
	UseBase64() bool
	// IncludeMeta ...
	IncludeMeta() bool
}
