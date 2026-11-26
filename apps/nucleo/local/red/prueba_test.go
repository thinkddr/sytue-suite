// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package red

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Con PuntualDePrueba, el permiso puntual llega a un servidor `httptest` de este equipo
// con el nombre que se le pida, y lo demás es lo de verdad: el permiso temporal mientras
// dura la petición (y se cierra después), solo https, nada con «Sin conexión». Sin él, el
// mismo servidor no se alcanza.
func TestPuntualDePrueba(t *testing.T) {
	var vistos []Destino
	p := Abrir("")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		vistos = p.Destinos()
		_, _ = io.WriteString(w, "hola")
	}))
	defer srv.Close()
	if _, err := p.Puntual(ImagenRemota, "").Get(srv.URL); !errors.Is(err, ErrDenegado) {
		t.Fatalf("sin la prueba, este equipo: %v", err)
	}
	raices := x509.NewCertPool()
	raices.AddCert(srv.Certificate())
	p.PuntualDePrueba(func(ctx context.Context, red, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, red, srv.Listener.Addr().String())
	}, raices)
	c := p.Puntual(ImagenRemota, "Imagen de una nota")
	res, err := c.Get("https://example.com/foto.png")
	if err != nil {
		t.Fatal(err)
	}
	cuerpo, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if string(cuerpo) != "hola" || len(vistos) != 1 || vistos[0].Host != "example.com" || !vistos[0].Temporal {
		t.Fatalf("por el permiso puntual: %q %+v", cuerpo, vistos)
	}
	if ds := p.Destinos(); len(ds) != 0 {
		t.Fatalf("acabada, sigue abierto: %+v", ds)
	}
	if _, err := c.Get("http://example.com/foto.png"); !errors.Is(err, ErrDenegado) {
		t.Fatalf("http: %v", err)
	}
	if err := p.PonerSinConexion(true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get("https://example.com/foto.png"); !errors.Is(err, ErrDenegado) {
		t.Fatalf("sin conexión: %v", err)
	}
}

// TestPuntualDePruebaSoloEnPruebas: PuntualDePrueba abre el permiso puntual a cualquier
// sitio; fuera de un `_test.go` no lo llama nadie.
//
// VISTO EN ROJO (2026-11-25): con una llamada en `local/red/red.go` (fuera de su
// definición), lo nombró con su fichero.
func TestPuntualDePruebaSoloEnPruebas(t *testing.T) {
	raiz := filepath.Join("..", "..", "..", "..")
	for _, dir := range []string{"apps/nucleo", "apps/fyne", "services", "platform"} {
		err := filepath.WalkDir(filepath.Join(raiz, dir), func(ruta string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil //nolint:nilerr // una carpeta que falta (una copia parcial) o no se deja leer no llama a nadie
			}
			if d.IsDir() {
				if n := d.Name(); n == "node_modules" || n == "testdata" || strings.HasPrefix(n, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
				return nil
			}
			datos, err := os.ReadFile(ruta) //nolint:gosec // el propio repositorio
			if err != nil {
				return nil //nolint:nilerr // ídem: lo que no se lee no llama a nadie
			}
			usos := bytes.Count(datos, []byte("PuntualDePrueba("))
			if filepath.ToSlash(ruta) == filepath.ToSlash(filepath.Join(raiz, "apps", "nucleo", "local", "red", "red.go")) {
				usos -= bytes.Count(datos, []byte(") PuntualDePrueba("))
			}
			if usos > 0 {
				t.Errorf("%s llama a PuntualDePrueba fuera de una prueba: abre el permiso puntual a cualquier sitio", ruta)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
