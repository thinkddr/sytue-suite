// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image/color"
	"math"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// LOS GUARDIANES DEL EDITOR: escribir de verdad (letras, teclas y atajos que llegan al
// cuerpo como llegarían del teclado) y lo que se manda al servicio.

func teclear(z *zonaEdicion, s string) {
	for _, r := range s {
		z.TypedRune(r)
	}
}

// TestEscribirUnaNotaLaGuardaAlSalir: se abre el editor, se escribe y al salir llega el
// PUT con el HTML nuevo, que conserva la forma del párrafo.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin `guardarEdicion` en `cerrarEditor`, salir
// no manda nada y lo escrito se pierde.
func TestEscribirUnaNotaLaGuardaAlSalir(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"PUT /notes/n1": `{"id":"n1","notebook_id":"c1","kind":"text","content":"<p>hola mundo</p>"}`,
	}, api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Content: "<p>hola</p>"})
	n, _ := p.nota("n1")
	p.abrirEditor(n)
	if p.edicion == nil {
		t.Fatal("pulsar una nota de texto no abre el editor")
	}
	ed := p.edicion.editor
	teclear(ed.zona, " mundo")
	if got := ed.HTML(); got != "<p>hola mundo</p>" {
		t.Fatalf("el editor tiene %q", got)
	}
	p.cerrarEditor()
	for i := 0; i < 200 && len(falso.rutas()) == 0; i++ {
		fyne.DoAndWait(func() {}) // el guardado pinta antes de mandar: que el test lo deje pasar
		time.Sleep(10 * time.Millisecond)
	}
	if c := falso.ultima(t, "PUT /notes/n1"); c["content"] != "<p>hola mundo</p>" {
		t.Errorf("al salir se manda %v", c)
	}
	if p.lienzo.Editor != nil {
		t.Error("al salir la tarjeta sigue con el editor")
	}
}

// TestLasTeclasDelEditor: Intro parte, Retroceso junta, Mayús+Intro es un salto de línea,
// «# » es un título y la negrita sin selección es para lo que se escriba.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin los atajos de Markdown, «## » se queda
// escrito tal cual.
func TestLasTeclasDelEditor(t *testing.T) {
	ed := NuevoEditorNota(trInterno(t), Leer("<p>uno</p>"), "")
	z := ed.zona
	z.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	teclear(z, "dos")
	if got := ed.HTML(); got != "<p>uno</p><p>dos</p>" {
		t.Errorf("Intro deja %q", got)
	}
	z.TypedKey(&fyne.KeyEvent{Name: fyne.KeyHome})
	z.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	if got := ed.HTML(); got != "<p>unodos</p>" {
		t.Errorf("Retroceso al principio deja %q", got)
	}
	z.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	z.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	z.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	if got := ed.HTML(); got != "<p>uno<br/><br/>dos</p>" && got != "<p>uno<br/>dos</p>" {
		t.Errorf("Mayús+Intro deja %q", got)
	}

	md := NuevoEditorNota(trInterno(t), Leer(""), "")
	teclear(md.zona, "## Plan")
	if got := md.HTML(); got != "<h2>Plan</h2>" {
		t.Errorf("«## » deja %q", got)
	}
	md.zona.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	md.Orden("b", "")
	teclear(md.zona, "ya")
	if got := md.HTML(); got != "<h2>Plan</h2><div><b>ya</b></div>" {
		t.Errorf("negrita sin selección deja %q", got)
	}
	md.zona.TypedShortcut(&fyne.ShortcutUndo{})
	if strings.Contains(md.HTML(), "ya") {
		t.Errorf("deshacer no quita lo escrito: %q", md.HTML())
	}
}

// TestLaCasillaDeUnaTareaSeMarcaEscribiendo: pulsar la casilla en el cuerpo la marca.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin invertir la marca, la casilla no cambia.
func TestLaCasillaDeUnaTareaSeMarcaEscribiendo(t *testing.T) {
	ed := NuevoEditorNota(trInterno(t), Leer(`<div class="task-row"><input type="checkbox"/> <span>pan</span></div>`), "")
	w := test.NewWindow(ed)
	defer w.Close()
	w.Resize(fyne.NewSize(400, 200))
	var casilla Adorno
	for _, a := range ed.zona.maquetado().Adornos {
		if a.Tipo == adornoCasilla {
			casilla = a
		}
	}
	ed.zona.Tapped(&fyne.PointEvent{Position: fyne.NewPos(PXZonaEd+casilla.X+4, PYZonaEd+casilla.Y+4)})
	if got := ed.HTML(); !strings.Contains(got, `checked`) {
		t.Errorf("pulsar la casilla deja %q", got)
	}
}

