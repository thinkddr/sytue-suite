// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestLosChipsDelLectorSonLaEtiquetaComun: las etiquetas del hilo son `<Tag size="xs"
// tone="muted">`: `ui.Etiqueta` densa y apagada, con el punto de su color y el aspa de la
// pieza común, que mide los 24 de WCAG 2.5.8 con ratón y 44 con el dedo (la copia local
// medía 21). En solo lectura no hay aspa.
func TestLosChipsDelLectorSonLaEtiquetaComun(t *testing.T) {
	tr := traductor(t)
	e := Etiqueta{ID: "e1", Nombre: "Proveedores", Color: "blue"}
	for _, c := range []struct {
		tactil bool
		lado   float32
	}{{false, 24}, {true, 44}} {
		func() {
			defer ui.TactilDePrueba(c.tactil)()
			quitadas := 0
			ch := nuevoChipLector(tr, e, true, func() { quitadas++ })
			test.NewTempWindow(t, ch)
			ch.Resize(ch.MinSize())
			if !ch.Densa || !ch.Apagada || ch.Punto == nil {
				t.Errorf("el chip no es el `xs muted` con punto de MessageReader.tsx: %+v", ch)
			}
			a := ch.Aspa()
			if a == nil {
				t.Fatal("con permiso para etiquetar el chip no tiene aspa")
			}
			if s := a.Size(); s.Width < c.lado || s.Height < c.lado {
				t.Errorf("táctil=%v: el aspa mide %v y la diana pide %v", c.tactil, s, c.lado)
			}
			if got, want := a.AccessibilityLabel(), tr.T("reader.removeLabel", i18n.Vars{"label": e.Nombre}); got != want {
				t.Errorf("el aspa se llama %q y debería llamarse %q", got, want)
			}
			a.AlPulsar()
			if quitadas != 1 {
				t.Errorf("pulsar el aspa quita %d veces", quitadas)
			}
		}()
	}
	if sin := nuevoChipLector(tr, e, false, func() {}); sin.Aspa() != nil {
		t.Error("en solo lectura el chip ofrece quitar la etiqueta")
	}
	// El nombre larguísimo se trunca en el `max-w-[12rem]`: no estira la fila (el resto
	// del chip, punto, huecos y bordes, son menos de 40).
	largo := nuevoChipLector(tr, Etiqueta{ID: "e2", Nombre: "Proveedores del proyecto de rehabilitación del ayuntamiento"}, false, nil)
	if w := largo.MinSize().Width; w > maxNombreChip+40 {
		t.Errorf("un nombre larguísimo pide %v de ancho y el tope del nombre es %v", w, maxNombreChip)
	}
}

// TestLosChipsDelLectorSeRehacenAlCambiarDeTema: el punto de una etiqueta sin color es
// `border-strong`, que cambia con el tema, y `ui.Etiqueta` guarda el color que le dieron
// al crearla. La barra solo se rehace cuando cambia su clave: si el tema no entra en ella,
// el punto se queda con el del tema anterior.
func TestLosChipsDelLectorSeRehacenAlCambiarDeTema(t *testing.T) {
	x := NuevoLector(traductor(t), "es")
	x.Acciones = &Acciones{Etiquetas: []Etiqueta{{ID: "e1", Nombre: "Sin color"}}}
	t.Cleanup(func() { tema.Elegir("") })
	tema.Elegir("claro")
	claro := x.claveBarra(false)
	tema.Elegir("oscuro")
	if oscuro := x.claveBarra(false); oscuro == claro {
		t.Errorf("la clave de la barra no cambia con el tema (%q): los chips no se rehacen", claro)
	}
}
