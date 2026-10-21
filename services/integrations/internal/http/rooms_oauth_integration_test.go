// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

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

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func crearCuentaRooms(t *testing.T, c *consentimiento, key string, cfg map[string]string) string {
	t.Helper()
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	if err := c.st.UpsertTemplate(ctx, key, json.RawMessage(manifiestoConConsentimiento()), "standard"); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(cfg)
	inst, err := c.st.CreateInstance(ctx, key, "Cuenta Rooms", encoded, nil)
	if err != nil {
		t.Fatal(err)
	}
	return inst.ID
}

func pedirDestinosRooms(t *testing.T, c *consentimiento, tenant, instance string) (int, string) {
	t.Helper()
	q := url.Values{"tenant": {tenant}, "instance": {instance}}
	r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.srv.URL+"/internal/rooms/destinations?"+q.Encode(), http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("X-Sytue-Internal", "suite-secret")
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(body)
}

func TestSlackOAuthGuardaElWebhookEnVaultYRoomsSoloVeElCanal(t *testing.T) {
	c := montarConsentimiento(t, func(h *Handlers, r chi.Router) {
		r.Post("/oauth/finish", h.OAuthFinish)
		r.With(requireInternal("suite-secret")).Get("/internal/rooms/destinations", h.InternalRoomsDestinations)
	})
	c.inst = crearCuentaRooms(t, c, tmplSlackRooms, map[string]string{configKeyClientID: "app", "service_url": "https://erp.acme.example"})
	res := c.pedir(t, http.MethodPost, "/instances/"+c.inst+"/oauth/start", "ana", "acme", map[string]string{"return_to": "rooms"})
	var start oauthStartResp
	if err := json.NewDecoder(res.Body).Decode(&start); err != nil {
		t.Fatal(err)
	}
	out := c.terminar(t, "ana", "acme", stateDe(t, start.AuthorizeURL), "rooms-slack-grant")
	if out.Error != "" || out.Section != "rooms" {
		t.Fatalf("el consentimiento no volvió a Rooms: %+v", out)
	}
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	inst, err := c.st.GetInstance(ctx, c.inst)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(inst.Config), "secret-only-vault") || strings.Contains(string(inst.Config), "hooks.slack.com") {
		t.Fatal("el webhook se guardó en configuración pública")
	}
	conn, err := c.st.LoadInstanceWithSecrets(ctx, c.inst)
	if err != nil {
		t.Fatal(err)
	}
	if conn.Secrets[secretKeyRoomsWebhook] != "https://hooks.slack.com/services/T1/B1/secret-only-vault" || conn.Config[roomsChannelID] != "C123" {
		t.Fatal("el grant perdió su secreto o su canal autorizado")
	}
	status, body := pedirDestinosRooms(t, c, "acme", c.inst)
	if status != http.StatusOK || !strings.Contains(body, "C123") || strings.Contains(body, "slack-token") || strings.Contains(body, "secret-only-vault") || strings.Contains(body, "hooks.slack.com") {
		t.Fatalf("la lista interna debe devolver solo destinos: status=%d body=%s", status, body)
	}
	status, _ = pedirDestinosRooms(t, c, "otra", c.inst)
	if status != http.StatusNotFound {
		t.Fatalf("una organización accedió al canal de otra: %d", status)
	}
	if err := c.st.SetStatus(ctx, c.inst, domain.StatusDisabled); err != nil {
		t.Fatal(err)
	}
	status, _ = pedirDestinosRooms(t, c, "acme", c.inst)
	if status != http.StatusNotFound {
		t.Fatalf("una conexión desactivada sigue publicando: %d", status)
	}
}

func TestRoomsYDriveCompartenLaRenovacionDelConsentimiento(t *testing.T) {
	var handlers *Handlers
	c := montarConsentimiento(t, func(h *Handlers, _ chi.Router) { handlers = h })
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	id := crearCuentaRooms(t, c, tmplGoogleChat, map[string]string{
		configKeyClientID: "app", configKeyExpires: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), "service_url": "https://erp.acme.example",
	})
	if err := c.st.PutSecrets(ctx, id, map[string]string{store.SecretKeyBearer: "old-access", secretKeyRefresh: "refresh-consent"}); err != nil {
		t.Fatal(err)
	}
	if err := c.st.SetStatus(ctx, id, domain.StatusActive); err != nil {
		t.Fatal(err)
	}
	before := len(canjesDelProveedor())
	conn, expires, err := handlers.oauthConnection(ctx, id, esPlantillaDeRooms)
	if err != nil {
		t.Fatal(err)
	}
	if conn.Secrets[store.SecretKeyBearer] != "token-de-" || time.Until(expires) < 50*time.Minute {
		t.Fatal("Rooms no recibió el access token renovado")
	}
	requests := canjesDelProveedor()[before:]
	if len(requests) != 1 || requests[0].Get("grant_type") != "refresh_token" || requests[0].Get("refresh_token") != "refresh-consent" {
		t.Fatalf("no usó el consentimiento renovable: %v", requests)
	}
	if _, _, err := handlers.oauthConnection(ctx, id, esPlantillaDeAlmacen); err == nil {
		t.Fatal("la puerta de almacenamiento prestó un token de chat")
	}
}
