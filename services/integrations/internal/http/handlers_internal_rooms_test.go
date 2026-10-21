// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

type roomsTestTransport func(*http.Request) (*http.Response, error)

func (f roomsTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func roomsTestResponse(status int, body string) *http.Response {
	w := httptest.NewRecorder()
	w.WriteHeader(status)
	_, _ = w.WriteString(body)
	return w.Result()
}

func roomsTestRuntime(handle roomsTestTransport) connector.Runtime {
	return connector.Runtime{HTTP: &http.Client{Transport: handle}}
}

func TestElConsentimientoSlackSeparaElSecretoDeLosMetadatos(t *testing.T) {
	var tok tokenResponse
	if err := json.Unmarshal([]byte(`{"access_token":"bearer-secreto","incoming_webhook":{"url":"https://hooks.slack.com/services/T1/B1/secreto","channel_id":"C123","channel":"#equipo"},"team":{"id":"T1","name":"Acme"}}`), &tok); err != nil {
		t.Fatal(err)
	}
	cfg, webhook, err := slackRoomsGrant(tok)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(cfg)
	if strings.Contains(string(encoded), "secreto") || strings.Contains(string(encoded), "hooks.slack.com") || webhook != tok.IncomingWebhook.URL {
		t.Fatalf("el webhook no puede quedar en la configuración pública: %s", encoded)
	}
	items, err := listRoomsDestinations(context.Background(), connector.Instance{
		TemplateKey: tmplSlackRooms, Config: cfg, Secrets: map[string]string{secretKeyRoomsWebhook: webhook},
	}, connector.Runtime{})
	if err != nil || len(items) != 1 || items[0].ID != "C123" || items[0].Name != "Acme / #equipo" {
		t.Fatalf("destino autorizado de Slack: %+v %v", items, err)
	}
	public, _ := json.Marshal(items)
	if strings.Contains(string(public), "secreto") || strings.Contains(string(public), "https://") {
		t.Fatal("el destino público filtró una credencial")
	}
	for _, raw := range []string{"http://hooks.slack.com/services/x", "https://evil.example/services/x", "https://hooks.slack.com.evil.example/services/x", "https://user:pass@hooks.slack.com/services/x"} {
		tok.IncomingWebhook.URL = raw
		if _, _, err := slackRoomsGrant(tok); err == nil {
			t.Fatalf("se aceptó un webhook de otro destino: %s", raw)
		}
	}
}

func TestRoomsPublicaTextoEnLasAPIsRealesDeCadaProveedor(t *testing.T) {
	const team = "00000000-0000-4000-8000-000000000001"
	for _, tc := range []struct{ key, remote, host, path string }{
		{tmplSlackRooms, "C123", "hooks.slack.com", "/services/T1/B1/secreto"},
		{tmplGoogleChat, "spaces/AAAA", "chat.googleapis.com", "/v1/spaces/AAAA/messages"},
		{tmplTeamsRooms, team + "/19:channel@thread.tacv2", "graph.microsoft.com", "/v1.0/teams/" + team + "/channels/19:channel@thread.tacv2/messages"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			called := false
			rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
				called = true
				if r.Method != http.MethodPost || r.URL.Host != tc.host || r.URL.Path != tc.path || r.URL.RawQuery != "" {
					t.Fatalf("destino incorrecto: %s %s", r.Method, r.URL)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if tc.key == tmplTeamsRooms {
					b, ok := body["body"].(map[string]any)
					if !ok || b["contentType"] != "text" || b["content"] != "<b>texto</b>" {
						t.Fatalf("Teams debe recibir texto sin convertirlo en HTML: %v", body)
					}
				} else if body["text"] != "<b>texto</b>" {
					t.Fatalf("se perdió el texto: %v", body)
				}
				return roomsTestResponse(http.StatusOK, "ok"), nil
			})
			conn := connector.Instance{TemplateKey: tc.key, Config: map[string]string{roomsChannelID: "C123"}, Secrets: map[string]string{secretKeyRoomsWebhook: "https://hooks.slack.com/services/T1/B1/secreto"}}
			if err := sendRoomsMessage(context.Background(), conn, rt, tc.remote, "<b>texto</b>"); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("no se envió el mensaje")
			}
		})
	}
}

