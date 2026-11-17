// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"net/http"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

func (h *Handlers) MatrixBootstrap(w http.ResponseWriter, r *http.Request) {
	actor, name := caller(r)
	result, err := h.svc.BootstrapMatrix(r.Context(), actor, name)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) MatrixStorageBootstrap(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	version := storage.LegacyIdentity
	if raw, ok := r.URL.Query()["identity_version"]; ok {
		n, err := strconv.Atoi(raw[0])
		version = storage.IdentityVersion(n)
		if len(raw) != 1 || err != nil || !version.Valid() {
			httpx.Fail(w, r, toAPIError(domain.ErrInvalidInput))
			return
		}
	}
	result, err := h.svc.BootstrapMatrixStorageForIdentity(r.Context(), actor, r.URL.Query().Get("product"), version)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

func (h *Handlers) MatrixAccountAccess(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject string `json:"subject"`
		Status  string `json:"status"`
		Version int64  `json:"version"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.MatrixAccountAccess(r.Context(), req.Subject, req.Status, req.Version); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) MatrixTenantAccess(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject string `json:"subject"`
		Tenant  string `json:"tenant"`
		Status  string `json:"status"`
		Version int64  `json:"version"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.MatrixTenantAccess(r.Context(), req.Subject, req.Tenant, req.Status, req.Version); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MatrixRecoveryKey devuelve la clave de recuperación custodiada de quien llama
// (ADR 0081): 404 si todavía no hay ninguna.
func (h *Handlers) MatrixRecoveryKey(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	key, err := h.svc.MatrixRecoveryKey(r.Context(), actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]string{"recovery_key": key})
}

// StoreMatrixRecoveryKey ya no custodia nada (ADR 0082): responde 410 para que un
// cliente viejo no crea que su clave quedó guardada.
func (h *Handlers) StoreMatrixRecoveryKey(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	var req struct {
		RecoveryKey string `json:"recovery_key"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.StoreMatrixRecoveryKey(r.Context(), actor, req.RecoveryKey); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MatrixRecoveryWrappers devuelve los envoltorios de la clave de recuperación de
// una cuenta de quien llama (ADR 0082). Sin parámetros es la cuenta de Rooms;
// `product` (e `identity_version=2` para la cuenta aislada de ese producto en la
// organización de la petición) elige una cuenta de almacenamiento.
func (h *Handlers) MatrixRecoveryWrappers(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	account, ok := recoveryAccount(r)
	if !ok {
		httpx.Fail(w, r, toAPIError(domain.ErrInvalidInput))
		return
	}
	result, err := h.svc.MatrixRecoveryWrappers(r.Context(), actor, account)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}

// PutMatrixRecoveryWrapper guarda un envoltorio hecho en el dispositivo.
func (h *Handlers) PutMatrixRecoveryWrapper(w http.ResponseWriter, r *http.Request) {
	actor, _ := caller(r)
	account, ok := recoveryAccount(r)
	if !ok {
		httpx.Fail(w, r, toAPIError(domain.ErrInvalidInput))
		return
	}
	var req recoverywrap.Wrapper
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	put := h.svc.PutMatrixRecoveryWrapper
	// `retire_older=1`: un envoltorio `sytue-id` que el dispositivo acaba de comprobar
	// que abre, y en la misma transacción se retiran los de la ADR 0082 (ADR 0085).
	if retiro := r.URL.Query()["retire_older"]; len(retiro) > 0 {
		if len(retiro) != 1 || retiro[0] != "1" {
			httpx.Fail(w, r, toAPIError(domain.ErrInvalidInput))
			return
		}
		put = h.svc.PutMatrixRecoveryWrapperRetiringOlder
	}
	id, err := put(r.Context(), actor, account, req)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]string{"id": id})
}

// recoveryAccount lee `product` e `identity_version` de la consulta. Cada uno a lo
// sumo una vez; la versión sin producto no elige ninguna cuenta y se rechaza.
func recoveryAccount(r *http.Request) (service.MatrixRecoveryAccount, bool) {
	query := r.URL.Query()
	var account service.MatrixRecoveryAccount
	if raw, ok := query["product"]; ok {
		if len(raw) != 1 || raw[0] == "" {
			return account, false
		}
		account.Product = raw[0]
	}
	if raw, ok := query["identity_version"]; ok {
		n, err := strconv.Atoi(raw[0])
		account.IdentityVersion = storage.IdentityVersion(n)
		if len(raw) != 1 || err != nil || !account.IdentityVersion.Valid() || account.Product == "" {
			return account, false
		}
	}
	return account, true
}
