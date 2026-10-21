// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// LOS GUARDIANES DE LA SEGUNDA TANDA: etiquetas, papelera, renombrar, el árbol de notas,
// la barra que envuelve, las columnas, el teclado y la prosa. Las medidas son de la HOJA
// (clases de Tailwind con el rem de 14) y de las capturas del caso de la web con los
// mismos datos (`capturas_test.go`), no de lo que calcula el propio widget.

// TestUnaEtiquetaSinNombreONiUnEstadoNoSeGuarda: es lo que la web no deja guardar, y
// los estados sin nombre no viajan.
func TestUnaEtiquetaSinNombreONiUnEstadoNoSeGuarda(t *testing.T) {
	b := nuevaEtiqueta()
	if _, ok := b.entrada(); ok {
		t.Error("una etiqueta vacía se puede guardar")
	}
	b.nombre = "  Prioridad "
	if _, ok := b.entrada(); ok {
		t.Error("una etiqueta sin ningún estado con nombre se puede guardar")
	}
	b.estados[0].Name = "Alta"
	b.añadirEstado()
	in, ok := b.entrada()
	if !ok || in.Name != "Prioridad" || len(in.States) != 1 {
		t.Errorf("se manda %+v", in)
	}
	if b.estados[1].Color != ColoresEtiqueta[1] {
		t.Errorf("el segundo estado nace con %q y la paleta da %q", b.estados[1].Color, ColoresEtiqueta[1])
	}
}

// TestCambiarUnaEtiquetaConservaSusEstados: los estados viajan con su ID, que es lo que
// hace que el servicio los mantenga y con ellos las notas que ya los llevan; y después se
// vuelven a leer todas.
func TestCambiarUnaEtiquetaConservaSusEstados(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"PUT /tags/t1": `{"id":"t1","name":"Urgencia"}`,
		"GET /tags":    `{"tags":[{"id":"t1","name":"Urgencia","states":[{"id":"s1","name":"Alta","color":"#b0413a"}]}]}`,
	})
	b := borradorDe(api.Tag{ID: "t1", Name: "Prioridad", States: []api.TagState{{ID: "s1", Name: "Alta", Color: "#b0413a"}}})
	b.nombre = "Urgencia"
	in, _ := b.entrada()
	if err := p.guardarEtiqueta(b.id, in); err != nil {
		t.Fatal(err)
	}
	cuerpo := falso.ultima(t, "PUT /tags/t1")
	estados, _ := cuerpo["states"].([]any)
	if cuerpo["name"] != "Urgencia" || len(estados) != 1 || estados[0].(map[string]any)["id"] != "s1" {
		t.Errorf("guardar manda %v", cuerpo)
	}
	if len(p.etiquetas) != 1 || p.etiquetas[0].Name != "Urgencia" {
		t.Errorf("tras guardar las etiquetas son %+v", p.etiquetas)
	}
}

// TestElAtajoDeUnEstadoMarcaLaNotaSeleccionada: con una nota elegida, pulsar el carácter
// de un estado se lo pone; sin nota elegida, la tecla no hace nada.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con `atajoDeEstado` sin reconocer el
// atajo, la «1» no es el de «Alta». Deshecho.
func TestElAtajoDeUnEstadoMarcaLaNotaSeleccionada(t *testing.T) {
	p, falso := pantallaConServicio(t, nil, api.Note{ID: "n1", NotebookID: "c1"})
	p.etiquetas = []api.Tag{{ID: "t1", Name: "Prioridad", States: []api.TagState{{ID: "s1", Name: "Alta", Shortcut: "1"}}}}
	p.tecla('1')
	if strings.Contains(strings.Join(falso.rutas(), "|"), "/state") {
		t.Fatal("sin nota elegida el atajo ha marcado algo")
	}
	p.lienzo.Seleccionada = "n1"
	p.marcarEstado("n1", "t1", "s1") // lo que lanza `tecla` en su rutina, aquí en línea
	if c := falso.ultima(t, "POST /notes/n1/state"); c["tag_id"] != "t1" || c["state_id"] != "s1" {
		t.Errorf("el atajo manda %v", c)
	}
	if tag, estado, ok := p.atajoDeEstado('1'); !ok || tag != "t1" || estado != "s1" {
		t.Errorf("la «1» no es el atajo de «Alta»: %q %q %v", tag, estado, ok)
	}
}

