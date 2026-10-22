// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"testing"

	"fyne.io/fyne/v2/test"
	temafyne "fyne.io/fyne/v2/theme"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

func TestElTraslucidoSeComparteYSeRecuperaAlAbrir(t *testing.T) {
	aparte(t)
	t.Cleanup(func() { tema.Elegir("") })
	a := test.NewApp()
	defer a.Quit()
	mail, drive := Abrir(a, "sytue.eu"), Abrir(a, "sytue.eu")
	if err := mail.Poner(func(aj *ajustes.Ajustes) { aj.Tema = "traslucido" }); err != nil {
		t.Fatal(err)
	}
	drive.aplicar(ajustes.Leer())
	if e := drive.Estado(); e.Tema != "traslucido" {
		t.Fatalf("Drive no ha recibido la elección de Mail: %+v", e)
	}
	if tema.De(temafyne.VariantLight) != tema.Translucido || tema.De(temafyne.VariantDark) != tema.Translucido {
		t.Error("la apariencia compartida no manda sobre el sistema")
	}

	// Otro proceso empieza sin ninguna elección en memoria y recupera la del disco.
	tema.Elegir("")
	photos := Abrir(a, "sytue.eu")
	if e := photos.Estado(); e.Tema != "traslucido" || tema.De(temafyne.VariantDark) != tema.Translucido {
		t.Fatalf("la app nueva no recupera la apariencia guardada: %+v", e)
	}
	if err := drive.Poner(func(aj *ajustes.Ajustes) { aj.Tema = "" }); err != nil {
		t.Fatal(err)
	}
	photos.aplicar(ajustes.Leer())
	if photos.Estado().Tema != "" || tema.De(temafyne.VariantLight) != tema.Claro || tema.De(temafyne.VariantDark) != tema.Oscuro {
		t.Error("volver a Sistema no se comparte con las otras aplicaciones")
	}
}

// TestElColorSeComparteYSeRecuperaAlAbrir: el color de la suite es un eje aparte del
// tema (ADR 0071) y viaja igual que él, por los ajustes compartidos: lo elige una app,
// lo aplican las otras y lo recupera la que se abre después. Cambiar el tema no lo toca.
//
// VISTO EN ROJO sin `tema.ElegirColor` en `aplicar`: Drive sigue pintando el primario
// de la casa.
func TestElColorSeComparteYSeRecuperaAlAbrir(t *testing.T) {
	aparte(t)
	t.Cleanup(func() { tema.Elegir(""); tema.ElegirColor("") })
	a := test.NewApp()
	defer a.Quit()
	deLaCasa := tema.Color("primary", tema.Claro)
	mail, drive := Abrir(a, "sytue.eu"), Abrir(a, "sytue.eu")
	if err := mail.Poner(func(aj *ajustes.Ajustes) { aj.Color = "cobalto" }); err != nil {
		t.Fatal(err)
	}
	drive.aplicar(ajustes.Leer())
	cobalto := tema.Color("primary", tema.Claro)
	if e := drive.Estado(); e.Color != "cobalto" || cobalto == deLaCasa {
		t.Fatalf("Drive no ha recibido el color de Mail: %+v, primario %v", e, cobalto)
	}
	// El tema es otro eje: cambiarlo no devuelve el color al de la casa.
	if err := drive.Poner(func(aj *ajustes.Ajustes) { aj.Tema = "oscuro" }); err != nil {
		t.Fatal(err)
	}
	if e := drive.Estado(); e.Color != "cobalto" || e.Tema != "oscuro" || tema.Color("primary", tema.Claro) != cobalto {
		t.Errorf("cambiar el tema ha tocado el color: %+v", e)
	}

	// Otro proceso empieza sin ninguna elección en memoria y recupera la del disco.
	tema.ElegirColor("")
	photos := Abrir(a, "sytue.eu")
	if e := photos.Estado(); e.Color != "cobalto" || tema.Color("primary", tema.Claro) != cobalto {
		t.Fatalf("la app nueva no recupera el color guardado: %+v", e)
	}
	if err := photos.Poner(func(aj *ajustes.Ajustes) { aj.Color = "" }); err != nil {
		t.Fatal(err)
	}
	if photos.Estado().Color != "app" || photos.Estado().ColorPropio != "" || tema.ColorElegido() != "app" {
		t.Error("olvidar la elección no vuelve al color predeterminado de cada app")
	}
}

