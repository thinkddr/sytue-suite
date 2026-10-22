// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// TestNewPushSender cubre los tres estados: sin claves (deshabilitado), media configuración
// (error) y par válido (habilitado, con la pública expuesta).
func TestNewPushSender(t *testing.T) {
	// Sin claves: deshabilitado, sin error.
	p, err := NewPushSender(config.Config{})
	if err != nil || p != nil {
		t.Fatalf("sin claves: p=%v err=%v, quiero nil/nil", p, err)
	}

	// Solo pública: error explícito.
	if _, err := NewPushSender(config.Config{VAPIDPublic: "abc"}); err == nil {
		t.Fatal("media configuración (solo pública) debería fallar")
	}

	// Par válido: habilitado y la pública coincide.
	priv, _ := ecdh.P256().GenerateKey(rand.Reader)
	pubB64 := b64urlEncode(priv.PublicKey().Bytes())
	sender, err := NewPushSender(config.Config{
		VAPIDPublic:  pubB64,
		VAPIDPrivate: b64urlEncode(priv.Bytes()),
		VAPIDSubject: "mailto:ops@sytue.eu",
	})
	if err != nil || sender == nil {
		t.Fatalf("par válido: sender=%v err=%v", sender, err)
	}
	svc := &Service{}
	svc.SetPushSender(sender)
	if !svc.PushEnabled() || svc.VAPIDPublicKey() != pubB64 {
		t.Fatalf("VAPIDPublicKey=%q enabled=%v, quiero %q/true", svc.VAPIDPublicKey(), svc.PushEnabled(), pubB64)
	}
	// Servicio sin push: clave vacía, deshabilitado.
	if empty := (&Service{}); empty.PushEnabled() || empty.VAPIDPublicKey() != "" {
		t.Fatal("Service sin push debería reportar deshabilitado y clave vacía")
	}
}

// TestClassifyPushStatus fija la política de limpieza: 404/410 => errPushGone (borrar), 2xx
// => nil, resto => error transitorio (best-effort).
func TestClassifyPushStatus(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusCreated, 202} {
		if err := classifyPushStatus(code); err != nil {
			t.Fatalf("status %d debería ser éxito, got %v", code, err)
		}
	}
	for _, code := range []int{http.StatusNotFound, http.StatusGone} {
		if !errors.Is(classifyPushStatus(code), errPushGone) {
			t.Fatalf("status %d debería ser errPushGone (limpiar suscripción)", code)
		}
	}
	for _, code := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadRequest} {
		if err := classifyPushStatus(code); err == nil || errors.Is(err, errPushGone) {
			t.Fatalf("status %d debería ser fallo transitorio (ni éxito ni gone)", code)
		}
	}
}

// TestPushMessageJSON verifica el payload que ve el Service Worker: título (autor), cuerpo
// recortado, deep-link «?channel=&message=» y tag por canal.
func TestPushMessageJSON(t *testing.T) {
	ch := uuid.New()
	id := uuid.New()
	raw := pushMessageJSON(pushPayload{ID: id, ChannelID: ch, AuthorName: "Ana", Body: "hola mundo"})
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("payload no es JSON: %v", err)
	}
	if got["title"] != "Sytue Rooms" || got["body"] != hubnotify.TextoAvisoRooms || strings.Contains(string(raw), "hola mundo") || strings.Contains(string(raw), "Ana") {
		t.Fatalf("title/body inesperados: %v", got)
	}
	if !strings.Contains(got["url"], "channel="+ch.String()) || !strings.Contains(got["url"], "message="+id.String()) {
		t.Fatalf("deep-link sin channel/message: %q", got["url"])
	}
	if got["tag"] != "sytue-rooms-"+ch.String() {
		t.Fatalf("tag inesperado: %q", got["tag"])
	}
	// Sin nombre de autor cae a «Sytue Rooms».
	raw = pushMessageJSON(pushPayload{ID: id, ChannelID: ch, Body: "x"})
	_ = json.Unmarshal(raw, &got)
	if got["title"] != "Sytue Rooms" {
		t.Fatalf("título por defecto inesperado: %q", got["title"])
	}
}

type fakePushStore struct {
	recipients  []domain.NotificationRecipient
	recErr      error
	pushTargets []domain.PushTarget
	apnsTargets []domain.APNSTarget
}

