// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// El envío va a /{phone_number_id}/messages con el token de la cuenta, y si Meta lo
// rechaza (la ventana de 24 h) vuelve SU explicación, que es lo que ve el canal.
func TestSendWhatsApp(t *testing.T) {
	var ruta, autorizacion string
	var cuerpo map[string]any
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta, autorizacion = r.URL.Path, r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &cuerpo)
		requests++
		if requests == 2 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"(#131047) Re-engagement message","code":131047,
				"error_data":{"details":"Message failed to send because more than 24 hours have passed since the customer last replied to this number."}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"messaging_product":"whatsapp","messages":[{"id":"wamid.Z"}]}`))
	}))
	defer srv.Close()
	svc := &Service{outgoing: srv.Client(), cfg: config.Config{WhatsAppGraphURL: srv.URL + "/v23.0"}}
	cuenta := domain.WhatsAppAccount{PhoneNumberID: "1234567890", AccessToken: "EAAG-token"}
	notice := externalNotice(hubnotify.AvisoMensajeRooms("https://rooms.sytue.eu", "acme", "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"))

	if err := svc.sendWhatsApp(context.Background(), cuenta, "34600111222", notice); err != nil {
		t.Fatal(err)
	}
	texto, _ := cuerpo["text"].(map[string]any)
	if ruta != "/v23.0/1234567890/messages" || autorizacion != "Bearer EAAG-token" || cuerpo["to"] != "34600111222" ||
		cuerpo["messaging_product"] != "whatsapp" || texto["body"] != string(notice) || texto["preview_url"] != false {
		t.Fatalf("petición: %s %s %v", ruta, autorizacion, cuerpo)
	}

	err := svc.sendWhatsApp(context.Background(), cuenta, "34600111222", notice)
	if err == nil || !strings.Contains(err.Error(), "131047") || strings.Contains(err.Error(), "24 hours") {
		t.Fatalf("rechazo de Meta: %v", err)
	}
}
