//go:build integration

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func TestPersonalCapacityStaysWithActorAndNeverExportsCredentials(t *testing.T) {
	c := montarConsentimiento(t, func(h *Handlers, r chi.Router) {
		// No se necesita alcanzar Google para verificar el aislamiento: un
		// proveedor no disponible sólo deja la capacidad desconocida.
		h.egressTimeout = time.Nanosecond
		r.With(requireInternal("internal-capacity-fixture")).Get("/personal-capacity", h.InternalPersonalStorageCapacity)
	})
	ctx := tenancy.Into(authn.Into(context.Background(), authn.Claims{Subject: "bea", Tenant: "acme"}), tenancy.Tenant{Slug: "acme"})
	if err := c.st.UpsertTemplate(ctx, tmplGoogleDrive, json.RawMessage(manifiestoConConsentimiento()), "standard"); err != nil {
		t.Fatal(err)
	}
	inst, err := c.st.CreateInstance(ctx, tmplGoogleDrive, "Bea private", json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.st.BindExternalChatActor(ctx, inst.ID, "bea"); err != nil {
		t.Fatal(err)
	}
	if err := c.st.PutSecrets(ctx, inst.ID, map[string]string{store.SecretKeyBearer: "fixture-private-bea"}); err != nil {
		t.Fatal(err)
	}
	if err := c.st.SetStatus(ctx, inst.ID, domain.StatusActive); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{}
	request := func(actor, tenant, secret string) (personalCapacity, int) {
		t.Helper()
		query := url.Values{"actor": {actor}, "tenant": {tenant}}
		req, err := http.NewRequest(http.MethodGet, c.srv.URL+"/personal-capacity?"+query.Encode(), http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Sytue-Internal", secret)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "fixture-private-bea") || strings.Contains(string(body), inst.ID) {
			t.Fatalf("el resumen filtró una credencial o inventario: %s", body)
		}
		var out personalCapacity
		if res.StatusCode == http.StatusOK {
			if err := json.Unmarshal(body, &out); err != nil || res.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("resumen inválido/cacheable: %s %v", body, err)
			}
		}
		return out, res.StatusCode
	}
	if _, status := request("bea", "acme", "wrong"); status != http.StatusUnauthorized {
		t.Fatalf("se aceptó un llamador sin secreto: %d", status)
	}
	if _, status := request("", "acme", "internal-capacity-fixture"); status != http.StatusBadRequest {
		t.Fatalf("actor vacío puede consultar instancias del tenant: %d", status)
	}
	for _, actor := range []string{"ana", "bea", "bea ", "ana"} {
		out, status := request(actor, "acme", "internal-capacity-fixture")
		if status != http.StatusOK || out.Connected != (actor == "bea") || out.TotalBytes != nil {
			t.Fatalf("la conexión de Bea se compartió con %q: %d %+v", actor, status, out)
		}
	}
	if out, status := request("bea", "otra", "internal-capacity-fixture"); status != http.StatusOK || out.Connected {
		t.Fatalf("otro tenant heredó la conexión: %d %+v", status, out)
	}
	if err := c.st.SetStatus(ctx, inst.ID, domain.StatusDisabled); err != nil {
		t.Fatal(err)
	}
	if out, status := request("bea", "acme", "internal-capacity-fixture"); status != http.StatusOK || out.Connected {
		t.Fatalf("se cacheó la conexión desactivada: %d %+v", status, out)
	}
}
