// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// bandejaConTriaje es la bandeja de ejemplo con todas las acciones puestas.
func bandejaConTriaje(t *testing.T, etiquetas []Etiqueta) (*Bandeja, fyne.Window) {
	t.Helper()
	b := NuevaBandeja(traductor(t), "es")
	b.Ahora = reloj()
	b.Titulo, b.Estado = "Recibidos", Listo
	b.Conversaciones = ejemplo()
	nada := func(Conversacion) {}
	b.Acciones = &Acciones{Destacar: nada, Leido: nada, Archivar: nada, Papelera: nada, Spam: nada}
	b.Etiquetas = etiquetas
	w := test.NewWindow(b)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ui.AnchoPanelBase, 744))
	w.Canvas().Capture()
	return b, w
}

// TestLaTiraDeTriajeRelevaAlExtracto: al pasar por encima, el extracto se aparta y en
// su tira de 24 aparecen los cinco botones de Recibidos —destacar, leído, archivar,
// papelera, spam— de 24 × 24 con el `gap-0.5`, empezando donde empieza el texto. Y
// pasar del hueco de la fila a uno de sus botones NO la apaga.
//
// VISTO EN ROJO (2026-09-23): con un `ui.Boton` que no avisa a su fila, el
// `MouseOut` que la fila recibe al entrar el puntero en el botón esconde la tira
// entera, y el botón se queda sin poder pulsarse.
func TestLaTiraDeTriajeRelevaAlExtracto(t *testing.T) {
	b, _ := bandejaConTriaje(t, nil)
	f := primeraFila(t, b)
	r := test.WidgetRenderer(f).(*filaRenderer)
	if r.extracto.Visible() == false || visibles(r.triaje) != 0 {
		t.Fatalf("en reposo: extracto %v y %d botones", r.extracto.Visible(), visibles(r.triaje))
	}

	f.MouseIn(nil)
	if r.extracto.Visible() || visibles(r.triaje) != 5 {
		t.Fatalf("encima: extracto %v y %d botones; tenían que ser 5 y el extracto fuera", r.extracto.Visible(), visibles(r.triaje))
	}
	xTexto := float32(xTextoMedido - 260) // el carril del menú mide 260
	yTira := RellenoFila + tema.TextoBase.Linea + tema.TextoSM.Linea + HuecoLineas
	x := xTexto
	for _, bt := range r.triaje {
		if !bt.Visible() {
			continue
		}
		if p, s := bt.Position().Add(r.tira.Position()), bt.Size(); p.X != x || p.Y != yTira || s.Width != 24 || s.Height != 24 {
			t.Errorf("el botón %q está en %v con %v; tocaba (%v, %v) y 24 × 24", bt.Rotulo, p, s, x, yTira)
		}
		x += 24 + HuecoTriaje
	}

	// Del hueco de la fila al primer botón: Fyne le da el hover al botón y se lo quita
	// a la fila, en ese orden.
	var primero *botonTriaje
	for _, bt := range r.triaje {
		if bt.Visible() {
			primero = bt
			break
		}
	}
	f.MouseOut()
	primero.MouseIn(nil)
	if visibles(r.triaje) != 5 {
		t.Error("al llegar al botón, la tira se ha escondido")
	}
	primero.MouseOut()
	if visibles(r.triaje) != 0 || !r.extracto.Visible() {
		t.Error("al salir del todo, la tira sigue puesta")
	}
}

func visibles(bs []*botonTriaje) int {
	n := 0
	for _, b := range bs {
		if b.Visible() {
			n++
		}
	}
	return n
}

