// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// menuDeCampo es el botón ▾ de la cabecera del campo `nombre`, si se ve.
func menuDeCampo(w fyne.Window, nombre string) *ui.BotonIcono {
	var hallado *ui.BotonIcono
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.BotonIcono); ok && b.Rotulo == "Opciones del campo "+nombre {
			hallado = b
		}
	})
	return hallado
}

// opcionesAbiertas son los elementos del menú abierto, y el menú.
func opcionesAbiertas(w fyne.Window) ([]string, *ui.Menu) {
	var out []string
	var menu *ui.Menu
	enCapas(w, func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.ElementoMenu:
			out = append(out, x.Texto)
		case *ui.Menu:
			menu = x
		}
	})
	return out, menu
}

// elegirDelCampo abre el menú ▾ del campo `nombre` y pulsa su elemento `texto`.
func elegirDelCampo(t *testing.T, w fyne.Window, nombre, texto string) {
	t.Helper()
	b := menuDeCampo(w, nombre)
	if b == nil {
		t.Fatalf("la cabecera de %s no tiene menú:\n%s", nombre, textos(w))
	}
	test.Tap(b)
	var e *ui.ElementoMenu
	enCapas(w, func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.ElementoMenu); ok && x.Texto == texto {
			e = x
		}
	})
	if e == nil {
		t.Fatalf("el menú de %s no ofrece «%s»:\n%s", nombre, texto, textos(w))
	}
	test.Tap(e)
}

func nombresDeCampos(p *Pantalla) []string {
	out := make([]string, 0, len(p.abierta.rejilla.Campos))
	for _, c := range p.abierta.rejilla.Campos {
		out = append(out, c.Name)
	}
	return out
}

