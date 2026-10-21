// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de la Ola 16 (Misc de paridad) contra PostgreSQL real (pgtest):
//   - caché de unfurl (link_unfurls): round-trip, resultado negativo, aislamiento tenant;
//   - fetch-around por keyset (MessagesAround): ventana centrada en un mensaje;
//   - receipts agregados de DM (DMReceipts): last_read_at de los otros miembros, y
//     vacío en canales no directos.
//
// Detrás del build-tag `integration`. Sigue el patrón de ola14/ola15.
package store_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func TestUnfurlCacheRoundTrip(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	// Ausente ⇒ found=false.
	if _, ok, err := st.GetUnfurl(ctx, "https://ejemplo.eu/a"); err != nil || ok {
		t.Fatalf("GetUnfurl ausente: ok=%v err=%v", ok, err)
	}

	u := domain.LinkUnfurl{URL: "https://ejemplo.eu/a", Title: "Título", Description: "desc", ImageURL: "https://cdn.eu/i.png", SiteName: "Ejemplo"}
	if err := st.UpsertUnfurl(ctx, u); err != nil {
		t.Fatalf("UpsertUnfurl: %v", err)
	}
	got, ok, err := st.GetUnfurl(ctx, "https://ejemplo.eu/a")
	if err != nil || !ok {
		t.Fatalf("GetUnfurl tras upsert: ok=%v err=%v", ok, err)
	}
	if got.Title != "Título" || got.ImageURL != "https://cdn.eu/i.png" || got.SiteName != "Ejemplo" {
		t.Fatalf("unfurl cacheado inesperado: %+v", got)
	}
	if got.FetchedAt.IsZero() {
		t.Fatal("fetched_at no debe ser cero (marca de frescura para el TTL)")
	}

	// Upsert idempotente: refresca campos y fetched_at (no duplica).
	u.Title = "Nuevo"
	if err := st.UpsertUnfurl(ctx, u); err != nil {
		t.Fatalf("UpsertUnfurl (update): %v", err)
	}
	if got, _, _ := st.GetUnfurl(ctx, "https://ejemplo.eu/a"); got.Title != "Nuevo" {
		t.Fatalf("el upsert debe actualizar el título: %+v", got)
	}

	// Resultado NEGATIVO cacheado (URL sin metadatos): fila presente, HasContent false.
	neg := domain.LinkUnfurl{URL: "https://ejemplo.eu/vacia"}
	if err := st.UpsertUnfurl(ctx, neg); err != nil {
		t.Fatalf("UpsertUnfurl negativo: %v", err)
	}
	if got, ok, _ := st.GetUnfurl(ctx, "https://ejemplo.eu/vacia"); !ok || got.HasContent() {
		t.Fatalf("negativo: ok=%v hasContent=%v", ok, got.HasContent())
	}

	// Aislamiento por tenant: otro tenant no ve la caché de acme (RLS).
	if _, ok, _ := st.GetUnfurl(tenantCtx("other"), "https://ejemplo.eu/a"); ok {
		t.Fatal("la caché de unfurl no debe cruzar tenants (RLS)")
	}
}

func TestMessagesAroundWindow(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	_, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	// 7 mensajes con created_at distintos (sleep breve garantiza el orden por keyset).
	ids := make([]domain.Message, 0, 7)
	for i := 0; i < 7; i++ {
		m, err := st.CreateMessage(ctx, store.NewMessageRecord{
			ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "m",
		})
		if err != nil {
			t.Fatalf("CreateMessage %d: %v", i, err)
		}
		ids = append(ids, m)
		time.Sleep(2 * time.Millisecond)
	}
	target := ids[3] // el del medio

	// limit=4 ⇒ half=2: 2 anteriores + objetivo + 2 posteriores = ventana de 5, ASC.
	win, root, err := st.MessagesAround(ctx, ch.ID, "alice", target.ID, 4)
	if err != nil {
		t.Fatalf("MessagesAround: %v", err)
	}
	if len(win) != 5 {
		t.Fatalf("ventana de %d, quiero 5: %+v", len(win), win)
	}
	wantIDs := []domain.Message{ids[1], ids[2], ids[3], ids[4], ids[5]}
	for i := range win {
		if win[i].ID != wantIDs[i].ID {
			t.Fatalf("ventana[%d]=%s, quiero %s (orden ASC centrado en el objetivo)", i, win[i].ID, wantIDs[i].ID)
		}
	}
	if win[2].ID != target.ID {
		t.Fatalf("el objetivo debe quedar en el centro: %s", win[2].ID)
	}
	if root != uuid.Nil {
		t.Fatalf("un objetivo que YA es raíz no debe devolver hilo que abrir: %s", root)
	}
	// Orden cronológico ascendente estricto.
	for i := 1; i < len(win); i++ {
		if win[i].CreatedAt.Before(win[i-1].CreatedAt) {
			t.Fatalf("la ventana debe ir ASC por created_at: %v", win)
		}
	}

	// Objetivo inexistente ⇒ ErrNotFound.
	if _, _, err := st.MessagesAround(ctx, ch.ID, "alice", target.ChannelID, 4); err == nil {
		t.Fatal("un messageID inexistente debe dar error")
	}
}

