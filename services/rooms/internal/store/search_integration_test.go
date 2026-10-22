// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del MOTOR DE BÚSQUEDA (Ola 2D de paridad de Rooms) contra un
// PostgreSQL real: es en la consulta —no en el parser— donde se comprueba de verdad
// el AND de términos, el escape de comodines, la frase exacta, la exclusión y los
// operadores on:/has:/is:. Fijan además el invariante que más importa: la búsqueda
// NUNCA devuelve mensajes de un canal al que el llamante no tiene acceso.
package store_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// Corpus del canal público. Cada cuerpo está elegido para que un caso lo encuentre y
// su vecino sirva de señuelo (mismas palabras en otro orden, «50» sin «%», «http»
// sin URL): así un filtro que se pase de ancho falla el test.
const (
	bViernes  = "el viernes hay reunión de equipo"
	bReunion  = "hablamos de http en la reunión, sin enlace"
	bInforme  = "informe de cierre mensual listo"
	bCierre   = "el cierre del informe va con retraso"
	bPorcien  = "la subida del 50% ya se aprobó"
	bCincuen  = "el descuento del 50 en la tienda"
	bPlan     = "plan definitivo aprobado"
	bBorrador = "plan borrador sin revisar"
	bEnlace   = "mira https://sytue.eu/tablero"
	bAdjunto  = "adjunto el contrato firmado"
	bGuardado = "esto lo guardo para luego"
	bFijado   = "esto queda fijado arriba"
	bAntiguo  = "el balance del día antiguo"
	bSecreto  = "secreto del canal privado"
	bPolitica = "nueva política de vacaciones"
)

// diaAntiguo es el día al que se retrasa bAntiguo para poder fijar on:/before:/after:
// sin depender de la fecha en que se ejecute el test.
const diaAntiguo = "2024-03-05"