// TestElPropietarioGestionaUnCampoDesdeSuCabecera: el menú ▾ de cada cabecera renombra,
// cambia la fórmula (por ids, y solo si cambia) y borra; el error del servicio se queda
// en el diálogo, y lo que usa una fórmula no se deja borrar. Tras cada cambio la rejilla
// trae la estructura nueva.
//
// Verlo en rojo: quitar `g.AlCampo = p.alCampo` en ponerRejilla (no hay menú), o el
// `formulasQueUsan` de dialogoBorrarCampo (se borra «Edad» con la fórmula colgando).
func TestElPropietarioGestionaUnCampoDesdeSuCabecera(t *testing.T) {
	f := nuevoFalso(2)
	p, w := abrir(t, f)
	g := p.abierta.rejilla

	// El menú se anuncia por su nombre y ofrece lo que corresponde a cada tipo.
	b := menuDeCampo(w, "Edad")
	if b == nil {
		t.Fatalf("la cabecera de Edad no tiene menú:\n%s", textos(w))
	}
	if b.AccessibilityLabel() != "Opciones del campo Edad" || b.AccessibilityRole() != fyne.AccessibleRoleButton {
		t.Errorf("el lector anuncia %q (%v)", b.AccessibilityLabel(), b.AccessibilityRole())
	}
	test.Tap(b)
	opciones, menu := opcionesAbiertas(w)
	if !slices.Equal(opciones, []string{"Renombrar…", "Mover a la izquierda", "Mover a la derecha", "Eliminar campo"}) || menu == nil || menu.AccessibilityLabel() != "Opciones del campo Edad" {
		t.Errorf("el menú de un número ofrece %v (%v)", opciones, menu)
	}
	menu.Cerrar()
	test.Tap(menuDeCampo(w, "Doble"))
	opciones, menu = opcionesAbiertas(w)
	if !slices.Equal(opciones, []string{"Renombrar…", "Editar fórmula…", "Mover a la izquierda", "Mover a la derecha", "Eliminar campo"}) {
		t.Errorf("el menú de una fórmula ofrece %v", opciones)
	}
	capturar(t, w, "menu-campo")
	menu.Cerrar()

	// Renombrar: un nombre repetido deja el error en el diálogo, abierto y con lo escrito.
	elegirDelCampo(t, w, "Edad", "Renombrar…")
	nombre := campoDeDialogo(t, w, "Nombre del campo")
	if nombre.Texto() != "Edad" || !boton(w, "Guardar").Inerte {
		t.Fatalf("el diálogo no trae el nombre de ahora (%q) o deja guardarlo igual", nombre.Texto())
	}
	nombre.PonerTexto("Nombre")
	pulsar(t, w, "Guardar")
	if got := textos(w); !strings.Contains(got, p.t("err.invalid")) || !strings.Contains(got, "Renombrar el campo Edad") {
		t.Fatalf("el error del servicio no se lee en el diálogo:\n%s", got)
	}
	nombre.PonerTexto("  Años ")
	capturar(t, w, "dialogo-renombrar-campo")
	pulsar(t, w, "Guardar")
	if !f.vio(`PATCH /fields/eda {"name":"Años"}`) {
		t.Fatalf("no se ha renombrado: %q", f.peticiones("PATCH /fields"))
	}
	if !slices.Contains(nombresDeCampos(p), "Años") || menuDeCampo(w, "Años") == nil || len(w.Canvas().Overlays().List()) != 0 {
		t.Fatalf("la rejilla no trae el nombre nuevo o el diálogo sigue abierto: %v", nombresDeCampos(p))
	}

	// La fórmula se enseña con los nombres de ahora y viaja por ids, solo si cambia.
	elegirDelCampo(t, w, "Doble", "Editar fórmula…")
	formula := campoDeDialogo(t, w, "Fórmula")
	if formula.Texto() != "{Años} * 2" {
		t.Fatalf("la fórmula guardada se enseña como %q", formula.Texto())
	}
	if !boton(w, "Guardar").Inerte {
		t.Error("se puede guardar la misma fórmula")
	}
	formula.PonerTexto("{Años} * {Nada}")
	if got := textos(w); !strings.Contains(got, "Hay un campo entre llaves que no existe en esta tabla.") || !boton(w, "Guardar").Inerte {
		t.Errorf("una fórmula con un campo que no existe no avisa o se deja guardar:\n%s", got)
	}
	formula.PonerTexto("{Años} * 3")
	capturar(t, w, "dialogo-formula-campo")
	pulsar(t, w, "Guardar")
	if !f.vio(`PATCH /fields/dob {"formula":"{eda} * 3"}`) {
		t.Fatalf("la fórmula no viaja por ids: %q", f.peticiones("PATCH /fields"))
	}
	if c := p.abierta.Campos[slices.IndexFunc(p.abierta.Campos, func(c api.BasesField) bool { return c.ID == "dob" })]; c.Formula != "{eda} * 3" {
		t.Errorf("la estructura no se ha recargado: %q", c.Formula)
	}

	// Lo que usa una fórmula no se borra: el diálogo dice cuál y el botón no se activa.
	elegirDelCampo(t, w, "Años", "Eliminar campo")
	if got := textos(w); !strings.Contains(got, "El campo Años lo usan estas fórmulas: Doble.") || !boton(w, "Borrar").Inerte {
		t.Fatalf("borrar un campo que usa una fórmula no se impide:\n%s", got)
	}
	pulsar(t, w, "Cancelar")
	elegirDelCampo(t, w, "Activa", "Eliminar campo")
	if got := textos(w); !strings.Contains(got, "¿Eliminar el campo Activa? Sus valores se borran de todos los registros") {
		t.Fatalf("borrar no avisa de que se lleva los valores:\n%s", got)
	}
	capturar(t, w, "dialogo-borrar-campo")
	pulsar(t, w, "Borrar")
	if !f.vio("DELETE /fields/act") || slices.Contains(nombresDeCampos(p), "Activa") || len(g.Campos) != 7 {
		t.Fatalf("el campo no se ha borrado de la rejilla: %v, %q", nombresDeCampos(p), f.peticiones("DELETE"))
	}
	if len(f.peticiones("DELETE /fields/eda")) != 0 {
		t.Error("se ha pedido borrar un campo que usa una fórmula")
	}
}

