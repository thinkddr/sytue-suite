// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// LOS GUARDIANES DE LOS DIÁLOGOS. Las medidas son de la HOJA —las clases de Tailwind con
// el rem de 14—, no de un barrido de producción: estos diálogos no están en
// `test/e2e/medidas-web.mjs`. Se dice así para que nadie las lea como medidas.

func cerca(t *testing.T, que string, tengo, quiero float32) {
	t.Helper()
	if d := tengo - quiero; d > 1.0/16 || d < -1.0/16 {
		t.Errorf("%s: %v, y la hoja da %v", que, tengo, quiero)
	}
}

// TestLasFilasDeLosDialogosMidenLoDeLaHoja: la fila de «mover a» y la de exportar son
// `px-3 py-2 text-sm` (7 + 15,75 + 7), y la píldora de un estado de etiqueta es
// `rounded-control border px-2.5 py-1 text-base` (1 + 3,5 + 19,25 + 3,5 + 1).
func TestLasFilasDeLosDialogosMidenLoDeLaHoja(t *testing.T) {
	p, _ := pantallaConServicio(t, nil)
	v := tema.Claro
	fila := p.filaMover(Cuaderno{ID: "c9", Titulo: "Trabajo"}, 0, nil, v)
	cerca(t, "la fila de mover", MedidaOpcion(fila).Height, 34)
	// Y un nivel más sangra un rem entero, que es el `0.75 + nivel × 1 rem` de la web.
	hija := p.filaMover(Cuaderno{ID: "c8", Titulo: "Hija"}, 1, nil, v)
	cerca(t, "la sangría de un nivel", hija.PX-fila.PX, tema.Rem)
	if c := p.filaMover(Cuaderno{ID: "c7", Titulo: "Cifrado", Cifrado: true}, 0, nil, v); !c.Inerte {
		t.Error("un cuaderno cifrado se puede elegir como destino")
	}

	pildora := pildoraEstado(api.TagState{ID: "s1", Name: "Alta", Color: "#b0413a"}, true, nil, v)
	cerca(t, "la píldora de estado", MedidaOpcion(pildora).Height, 32)
	if pildora.FondoActivo != hexO("#b0413a", tema.Color("primary", v)) {
		t.Error("el estado puesto no lleva de fondo el color del estado")
	}
}

// TestElMasSeEnseñaAlPasarYNoParpadea: el «…» solo se ve con la tarjeta encima, y
// sigue viéndose cuando el puntero pasa de la tarjeta al propio botón —que es cuando
// Fyne le dice a la tarjeta que el puntero se ha ido—. Sin red no hay «…».
//
// VISTO EN ROJO (2026-09-22, en el frontal): sin el aviso de `botonMas`, al pasar al
// botón la tarjeta lo esconde y el test lo dice. Deshecho.
func TestElMasSeEnseñaAlPasarYNoParpadea(t *testing.T) {
	tr := trInterno(t)
	tj := NuevaTarjeta(tr, DeNota(api.Note{ID: "a", Kind: "text", Title: "Una"}))
	w := test.NewWindow(container.NewWithoutLayout(tj))
	w.SetPadded(false)
	defer w.Close()
	tj.Resize(fyne.NewSize(300, 120))
	w.Resize(fyne.NewSize(320, 140))

	if tj.mas == nil || tj.mas.Visible() {
		t.Fatal("en reposo el «…» tiene que existir y no verse")
	}
	tj.MouseIn(nil)
	if !tj.mas.Visible() {
		t.Fatal("con la tarjeta encima el «…» tiene que verse")
	}
	// Lo que hace Fyne al pasar al botón: MouseOut a la tarjeta y MouseIn al botón.
	tj.MouseOut()
	tj.mas.MouseIn(nil)
	if !tj.mas.Visible() {
		t.Error("el «…» se esconde justo al llegar a él")
	}
	tj.mas.MouseOut()
	if tj.mas.Visible() {
		t.Error("fuera de la tarjeta y del botón el «…» sigue visible")
	}

	sin := NuevaTarjeta(tr, DeNota(api.Note{ID: "b", Kind: "text"}))
	sin.SinAcciones = true
	test.WidgetRenderer(sin)
	if sin.mas != nil {
		t.Error("sin conexión la tarjeta no lleva «…»")
	}
	cerca(t, "la cabecera sin «…»", sin.altoCabecera(300), tema.TextoBase.Linea)

	// Sin conexión el título es texto suelto en el `<h3>` y PARTE en líneas: medido en la
	// web, «Cajas de cartón para la mudanza del piso de la calle Mayor» en una tarjeta de
	// 323,7 son dos líneas, 38,5. (Con conexión es el botón, que recorta: una línea.)
	//
	// VISTO EN ROJO (2026-09-23, en el frontal): con el título en `ui.Texto`, 19,25.
	largo := NuevaTarjeta(tr, DeNota(api.Note{ID: "c", Kind: "text", Title: "Cajas de cartón para la mudanza del piso de la calle Mayor"}))
	largo.SinAcciones = true
	cerca(t, "el título largo sin conexión", largo.altoCabecera(300.7), 2*tema.TextoBase.Linea)
	largo.SinAcciones = false
	largo.AlRenombrar = func() {}
	cerca(t, "el título largo con conexión", largo.altoCabecera(300.7), ui.LadoIconoSM)
}

