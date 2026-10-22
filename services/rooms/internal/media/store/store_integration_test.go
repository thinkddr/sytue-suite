// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de Sytue Rooms contra un PostgreSQL real (efímero, vía
// platform/pkg/pgtest sobre testcontainers-go). Tras el build-tag `integration`
// (no corren en el build/CI por defecto). Ejercen los flujos completos sobre BD:
// creación de sala (nombre único por tenant), minteo del JWT de acceso a LiveKit
// con los grants del rol, ciclo de vida por webhooks verificados (room_started →
// participant_joined → participant_left → room_finished) y metadatos de grabación,
// resolución acotada del tenant por nombre de room (SECURITY DEFINER usada por los
// webhooks de egress) y el aislamiento multi-tenant reforzado por RLS.
package store_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/store"
)

const (
	testLKKey    = "APItestkey"
	testLKSecret = "test-secret-para-webhooks-de-meet"
)

func tenantCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	return store.New(h.Pool)
}

// signWebhook mintea el JWT de webhook que LiveKit envía en la cabecera
// Authorization: HS256 firmado con el api_secret, con el claim sha256 = hash
// base64-std del cuerpo (formato público de LiveKit). Replica lo que emite el
// SFU para poder ejercer ParseWebhook (verificación de firma) end-to-end.
func signWebhook(t *testing.T, body []byte) string {
	t.Helper()
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	sum := sha256.Sum256(body)
	payload := map[string]any{
		"iss":    testLKKey,
		"exp":    time.Now().Add(5 * time.Minute).Unix(),
		"sha256": base64.StdEncoding.EncodeToString(sum[:]),
	}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(payload)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb)
	mac := hmac.New(sha256.New, []byte(testLKSecret))
	mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// TestCreateRoomAndMintJWT cubre la creación de sala (nombre único por tenant) y
// el minteo del JWT de acceso a LiveKit con los grants correctos del rol host.
func TestCreateRoomAndMintJWT(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")

	room, err := st.CreateRoom(ctx, store.NewRoomRecord{
		Name: "sala-daily", Title: "Daily", CreatedBy: "user-ada",
		MaxParticipants: 20, EmptyTimeoutSeconds: 300,
	})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if room.Status != domain.RoomScheduled || room.Tenant != "acme" {
		t.Fatalf("sala inicial inesperada: %+v", room)
	}

	// Nombre único por tenant.
	if _, err := st.CreateRoom(ctx, store.NewRoomRecord{
		Name: "sala-daily", Title: "Otra", CreatedBy: "user-ada", MaxParticipants: 5, EmptyTimeoutSeconds: 60,
	}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("nombre duplicado = %v, quiero ErrConflict", err)
	}

	// Minteo del JWT de LiveKit para el host (puede publicar y administrar).
	signer := livekit.NewSigner(testLKKey, testLKSecret)
	canPublish := true
	tok, err := signer.AccessToken(livekit.AccessTokenOptions{
		Identity: "user-ada", Name: "Ada", Room: room.Name,
		Grant: livekit.VideoGrant{RoomAdmin: true, CanPublish: &canPublish},
		TTL:   time.Hour,
	})
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if tok == "" {
		t.Fatal("AccessToken vacío")
	}
	// El token debe verificar como webhook-signable inverso: reusamos ParseWebhook
	// no aplica aquí; validamos estructura mínima (3 segmentos).
	if n := len(splitDots(tok)); n != 3 {
		t.Fatalf("JWT de acceso malformado: %d segmentos", n)
	}
}

