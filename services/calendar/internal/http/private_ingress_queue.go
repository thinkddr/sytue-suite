// SPDX-License-Identifier: Apache-2.0
package calendarhttp

import (
	"errors"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

func mountCalendarPrivateIngress(api chi.Router, port httpx.PrivateIngressPort, maxBytes int64) {
	handlers := httpx.NewPrivateIngressHandlers(httpx.PrivateIngressHTTPConfig{Product: "calendar", Port: port, Identity: authn.PrivateStorageIdentity, MaxBodyBytes: maxBytes, MapError: calendarIngressError})
	handlers.Mount(api)
}

func calendarIngressError(err error) *httpx.Error {
	if shared := httpx.PrivateIngressError(err); shared != nil {
		return shared
	}
	if errors.Is(err, store.ErrPrivateStorageIngress) || errors.Is(err, callaccess.ErrUnavailable) {
		return httpx.ErrUnavailable.WithCode("private_ingress_required")
	}
	return calendarSourceError(err)
}
