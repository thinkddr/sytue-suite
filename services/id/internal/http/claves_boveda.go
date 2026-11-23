// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

// El registro de las claves de firma de las bóvedas (ADR 0087, entrega 2).
//
// Cada bóveda post-cuántica de una persona (por organización y producto) tiene una
// clave de firma híbrida dentro de su secreto. Sytue ID guarda su parte PÚBLICA y
// es el ancla: un servicio que recibe una acción firmada por la bóveda (el correo
// entrante) la verifica contra la clave activa de aquí. Cuatro maneras de llegar a
// una generación nueva, y solo cuatro:
//
//   - alta: la primera clave de esa bóveda, con la sesión de la persona.
//   - continuidad: la nueva viene firmada también por la activa. Quien tenía la
//     clave de antes aprueba la de ahora; no hace falta nada más.
//   - vaciado: la nueva sustituye a una que ya no se puede abrir (se perdió la
//     clave de desbloqueo). Es la salida que la autoridad de Matrix no tenía, y por
//     eso cuesta: identificación de hace menos de cinco minutos, con el segundo
//     factor si la cuenta lo tiene, una línea de auditoría y un correo a la persona.
//   - traspaso: solo el correo. La bóveda pasa a ser la autoridad de quien ya no
//     puede abrir su correo de Matrix. Cuesta lo mismo que un vaciado, también en
//     un alta, porque Rooms la toma como prueba de esa identificación reciente.
//
// En las tres, la clave nueva firma la declaración de su generación: nadie
// registra una clave pública que no tiene. Las firmas se verifican aquí, dentro de
// la transacción que bloquea la bóveda, sobre la activa de ESE momento.

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

const (
	motivoAlta        = "alta"
	motivoContinuidad = "continuidad"
	motivoVaciado     = "vaciado"
	// motivoTraspaso: la bóveda pasa a ser la autoridad del correo de quien ya no
	// puede abrir su correo de Matrix (ADR 0087). Rooms solo da la autoridad de la
	// bóveda sobre una de Matrix si la clave activa tiene este motivo y es
	// posterior al último cambio de la autoridad de Matrix.
	motivoTraspaso = "traspaso"
)

// productosConTraspaso son los productos cuya entrada puede pasar de Matrix a la
// bóveda. Es la misma lista que la de Rooms (`productosConEntradaDeBoveda`).
var productosConTraspaso = map[string]bool{"mail": true}

var (
	// errClaveBovedaReautenticar: un vaciado sin identificación reciente. La app
	// manda a la persona a identificarse (max_age) y repite la misma petición.
	errClaveBovedaReautenticar = httpx.ErrForbidden.WithCode("vault_key_reauth").
					WithMessage("Replacing a vault key that cannot be opened needs a fresh sign-in.")
	// errClaveBovedaDesfasada: la declaración se firmó para otra generación (otro
	// dispositivo registró antes). La app vuelve a leer la activa y repite.
	errClaveBovedaDesfasada = httpx.ErrConflict.WithCode("vault_key_stale").
				WithMessage("The vault key changed meanwhile; read it again.")
	errClaveBovedaPrueba = httpx.ErrBadRequest.WithCode("vault_key_proof").
				WithMessage("The vault key signatures do not verify.")
	// errClaveBovedaTraspaso: un traspaso en un producto que no lo admite, o que
	// además trae la aprobación de la anterior (un traspaso no es continuidad).
	errClaveBovedaTraspaso = httpx.ErrBadRequest.WithCode("vault_key_handover").
				WithMessage("This vault cannot take over from Matrix.")
)

// solicitudClaveBoveda es el cuerpo de PUT /api/v1/me/vault-keys/{product}.
type solicitudClaveBoveda struct {
	PublicKey     recoverywrap.PQVaultVerifyKey `json:"public_key"`
	PreviousKeyID string                        `json:"previous_key_id"`
	Proof         []byte                        `json:"proof"`
	Continuity    []byte                        `json:"continuity,omitempty"`
	// Handover es la marca de traspaso: la bóveda sustituye a la autoridad de
	// Matrix del correo. Exige SIEMPRE identificación reciente, también en un alta.
	Handover bool `json:"handover,omitempty"`
}