// TestVaciarLaPapeleraDejaLasQueFallan: cada nota se borra por su lado y las que no se
// pudieron borrar se quedan, que es el `Promise.allSettled` de la web.
//
// VISTO EN ROJO (2026-09-23, en el frontal): quitando también las que fallan, la papelera
// se queda vacía. Deshecho.
func TestVaciarLaPapeleraDejaLasQueFallan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/x2/") {
			http.Error(w, `{"error":"legal_hold"}`, http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	p := NuevaPantalla(trInterno(t), "es", api.NewNotes(srv.URL, nil, nil), nil, nil)
	notas := DeNotas([]api.Note{{ID: "x1"}, {ID: "x2"}, {ID: "x3"}})
	p.papelera.Notas, p.papelera.Fase = notas, Exito
	p.vaciarYa(notas)
	if ids(p.papelera.Notas) != "x2" {
		t.Errorf("tras vaciar quedan %s y solo tenía que quedar la que falló", ids(p.papelera.Notas))
	}
}

// TestRenombrarGuardaConIntroYEscapeDescarta: el campo del título guarda al confirmar y
// no manda nada si se descarta o no cambia.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con Intro cancelando, no llega el PUT.
func TestRenombrarGuardaConIntroYEscapeDescarta(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"PUT /notes/n1": `{"id":"n1","notebook_id":"c1","kind":"text","title":"Nuevo"}`,
	}, api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Title: "Viejo"})
	n, _ := p.nota("n1")

	p.renombrar(n)
	if p.lienzo.CampoTitulo == nil || p.lienzo.Renombrando != "n1" {
		t.Fatal("renombrar no abre el campo en la tarjeta")
	}
	p.lienzo.CampoTitulo.cancelar()
	if p.lienzo.CampoTitulo != nil || len(falso.rutas()) != 0 {
		t.Errorf("Escape ha mandado %v o ha dejado el campo abierto", falso.rutas())
	}

	p.renombrar(n)
	c := p.lienzo.CampoTitulo
	c.ed.SetText("  Nuevo ")
	c.ed.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	// La escritura va en su rutina, y pinta antes de mandar: se le deja pasar y se espera.
	for i := 0; i < 200 && len(falso.rutas()) == 0; i++ {
		fyne.DoAndWait(func() {})
		time.Sleep(10 * time.Millisecond)
	}
	if cuerpo := falso.ultima(t, "PUT /notes/n1"); cuerpo["title"] != "Nuevo" {
		t.Errorf("renombrar manda %v", cuerpo)
	}
}

// TestLaFilaDelArbolMide43Coma75: `py-1.5` con el título y la meta, que es lo MEDIDO en
// producción; renombrando, el campo (`py-1`) y la fila miden 28,25.
//
// VISTO EN ROJO (2026-09-23, en el frontal): con `py-2` en el botón, 47,25. Deshecho.
func TestLaFilaDelArbolMide43Coma75(t *testing.T) {
	f := NuevaFilaNotaArbol("Presupuesto")
	f.Meta = "21 sept"
	cerca(t, "la fila del árbol", test.WidgetRenderer(f).MinSize().Height, 50)
	g := NuevaFilaNotaArbol("Presupuesto")
	g.Campo = NuevoCampoEnLinea("Presupuesto", "", PYCampoArbol, "bg", 0)
	cerca(t, "la fila renombrando", test.WidgetRenderer(g).MinSize().Height, 32)
	cerca(t, "el campo de la tarjeta", AltoCampoEnLinea(PYCampoTarjeta), 28)
}