// TestLosDialogosSePintan abre cada diálogo en una ventana de 1280×800 y comprueba lo
// que enseña. Con SYTUE_CAPTURAS=<carpeta> deja los PNG: estos diálogos no tienen
// medida de producción y la única forma de ver si están bien es mirarlos.
func TestLosDialogosSePintan(t *testing.T) {
	w := test.NewWindow(nil)
	w.SetPadded(false)
	defer w.Close()
	p := NuevaPantalla(trInterno(t), "es", nil, nil, w)
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.cuadernos = []Cuaderno{
		{ID: "c1", Titulo: "Mudanza", Color: "#86198f"},
		{ID: "c2", Titulo: "Trabajo", Color: "#0e7490"},
		{ID: "c3", Padre: "c2", Titulo: "Clientes", Color: "#0e7490"},
		{ID: "c4", Titulo: "Diario", Cifrado: true},
	}
	p.activo = p.cuadernos[0]
	estados := []api.TagState{{ID: "s1", Name: "Alta", Color: "#b0413a"}, {ID: "s2", Name: "Baja", Color: "#16794a"}}
	p.etiquetas = []api.Tag{{ID: "t1", Name: "Prioridad", States: estados}}
	puesto := []api.NoteState{{TagID: "t1", StateID: "s1", Name: "Alta", Color: "#b0413a"}}
	n := DeNota(api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Title: "Presupuesto", States: puesto})

	casos := []struct {
		nombre string
		abrir  func()
		textos []string
	}{
		{"ajustes-nuevo", func() { p.dialogoAjustes(nil, "") }, []string{
			p.tr.T("sidebar.newNotebook", nil), p.tr.T("layout.tree", nil), p.tr.T("visibility.private.hint", nil),
			p.tr.T("index.label", nil), p.tr.T("crypto.title", nil), p.tr.T("common.save", nil),
		}},
		{"ajustes-cuaderno", func() { c := p.cuadernos[0]; p.dialogoAjustes(&c, "") }, []string{
			p.tr.T("notebook.settings", nil), p.tr.T("notebook.delete", nil),
		}},
		{"mover", func() { p.dialogoMover(n) }, []string{
			p.tr.T("move.title", nil), "Trabajo", "Clientes", p.tr.T("move.encrypted", nil),
		}},
		{"exportar", func() { p.dialogoExportarNota(n) }, []string{
			p.tr.T("note.export", nil), p.tr.T("export.html", nil), "ONEX",
		}},
		{"etiquetas", func() { p.aplicarEtiquetas(n) }, []string{
			p.tr.T("tags.apply", nil), "Prioridad", "Alta", "Baja", p.tr.T("common.close", nil),
		}},
		{"retroenlaces", func() { p.dialogoRetroenlaces(n) }, []string{
			p.tr.T("backlinks.title", nil),
		}},
		{"clave-abrir", func() { p.dialogoClave(false, nil, func(string) error { return nil }) }, []string{
			p.tr.T("crypto.unlock", nil), p.tr.T("crypto.unlockHint", nil), p.tr.T("crypto.passphrase", nil),
		}},
		{"clave-crear", func() { p.dialogoClave(true, nil, func(string) error { return nil }) }, []string{
			p.tr.T("crypto.title", nil), p.tr.T("crypto.passphraseRepeat", nil), p.tr.T("crypto.enable", nil),
		}},
		{"importar", func() {
			p.dialogoImportar([]byte("{}"), api.VistaPreviaVolcado{
				Notebook: api.Notebook{Title: "Mudanza vieja"}, Descartadas: 2,
				Notas: []api.Note{
					{ID: "a", Kind: "text", Title: "Cajas", Content: "<p>Veinte <b>grandes</b>.</p>"},
					{ID: "b", Kind: "text", Title: "Llaves", ContentCipher: "encv1:xx"},
				},
			})
		}, []string{
			p.tr.T("import.label", nil), "Mudanza vieja", "Cajas", p.tr.T("crypto.locked", nil),
			p.tr.T("import.action", nil),
		}},
		{"etiquetas-lista", p.dialogoEtiquetas, []string{
			p.tr.T("tags.title", nil), "Prioridad", "Alta", "Baja", p.tr.T("notebook.rename", nil),
			p.tr.T("tags.new", nil),
		}},
		{"etiquetas-editar", func() { p.abrirEtiquetas(borradorDe(p.etiquetas[0])) }, []string{
			p.tr.T("tags.title", nil), p.tr.T("tags.states", nil), p.tr.T("tags.addState", nil),
			p.tr.T("common.save", nil), p.tr.T("common.cancel", nil),
		}},
		// La herramienta de enlaces con «cajas» escrito: la nota que lo contiene en su
		// título y la que lo tiene en un título de dentro, con su «›».
		{"enlace", func() {
			p.notas = DeNotas([]api.Note{
				{ID: "n1", NotebookID: "c1", Kind: "text", Title: "Presupuesto", Content: "<h2>Cajas grandes</h2><p>x</p>"},
				{ID: "n2", NotebookID: "c1", Kind: "text", Title: "Cajas", Content: "<p>Veinte.</p>"},
			})
			p.lienzo.Fase = Exito
			p.Repintar()
			nota, _ := p.nota("n1")
			p.abrirEditor(nota)
			p.herramientaEnlace(p.edicion.editor, p.edicion.editor)
			recorrerInterno(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
				if e, ok := o.(*ui.Entrada); ok {
					e.PonerTexto("cajas")
				}
			})
		}, []string{
			p.tr.T("editor.linkThisNotebook", nil), "Cajas", "Presupuesto", " › Cajas grandes",
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			c.abrir()
			defer func() {
				for _, o := range w.Canvas().Overlays().List() {
					w.Canvas().Overlays().Remove(o)
				}
			}()
			capa := w.Canvas().Overlays().Top()
			if capa == nil {
				t.Fatal("el diálogo no se ha abierto")
			}
			vistos := map[string]bool{}
			recorrerInterno(capa, func(o fyne.CanvasObject) {
				if x, ok := o.(*canvas.Text); ok {
					vistos[x.Text] = true
				}
			})
			for _, s := range c.textos {
				if !vistos[s] {
					t.Errorf("el diálogo no enseña %q", s)
				}
			}
			capturar(t, w, c.nombre)
		})
	}
}

