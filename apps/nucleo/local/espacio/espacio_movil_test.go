// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package espacio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// En el móvil con App Group, los ajustes son los mismos para todas las apps: la primera
// que llega lleva los suyos y la segunda no los pisa; los datos siguen siendo de cada una.
func TestLosAjustesDelMovilSeCompartenPorElGrupo(t *testing.T) {
	t.Setenv("SYTUE_OFFICE_DATOS", "")
	grupo := t.TempDir()
	docus, notes := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(docus, "ajustes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docus, "ajustes", "perfil.json"), []byte(`{"nombre":"Ana"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	e1, err := AbrirMovil(docus, grupo)
	if err != nil {
		t.Fatal(err)
	}
	if e1.Rutas.Ajustes != filepath.Join(grupo, "ajustes") || e1.Rutas.Datos != filepath.Join(docus, "datos") || e1.Perfil.Nombre != "Ana" {
		t.Fatalf("%+v %+v", e1.Rutas, e1.Perfil)
	}
	if err := os.MkdirAll(filepath.Join(notes, "ajustes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notes, "ajustes", "perfil.json"), []byte(`{"nombre":"Otra"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	e2, err := AbrirMovil(notes, grupo)
	if err != nil {
		t.Fatal(err)
	}
	if e2.Perfil.Nombre != "Ana" || e2.ID != e1.ID || e2.Rutas.Datos != filepath.Join(notes, "datos") {
		t.Fatalf("la segunda app no comparte los ajustes: %+v %+v", e2.Rutas, e2.Perfil)
	}
	// Sin grupo (perfil sin registrar), cada app tiene los suyos, como antes.
	e3, err := AbrirMovil(notes, "")
	if err != nil || e3.Perfil.Nombre != "Otra" {
		t.Fatalf("sin grupo: %+v %v", e3, err)
	}
}

// La carpeta «para-enviar» (lo que una app deja para que Mail lo adjunte) es una que
// comparten todas las apps de la edición y que no es de la persona: en el escritorio, en
// la caché; en iOS, en el App Group; y sin grupo (Android), ninguna.
func TestLaCarpetaParaEnviarLaCompartenLasApps(t *testing.T) {
	t.Setenv("SYTUE_OFFICE_DATOS", "")
	grupo := t.TempDir()
	movil, err := AbrirMovil(t.TempDir(), grupo)
	if err != nil || movil.Rutas.ParaEnviar != filepath.Join(grupo, CarpetaParaEnviar) {
		t.Fatalf("con grupo: %+v %v", movil, err)
	}
	if solo, err := AbrirMovil(t.TempDir(), ""); err != nil || solo.Rutas.ParaEnviar != "" {
		t.Fatalf("sin grupo: %+v %v", solo, err)
	}
	casa := t.TempDir()
	t.Setenv("HOME", casa)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(casa, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(casa, ".cache"))
	t.Setenv("AppData", filepath.Join(casa, "AppData"))
	t.Setenv("LocalAppData", filepath.Join(casa, "LocalAppData"))
	e, err := Abrir("")
	if err != nil {
		t.Fatal(err)
	}
	if e.Rutas.ParaEnviar != filepath.Join(e.Rutas.Cache, CarpetaParaEnviar) || strings.HasPrefix(e.Rutas.ParaEnviar, e.Rutas.Datos) {
		t.Fatalf("escritorio: %+v", e.Rutas)
	}
}
