// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestSoltarUnaCarpetaSubeSuArbol: la web recrea el árbol de una carpeta soltada dentro
// de Drive. Las subcarpetas se crean de las someras a las profundas, una que ya existe
// (409) se reutiliza, y cada fichero cae en la suya.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con `AsegurarCarpeta` sin el camino del
// 409, «Proyecto» ya existía y sus ficheros no se subían.
func TestSoltarUnaCarpetaSubeSuArbol(t *testing.T) {
	raiz := t.TempDir()
	proyecto := filepath.Join(raiz, "Proyecto")
	if err := os.MkdirAll(filepath.Join(proyecto, "planos"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{filepath.Join(proyecto, "acta.txt"), filepath.Join(proyecto, "planos", "a.dwg")} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	entradas := EntradasDe([]string{proyecto})
	if len(entradas) != 2 {
		t.Fatalf("entradas %+v", entradas)
	}
	if got := Subcarpetas(entradas); !slices.Equal(got, []string{"Proyecto", "Proyecto/planos"}) {
		t.Fatalf("subcarpetas %q", got)
	}

	var mu sync.Mutex
	subidas := map[string]string{} // nombre → carpeta
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/folders":
			var c struct {
				Padre  string `json:"parent_id"`
				Nombre string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&c)
			if c.Nombre == "Proyecto" {
				w.WriteHeader(http.StatusConflict) // ya existía
				return
			}
			_, _ = io.WriteString(w, `{"id":"id-`+c.Nombre+`","parent_id":"`+c.Padre+`","name":"`+c.Nombre+`"}`)
		case r.URL.Path == "/api/v1/folders/raiz/children":
			_, _ = io.WriteString(w, `{"folders":[{"id":"id-Proyecto","name":"Proyecto"}],"files":[]}`)
		case r.Method == http.MethodPost:
			mu.Lock()
			subidas[r.URL.Query().Get("name")] = filepath.Base(filepath.Dir(r.URL.Path))
			mu.Unlock()
			_, _ = io.WriteString(w, `{"id":"f"}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()
	n, err := SubirArbol(context.Background(), api.NewDrive(srv.URL, sesionDoble{}, nil), "raiz", entradas, NuevoProgreso(2))
	if err != nil || n != 2 {
		t.Fatalf("subidos %d, %v", n, err)
	}
	if subidas["acta.txt"] != "id-Proyecto" || subidas["a.dwg"] != "id-planos" {
		t.Errorf("los ficheros caen en %v", subidas)
	}
}

// cierre apunta que el flujo se ha cerrado.
type cierre struct {
	io.Reader
	cerrado *int
}

func (c cierre) Close() error { *c.cerrado++; return nil }

// TestSubirUnFlujoDelMovil: lo que da el selector de Android (`content://`) sube en flujo,
// sin copiarlo a ningún sitio: con su tamaño si se sabe —y así la barra va en porcentaje—
// o troceado si no, con el nombre que dice el proveedor, entero, y cerrado al acabar.
//
// VISTO EN ROJO (2026-09-24, en el frontal): pasando siempre -1 como tamaño, el fichero
// de tamaño conocido llega troceado y la barra no tendría con qué avanzar.
func TestSubirUnFlujoDelMovil(t *testing.T) {
	var mu sync.Mutex
	llegado := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		llegado[r.URL.Query().Get("name")] = fmt.Sprintf("%d %s %s", r.ContentLength, r.Header.Get("Content-Type"), b)
		mu.Unlock()
		_, _ = io.WriteString(w, `{"id":"f"}`)
	}))
	defer srv.Close()
	cerrados := 0
	entradas := []Entrada{
		Fluido("foto.jpg", 5, cierre{strings.NewReader("hola "), &cerrados}),
		Fluido("notas.txt", -1, cierre{io.MultiReader(strings.NewReader("mun"), strings.NewReader("do")), &cerrados}),
	}
	n, err := SubirArbol(context.Background(), api.NewDrive(srv.URL, sesionDoble{}, nil), "c1", entradas, NuevoProgreso(2))
	if err != nil || n != 2 {
		t.Fatalf("subidos %d, %v", n, err)
	}
	quiero := map[string]string{"foto.jpg": "5 image/jpeg hola ", "notas.txt": "-1 text/plain; charset=utf-8 mundo"}
	for k, v := range quiero {
		if llegado[k] != v {
			t.Errorf("%s llega como %q y se esperaba %q", k, llegado[k], v)
		}
	}
	if cerrados != 2 {
		t.Errorf("se han cerrado %d flujos de 2", cerrados)
	}
}

// TestSoltarUnaCarpetaNoSigueLosEnlaces (FYS-01): un enlace simbólico dentro de la
// carpeta soltada no se sube; con él, «notas.txt» subía la clave privada a la que
// apuntaba.
func TestSoltarUnaCarpetaNoSigueLosEnlaces(t *testing.T) {
	casa := t.TempDir()
	secreto := filepath.Join(casa, "id_ed25519")
	if err := os.WriteFile(secreto, []byte("CLAVE"), 0o600); err != nil {
		t.Fatal(err)
	}
	proyecto := filepath.Join(casa, "proyecto")
	if err := os.MkdirAll(proyecto, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proyecto, "leeme.txt"), []byte("hola"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../id_ed25519", filepath.Join(proyecto, "notas.txt")); err != nil {
		t.Skip("este sistema no deja crear enlaces:", err)
	}
	for _, e := range EntradasDe([]string{proyecto}) {
		if filepath.Base(e.Ruta) == "notas.txt" {
			t.Fatal("el enlace simbólico entra en la subida")
		}
	}
}