// antesDelTraspaso resuelve lo que va antes de verificar firmas: la misma clave
// otra vez no cambia nada, y repetir un traspaso (una respuesta perdida) devuelve
// el ya hecho. Pero un traspaso no se cuelga de una clave que ya estaba sin él: la
// declaración no admite una clave que se sucede a sí misma, y la marca tiene que
// llegar con una clave nueva (la bóveda se vacía). Y siempre, reciente.
func antesDelTraspaso(product, keyID string, sol solicitudClaveBoveda, actual *store.VaultKeyRow, reciente bool) error {
	if sol.Handover && (!productosConTraspaso[product] || len(sol.Continuity) > 0) {
		return errClaveBovedaTraspaso
	}
	if actual != nil && actual.KeyID == keyID {
		if sol.Handover && actual.Reason != motivoTraspaso {
			return errClaveBovedaTraspaso
		}
		return store.ErrVaultKeyUnchanged
	}
	if sol.Handover && !reciente {
		return errClaveBovedaReautenticar
	}
	return nil
}

// decidirClaveBoveda aplica las reglas de arriba a una solicitud frente a la clave
// activa (nil si no hay). `reciente` dice si la sesión vale como reautenticación.
// No toca nada: devuelve el cambio que hay que guardar o el error que contestar.
func decidirClaveBoveda(sub, tenant, product string, sol solicitudClaveBoveda, actual *store.VaultKeyRow, reciente bool) (store.VaultKeyChange, error) {
	var nada store.VaultKeyChange
	keyID, err := sol.PublicKey.ID()
	if err != nil {
		return nada, httpx.ErrBadRequest.WithMessage("Invalid vault public key.")
	}
	if err := antesDelTraspaso(product, keyID, sol, actual, reciente); err != nil {
		return nada, err
	}
	generacion, anterior := int64(1), ""
	if actual != nil {
		generacion, anterior = actual.Generation+1, actual.KeyID
	}
	if sol.PreviousKeyID != anterior {
		return nada, errClaveBovedaDesfasada
	}
	declaracion, err := recoverywrap.PQVaultKeyStatement(sub, tenant, product, generacion, keyID, anterior)
	if err != nil {
		return nada, httpx.ErrBadRequest
	}
	if sol.PublicKey.Verify(recoverywrap.VaultKeyEnrollContext, declaracion, sol.Proof) != nil {
		return nada, errClaveBovedaPrueba
	}
	publica, err := json.Marshal(sol.PublicKey)
	if err != nil {
		return nada, err
	}
	cambio := store.VaultKeyChange{KeyID: keyID, PreviousKeyID: anterior, PublicKey: publica}
	switch {
	case sol.Handover:
		cambio.Reason = motivoTraspaso
	case actual == nil:
		cambio.Reason = motivoAlta
	case len(sol.Continuity) > 0:
		var vieja recoverywrap.PQVaultVerifyKey
		if json.Unmarshal(actual.PublicKey, &vieja) != nil ||
			vieja.Verify(recoverywrap.VaultKeyRotateContext, declaracion, sol.Continuity) != nil {
			return nada, errClaveBovedaPrueba
		}
		cambio.Reason = motivoContinuidad
	case !reciente:
		return nada, errClaveBovedaReautenticar
	default:
		cambio.Reason = motivoVaciado
	}
	return cambio, nil
}

// vistaClaveBoveda es una generación tal como la ve la persona o un servicio.
type vistaClaveBoveda struct {
	Generation    int64           `json:"generation"`
	KeyID         string          `json:"key_id"`
	PreviousKeyID string          `json:"previous_key_id"`
	PublicKey     json.RawMessage `json:"public_key"`
	Reason        string          `json:"reason"`
	CreatedAt     time.Time       `json:"created_at"`
	RetiredAt     *time.Time      `json:"retired_at,omitempty"`
}

func vistaDeClave(r store.VaultKeyRow) vistaClaveBoveda {
	return vistaClaveBoveda{
		Generation: r.Generation, KeyID: r.KeyID, PreviousKeyID: r.PreviousKeyID,
		PublicKey: r.PublicKey, Reason: r.Reason, CreatedAt: r.CreatedAt, RetiredAt: r.RetiredAt,
	}
}

