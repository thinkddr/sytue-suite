// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// peticionVista es lo que le llegó al servicio de mentira.
type peticionVista struct {
	cuerpo string
	token  string
}

// Las rutas y los cuerpos son los de services/cast/internal/http/moderacion.go, y en
// NINGUNO viaja quién denuncia o quién bloquea: el actor lo pone el token.
func TestCastDenunciarYBloquear(t *testing.T) {
	vistas := map[string]peticionVista{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		clave := r.Method + " " + r.URL.RequestURI()
		vistas[clave] = peticionVista{string(cuerpo), r.Header.Get("Authorization")}
		switch clave {
		case "GET /api/v1/blocks":
			_, _ = io.WriteString(w, `{"blocks":[{"sub":"u2","name":"Grace","created_at":"2026-10-06T10:00:00Z"}]}`)
		case "GET /api/v1/moderation/blocks":
			_, _ = io.WriteString(w, `{"blocks":[]}`)
		case "GET /api/v1/moderation/summary":
			_, _ = io.WriteString(w, `{"open":3}`)
		case "GET /api/v1/moderation/reports?status=open", "GET /api/v1/moderation/reports?status=resolved":
			_, _ = io.WriteString(w, `{"reports":[{"id":"r1","kind":"comment","video_id":"v1","target_id":"k1","target_sub":"u2","target_name":"Grace","excerpt":"texto","reason":"spam","anonymous":true,"status":"open","created_at":"2026-10-06T10:00:00Z","actions":["dismiss","delete","block"]}]}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c := NewCast(srv.URL, nil, nil)
	ctx := context.Background()

	if err := c.Report(ctx, CastNewReport{Kind: CastReportComment, VideoID: "v1", Target: "k1", Reason: "spam", Details: "nota"}); err != nil {
		t.Fatal(err)
	}
	if got := vistas["POST /api/v1/videos/v1/reports"].cuerpo; got != `{"kind":"comment","target":"k1","reason":"spam","details":"nota"}` {
		t.Errorf("cuerpo de la denuncia: %s", got)
	}
	if err := c.Report(ctx, CastNewReport{Kind: CastReportChannel, Org: "acme", Reason: "other"}); err != nil {
		t.Fatal(err)
	}
	if got := vistas["POST /api/v1/channels/acme/reports"].cuerpo; got != `{"kind":"channel","reason":"other"}` {
		t.Errorf("cuerpo de la denuncia del canal: %s", got)
	}
	if err := c.Block(ctx, "v1", "u2"); err != nil {
		t.Fatal(err)
	}
	if got := vistas["POST /api/v1/videos/v1/blocks"].cuerpo; got != `{"sub":"u2"}` {
		t.Errorf("cuerpo del bloqueo: %s", got)
	}
	if bs, err := c.Blocks(ctx); err != nil || len(bs) != 1 || bs[0].Sub != "u2" || bs[0].Name != "Grace" {
		t.Fatalf("Blocks = %+v (%v)", bs, err)
	}
	// Un sub no es de fiar como trozo de ruta: va escapado, y una «/» no abre otra.
	if err := c.Unblock(ctx, "ext/u2"); err != nil {
		t.Fatal(err)
	}
	if _, ok := vistas["DELETE /api/v1/blocks/ext%2Fu2"]; !ok {
		t.Errorf("Unblock no escapó el sub: %v", vistas)
	}
	if err := c.UnblockOrg(ctx, "u2"); err != nil {
		t.Fatal(err)
	}
	if _, ok := vistas["DELETE /api/v1/moderation/blocks/u2"]; !ok {
		t.Errorf("UnblockOrg fue a otra ruta: %v", vistas)
	}
	if bs, err := c.OrgBlocks(ctx); err != nil || len(bs) != 0 {
		t.Fatalf("OrgBlocks = %+v (%v)", bs, err)
	}
	if n, err := c.OpenReports(ctx); err != nil || n != 3 {
		t.Fatalf("OpenReports = %d (%v)", n, err)
	}
	for _, resueltas := range []bool{false, true} {
		ds, err := c.Reports(ctx, resueltas)
		if err != nil || len(ds) != 1 || !ds[0].Anonymous || ds[0].Excerpt != "texto" || len(ds[0].Actions) != 3 {
			t.Fatalf("Reports(%v) = %+v (%v)", resueltas, ds, err)
		}
	}
	if err := c.ResolveReport(ctx, "r1", CastActionDelete); err != nil {
		t.Fatal(err)
	}
	if got := vistas["POST /api/v1/moderation/reports/r1"].cuerpo; got != `{"action":"delete"}` {
		t.Errorf("cuerpo de resolver: %s", got)
	}
}

// Sin sesión la denuncia va por la puerta pública: pide el reto, lo resuelve y lo
// adjunta. La solución es la que comprueba `platform/pkg/altcha`: el número cuyo
// SHA-256 con la sal es el reto, en una carga base64 con la firma tal cual.
func TestCastDenunciaSinSesionResuelveLaPruebaDeTrabajo(t *testing.T) {
	const sal, numero = "sal?expires=9999999999&", 4321
	h := sha256.Sum256([]byte(sal + "4321"))
	reto := `{"algorithm":"SHA-256","challenge":"` + hex.EncodeToString(h[:]) + `","maxnumber":100000,"salt":"` + sal + `","signature":"firma"}`
	var enviado struct {
		Kind, Reason, Altcha string
	}
	ruta, token := "", "sin mirar"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/public/reports/altcha" {
			_, _ = io.WriteString(w, reto)
			return
		}
		ruta, token = r.Method+" "+r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&enviado)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewCast(srv.URL, nil, nil)
	if err := c.PublicReport(context.Background(), CastNewReport{Kind: CastReportVideo, VideoID: "v1", Reason: "violence"}); err != nil {
		t.Fatal(err)
	}
	if ruta != "POST /api/v1/public/videos/v1/reports" || token != "" {
		t.Fatalf("fue a %q con token %q", ruta, token)
	}
	if enviado.Kind != "video" || enviado.Reason != "violence" {
		t.Fatalf("cuerpo: %+v", enviado)
	}
	carga, err := base64.StdEncoding.DecodeString(enviado.Altcha)
	if err != nil {
		t.Fatal(err)
	}
	var solucion struct {
		Algorithm, Challenge, Salt, Signature string
		Number                                int64
	}
	if err := json.Unmarshal(carga, &solucion); err != nil {
		t.Fatal(err)
	}
	if solucion.Number != numero || solucion.Salt != sal || solucion.Signature != "firma" || solucion.Algorithm != "SHA-256" ||
		!strings.EqualFold(solucion.Challenge, hex.EncodeToString(h[:])) {
		t.Fatalf("solución: %+v", solucion)
	}

	// Un reto sin solución no manda nada.
	reto = `{"algorithm":"SHA-256","challenge":"00","maxnumber":50,"salt":"x","signature":"f"}`
	ruta = ""
	if err := c.PublicReport(context.Background(), CastNewReport{Kind: CastReportVideo, VideoID: "v1", Reason: "spam"}); err == nil || ruta != "" {
		t.Fatalf("con un reto sin solución: err=%v, ruta=%q", err, ruta)
	}
}