// TestLaBarraBajaLoQueNoCabe: la barra envuelve, pero desde que sus menús son iconos
// —sin «Orden propio», «Filtrar por etiqueta» ni «Exportar» escritos— cabe en UNA fila
// también a 740, el lienzo de la vista de árbol en una ventana de 1280: «Nueva nota» se
// queda la última de esa fila, el título cede sin desaparecer y la barra mide `min-h-12`.
// En el móvil sigue partiendo: el filtro baja a su fila, entera.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin partir la fila, la barra mide 43 donde
// la hoja da 78, «Nueva nota» se queda en la primera fila y el título a cero de ancho,
// que era lo que hacía la franja de antes. Deshecho.
func TestLaBarraBajaLoQueNoCabe(t *testing.T) {
	b := NuevaBarra(trInterno(t))
	b.Cuaderno = Cuaderno{ID: "c1", Titulo: "Mudanza"}
	b.Etiquetas = []api.Tag{{ID: "t1", Name: "Prioridad"}}
	o := b.Objeto(tema.Claro).(*fyne.Container)
	flex := o.Layout.(*barraFlex)
	cerca(t, "la barra en una fila a 1020", flex.AltoPara(o.Objects, 1020), AltoBarra+tema.Borde)
	cerca(t, "la barra en una fila a 740", flex.AltoPara(o.Objects, 740), AltoBarra+tema.Borde)
	o.Resize(fyne.NewSize(740, flex.AltoPara(o.Objects, 740)))
	flex.Layout(o.Objects, o.Size())
	nueva := o.Objects[len(o.Objects)-1]
	if fin := nueva.Position().X + nueva.Size().Width; nueva.Position().Y >= AltoBarra || fin > 740-MargenBarra+0.5 {
		t.Errorf("«Nueva nota» está en %v y acaba en %.1f: tenía que quedarse en la primera fila, dentro de la barra", nueva.Position(), fin)
	}
	if titulo := o.Objects[2]; titulo.Size().Width < 60 {
		t.Errorf("el título mide %v de ancho: no ha cedido, ha desaparecido", titulo.Size().Width)
	}
	// En el móvil el filtro va el último y en su fila, entera.
	b.Estrecha = true
	o = b.Objeto(tema.Claro).(*fyne.Container)
	o.Resize(fyne.NewSize(360, 200))
	o.Layout.Layout(o.Objects, o.Size())
	filtro := o.Objects[len(o.Objects)-1]
	if _, es := filtro.(*ui.Entrada); !es || filtro.Size().Width != 360-2*MargenBarra {
		t.Errorf("en el móvil el último es %T de %v de ancho", filtro, filtro.Size().Width)
	}
}

// TestLasColumnasSiguenALaVentana: `grid-cols-1 md:grid-cols-2 lg:grid-cols-3`, contra el
// ancho de la VENTANA.
func TestLasColumnasSiguenALaVentana(t *testing.T) {
	for _, c := range []struct {
		ventana float32
		cols    int
	}{{1280, 3}, {900, 2}, {360, 1}} {
		if n := ui.RejillaTarjetas(c.ventana, HuecoColumnas, ui.Cortes{1, 0, 2, 3, 0, 0}).Fijas; n != c.cols {
			t.Errorf("a %v la rejilla tiene %d columnas y la web %d", c.ventana, n, c.cols)
		}
	}
}

// TestAltFlechaMueveLaNotaSeleccionada: la alternativa de teclado al arrastre (el
// `PASO = 24` de NoteCanvas.tsx). En columnas, Alt+→ la pasa a la siguiente y no más allá
// de la tercera; en el libre, la desplaza 24 y no por debajo de cero.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el tope, la lleva a una cuarta columna.
func TestAltFlechaMueveLaNotaSeleccionada(t *testing.T) {
	n := DeNota(api.Note{ID: "n1", Col: 2, X: 10, Y: 30, Position: 0})
	if pos := posicionConTeclado(n, fyne.KeyRight, false); pos.Col != 2 {
		t.Errorf("Alt+→ en la última columna la lleva a la %d", pos.Col)
	}
	if pos := posicionConTeclado(n, fyne.KeyLeft, false); pos.Col != 1 {
		t.Errorf("Alt+← la lleva a la %d", pos.Col)
	}
	if pos := posicionConTeclado(n, fyne.KeyUp, false); pos.Position != 0 {
		t.Errorf("Alt+↑ en la primera posición la lleva a la %d", pos.Position)
	}
	if pos := posicionConTeclado(n, fyne.KeyLeft, true); pos.X != 0 {
		t.Errorf("Alt+← en el libre la deja en x = %d", pos.X)
	}
	if pos := posicionConTeclado(n, fyne.KeyDown, true); pos.Y != 30+PasoTeclado {
		t.Errorf("Alt+↓ en el libre la deja en y = %d", pos.Y)
	}
}