// TestElAutoguardadoEsperaLoDeLaWeb: 400 ms desde la última tecla y, escribiendo sin
// parar, como mucho 2 s desde la primera sin guardar.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con un techo de 3 s, a 1,8 s espera 400 ms.
func TestElAutoguardadoEsperaLoDeLaWeb(t *testing.T) {
	t0 := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		ahora  time.Duration
		quiero time.Duration
	}{{0, 400 * time.Millisecond}, {1800 * time.Millisecond, 200 * time.Millisecond}, {3 * time.Second, 0}} {
		if got := EsperaDeAutoguardado(t0, t0.Add(c.ahora)); got != c.quiero {
			t.Errorf("a %v del primer cambio espera %v y la web %v", c.ahora, got, c.quiero)
		}
	}
}

// TestLosEnlacesANotasVanALaNota: al guardar, los `links[]` de nota salen del texto y los
// de otras cosas se quedan.
//
// VISTO EN ROJO (2026-09-23, en el frontal): conservando también los de nota, el enlace
// borrado del texto («viejo») seguía en el retroenlace.
func TestLosEnlacesANotasVanALaNota(t *testing.T) {
	app := func(ruta string) string { return "https://notes.sytue.eu" + ruta }
	d := Leer(`<p>ver <a href="https://notes.sytue.eu/?notebook=c1&amp;note=n9" target="_blank" rel="noopener noreferrer">la lista</a></p>`)
	n := api.Note{Links: []api.NoteLink{{TargetKind: "drive_file", TargetID: "f1"}, {TargetKind: "note", TargetID: "viejo"}}}
	got := enlacesSincronizados(n, d, app)
	if len(got) != 2 || got[0].TargetKind != "drive_file" || got[1].TargetID != "n9" || got[1].Label != "la lista" {
		t.Errorf("los enlaces son %+v", got)
	}
}

// TestUnCuadernoCifradoSeLeeYSeEscribeCifrado: con la clave, las notas se ven en claro y
// lo que se escribe viaja DENTRO del criptograma (título y cuerpo vacíos en sus
// columnas); cerrado, fijar sigue pudiendo (el criptograma viaja tal cual) y cambiar el
// texto no.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin `cifrar` en la entrada, el título nuevo
// viaja en claro en su columna.
func TestUnCuadernoCifradoSeLeeYSeEscribeCifrado(t *testing.T) {
	_, clave, err := api.NuevoCifradoNotas("contraseña larga de prueba")
	if err != nil {
		t.Fatal(err)
	}
	cifrado, _ := api.EmpaquetarNota(clave, api.NotaEnClaro{Title: "Diario", Content: "<p>hoy</p>"})
	nota := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", ContentCipher: cifrado, Data: []byte(`{}`)}
	p, falso := pantallaConServicio(t, map[string]string{
		"PUT /notes/n1/encrypted": `{"id":"n1","notebook_id":"c1","kind":"text"}`,
	}, nota)
	p.activo.Cifrado, p.bloqueado = true, true
	if n, _ := p.nota("n1"); !n.Cifrada || n.Rotulo(p.tr) != p.tr.T("crypto.locked", nil) {
		t.Fatal("cerrado, la nota se lee")
	}
	if err := p.editar("n1", Parche{Titulo: ptr("otro")}, true); err == nil {
		t.Error("cerrado, se puede cambiar el título")
	}
	if err := p.editar("n1", Parche{Fijada: ptr(true)}, true); err != nil {
		t.Fatal(err)
	}
	if c := falso.ultima(t, "PUT /notes/n1/encrypted"); c["content_cipher"] != cifrado {
		t.Errorf("fijar cerrado cambia el criptograma: %v", c)
	}

	p.clave, p.bloqueado = clave, false
	p.notas = p.deNotas([]api.Note{nota})
	n, _ := p.nota("n1")
	if n.Titulo != "Diario" || n.Cifrada {
		t.Fatalf("abierto, la nota es %q (cifrada %v)", n.Titulo, n.Cifrada)
	}
	if err := p.editar("n1", Parche{Titulo: ptr("Diario de viaje")}, true); err != nil {
		t.Fatal(err)
	}
	c := falso.ultima(t, "PUT /notes/n1/encrypted")
	if c["title"] != "" || c["content"] != "" {
		t.Errorf("el texto viaja en claro: %v", c)
	}
	claro, err := api.DesempaquetarNota(clave, c["content_cipher"].(string))
	if err != nil || claro.Title != "Diario de viaje" || claro.Content != "<p>hoy</p>" {
		t.Errorf("dentro del criptograma va %+v, %v", claro, err)
	}
}

