// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Las rutas y los cuerpos son los del servicio (services/rooms/internal/http/
// moderacion.go), y en NINGUNO viaja quién denuncia o quién bloquea: el servicio
// rechaza con 400 un cuerpo con campos de más, y el actor lo pone el token.
func TestRoomsDenunciarYBloquear(t *testing.T) {
	vistas := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		vistas[r.Method+" "+r.URL.Path] = string(cuerpo)
		switch r.Method + " " + r.URL.Path {
		case "PUT /api/v1/channels/c1/members/u2/block":
			_, _ = io.WriteString(w, `{"subject":"u2","name":"Grace"}`)
		case "GET /api/v1/me/blocks":
			_, _ = io.WriteString(w, `{"items":[{"subject":"u2","name":"Grace"}]}`)
		case "GET /api/v1/moderation/reports":
			_, _ = io.WriteString(w, `{"items":[{"id":"r1","channel_id":"c1","channel_name":"general","channel_kind":"public","target_sub":"u2","target_name":"Grace","message_id":"m1","message_body":"texto","reason":"spam","created_at":"2026-10-06T10:00:00Z"}]}`)
		case "DELETE /api/v1/me/blocks/u2":
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = io.WriteString(w, `{"id":"r1"}`)
		}
	}))
	defer srv.Close()
	r := NewRooms(srv.URL, nil, nil)
	ctx := context.Background()

	if err := r.ReportMessage(ctx, "m1", "spam", "nota"); err != nil {
		t.Fatal(err)
	}
	if err := r.ReportMember(ctx, "c1", "u2", "harassment", ""); err != nil {
		t.Fatal(err)
	}
	if b, err := r.Block(ctx, "c1", "u2"); err != nil || b.Name != "Grace" {
		t.Fatalf("Block = %+v (%v)", b, err)
	}
	if bs, err := r.Blocks(ctx); err != nil || len(bs) != 1 || bs[0].Subject != "u2" {
		t.Fatalf("Blocks = %+v (%v)", bs, err)
	}
	if err := r.Unblock(ctx, "u2"); err != nil {
		t.Fatal(err)
	}
	ds, err := r.Reports(ctx)
	if err != nil || len(ds) != 1 || ds[0].MessageBody != "texto" || ds[0].TargetName != "Grace" {
		t.Fatalf("Reports = %+v (%v)", ds, err)
	}
	if err := r.ResolveReport(ctx, "r1", DenunciaMensajeBorrado); err != nil {
		t.Fatal(err)
	}
	for ruta, quiero := range map[string]string{
		"POST /api/v1/messages/m1/report":            `{"reason":"spam","note":"nota"}`,
		"POST /api/v1/channels/c1/members/u2/report": `{"reason":"harassment","note":""}`,
		"PUT /api/v1/channels/c1/members/u2/block":   ``,
		"DELETE /api/v1/me/blocks/u2":                ``,
		"POST /api/v1/moderation/reports/r1/resolve": `{"action":"message_deleted"}`,
	} {
		if tengo, hay := vistas[ruta]; !hay || tengo != quiero {
			t.Errorf("%s: cuerpo %q (pedida=%v), quiero %q", ruta, tengo, hay, quiero)
		}
	}
}