// TestLaProsaColapsaLosMargenesComoLaWeb: un párrafo (`mb-2`) seguido de una lista
// (`my-1.5`) deja 7 entre los dos, no 12,25; el primer título no lleva margen arriba
// (`first:mt-0`) y el último párrafo no lo lleva abajo (`last:mb-0`). Los números son los
// de la captura de la web: el primer elemento de la lista empieza 27 px por debajo del
// párrafo (19,906 + 7).
//
// VISTO EN ROJO (2026-09-23, en el frontal): sumando los márgenes en vez de colapsarlos,
// 32,16 y 23,41 donde la web da 26,91 y 21,66. Deshecho.
func TestLaProsaColapsaLosMargenesComoLaWeb(t *testing.T) {
	d := Leer("<p>Pedir tres presupuestos.</p><ul><li>Cajas</li><li>Camión</li></ul>")
	m := Maquetar(d, 600, tema.Claro)
	if len(m.Lineas) != 3 {
		t.Fatalf("la prosa tiene %d líneas y son tres", len(m.Lineas))
	}
	cerca(t, "el primer elemento de la lista", m.Lineas[1].Y, escalonCuerpo.Linea+margenParrafo)
	cerca(t, "entre dos elementos", m.Lineas[2].Y-m.Lineas[1].Y, escalonCuerpo.Linea+margenElemento)
	cerca(t, "la sangría de la lista", m.Lineas[1].Trozos[0].X, sangriaLista)
	cerca(t, "el alto con el margen de la lista abajo", m.Alto, m.Lineas[2].Y+escalonCuerpo.Linea+margenLista)

	h := Maquetar(Leer("<h1>Título</h1><p>uno</p>"), 600, tema.Claro)
	cerca(t, "el primer título", h.Lineas[0].Y, 0)
	cerca(t, "el párrafo tras el título", h.Lineas[1].Y, tema.Texto2XL.Tam*1.25+margenTituloB)
}

// TestLaProsaRepartePorPalabrasYPintaLasMarcas: una línea que no cabe se parte por el
// blanco, la negrita va en su cara y un trozo recuerda qué letras del bloque pinta (el
// editor coloca el cursor con eso).
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin cerrar la línea al no caber, a 120 no se
// parte.
func TestLaProsaRepartePorPalabrasYPintaLasMarcas(t *testing.T) {
	d := Leer("<p><b>Nueva:</b> calle Mayor 3 esquina con la plaza</p>")
	ancha := Maquetar(d, 1000, tema.Claro)
	if len(ancha.Lineas) != 1 || len(ancha.Lineas[0].Trozos) != 2 {
		t.Fatalf("a 1000 son %d líneas", len(ancha.Lineas))
	}
	if ancha.Lineas[0].Trozos[0].estilo.peso != ui.Negrita || ancha.Lineas[0].Trozos[1].Desde != 6 {
		t.Errorf("los trozos son %+v", ancha.Lineas[0].Trozos)
	}
	estrecha := Maquetar(d, 120, tema.Claro)
	if len(estrecha.Lineas) < 2 {
		t.Fatal("a 120 no se parte en líneas")
	}
	for _, l := range estrecha.Lineas {
		if u := l.Trozos[len(l.Trozos)-1]; u.X+medirTrozo(strings.TrimSpace(u.Texto), u.estilo) > 120.5 {
			t.Errorf("una línea se sale: %+v", l.Trozos)
		}
	}
	// El cursor de la letra 6 cae detrás de «Nueva:», y el clic ahí devuelve la 6.
	l := ancha.Lineas[0]
	x := l.XDe(6)
	if pos := l.PosEn(x); pos != 6 {
		t.Errorf("el clic en la X de la letra 6 da la %d", pos)
	}
}

// TestElPanelDelArbolEsPapelBlanco: el panel de la lista de notas es `bg-surface
// border-r`, no el fondo de la página.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin `conFondo` el panel es el árbol a secas.
func TestElPanelDelArbolEsPapelBlanco(t *testing.T) {
	p, w := montarCaptura(t, 1280, 800, "tree")
	_ = w
	a := p.armazon(tema.Claro)
	c, ok := a.Panel.(*fyne.Container)
	if !ok {
		t.Fatalf("el panel es %T", a.Panel)
	}
	if _, es := c.Layout.(*conFondo); !es {
		t.Errorf("el panel no lleva su fondo: %T", c.Layout)
	}
}

