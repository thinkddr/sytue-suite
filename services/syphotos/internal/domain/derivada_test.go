// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func act() uuid.UUID { return uuid.MustParse("6f1c8f2e-3a4b-4c5d-8e9f-0a1b2c3d4e5f") }

// LA FORMA DE LA CLAVE ES EL DISEÑO, así que se fija: el activo primero
// (limpiar una foto borrada es un prefijo) y luego la huella de la receta.
//
// SIN TENANT: lo antepone `objectstore` (`tenants/<slug>/`). Se comprueba que NO
// esté, porque tenerlo aquí además duplicaría el aislamiento entre
// organizaciones en dos sitios que pueden discrepar.
func TestClave_forma(t *testing.T) {
	d := Derivada{
		ActivoID: act(), Huella: "ab12cd34ef56",
		Variante: VarianteMiniatura, Formato: "webp",
	}
	k, err := d.Clave()
	if err != nil {
		t.Fatal(err)
	}
	esperada := "a/6f1c8f2e-3a4b-4c5d-8e9f-0a1b2c3d4e5f/ab12cd34ef56/miniatura.webp"
	if k != esperada {
		t.Errorf("clave %q, se esperaba %q", k, esperada)
	}
	if strings.Contains(k, "tenant") || strings.HasPrefix(k, "t/") {
		t.Errorf("la clave lleva el tenant: lo pone el almacén, no el dominio (%q)", k)
	}
	if !strings.HasPrefix(k, PrefijoDelActivo(act())) {
		t.Error("la clave no cae bajo el prefijo de su activo")
	}
}

// Editar escribe en OTRO sitio en vez de invalidar: por eso deshacer devuelve un
// fichero que probablemente siga ahí, en vez de costar lo mismo que ir adelante.
func TestClave_cadaRecetaSuSitio(t *testing.T) {
	base := Derivada{ActivoID: act(), Variante: VariantePrevia, Formato: "webp"}
	sinEditar := base
	sinEditar.Huella = "000000000000"
	editada := base
	editada.Huella = "aaaaaaaaaaaa"

	a, _ := sinEditar.Clave()
	b, _ := editada.Clave()
	if a == b {
		t.Fatal("dos recetas distintas comparten fichero: una pisaría a la otra")
	}
	// Y comparten el prefijo del activo, que es lo que permite limpiarlas juntas.
	if !strings.HasPrefix(a, PrefijoDelActivo(act())) ||
		!strings.HasPrefix(b, PrefijoDelActivo(act())) {
		t.Error("las versiones de una misma foto no caen bajo un prefijo común")
	}
}

// LO QUE NO PUEDE PASAR: una clave es una ruta, y un segmento con «..» escribiría
// fuera de su prefijo — la fuga entre organizaciones que el prefijo evita.
// `path.Join` limpia los «..», pero limpiar en silencio deja la clave apuntando a
// otro sitio sin que nadie se entere, así que aquí se rechaza.
func TestClave_rechazaSegmentosQueCambianLaRuta(t *testing.T) {
	ok := Derivada{
		ActivoID: act(), Huella: "abc123",
		Variante: VarianteMiniatura, Formato: "webp",
	}
	casos := map[string]func(*Derivada){
		"huella con salto":   func(d *Derivada) { d.Huella = "../x" },
		"huella con barra":   func(d *Derivada) { d.Huella = "a/b" },
		"huella vacía":       func(d *Derivada) { d.Huella = "" },
		"huella con punto":   func(d *Derivada) { d.Huella = ".." },
		"formato con salto":  func(d *Derivada) { d.Formato = "../../etc/passwd" },
		"activo sin id":      func(d *Derivada) { d.ActivoID = uuid.Nil },
		"variante inventada": func(d *Derivada) { d.Variante = "gigante" },
		"huella desmesurada": func(d *Derivada) { d.Huella = strings.Repeat("a", 65) },
	}
	for nombre, romper := range casos {
		d := ok
		romper(&d)
		if k, err := d.Clave(); err == nil {
			t.Errorf("%s: se aceptó y produjo %q", nombre, k)
		}
	}
	// Y el caso bueno sigue pasando, que es la otra mitad.
	if _, err := ok.Clave(); err != nil {
		t.Errorf("una derivada correcta fue rechazada: %v", err)
	}
}

// Los tamaños se consultan, no se escriben en cada sitio: cambiarlos en dos
// lugares distintos genera dos ficheros para la misma cosa.
func TestVariantes_tienenLadoYEstanTodas(t *testing.T) {
	vs := Variantes()
	if len(vs) != 3 {
		t.Fatalf("%d variantes, se esperaban 3", len(vs))
	}
	previo := 0
	for _, v := range vs {
		n, ok := v.LadoMayor()
		if !ok || n <= 0 {
			t.Fatalf("la variante %q no declara lado mayor", v)
		}
		// En orden de tamaño: la interfaz pide la más pequeña que sirva.
		if n <= previo {
			t.Errorf("las variantes no van de menor a mayor: %q mide %d tras %d", v, n, previo)
		}
		previo = n
	}
	if _, ok := Variante("gigante").LadoMayor(); ok {
		t.Error("una variante inventada devolvió tamaño")
	}
}

func TestClavesDeLasDerivadas_incluyeJpgYPng(t *testing.T) {
	claves, err := ClavesDeLasDerivadas(act(), "huella123")
	if err != nil {
		t.Fatal(err)
	}
	if len(claves) != len(VariantesConocidas)*2 {
		t.Fatalf("se esperaban %d claves (jpg + png para 3 variantes), got %d", len(VariantesConocidas)*2, len(claves))
	}
	tieneJpg, tienePng := false, false
	for _, c := range claves {
		if strings.HasSuffix(c, ".jpg") {
			tieneJpg = true
		}
		if strings.HasSuffix(c, ".png") {
			tienePng = true
		}
	}
	if !tieneJpg || !tienePng {
		t.Errorf("las claves deben incluir tanto .jpg como .png para limpiar todas las derivadas")
	}
}
