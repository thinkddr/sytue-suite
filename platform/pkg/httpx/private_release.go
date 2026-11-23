// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ReauthenticationMaxAge es el `max_age` (segundos) que se pide a Sytue ID
// cuando falta una identificación reciente: el de authn.FrescuraReautenticacion.
const ReauthenticationMaxAge = 300

// Reauthentication is the proof of a recent identification taken from the
// verified claims (authn.PrivateStorageReauthentication). httpx cannot import
// authn, so the service passes the adapter.
type Reauthentication struct {
	AuthTime time.Time
	AMR      []string
}

// errReauthentication is RFC 9470's answer, the same code Sytue ID uses, so a
// client sends the person to /authorize with max_age and repeats the request.
var errReauthentication = NewError(http.StatusUnauthorized, "insufficient_user_authentication",
	"This action needs you to confirm your identity again.")

// FailReauthentication writes the RFC 9470 response with the max_age to ask for.
func FailReauthentication(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_user_authentication", `+
		`error_description="reauthentication required", max_age=`+strconv.Itoa(ReauthenticationMaxAge))
	Fail(w, r, errReauthentication)
}

// PrivateStorageReleaseConfig mounts «soltar la fuente» (ADR 0086) for one
// product: POST /private-storage/migration/release. Release runs the domain's
// tenant transaction around pg.ReleasePrivateStorageState.
type PrivateStorageReleaseConfig struct {
	Product         string
	Identity        func(context.Context) (PrivateSourceIdentity, bool)
	Reauthenticated func(context.Context) (Reauthentication, bool)
	Release         func(context.Context, storage.Scope, uuid.UUID, Reauthentication) error
	MapError        func(error) *Error
	// Notify tells the person (best effort, after the commit). Optional.
	Notify func(context.Context, storage.Scope)
	Logger *slog.Logger
}

// PrivateStorageReleaseResponse is what a successful release returns.
type PrivateStorageReleaseResponse struct {
	Mode          string `json:"mode"`
	ReleasedEpoch string `json:"released_epoch"`
}

// NewPrivateStorageRelease checks, in this order: the owner (no delegation),
// the storage scope header, the explicit confirmation and epoch in the body,
// and a recent identification with the account's second factor. Only then the
// domain releases. The confirmation is required here too, not only in the UI:
// the operation discards what devices wrote after the freeze.
func NewPrivateStorageRelease(cfg PrivateStorageReleaseConfig) (http.HandlerFunc, error) {
	if !storage.Product(cfg.Product) || cfg.Identity == nil || cfg.Reauthenticated == nil || cfg.Release == nil || cfg.MapError == nil {
		return nil, errInvalidPrivateRelease
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		scope, ok := privateStorageHTTPScope(w, r, cfg.Product, cfg.Identity)
		if !ok {
			return
		}
		var request struct {
			Epoch   string `json:"epoch"`
			Discard bool   `json:"discard_private_writes"`
		}
		if DecodeJSONLimit(w, r, &request, 256) != nil || !storage.MigrationEpoch(request.Epoch) {
			Fail(w, r, ErrBadRequest)
			return
		}
		epoch, err := uuid.Parse(request.Epoch)
		if err != nil {
			Fail(w, r, ErrBadRequest)
			return
		}
		if !request.Discard {
			Fail(w, r, ErrBadRequest.WithCode("private_storage_release_unconfirmed"))
			return
		}
		proof, ok := cfg.Reauthenticated(r.Context())
		if !ok || proof.AuthTime.IsZero() {
			FailReauthentication(w, r)
			return
		}
		if err := cfg.Release(r.Context(), scope, epoch, proof); err != nil {
			privateStorageHTTPFailure(w, r, err, cfg.MapError)
			return
		}
		// Auditoría: la fila de estado guarda la entrada (releases); esto la deja
		// también en el registro del servicio, sin datos personales.
		logger.InfoContext(r.Context(), "auditoría: fuente privada soltada",
			"product", scope.Product, "tenant", scope.Tenant, "sub", scope.Subject,
			"epoch", epoch.String(), "auth_time", proof.AuthTime.UTC())
		if cfg.Notify != nil {
			cfg.Notify(context.WithoutCancel(r.Context()), scope)
		}
		JSON(w, r, http.StatusOK, PrivateStorageReleaseResponse{Mode: "legacy", ReleasedEpoch: epoch.String()})
	}, nil
}

var errInvalidPrivateRelease = errors.New("httpx: invalid private storage release configuration")