// TestLaFormaYLosDefectosDeLaOrganizacion: la forma es el tercer eje y viaja como el
// tema y el color. Y lo que la persona NO ha elegido lo dice su organización, que llega
// con la sesión (reclamos `org_color` y `org_forma`): manda la persona, después la
// organización y después el color de cada app y Marco, como en la web.
//
// VISTO EN ROJO sin mirar la sesión en `apariencia`: con la organización en cobalto y
// Carril, quien no ha elegido nada sigue viendo petróleo y Marco.
func TestLaFormaYLosDefectosDeLaOrganizacion(t *testing.T) {
	tema.ElegirApp("")
	aparte(t)
	t.Cleanup(func() { tema.Elegir(""); tema.ElegirColor(""); tema.ElegirForma("") })
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	deLaCasa := tema.Color("primary", tema.Claro)
	if e := s.Estado(); e.Color != "app" || e.ColorOrg != "app" || e.ColorPropio != "" || e.Forma != "" || !tema.EsMarco() {
		t.Fatalf("sin elección y sin sesión tiene que verse el color de cada app y Marco: %+v, marco=%v", e, tema.EsMarco())
	}
	// La organización dice cobalto y Carril; la persona no ha elegido.
	s.Sesion.PonerApariencia("cobalto", "carril")
	s.aplicar(ajustes.Leer())
	if e := s.Estado(); e.Color != "cobalto" || e.Forma != "carril" || tema.EsMarco() || tema.Color("primary", tema.Claro) == deLaCasa {
		t.Fatalf("lo de la organización no se aplica a quien no ha elegido: %+v, marco=%v", e, tema.EsMarco())
	}
	// La persona elige: manda sobre su organización, también para volver a lo de la casa.
	if err := s.Poner(func(aj *ajustes.Ajustes) { aj.Color, aj.Forma = "petroleo", "marco" }); err != nil {
		t.Fatal(err)
	}
	if e := s.Estado(); e.Color != "petroleo" || e.Forma != "marco" || !tema.EsMarco() || tema.Color("primary", tema.Claro) != deLaCasa {
		t.Errorf("la elección de la persona no manda sobre la de su organización: %+v, marco=%v", e, tema.EsMarco())
	}
	// Y otra app lo recibe por los ajustes compartidos.
	otra := Abrir(a, "sytue.eu")
	if err := otra.Poner(func(aj *ajustes.Ajustes) { aj.Forma = "carril" }); err != nil {
		t.Fatal(err)
	}
	s.aplicar(ajustes.Leer())
	if e := s.Estado(); e.Forma != "carril" || tema.EsMarco() {
		t.Errorf("la forma elegida en otra app no llega: %+v", e)
	}
}

// El valor predeterminado se resuelve al pintar, sin guardarlo como si la persona
// lo hubiera elegido: una organización o una elección posterior siguen mandando.
func TestElColorDeCadaAppEsElPredeterminadoSinGuardarEleccion(t *testing.T) {
	aparte(t)
	t.Cleanup(func() { tema.Elegir(""); tema.ElegirColor(""); tema.ElegirApp("") })
	tema.ElegirColor("petroleo")
	deLaCasa := tema.Color("primary", tema.Claro)
	tema.ElegirApp("drive")
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	if e := s.Estado(); e.Color != "app" || e.ColorPropio != "" || tema.ColorElegido() != "app" || tema.Color("primary", tema.Claro) == deLaCasa {
		t.Fatalf("Drive no usa su color sin fijar una elección propia: %+v", e)
	}
	if ajustes.Leer().Color != "" {
		t.Error("el valor predeterminado se ha guardado como una elección de la persona")
	}
	s.Sesion.PonerApariencia("petroleo", "")
	s.aplicar(ajustes.Leer())
	if e := s.Estado(); e.Color != "petroleo" || e.ColorPropio != "" || tema.Color("primary", tema.Claro) != deLaCasa {
		t.Fatalf("el petróleo explícito de la organización se ha perdido: %+v", e)
	}
}
