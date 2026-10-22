// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package social

import (
	"errors"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/jackc/pgx/v5"
)

// De extremo a extremo con el store real: dos personas conectadas, una escribe y
// las dos lo reciben; el historial lo tiene; la moderación lo retira y avisa.
func TestChat_EnviarRecibirYModerar(t *testing.T) {
	h := montar(t)
	acc := nuevoAcceso()
	svc := New(h.Pool, acc.resolver)
	v := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "org", "live"), false)
	abrir, ctx := servidorDeChat(t, svc, v, time.Time{})

	ana, bea := abrir("ana"), abrir("bea")
	leerTipo(ctx, t, ana, TramaEspectadores)
	leerTipo(ctx, t, bea, TramaEspectadores)

	if err := wsjson.Write(ctx, ana, Orden{Type: OrdenEnviar, Body: "  ¡Hola a todos!  "}); err != nil {
		t.Fatal(err)
	}
	deAna := leerTipo(ctx, t, ana, TramaMensaje)
	deBea := leerTipo(ctx, t, bea, TramaMensaje)
	if deAna.Type != TramaMensaje || deAna.Body != "¡Hola a todos!" || deAna.Author == nil || deAna.Author.Sub != "ana" || deAna.ID == nil || deAna.CreatedAt == nil {
		t.Fatalf("lo que recibió ana: %+v", deAna)
	}
	if deBea.ID == nil || *deBea.ID != *deAna.ID {
		t.Fatalf("bea recibió otro mensaje: %+v", deBea)
	}

	// El historial lo tiene, y `since` lo filtra.
	hist, err := svc.HistorialChat(ctx, persona("bea", "acme"), v.ID, time.Time{})
	if err != nil || len(hist) != 1 || hist[0].ID != *deAna.ID || hist[0].Body != "¡Hola a todos!" {
		t.Fatalf("historial: %+v, %v", hist, err)
	}
	if hist, err := svc.HistorialChat(ctx, persona("bea", "acme"), v.ID, hist[0].CreatedAt); err != nil || len(hist) != 0 {
		t.Fatalf("historial desde el último: %+v, %v", hist, err)
	}
	if _, err := svc.HistorialChat(ctx, nadie, v.ID, time.Time{}); !errors.Is(err, ErrProhibido) {
		t.Fatalf("historial sin sesión: %v", err)
	}

	// Bea no puede retirar lo de ana; la dueña sí, y la sala se entera.
	if err := svc.BorrarChat(ctx, persona("bea", "acme"), v.ID, *deAna.ID); !errors.Is(err, ErrProhibido) {
		t.Fatalf("bea borró lo de ana: %v", err)
	}
	if err := svc.BorrarChat(ctx, persona("duenya", "acme"), v.ID, *deAna.ID); err != nil {
		t.Fatal(err)
	}
	for nombre, c := range map[string]*websocket.Conn{"ana": ana, "bea": bea} {
		if tr := leerTipo(ctx, t, c, TramaBorrado); tr.ID == nil || *tr.ID != *deAna.ID {
			t.Fatalf("%s no recibió el borrado: %+v", nombre, tr)
		}
	}
	if hist, _ := svc.HistorialChat(ctx, persona("bea", "acme"), v.ID, time.Time{}); len(hist) != 0 {
		t.Fatalf("el mensaje retirado sigue en el historial: %+v", hist)
	}
}