func TestElDestinoNoPuedeConvertirRoomsEnUnProxy(t *testing.T) {
	called := false
	rt := roomsTestRuntime(func(*http.Request) (*http.Response, error) { called = true; return nil, errors.New("no debe salir") })
	for _, tc := range []struct{ key, remote string }{
		{tmplGoogleChat, "https://evil.example/messages"},
		{tmplGoogleChat, "spaces/AAAA/../../users"},
		{tmplTeamsRooms, "00000000-0000-4000-8000-000000000001/channel/messages?x=y"},
		{tmplSlackRooms, "otro-canal"},
		{tmplGoogleMeet, "spaces/AAAA"},
	} {
		conn := connector.Instance{TemplateKey: tc.key, Config: map[string]string{roomsChannelID: "C123"}, Secrets: map[string]string{secretKeyRoomsWebhook: "https://hooks.slack.com/services/T1/B1/secreto"}}
		if err := sendRoomsMessage(context.Background(), conn, rt, tc.remote, "mensaje"); err == nil {
			t.Fatalf("se aceptó un destino inventado: %+v", tc)
		}
	}
	if called {
		t.Fatal("un destino no válido llegó al transporte")
	}
}

func TestGoogleChatPaginaLosEspaciosDeLaCuenta(t *testing.T) {
	calls := 0
	rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "chat.googleapis.com" || r.URL.Path != "/v1/spaces" || r.Method != http.MethodGet {
			t.Fatalf("ruta inesperada: %s", r.URL)
		}
		if calls == 1 {
			return roomsTestResponse(http.StatusOK, `{"spaces":[{"name":"spaces/AAAA","displayName":"Equipo"}],"nextPageToken":"siguiente"}`), nil
		}
		if r.URL.Query().Get("pageToken") != "siguiente" {
			t.Fatal("se perdió la página del proveedor")
		}
		return roomsTestResponse(http.StatusOK, `{"spaces":[{"name":"spaces/BBBB","displayName":"Proyecto"}]}`), nil
	})
	items, err := listGoogleRooms(context.Background(), rt)
	if err != nil || len(items) != 2 || calls != 2 || items[1].Name != "Proyecto" {
		t.Fatalf("espacios incompletos: %+v %v", items, err)
	}
}

func TestTeamsListaCanalesDeLosEquiposDeLaCuenta(t *testing.T) {
	const team = "00000000-0000-4000-8000-000000000001"
	calls := 0
	rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v1.0/me/joinedTeams" {
			return roomsTestResponse(http.StatusOK, `{"value":[{"id":"`+team+`","displayName":"Equipo"}]}`), nil
		}
		if r.URL.Path != "/v1.0/teams/"+team+"/channels" {
			t.Fatalf("ruta inesperada: %s", r.URL)
		}
		return roomsTestResponse(http.StatusOK, `{"value":[{"id":"19:channel@thread.tacv2","displayName":"General"}]}`), nil
	})
	items, err := listTeamsRooms(context.Background(), rt)
	if err != nil || len(items) != 1 || calls != 2 || items[0].ID != team+"/19:channel@thread.tacv2" || items[0].Name != "Equipo / General" {
		t.Fatalf("canales incompletos: %+v %v", items, err)
	}
	calls = 0
	rt = roomsTestRuntime(func(*http.Request) (*http.Response, error) {
		calls++
		return roomsTestResponse(http.StatusOK, `{"value":[],"@odata.nextLink":"https://evil.example/v1.0/teams"}`), nil
	})
	if _, err := listTeamsRooms(context.Background(), rt); err == nil || calls != 1 {
		t.Fatalf("un nextLink debe quedarse en Graph: llamadas=%d error=%v", calls, err)
	}
}

