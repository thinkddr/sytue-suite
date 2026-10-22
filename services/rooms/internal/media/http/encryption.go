// SPDX-License-Identifier: Apache-2.0
package meethttp

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/service"
)

type encryptionService interface {
	RoomEncryption(context.Context, uuid.UUID, string, string) (domain.CallEncryption, error)
	ReadyRoomEncryption(context.Context, uuid.UUID, string, string, int64) error
	ReconnectRoomEncryption(context.Context, uuid.UUID, string, string, int64) (service.JoinResult, error)
}

func (h *Handlers) RoomEncryption(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	svc, ok := h.svc.(encryptionService)
	if !ok {
		httpx.Fail(w, r, toAPIError(domain.ErrEncryptionUnavailable))
		return
	}
	out, err := svc.RoomEncryption(r.Context(), id, r.URL.Query().Get("matrix_device_id"), r.URL.Query().Get("join_id"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, out)
}

func (h *Handlers) ReadyRoomEncryption(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req struct {
		MatrixDeviceID string `json:"matrix_device_id"`
		JoinID         string `json:"join_id"`
		Generation     int64  `json:"generation"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	svc, ok := h.svc.(encryptionService)
	if !ok {
		httpx.Fail(w, r, toAPIError(domain.ErrEncryptionUnavailable))
		return
	}
	if err := svc.ReadyRoomEncryption(r.Context(), id, req.MatrixDeviceID, req.JoinID, req.Generation); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ReconnectRoomEncryption(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req struct {
		MatrixDeviceID string `json:"matrix_device_id"`
		JoinID         string `json:"join_id"`
		Generation     int64  `json:"generation"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	svc, ok := h.svc.(encryptionService)
	if !ok {
		httpx.Fail(w, r, toAPIError(domain.ErrEncryptionUnavailable))
		return
	}
	out, err := svc.ReconnectRoomEncryption(r.Context(), id, req.MatrixDeviceID, req.JoinID, req.Generation)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, out)
}
