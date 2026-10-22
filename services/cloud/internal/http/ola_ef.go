// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// CancelBuild cancela una build en cola o en curso.
func (h *Handlers) CancelBuild(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid build id."))
		return
	}
	if err := h.svc.CancelBuild(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PatchDomain actualiza un dominio (p. ej. marcar como principal).
func (h *Handlers) PatchDomain(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid domain id."))
		return
	}
	var req struct {
		IsPrimary *bool `json:"is_primary"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.IsPrimary != nil && *req.IsPrimary {
		d, err := h.svc.SetDomainPrimary(r.Context(), id)
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{"domain": d})
		return
	}
	httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("No changes requested."))
}

// ListStorageObjects lista objetos del bucket del proyecto.
func (h *Handlers) ListStorageObjects(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid project id."))
		return
	}
	prefix := r.URL.Query().Get("prefix")
	objects, err := h.svc.ListStorageObjects(r.Context(), id, prefix)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	info, _ := h.svc.ProjectStorage(r.Context(), id)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"storage": info, "objects": objects})
}

// DeleteStorageObject borra un objeto del bucket del proyecto.
//
// La clave viaja por query y no en la ruta: una clave de S3 lleva barras dentro
// (`assets/img/logo.png`) y en un segmento de ruta habría que escaparlas dos
// veces —chi decodifica una— para acabar en la misma cadena.
func (h *Handlers) DeleteStorageObject(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid project id."))
		return
	}
	key := r.URL.Query().Get("key")
	if strings.TrimSpace(key) == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("key is required."))
		return
	}
	if err := h.svc.DeleteStorageObject(r.Context(), id, key); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// PresignStorage genera URL prefirmada para subir/descargar un objeto.
func (h *Handlers) PresignStorage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid project id."))
		return
	}
	var req struct {
		Key    string `json:"key"`
		Method string `json:"method"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil || req.Key == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("key is required."))
		return
	}
	url, err := h.svc.PresignStorageURL(r.Context(), id, req.Key, req.Method, 15*time.Minute)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"url": url})
}

// RotateWebhookSecret emite un secreto de webhook nuevo para el proyecto y
// devuelve el claro UNA vez.
//
// Es una credencial: el valor viaja en la respuesta de un POST sin caché y no se
// vuelve a poder leer así —el detalle del proyecto sigue revelándolo solo a un
// admin/owner—. El de antes deja de valer en el acto, que es justo lo que se pide
// cuando se rota algo comprometido.
func (h *Handlers) RotateWebhookSecret(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid project id."))
		return
	}
	secret, err := h.svc.RotateProjectWebhookSecret(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"webhook_secret": secret,
		"webhook_url":    h.svc.WebhookURL(),
	})
}

// BackupServiceNow pide una copia inmediata del Postgres del servicio.
//
// Responde 202 y no 200: el Job corre en el clúster y la copia aparecerá en la
// lista cuando termine. Un 200 diría «hecho» sobre algo que acaba de empezar.
func (h *Handlers) BackupServiceNow(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid service id."))
		return
	}
	if err := h.svc.BackupServiceNow(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"status": "requested"})
}

// DownloadServiceBackup devuelve una URL temporal para bajarse un backup.
//
// La URL viaja en el cuerpo en vez de responder con un 302: el panel la abre en
// una pestaña nueva, y un redirect obligaría al navegador a mandar la sesión al
// almacenamiento de objetos, que no la necesita ni debe verla.
func (h *Handlers) DownloadServiceBackup(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid service id."))
		return
	}
	name := r.URL.Query().Get("backup")
	if strings.TrimSpace(name) == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("backup is required."))
		return
	}
	url, err := h.svc.DownloadServiceBackupURL(r.Context(), id, name)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"url": url})
}

// RunServiceNow ejecuta una tarea programada o puntual sin esperar a su hora.
func (h *Handlers) RunServiceNow(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid service id."))
		return
	}
	if err := h.svc.RunServiceNow(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"status": "requested"})
}

// ServiceLogs devuelve el registro reciente de un servicio gestionado.
//
// Autorización member+ y solo humanos, igual que los logs de la app: el registro
// de una función lleva lo que su código imprima, que puede ser cualquier cosa.
func (h *Handlers) ServiceLogs(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid service id."))
		return
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	out, err := h.svc.ServiceLogs(r.Context(), id, lines)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if out == nil {
		out = []domain.LogLine{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"lines": out})
}

// RestartApp reinicia las instancias de la producción activa del proyecto.
//
// Devuelve cuántas se reiniciaron: cero es una respuesta legítima —no hay nada
// corriendo— y el panel lo dice tal cual en vez de fingir que hizo algo.
func (h *Handlers) RestartApp(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid project id."))
		return
	}
	n, err := h.svc.RestartApp(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"restarted": n})
}

// ProtectPreviews cierra las previews del proyecto con una contraseña nueva y la
// devuelve EN CLARO una sola vez.
//
// Sobre una protección ya activa, rota la contraseña: la anterior deja de valer.
// Es lo que se quiere cuando la que había se ha ido de las manos, y evita un
// segundo verbo para lo mismo.
func (h *Handlers) ProtectPreviews(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid project id."))
		return
	}
	password, err := h.svc.ProtectPreviews(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"username": domain.PreviewAuthUser,
		"password": password,
	})
}

// UnprotectPreviews deja las previews abiertas otra vez.
func (h *Handlers) UnprotectPreviews(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid project id."))
		return
	}
	if err := h.svc.UnprotectPreviews(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// GetQuota devuelve el uso de la organización frente a los límites de su plan.
//
// Lectura para cualquier miembro: saber cuánto queda no es información sensible
// —el 402 al chocar ya lo dice— y esconderlo solo consigue que el tope aparezca
// de golpe.
func (h *Handlers) GetQuota(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.QuotaStatus(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, out)
}
