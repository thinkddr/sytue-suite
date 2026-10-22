// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Sytue ID estuvo sin icono y nadie lo notó: no hay pantalla más vista en toda
// la suite —entrar en cualquier app sin sesión termina en su login— y salía con
// el cuadro en blanco del navegador. Lo que sigue comprueba las DOS mitades,
// porque cada una falla sola: que la marca se sirva, y que las páginas la
// declaren. Declararla sin servirla da un icono roto; servirla sin declararla
// deja al navegador pidiendo `/favicon.ico` a ciegas.

func TestFavicon_sirveLaMarcaComoSVG(t *testing.T) {
	rec := httptest.NewRecorder()
	handleFavicon(rec, httptest.NewRequest(http.MethodGet, "/favicon.svg", http.NoBody))

	if got := rec.Code; got != 200 {
		t.Fatalf("estado %d, se esperaba 200", got)
	}
	// EL TIPO ES LO QUE DECIDE qué pinta el navegador, no la extensión de la URL.
	if got := rec.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Errorf("Content-Type %q, se esperaba image/svg+xml", got)
	}
	cuerpo := rec.Body.String()
	if !strings.Contains(cuerpo, "<svg") {
		t.Errorf("el cuerpo no parece un SVG: %.60q", cuerpo)
	}
	// Que sea LA MARCA vendorizada y no un dibujo a mano que derive: es
	// exactamente el problema que sync-branding.sh viene a evitar.
	marca, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "branding", "marks", "id.svg"))
	if err != nil {
		t.Fatalf("no se puede leer branding/marks/id.svg: %v", err)
	}
	if cuerpo != string(marca) {
		t.Error("lo servido no es branding/marks/id.svg; ¿se editó marca.svg a mano " +
			"en vez de regenerarla con scripts/sync-branding.sh?")
	}
}

// TestFavicon_sirveUnIcoDeVerdad — antes esto redirigía al SVG, y la
// redirección cubría a casi todo lo que pide `/favicon.ico` sin leer HTML.
// A casi todo menos al único consumidor que EXIGE el formato: Windows no acepta
// un SVG para anclar a la barra de tareas, así que ahí el icono seguía saliendo
// en blanco justo en la pantalla más vista de la suite.
//
// Se comprueba la CABECERA del fichero y no su tamaño: los cuatro primeros bytes
// de un ICO son 00 00 01 00, y el quinto par dice cuántas imágenes trae. Un PNG o
// el propio SVG renombrados se servirían igual y fallarían solo en el consumidor
// que los pedía por ser `.ico`.
func TestFavicon_sirveUnIcoDeVerdad(t *testing.T) {
	rec := httptest.NewRecorder()
	handleFaviconICO(rec, httptest.NewRequest(http.MethodGet, "/favicon.ico", http.NoBody))

	if rec.Code != 200 {
		t.Fatalf("estado %d, se esperaba 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/x-icon" {
		t.Errorf("Content-Type %q, se esperaba image/x-icon", got)
	}
	cuerpo := rec.Body.Bytes()
	if len(cuerpo) < 6 {
		t.Fatalf("cuerpo de %d bytes: no es un ICO", len(cuerpo))
	}
	if cuerpo[0] != 0 || cuerpo[1] != 0 || cuerpo[2] != 1 || cuerpo[3] != 0 {
		t.Errorf("cabecera %v: no es un ICO; ¿se renombró un PNG o el SVG?", cuerpo[:4])
	}
	// Multi-resolución: con un solo tamaño el escalado lo hace el sistema y se
	// nota en la pestaña, que es donde más se mira.
	if n := int(cuerpo[4]) | int(cuerpo[5])<<8; n < 2 {
		t.Errorf("el ICO trae %d imagen(es); se generan cuatro (64…16)", n)
	}
}

// TestFavicon_todaPaginaLoDeclara — la mitad que se pierde sola.
//
// Se lee el disco a propósito: una lista de ficheros escrita aquí sería un segundo
// sitio que mantener, y el que se olvida cuando alguien añade una pantalla nueva. Lo
// que se busca es todo lo que abra un `<head>`; si abre uno, tiene que declarar el icono.
func TestFavicon_todaPaginaLoDeclara(t *testing.T) {
	ficheros, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	vistos := 0
	for _, f := range ficheros {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		texto := string(src)
		// El `<head>` DENTRO de una cadena de Go, no en un comentario: se exige el
		// literal que usan todos los emisores.
		if !strings.Contains(texto, `<head><meta charset="utf-8">`) {
			continue
		}
		vistos++
		if !strings.Contains(texto, `<link rel="icon" href="/favicon.svg"`) {
			t.Errorf("%s emite un <head> y no declara el icono: el navegador se queda "+
				"pidiendo /favicon.ico a ciegas", f)
		}
		if !strings.Contains(texto, `<link rel="apple-touch-icon" href="/apple-touch-icon.png">`) {
			t.Errorf("%s emite un <head> y no declara el apple-touch-icon: Safari e iOS "+
				"no rasterizan el SVG y se quedan sin icono", f)
		}
	}
	// Que no apruebe por no haber mirado nada, que es como esto se rompe de verdad.
	if vistos < 5 {
		t.Errorf("solo se han encontrado %d páginas con <head>; se esperaban al menos 5. "+
			"¿Cambió la forma de componer el HTML?", vistos)
	}
}

// TestFavicon_sirveElPngDeApple — Safari e iOS solo aceptan PNG aquí; se
// comprueba la cabecera (89 50 4E 47) y las medidas del IHDR, no el tamaño en
// bytes: un SVG renombrado se serviría igual y fallaría solo en el iPhone.
func TestFavicon_sirveElPngDeApple(t *testing.T) {
	rec := httptest.NewRecorder()
	handleAppleTouchIcon(rec, httptest.NewRequest(http.MethodGet, "/apple-touch-icon.png", http.NoBody))

	if rec.Code != 200 {
		t.Fatalf("estado %d, se esperaba 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type %q, se esperaba image/png", got)
	}
	b := rec.Body.Bytes()
	if len(b) < 24 || b[0] != 0x89 || b[1] != 'P' || b[2] != 'N' || b[3] != 'G' {
		t.Fatalf("no es un PNG: %v", b[:min(4, len(b))])
	}
	ancho := int(b[16])<<24 | int(b[17])<<16 | int(b[18])<<8 | int(b[19])
	alto := int(b[20])<<24 | int(b[21])<<16 | int(b[22])<<8 | int(b[23])
	if ancho != 180 || alto != 180 {
		t.Errorf("mide %d×%d; iOS espera 180×180", ancho, alto)
	}
}
