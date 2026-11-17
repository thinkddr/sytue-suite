// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
)

// ServiceCall accepts only the fixed service transport frame/body operations.
// All of them retain Storage's real pre/post device identity guard.
func (s *Storage) ServiceCall(ctx context.Context, method Method, raw []byte) ([]byte, error) {
	switch method {
	case ServiceRequest, ServiceRead, ServiceClose:
	default:
		return nil, ErrRejected
	}
	if method == ServiceClose {
		limit, _ := Limits(method)
		if len(raw) > limit.Request {
			return nil, ErrRejected
		}
		var value struct{ Stream string }
		if json.Unmarshal(raw, &value) != nil {
			return nil, ErrRejected
		}
		return nil, closeHostReplyHandle(ctx, s.port, s.identity.Scope, ServiceRequest, value.Stream)
	}
	return s.callBytesOwned(ctx, method, raw)
}

// AddServiceSource binds a single forward-only original to this exact SDK port.
func (s *Storage) AddServiceSource(ctx context.Context, reader io.Reader, size int64) (string, func(), error) {
	if reader == nil {
		return "", nil, ErrRejected
	}
	return s.sources.Add(ctx, s.port, s.identity, &forwardSource{reader: reader, size: size, hash: sha256.New()}, size)
}
