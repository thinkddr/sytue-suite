// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	docsdb "git.sytue.eu/sytue/one/services/docs/db"
	"git.sytue.eu/sytue/one/services/docs/internal/collab"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/logoorg"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// El logotipo de la organización contra PostgreSQL real y un Sytue ID de mentira: va con la
// marca al documento y a la portada de la presentación; cada organización recibe el SUYO
// (el slug sale de la sesión, nunca de la petición); y lo que ID sirva que no sea un PNG
// —un SVG con guiones, por ejemplo— no entra en ningún documento.
func TestLogoDeLaOrganizacionConLaMarca(t *testing.T) {
	pintar := func(c color.Color) []byte {
		img := image.NewRGBA(image.Rect(0, 0, 80, 20))
		for i := range 80 * 20 {
			img.Set(i%80, i/80, c)
		}
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	const svgHostil = `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><script>alert(2)</script></svg>`
	var mu sync.Mutex
	pedidos := []string{}
	id := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		pedidos = append(pedidos, r.URL.Path)
		mu.Unlock()
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Errorf("la petición a ID lleva credenciales: %v", r.Header)
		}
		w.Header().Set("Content-Type", "image/png")
		switch r.URL.Path {
		case "/branding/acme/logo.png":
			_, _ = w.Write(pintar(color.RGBA{R: 255, A: 255}))
		case "/branding/otra/logo.png":
			_, _ = w.Write(pintar(color.RGBA{B: 255, A: 255}))
		case "/branding/hostil/logo.png":
			_, _ = w.Write([]byte(svgHostil)) // miente en la cabecera
		default:
			http.NotFound(w, r)
		}
	}))
	defer id.Close()

	pg := pgtest.New(t, docsdb.Migrations, docsdb.MigrationsDir)
	st := store.New(pg.Pool)
	hd := &Handler{
		svc: domain.NewService(st), store: st, hub: collab.NewHub(nil),
		presence: collab.NewPresenceRegistry(), inline: 256 * 1024, marcaOrg: st,
		logos: logoorg.Nuevo(id.URL),
	}
	pedir := func(tenant, metodo, sub string, roles []string, params map[string]string, cuerpo string, f http.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		r := req(t, metodo, "/x", &authn.Claims{Subject: sub, Tenant: tenant, Roles: roles}, params, cuerpo)
		r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: tenant}))
		rec := httptest.NewRecorder()
		f(rec, r)
		return rec
	}
	crear := func(tenant, cuerpo string) string {
		t.Helper()
		rec := pedir(tenant, "POST", "marta", nil, nil, cuerpo, hd.createDocument)
		var r struct{ Document domain.Document }
		if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &r) != nil {
			t.Fatalf("crear en %s: %d %s", tenant, rec.Code, rec.Body.String())
		}
		ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: tenant})
		est, _, err := hd.svc.LoadFull(ctx, domain.Subject{Sub: r.Document.OwnerSub}, r.Document.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		var s string
		_ = json.Unmarshal(est.Materialize().Registers[domain.ContentKey(r.Document.Kind)], &s)
		return s
	}
	for _, tenant := range []string{"acme", "otra", "hostil", "sinlogo"} {
		cuerpo := `{"name":"Marca de ` + tenant + `","bg":"#FFFFFF","ink":"#111111","accent":"#B45309"}`
		if rec := pedir(tenant, "PUT", "ada", []string{"admin"}, nil, cuerpo, hd.guardarMarcaOrg); rec.Code != http.StatusCreated {
			t.Fatalf("marca de %s: %d %s", tenant, rec.Code, rec.Body.String())
		}
	}
	// Lo incrustado es el PNG recodificado por el servicio: se compara por sus píxeles.
	colorDe := func(contenido string) color.Color {
		t.Helper()
		_, resto, ok := strings.Cut(contenido, "data:image/png;base64,")
		if !ok {
			return nil
		}
		b64, _, _ := strings.Cut(resto, `"`)
		crudo, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			t.Fatalf("base64 del logo: %v", err)
		}
		img, err := png.Decode(bytes.NewReader(crudo))
		if err != nil {
			t.Fatalf("lo incrustado no es un PNG: %v", err)
		}
		return img.At(1, 1)
	}
	esRojo := func(c color.Color) bool { r, _, b, _ := c.RGBA(); return r > 0xF000 && b < 0x1000 }
	esAzul := func(c color.Color) bool { r, _, b, _ := c.RGBA(); return b > 0xF000 && r < 0x1000 }

	// --- Documento y presentación de acme llevan el logo de acme ---
	doc := crear("acme", `{"template":"carta","locale":"es","theme":"marca"}`)
	if c := colorDe(doc); c == nil || !esRojo(c) || !strings.HasPrefix(doc, `<p data-logo-marca="`) || !strings.Contains(doc, `alt="Marca de acme"`) {
		t.Fatalf("la carta de acme no lleva su logo en la cabecera: %.200s", doc)
	}
	pres := crear("acme", `{"template":"proyecto","locale":"es","theme":"marca"}`)
	if c := colorDe(pres); c == nil || !esRojo(c) || strings.Count(pres, `"logo-marca"`) != 1 {
		t.Errorf("la portada de la presentación de acme no lleva su logo: %.200s", pres)
	}
	// La hoja no tiene imágenes en su modelo: marca sin logo.
	if hoja := crear("acme", `{"template":"presupuesto","locale":"es","theme":"marca"}`); strings.Contains(hoja, "data:image") || !strings.Contains(hoja, "#B45309") {
		t.Errorf("la hoja lleva los colores de la marca y ningún logo: %.200s", hoja)
	}
	// Sin la marca (otro tema o ninguno) no hay logo.
	if d := crear("acme", `{"template":"carta","locale":"es","theme":"carbon"}`); strings.Contains(d, "data:image") {
		t.Error("un tema del catálogo incrustó el logo")
	}

	// --- Aislamiento: cada organización, el suyo ---
	if c := colorDe(crear("otra", `{"template":"carta","locale":"es","theme":"marca"}`)); c == nil || !esAzul(c) {
		t.Errorf("«otra» debe recibir SU logo (azul), no el de acme: %v", c)
	}
	// Tras servir a «otra», acme sigue recibiendo el suyo (la caché va por organización).
	if c := colorDe(crear("acme", `{"template":"carta","locale":"es","theme":"marca"}`)); c == nil || !esRojo(c) {
		t.Errorf("acme recibió un logo que no es el suyo: %v", c)
	}

	// --- Un «logo» que no es un PNG no entra; la marca se aplica igualmente ---
	if d := crear("hostil", `{"template":"informe","locale":"es","theme":"marca"}`); strings.Contains(d, "svg") || strings.Contains(d, "script") ||
		strings.Contains(d, "data:image") || strings.Contains(d, "data-logo-marca") || !strings.Contains(d, `data-tema="marca:`) {
		t.Errorf("el SVG hostil llegó al documento (o se perdió la marca): %.300s", d)
	}
	if d := crear("sinlogo", `{"template":"informe","locale":"es","theme":"marca"}`); strings.Contains(d, "data-logo-marca") || !strings.Contains(d, `data-tema="marca:`) {
		t.Errorf("una organización sin logo: marca sí, logo no: %.200s", d)
	}

	// ID solo recibió peticiones del logo de la organización de cada sesión.
	mu.Lock()
	defer mu.Unlock()
	for _, p := range pedidos {
		switch p {
		case "/branding/acme/logo.png", "/branding/otra/logo.png", "/branding/hostil/logo.png", "/branding/sinlogo/logo.png":
		default:
			t.Errorf("petición inesperada a Sytue ID: %s", p)
		}
	}
	if len(pedidos) == 0 {
		t.Error("no se pidió ningún logo: el test no ha probado nada")
	}
}
