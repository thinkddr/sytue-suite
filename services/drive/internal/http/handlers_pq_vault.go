// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type pqVaultService interface {
	GetPQVault(context.Context, string) (service.PQVaultState, error)
	PutPQVaultKey(context.Context, string, recoverywrap.PQVaultWrapper, bool, *service.PQVaultExpect) (service.PQVaultState, error)
	ClaimPQVaultCopy(context.Context, string, service.PQVaultCopyClaim) (service.PQVaultState, error)
	ReleasePQVaultCopy(context.Context, string, string) (service.PQVaultState, error)
	AppendPQVaultRecord(context.Context, string, string, string, json.RawMessage) (service.PQVaultRecord, error)
	GetPQVaultRecord(context.Context, string, string, string) (service.PQVaultRecord, error)
	ListPQVaultRecords(context.Context, string, string, int64, int) (service.PQVaultRecordPage, error)
	FindPQVaultRecord(context.Context, string, string, string) (service.PQVaultRecord, error)
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
		// Expect es lo que el cliente vio al decidir vaciar. Los clientes de ahora
		// siempre lo mandan; los nativos ya instalados (v3.22 y anteriores), no.
		Expect *service.PQVaultExpect `json:"expect"`
		// CopyID es la marca de copia con la que vacía quien copia (ADR 0086).
		CopyID string `json:"copy_id"`
	}
	if httpx.DecodeJSONLimit(w, r, &req, 16<<10) != nil || (!req.Reset && req.Expect != nil) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	var reset *service.PQVaultExpect
	switch {
	case req.Reset && req.Expect != nil:
		reset = req.Expect
	case req.Reset:
		// Un cliente nativo de antes (v3.22 y anteriores) vacía sin decir qué vio:
		// se le vacía como entonces, sin comparar, pero nunca con la marca de copia
		// activa de otro. Queda en el registro para saber cuándo retirarlo.
		reset = &service.PQVaultExpect{Unchecked: true}
		log.From(r.Context()).Warn("pq-vault: vaciado sin expect (cliente anterior)", "product", product)
	}
	ctx := r.Context()
	if req.CopyID != "" {
		ctx = store.WithPQVaultCopy(ctx, req.CopyID)
	}
	state, err := h.pqVault.PutPQVaultKey(ctx, product, req.Wrapper, req.SetActive, reset)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}

// PutPQVaultCopy pone o renueva la marca «copia en curso» de la bóveda (ADR
// 0086): la pone el cliente nativo antes de copiar en ella el Drive de Matrix.
func (h *Handlers) PutPQVaultCopy(w http.ResponseWriter, r *http.Request) {
	product, ok := h.pqVaultScope(w, r)
	if !ok {
		return
	}
	var req struct {
		ID         string `json:"id"`
		Holder     string `json:"holder"`
		Kind       string `json:"kind"`
		Epoch      string `json:"epoch"`
		TTLSeconds int64  `json:"ttl_seconds"`
	}
	if httpx.DecodeJSONLimit(w, r, &req, 4<<10) != nil || req.TTLSeconds <= 0 || req.TTLSeconds > int64(service.MaxPQVaultCopyTTL/time.Second) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	state, err := h.pqVault.ClaimPQVaultCopy(r.Context(), product, service.PQVaultCopyClaim{
		ID: req.ID, Holder: req.Holder, Kind: req.Kind, Epoch: req.Epoch, TTL: time.Duration(req.TTLSeconds) * time.Second,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}

// DeletePQVaultCopy retira la marca `?id=`: quien copia al terminar, o la persona
// que la cancela desde otro cliente. No borra ningún registro.
func (h *Handlers) DeletePQVaultCopy(w http.ResponseWriter, r *http.Request) {
	product, ok := h.pqVaultScope(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	if len(q) != 1 || len(q["id"]) != 1 {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	state, err := h.pqVault.ReleasePQVaultCopy(r.Context(), product, q.Get("id"))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, state)
}

// pqVaultAPIError traduce los rechazos de la bóveda que el cliente tiene que
// distinguir: la bóveda cambió desde que la leyó, u otro dispositivo copia en ella.
func pqVaultAPIError(err error) error {
	switch {
	case errors.Is(err, domain.ErrPQVaultChanged):
		return httpx.ErrConflict.WithCode("pq_vault_changed").WithMessage("The vault changed since it was read. Nothing was removed.")
	case errors.Is(err, domain.ErrPQVaultCopyInProgress):
		return httpx.ErrConflict.WithCode("pq_vault_copy_in_progress").WithMessage("Another device is copying into this vault.")
	default:
		return nil
	}
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
		CopyID        string          `json:"copy_id"`
	}
	if httpx.DecodeJSONLimit(w, r, &req, service.MaxPQVaultRecordBytes+2048) != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	ctx := r.Context()
	if req.CopyID != "" {
		ctx = store.WithPQVaultCopy(ctx, req.CopyID)
	}
	rec, err := h.pqVault.AppendPQVaultRecord(ctx, product, req.VaultID, req.TransactionID, req.Payload)
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
	if q.Has("transaction_id") {
		h.findPQVaultRecord(w, r, product, vaultID, q)
		return
	}
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

// findPQVaultRecord contesta `GET records?vault_id=…&transaction_id=…` con una
// página de cero o un registro: el guardado con ese identificador.
func (h *Handlers) findPQVaultRecord(w http.ResponseWriter, r *http.Request, product, vaultID string, q url.Values) {
	if len(q) != 2 {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	page := service.PQVaultRecordPage{Entries: []service.PQVaultRecord{}}
	rec, err := h.pqVault.FindPQVaultRecord(r.Context(), product, vaultID, q.Get("transaction_id"))
	switch {
	case err == nil:
		page.Entries = append(page.Entries, rec)
	case errors.Is(err, domain.ErrNotFound):
	default:
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