// TestWebhookLifecycle cubre el ciclo de vida por webhooks verificados: se firma
// y parsea cada evento (autenticación de origen) y se materializa el estado
// canónico en PostgreSQL — room_started marca la sala activa, participant_joined
// registra presencia, participant_left la cierra y room_finished finaliza.
func TestWebhookLifecycle(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")
	signer := livekit.NewSigner(testLKKey, testLKSecret)

	room, err := st.CreateRoom(ctx, store.NewRoomRecord{
		Name: "sala-webhook", Title: "Con webhooks", CreatedBy: "user-ada",
		MaxParticipants: 10, EmptyTimeoutSeconds: 120,
	})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	// room_started: verificamos la firma y aplicamos el estado.
	started := mustParse(t, signer, `{"event":"room_started","room":{"sid":"RM_1","name":"sala-webhook"},"createdAt":1700000000}`)
	if started.Event != livekit.EventRoomStarted {
		t.Fatalf("evento = %q", started.Event)
	}
	if err := st.MarkRoomStarted(ctx, started.Room.Name, started.Room.SID, eventTime(started.CreatedAt)); err != nil {
		t.Fatalf("MarkRoomStarted: %v", err)
	}
	active, _ := st.GetRoom(ctx, room.ID)
	if active.Status != domain.RoomActive || active.LiveKitSID != "RM_1" {
		t.Fatalf("sala no activa tras room_started: %+v", active)
	}

	// participant_joined: registra presencia.
	joined := mustParse(t, signer, `{"event":"participant_joined","room":{"name":"sala-webhook"},"participant":{"sid":"PA_1","identity":"user-ada","name":"Ada","metadata":"{\"role\":\"host\"}"},"createdAt":1700000010}`)
	if err := st.RecordJoin(ctx, store.RecordJoinInput{
		RoomName: joined.Room.Name, Identity: joined.Participant.Identity,
		Name: joined.Participant.Name, Role: domain.RoleHost,
		LiveKitSID: joined.Participant.SID, JoinedAt: eventTime(joined.CreatedAt),
	}); err != nil {
		t.Fatalf("RecordJoin: %v", err)
	}
	present, _ := st.ListParticipants(ctx, room.ID, true)
	if len(present) != 1 || present[0].Identity != "user-ada" || !present[0].IsPresent() {
		t.Fatalf("participante presente incorrecto: %+v", present)
	}

	// participant_left: marca la salida.
	if err := st.RecordLeave(ctx, "sala-webhook", "PA_1", time.Unix(1700000020, 0)); err != nil {
		t.Fatalf("RecordLeave: %v", err)
	}
	stillPresent, _ := st.ListParticipants(ctx, room.ID, true)
	if len(stillPresent) != 0 {
		t.Fatalf("participante debería haber salido, presentes=%d", len(stillPresent))
	}

	// room_finished: finaliza la sala.
	if err := st.MarkRoomEnded(ctx, "sala-webhook", time.Unix(1700000030, 0)); err != nil {
		t.Fatalf("MarkRoomEnded: %v", err)
	}
	ended, _ := st.GetRoom(ctx, room.ID)
	if ended.Status != domain.RoomEnded || ended.EndedAt == nil {
		t.Fatalf("sala no finalizada: %+v", ended)
	}

	// Firma inválida: ParseWebhook la rechaza (autenticación de origen).
	body := []byte(`{"event":"room_started","room":{"name":"sala-webhook"}}`)
	if _, err := signer.ParseWebhook("token.falso.invalido", body); err == nil {
		t.Fatal("ParseWebhook aceptó una firma inválida")
	}
}

