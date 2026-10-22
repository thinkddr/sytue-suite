// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Lo que de verdad hay que garantizar: que un fichero NUNCA queda a medias. Si el
// texto no aparece las veces declaradas, el contenido tiene que seguir byte a byte
// como estaba, porque el llamante suele encadenar varias sustituciones y una
// escritura parcial deja el fichero en un estado que nadie escribió.
func TestNoTocaNadaSiElConteoNoCuadra(t *testing.T) {
	for _, caso := range []struct {
		nombre, viejo string
		veces         int
	}{
		{"texto ausente", "no está en el fichero", 1},
		{"aparece más veces de las declaradas", "mundo\n", 1},
		{"aparece menos veces de las declaradas", "mundo\n", 3},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			ruta := filepath.Join(t.TempDir(), "f.txt")
			antes := "hola mundo\nadiós mundo\n"
			if err := os.WriteFile(ruta, []byte(antes), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := aplicar(ruta, caso.viejo, "X", caso.veces); err == nil {
				t.Fatal("se esperaba error y no lo hubo")
			}
			despues, err := os.ReadFile(ruta)
			if err != nil {
				t.Fatal(err)
			}
			if string(despues) != antes {
				t.Fatalf("el fichero se modificó pese al error:\n%q", despues)
			}
		})
	}
}

func TestSustituyeTodasLasDeclaradas(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(ruta, []byte("a\nb\na\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := aplicar(ruta, "a\n", "c\n", 2); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "c\nb\nc\n" {
		t.Fatalf("resultado inesperado: %q", got)
	}
}

// Los scripts del puesto son ejecutables. Escribirlos con 0644 los deja sin
// arrancar, y el fallo aparece mucho después y lejos de aquí.
func TestConservaLosPermisos(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "s.sh")
	if err := os.WriteFile(ruta, []byte("#!/bin/sh\necho hola\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := aplicar(ruta, "hola", "adiós", 1); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(ruta)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("los permisos pasaron de 0755 a %o", info.Mode().Perm())
	}
}

func TestPartirExigeLasMarcas(t *testing.T) {
	if _, _, err := partir("sin marcas\n", false); err == nil {
		t.Fatal("se esperaba error sin @@VIEJO@@")
	}
	if _, _, err := partir("@@VIEJO@@\nalgo\n", false); err == nil {
		t.Fatal("se esperaba error sin @@NUEVO@@")
	}
	viejo, nuevo, err := partir("@@VIEJO@@\nuno\n@@NUEVO@@\ndos\n", false)
	if err != nil || viejo != "uno\n" || nuevo != "dos\n" {
		t.Fatalf("partir devolvió (%q, %q, %v)", viejo, nuevo, err)
	}
	viejo, nuevo, err = partir("@@VIEJO@@\nuno\n", true)
	if err != nil || viejo != "uno\n" || nuevo != "" {
		t.Fatalf("borrado devolvió (%q, %q, %v)", viejo, nuevo, err)
	}
}
