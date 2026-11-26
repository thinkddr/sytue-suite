// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package copia

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func poner(t *testing.T, ruta, contenido string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
}

func leer(t *testing.T, ruta string) string {
	t.Helper()
	raw, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func nombres(t *testing.T, ruta string) map[string]bool {
	t.Helper()
	z, err := zip.OpenReader(ruta)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	out := map[string]bool{}
	for _, f := range z.File {
		out[f.Name] = true
	}
	return out
}

// La copia lleva datos y ajustes, nunca el llavero de reserva ni las copias anteriores,
// y la base abierta sale de su copia en caliente (sin su diario).
func TestCrearSinSecretos(t *testing.T) {
	datos, ajustes := t.TempDir(), t.TempDir()
	poner(t, filepath.Join(datos, "Datos", "notes", "registros.jsonl"), `{"a":1}`)
	poner(t, filepath.Join(datos, "Correo", "indice.sqlite"), "A MEDIAS")
	poner(t, filepath.Join(datos, "Correo", "indice.sqlite-wal"), "DIARIO")
	poner(t, filepath.Join(datos, "Copias", "vieja.zip"), "x")
	poner(t, filepath.Join(ajustes, "perfil.json"), `{"nombre":"Ana"}`)
	poner(t, filepath.Join(ajustes, "secretos", "ia-clave"), "sk-secreta")
	c := Nueva(datos, ajustes, "notes")
	quitar := c.BaseAbierta(filepath.Join(datos, "Correo", "indice.sqlite"), func(_ context.Context, destino string) error {
		return os.WriteFile(destino, []byte("COPIA EN CALIENTE"), 0o600)
	})
	defer quitar()
	ruta, err := c.Crear(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(ruta) != filepath.Join(datos, "Copias") || !strings.HasPrefix(filepath.Base(ruta), "sytue-office-") {
		t.Fatalf("ruta de la copia: %s", ruta)
	}
	n := nombres(t, ruta)
	for _, quiero := range []string{"manifiesto.json", "datos/Datos/notes/registros.jsonl", "datos/Correo/indice.sqlite", "ajustes/perfil.json"} {
		if !n[quiero] {
			t.Errorf("falta %s en %v", quiero, n)
		}
	}
	for nombre := range n {
		if strings.Contains(nombre, "secretos") || strings.Contains(nombre, "Copias") || strings.HasSuffix(nombre, "-wal") {
			t.Errorf("no debía ir: %s", nombre)
		}
	}
	z, _ := zip.OpenReader(ruta)
	defer z.Close()
	for _, f := range z.File {
		if f.Name != "datos/Correo/indice.sqlite" {
			continue
		}
		r, _ := f.Open()
		buf := make([]byte, 64)
		k, _ := r.Read(buf)
		_ = r.Close()
		if string(buf[:k]) != "COPIA EN CALIENTE" {
			t.Errorf("la base abierta no salió de su copia en caliente: %q", buf[:k])
		}
	}
	m, err := Comprobar(ruta)
	if err != nil || m.Formato != Formato || !m.SinSecretos || m.Producto != "notes" {
		t.Fatalf("manifiesto: %+v, %v", m, err)
	}
	lista, err := c.Lista()
	if err != nil || len(lista) != 2 {
		t.Fatalf("lista: %+v, %v", lista, err)
	}
}

// Restaurar deja lo de la copia, guarda antes lo que había y no toca el llavero.
func TestRestaurarConCopiaPrevia(t *testing.T) {
	datos, ajustes := t.TempDir(), t.TempDir()
	poner(t, filepath.Join(datos, "Datos", "notes", "registros.jsonl"), "ANTES")
	poner(t, filepath.Join(ajustes, "perfil.json"), "PERFIL ANTES")
	poner(t, filepath.Join(ajustes, "secretos", "ia-clave"), "sk-secreta")
	c := Nueva(datos, ajustes, "notes")
	reloj := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	c.ahora = func() time.Time { return reloj }
	copiaRuta, err := c.Crear(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Después de la copia, la persona cambia cosas.
	poner(t, filepath.Join(datos, "Datos", "notes", "registros.jsonl"), "DESPUÉS")
	poner(t, filepath.Join(datos, "Datos", "nuevo.txt"), "nuevo")
	poner(t, filepath.Join(ajustes, "perfil.json"), "PERFIL DESPUÉS")
	reloj = reloj.Add(time.Hour)

	previa, err := c.Restaurar(context.Background(), copiaRuta)
	if err != nil {
		t.Fatal(err)
	}
	if got := leer(t, filepath.Join(datos, "Datos", "notes", "registros.jsonl")); got != "ANTES" {
		t.Errorf("datos restaurados: %q", got)
	}
	if _, err := os.Stat(filepath.Join(datos, "Datos", "nuevo.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("quedó lo que no estaba en la copia")
	}
	if got := leer(t, filepath.Join(ajustes, "perfil.json")); got != "PERFIL ANTES" {
		t.Errorf("ajustes restaurados: %q", got)
	}
	if got := leer(t, filepath.Join(ajustes, "secretos", "ia-clave")); got != "sk-secreta" {
		t.Errorf("el llavero cambió: %q", got)
	}
	if !nombres(t, previa)["datos/Datos/nuevo.txt"] {
		t.Error("la copia previa no tiene lo que había antes de restaurar")
	}
	if _, err := os.Stat(copiaRuta); err != nil {
		t.Errorf("la copia restaurada desapareció: %v", err)
	}
}

// Un ZIP que no es una copia, o que intenta salirse de su carpeta, no se restaura.
func TestRechazaLoQueNoEsUnaCopia(t *testing.T) {
	datos, ajustes := t.TempDir(), t.TempDir()
	c := Nueva(datos, ajustes, "notes")
	for nombre, ficheros := range map[string][]string{
		"sin-manifiesto": {"datos/x.txt"},
		"fuera":          {"manifiesto.json", "datos/../../fuera.txt"},
		"secretos":       {"manifiesto.json", "ajustes/secretos/clave"},
		"absoluta":       {"manifiesto.json", "/etc/passwd"},
	} {
		ruta := filepath.Join(t.TempDir(), nombre+".zip")
		f, _ := os.Create(ruta)
		z := zip.NewWriter(f)
		for _, n := range ficheros {
			w, _ := z.Create(n)
			if n == "manifiesto.json" {
				_, _ = w.Write([]byte(`{"formato":1}`))
			} else {
				_, _ = w.Write([]byte("x"))
			}
		}
		_ = z.Close()
		_ = f.Close()
		if _, err := c.Restaurar(context.Background(), ruta); !errors.Is(err, ErrNoEsCopia) {
			t.Errorf("%s: %v; quiero ErrNoEsCopia", nombre, err)
		}
	}
	if es, _ := os.ReadDir(filepath.Join(datos, "Copias")); len(es) != 0 {
		t.Errorf("un ZIP rechazado dejó copias previas: %v", es)
	}
}
