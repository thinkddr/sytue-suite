// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

const emisorPrueba = "https://id.sytue.eu"

func alcanceDe(sujeto, org string) Alcance {
	return Alcance{Emisor: emisorPrueba, Sujeto: sujeto, Organizacion: org}
}

// Lo que guarda una persona no lo lee otra, y sin nadie dentro no se guarda nada.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el sujeto en la clave, Beto lee lo que
// plegó Ana.
func TestPorPersonaSeparaALasPersonas(t *testing.T) {
	prefs := test.NewApp().Preferences()
	quien := alcanceDe("ana", "acme")
	p := NuevoPorPersona(prefs, func() Alcance { return quien })
	p.Poner("plegadas", `["dms"]`)
	if p.Leer("plegadas") != `["dms"]` {
		t.Fatalf("Ana no lee lo suyo: %q", p.Leer("plegadas"))
	}
	quien = alcanceDe("beto", "acme")
	if got := p.Leer("plegadas"); got != "" {
		t.Errorf("Beto lee lo de Ana: %q", got)
	}
	quien = Alcance{Emisor: emisorPrueba}
	p.Poner("plegadas", "x")
	if got := p.Leer("plegadas"); got != "" {
		t.Errorf("sin nadie dentro se guarda %q", got)
	}
	quien = alcanceDe("ana", "acme")
	p.Poner("plegadas", "")
	if got := p.Leer("plegadas"); got != "" {
		t.Errorf("poner vacío no lo borra: %q", got)
	}
}

// La misma persona en dos organizaciones —o el mismo `sub` en otra instalación de Sytue
// ID— son dos cuentas: lo de una no lo lee ni lo pisa la otra.
//
// VISTO EN ROJO (2026-11-25): con la clave de solo el sujeto, lo que Ana pliega en Acme
// sale plegado en su cuenta de Globex, y lo de Globex pisa lo de Acme.
func TestPorPersonaNoMezclaDosOrganizacionesDelMismoSujeto(t *testing.T) {
	prefs := test.NewApp().Preferences()
	quien := alcanceDe("ana", "acme")
	p := NuevoPorPersona(prefs, func() Alcance { return quien })
	p.Poner("plegadas", `["dms"]`)

	quien = alcanceDe("ana", "globex")
	if got := p.Leer("plegadas"); got != "" {
		t.Errorf("en Globex Ana lee lo de Acme: %q", got)
	}
	p.Poner("plegadas", `["canales"]`)

	quien = alcanceDe("ana", "acme")
	if got := p.Leer("plegadas"); got != `["dms"]` {
		t.Errorf("lo de Globex pisa lo de Acme: %q", got)
	}
	quien = Alcance{Emisor: "https://id.otra.example", Sujeto: "ana", Organizacion: "acme"}
	if got := p.Leer("plegadas"); got != "" {
		t.Errorf("el mismo sub en otro Sytue ID lee lo de este: %q", got)
	}
}

// Persona cambia con la cuenta, también al cambiar SOLO de organización: lo usa quien
// además lo tiene en memoria (las macros de los editores) para saber cuándo recargarlo.
func TestPorPersonaDiceQuienEstaDentro(t *testing.T) {
	quien := alcanceDe("ana", "acme")
	p := NuevoPorPersona(test.NewApp().Preferences(), func() Alcance { return quien })
	acme := p.Persona()
	if acme == "" {
		t.Fatal("dentro está Ana y no dice nadie")
	}
	quien = alcanceDe("ana", "globex")
	if p.Persona() == acme || p.Persona() == "" {
		t.Errorf("Ana en Globex se dice igual que en Acme: %q", p.Persona())
	}
	quien = Alcance{Emisor: emisorPrueba}
	if p.Persona() != "" {
		t.Errorf("sin nadie dentro dice %q", p.Persona())
	}
	if (PorPersona{}).Persona() != "" {
		t.Error("sin preferencias no hay nadie")
	}
}

// Lo guardado con la clave de antes (solo el sujeto) pasa ENTERO a la primera cuenta de
// esa persona que lo lee, una sola vez: la otra organización empieza sin nada, como
// cualquier cuenta nueva, y la clave vieja desaparece.
//
// VISTO EN ROJO (2026-11-25): sin la migración, Ana pierde lo que había plegado al
// actualizar; sin borrar la vieja, Globex hereda lo de Acme.
func TestPorPersonaMigraLoDeAntesAUnaSolaCuenta(t *testing.T) {
	prefs := test.NewApp().Preferences()
	prefs.SetString("ana\x00plegadas", `["dms"]`)
	prefs.SetString("ana\x00formato", "si")
	prefs.SetString("beto\x00plegadas", `["hilos"]`)

	quien := alcanceDe("ana", "acme")
	p := NuevoPorPersona(prefs, func() Alcance { return quien })
	if got := p.Leer("plegadas"); got != `["dms"]` {
		t.Fatalf("Ana pierde lo que había guardado: %q", got)
	}
	if got := prefs.String("ana\x00plegadas"); got != "" {
		t.Errorf("la clave de antes sigue ahí: %q", got)
	}
	if got := p.Leer("plegadas"); got != `["dms"]` {
		t.Errorf("leído otra vez, ya desde su ámbito: %q", got)
	}

	quien = alcanceDe("ana", "globex")
	if got := p.Leer("plegadas"); got != "" {
		t.Errorf("Globex hereda lo que Acme ya se quedó: %q", got)
	}
	// Escribir también se queda con la clave: «formato» lo escribe Globex primero y Acme
	// ya no lo hereda.
	p.Poner("formato", "no")
	if got := prefs.String("ana\x00formato"); got != "" {
		t.Errorf("escribir no consume la clave de antes: %q", got)
	}
	quien = alcanceDe("ana", "acme")
	if got := p.Leer("formato"); got != "" {
		t.Errorf("Acme hereda lo de antes después de que Globex escribiera: %q", got)
	}

	// Lo de otra persona sigue a la espera de la suya.
	quien = alcanceDe("beto", "acme")
	if got := p.Leer("plegadas"); got != `["hilos"]` {
		t.Errorf("Beto pierde lo suyo: %q", got)
	}
}
