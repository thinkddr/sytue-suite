// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package social

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
)

// Un comentario vive en la organización del VÍDEO: una persona de otra
// organización invitada a un vídeo público escribe ahí, y desde su propia
// organización no aparece nada.
func TestComentarios_ViveEnLaOrganizacionDelVideo(t *testing.T) {
	h := montar(t)
	ctx := ctxCorto(t)
	acc := nuevoAcceso()
	svc := New(h.Pool, acc.resolver)
	v := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "public", "ready"), true)

	ana := persona("ana", "acme")
	bea := persona("bea", "globex") // de otra organización, ve el vídeo por ser público
	c1, err := svc.Comentar(ctx, ana, v.ID, "  Primero  ", nil)
	if err != nil {
		t.Fatal(err)
	}
	if c1.Body != "Primero" || c1.Author.Name != "Persona ana" || !c1.Can.Delete {
		t.Fatalf("comentario devuelto: %+v", c1)
	}
	if _, err := svc.Comentar(ctx, bea, v.ID, "Desde fuera", nil); err != nil {
		t.Fatal(err)
	}

	lista, siguiente, err := svc.Comentarios(ctx, ana, v.ID, "", 0)
	if err != nil || len(lista) != 2 || siguiente != "" {
		t.Fatalf("lista = %d, siguiente %q, %v", len(lista), siguiente, err)
	}
	if lista[0].ID != c1.ID || lista[1].Author.Sub != "bea" || lista[1].Can.Delete {
		t.Fatalf("orden o permisos: %+v", lista)
	}

	// La fila de bea está en acme, no en globex: leyendo en globex no hay nada.
	fuera, err := svc.store.comentarios(ctx, Video{ID: v.ID, Tenant: "globex"}, "", nil, 10)
	if err != nil || len(fuera) != 0 {
		t.Fatalf("RLS: en globex se ven %d comentarios (%v)", len(fuera), err)
	}
}

func TestComentarios_PaginaConCursor(t *testing.T) {
	h := montar(t)
	ctx := ctxCorto(t)
	acc := nuevoAcceso()
	svc := New(h.Pool, acc.resolver)
	v := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "org", "ready"), false)
	ana := persona("ana", "acme")
	var ids []uuid.UUID
	for i := 0; i < 5; i++ {
		c, err := svc.Comentar(ctx, ana, v.ID, "n."+string(rune('0'+i)), nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	p1, cur, err := svc.Comentarios(ctx, ana, v.ID, "", 2)
	if err != nil || len(p1) != 2 || cur == "" {
		t.Fatalf("página 1: %d, %q, %v", len(p1), cur, err)
	}
	p2, cur2, err := svc.Comentarios(ctx, ana, v.ID, cur, 2)
	if err != nil || len(p2) != 2 || cur2 == "" {
		t.Fatalf("página 2: %d, %q, %v", len(p2), cur2, err)
	}
	p3, cur3, err := svc.Comentarios(ctx, ana, v.ID, cur2, 2)
	if err != nil || len(p3) != 1 || cur3 != "" {
		t.Fatalf("página 3: %d, %q, %v", len(p3), cur3, err)
	}
	vistos := append(append(p1, p2...), p3...)
	for i, c := range vistos {
		if c.ID != ids[i] {
			t.Fatalf("posición %d: %s, quiero %s", i, c.ID, ids[i])
		}
	}
	if _, _, err := svc.Comentarios(ctx, ana, v.ID, "no-es-un-cursor", 2); !errors.Is(err, ErrEntradaInvalida) {
		t.Fatalf("cursor malo: %v", err)
	}
}

// Hilos de un nivel: responder a una respuesta cuelga de la raíz. Borrar una raíz
// con respuestas deja un hueco; borrar una sin respuestas la hace desaparecer.
func TestComentarios_HilosYHuecos(t *testing.T) {
	h := montar(t)
	ctx := ctxCorto(t)
	acc := nuevoAcceso()
	svc := New(h.Pool, acc.resolver)
	v := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "org", "ready"), false)
	ana, bea := persona("ana", "acme"), persona("bea", "acme")

	raiz, _ := svc.Comentar(ctx, ana, v.ID, "raíz", nil)
	resp, err := svc.Comentar(ctx, bea, v.ID, "respuesta", &raiz.ID)
	if err != nil || resp.ReplyTo == nil || *resp.ReplyTo != raiz.ID {
		t.Fatalf("respuesta: %+v, %v", resp, err)
	}
	anidada, err := svc.Comentar(ctx, ana, v.ID, "a la respuesta", &resp.ID)
	if err != nil || anidada.ReplyTo == nil || *anidada.ReplyTo != raiz.ID {
		t.Fatalf("la anidada no se aplanó a la raíz: %+v, %v", anidada, err)
	}
	inexistente := uuid.New()
	if _, err := svc.Comentar(ctx, ana, v.ID, "a nada", &inexistente); !errors.Is(err, ErrNoExiste) {
		t.Fatalf("responder a un comentario inexistente: %v", err)
	}

	if err := svc.BorrarComentario(ctx, ana, v.ID, raiz.ID); err != nil {
		t.Fatal(err)
	}
	lista, _, _ := svc.Comentarios(ctx, ana, v.ID, "", 0)
	if len(lista) != 3 || !lista[0].Deleted || lista[0].Body != "" || lista[0].Author.Sub != "" || lista[0].Can.Delete {
		t.Fatalf("la raíz borrada con respuestas debe quedar como hueco: %+v", lista)
	}
	suelto, _ := svc.Comentar(ctx, ana, v.ID, "suelto", nil)
	if err := svc.BorrarComentario(ctx, ana, v.ID, suelto.ID); err != nil {
		t.Fatal(err)
	}
	lista, _, _ = svc.Comentarios(ctx, ana, v.ID, "", 0)
	for _, c := range lista {
		if c.ID == suelto.ID {
			t.Fatalf("un comentario borrado sin respuestas no debe aparecer: %+v", lista)
		}
	}
}

