// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// veloSoltarDe dice si lo que pinta la ventana lleva el velo de soltar: el texto
// «Suelta para subir aquí» y su borde discontinuo `primary` de 2 px y `rounded-xl`.
func veloSoltarDe(t *testing.T, p *Pantalla, o fyne.CanvasObject) (texto, borde bool) {
	t.Helper()
	// El armazón monta su contenido al colocarse: se pinta antes de mirar.
	_ = p.win.Canvas().Capture()
	quiere := p.tr.T("files.dropHere", nil)
	recorrer(o, func(x fyne.CanvasObject) {
		switch x := x.(type) {
		case *ui.Rotulo:
			if x.Texto == quiere && x.Visible() {
				texto = true
			}
		case *ui.BordeDiscontinuo:
			if x.Token == "primary" && x.Grosor == GrosorVeloSoltar && x.Radio == RadioVeloSoltar {
				borde = true
			}
		}
	})
	return texto, borde
}

// TestArrastrarFicherosEnciendeElVeloDeSoltar: con ficheros del sistema por encima de la
// ventana, «Mi unidad» se tapa con «Suelta para subir aquí» y su borde discontinuo
// (FileBrowser.tsx); al salir el arrastre se destapa. En la papelera, en otra sección o
// sin red no aparece, que es donde soltar no sube nada.
func TestArrastrarFicherosEnciendeElVeloDeSoltar(t *testing.T) {
	w := test.NewWindow(nil)
	defer w.Close()
	p := NuevaPantalla(traductor(t), nil, nil, w)
	p.explor.Fase = Exito
	p.explor.Elementos = []Elemento{{ID: "f1", Nombre: "informe.pdf", Tamano: 1024}}
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))

	if texto, borde := veloSoltarDe(t, p, w.Content()); texto || borde {
		t.Fatal("sin arrastre ya está el velo de soltar")
	}
	p.Arrastrando(true)
	if texto, borde := veloSoltarDe(t, p, w.Content()); !texto || !borde {
		t.Fatalf("arrastrando ficheros sobre «Mi unidad» no sale el velo (texto %v, borde %v)", texto, borde)
	}
	p.Arrastrando(false)
	if texto, _ := veloSoltarDe(t, p, w.Content()); texto {
		t.Error("el velo sigue a la vista al salir el arrastre")
	}

	for _, s := range []Seccion{Papelera, Recientes} {
		p.irA(s)
		p.Arrastrando(true)
		if texto, _ := veloSoltarDe(t, p, w.Content()); texto {
			t.Errorf("en la sección %v sale el velo de soltar", s)
		}
		p.Arrastrando(false)
	}

	p.irA(MiUnidad)
	p.explor.Fase = Exito
	p.conexion(false)
	p.Arrastrando(true)
	if texto, _ := veloSoltarDe(t, p, w.Content()); texto {
		t.Error("sin red sale el velo de soltar")
	}
	// Vuelve la red con el arrastre encima: el velo aparece, que soltar ya subiría.
	p.explor.Desconectado = false
	p.Repintar()
	if texto, _ := veloSoltarDe(t, p, w.Content()); !texto {
		t.Error("con red otra vez y el arrastre encima no sale el velo")
	}
}

// TestElVeloDeSoltarTapaElCuerpoConSuMargen: el velo cubre el cuerpo menos su `inset-2`,
// así que la cabecera con «Subir» sigue a la vista y el velo no se sale por ningún lado.
func TestElVeloDeSoltarTapaElCuerpoConSuMargen(t *testing.T) {
	tr := traductor(t)
	e := NuevoExplorador(tr)
	e.Fase, e.Ventana, e.Arrastrando = Exito, 1280, true
	e.Elementos = []Elemento{{ID: "f1", Nombre: "informe.pdf", Tamano: 1024}}
	w := test.NewWindow(e.Objeto(tema.Claro))
	defer w.Close()
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1000, 700))
	_ = w.Canvas().Capture()

	var borde *ui.BordeDiscontinuo
	recorrer(w.Content(), func(x fyne.CanvasObject) {
		if b, ok := x.(*ui.BordeDiscontinuo); ok && b.Token == "primary" {
			borde = b
		}
	})
	if borde == nil {
		t.Fatal("no está el borde del velo")
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(borde)
	tam := borde.Size()
	if pos.X != MargenVeloSoltar || pos.Y <= MargenVeloSoltar {
		t.Errorf("el velo empieza en %v: debía ir a %v del borde y por debajo de la cabecera", pos, MargenVeloSoltar)
	}
	if pos.X+tam.Width != 1000-MargenVeloSoltar || pos.Y+tam.Height != 700-MargenVeloSoltar {
		t.Errorf("el velo acaba en %v+%v: debía quedarse a %v del borde", pos, tam, MargenVeloSoltar)
	}
}