// TestUnaFilaConEtiquetasCrece: los chips van debajo de la tira, con el `gap-0.5` y el
// `mt-0.5`, y la fila crece lo que miden (16,03 de chip: `text-2xs leading-tight`,
// `py-px` y el borde). Sin etiquetas sigue en los 75,75 medidos.
func TestUnaFilaConEtiquetasCrece(t *testing.T) {
	obra := Etiqueta{ID: "e1", Nombre: "Obra", Color: "teal"}
	cs := ejemplo()
	cs[0].Mensajes[0].Etiquetas = map[string]bool{"e1": true}
	b := NuevaBandeja(traductor(t), "es")
	b.Ahora = reloj()
	b.Titulo, b.Estado, b.Conversaciones, b.Etiquetas = "Recibidos", Listo, cs, []Etiqueta{obra}
	w := test.NewWindow(b)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ui.AnchoPanelBase, 744))
	w.Canvas().Capture()
	w.Canvas().Capture()

	const chip = 1.25*11 + 2 + 2
	want := float32(filaMedida + 2*2 + chip)
	if got := AltoFilaCon([]Etiqueta{obra}, 120); got != want {
		t.Errorf("una fila con una línea de chips mide %v y la hoja da %v", got, want)
	}
	f := primeraFila(t, b)
	if got := f.Size().Height; got != want {
		t.Errorf("la fila pintada con chips mide %v y tenía que medir %v", got, want)
	}
	if got := AltoFilaCon(nil, 120); got != filaMedida {
		t.Errorf("sin chips la fila mide %v: tiene que seguir en los %v medidos", got, filaMedida)
	}
	// Tres chips largos en una columna estrecha parten línea (`flex-wrap`).
	largas := []Etiqueta{{Nombre: "Presupuestos 2026"}, {Nombre: "Proveedores"}, {Nombre: "Urgente"}}
	if _, _, alto := ColocarChips(largas, 120); alto <= chip {
		t.Errorf("tres chips en 120 px caben en una línea de %v: no se reparten", alto)
	}
}

// TestLaCabeceraDelLectorSeQuedaEnGlifosEnElMovil: por debajo de 768 px de ventana,
// «Responder a todos» es su glifo con `px-3` y su borde de secundario (39 de ancho) y
// «Etiquetar», glifo y galón (60); en escritorio llevan el rótulo. MEDIDO en el arnés
// sin sesión a 360 px (2026-09-22): 39,00 y 60,00.
func TestLaCabeceraDelLectorSeQuedaEnGlifosEnElMovil(t *testing.T) {
	for _, c := range []struct {
		ancho          float32
		todos, etiq    float32
		conRotulo      bool
		descripcionCas string
	}{
		{360, 42, 64, false, "móvil"},
		{1280, 0, 0, true, "escritorio"},
	} {
		x := NuevoLector(traductor(t), "es")
		x.Ahora = reloj()
		x.Estado, x.Conversacion = Listo, ejemplo()[0]
		nada := func(Conversacion) {}
		x.Acciones = &Acciones{
			Destacar: nada, Leido: nada, Archivar: nada, Papelera: nada, Spam: nada,
			Etiquetar: func(Conversacion, string, bool) {}, Etiquetas: []Etiqueta{{ID: "e1", Nombre: "Obra"}},
		}
		w := test.NewWindow(x)
		w.SetPadded(false)
		w.Resize(fyne.NewSize(c.ancho, 740))
		x.Refresh()
		w.Canvas().Capture()
		r := test.WidgetRenderer(x).(*lectorRenderer)
		todos := r.barra.Objects[1].(*ui.Boton)
		grupo := r.barra.Objects[len(r.barra.Objects)-1].(*fyne.Container)
		var etiq *ui.Boton
		for _, o := range grupo.Objects {
			if b, ok := o.(*ui.Boton); ok && b.Rotulo == traductor(t).T("action.label", nil) {
				etiq = b
			}
		}
		if etiq == nil {
			t.Fatalf("%s: no hay botón «Etiquetar»", c.descripcionCas)
		}
		if c.conRotulo {
			if todos.Texto == "" || etiq.Texto == "" {
				t.Errorf("%s: los rótulos se han quedado en `sr-only`", c.descripcionCas)
			}
		} else if todos.MinSize().Width != c.todos || etiq.MinSize().Width != c.etiq {
			t.Errorf("%s: «Responder a todos» mide %v (web %v) y «Etiquetar» %v (web %v)",
				c.descripcionCas, todos.MinSize().Width, c.todos, etiq.MinSize().Width, c.etiq)
		}
		w.Close()
	}
}

