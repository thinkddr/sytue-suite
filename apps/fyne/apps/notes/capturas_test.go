// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// LAS CAPTURAS DE LA APP ENTERA, en escritorio (1280×800) y en móvil (360×740), que son
// los dos anchos contra los que se compara la web. Sin `SYTUE_CAPTURAS` no guardan nada
// y solo comprueban que la pantalla pinta.

// cuadernoDePrueba es un cuaderno con notas de todo tipo: una con hijas, una fijada,
// una tarea hecha y una con estado de etiqueta.
func cuadernoDePrueba(vista string) (Cuaderno, []api.Note) {
	grupo := "n1"
	ayer := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	c := DeCuaderno(api.Notebook{ID: "c1", Title: "Mudanza", Color: "#86198f", Layout: vista, NoteCount: 4})
	notas := []api.Note{
		{
			ID: "n1", NotebookID: "c1", Kind: "text", Title: "Presupuesto", UpdatedAt: ayer,
			Content: "<p>Pedir tres presupuestos a empresas de mudanza.</p><ul><li>Cajas</li><li>Camión</li></ul>",
			States:  []api.NoteState{{TagID: "t1", StateID: "s1", Name: "Alta", TagName: "Prioridad", Color: "#b0413a"}},
		},
		{ID: "n2", NotebookID: "c1", Kind: "text", Title: "Cajas", GroupID: &grupo, UpdatedAt: ayer, Content: "<p>Veinte grandes.</p>", Col: 1},
		{ID: "n3", NotebookID: "c1", Kind: "todo", Title: "Llamar al casero", Checked: true, UpdatedAt: ayer, Col: 1},
		{ID: "n4", NotebookID: "c1", Kind: "text", Title: "Direcciones", Pinned: true, UpdatedAt: ayer, Content: "<p><b>Nueva:</b> calle Mayor 3</p>", Col: 2},
	}
	return c, notas
}

// montarCaptura arma la pantalla en una ventana de prueba con ese cuaderno abierto.
func montarCaptura(t *testing.T, ancho, alto float32, vista string) (*Pantalla, fyne.Window) {
	t.Helper()
	w := test.NewWindow(nil)
	w.SetPadded(false)
	t.Cleanup(w.Close)
	// El tamaño ANTES de montar: la pantalla se arma según el corte de la ventana.
	w.Resize(fyne.NewSize(ancho, alto))
	p := NuevaPantalla(trInterno(t), "es", nil, nil, w)
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(ancho, alto))
	c, notas := cuadernoDePrueba(vista)
	otro := DeCuaderno(api.Notebook{ID: "c2", Title: "Trabajo", Color: "#0e7490", NoteCount: 9})
	cifrado := DeCuaderno(api.Notebook{ID: "c3", Title: "Diario", Color: "#3a4a5e", Encrypted: true, NoteCount: 2})
	p.cuadernos, p.activo = []Cuaderno{c, otro, cifrado}, c
	p.lateral.Fase = Exito
	p.notas, p.lienzo.Fase = DeNotas(notas), Exito
	p.etiquetas = []api.Tag{{ID: "t1", Name: "Prioridad", States: []api.TagState{
		{ID: "s1", Name: "Alta", Color: "#b0413a", Shortcut: "1"},
		{ID: "s2", Name: "Baja", Color: "#16794a", Shortcut: "2"},
	}}}
	return p, w
}

