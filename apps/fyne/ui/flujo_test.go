// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// bloque es un hijo de medida fija para contar el reparto sin depender de la letra.
func bloque(w, h float32) *canvas.Rectangle {
	r := canvas.NewRectangle(nil)
	r.SetMinSize(fyne.NewSize(w, h))
	return r
}

// TestFlujoPasaALaLineaSiguienteElQueNoCabe: el `flex-wrap` con su `gap`, que es el
// mismo entre columnas y entre filas.
func TestFlujoPasaALaLineaSiguienteElQueNoCabe(t *testing.T) {
	a, b, c := bloque(40, 20), bloque(40, 20), bloque(40, 20)
	f := container.New(Flujo{Hueco: 4}, a, b, c)
	if m := f.MinSize(); m != fyne.NewSize(128, 20) {
		t.Errorf("sin ancho el flujo pide %v y es la fila entera sin partir, 128×20", m)
	}
	f.Resize(fyne.NewSize(90, 44))
	if a.Position() != fyne.NewPos(0, 0) || b.Position() != fyne.NewPos(44, 0) {
		t.Errorf("la primera línea va en %v y %v, y son (0,0) y (44,0)", a.Position(), b.Position())
	}
	if c.Position() != fyne.NewPos(0, 24) {
		t.Errorf("el tercero va en %v y, como no cabe, baja a (0,24)", c.Position())
	}
	if h := AltoFlujo(f, 90); h != 44 {
		t.Errorf("a 90 el flujo mide %v de alto y son dos líneas con su hueco, 44", h)
	}
	if h := AltoFlujo(f, 200); h != 20 {
		t.Errorf("a 200 el flujo mide %v de alto y cabe en una línea, 20", h)
	}
}

// TestFlujoConAnchoDeclaraElAltoPartido: con `Ancho`, el mínimo es el del flujo ya
// partido, que es lo que necesita una columna que suma mínimos para crecer.
func TestFlujoConAnchoDeclaraElAltoPartido(t *testing.T) {
	f := container.New(Flujo{Hueco: 4, Ancho: 90}, bloque(40, 20), bloque(40, 20), bloque(40, 20))
	if m := f.MinSize(); m != fyne.NewSize(90, 44) {
		t.Errorf("con Ancho 90 el flujo pide %v y son 90×44", m)
	}
}

// TestFlujoIgnoraLosOcultosYCentraCadaLinea: un hijo oculto no ocupa sitio ni hueco, y
// en cada línea los bajos se centran sobre el más alto (`items-center`).
func TestFlujoIgnoraLosOcultosYCentraCadaLinea(t *testing.T) {
	oculto, alto, bajo := bloque(50, 50), bloque(30, 30), bloque(30, 10)
	oculto.Hide()
	f := container.New(Flujo{Hueco: 6}, oculto, alto, bajo)
	if m := f.MinSize(); m != fyne.NewSize(66, 30) {
		t.Errorf("el flujo pide %v y el oculto no cuenta: 66×30", m)
	}
	f.Resize(fyne.NewSize(100, 30))
	if alto.Position() != fyne.NewPos(0, 0) || bajo.Position() != fyne.NewPos(36, 10) {
		t.Errorf("van en %v y %v, y son (0,0) y (36,10)", alto.Position(), bajo.Position())
	}
	if h := AltoFlujo(container.New(Flujo{}, oculto), 100); h != 0 {
		t.Errorf("un flujo sin hijos visibles mide %v y no ocupa nada", h)
	}
}

// TestFlujoEmpujaElUltimoALaDerechaDeSuLinea: `UltimoADerecha` es el `ms-auto` del
// último hijo, en la línea en que caiga.
func TestFlujoEmpujaElUltimoALaDerechaDeSuLinea(t *testing.T) {
	a, ultimo := bloque(40, 20), bloque(30, 20)
	f := container.New(Flujo{Hueco: 4, UltimoADerecha: true}, a, ultimo)
	f.Resize(fyne.NewSize(200, 20))
	if x := ultimo.Position().X; x != 170 {
		t.Errorf("en ancho el último va en x = %v y es el borde derecho, 170", x)
	}
	f.Resize(fyne.NewSize(60, 44))
	if p := ultimo.Position(); p != fyne.NewPos(30, 24) {
		t.Errorf("estrecho el último va en %v y baja a la derecha de su línea, (30,24)", p)
	}
}

// TestFlujoAnidadoNoSeSaleDeLaLinea: un flujo dentro de otro, más ancho que la línea,
// se queda con el ancho de la línea y se parte por dentro.
func TestFlujoAnidadoNoSeSaleDeLaLinea(t *testing.T) {
	dentro := container.New(Flujo{Hueco: 4}, bloque(40, 20), bloque(40, 20), bloque(40, 20))
	fuera := container.New(Flujo{Hueco: 4}, dentro)
	fuera.Resize(fyne.NewSize(90, 44))
	if s := dentro.Size(); s != fyne.NewSize(90, 44) {
		t.Errorf("el flujo de dentro mide %v y se ajusta a la línea, 90×44", s)
	}
	if h := AltoFlujo(fuera, 90); h != 44 {
		t.Errorf("el de fuera mide %v de alto y es el del de dentro partido, 44", h)
	}
}

// TestFlujoPrimeroEnteroOcupaSuLinea: el rótulo `w-full` de una sección de la vista
// rápida de Calendar va solo en su línea y los demás debajo.
func TestFlujoPrimeroEnteroOcupaSuLinea(t *testing.T) {
	r, a, b := bloque(30, 10), bloque(40, 20), bloque(40, 20)
	f := container.New(Flujo{Hueco: 4, Ancho: 100, PrimeroEntero: true}, r, a, b)
	if m := f.MinSize(); m != fyne.NewSize(100, 34) {
		t.Errorf("el flujo pide %v y son el rótulo y una línea con su hueco, 100×34", m)
	}
	f.Resize(fyne.NewSize(100, 34))
	if r.Size().Width != 100 {
		t.Errorf("el rótulo mide %v de ancho y ocupa la línea, 100", r.Size().Width)
	}
	if a.Position() != fyne.NewPos(0, 14) || b.Position() != fyne.NewPos(44, 14) {
		t.Errorf("los demás van en %v y %v, y son (0,14) y (44,14)", a.Position(), b.Position())
	}
}

// TestFlujoRepartirLlevaElSobranteEntreLosHijos: `justify-between` por línea, como el
// resumen y los controles de los huecos libres de Calendar; una línea de un solo hijo
// no se mueve.
func TestFlujoRepartirLlevaElSobranteEntreLosHijos(t *testing.T) {
	a, b, c := bloque(20, 10), bloque(20, 10), bloque(70, 10)
	f := container.New(Flujo{Hueco: 4, Repartir: true}, a, b, c)
	f.Resize(fyne.NewSize(100, 24))
	if a.Position().X != 0 || b.Position().X != 80 {
		t.Errorf("la primera línea va en x=%v y x=%v, y son 0 y 80", a.Position().X, b.Position().X)
	}
	if c.Position() != fyne.NewPos(0, 14) {
		t.Errorf("el tercero va en %v y, solo en su línea, se queda en (0,14)", c.Position())
	}
}