// TestArrastrarUnaNotaEnElLienzoLibre: el asa mueve la tarjeta con el puntero, recortada
// a cero por la izquierda, y soltar la coloca con UNA escritura en `{x, y, col: 0}`.
// (El lienzo se llama sin la rutina de `lanzar`: una escritura en vuelo al acabar el test
// se cruzaría con el siguiente.)
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin restar el margen del lienzo al soltar,
// la nota caía 14 px a la derecha y 14 abajo de donde se soltó; y sin `alFrente`, la que
// se arrastra pasaba por debajo de la de al lado.
func TestArrastrarUnaNotaEnElLienzoLibre(t *testing.T) {
	p, falso := pantallaConServicio(t, nil, api.Note{ID: "a", NotebookID: "c1", X: 40, Y: 30, Col: 2, Position: 5},
		api.Note{ID: "b", NotebookID: "c1", X: 300, Y: 30})
	p.activo.Vista = Libre
	p.lienzo.AlMoverLibre = func(n Nota, en fyne.Position) { _ = p.moverLibre(n, en) }
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	p.win = w
	w.Resize(fyne.NewSize(1280, 800))
	p.Repintar()
	var (
		asa    *asaArrastre
		corcho *fyne.Container
	)
	recorrerInterno(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if a, ok := o.(*asaArrastre); ok && a.t.nota.ID == "a" {
			asa = a
		}
		if c, ok := o.(*fyne.Container); ok {
			if _, es := c.Layout.(*libre); es {
				corcho = c
			}
		}
	})
	if asa == nil || corcho == nil {
		t.Fatal("la tarjeta del lienzo libre no lleva asa, o no está el lienzo")
	}
	// `cursor-grab`: la mano, con algo pintado.
	img, _, _ := asa.Cursor().Image()
	if img == nil || img.Bounds().Dx() != LadoCursor {
		t.Fatalf("el asa no tiene la mano de cursor: %v", img)
	}
	if _, _, _, a := img.At(LadoCursor/2, LadoCursor/2).RGBA(); a == 0 {
		t.Error("la mano del cursor sale vacía por el centro")
	}
	asa.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(30, 20)})
	// `cursor-grabbing`: mientras se arrastra, la mano CERRADA, que es otra imagen.
	cerrada, _, _ := asa.Cursor().Image()
	if _, es := asa.Cursor().(cursorAgarrando); !es || cerrada == nil || iguales(img, cerrada) {
		t.Errorf("arrastrando, el cursor es %T: no es la mano cerrada", asa.Cursor())
	}
	if d := os.Getenv("SYTUE_CAPTURAS"); d != "" {
		escribirPNG(t, filepath.Join(d, "notes-cursor-abierta.png"), img)
		escribirPNG(t, filepath.Join(d, "notes-cursor-cerrada.png"), cerrada)
	}
	// `z-10`: la que se arrastra se pinta encima, y el lienzo no la devuelve a su sitio
	// al colocarse.
	if corcho.Objects[len(corcho.Objects)-1] != fyne.CanvasObject(asa.t) {
		t.Error("la nota que se arrastra no pasa por encima de las demás")
	}
	corcho.Refresh()
	if asa.t.Position().X != MargenLienzo+40+30 {
		t.Errorf("recolocar el lienzo en pleno arrastre la lleva a x=%v", asa.t.Position().X)
	}
	asa.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(-200, 5)})
	asa.DragEnd()
	if _, es := asa.Cursor().(cursorAgarre); !es {
		t.Errorf("al soltar, el cursor sigue siendo %T", asa.Cursor())
	}
	c := falso.ultima(t, "POST /notes/a/move")
	if c["x"] != float64(0) || c["y"] != float64(55) || c["col"] != float64(0) || c["position"] != float64(5) {
		t.Errorf("soltar manda %v", c)
	}
	if r := falso.rutas(); len(r) != 1 {
		t.Errorf("un arrastre son %d escrituras: %v", len(r), r)
	}
}