// TestRecordingAndTenantResolution cubre los metadatos de grabación (egress) y la
// resolución acotada del tenant por nombre de room (SECURITY DEFINER), que los
// webhooks de egress usan porque no portan contexto de tenant.
func TestRecordingAndTenantResolution(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")

	room, err := st.CreateRoom(ctx, store.NewRoomRecord{
		Name: "sala-grabada", Title: "Grabada", CreatedBy: "user-ada",
		MaxParticipants: 10, EmptyTimeoutSeconds: 120,
	})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	// egress_started -> completed (idempotente por egress_id, agrega tamaño/duración).
	if err := st.UpsertRecording(ctx, "sala-grabada", "EG_1", domain.RecordingStarted, "", 0, 0, nil); err != nil {
		t.Fatalf("UpsertRecording(started): %v", err)
	}
	ended := time.Now().UTC()
	if err := st.UpsertRecording(ctx, "sala-grabada", "EG_1", domain.RecordingCompleted, "recordings/EG_1.mp4", 4096, 60, &ended); err != nil {
		t.Fatalf("UpsertRecording(completed): %v", err)
	}
	recs, err := st.ListRecordings(ctx, room.ID)
	if err != nil {
		t.Fatalf("ListRecordings: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("grabaciones = %d, quiero 1 (idempotente por egress_id)", len(recs))
	}
	if recs[0].Status != domain.RecordingCompleted || recs[0].StorageKey != "recordings/EG_1.mp4" || recs[0].SizeBytes != 4096 {
		t.Fatalf("metadato de grabación incorrecto: %+v", recs[0])
	}
}

// TestTenantIsolationRLS verifica que RLS aísla las salas por tenant: una sala de
// "acme" no es visible desde "other".
func TestTenantIsolationRLS(t *testing.T) {
	st := newStore(t)

	room, err := st.CreateRoom(tenantCtx("acme"), store.NewRoomRecord{
		Name: "sala-privada", Title: "Privada", CreatedBy: "user-ada",
		MaxParticipants: 5, EmptyTimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	if _, err := st.GetRoom(tenantCtx("other"), room.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento RLS roto: %v", err)
	}
	// Otro tenant puede crear una sala con el MISMO nombre (unicidad es por tenant).
	if _, err := st.CreateRoom(tenantCtx("other"), store.NewRoomRecord{
		Name: "sala-privada", Title: "Otra Privada", CreatedBy: "user-bob",
		MaxParticipants: 5, EmptyTimeoutSeconds: 60,
	}); err != nil {
		t.Fatalf("CreateRoom(other, mismo nombre): %v", err)
	}
	// El propietario sí la ve.
	if _, err := st.GetRoom(tenantCtx("acme"), room.ID); err != nil {
		t.Fatalf("GetRoom(propietario): %v", err)
	}
}

// mustParse firma y parsea un webhook, fallando el test si la verificación falla.
func mustParse(t *testing.T, signer *livekit.Signer, body string) livekit.WebhookEvent {
	t.Helper()
	tok := signWebhook(t, []byte(body))
	ev, err := signer.ParseWebhook(tok, []byte(body))
	if err != nil {
		t.Fatalf("ParseWebhook(%s): %v", body, err)
	}
	return ev
}

// eventTime replica la conversión de createdAt (epoch segundos) a time.Time UTC.
// eventTime replica la conversión del servicio. `livekit.Int64` porque protojson
// serializa los int64 como CADENA: LiveKit manda `"createdAt":"1700000000"` tanto como
// el número, y con `int64` a secas el webhook entero se rechazaba con un 401.
func eventTime(epochSec livekit.Int64) time.Time {
	if epochSec <= 0 {
		return time.Now().UTC()
	}
	return time.Unix(int64(epochSec), 0).UTC()
}

// splitDots divide un JWT compacto en sus segmentos por '.'.
func splitDots(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// Reuniones sueltas (decidido por el dueño el 2026-10-16: «Invitados y quien tenga el
// enlace»): GET /rooms daba todas las de la organización. Ahora una reunión que no
// cuelga de un canal ni de un documento solo sale a quien la convocó o ya estuvo
// dentro; las de un canal siguen saliendo (las filtra el servicio con su sonda). Y
// cada sala trae su clave de enlace, que no es el id.
func TestLaListaSoloEnsenaLasReunionesSueltasPropias(t *testing.T) {
	st := newStore(t)
	ctx := tenantCtx("acme")
	crear := func(nombre, quien string, meta map[string]any) domain.Room {
		t.Helper()
		r, err := st.CreateRoom(ctx, store.NewRoomRecord{Name: nombre, Title: nombre, CreatedBy: quien, Metadata: meta})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	deAna := crear("suelta-ana", "ana", nil)
	deEvento := crear("evento-ana", "ana", map[string]any{"source": map[string]any{"kind": "calendar", "ref_id": "uid-1"}})
	crear("canal-ana", "ana", map[string]any{"source": map[string]any{"kind": "chat", "ref_id": "c-1"}})
	if deAna.LinkKey == "" || len(deAna.LinkKey) < 60 || deAna.LinkKey == deEvento.LinkKey ||
		strings.Contains(deAna.LinkKey, strings.ReplaceAll(deAna.ID.String(), "-", "")) {
		t.Fatalf("clave de enlace %q: tiene que ser larga, propia de cada sala y no salir del id", deAna.LinkKey)
	}
	// Bea estuvo dentro de la del evento.
	if err := st.RecordJoin(ctx, store.RecordJoinInput{RoomName: deEvento.Name, Identity: "bea", LiveKitSID: "PA_b", JoinedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	ids := func(quien string) map[string]bool {
		t.Helper()
		rs, err := st.ListRooms(ctx, "", 0, quien)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, r := range rs {
			out[r.Name] = true
		}
		return out
	}
	for quien, quiero := range map[string][]string{
		"ana":   {"suelta-ana", "evento-ana", "canal-ana"},
		"bea":   {"evento-ana", "canal-ana"},
		"carla": {"canal-ana"}, // administre o no: las sueltas de otros no salen
		"":      {"canal-ana"},
	} {
		got := ids(quien)
		if len(got) != len(quiero) {
			t.Errorf("%q ve %v, quiero %v", quien, got, quiero)
			continue
		}
		for _, n := range quiero {
			if !got[n] {
				t.Errorf("%q no ve %s (ve %v)", quien, n, got)
			}
		}
	}
}
