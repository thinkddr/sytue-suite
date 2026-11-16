// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
)

// documentoLargo es un documento de `n` A4 sin texto leído.
func documentoLargo(n int) *Contenido {
	c := &Contenido{Total: n, Paginas: make([]Pagina, n)}
	for i := range c.Paginas {
		c.Paginas[i] = a4()
		c.Paginas[i].Numero = i + 1
	}
	return c
}

// miniaturaVisible dice si la fila de la página `n` cae entera dentro de lo que enseña
// el desplazador del panel.
func miniaturaVisible(p *PanelLateral, n int) bool {
	s, f := p.desplazador, p.miniaturas[n]
	arriba := f.Position().Y + rellenoPanel
	return arriba >= s.Offset.Y && arriba+f.Size().Height <= s.Offset.Y+s.Size().Height
}

// TestLaMiniaturaSigueALaPaginaQueSeLee: con 50 páginas, ir a la 40 deja su miniatura a
// la vista (`scrollIntoView({ block: 'nearest' })` de `Miniaturas` en `Paneles.tsx`), y
// volver a una que ya se ve no mueve la tira.
func TestLaMiniaturaSigueALaPaginaQueSeLee(t *testing.T) {
	p := NuevoPanelLateral(traductorES(t), documentoLargo(50))
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(AnchoPanel, 600))
	if p.desplazador.Offset.Y != 0 {
		t.Fatalf("el panel abre desplazado (y=%g)", p.desplazador.Offset.Y)
	}
	p.PaginaActual = 40
	p.RefrescarActual()
	if !miniaturaVisible(p, 40) {
		t.Fatalf("ir a la 40 deja su miniatura fuera (y=%g)", p.desplazador.Offset.Y)
	}
	// `nearest`: entra por abajo, así que queda pegada al borde de abajo, no arriba.
	f := p.miniaturas[40]
	if quiero := f.Position().Y + rellenoPanel + f.Size().Height - p.desplazador.Size().Height; p.desplazador.Offset.Y != quiero {
		t.Errorf("la 40 deja la tira en y=%g; pegada abajo sería %g", p.desplazador.Offset.Y, quiero)
	}
	// La 39 ya se ve: la tira no se mueve.
	antes := p.desplazador.Offset.Y
	p.PaginaActual = 39
	p.RefrescarActual()
	if p.desplazador.Offset.Y != antes {
		t.Errorf("ir a una miniatura que ya se ve mueve la tira de y=%g a y=%g", antes, p.desplazador.Offset.Y)
	}
	// Volver a la 2 la trae por arriba.
	p.PaginaActual = 2
	p.RefrescarActual()
	if !miniaturaVisible(p, 2) {
		t.Errorf("volver a la 2 la deja fuera (y=%g)", p.desplazador.Offset.Y)
	}
	// En otro panel no hay miniaturas que seguir, y no se toca nada.
	p.Abrir(PanelBuscar)
	p.PaginaActual = 45
	p.RefrescarActual()
	if p.desplazador.Offset.Y != 0 {
		t.Errorf("con Buscar abierto, cambiar de página desplaza el panel (y=%g)", p.desplazador.Offset.Y)
	}
}

// TestIrALlevaLaMiniaturaALaVista es lo mismo con la pantalla entera: el «Ir a» de la
// barra mueve el documento y, con él, la tira de Páginas.
func TestIrALlevaLaMiniaturaALaVista(t *testing.T) {
	p := NuevaPantalla(traductorES(t))
	p.Estado = editor.Listo
	p.Contenido = documentoLargo(50)
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(1100, 700))
	if p.panel == nil || p.panel.desplazador == nil {
		t.Skip("la pantalla no enseña el panel a este ancho")
	}
	p.IrA(40)
	if !miniaturaVisible(p.panel, 40) {
		t.Errorf("IrA(40) deja la miniatura de la 40 fuera (y=%g)", p.panel.desplazador.Offset.Y)
	}
}

// TestVolverAPaginasEnseñaLaMiniaturaActual: la lista se monta de nuevo al volver a
// la pestaña Páginas, y como el `useEffect` de `Miniaturas` al montarse, la de la
// página que se lee tiene que quedar a la vista; al volver a leer el documento tras un
// cambio la tira no salta al principio.
func TestVolverAPaginasEnseñaLaMiniaturaActual(t *testing.T) {
	p := NuevoPanelLateral(traductorES(t), documentoLargo(50))
	w := ventanaSinRelleno(p.Objeto())
	defer w.Close()
	w.Resize(fyne.NewSize(AnchoPanel, 600))
	p.Abrir(PanelBuscar)
	p.PaginaActual = 40
	p.RefrescarActual()
	p.Abrir(PanelPaginas)
	if !miniaturaVisible(p, 40) {
		t.Fatalf("volver a Páginas en la 40 deja su miniatura fuera (y=%g)", p.desplazador.Offset.Y)
	}
	antes := p.desplazador.Offset.Y
	p.PonerContenido(documentoLargo(50))
	if p.desplazador.Offset.Y != antes || !miniaturaVisible(p, 40) {
		t.Errorf("volver a leer el documento mueve la tira de y=%g a y=%g", antes, p.desplazador.Offset.Y)
	}
}
