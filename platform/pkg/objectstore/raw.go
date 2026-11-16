// SPDX-License-Identifier: Apache-2.0
package objectstore

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// The standard transport honors an explicit Accept-Encoding and cancels the
// request body on context cancellation. Unknown RoundTrippers cannot prove it.
func rawHTTPTransport(transport http.RoundTripper) bool {
	if transport == nil {
		return true
	} // MinIO's default transport disables decompression.
	t, ok := transport.(*http.Transport)
	return ok && t != nil
}

func rawHTTPClient(client *http.Client) bool {
	if client == nil || client.Transport == nil {
		return false
	}
	return rawHTTPTransport(client.Transport)
}

type rawBody struct {
	io.ReadCloser
	cancel context.CancelFunc
	closed atomic.Bool
	once   sync.Once
	err    error
}

func (b *rawBody) Read(p []byte) (int, error) {
	if b.closed.Load() {
		return 0, net.ErrClosed
	}
	n, err := b.ReadCloser.Read(p)
	if b.closed.Load() {
		return n, net.ErrClosed
	}
	return n, err
}

func (b *rawBody) Close() error {
	b.once.Do(func() {
		b.closed.Store(true)
		b.cancel()
		b.err = b.ReadCloser.Close()
	})
	return b.err
}

// The provider must honor identity: a content-encoded or transparently
// decompressed response is ambiguous and cannot prove stored object bytes.
func acceptRawHTTP(res *http.Response, cancel context.CancelFunc) (io.ReadCloser, error) {
	if res.Uncompressed || !rawIdentityEncoding(res.Header) {
		cancel()
		_ = res.Body.Close()
		return nil, ErrRawUnsupported
	}
	return &rawBody{ReadCloser: res.Body, cancel: cancel}, nil
}

func rawIdentityEncoding(headers http.Header) bool {
	for _, value := range headers.Values("Content-Encoding") {
		encoding := strings.TrimSpace(value)
		if encoding != "" && !strings.EqualFold(encoding, "identity") {
			return false
		}
	}
	return true
}