// TestEnsancharLaNotaQueSeEscribe: mientras se escribe, la cabecera lleva «Ensanchar la
// nota»; pulsarlo da a la tarjeta el `min-h-[70dvh]` de la web y cambia el botón a
// «Devolverla a su tamaño», y cerrar el editor la devuelve.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el `min-h` en `MedidaTarjetaNota`, la
// tarjeta ensanchada mide lo mismo que antes.
func TestEnsancharLaNotaQueSeEscribe(t *testing.T) {
	p, w := montarCaptura(t, 1280, 800, "columns")
	n, _ := p.nota("n2")
	p.abrirEditor(n)
	buscar := func() (*Tarjeta, *botonBarra) {
		var tj *Tarjeta
		var b *botonBarra
		recorrerInterno(w.Canvas().Content(), func(o fyne.CanvasObject) {
			if x, ok := o.(*Tarjeta); ok && x.Nota().ID == "n2" {
				tj = x
			}
			if x, ok := o.(*botonBarra); ok && (x.Rotulo == p.tr.T("note.widen", nil) || x.Rotulo == p.tr.T("note.narrow", nil)) {
				b = x
			}
		})
		if tj == nil || b == nil {
			t.Fatal("no está la tarjeta que se escribe o su botón de ensanchar")
		}
		return tj, b
	}
	tj, b := buscar()
	if b.Rotulo != p.tr.T("note.widen", nil) || tj.Size().Height >= AltoAncha*800 {
		t.Fatalf("antes de ensanchar: botón %q, alto %v", b.Rotulo, tj.Size().Height)
	}
	b.Tapped(&fyne.PointEvent{})
	tj, b = buscar()
	if b.Rotulo != p.tr.T("note.narrow", nil) || tj.Size().Height < AltoAncha*800 {
		t.Errorf("ensanchada: botón %q, alto %v (quiero al menos %v)", b.Rotulo, tj.Size().Height, AltoAncha*800)
	}
	p.cerrarEditor()
	if p.lienzo.Ancha {
		t.Error("cerrar el editor deja la nota ensanchada para la próxima vez")
	}
}