// Quién borra: el autor, el dueño del vídeo y quien administra la organización.
// Nadie más, y lo que no existe es «no existe», no «prohibido».
func TestComentarios_QuienBorra(t *testing.T) {
	h := montar(t)
	ctx := ctxCorto(t)
	acc := nuevoAcceso()
	acc.administra["admin"] = true
	svc := New(h.Pool, acc.resolver)
	v := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "public", "ready"), true)
	ana, bea, duenya, admin := persona("ana", "acme"), persona("bea", "acme"), persona("duenya", "acme"), persona("admin", "acme")
	adminDeFuera := persona("admin", "globex")
	acc.administra["admin"] = true

	nuevo := func() uuid.UUID {
		c, err := svc.Comentar(ctx, ana, v.ID, "de ana", nil)
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	if err := svc.BorrarComentario(ctx, bea, v.ID, nuevo()); !errors.Is(err, ErrProhibido) {
		t.Fatalf("otra persona: %v", err)
	}
	if err := svc.BorrarComentario(ctx, adminDeFuera, v.ID, nuevo()); !errors.Is(err, ErrProhibido) {
		t.Fatalf("admin de OTRA organización no modera aquí: %v", err)
	}
	if err := svc.BorrarComentario(ctx, nadie, v.ID, nuevo()); !errors.Is(err, ErrProhibido) {
		t.Fatalf("sin sesión: %v", err)
	}
	for nombre, q := range map[string]Quien{"autor": ana, "dueño": duenya, "admin": admin} {
		if err := svc.BorrarComentario(ctx, q, v.ID, nuevo()); err != nil {
			t.Fatalf("%s no pudo borrar: %v", nombre, err)
		}
	}
	if err := svc.BorrarComentario(ctx, ana, v.ID, uuid.New()); !errors.Is(err, ErrNoExiste) {
		t.Fatalf("inexistente: %v", err)
	}
}

// Validación: cuerpo vacío o por encima del tope, comentarios apagados, sin sesión.
func TestComentarios_Validacion(t *testing.T) {
	h := montar(t)
	ctx := ctxCorto(t)
	acc := nuevoAcceso()
	svc := New(h.Pool, acc.resolver)
	v := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "org", "ready"), false)
	ana := persona("ana", "acme")
	if _, err := svc.Comentar(ctx, ana, v.ID, "   ", nil); !errors.Is(err, ErrEntradaInvalida) {
		t.Errorf("vacío: %v", err)
	}
	if _, err := svc.Comentar(ctx, ana, v.ID, strings.Repeat("x", MaxComentario+1), nil); !errors.Is(err, ErrEntradaInvalida) {
		t.Errorf("largo: %v", err)
	}
	if _, err := svc.Comentar(ctx, nadie, v.ID, "hola", nil); !errors.Is(err, ErrProhibido) {
		t.Errorf("sin sesión: %v", err)
	}
	apagado := v
	apagado.ComentariosActivos = false
	acc.videos[v.ID] = apagado
	if _, err := svc.Comentar(ctx, ana, v.ID, "hola", nil); !errors.Is(err, ErrDesactivado) {
		t.Errorf("comentarios apagados: %v", err)
	}
	// Leer sigue pudiéndose con los comentarios apagados.
	if _, _, err := svc.Comentarios(ctx, ana, v.ID, "", 0); err != nil {
		t.Errorf("leer con comentarios apagados: %v", err)
	}
}