// TestLaCabeceraEnteraAbreElMenu: no hace falta atinar con el ▾; un toque o el clic
// derecho en cualquier punto de la cabecera abren su menú, y la cabecera crece hasta el
// botón como en la web.
func TestLaCabeceraEnteraAbreElMenu(t *testing.T) {
	p, w := abrir(t, nuevoFalso(1))
	g := p.abierta.rejilla
	if g.altoCab() != 2*margenCabeza+ladoMenuCampo {
		t.Errorf("con menú la cabecera mide %v", g.altoCab())
	}
	x, _ := g.r.columna(1)
	g.Tapped(&fyne.PointEvent{Position: fyne.NewPos(x+2, 2)})
	if _, menu := opcionesAbiertas(w); menu == nil || menu.Rotulo != "Opciones del campo Edad" {
		t.Fatal("tocar la cabecera de Edad no abre su menú")
	} else {
		menu.Cerrar()
	}
	g.TappedSecondary(&fyne.PointEvent{Position: fyne.NewPos(x+2, g.altoCab()-2)})
	if _, menu := opcionesAbiertas(w); menu == nil || menu.Rotulo != "Opciones del campo Edad" {
		t.Fatal("el clic derecho en la cabecera no abre su menú")
	} else {
		menu.Cerrar()
	}
	// Una celda sigue siendo una celda.
	g.Tapped(&fyne.PointEvent{Position: fyne.NewPos(x+2, g.altoCab()+2)})
	if _, menu := opcionesAbiertas(w); menu != nil || g.foco != (celda{0, 1}) {
		t.Errorf("tocar una celda abre un menú o no la enfoca (%v)", g.foco)
	}
}

// TestSinEsquemaNoHayMenuDeCampo: quien edita registros pero no el esquema no ve el ▾,
// la cabecera mide su texto y tocarla no abre nada.
func TestSinEsquemaNoHayMenuDeCampo(t *testing.T) {
	for _, rol := range []string{"editor", "viewer"} {
		f := nuevoFalso(1)
		f.rol = rol
		p, w := abrir(t, f)
		g := p.abierta.rejilla
		if menuDeCampo(w, "Edad") != nil || g.AlCampo != nil || g.altoCab() != altoCabeza {
			t.Errorf("%s: hay menú de campo en la cabecera", rol)
		}
		g.TappedSecondary(&fyne.PointEvent{Position: fyne.NewPos(g.r.ancho0+2, 2)})
		if _, menu := opcionesAbiertas(w); menu != nil {
			t.Errorf("%s: la cabecera abre un menú", rol)
		}
	}
}

// TestLasOpcionesDeUnaSeleccionSeEditanYLasRetiradasSeConservan: «Editar opciones…» sale
// en las selecciones, avisa de las que se retiran y manda la lista limpia; el registro que
// usaba una retirada la sigue enseñando, en gris y dicho con texto.
//
// Verlo en rojo: quitar el `accionOpciones` del menú, o `piezaDeOpcion` (la retirada sale
// como una opción más, sin decir nada).
func TestLasOpcionesDeUnaSeleccionSeEditanYLasRetiradasSeConservan(t *testing.T) {
	f := nuevoFalso(1)
	f.registros["t1"][0]["data"].(m)["niv"] = "Plata"
	p, w := abrir(t, f)

	elegirDelCampo(t, w, "Nivel", "Editar opciones…")
	area := campoDeDialogo(t, w, "Opciones")
	if area.Texto() != "Oro\nPlata" || !boton(w, "Guardar").Inerte {
		t.Fatalf("el diálogo no trae las opciones de ahora (%q) o deja guardarlas igual", area.Texto())
	}
	area.PonerTexto(" Oro \nBronce\n\nBronce")
	if got := textos(w); !strings.Contains(got, "Estas opciones quedarán retiradas: Plata.") {
		t.Fatalf("no avisa de la opción que se retira:\n%s", got)
	}
	capturar(t, w, "dialogo-opciones-campo")
	pulsar(t, w, "Guardar")
	if !f.vio(`PATCH /fields/niv {"choices":["Oro","Bronce"]}`) {
		t.Fatalf("las opciones no viajan limpias: %q", f.peticiones("PATCH /fields"))
	}
	g := p.abierta.rejilla
	i := slices.IndexFunc(g.Campos, func(c api.BasesField) bool { return c.ID == "niv" })
	if i < 0 || !slices.Equal(g.Campos[i].Choices, []string{"Oro", "Bronce"}) {
		t.Fatalf("la rejilla no trae las opciones nuevas: %+v", g.Campos)
	}
	piezas := g.piezas(g.Registros[0], g.Campos[i])
	if len(piezas) != 1 || piezas[0].texto != "Plata · opción retirada" || piezas[0].tono != ui.InsigniaNeutra {
		t.Errorf("la opción retirada se pinta %+v", piezas)
	}
	if got := g.piezas(api.BasesRecord{Data: map[string]any{"niv": "Oro"}}, g.Campos[i]); got[0].texto != "Oro" || got[0].tono == ui.InsigniaNeutra {
		t.Errorf("una opción viva se pinta %+v", got)
	}

	// En un campo que no es de selección no se ofrece.
	test.Tap(menuDeCampo(w, "Edad"))
	opciones, menu := opcionesAbiertas(w)
	if slices.Contains(opciones, "Editar opciones…") {
		t.Errorf("un número ofrece editar opciones: %v", opciones)
	}
	menu.Cerrar()
}