// TestLasCapturasDelTriajeSeGuardan deja los PNG del triaje cuando se pide
// (SYTUE_CAPTURAS=directorio): escritorio y móvil, con el hilo abierto, la fila con
// el puntero encima y el menú «Más acciones» abierto.
func TestLasCapturasDelTriajeSeGuardan(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	for _, c := range []struct {
		nombre string
		tam    fyne.Size
	}{{"escritorio", fyne.NewSize(1280, 744)}, {"movil", fyne.NewSize(360, 740)}} {
		s := servicioConTodas()
		s.baja = api.MailBaja{OneClick: true}
		m1 := s.paginas[""][0]
		m1.Keywords = append(m1.Keywords, "e1")
		s.paginas[""][0] = m1
		s.hilo = []api.MailMessage{m1, s.paginas[""][2]}
		b := buzonConAvisos(t, s)
		w := test.NewWindow(b.P.Objeto())
		w.SetPadded(false)
		w.Resize(c.tam)
		b.Arrancar(context.Background())
		b.Abrir(b.P.Bandeja.Conversaciones[0])
		b.P.Refrescar()
		guardar(t, filepath.Join(dir, "lector-"+c.nombre+".png"), w.Canvas().Capture())

		r := test.WidgetRenderer(b.P.Lector).(*lectorRenderer)
		grupo := r.barra.Objects[len(r.barra.Objects)-1].(*fyne.Container)
		for _, o := range grupo.Objects {
			if bt, ok := o.(*ui.Boton); ok && bt.Icono == iconos.Puntos {
				bt.Tapped(nil) // «Más acciones»
			}
		}
		guardar(t, filepath.Join(dir, "lector-menu-"+c.nombre+".png"), w.Canvas().Capture())
		w.Close()
	}
	b, w := bandejaConTriaje(t, []Etiqueta{{ID: "e1", Nombre: "Obra", Color: "teal"}})
	primeraFila(t, b).MouseIn(nil)
	guardar(t, filepath.Join(dir, "fila-encima.png"), w.Canvas().Capture())
}

func guardar(t *testing.T, ruta string, img image.Image) {
	t.Helper()
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("captura en %s", ruta)
}

// TestLoQueNoCabeSeDesplazaYNoSeEsconde: con seis acciones en el panel de 280 la tira
// de triaje no cabe; el último botón no se esconde —antes se escondía y no había forma
// de llegar a él—: se recorta y el tabulador lo trae a la vista. La barra de lote, con
// siete acciones, igual. `overflow-x-auto` sin barra en los dos casos.
func TestLoQueNoCabeSeDesplazaYNoSeEsconde(t *testing.T) {
	b, _ := bandejaConTriaje(t, nil)
	nada := func(Conversacion) {}
	b.Acciones.NoSpam = nada
	b.Refresh()
	f := primeraFila(t, b)
	// 300 px fija una columna donde seis acciones24 + cinco gaps2 desbordan.
	// El panel20rem ahora mide320: el fixture anterior ya cabía y no ejercía scroll.
	f.Resize(fyne.NewSize(300, f.Size().Height))
	r := test.WidgetRenderer(f).(*filaRenderer)
	f.MouseIn(nil)
	if visibles(r.triaje) != 6 {
		t.Fatalf("encima hay %d botones y tenían que ser 6", visibles(r.triaje))
	}
	if r.tira.AnchoContenido() <= r.tira.Size().Width {
		t.Fatalf("la tira cabe (%g en %g): el caso no prueba nada", r.tira.AnchoContenido(), r.tira.Size().Width)
	}
	var ultimo *botonTriaje
	for _, bt := range r.triaje {
		if bt.Visible() {
			ultimo = bt
		}
	}
	ultimo.FocusGained()
	if x := ultimo.Position().X + LadoBotonTriaje; r.tira.Desplazamiento() == 0 || x > r.tira.Size().Width+0.01 {
		t.Errorf("el último enfocado acaba en %g de una tira de %g (desplazada %g)", x, r.tira.Size().Width, r.tira.Desplazamiento())
	}

	barra := nuevaBarraSeleccion(traductor(t), &Lote{
		Leer: func() {}, NoLeer: func() {}, Archivar: func() {}, Papelera: func() {}, Spam: func() {}, Purgar: func() {},
		Etiquetar: func(string) {}, Etiquetas: []Etiqueta{{ID: "e1", Nombre: "Obra"}},
	})
	barra.Cuantas = 3
	rb := test.WidgetRenderer(barra).(*barraSeleccionRenderer)
	barra.Resize(fyne.NewSize(ui.AnchoPanelBase, AltoBarraSeleccion))
	rb.Layout(barra.Size())
	if len(rb.acciones) != 7 || rb.tira.AnchoContenido() <= rb.tira.Size().Width {
		t.Fatalf("la barra: %d acciones, %g en %g", len(rb.acciones), rb.tira.AnchoContenido(), rb.tira.Size().Width)
	}
	etiquetar := rb.acciones[6]
	if !etiquetar.Visible() {
		t.Fatal("«Etiquetar» se esconde")
	}
	etiquetar.FocusGained()
	if x := etiquetar.Position().X + etiquetar.Size().Width; x > rb.tira.Size().Width+0.01 {
		t.Errorf("«Etiquetar» enfocado acaba en %g de una tira de %g", x, rb.tira.Size().Width)
	}
}