// TestGuardarSinNombreNoSePuede: el botón de guardar un cuaderno nuevo nace apagado y
// se enciende al escribir el nombre: el `disabled` de la web con el título en blanco.
func TestGuardarSinNombreNoSePuede(t *testing.T) {
	w := test.NewWindow(nil)
	w.SetPadded(false)
	defer w.Close()
	p := NuevaPantalla(trInterno(t), "es", nil, nil, w)
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.dialogoAjustes(nil, "")
	defer p.cerrarAjustes()

	var guardar *ui.Boton
	var titulo *ui.Entrada
	recorrerInterno(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && b.Texto == p.tr.T("common.save", nil) {
			guardar = b
		}
		if e, ok := o.(*ui.Entrada); ok && titulo == nil && p.ajustes != nil && p.ajustes.Foco == e {
			titulo = e
		}
	})
	if guardar == nil || titulo == nil {
		t.Fatal("no están el botón de guardar o el campo del nombre")
	}
	if !guardar.Inerte {
		t.Error("sin nombre se puede guardar")
	}
	titulo.PonerTexto("Recetas")
	if guardar.Inerte {
		t.Error("con nombre sigue sin poder guardarse")
	}
	titulo.PonerTexto("   ")
	if !guardar.Inerte {
		t.Error("un nombre en blanco no es un nombre")
	}
}

func recorrerInterno(o fyne.CanvasObject, f func(fyne.CanvasObject)) {
	if o == nil {
		return
	}
	f(o)
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			recorrerInterno(h, f)
		}
	case *container.Scroll:
		recorrerInterno(x.Content, f)
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(x).Objects() {
			recorrerInterno(h, f)
		}
	}
}

func capturar(t *testing.T, w fyne.Window, nombre string) {
	t.Helper()
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		return
	}
	img := w.Canvas().Capture()
	escribirPNG(t, filepath.Join(dir, "notes-"+nombre+".png"), img)
}

func escribirPNG(t *testing.T, ruta string, img image.Image) {
	t.Helper()
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
