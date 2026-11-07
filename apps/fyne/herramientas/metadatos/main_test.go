// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func appDePrueba(t *testing.T, toml string, lado int) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "FyneApp.toml"), []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
	if lado > 0 {
		var buf bytes.Buffer
		if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, lado, lado))); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Icon.png"), buf.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const tomlRooms = `
[Details]
  Name = "Sytue Rooms"
  ID = "eu.sytue.rooms"
  Version = "0.1.0"
  Build = 1
[Development]
  esquema = "sytue-rooms"
  client_id = "sytue-chat"
[Release]
  client_id = "cliente-release"
  extensiones = ""
`

// Lo que sale tiene que ser Go válido del paquete main, con lo que la CLI pondría.
func TestEscribeLoQueInyectabaFyneBuild(t *testing.T) {
	dir := appDePrueba(t, tomlRooms, 1024)
	d, err := Preparar(Opciones{
		Dir: dir, Version: "3.18.3", Build: 83, Release: true,
		Metadata: map[string]string{"esquema": "sytue-rooms"},
	})
	if err != nil {
		t.Fatal(err)
	}
	salida := filepath.Join(dir, "fyne_metadata_init.go")
	if err := Escribir(d, salida); err != nil {
		t.Fatal(err)
	}
	fichero, err := parser.ParseFile(token.NewFileSet(), salida, nil, 0)
	if err != nil {
		t.Fatalf("no es Go válido: %v", err)
	}
	if fichero.Name.Name != "main" {
		t.Fatalf("paquete %q, se esperaba main", fichero.Name.Name)
	}
	texto, _ := os.ReadFile(salida)
	for _, esperado := range []string{
		`ID:      "eu.sytue.rooms"`, `Name:    "Sytue Rooms"`, `Version: "3.18.3"`, `Build:   83`,
		`Release: true`, `"client_id": "cliente-release"`, `"esquema": "sytue-rooms"`, `"extensiones": ""`,
		`StaticName: "Icon-512.png"`,
	} {
		if !strings.Contains(string(texto), esperado) {
			t.Errorf("falta %s en\n%s", esperado, texto)
		}
	}
	if strings.Contains(string(texto), "sytue-chat") {
		t.Error("con -release se coló [Development]")
	}
	if strings.Join(d.Etiquetas, ",") != "release" {
		t.Errorf("etiquetas %v", d.Etiquetas)
	}
}

// Sin -release manda [Development], la versión y el build salen del TOML, y un
// --metadata pisa a la tabla (como appendCustomMetadata de la CLI).
func TestDevelopmentYPrecedencia(t *testing.T) {
	d, err := Preparar(Opciones{Dir: appDePrueba(t, tomlRooms, 0), Metadata: map[string]string{"client_id": "pisado"}})
	if err != nil {
		t.Fatal(err)
	}
	if d.AppVersion != "0.1.0" || d.AppBuild != 1 || d.Release || d.ResGoString != "nil" {
		t.Fatalf("%+v", d)
	}
	if d.CustomMetadata["client_id"] != "pisado" || d.CustomMetadata["esquema"] != "sytue-rooms" {
		t.Fatalf("custom %v", d.CustomMetadata)
	}
	if _, hay := d.CustomMetadata["extensiones"]; hay {
		t.Fatal("sin -release no debe leerse [Release]")
	}
}

func TestIconoReducidoComoLaCLI(t *testing.T) {
	for lado, esperado := range map[int]int{1024: iconoMaximo, 256: 256} {
		d, err := Preparar(Opciones{Dir: appDePrueba(t, tomlRooms, lado)})
		if err != nil {
			t.Fatal(err)
		}
		crudo, err := contenidoDe(d.ResGoString)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(strings.NewReader(crudo))
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != esperado {
			t.Errorf("icono de %d px quedó en %d, se esperaba %d", lado, img.Bounds().Dx(), esperado)
		}
	}
}

// contenidoDe saca los bytes del GoString de fyne.StaticResource, en la forma que use
// el Fyne de apps/fyne/go.mod (lista de bytes o cadena).
func contenidoDe(goString string) (string, error) {
	if m := regexp.MustCompile(`StaticContent: \[\]byte\{([^}]*)\}`).FindStringSubmatch(goString); m != nil {
		var buf []byte
		for _, campo := range strings.FieldsFunc(m[1], func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
			n, err := strconv.Atoi(campo)
			if err != nil {
				return "", err
			}
			buf = append(buf, byte(n))
		}
		return string(buf), nil
	}
	if m := regexp.MustCompile(`StaticContent: \[\]byte\(("(?:[^"\\]|\\.)*")\)`).FindStringSubmatch(goString); m != nil {
		return strconv.Unquote(m[1])
	}
	return "", fmt.Errorf("sin contenido: %.80s", goString)
}

func TestSinIDNiNombreEsError(t *testing.T) {
	if _, err := Preparar(Opciones{Dir: appDePrueba(t, "[Details]\n  Name = \"x\"\n", 0)}); err == nil {
		t.Fatal("un FyneApp.toml sin ID debe fallar, no inventar metadatos vacíos")
	}
}

func TestMigracionFyneDoDaSuEtiqueta(t *testing.T) {
	d, err := Preparar(Opciones{Dir: appDePrueba(t, tomlRooms+"[Migrations]\n  fyneDo = true\n", 0)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(d.Etiquetas, ",") != "migrated_fynedo" {
		t.Fatalf("etiquetas %v", d.Etiquetas)
	}
}

// Las apps de verdad: cada FyneApp.toml del catálogo se lee y su icono baja a 512.
func TestCatalogoReal(t *testing.T) {
	tomls, _ := filepath.Glob("../../cmd/*/FyneApp.toml")
	if len(tomls) < 10 {
		t.Fatalf("catálogo con %d apps", len(tomls))
	}
	for _, p := range tomls {
		d, err := Preparar(Opciones{Dir: filepath.Dir(p), Release: true})
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		if !strings.Contains(d.ResGoString, "-512.png") {
			t.Errorf("%s: el icono no se redujo", p)
		}
	}
}