// TestUnEnlaceConTituloLlevaHastaEl: un enlace a una nota con `h=` la abre y desplaza el
// lienzo hasta el título con ese slug —el segundo «Destino» es `destino-2`—, como el
// `scrollIntoView({block: 'start'})` de la web.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin guardar el `h=` en `seguirEnlace`, el
// lienzo se queda arriba.
func TestUnEnlaceConTituloLlevaHastaEl(t *testing.T) {
	relleno := strings.Repeat("<p>Una línea de relleno para que haya que bajar.</p>", 40)
	p, _ := pantallaConServicio(t, nil, api.Note{
		ID: "a", NotebookID: "c1", Kind: "text", Title: "Larga",
		Content: "<h2>Destino</h2>" + relleno + "<h2>Destino</h2>" + relleno,
	})
	p.activo.Vista = Lista
	p.AppURL = func(r string) string { return "https://notes.sytue.eu" + r }
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	p.win = w
	w.Resize(fyne.NewSize(1280, 800))
	p.Repintar()
	if i, ok := bloqueDeAncla(p.notas[0].Doc, "destino-2"); !ok || i != 41 {
		t.Fatalf("destino-2 es el bloque %d (%v)", i, ok)
	}
	p.seguirEnlace("https://notes.sytue.eu/?notebook=c1&note=a&h=destino-2")
	tj := p.lienzo.vivas["a"]
	if tj == nil || tj.prosa == nil || tj.prosa.maq == nil {
		t.Fatal("no está la prosa de la nota abierta")
	}
	d := fyne.CurrentApp().Driver()
	titulo := d.AbsolutePositionForObject(tj.prosa).Y + tj.prosa.maq.Bloques[41].Y
	arriba := d.AbsolutePositionForObject(p.desplazador).Y
	if p.desplazador.Offset.Y == 0 || titulo < arriba-1 || titulo > arriba+1 {
		t.Errorf("el título queda en y=%v y el lienzo empieza en %v (desplazado %v)", titulo, arriba, p.desplazador.Offset.Y)
	}
}

// TestUnaTablaSePintaComoEnLaWeb: medido en la web con esta misma nota en una tarjeta de
// 323,7 (300,7 de cuerpo): la tabla empieza a 30,75 (la línea de «Recuento:» y su `mb-2`),
// sin bordes ni relleno —filas de 22,75—, las columnas a lo que piden (152,546875 y 35,671875), la
// cabecera en negrita y centrada, y «Fin.» a 99.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el caso de la tabla en `bloque`, la
// tabla sale como una línea de texto corrido y «Fin.» sube a 46,8.
func TestUnaTablaSePintaComoEnLaWeb(t *testing.T) {
	d := Leer(`<p>Recuento:</p><table><tbody><tr><th>Cuarto</th><th>Cajas</th></tr>` +
		`<tr><td>Cocina</td><td>12</td></tr><tr><td>Salón y comedor grande</td><td>7</td></tr></tbody></table><p>Fin.</p>`)
	m := Maquetar(d, 300.7, tema.Claro)
	donde := map[string]LineaProsa{}
	x := map[string]float32{}
	for _, l := range m.Lineas {
		for _, tr := range l.Trozos {
			donde[strings.TrimSpace(tr.Texto)] = l
			x[strings.TrimSpace(tr.Texto)] = tr.X
		}
	}
	casi := func(que string, got, quiero float32) {
		t.Helper()
		if got < quiero-1 || got > quiero+1 {
			t.Errorf("%s: %.2f, y la web da %.2f", que, got, quiero)
		}
	}
	casi("la fila de cabecera", donde["Cuarto"].Y, 30.75)
	casi("la segunda fila", donde["Cocina"].Y, 53.5)
	casi("la segunda columna", x["12"], 152.546875)
	casi("«Cajas» (la cabecera, que es la más ancha)", x["Cajas"], 152.546875)
	casi("«Cuarto» centrada", x["Cuarto"]+medirTrozo("Cuarto", donde["Cuarto"].Trozos[0].estilo)/2, 152.546875/2)
	casi("«Fin.»", donde["Fin."].Y, 99)
	if donde["Cuarto"].Trozos[0].estilo.peso < ui.SemiNegrita {
		t.Error("la cabecera no va en negrita")
	}
}

// iguales dice si dos imágenes tienen los mismos píxeles.
func iguales(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if a.At(x, y) != b.At(x, y) {
				return false
			}
		}
	}
	return true
}
