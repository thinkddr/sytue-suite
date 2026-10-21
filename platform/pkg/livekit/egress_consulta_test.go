// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package livekit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListEgressConsultaElResultadoFinalPorIdentificador(t *testing.T) {
	c, capt := servidorTwirp(t, `{"items":[{"egress_id":"EG_1","room_name":"cast-acme-1","status":"EGRESS_COMPLETE",`+
		`"segment_results":[{"playlist_name":"tenants/acme/v1/live/index.m3u8","duration":"125000000000","size":"48000000","segment_count":"32"}]}]}`)
	items, err := c.ListEgress(context.Background(), "EG_1")
	if err != nil {
		t.Fatal(err)
	}
	if capt.Ruta != "/twirp/livekit.Egress/ListEgress" || capt.Cuerpo["egress_id"] != "EG_1" || !capt.Grant.RoomRecord {
		t.Errorf("consulta equivocada: %s %+v %+v", capt.Ruta, capt.Cuerpo, capt.Grant)
	}
	if _, limitaActivos := capt.Cuerpo["active"]; limitaActivos {
		t.Error("la consulta excluye las grabaciones que ya acabaron")
	}
	if len(items) != 1 || items[0].EgressID != "EG_1" || items[0].Status != "EGRESS_COMPLETE" || len(items[0].SegmentResults) != 1 {
		t.Fatalf("resultado final: %+v", items)
	}
	seg := items[0].SegmentResults[0]
	if seg.PlaylistName != "tenants/acme/v1/live/index.m3u8" || seg.Duration != 125000000000 || seg.Size != 48000000 || seg.SegmentCount != 32 {
		t.Errorf("segmentos de Twirp mal leídos: %+v", seg)
	}
	if _, err := c.ListEgress(context.Background(), " "); err == nil {
		t.Error("sin id se descargaría el historial global")
	}
}

func TestElErrorTwirpDistingueRecursoCaducadoDeRutaInvalida(t *testing.T) {
	for _, code := range []string{"not_found", "bad_route"} {
		t.Run(code, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"code":"` + code + `","msg":"no disponible"}`))
			}))
			defer srv.Close()
			_, err := NewClient(srv.URL, NewSigner("k", "s")).ListEgress(context.Background(), "EG_1")
			var fallo *APIError
			if !errors.As(err, &fallo) || fallo.StatusCode != http.StatusNotFound || fallo.Code != code {
				t.Fatalf("no se conserva el motivo del 404: %v", err)
			}
		})
	}
}