// TestLaHerramientaDeEnlacesOfreceLoDeLaWeb: los títulos de dentro de una nota salen con
// el slug de `titulosDe` (único, sin acentos), el enlace lleva los parámetros en el orden
// de la web, y lo de otros cuadernos viene de la búsqueda sin repetir el propio.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con `url.Values.Encode`, el enlace salía como
// `?h=…&note=…&notebook=…`, distinto del que guarda la web para la misma nota.
func TestLaHerramientaDeEnlacesOfreceLoDeLaWeb(t *testing.T) {
	html := `<h2>Presupuesto &amp; plazos</h2><p>x</p><h2>Presupuesto &amp; plazos</h2>` +
		`<h3 class="c">Año <b>2026</b></h3><h2>roto</h3>`
	got := titulosDe(html)
	quiero := []titulo{{2, "Presupuesto & plazos", "presupuesto-plazos"}, {2, "Presupuesto & plazos", "presupuesto-plazos-2"}, {3, "Año 2026", "ano-2026"}}
	if len(got) != len(quiero) {
		t.Fatalf("títulos %+v", got)
	}
	for i := range quiero {
		if got[i] != quiero[i] {
			t.Errorf("título %d: %+v, quiero %+v", i, got[i], quiero[i])
		}
	}

	p, _ := pantallaConServicio(t, nil,
		api.Note{ID: "a", NotebookID: "c1", Title: "Mudanza", Content: "<h2>Cajas grandes</h2>"},
		api.Note{ID: "b", NotebookID: "c1", Title: "Otra", Content: "<p>nada</p>"})
	p.AppURL = func(r string) string { return "https://notes.sytue.eu" + r }
	op := p.opcionesEnlace("cajas")
	if len(op) != 1 || op[0].nota != "Mudanza" || op[0].titulo != "Cajas grandes" ||
		op[0].url != "https://notes.sytue.eu/?notebook=c1&note=a&h=cajas-grandes" {
		t.Errorf("las opciones locales de «cajas» son %+v", op)
	}
	if op := p.opcionesEnlace("https://sytue.eu"); len(op) != 1 || op[0].tipo != enlaceURL {
		t.Errorf("una URL ofrece %+v", op)
	}
	rem := p.opcionesRemotas("cajas", []api.NoteHit{
		{ID: "a", NotebookID: "c1", Title: "Mudanza"},
		{ID: "z", NotebookID: "c9", Notebook: "Trabajo", Title: "Almacén", Headings: []string{"Cajas pequeñas", "Otro"}},
	})
	if len(rem) != 2 || rem[0].nota != "Almacén" || rem[0].cuaderno != "Trabajo" ||
		rem[1].titulo != "Cajas pequeñas" || rem[1].url != "https://notes.sytue.eu/?notebook=c9&note=z&h=cajas-pequenas" {
		t.Errorf("las remotas son %+v", rem)
	}
}

// TestElAroDeLaZonaEsElDeLaSuite: la web escribía `focus-visible:ring-2 ring-primary/40`,
// que no pintaba —gana el aro global, sin capa—, y este port copiaba lo escrito: la tinta
// al 40 %, centrada en el borde y en cuadrado. Lo que se ve en producción es el aro de la
// suite: 2 px de `focus-ring` por fuera, con «4px 4px 6px 6px» de esquina.
func TestElAroDeLaZonaEsElDeLaSuite(t *testing.T) {
	z := NuevoEditorNota(trInterno(t), Leer("<p>uno</p>"), "").zona
	r := test.WidgetRenderer(z).(*zonaRenderer)
	z.enfocado = true
	r.Layout(fyne.NewSize(300, 100))
	// El aro abierto de `ui.Aro` son cuatro franjas pintadas por fuera de la caja; las dos
	// primeras, la de arriba y la de abajo, llevan las esquinas.
	a, ok := r.objetos[0].(*canvas.Raster)
	if !ok {
		t.Fatalf("lo primero de la zona enfocada es %T, y es el aro", r.objetos[0])
	}
	abajo, _ := r.objetos[1].(*canvas.Raster)
	rs, rm := float32(math.Ceil(float64(tema.RadioSM))), float32(math.Ceil(float64(tema.RadioMD)))
	if a.Position() != fyne.NewPos(-2, -2) || a.Size() != fyne.NewSize(304, 2+rs) ||
		abajo == nil || abajo.Position() != fyne.NewPos(-2, 100-rm) || abajo.Size() != fyne.NewSize(304, rm+2) {
		t.Errorf("la franja de arriba del aro va en %v con %v: rodea la caja por fuera, con la esquina de %v arriba y la de %v abajo",
			a.Position(), a.Size(), tema.RadioSM, tema.RadioMD)
	}
	// 2 px de `focus-ring`, y nada dentro de la caja ni en el pico de la esquina.
	anillo := color.Color(tema.Color("focus-ring", tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
	img := a.Generator(304, int(2+rs))
	tinta := func(x, y int) color.Color { return color.NRGBAModel.Convert(img.At(x, y)) }
	_, _, _, dentro := img.At(152, 2).RGBA()
	_, _, _, pico := img.At(0, 0).RGBA()
	if tinta(152, 0) != anillo || tinta(152, 1) != anillo || dentro != 0 || pico != 0 {
		t.Errorf("el aro pinta %v y %v, con alfa %v dentro de la caja y %v en el pico; son 2 px de `focus-ring` por fuera, con la esquina redondeada",
			tinta(152, 0), tinta(152, 1), dentro, pico)
	}
}
