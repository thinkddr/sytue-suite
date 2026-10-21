// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// Almacenamiento de TERCEROS (mig 0023, ADR 0038): una organización —o una
// persona— conecta su propio bucket S3 y, desde entonces, sus ficheros nuevos
// van allí y no gastan cuota de Sytue. Es un ajuste de administración, como el
// editor WOPI, y sigue su mismo patrón: comprobar, guardar, desconectar.
//
// La clave secreta se sella con la KEK del servicio (EXTERNAL_STORAGE_KEY) y
// NUNCA vuelve por la API: la respuesta lleva el access key y el bucket, que es
// lo que hace falta para reconocerlo.

type externalStorageStore interface {
	ExternalStorage(ctx context.Context) (store.ExternalStorage, error)
	SaveExternalStorage(ctx context.Context, e store.ExternalStorage, actor string) (store.ExternalStorage, error)
	DeleteExternalStorage(ctx context.Context) error
}

type externalStorageRequest struct {
	// Kind: "s3" (por defecto) o "webdav" (NAS, Nextcloud, la CLI de Internxt…).
	Kind       string `json:"kind"`
	InstanceID string `json:"instance_id"`
	Endpoint   string `json:"endpoint"`
	BasePath   string `json:"base_path"`
	Region     string `json:"region"`
	Bucket     string `json:"bucket"`
	AccessKey  string `json:"access_key"`
	SecretKey  string `json:"secret_key"`
	Label      string `json:"label"`
	Enabled    *bool  `json:"enabled"`
	// AcceptTerms es la aceptación del aviso de responsabilidad: las copias de
	// seguridad y la recuperación de lo que vive en un almacén conectado son del
	// cliente (usted), y Sytue no responde de su pérdida. Sin ella no se guarda.
	AcceptTerms bool `json:"accept_terms"`
}

func externalStoragePayload(e store.ExternalStorage) map[string]any {
	out := map[string]any{
		"kind": e.Kind, "instance_id": e.InstanceID, "endpoint": e.Endpoint, "base_path": e.BasePath, "region": e.Region, "bucket": e.Bucket, "use_ssl": e.UseSSL,
		"access_key": e.AccessKey, "label": e.Label, "enabled": e.Enabled,
		"check_ok": e.CheckOK, "check_error": e.CheckError, "updated_at": e.UpdatedAt,
		"accepted_terms_at": e.AcceptedTermsAt,
	}
	if e.CheckedAt != nil {
		out["checked_at"] = *e.CheckedAt
	}
	return out
}

// externalCipher abre la KEK; nil ⇒ la función está apagada en esta instalación.
func (h *Handlers) externalCipher() *keycrypto.Cipher {
	raw := strings.TrimSpace(h.cfg.ExternalStorageKey)
	if raw == "" {
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}
	c, err := keycrypto.New(key)
	if err != nil {
		return nil
	}
	return c
}

// GetExternalStorage — GET /api/v1/storage/external. `connected:false` si no
// hay ninguno; `available:false` si la instalación no tiene KEK.
func (h *Handlers) GetExternalStorage(w http.ResponseWriter, r *http.Request) {
	if h.external == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	available := h.externalCipher() != nil
	e, err := h.external.ExternalStorage(r.Context())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.JSON(w, r, http.StatusOK, map[string]any{"connected": false, "available": available})
			return
		}
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	out := externalStoragePayload(e)
	out["connected"], out["available"] = true, available
	httpx.JSON(w, r, http.StatusOK, out)
}

// configDe valida la petición según la clase y devuelve el registro (sin sellar)
// y la clave secreta aparte.
func configDe(req externalStorageRequest) (store.ExternalStorage, string, error) {
	e := store.ExternalStorage{
		Kind: strings.TrimSpace(req.Kind), InstanceID: strings.TrimSpace(req.InstanceID),
		BasePath: strings.Trim(strings.TrimSpace(req.BasePath), "/"),
		Region:   strings.TrimSpace(req.Region), Bucket: strings.TrimSpace(req.Bucket),
		AccessKey: strings.TrimSpace(req.AccessKey), Label: strings.TrimSpace(req.Label),
	}
	if e.Kind == "" {
		e.Kind = "s3"
	}
	secret := req.SecretKey
	switch e.Kind {
	case "gdrive", "onedrive", "dropbox":
		// Por consentimiento: no hay credenciales aquí, solo la instancia que las guarda.
		if e.InstanceID == "" {
			return e, "", httpx.ErrBadRequest.WithMessage("instance_id is required: authorize the account first")
		}
		secret = ""
	case "webdav":
		e.Endpoint = strings.TrimSpace(req.Endpoint)
		e.UseSSL = true
		if !strings.HasPrefix(e.Endpoint, "https://") || e.AccessKey == "" || secret == "" {
			return e, "", httpx.ErrBadRequest.WithMessage("endpoint (https://…), access_key (user) and secret_key (password) are required")
		}
	case "s3":
		e.Endpoint, e.UseSSL = pcfg.NormalizeS3Endpoint(strings.TrimSpace(req.Endpoint), true)
		if e.Endpoint == "" || e.Bucket == "" || e.AccessKey == "" || secret == "" {
			return e, "", httpx.ErrBadRequest.WithMessage("endpoint, bucket, access_key and secret_key are required")
		}
		if strings.ContainsAny(e.Endpoint, "/?#") {
			return e, "", httpx.ErrBadRequest.WithMessage("endpoint must be host[:port]")
		}
		// Sin TLS no: las credenciales del bucket irían en claro, y un `http://` a un
		// nombre interno es justo lo que no puede salir de aquí.
		if !e.UseSSL {
			return e, "", httpx.ErrBadRequest.WithMessage("endpoint must use https")
		}
	default:
		return e, "", httpx.ErrBadRequest.WithMessage("kind must be s3, webdav, gdrive, onedrive or dropbox")
	}
	return e, secret, nil
}