func TestDMReceiptsAggregated(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	if err := st.AddTeamMember(ctx, tm.ID, "alice", "bob", "Bob", domain.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	dm, err := st.OpenDirect(ctx, tm.ID, "alice", "bob")
	if err != nil {
		t.Fatalf("OpenDirect: %v", err)
	}

	// alice publica; bob aún no ha leído ⇒ receipt de bob con last_read_at nil.
	msg, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: dm.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "¿lo has visto?",
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	rs, err := st.DMReceipts(ctx, dm.ID, "alice")
	if err != nil {
		t.Fatalf("DMReceipts: %v", err)
	}
	if len(rs) != 1 || rs[0].Subject != "bob" {
		t.Fatalf("receipts esperados [bob]: %+v", rs)
	}
	if rs[0].LastReadAt != nil {
		t.Fatalf("bob no ha leído: last_read_at debe ser nil, got %v", rs[0].LastReadAt)
	}

	// bob lee el DM ⇒ su last_read_at >= created_at del mensaje de alice («Visto»).
	if err := st.MarkRead(ctx, dm.ID, "bob", nil); err != nil {
		t.Fatalf("MarkRead(bob): %v", err)
	}
	rs, _ = st.DMReceipts(ctx, dm.ID, "alice")
	if len(rs) != 1 || rs[0].LastReadAt == nil {
		t.Fatalf("tras leer, bob debe tener last_read_at: %+v", rs)
	}
	if rs[0].LastReadAt.Before(msg.CreatedAt) {
		t.Fatalf("last_read_at (%v) debe ser >= created_at del mensaje (%v) ⇒ Visto", rs[0].LastReadAt, msg.CreatedAt)
	}

	// Canal NO directo: sin receipts (paridad Mattermost: solo DMs).
	if got, err := st.DMReceipts(ctx, ch.ID, "alice"); err != nil || len(got) != 0 {
		t.Fatalf("un canal público no debe dar receipts: %+v err=%v", got, err)
	}

	// No miembro del canal ⇒ ErrForbidden (no puede sondear los receipts).
	if _, err := st.DMReceipts(ctx, dm.ID, "stranger"); err == nil {
		t.Fatal("un no-miembro no debe poder leer los receipts")
	}
}

// Un permalink a una RESPUESTA de hilo tiene que funcionar.
//
// El ancla exigía `parent_id IS NULL`, así que un objetivo dentro de un hilo daba
// ErrNotFound y el cliente pintaba «No se pudieron cargar los mensajes» para TODO
// el canal. Y la búsqueda NO filtra respuestas, así que bastaba con que un
// resultado cayera dentro de un hilo —lo normal— para dejar el canal en blanco.
func TestMessagesAroundResuelveUnaRespuestaPorSuRaiz(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")
	_, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)

	raiz, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice", Body: "arranca el hilo",
	})
	if err != nil {
		t.Fatalf("CreateMessage raíz: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	respuesta, err := st.CreateMessage(ctx, store.NewMessageRecord{
		ChannelID: ch.ID, AuthorSub: "alice", AuthorName: "Alice",
		Body: "dentro del hilo", ParentID: &raiz.ID,
	})
	if err != nil {
		t.Fatalf("CreateMessage respuesta: %v", err)
	}

	win, root, err := st.MessagesAround(ctx, ch.ID, "alice", respuesta.ID, 4)
	if err != nil {
		t.Fatalf("MessagesAround sobre una respuesta: %v", err)
	}
	if root != raiz.ID {
		t.Fatalf("root = %s, quiero la raíz del hilo %s: sin esto el cliente no sabe qué hilo abrir", root, raiz.ID)
	}
	// La ventana es del FEED, que solo pinta raíces: la respuesta no sale en ella.
	var vioRaiz bool
	for _, m := range win {
		if m.ID == respuesta.ID {
			t.Fatalf("una respuesta de hilo no debe aparecer en el feed del canal: %s", m.ID)
		}
		if m.ID == raiz.ID {
			vioRaiz = true
		}
	}
	if !vioRaiz {
		t.Fatalf("la ventana debe estar anclada en la raíz %s: %+v", raiz.ID, win)
	}
}
