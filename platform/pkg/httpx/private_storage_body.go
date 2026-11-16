// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Immutable migration sources and ongoing opaque ingress use the same bounded,
// cancellable stream and final-chunk digest guard. No product copies that IO.
func servePrivateStorageBody(w http.ResponseWriter, r *http.Request, row storage.StreamSourceRow, body io.ReadCloser, err error, valid bool, maxBytes int64, mapping func(error) *Error) {
	closeBody := sync.OnceFunc(func() {
		if body != nil {
			_ = body.Close()
		}
	})
	defer closeBody()
	if err != nil {
		privateStorageHTTPFailure(w, r, err, mapping)
		return
	}
	if body == nil || row.Validate() != nil || !valid || row.Size > maxBytes {
		privateSourceInvalidResponse(w, r)
		return
	}
	// OnceFunc protects readers that are cancelled while Read is blocked. The
	// domain port's Close contract, not a lock around foreign IO, interrupts it.
	stop := context.AfterFunc(r.Context(), closeBody)
	defer stop()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(row.Size, 10))
	w.Header().Set("X-Sytue-Source-SHA256", row.SHA256)
	written, err := writePrivateSource(r.Context(), w, body, row)
	if err != nil {
		if written > 0 {
			panic(http.ErrAbortHandler)
		}
		w.Header().Del("Content-Length")
		w.Header().Del("X-Sytue-Source-SHA256")
		privateSourceInvalidResponse(w, r)
		return
	}
	if written == 0 {
		w.WriteHeader(http.StatusOK)
	}
}
