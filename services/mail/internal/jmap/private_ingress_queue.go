// SPDX-License-Identifier: Apache-2.0
package jmap

import (
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// WithPrivateIngressQueue supplies control/data transport only. Readiness stays
// separate until the real authorized-device publication/recovery proof passes.
func (a *API) WithPrivateIngressQueue(port httpx.PrivateIngressPort) *API {
	a.ingressQueue = port
	return a
}

func (a *API) privateIngressHandlers() *httpx.PrivateIngressHandlers {
	maxCipher := a.cfg.MaxMessageBytes*2 + (128 << 10)
	if maxCipher < 1 || maxCipher > storage.MaxFileBytes {
		maxCipher = storage.MaxFileBytes
	}
	return httpx.NewPrivateIngressHandlers(httpx.PrivateIngressHTTPConfig{Product: "mail", Port: a.ingressQueue, Identity: authn.PrivateStorageIdentity, MaxBodyBytes: maxCipher, MapError: mailIngressError})
}

func mailIngressError(err error) *httpx.Error {
	if shared := httpx.PrivateIngressError(err); shared != nil {
		return shared
	}
	switch {
	case errors.Is(err, pg.ErrPrivateIngressMissing), errors.Is(err, callaccess.ErrUnavailable):
		return httpx.ErrUnavailable.WithCode("private_ingress_required")
	case errors.Is(err, pg.ErrPrivateStorageScope):
		return httpx.ErrConflict.WithCode("storage_identity_changed")
	default:
		var failure *httpx.Error
		if errors.As(privateMailError(err), &failure) {
			return failure
		}
		return httpx.ErrInternal
	}
}
