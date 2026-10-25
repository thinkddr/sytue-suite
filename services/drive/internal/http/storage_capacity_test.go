// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

func TestGetQuotaIncluyeLaExtensionYConservaLaCuotaDelPlan(t *testing.T) {
	total, used, free := int64(1500), int64(600), int64(900)
	q := (domain.Quota{LimitBytes: 500, UsedBytes: 200}).WithStorageExtension(&total, &used, &free)
	h := newHandlers(&fakeSvc{quota: q})
	rec := httptest.NewRecorder()
	h.GetQuota(rec, mkReq(http.MethodGet, "/api/v1/quota", "", nil))
	var got struct {
		Limit     int64 `json:"limit_bytes"`
		Used      int64 `json:"used_bytes"`
		Available int64 `json:"available_bytes"`
		BaseLimit int64 `json:"base_limit_bytes"`
		BaseUsed  int64 `json:"base_used_bytes"`
		Connected bool  `json:"external_storage"`
		Capacity  struct {
			Total int64 `json:"total_bytes"`
			Used  int64 `json:"used_bytes"`
			Free  int64 `json:"available_bytes"`
		} `json:"external_capacity"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rec.Code != http.StatusOK || got.Limit != 2000 || got.Used != 800 || got.Available != 1200 ||
		got.BaseLimit != 500 || got.BaseUsed != 200 || !got.Connected ||
		got.Capacity.Total != 1500 || got.Capacity.Used != 600 || got.Capacity.Free != 900 {
		t.Fatalf("respuesta de cuota ampliada: %d %s", rec.Code, rec.Body)
	}
}

func TestGetQuotaDistingueCapacidadDesconocidaDeCero(t *testing.T) {
	q := (domain.Quota{LimitBytes: 500, UsedBytes: 200}).WithStorageExtension(nil, nil, nil)
	h := newHandlers(&fakeSvc{quota: q})
	rec := httptest.NewRecorder()
	h.GetQuota(rec, mkReq(http.MethodGet, "/api/v1/quota", "", nil))
	var got struct {
		Limit     int64                      `json:"limit_bytes"`
		Used      int64                      `json:"used_bytes"`
		Connected bool                       `json:"external_storage"`
		Capacity  map[string]json.RawMessage `json:"external_capacity"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rec.Code != http.StatusOK || !got.Connected || got.Limit != 500 || got.Used != 200 || len(got.Capacity) != 4 {
		t.Fatalf("conexión sin cuota conocida: %d %s", rec.Code, rec.Body)
	}
	for _, key := range []string{"total_bytes", "used_bytes", "available_bytes"} {
		value := got.Capacity[key]
		if string(value) != "null" {
			t.Errorf("%s=%s; una cifra desconocida no es cero", key, value)
		}
	}
	if string(got.Capacity["partial"]) != "true" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("capacidad personal parcial y no cacheable: %s %v", rec.Body, rec.Header())
	}
}