// CheckExternalStorage — POST /api/v1/storage/external/check: prueba sin guardar.
// Ni la web ni la app nativa lo llaman ya (guardar prueba antes de sellar); se
// conserva por compatibilidad con clientes antiguos y la API pública.
func (h *Handlers) CheckExternalStorage(w http.ResponseWriter, r *http.Request) {
	var req externalStorageRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	e, secret, err := configDe(req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	msg := h.svc.ProbeExternal(r.Context(), e, secret)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": msg == "", "error": msg})
}

// SaveExternalStorage — PUT /api/v1/storage/external: prueba, sella y guarda.
func (h *Handlers) SaveExternalStorage(w http.ResponseWriter, r *http.Request) {
	if h.external == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var req externalStorageRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	e, secret, err := configDe(req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cipher := h.externalCipher()
	if secret != "" && cipher == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("External storage is not enabled on this installation."))
		return
	}
	if !req.AcceptTerms {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("You must accept that backups and recovery of connected storage are your responsibility."))
		return
	}
	if msg := h.svc.ProbeExternal(r.Context(), e, secret); msg != "" {
		// No se guarda lo que no funciona: guardarlo sería mandar los ficheros
		// nuevos a un sitio que no responde.
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithMessage("Could not reach the bucket: "+msg))
		return
	}
	sealed := ""
	if secret != "" {
		if sealed, err = cipher.Encrypt(secret); err != nil {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
			return
		}
	}
	now := time.Now()
	e.SecretKeyEnc, e.Enabled, e.CheckedAt, e.CheckOK, e.AcceptedTermsAt = sealed, true, &now, true, now
	if req.Enabled != nil {
		e.Enabled = *req.Enabled
	}
	actor := ""
	if c, ok := authn.FromContext(r.Context()); ok {
		actor = c.Subject
	}
	saved, err := h.external.SaveExternalStorage(r.Context(), e, actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if t, terr := tenancy.From(r.Context()); terr == nil {
		h.svc.ForgetExternalStorage(t.Slug)
	}
	out := externalStoragePayload(saved)
	out["connected"], out["available"] = true, true
	httpx.JSON(w, r, http.StatusOK, out)
}

// DeleteExternalStorage — DELETE /api/v1/storage/external. 409 mientras queden
// ficheros escritos allí: sin la configuración no habría forma de leerlos.
func (h *Handlers) DeleteExternalStorage(w http.ResponseWriter, r *http.Request) {
	if h.external == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	// Un NAS por túnel: desmontar el sitio y el recurso del proxy antes de
	// olvidar la configuración (best-effort: el proxy puede estar caído).
	if e, gerr := h.external.ExternalStorage(r.Context()); gerr == nil && e.Kind == "nas" {
		_ = h.svc.TeardownNAS(r.Context(), e)
	}
	if err := h.external.DeleteExternalStorage(r.Context()); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			httpx.Fail(w, r, httpx.ErrConflict.WithMessage("There are still files stored in this bucket; move or delete them first."))
			return
		}
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if t, terr := tenancy.From(r.Context()); terr == nil {
		h.svc.ForgetExternalStorage(t.Slug)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListExternalInstances — GET /api/v1/storage/external/instances: las cuentas
// de Google Drive, OneDrive y Dropbox conectadas por consentimiento desde el propio Drive.
func (h *Handlers) ListExternalInstances(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.svc.ExternalInstances(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage(err.Error()))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, accounts)
}

type nasRequest struct {
	LocalEndpoint string `json:"local_endpoint"`
	User          string `json:"user"`
	Password      string `json:"password"`
	BasePath      string `json:"base_path"`
	Label         string `json:"label"`
	AcceptTerms   bool   `json:"accept_terms"`
}

// ConnectNAS — POST /api/v1/storage/external/nas: crea el túnel del NAS del
// cliente y devuelve el comando del agente. El almacén queda apagado hasta que
// el túnel responda (ver NASStatus).
func (h *Handlers) ConnectNAS(w http.ResponseWriter, r *http.Request) {
	var req nasRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor := ""
	if c, ok := authn.FromContext(r.Context()); ok {
		actor = c.Subject
	}
	st, err := h.svc.ConnectNAS(r.Context(), service.NASRequest(req), actor)
	switch {
	case errors.Is(err, service.ErrTunnelOff), errors.Is(err, service.ErrExternalStorageOff):
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("Connecting a NAS is not available on this installation."))
		return
	case errors.Is(err, service.ErrNASTerms):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("You must accept that backups and recovery of connected storage are your responsibility."))
		return
	case errors.Is(err, domain.ErrInvalidName):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(err.Error()))
		return
	case err != nil:
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage(err.Error()))
		return
	}
	httpx.JSON(w, r, http.StatusOK, st)
}

// NASStatus — GET /api/v1/storage/external/nas/status.
func (h *Handlers) NASStatus(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.NASStatus(r.Context())
	if err != nil {
		if errors.Is(err, service.ErrTunnelOff) {
			httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("Connecting a NAS is not available on this installation."))
			return
		}
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, st)
}