func (f *fakePushStore) PushTargetsForMessage(context.Context, uuid.UUID, string, []string) ([]domain.PushTarget, error) {
	return f.pushTargets, nil
}

func (f *fakePushStore) DeletePushSubscription(context.Context, string, string) error { return nil }

func (f *fakePushStore) APNSTargetsForMessage(context.Context, uuid.UUID, string, []string) ([]domain.APNSTarget, error) {
	return f.apnsTargets, nil
}

func (f *fakePushStore) DeleteAPNSDevice(context.Context, string, string) error { return nil }

func (f *fakePushStore) NotificationRecipientsForMessage(context.Context, uuid.UUID, string, []string) ([]domain.NotificationRecipient, error) {
	return f.recipients, f.recErr
}

func (f *fakePushStore) SavePushSubscription(context.Context, string, domain.PushSubscription) error {
	return nil
}

func (f *fakePushStore) SaveAPNSDevice(context.Context, string, string) error { return nil }

func TestDeliverPushForEvent_HubNotification(t *testing.T) {
	received := make(chan hubnotify.Notification, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/notifications" {
			http.NotFound(w, r)
			return
		}
		var notif hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&notif)
		received <- notif
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	chID := uuid.New()
	msgID := uuid.New()
	authorSub := "author-sub"

	fps := &fakePushStore{
		recipients: []domain.NotificationRecipient{
			{Subject: "bob-sub", Mentioned: true},
			{Subject: "carol-sub", Mentioned: false},
		},
	}

	svc := &Service{
		cfg: config.Config{
			PublicURL: "https://rooms.sytue.eu",
		},
	}
	svc.SetPushStoreForTest(fps)
	svc.SetHubNotifier(hubnotify.New(srv.URL, "test-secret"))

	payload, _ := json.Marshal(map[string]any{
		"channel_id": chID,
		"data": map[string]any{
			"id":          msgID,
			"channel_id":  chID,
			"author_sub":  authorSub,
			"author_name": "Alice",
			"body":        "Hola @Bob y Carol!",
			"type":        domain.MessageTypeUser,
			"mentions":    []string{"bob-sub"},
		},
	})

	ev := events.Event{
		Channel: EventChannel,
		Type:    EventMessageCreated,
		Tenant:  "acme",
		Payload: payload,
	}

	svc.deliverPushForEvent(context.Background(), ev)

	var notifs []hubnotify.Notification
	for i := 0; i < 2; i++ {
		select {
		case n := <-received:
			notifs = append(notifs, n)
		case <-time.After(2 * time.Second):
			t.Fatalf("esperaba 2 notificaciones, recibidas %d", len(notifs))
		}
	}

	var bobNotif, carolNotif *hubnotify.Notification
	for i := range notifs {
		switch notifs[i].Subject {
		case "bob-sub":
			bobNotif = &notifs[i]
		case "carol-sub":
			carolNotif = &notifs[i]
		}
	}

	if bobNotif == nil {
		t.Fatal("no se recibió notificación para Bob")
	}
	if bobNotif.Type != "rooms.message" || bobNotif.Title != "Sytue Rooms" || bobNotif.Body != hubnotify.TextoAvisoRooms {
		t.Errorf("bob notificación inesperada: %+v", bobNotif)
	}
	if bobNotif.Source != "rooms" || bobNotif.Tenant != "acme" {
		t.Errorf("bob source/tenant inesperado: %+v", bobNotif)
	}
	// El enlace abre el canal en el mensaje, en el host del servicio, con el prefijo
	// de la app y la organización detrás de él: antes era
	// `https://rooms.sytue.localhost/?channel=…` en producción.
	if quiero := "https://rooms.sytue.eu/rooms/acme/channels/" + chID.String() + "?message=" + msgID.String(); bobNotif.URL != quiero {
		t.Errorf("enlace = %q, quiero %q", bobNotif.URL, quiero)
	}
	if bobNotif.Params["actor"] != "Sytue Rooms" {
		t.Errorf("el aviso revela al autor: %+v", bobNotif.Params)
	}

	if carolNotif == nil {
		t.Fatal("no se recibió notificación para Carol")
	}
	if carolNotif.Type != "rooms.message" || carolNotif.Title != "Sytue Rooms" || carolNotif.Body != hubnotify.TextoAvisoRooms {
		t.Errorf("carol notificación inesperada: %+v", carolNotif)
	}
}