// Quien manda más deprisa de lo que teclea una persona recibe un error y no se
// le guarda el mensaje.
func TestChat_RitmoPorPersona(t *testing.T) {
	h := montar(t)
	acc := nuevoAcceso()
	svc := New(h.Pool, acc.resolver)
	v := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "org", "live"), false)
	abrir, ctx := servidorDeChat(t, svc, v, time.Time{})
	ana := abrir("ana")
	leerTipo(ctx, t, ana, TramaEspectadores)

	for i := 0; i < 12; i++ {
		if err := wsjson.Write(ctx, ana, Orden{Type: OrdenEnviar, Body: "ráfaga"}); err != nil {
			t.Fatal(err)
		}
	}
	mensajes, frenados := 0, 0
	for mensajes+frenados < 12 {
		var tr Trama
		if err := wsjson.Read(ctx, ana, &tr); err != nil {
			t.Fatal(err)
		}
		switch {
		case tr.Type == TramaMensaje:
			mensajes++
		case tr.Type == TramaError && tr.Code == "too_many_requests":
			frenados++
		case tr.Type == TramaError:
			t.Fatalf("error inesperado: %+v", tr)
		}
	}
	if frenados == 0 || mensajes > 6 {
		t.Fatalf("12 de golpe: %d pasaron, %d frenados", mensajes, frenados)
	}
	hist, _ := svc.HistorialChat(ctx, persona("ana", "acme"), v.ID, time.Time{})
	if len(hist) != mensajes {
		t.Fatalf("se guardaron %d y se difundieron %d", len(hist), mensajes)
	}
}

// El chat se conserva 30 días. La limpieza corre en mantenimiento y cruza
// organizaciones; sin la variable, la misma sentencia no borra nada (guardián).
func TestChat_Limpieza(t *testing.T) {
	h := montar(t)
	ctx := ctxCorto(t)
	acc := nuevoAcceso()
	svc := New(h.Pool, acc.resolver)
	v1 := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "org", "ended"), false)
	v2 := acc.conoce(videoEn(t, h.Pool, "globex", "otra", "org", "ended"), false)
	if _, err := svc.EnviarChat(ctx, persona("ana", "acme"), v1.ID, "reciente"); err != nil {
		t.Fatal(err)
	}
	// Dos mensajes viejos, uno por organización, con la fecha retocada por el rol privilegiado.
	adm, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = adm.Close(ctx) }()
	for _, v := range []Video{v1, v2} {
		if _, err := adm.Exec(ctx, `
			INSERT INTO chat_messages (tenant, video_id, author_sub, body, created_at)
			VALUES ($1, $2, 'x', 'viejo', now() - interval '31 days')`, v.Tenant, v.ID); err != nil {
			t.Fatal(err)
		}
	}
	contar := func() int {
		var n int
		if err := adm.QueryRow(ctx, `SELECT count(*) FROM chat_messages`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if contar() != 3 {
		t.Fatalf("montaje: %d mensajes", contar())
	}

	// Guardián: la misma sentencia SIN mantenimiento (rol de aplicación, sin tenant) no borra nada.
	err = h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM chat_messages WHERE created_at < now() - $1::interval`, RetencionChat)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 0 {
			t.Fatalf("sin app.maintenance se borraron %d filas", tag.RowsAffected())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := svc.Limpiar(ctx)
	if err != nil || n != 2 {
		t.Fatalf("Limpiar = %d, %v", n, err)
	}
	if contar() != 1 {
		t.Fatalf("quedan %d mensajes, quiero 1 (el reciente)", contar())
	}
	if n, err := svc.Limpiar(ctx); err != nil || n != 0 {
		t.Fatalf("segunda pasada: %d, %v", n, err)
	}
}

// El WebSocket no vuelve a resolver el acceso por mensaje, pero el servicio SÍ lo
// hace en la vía REST: sin poder ver el vídeo, ni historial ni envío.
func TestChat_SinAccesoNoHayChat(t *testing.T) {
	h := montar(t)
	ctx := ctxCorto(t)
	acc := nuevoAcceso()
	svc := New(h.Pool, acc.resolver)
	v := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "org", "live"), false)
	deFuera := persona("bea", "globex")
	if _, err := svc.EnviarChat(ctx, deFuera, v.ID, "hola"); !errors.Is(err, ErrNoExiste) {
		t.Fatalf("enviar sin acceso: %v", err)
	}
	if _, err := svc.HistorialChat(ctx, deFuera, v.ID, time.Time{}); !errors.Is(err, ErrNoExiste) {
		t.Fatalf("historial sin acceso: %v", err)
	}
	apagado := v
	apagado.ChatActivo = false
	acc.videos[v.ID] = apagado
	if _, err := svc.EnviarChat(ctx, persona("ana", "acme"), v.ID, "hola"); !errors.Is(err, ErrDesactivado) {
		t.Fatalf("chat apagado: %v", err)
	}
}