// ambitoClaveBoveda saca (sub, tenant, product) del token y de la ruta.
func ambitoClaveBoveda(r *http.Request) (sub, tenant, product string, ok bool) {
	c, ok := claimsFrom(r.Context())
	if !ok || c.Subject == "" {
		return "", "", "", false
	}
	product = chi.URLParam(r, "product")
	if !tenancy.ValidSlug(c.Tenant) || !storage.Product(product) {
		return "", "", "", false
	}
	return c.Subject, c.Tenant, product, true
}

// handleMeVaultKeysGet — GET /api/v1/me/vault-keys/{product} — el historial de la
// clave de firma de una bóveda propia, la activa primero.
func (s *Server) handleMeVaultKeysGet(w http.ResponseWriter, r *http.Request) {
	sub, tenant, product, ok := ambitoClaveBoveda(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	filas, err := s.store.VaultKeys(r.Context(), sub, tenant, product)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	out := make([]vistaClaveBoveda, 0, len(filas))
	for _, f := range filas {
		out = append(out, vistaDeClave(f))
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"keys": out})
}

// handleMeVaultKeyPut — PUT /api/v1/me/vault-keys/{product} — registra la
// generación siguiente de la clave de firma de una bóveda propia.
func (s *Server) handleMeVaultKeyPut(w http.ResponseWriter, r *http.Request) {
	sub, tenant, product, ok := ambitoClaveBoveda(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	var sol solicitudClaveBoveda
	if err := httpx.DecodeJSON(w, r, &sol); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	user, err := s.store.GetUserByID(r.Context(), sub)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	reciente := identificacionRecienteDelToken(c, user.MFAEnabled, time.Now())
	motivo := ""
	fila, err := s.store.RegisterVaultKey(r.Context(), sub, tenant, product, func(actual *store.VaultKeyRow) (store.VaultKeyChange, error) {
		cambio, err := decidirClaveBoveda(sub, tenant, product, sol, actual, reciente)
		motivo = cambio.Reason
		return cambio, err
	})
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if motivo != "" {
		_ = s.store.Audit(r.Context(), nil, auditEntry(sub, tenant, "e2e.vault_key."+motivo, product, r))
		if (motivo == motivoVaciado || motivo == motivoTraspaso) && user.Email != "" {
			// Mejor esfuerzo: la clave ya está registrada; un correo que no sale no
			// la deshace, pero queda en el registro.
			if err := s.mailer.SendVaultKeyReset(r.Context(), user.Email, product, fila.Generation); err != nil {
				s.log.WarnContext(r.Context(), "claves de bóveda: el aviso de vaciado no se envió", "error", err)
			}
		}
	}
	httpx.JSON(w, r, http.StatusOK, vistaDeClave(fila))
}

// identificacionRecienteDelToken es identificacionReciente con lo que trae un
// access token: la persona se identificó hace menos de frescuraStepUp y, si la
// cuenta tiene segundo factor, con él (TOTP o passkey con verificación).
func identificacionRecienteDelToken(c tokenClaims, conMFA bool, ahora time.Time) bool {
	fresca := !c.AuthTime.IsZero() && ahora.Sub(c.AuthTime) <= frescuraStepUp
	return fresca && (!conMFA || slices.Contains(c.AMR, "mfa") || slices.Contains(c.AMR, "otp"))
}

// handleInternalVaultKeyLookup — POST /internal/vault-keys/lookup — la clave activa
// de una bóveda, para un servicio que verifica una firma suya. 404 si no hay.
func (s *Server) handleInternalVaultKeyLookup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject string `json:"subject"`
		Tenant  string `json:"tenant"`
		Product string `json:"product"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Subject == "" || !tenancy.ValidSlug(req.Tenant) || !storage.Product(req.Product) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	fila, err := s.store.ActiveVaultKey(r.Context(), req.Subject, req.Tenant, req.Product)
	if errors.Is(err, domain.ErrNotFound) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, vistaDeClave(*fila))
}