func TestGoogleMeetCreaUnEnlaceRealYRechazaOtrosHosts(t *testing.T) {
	rt := roomsTestRuntime(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != roomsMeetAPI {
			t.Fatalf("no se creó el espacio de Meet: %s %s", r.Method, r.URL)
		}
		return roomsTestResponse(http.StatusOK, `{"meetingUri":"https://meet.google.com/abc-defg-hij"}`), nil
	})
	u, err := createRoomsMeeting(context.Background(), rt)
	if err != nil || u != "https://meet.google.com/abc-defg-hij" {
		t.Fatalf("enlace de Meet: %q %v", u, err)
	}
	rt = roomsTestRuntime(func(*http.Request) (*http.Response, error) {
		return roomsTestResponse(http.StatusOK, `{"meetingUri":"https://evil.example/abc"}`), nil
	})
	if _, err := createRoomsMeeting(context.Background(), rt); err == nil {
		t.Fatal("se aceptó un enlace que no es de Meet")
	}
}

func TestUnErrorDelProveedorNoFiltraElWebhookNiSuCuerpo(t *testing.T) {
	const secret = "https://hooks.slack.com/services/T1/B1/secreto"
	for _, transportError := range []bool{false, true} {
		rt := roomsTestRuntime(func(*http.Request) (*http.Response, error) {
			if transportError {
				return nil, errors.New("falló el destino " + secret)
			}
			return roomsTestResponse(http.StatusForbidden, `{"echo":"`+secret+`"}`), nil
		})
		err := roomsProviderJSON(context.Background(), rt, http.MethodPost, secret, map[string]string{"text": "x"}, nil)
		if err == nil || strings.Contains(err.Error(), "secreto") || strings.Contains(err.Error(), "hooks.slack.com") {
			t.Fatalf("el error filtró credenciales: %v", err)
		}
	}
}

func TestLaPuertaInternaDeRoomsExigeElSecretoCompartido(t *testing.T) {
	called := false
	h := requireInternal("suite-secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) }))
	for _, secret := range []string{"", "otro", "suite-secret"} {
		called = false
		r := httptest.NewRequest(http.MethodPost, "/internal/v1/rooms/messages", http.NoBody)
		r.Header.Set("X-Sytue-Internal", secret)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if (secret == "suite-secret") != called {
			t.Fatalf("autorización incorrecta con secreto %q", secret)
		}
	}
}

func TestRoomsNoticeRejectsContentAndInvalidIdentifiersBeforeProviderAccess(t *testing.T) {
	h := &Handlers{roomsPublicURL: "https://rooms.sytue.eu"}
	const prefix = `{"tenant":"acme","instance":"11111111-1111-4111-8111-111111111111","remote_id":"spaces/AAAA",`
	for _, payload := range []string{
		prefix + `"channel_id":"22222222-2222-4222-8222-222222222222","text":"PRIVATE body or key"}`,
		prefix + `"channel_id":"invalid"}`,
		prefix + `"channel_id":"22222222-2222-4222-8222-222222222222","message_id":"encryption-key"}`,
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/internal/v1/rooms/messages", strings.NewReader(payload))
		h.InternalRoomsMessage(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("unsafe notification contract accepted: %d %s", w.Code, w.Body)
		}
	}
}

func TestElErrorDePermisosDelProveedorEsAccionable(t *testing.T) {
	rt := roomsTestRuntime(func(*http.Request) (*http.Response, error) {
		return roomsTestResponse(http.StatusForbidden, "privado"), nil
	})
	err := roomsProviderJSON(context.Background(), rt, http.MethodGet, roomsChatAPI+"spaces", nil, &struct{}{})
	var apiError *httpx.Error
	if !errors.As(err, &apiError) || apiError.Status != http.StatusUnprocessableEntity || apiError.Code != "rooms_provider_permission" {
		t.Fatalf("error de permisos opaco: %v", err)
	}
}