// backdate mueve un mensaje en el tiempo: la BD pone now() al insertar y no hay API
// para elegir created_at, así que el test de fechas lo ajusta por SQL directo (dentro
// de una tx con app.tenant fijado, o RLS lo rechazaría).
func backdate(t *testing.T, ctx context.Context, h *pgtest.Harness, id uuid.UUID, day string) {
	t.Helper()
	at, err := time.Parse("2006-01-02", day)
	if err != nil {
		t.Fatalf("fecha de test inválida %q: %v", day, err)
	}
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE messages SET created_at=$1 WHERE id=$2`, at.Add(12*time.Hour), id)
		return err
	}); err != nil {
		t.Fatalf("backdate: %v", err)
	}
}

func TestSearchEngine(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, nil)
	ctx := tenantCtx("acme")

	tm, ch := seedTeamChannel(t, ctx, st, "alice", domain.ChannelPublic)
	// bob es miembro del team: ve el canal público, NO el privado de carol.
	for _, sub := range []string{"bob", "carol"} {
		if err := st.AddTeamMember(ctx, tm.ID, "alice", sub, sub, domain.RoleMember); err != nil {
			t.Fatalf("AddTeamMember(%s): %v", sub, err)
		}
	}

	post := func(channel uuid.UUID, author, body string, att ...domain.Attachment) domain.Message {
		t.Helper()
		m, err := st.CreateMessage(ctx, store.NewMessageRecord{
			ChannelID: channel, AuthorSub: author, AuthorName: author, Body: body, Attachments: att,
		})
		if err != nil {
			t.Fatalf("CreateMessage(%q): %v", body, err)
		}
		return m
	}

	for _, body := range []string{
		bViernes, bReunion, bInforme, bCierre, bPorcien, bCincuen, bPlan, bBorrador, bEnlace, bPolitica,
	} {
		post(ch.ID, "alice", body)
	}
	post(ch.ID, "alice", bAdjunto, domain.Attachment{FileID: uuid.New(), Name: "contrato.pdf"})
	guardado := post(ch.ID, "alice", bGuardado)
	fijado := post(ch.ID, "alice", bFijado)
	backdate(t, ctx, h, post(ch.ID, "alice", bAntiguo).ID, diaAntiguo)

	// Guardado por BOB (los guardados son por-usuario) y fijado en el canal.
	if err := st.SaveMessage(ctx, guardado.ID, "bob"); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}
	if _, err := st.PinMessage(ctx, fijado.ID, "bob"); err != nil {
		t.Fatalf("PinMessage: %v", err)
	}

	// Canal PRIVADO de carol: bob no es miembro, así que su mensaje no debe salir
	// en ninguna búsqueda de bob, por ancha que sea.
	priv, err := st.CreateChannel(ctx, store.NewChannelRecord{
		TeamID: tm.ID, Slug: "priv", Name: "Privado", Kind: domain.ChannelPrivate, CreatedBy: "carol",
	})
	if err != nil {
		t.Fatalf("CreateChannel privado: %v", err)
	}
	post(priv.ID, "carol", bSecreto)

	// bodies devuelve, ordenados, los cuerpos que la búsqueda global entrega a sub.
	bodies := func(t *testing.T, sub, q string) []string {
		t.Helper()
		msgs, err := st.SearchAll(ctx, sub, q, 100)
		if err != nil {
			t.Fatalf("SearchAll(%q): %v", q, err)
		}
		out := make([]string, 0, len(msgs))
		for _, m := range msgs {
			out = append(out, m.Body)
		}
		slices.Sort(out)
		return out
	}
	// want compara el resultado exacto de una consulta (ni de menos ni de más).
	want := func(t *testing.T, q string, esperado ...string) {
		t.Helper()
		slices.Sort(esperado)
		if got := bodies(t, "bob", q); !slices.Equal(got, esperado) {
			t.Fatalf("q=%q\n got  %q\n want %q", q, got, esperado)
		}
	}

	// (a) El caso más común: varias palabras casan TODAS, en cualquier orden y
	// posición. Antes se buscaba «reunión viernes» como UNA subcadena literal.
	t.Run("AND de términos en cualquier orden", func(t *testing.T) {
		want(t, "reunión viernes", bViernes)
		want(t, "viernes reunión", bViernes)
		want(t, "VIERNES reunión", bViernes)   // sigue siendo insensible a mayúsculas
		want(t, "reunión", bViernes, bReunion) // una sola palabra casa las dos
		want(t, "reunión inexistente")         // pero se exigen TODAS las palabras
		// Sin acentos ni mayúsculas, en los dos sentidos (buscar.Plegar ↔ plegar()).
		want(t, "reunion", bViernes, bReunion)
		want(t, "REUNION VIERNES", bViernes)
		want(t, "aprobo", bPorcien) // «aprobó» en el texto
		want(t, "INFÓRME", bInforme, bCierre)
		want(t, "politica", bPolitica)
		want(t, "POLÍTICA vacaciones", bPolitica)
	})

	// (b) Comodines escapados: lo que el usuario escribe es LITERAL.
	t.Run("comodines del usuario escapados", func(t *testing.T) {
		want(t, "50%", bPorcien) // no casa bCincuen («50» sin «%»)
		want(t, "%", bPorcien)   // antes: TODO el historial accesible
		want(t, "_")             // «_» no es «un carácter cualquiera»
		// Contraste: el corpus accesible es mucho mayor que lo que devuelve «%».
		if todo := bodies(t, "bob", "from:alice"); len(todo) < 13 {
			t.Fatalf("el corpus de alice debería ser amplio, got %d: %q", len(todo), todo)
		}
	})

	// (c) Frase exacta entre comillas: la secuencia, no las palabras sueltas.
	t.Run("frase exacta", func(t *testing.T) {
		want(t, `"informe de cierre"`, bInforme)
		want(t, "informe cierre", bInforme, bCierre) // sin comillas: AND de palabras
		want(t, `"cierre de informe"`)
	})

	// (d) Exclusión con «-».
	t.Run("exclusión con guion", func(t *testing.T) {
		want(t, "plan", bPlan, bBorrador)
		want(t, "plan -borrador", bPlan)
		want(t, `plan -"borrador sin revisar"`, bPlan)
	})

	// (e) Operadores nuevos: cada uno filtra exactamente lo que dice.
	t.Run("operadores on/has/is", func(t *testing.T) {
		want(t, "on:"+diaAntiguo, bAntiguo)
		want(t, "on:2024-03-06")
		want(t, "balance on:"+diaAntiguo, bAntiguo)
		want(t, "has:link", bEnlace) // bReunion dice «http» pero no lleva URL
		want(t, "has:file", bAdjunto)
		want(t, "is:pinned", bFijado)
		want(t, "is:saved", bGuardado)
		// is:saved es POR-USUARIO: alice no guardó nada.
		if got := bodies(t, "alice", "is:saved"); len(got) != 0 {
			t.Fatalf("is:saved debe ser por-usuario, alice ve %q", got)
		}
		// Los operadores se combinan con AND (y con el texto).
		want(t, "has:file from:alice in:canal", bAdjunto)
		want(t, "has:file has:link")
	})

	// (f) AISLAMIENTO POR-USUARIO: el canal privado de carol es invisible para bob
	// incluso con la consulta más ancha posible (solo-negativa = «todo menos X»).
	t.Run("aislamiento por usuario", func(t *testing.T) {
		want(t, "secreto")
		if got := bodies(t, "bob", "-inexistente"); slices.Contains(got, bSecreto) {
			t.Fatalf("bob no debe ver el canal privado de carol: %q", got)
		}
		if got := bodies(t, "carol", "secreto"); !slices.Equal(got, []string{bSecreto}) {
			t.Fatalf("carol sí debe encontrar su mensaje: %q", got)
		}
		// Y lo mismo por la búsqueda acotada al team, que es OTRA consulta.
		msgs, err := st.SearchMessages(ctx, tm.ID, "bob", "-inexistente", 100)
		if err != nil {
			t.Fatalf("SearchMessages: %v", err)
		}
		for _, m := range msgs {
			if m.Body == bSecreto {
				t.Fatalf("SearchMessages filtró mal: bob ve el canal privado")
			}
		}
	})

	// La búsqueda del team comparte el ensamblado, así que basta una pasada que
	// confirme que el AND de términos y el escape también llegan por esa vía.
	t.Run("misma semántica en la búsqueda del team", func(t *testing.T) {
		for q, esperado := range map[string][]string{
			"reunión viernes":      {bViernes},
			"%":                    {bPorcien},
			`"informe de cierre"`:  {bInforme},
			"plan -borrador":       {bPlan},
			"is:pinned":            {bFijado},
			"on:" + diaAntiguo:     {bAntiguo},
			"contrato has:file":    {bAdjunto},
			"tablero has:link":     {bEnlace},
			"inexistente has:link": nil,
			"reunión viernes -hay": nil,
		} {
			msgs, err := st.SearchMessages(ctx, tm.ID, "bob", q, 100)
			if err != nil {
				t.Fatalf("SearchMessages(%q): %v", q, err)
			}
			got := make([]string, 0, len(msgs))
			for _, m := range msgs {
				got = append(got, m.Body)
			}
			slices.Sort(got)
			slices.Sort(esperado)
			if !slices.Equal(got, esperado) {
				t.Errorf("SearchMessages(%q)\n got  %q\n want %q", q, got, esperado)
			}
		}
	})
}