// TestLosCamposSeReordenanDesdeElMenuYConAltFlechas: «Mover a la derecha» y Alt+← sobre
// una celda cambian el orden en la rejilla al momento y mandan el de TODOS los campos; el
// foco va con la columna, y se dice adónde ha ido.
//
// Verlo en rojo: quitar `g.AlMoverCampo = p.moverCampo` en ponerRejilla, o el caso de
// Alt en TypedShortcut.
func TestLosCamposSeReordenanDesdeElMenuYConAltFlechas(t *testing.T) {
	f := nuevoFalso(1)
	p, w := abrir(t, f)
	g := p.abierta.rejilla

	elegirDelCampo(t, w, "Nombre", "Mover a la derecha")
	if got := nombresDeCampos(p); !slices.Equal(got[:3], []string{"Edad", "Nombre", "Alta"}) {
		t.Fatalf("orden tras mover: %v", got)
	}
	if !f.vio(`PUT /tables/t1/field-order {"fields":["eda","nom","alt","act","niv","eti","emp","dob"]}`) {
		t.Fatalf("no se manda el orden entero: %q", f.peticiones("PUT /tables"))
	}

	// Con el teclado: el foco en una celda de «Alta» (columna 3) y Alt+←.
	g.enfocar(celda{fila: 0, col: 2})
	g.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyLeft, Modifier: fyne.KeyModifierAlt})
	if got := nombresDeCampos(p); !slices.Equal(got[:3], []string{"Edad", "Alta", "Nombre"}) {
		t.Fatalf("orden tras Alt+←: %v", got)
	}
	if g.foco.col != 1 || g.Aviso != "Alta pasa a la columna 2." {
		t.Errorf("el foco no sigue a la columna (%d) o no se dice: %q", g.foco.col, g.Aviso)
	}
	// En el borde no se mueve, y la primera columna no ofrece ir más a la izquierda.
	g.enfocar(celda{fila: 0, col: 0})
	antes := len(f.peticiones("PUT /tables"))
	g.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyLeft, Modifier: fyne.KeyModifierAlt})
	if len(f.peticiones("PUT /tables")) != antes {
		t.Error("la primera columna se ha intentado mover a la izquierda")
	}
	test.Tap(menuDeCampo(w, "Edad"))
	var izq *ui.ElementoMenu
	enCapas(w, func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.ElementoMenu); ok && x.Texto == "Mover a la izquierda" {
			izq = x
		}
	})
	if izq == nil || !izq.Inerte {
		t.Error("la primera columna ofrece moverse a la izquierda")
	}
	capturar(t, w, "menu-campo-mover")
}