func TestLasCapturasDeLaApp(t *testing.T) {
	casos := []struct {
		nombre      string
		ancho, alto float32
		vista       string
		preparar    func(p *Pantalla)
	}{
		{"escritorio-arbol", 1280, 800, "tree", func(p *Pantalla) { p.lienzo.Seleccionada = "n1" }},
		{"escritorio-arbol-sin-nota", 1280, 800, "tree", nil},
		{"escritorio-columnas", 1280, 800, "columns", nil},
		{"escritorio-lista", 1280, 800, "list", func(p *Pantalla) { p.lienzo.Seleccionada = "n4" }},
		{"escritorio-libre", 1280, 800, "free", nil},
		// La de la web `columnas-parcial`: un título que no cabe (con conexión recorta) y
		// una tabla en el cuerpo.
		{"escritorio-parcial", 1280, 800, "columns", func(p *Pantalla) {
			c := p.notas[1].Crudo
			c.Title = "Cajas de cartón para la mudanza del piso de la calle Mayor"
			c.Content = `<p>Recuento:</p><table><tbody><tr><th>Cuarto</th><th>Cajas</th></tr><tr><td>Cocina</td><td>12</td></tr>` +
				`<tr><td>Salón y comedor grande</td><td>7</td></tr></tbody></table><p>Fin.</p>`
			p.notas[1] = DeNota(c)
		}},
		// Una nota de imagen, con la imagen ya bajada (un degradado de 400 × 240).
		{"escritorio-imagen", 1280, 800, "columns", func(p *Pantalla) {
			img := image.NewNRGBA(image.Rect(0, 0, 400, 240))
			for y := range 240 {
				for x := range 400 {
					img.Set(x, y, color.NRGBA{R: uint8(x * 255 / 400), G: 120, B: uint8(y * 255 / 240), A: 255})
				}
			}
			const url = "https://fotos.example.eu/plano.png"
			p.imagenes = map[string]imagenRemota{url: {img: img, lista: true}}
			p.notas = append(p.notas, DeNota(api.Note{
				ID: "n5", NotebookID: "c1", Kind: "image", Title: "Plano",
				Data: json.RawMessage(`{"url":"` + url + `"}`), Col: 2,
			}))
		}},
		// Una nota `doc`, con el documento ya pedido a Docs: la de la web `columnas-doc`.
		{"escritorio-doc", 1280, 800, "columns", func(p *Pantalla) {
			p.docs = map[string]docRemoto{"d1": {listo: true, html: "<h1>Acta de la reunión</h1>" +
				"<p>Asistentes: Ana, Luis y Marta.</p><ul><li>Presupuesto de la mudanza</li><li>Fechas</li></ul>" +
				strings.Repeat("<p>Una línea más del acta para que haya que desplazar.</p>", 8)}}
			p.notas = append(p.notas, DeNota(api.Note{
				ID: "n5", NotebookID: "c1", Kind: "doc", Title: "Acta", Col: 2,
				Links: []api.NoteLink{{TargetKind: "doc", TargetID: "d1", Label: "Acta", URL: "https://docs.sytue.eu/d/d1"}},
			}))
		}},
		{"escritorio-adjuntos", 1280, 800, "columns", func(p *Pantalla) {
			c := p.notas[3].Crudo
			c.Data = json.RawMessage(`{"attachments":[` +
				`{"id":"a1","name":"contrato-de-alquiler-firmado-2026.pdf","size":248312,"type":"application/pdf","dataUrl":"data:application/pdf;base64,AA=="},` +
				`{"id":"a2","name":"plano.png","size":900,"type":"image/png","dataUrl":"data:image/png;base64,AA=="}]}`)
			p.notas[3] = DeNota(c)
		}},
		// «Configurar icono…» de la nota con la chincheta puesta: la web `icono`.
		{"escritorio-icono", 1280, 800, "columns", func(p *Pantalla) { conIconoPin(p); p.dialogoIcono(p.notas[3]) }},
		{"movil-icono", 360, 740, "columns", func(p *Pantalla) { conIconoPin(p); p.dialogoIcono(p.notas[3]) }},
		{"escritorio-vacio", 1280, 800, "list", func(p *Pantalla) { p.notas, p.lienzo.Fase = nil, Vacia }},
		{"escritorio-papelera", 1280, 800, "list", func(p *Pantalla) {
			p.vista, p.papelera.Fase = VistaPapelera, Exito
			p.papelera.Notas = DeNotas([]api.Note{{ID: "x1", Title: "Borrador viejo"}, {ID: "x2"}})
		}},
		{"escritorio-busqueda", 1280, 800, "list", func(p *Pantalla) {
			p.vista, p.busqueda.Fase = VistaBusqueda, Exito
			p.busqueda.Resultados = []api.NoteHit{
				{ID: "n1", NotebookID: "c1", Title: "Presupuesto", Snippet: "Pedir tres presupuestos a empresas de mudanza."},
				{ID: "n9", NotebookID: "c2", Notebook: "Trabajo", Snippet: "Sin título, sale el cuaderno."},
			}
		}},
		{"escritorio-editando", 1280, 800, "columns", func(p *Pantalla) {
			n, _ := p.nota("n1")
			p.abrirEditor(n)
			p.edicion.editor.Orden("b", "")
			teclear(p.edicion.editor.zona, " urgente")
		}},
		{"escritorio-editando-ancha", 1280, 800, "columns", func(p *Pantalla) {
			n, _ := p.nota("n2")
			p.abrirEditor(n)
			p.lienzo.AlEnsanchar()
		}},
		{"escritorio-arbol-editando", 1280, 800, "tree", func(p *Pantalla) {
			n, _ := p.nota("n1")
			p.abrirEditor(n)
		}},
		// Sin red, con dos capturas en la cola: la web `sin-red`.
		{"escritorio-sin-conexion", 1280, 800, "list", conColaSinRed},
		{"movil-sin-conexion", 360, 740, "list", conColaSinRed},
		{"movil-arbol", 360, 740, "tree", nil},
		{"movil-arbol-nota", 360, 740, "tree", func(p *Pantalla) { p.lienzo.Seleccionada = "n1" }},
		{"movil-columnas", 360, 740, "columns", nil},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			p, w := montarCaptura(t, c.ancho, c.alto, c.vista)
			if c.preparar != nil {
				c.preparar(p)
			}
			p.Repintar()
			img := w.Canvas().Capture()
			if img.Bounds().Dx() == 0 {
				t.Fatal("la captura ha salido vacía")
			}
			if dir := os.Getenv("SYTUE_CAPTURAS"); dir != "" {
				capturar(t, w, c.nombre)
			}
		})
	}
}

// conIconoPin le pone a «Direcciones» el icono de la chincheta.
func conIconoPin(p *Pantalla) {
	c := p.notas[3].Crudo
	c.Data = json.RawMessage(`{"icon":"pin"}`)
	p.notas[3] = DeNota(c)
}

// conColaSinRed corta la red y deja en la cola del cuaderno abierto una captura
// pendiente y otra que el servicio rechazó.
func conColaSinRed(p *Pantalla) {
	p.red = &offline.Monitor{}
	p.red.Report(offline.Offline)
	p.cola = []capturaPendiente{
		{ClientID: "a", Cuaderno: "c1", In: api.NoteInput{Kind: "text", Content: "<p>Llamar a Marta para la reunión del jueves</p>"}},
		{ClientID: "b", Cuaderno: "c1", In: api.NoteInput{Kind: "todo", Title: "Comprar el regalo de Lucía"}, Rechazada: true},
	}
}
