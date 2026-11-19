// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
)

type pqVaultService interface {
	GetPQVault(context.Context, string) (service.PQVaultState, error)
	PutPQVaultKey(context.Context, string, recoverywrap.PQVaultWrapper, bool, bool) (service.PQVaultState, error)
	AppendPQVaultRecord(context.Context, string, string, string, json.RawMessage) (service.PQVaultRecord, error)
	GetPQVaultRecord(context.Context, string, string, string) (service.PQVaultRecord, error)
	ListPQVaultRecords(context.Context, string, string, int64, int) (service.PQVaultRecordPage, error)
}

func (h *Handlers) pqVaultScope(w http.ResponseWriter, r *http.Request) (string, bool) {
	if h.pqVault == nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return "", false
	}
	product := chi.URLParam(r, "product")
	if !storageRequestScope(w, r, product) {
		return "", false
	}
	return product, true
}

func (h *Handlers) GetPQVaultKeys(w http.ResponseWriter, r *http.Request) {
	product, ok := h.pqVaultScope(w, r)
	if !ok {
		return
	}
	state, err := h.pqVault.GetPQVault(r.Context(), product)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}

func (h *Handlers) PutPQVaultKey(w http.ResponseWriter, r *http.Request) {
	product, ok := h.pqVaultScope(w, r)
	if !ok {
		return
	}
	var req struct {
		Wrapper   recoverywrap.PQVaultWrapper `json:"wrapper"`
		SetActive bool                        `json:"set_active"`
		Reset     bool                        `json:"reset"`
	}
	if httpx.DecodeJSONLimit(w, r, &req, 16<<10) != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	state, err := h.pqVault.PutPQVaultKey(r.Context(), product, req.Wrapper, req.SetActive, req.Reset)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}

func (h *Handlers) AppendPQVaultRecord(w http.ResponseWriter, r *http.Request) {
	product, ok := h.pqVaultScope(w, r)
	if !ok {
		return
	}
	var req struct {
		VaultID       string          `json:"vault_id"`
		TransactionID string          `json:"transaction_id"`
		Payload       json.RawMessage `json:"payload"`
	}
	if httpx.DecodeJSONLimit(w, r, &req, service.MaxPQVaultRecordBytes+2048) != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	rec, err := h.pqVault.AppendPQVaultRecord(r.Context(), product, req.VaultID, req.TransactionID, req.Payload)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusCreated, rec)
}

func (h *Handlers) ListPQVaultRecords(w http.ResponseWriter, r *http.Request) {
	product, ok := h.pqVaultScope(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	vaultID := q.Get("vault_id")
	var beforeSeq int64
	if raw := q.Get("cursor"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		beforeSeq = v
	}
	limit := 50
	if raw := q.Get("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > 100 {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		limit = v
	}
	page, err := h.pqVault.ListPQVaultRecords(r.Context(), product, vaultID, beforeSeq, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, page)
}

func (h *Handlers) GetPQVaultRecord(w http.ResponseWriter, r *http.Request) {
	product, ok := h.pqVaultScope(w, r)
	if !ok {
		return
	}
	vaultID := r.URL.Query().Get("vault_id")
	eventID := chi.URLParam(r, "event_id")
	rec, err := h.pqVault.GetPQVaultRecord(r.Context(), product, vaultID, eventID)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, rec)
}
