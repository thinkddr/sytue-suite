// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestNotasSobrevivenAReabrir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	n, err := AbrirNotas(dir, "Notas", "notas", "Para mí")
	if err != nil {
		t.Fatal(err)
	}
	cs, _, _ := n.Canales(ctx)
	if len(cs) != 1 || cs[0].Name != "notas" {
		t.Fatalf("canal por omisión: %+v", cs)
	}
	canal := cs[0].ID
	m1, err := n.Enviar(ctx, canal, "", "comprar pan")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.Enviar(ctx, canal, m1.ID, "y leche"); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Editar(ctx, canal, m1.ID, "comprar pan integral"); err != nil {
		t.Fatal(err)
	}
	if err := n.Reaccionar(ctx, canal, m1.ID, "👍", true); err != nil {
		t.Fatal(err)
	}
	otro, err := n.CrearCanal(ctx, "Ideas", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.Enviar(ctx, otro.ID, "", "una idea"); err != nil {
		t.Fatal(err)
	}

	// En claro y legible en el disco.
	raw, err := os.ReadFile(filepath.Join(dir, "mensajes", canal+".json"))
	if err != nil || !strings.Contains(string(raw), "comprar pan integral") {
		t.Fatalf("fichero en claro: %v %s", err, raw)
	}

	n2, err := AbrirNotas(dir, "Notas", "notas", "")
	if err != nil {
		t.Fatal(err)
	}
	cs, _, _ = n2.Canales(ctx)
	if len(cs) != 2 {
		t.Fatalf("canales tras reabrir: %+v", cs)
	}
	ms, err := n2.Mensajes(ctx, canal, time.Time{}, 50)
	if err != nil || len(ms) != 1 {
		t.Fatalf("mensajes tras reabrir (sin respuestas de hilo): %v %+v", err, ms)
	}
	m := ms[0]
	if m.Body != "comprar pan integral" || m.EditedAt == nil || m.ReplyCount != 1 || len(m.Reactions) != 1 || m.AuthorSub != Yo {
		t.Fatalf("mensaje tras reabrir: %+v", m)
	}
	h, err := n2.Hilo(ctx, canal, m.ID)
	if err != nil || len(h.Replies) != 1 || h.Replies[0].Body != "y leche" {
		t.Fatalf("hilo: %v %+v", err, h)
	}
	if err := n2.Borrar(ctx, canal, m.ID); err != nil {
		t.Fatal(err)
	}
	n3, _ := AbrirNotas(dir, "Notas", "notas", "")
	if ms, _ := n3.Mensajes(ctx, canal, time.Time{}, 50); len(ms) != 0 {
		t.Fatalf("borrado no persistió: %+v", ms)
	}
}

func TestAgregadorPrefijaYGuardaPreferencias(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	n, err := AbrirNotas(filepath.Join(dir, "rooms"), "Notas", "notas", "")
	if err != nil {
		t.Fatal(err)
	}
	prefs := filepath.Join(dir, "preferencias.json")
	a := NuevoAgregador(prefs)
	a.Poner(CuentaNotas, n)
	cs, err := a.Channels(ctx, Equipo)
	if err != nil || len(cs) != 1 || !strings.HasPrefix(cs[0].ID, CuentaNotas+"|") || cs[0].CategoryID != "" {
		t.Fatalf("canales: %v %+v", err, cs)
	}
	m, err := a.Post(ctx, cs[0].ID, "hola")
	if err != nil || m.ChannelID != cs[0].ID || !strings.HasPrefix(m.ID, cs[0].ID+"|") {
		t.Fatalf("post: %v %+v", err, m)
	}
	r, err := a.ReplyWith(ctx, m.ID, "respuesta", api.RoomsSend{})
	if err != nil || r.ParentID == nil || *r.ParentID != m.ID {
		t.Fatalf("respuesta: %v %+v", err, r)
	}
	if _, err := a.Edit(ctx, m.ID, "hola!"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetFavorite(ctx, cs[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if err := a.Save(ctx, m.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := a.PutDraft(ctx, cs[0].ID, "", "a medias"); err != nil {
		t.Fatal(err)
	}
	if res, _ := a.Search(ctx, "HOLA", 10); len(res) != 1 {
		t.Fatalf("búsqueda: %+v", res)
	}

	// Otra cuenta: cada una pasa a ser una categoría.
	n2, _ := AbrirNotas(filepath.Join(dir, "otra"), "Otra", "general", "")
	a.Poner("otra", n2)
	cats, _ := a.Categories(ctx)
	if len(cats) != 2 {
		t.Fatalf("categorías por cuenta: %+v", cats)
	}

	b := NuevoAgregador(prefs)
	b.Poner(CuentaNotas, n)
	cs2, _ := b.Channels(ctx, Equipo)
	if !cs2[0].Favorite {
		t.Fatal("el favorito no persistió")
	}
	if s, _ := b.Saved(ctx); len(s) != 1 || s[0].Body != "hola!" {
		t.Fatalf("guardados: %+v", s)
	}
	if d, _ := b.Drafts(ctx); len(d) != 1 || d[0].Body != "a medias" {
		t.Fatalf("borradores: %+v", d)
	}
}

func TestAgregadorEscuchaYPrefija(t *testing.T) {
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	n, _ := AbrirNotas(t.TempDir(), "Notas", "notas", "")
	a := NuevoAgregador("")
	a.Poner(CuentaNotas, n)
	llegan := make(chan api.RoomsEvent, 8)
	hecho := make(chan error, 1)
	go func() { hecho <- a.Listen(ctx, nil, func(ev api.RoomsEvent) { llegan <- ev }) }()
	if ev := <-llegan; ev.Type != api.RoomsReady {
		t.Fatalf("primera trama: %+v", ev)
	}
	cs, _ := a.Channels(ctx, Equipo)
	// La escucha de la cuenta arranca en su gorrutina: se espera a que esté apuntada.
	for range 100 {
		n.mu.Lock()
		listo := len(n.oyentes) > 0
		n.mu.Unlock()
		if listo {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := a.Post(ctx, cs[0].ID, "x"); err != nil {
		t.Fatal(err)
	}
	ev := <-llegan
	if ev.Type != "message" || ev.ChannelID != cs[0].ID || !strings.Contains(string(ev.Payload), cs[0].ID) {
		t.Fatalf("trama: %+v %s", ev, ev.Payload)
	}
	cancelar()
	if err := <-hecho; err == nil {
		t.Fatal("Listen debe volver con el porqué")
	}
}
