// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package logoorg

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func pngDe(t *testing.T, ancho, alto int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	for i := range ancho * alto {
		img.Set(i%ancho, i/ancho, c)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// idFalso imita el endpoint público de Sytue ID con un logo por organización.
type idFalso struct {
	mu       sync.Mutex
	logos    map[string][]byte // slug → cuerpo
	tipos    map[string]string // slug → Content-Type (por defecto image/png)
	pedidos  []string
	conMatch int
}

func (f *idFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pedidos = append(f.pedidos, r.URL.Path)
	slug := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/branding/"), "/logo.png")
	cuerpo, ok := f.logos[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}
	etag := `"v-` + slug + `"`
	if r.Header.Get("If-None-Match") == etag {
		f.conMatch++
		w.WriteHeader(http.StatusNotModified)
		return
	}
	tipo := f.tipos[slug]
	if tipo == "" {
		tipo = "image/png"
	}
	w.Header().Set("Content-Type", tipo)
	w.Header().Set("ETag", etag)
	_, _ = w.Write(cuerpo)
}

func montar(t *testing.T, f *idFalso) *Cliente {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return Nuevo(srv.URL)
}

// Cada organización recibe SU logo y solo el suyo: el slug decide la petición y la caché.
func TestCadaOrganizacionSuLogo(t *testing.T) {
	f := &idFalso{logos: map[string][]byte{
		"acme":  pngDe(t, 40, 10, color.RGBA{R: 200, A: 255}),
		"otra":  pngDe(t, 20, 20, color.RGBA{B: 200, A: 255}),
		"vacia": nil,
	}}
	delete(f.logos, "vacia")
	c := montar(t, f)
	ctx := context.Background()
	acme, err := c.Logo(ctx, "acme")
	if err != nil || acme.Ancho != 40 || acme.Alto != 10 || len(acme.Hash) != 16 {
		t.Fatalf("acme: %v %+v", err, acme)
	}
	otra, err := c.Logo(ctx, "otra")
	if err != nil || otra.Hash == acme.Hash || bytes.Equal(otra.PNG, acme.PNG) {
		t.Fatalf("otra organización recibió el logo de acme: %v", err)
	}
	if _, err := c.Logo(ctx, "vacia"); !errors.Is(err, ErrSinLogo) {
		t.Errorf("organización sin logo: %v", err)
	}
	// Segunda vuelta: todo de caché, y cada cual lo suyo.
	antes := len(f.pedidos)
	otraVez, _ := c.Logo(ctx, "acme")
	if _, err := c.Logo(ctx, "vacia"); !errors.Is(err, ErrSinLogo) || otraVez.Hash != acme.Hash || len(f.pedidos) != antes {
		t.Errorf("la caché no sirvió (o cruzó organizaciones): %d peticiones nuevas", len(f.pedidos)-antes)
	}
	for _, p := range f.pedidos {
		if p != "/branding/acme/logo.png" && p != "/branding/otra/logo.png" && p != "/branding/vacia/logo.png" {
			t.Errorf("petición inesperada a ID: %s", p)
		}
	}
}

// Un slug que no es un slug nunca llega a ID: ni rutas, ni otros hosts, ni consultas.
func TestSlugHostilNoSalePorLaRed(t *testing.T) {
	f := &idFalso{logos: map[string][]byte{}}
	c := montar(t, f)
	for _, slug := range []string{"", "../admin", "acme/../../api/v1/admin", "acme?x=1", "acme#", "ACME", "a b", "evil.example", strings.Repeat("a", 64), "-acme", "acme-", "%2e%2e"} {
		if _, err := c.Logo(context.Background(), slug); !errors.Is(err, ErrSinLogo) {
			t.Errorf("slug %q: %v", slug, err)
		}
	}
	if len(f.pedidos) != 0 {
		t.Errorf("un slug inválido llegó a ID: %v", f.pedidos)
	}
}

// Lo que no es un PNG decodificable no se incrusta, diga lo que diga su cabecera.
func TestSoloEntraUnPNG(t *testing.T) {
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><script>alert(2)</script><image href="https://example.invalid/x"/></svg>`
	grande := image.NewGray(image.Rect(0, 0, 4097, 4))
	var enorme bytes.Buffer
	_ = png.Encode(&enorme, grande)
	f := &idFalso{
		logos: map[string][]byte{
			"svg-honesto":  []byte(svg),
			"svg-disfraz":  []byte(svg),
			"html":         []byte("<html><script>alert(1)</script></html>"),
			"png-con-cola": append(pngDe(t, 8, 8, color.White), []byte(`<script>alert(1)</script>`)...),
			"png-enorme":   enorme.Bytes(),
			"png-truncado": pngDe(t, 64, 64, color.Black)[:60],
			"muy-pesado":   bytes.Repeat([]byte{0x89}, maxBytes+10),
			"otro-tipo":    pngDe(t, 8, 8, color.White),
		},
		tipos: map[string]string{"svg-honesto": "image/svg+xml", "otro-tipo": "text/html"},
	}
	c := montar(t, f)
	for _, slug := range []string{"svg-honesto", "svg-disfraz", "html", "png-enorme", "png-truncado", "muy-pesado", "otro-tipo"} {
		if l, err := c.Logo(context.Background(), slug); !errors.Is(err, ErrNoValido) || l != nil {
			t.Errorf("%s: se aceptó (%v)", slug, err)
		}
	}
	// Un PNG válido con basura detrás se acepta, pero la basura no sobrevive a recodificar.
	l, err := c.Logo(context.Background(), "png-con-cola")
	if err != nil || bytes.Contains(l.PNG, []byte("script")) {
		t.Errorf("la cola del PNG sobrevivió: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(l.PNG)); err != nil {
		t.Errorf("lo recodificado no es un PNG: %v", err)
	}
}

// No se siguen redirecciones (la respuesta es de la base configurada o de nadie) y, pasada
// la vida de la caché, se revalida con el ETag en vez de volver a bajar el logo.
func TestRedireccionesYRevalidacion(t *testing.T) {
	fuera := 0
	destino := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fuera++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngDe(t, 4, 4, color.White))
	}))
	defer destino.Close()
	redirige := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destino.URL+"/x.png", http.StatusFound)
	}))
	defer redirige.Close()
	if l, err := Nuevo(redirige.URL).Logo(context.Background(), "acme"); err == nil || l != nil || fuera != 0 {
		t.Errorf("se siguió una redirección: %v (peticiones fuera: %d)", err, fuera)
	}

	f := &idFalso{logos: map[string][]byte{"acme": pngDe(t, 10, 10, color.White)}}
	c := montar(t, f)
	reloj := time.Now()
	c.ahora = func() time.Time { return reloj }
	primero, err := c.Logo(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	reloj = reloj.Add(vidaCache + time.Second)
	segundo, err := c.Logo(context.Background(), "acme")
	if err != nil || segundo.Hash != primero.Hash || f.conMatch != 1 {
		t.Errorf("no revalidó con el ETag: %v (304: %d)", err, f.conMatch)
	}
	// ID cambia el logo: la versión cambia.
	f.mu.Lock()
	f.logos["acme"] = pngDe(t, 12, 12, color.Black)
	f.mu.Unlock()
	c2 := montar(t, f)
	if nuevo, err := c2.Logo(context.Background(), "acme"); err != nil || nuevo.Hash == primero.Hash {
		t.Errorf("un logo distinto debe dar otra versión: %v", err)
	}
	// Si ID se cae, se sigue sirviendo el último logo bueno.
	caido := Nuevo("http://127.0.0.1:1")
	caido.cache["acme"] = entrada{logo: primero, etag: `"v-acme"`}
	if l, err := caido.Logo(context.Background(), "acme"); err != nil || l.Hash != primero.Hash {
		t.Errorf("con ID caído debe servirse la copia: %v", err)
	}
}
