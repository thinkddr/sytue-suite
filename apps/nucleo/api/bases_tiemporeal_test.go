// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// TestBasesEventsLeeElFlujoYReanudaPorElCursor: el flujo se abre con la versión del
// contrato, la ventana y el `Last-Event-ID`; cada evento llega leído, en orden, y el
// cursor avanza también con los bloques de solo `id:`, que no disparan nada.
func TestBasesEventsLeeElFlujoYReanudaPorElCursor(t *testing.T) {
	var pedido *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedido = r
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`event: hello`, `data: {"v":1,"cursor":"c0","role":"editor"}`, ``,
			`: latido`, ``,
			`id: c1`, `event: change`, `data: {"action":"record.update","table_id":"t1","record_id":"r1","field_id":"f1","actor":"u2","op_id":"op-9"}`, ``,
			`event: presence`, `data: [{"conn":"otra-ventana","sub":"u2","name":"Bea","table_id":"t1","record_id":"r1","field_id":"f1"}]`, ``,
			`id: c2`, ``,
			`event: change`, `data: no es json`, ``,
			`event: futuro`, `data: {}`, ``,
			`id: c3`, `event: resync`, `data: {}`, ``,
			`event: bye`, `data: {"reason":"access"}`, ``,
		}, "\n")+"\n")
	}))
	defer srv.Close()

	var eventos []BasesEvento
	var cursores []string
	err := NewBases(srv.URL, nil, nil).Events(context.Background(), "b1", "mi-ventana-1", "c0",
		func(c string) { cursores = append(cursores, c) },
		func(e BasesEvento) { eventos = append(eventos, e) })
	if err != nil {
		t.Fatal(err)
	}
	if pedido.URL.Path != "/api/v1/bases/b1/events" || pedido.URL.Query().Get("v") != "1" ||
		pedido.URL.Query().Get("conn") != "mi-ventana-1" || pedido.Header.Get("Last-Event-ID") != "c0" ||
		pedido.Header.Get("Accept") != "text/event-stream" {
		t.Fatalf("el flujo no se abre así: %s %v", pedido.URL, pedido.Header)
	}
	quiero := []BasesEvento{
		{Tipo: BasesHola},
		{Tipo: BasesCambio, Cambio: BasesChange{Action: "record.update", TableID: "t1", RecordID: "r1", FieldID: "f1", Actor: "u2", OpID: "op-9"}},
		{Tipo: BasesPresencia, Presentes: []BasesPresence{{Conn: "otra-ventana", Sub: "u2", Name: "Bea", TableID: "t1", RecordID: "r1", FieldID: "f1"}}},
		{Tipo: BasesResync},
		{Tipo: BasesAdios, Motivo: BasesSinAcceso},
	}
	if !reflect.DeepEqual(eventos, quiero) {
		t.Fatalf("eventos = %+v", eventos)
	}
	if !reflect.DeepEqual(cursores, []string{"c1", "c2", "c3"}) {
		t.Fatalf("cursores = %q", cursores)
	}
}

// TestBasesEventsSinAccesoEsUnErrorDeLaAPI: sin acceso el servicio contesta 404 antes
// de abrir el flujo, y no se llama a nadie.
func TestBasesEventsSinAccesoEsUnErrorDeLaAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"code":"not_found"}}`)
	}))
	defer srv.Close()
	err := NewBases(srv.URL, nil, nil).Events(context.Background(), "b1", "", "", nil, func(BasesEvento) {
		t.Fatal("sin flujo no hay eventos")
	})
	var e *Error
	if !errors.As(err, &e) || e.Status != http.StatusNotFound {
		t.Fatalf("err = %v", err)
	}
}

// TestBasesHeartbeatMandaLaVentanaYLaCelda: el latido es un PUT con la ventana y, si
// hay, la tabla y la celda; lo que no hay no viaja. Y no avisa al widget de que algo ha
// cambiado: no cambia nada de lo que enseña.
func TestBasesHeartbeatMandaLaVentanaYLaCelda(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		vistas = append(vistas, r.Method+" "+r.URL.Path+" "+string(cuerpo))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	avisos := 0
	b, ctx := NewBases(srv.URL, nil, ConAvisoDeEscritura(nil, func() { avisos++ })), context.Background()
	if err := b.Heartbeat(ctx, "b1", BasesLatido{Conn: "mi-ventana-1", TableID: "t1", RecordID: "r1", FieldID: "f1"}); err != nil {
		t.Fatal(err)
	}
	if err := b.Heartbeat(ctx, "b1", BasesLatido{Conn: "mi-ventana-1"}); err != nil {
		t.Fatal(err)
	}
	quiero := []string{
		`PUT /api/v1/bases/b1/presence {"conn":"mi-ventana-1","table_id":"t1","record_id":"r1","field_id":"f1"}`,
		`PUT /api/v1/bases/b1/presence {"conn":"mi-ventana-1"}`,
	}
	if !reflect.DeepEqual(vistas, quiero) || avisos != 0 {
		t.Fatalf("latidos = %q, avisos al widget = %d", vistas, avisos)
	}
}

// TestLeerSSEConIDDaElUltimoID: el id sigue valiendo para los eventos que no traen el
// suyo, y un bloque de solo `id:` también se despacha (leerSSE, en cambio, lo salta).
func TestLeerSSEConIDDaElUltimoID(t *testing.T) {
	flujo := "id: 7\nevent: a\ndata: 1\n\nevent: b\ndata: 2\n\nid: 8\n\n"
	var vistos []string
	if err := leerSSEConID(strings.NewReader(flujo), func(id, evento, datos string) {
		vistos = append(vistos, id+"|"+evento+"|"+datos)
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(vistos, []string{"7|a|1", "7|b|2", "8||"}) {
		t.Fatalf("vistos = %q", vistos)
	}
	var sinID []string
	_ = leerSSE(strings.NewReader(flujo), func(evento, datos string) { sinID = append(sinID, evento+datos) })
	if !reflect.DeepEqual(sinID, []string{"a1", "b2"}) {
		t.Fatalf("leerSSE = %q", sinID)
	}
}