// La política pública: sin sesión se leen los comentarios de un vídeo public o
// unlisted; los de un vídeo `org` no; y sin la variable no se ve NADA, ni
// siquiera los del vídeo público.
func TestComentarios_PoliticaPublica(t *testing.T) {
	h := montar(t)
	ctx := ctxCorto(t)
	acc := nuevoAcceso()
	svc := New(h.Pool, acc.resolver)
	publico := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "public", "ready"), true)
	oculto := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "unlisted", "live"), true)
	interno := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "org", "ready"), false)
	ana := persona("ana", "acme")
	for _, v := range []Video{publico, oculto, interno} {
		if _, err := svc.Comentar(ctx, ana, v.ID, "hola", nil); err != nil {
			t.Fatal(err)
		}
	}

	for nombre, v := range map[string]Video{"public": publico, "unlisted": oculto} {
		lista, _, err := svc.Comentarios(ctx, nadie, v.ID, "", 0)
		if err != nil || len(lista) != 1 || lista[0].Can.Delete {
			t.Fatalf("%s sin sesión: %d comentarios, %v (%+v)", nombre, len(lista), err, lista)
		}
	}
	if _, _, err := svc.Comentarios(ctx, nadie, interno.ID, "", 0); !errors.Is(err, ErrNoExiste) {
		t.Fatalf("vídeo org sin sesión: %v", err)
	}
	// Aunque el resolutor de acceso se equivocara, la POLÍTICA no deja ver los
	// comentarios de un vídeo `org` por la vía pública.
	if lista, err := svc.store.comentariosPublicos(ctx, interno.ID, nil, 10); err != nil || len(lista) != 0 {
		t.Fatalf("la política pública enseñó %d comentarios de un vídeo org (%v)", len(lista), err)
	}
	// Y sin la variable —la lectura normal sin organización fijada— no hay filas.
	if lista, err := svc.store.comentarios(ctx, Video{ID: publico.ID, Tenant: "ninguna"}, "", nil, 10); err != nil || len(lista) != 0 {
		t.Fatalf("sin app.cast_publico se vieron %d comentarios (%v)", len(lista), err)
	}
	// El estado del vídeo también cuenta: un vídeo público aún en proceso no se abre.
	videoEnProceso := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "public", "processing"), true)
	if _, err := svc.Comentar(ctx, ana, videoEnProceso.ID, "pronto", nil); err != nil {
		t.Fatal(err)
	}
	if lista, err := svc.store.comentariosPublicos(ctx, videoEnProceso.ID, nil, 10); err != nil || len(lista) != 0 {
		t.Fatalf("un vídeo public en proceso enseñó %d comentarios por la vía pública (%v)", len(lista), err)
	}
}

// «Exportar tus datos»: cada autor lee SUS comentarios en cualquier organización, y
// solo los suyos (política comments_autor con app.cast_persona).
func TestComentarios_DeUnAutorCruzanOrganizaciones(t *testing.T) {
	h := montar(t)
	ctx := ctxCorto(t)
	acc := nuevoAcceso()
	svc := New(h.Pool, acc.resolver)
	enAcme := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "public", "ready"), true)
	enGlobex := acc.conoce(videoEn(t, h.Pool, "globex", "otra", "public", "ready"), true)

	bea := persona("bea", "globex")
	for _, v := range []Video{enAcme, enGlobex} {
		if _, err := svc.Comentar(ctx, bea, v.ID, "Hola desde "+v.Tenant, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Comentar(ctx, persona("ana", "acme"), enAcme.ID, "De otra persona", nil); err != nil {
		t.Fatal(err)
	}

	suyos, err := svc.ComentariosDe(ctx, bea)
	if err != nil || len(suyos) != 2 {
		t.Fatalf("bea tiene %d comentarios exportables (%v), esperaba 2", len(suyos), err)
	}
	for _, c := range suyos {
		if c.Body == "De otra persona" {
			t.Fatal("se exportó un comentario ajeno")
		}
	}
	if _, err := svc.ComentariosDe(ctx, nadie); err == nil {
		t.Fatal("sin sesión no se exporta nada")
	}
}

// El WebSocket vuelve a resolver el acceso en cada envío: si el vídeo deja de
// verse, el siguiente mensaje recibe un error y la conexión se cierra.
func TestChat_PierdeElAccesoYSeCierra(t *testing.T) {
	h := montar(t)
	acc := nuevoAcceso()
	svc := New(h.Pool, acc.resolver)
	v := acc.conoce(videoEn(t, h.Pool, "acme", "duenya", "org", "live"), false)
	abrir, ctx := servidorDeChat(t, svc, v, time.Time{})
	ana := abrir("ana")
	if err := wsjson.Write(ctx, ana, Orden{Type: OrdenEnviar, Body: "antes"}); err != nil {
		t.Fatal(err)
	}
	for {
		var tr Trama
		if err := wsjson.Read(ctx, ana, &tr); err != nil {
			t.Fatal(err)
		}
		if tr.Type == TramaMensaje {
			break
		}
	}
	delete(acc.videos, v.ID) // lo borran o le quitan el acceso
	if err := wsjson.Write(ctx, ana, Orden{Type: OrdenEnviar, Body: "después"}); err != nil {
		t.Fatal(err)
	}
	vioError := false
	for {
		var tr Trama
		err := wsjson.Read(ctx, ana, &tr)
		if err != nil {
			if !vioError {
				t.Fatalf("la conexión se cerró sin avisar: %v", err)
			}
			if errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("la conexión no se cerró al perder el acceso")
			}
			return
		}
		if tr.Type == TramaError && tr.Code == "not_found" {
			vioError = true
		}
	}
}
