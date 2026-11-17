// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// teselasDe recoge las miniaturas de la pantalla, en el orden en que se ven.
func teselasDe(t *testing.T, o fyne.CanvasObject) []*Miniatura {
	var out []*Miniatura
	var ir func(fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *Miniatura:
			out = append(out, x)
		case *fyne.Container:
			for _, h := range x.Objects {
				ir(h)
			}
		case *container.Scroll:
			ir(x.Content)
		case *container.ThemeOverride:
			ir(x.Content)
		case fyne.Widget:
			for _, h := range test.TempWidgetRenderer(t, x).Objects() {
				ir(h)
			}
		}
	}
	ir(o)
	return out
}

// TestMayusMarcaElTramoDeLaGaleria es el criterio de la pieza: marcar la foto 3 y hacer
// Mayús + clic en la 10 deja marcadas de la 3 a la 10, aunque crucen meses.
func TestMayusMarcaElTramoDeLaGaleria(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(12), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	mayus := false
	antes := mayusPulsada
	mayusPulsada = func() bool { return mayus }
	t.Cleanup(func() { mayusPulsada = antes })

	f := a.g.fotos
	a.entrarEnSeleccion()
	a.alternar(f[2])
	mayus = true
	a.alternar(f[9])
	for i, x := range f {
		if quiero := i >= 2 && i <= 9; a.g.seleccion[x.ID] != quiero {
			t.Errorf("la foto %d marcada = %v, y tocaba %v", i+1, a.g.seleccion[x.ID], quiero)
		}
	}
	// El ancla sigue en la 3: otro Mayús + clic, ahora en la 1, añade la 1 y la 2.
	a.alternar(f[0])
	if !a.g.seleccion[f[0].ID] || !a.g.seleccion[f[1].ID] || len(a.g.seleccion) != 10 {
		t.Errorf("Mayús + clic hacia atrás deja %d marcadas", len(a.g.seleccion))
	}
	// Salir y volver olvida el ancla: Mayús + clic sin ancla marca solo esa.
	a.salirDeSeleccion()
	a.entrarEnSeleccion()
	a.alternar(f[5])
	if len(a.g.seleccion) != 1 || !a.g.seleccion[f[5].ID] {
		t.Errorf("sin ancla, Mayús + clic deja %v", a.g.seleccion)
	}
}

// TestMantenerPulsadaEmpiezaASeleccionar: mantener pulsada una tesela (TappedSecondary,
// que es como Fyne entrega el gesto en el móvil) entra en el modo selección con esa
// foto marcada y SIN abrir el visor; y esa foto es el ancla de un Mayús + clic.
func TestMantenerPulsadaEmpiezaASeleccionar(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(6), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	teselas := teselasDe(t, a.pantallaGaleria())
	if len(teselas) != 6 {
		t.Fatalf("la galería pinta %d teselas y tocaban 6", len(teselas))
	}
	m := teselas[1]
	m.TappedSecondary(&fyne.PointEvent{})
	if a.vis != nil {
		t.Error("mantener pulsada abrió el visor")
	}
	if a.g.seleccion == nil || len(a.g.seleccion) != 1 || !a.g.seleccion[m.Foto.ID] {
		t.Fatalf("mantener pulsada deja la selección en %v", a.g.seleccion)
	}
	// Dentro del modo, la tesela marca y el gesto ya no hace nada más.
	teselas = teselasDe(t, a.pantallaGaleria())
	if !teselas[1].Seleccionando || !teselas[1].Seleccionada || teselas[1].AlMantener != nil {
		t.Errorf("tras mantener, la tesela no está marcada en modo selección")
	}
	antes := mayusPulsada
	mayusPulsada = func() bool { return true }
	t.Cleanup(func() { mayusPulsada = antes })
	a.alternar(a.g.fotos[4])
	if len(a.g.seleccion) != 4 {
		t.Errorf("Mayús + clic desde la mantenida deja %d marcadas y tocaban 4", len(a.g.seleccion))
	}
}

// TestElAnclaSigueALaFotoSiLaListaCambia: el ancla es la FOTO, no su posición. Si en pleno
// modo selección sale una foto de antes (quitar una favorita en su vista, por ejemplo), el
// tramo de Mayús + clic sigue empezando en la misma foto y no en la que ocupa su sitio.
func TestElAnclaSigueALaFotoSiLaListaCambia(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(8), porPagina: 100}
	a, _ := appDePrueba(t, s)
	a.Arrancar()
	antes := mayusPulsada
	t.Cleanup(func() { mayusPulsada = antes })
	mayusPulsada = func() bool { return false }

	f := append([]api.Photo(nil), a.g.fotos...)
	a.entrarEnSeleccion()
	a.alternar(f[3])
	a.g.fotos = quitarFoto(a.g.fotos, f[0].ID)
	mayusPulsada = func() bool { return true }
	a.alternar(f[1])
	for i, x := range f {
		if quiero := i >= 1 && i <= 3; a.g.seleccion[x.ID] != quiero {
			t.Errorf("la foto %d marcada = %v, y tocaba %v", i+1, a.g.seleccion[x.ID], quiero)
		}
	}
}
