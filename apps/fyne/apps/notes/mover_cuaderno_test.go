// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

func cuadernoDe(id, padre string, pos int, soloLectura bool) Cuaderno {
	nb := api.Notebook{ID: id, Title: strings.ToUpper(id), Position: pos, ReadOnly: soloLectura, OwnerSub: "ada"}
	if padre != "" {
		nb.ParentID = ptr(padre)
	}
	return DeCuaderno(nb)
}

func idsCuadernos(cs []Cuaderno) string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return strings.Join(out, ",")
}

// TestDestinosDeCuaderno es el mismo caso que cuadernos.test.ts: ni él, ni su rama, ni
// su padre, ni uno de solo lectura. VISTO EN ROJO quitando el filtro del padre.
func TestDestinosDeCuaderno(t *testing.T) {
	cs := []Cuaderno{
		cuadernoDe("a", "", 0, false), cuadernoDe("hijo", "a", 0, false),
		cuadernoDe("nieto", "hijo", 0, false), cuadernoDe("b", "", 1, false),
		cuadernoDe("ajeno", "", 2, true),
	}
	for id, quiero := range map[string]string{"a": "b", "hijo": "b", "nieto": "a,b"} {
		if got := idsCuadernos(destinosDeCuaderno(cs, id)); got != quiero {
			t.Errorf("destinos de %s = %q; quiero %q", id, got, quiero)
		}
	}
}

// TestEntradaMoverCuaderno: al final de los hijos del nuevo padre o de la raíz, con el
// cuaderno entero; nada si ya está ahí o si sería colgarlo de su propia rama.
func TestEntradaMoverCuaderno(t *testing.T) {
	cs := []Cuaderno{
		cuadernoDe("a", "", 1024, false), cuadernoDe("hijo", "a", 1024, false),
		cuadernoDe("b", "", 2048, false), cuadernoDe("dentro-b", "b", 1024, false),
	}
	in, ok := entradaMoverCuaderno(cs, cs[1], "b")
	if !ok || in.ParentID == nil || *in.ParentID != "b" || *in.Position != 2048 || in.Title != "HIJO" {
		t.Fatalf("a «b»: %+v %v", in, ok)
	}
	in, ok = entradaMoverCuaderno(cs, cs[1], "")
	if !ok || in.ParentID != nil || *in.Position != 3072 {
		t.Fatalf("a la raíz: %+v %v", in, ok)
	}
	// Detrás de TODAS las raíces, también de una ajena de solo lectura: lo mismo que
	// `moverCuadernoA` de la web.
	conAjena := append(slices.Clone(cs), cuadernoDe("ajeno", "", 4096, true))
	if in, ok := entradaMoverCuaderno(conAjena, cs[1], ""); !ok || *in.Position != 5120 {
		t.Fatalf("a la raíz con una ajena detrás: %+v %v", in, ok)
	}
	if _, ok := entradaMoverCuaderno(cs, cs[0], ""); ok {
		t.Error("ya en la raíz no hay nada que mover")
	}
	if _, ok := entradaMoverCuaderno(cs, cs[0], "hijo"); ok {
		t.Error("colgarlo de su propio hijo sería un ciclo")
	}
}

// TestMoverCuadernoALaRaiz recorre la pantalla: el «…» de la página lo ofrece al dueño,
// el diálogo da el nivel superior y los destinos, y elegir manda el PUT con el padre
// nuevo. VISTO EN ROJO sin la entrada del menú y sin la fila del nivel superior.
func TestMoverCuadernoALaRaiz(t *testing.T) {
	falso := &servicioFalso{respuesta: map[string]string{
		"PUT /notebooks/hijo": `{"id":"hijo","title":"HIJO","owner_sub":"ada"}`,
	}}
	srv := httptest.NewServer(falso)
	t.Cleanup(srv.Close)
	w := test.NewWindow(nil)
	defer w.Close()
	p := NuevaPantalla(trInterno(t), "es", api.NewNotes(srv.URL, nil, nil), nil, w)
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.Yo = func() string { return "ada" }
	p.cuadernos = []Cuaderno{
		cuadernoDe("a", "", 1024, false), cuadernoDe("hijo", "a", 1024, false),
		cuadernoDe("b", "", 2048, false),
	}
	p.activo = p.cuadernos[1]
	p.sincronizar()

	mover := p.tr.T("notebook.moveTo", nil)
	if !slices.Contains(textosInternos(p.barra.menuMas()), mover) {
		t.Fatalf("el «…» de la página no ofrece %q", mover)
	}
	// De otra persona no se ofrece: el servicio no deja cambiarle el padre.
	p.activo = cuadernoDe("ajeno", "", 0, false)
	p.activo.Crudo.OwnerSub = "otra"
	p.sincronizar()
	if slices.Contains(textosInternos(p.barra.menuMas()), mover) {
		t.Fatal("ofrece mover un cuaderno de otra persona")
	}
	p.activo = p.cuadernos[1]
	p.sincronizar()

	p.dialogoMoverCuaderno(p.cuadernos[1])
	capa := w.Canvas().Overlays().Top()
	textos := textosInternos(capa)
	raiz := p.tr.T("notebook.toRoot", nil)
	if !slices.Contains(textos, raiz) || !slices.Contains(textos, "B") || slices.Contains(textos, "A") {
		t.Fatalf("el diálogo ofrece %q; quiero el nivel superior y «B», no su padre «A»", textos)
	}
	recorrerInterno(capa, func(x fyne.CanvasObject) {
		if o, ok := x.(*Opcion); ok && slices.Contains(textosInternos(o), raiz) {
			test.Tap(o)
		}
	})
	esperarVeces(t, falso, "PUT /notebooks/hijo", 1)
	cuerpo := falso.ultima(t, "PUT /notebooks/hijo")
	if cuerpo["parent_id"] != nil || cuerpo["title"] != "HIJO" || cuerpo["position"] != float64(3072) {
		t.Fatalf("el PUT lleva %v", cuerpo)
	}
}

// TestMoverCuadernoQueFallaLoDiceComoLaWeb: si el servicio rechaza el PUT, el aviso es
// `move.failed` («No se ha podido mover»), el mismo que la web, y no el de guardar.
// VISTO EN ROJO llamando otra vez a `guardarCuaderno`.
func TestMoverCuadernoQueFallaLoDiceComoLaWeb(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict) // un 5xx se reintenta
	}))
	t.Cleanup(srv.Close)
	w := test.NewWindow(nil)
	defer w.Close()
	p := NuevaPantalla(trInterno(t), "es", api.NewNotes(srv.URL, nil, nil), nil, w)
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.Yo = func() string { return "ada" }
	p.cuadernos = []Cuaderno{cuadernoDe("a", "", 1024, false), cuadernoDe("hijo", "a", 1024, false)}
	p.activo = p.cuadernos[1]
	p.sincronizar()

	p.dialogoMoverCuaderno(p.cuadernos[1])
	raiz := p.tr.T("notebook.toRoot", nil)
	recorrerInterno(w.Canvas().Overlays().Top(), func(x fyne.CanvasObject) {
		if o, ok := x.(*Opcion); ok && slices.Contains(textosInternos(o), raiz) {
			test.Tap(o)
		}
	})
	quiero := p.tr.T("move.failed", nil)
	for range 300 {
		var titulos []string
		fyne.DoAndWait(func() {
			for _, a := range p.avisos.Avisos() {
				titulos = append(titulos, a.Titulo)
			}
		})
		if slices.Contains(titulos, quiero) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no ha salido el aviso %q", quiero)
}

// pantallaDelArbol monta la pantalla con «a» abierto y «hijo» colgando de él SIN abrir,
// que es el caso que obligaba a abrir un cuaderno para poder moverlo.
func pantallaDelArbol(t *testing.T, srv *httptest.Server) (*Pantalla, fyne.Window) {
	t.Helper()
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	p := NuevaPantalla(trInterno(t), "es", api.NewNotes(srv.URL, nil, nil), nil, w)
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.Yo = func() string { return "ada" }
	p.cuadernos = []Cuaderno{
		cuadernoDe("a", "", 1024, false), cuadernoDe("hijo", "a", 1024, false),
		cuadernoDe("b", "", 2048, false),
	}
	p.activo = p.cuadernos[0]
	p.lateral.Fase = Exito
	fyne.DoAndWait(p.Repintar)
	return p, w
}

// masDeFila es el «…» de la fila de un cuaderno del árbol.
func masDeFila(t *testing.T, p *Pantalla, id string) (*ui.FilaLateral, *ui.BotonIcono) {
	t.Helper()
	var f *ui.FilaLateral
	fyne.DoAndWait(func() { f = p.lateral.filasCuaderno[id] })
	if f == nil {
		t.Fatalf("el árbol no pinta la fila de %q", id)
	}
	var mas *ui.BotonIcono
	recorrerInterno(f.Rama.Cola, func(x fyne.CanvasObject) {
		if b, ok := x.(*ui.BotonIcono); ok && b.Rotulo == p.tr.T("notebook.settings", nil) {
			mas = b
		}
	})
	if mas == nil {
		t.Fatalf("la fila de %q no lleva el «…»", id)
	}
	return f, mas
}

// elementosDelMenu son los textos del menú abierto encima de todo.
func elementosDelMenu(w fyne.Window) []string {
	var out []string
	recorrerInterno(w.Canvas().Overlays().Top(), func(x fyne.CanvasObject) {
		if e, ok := x.(*ui.ElementoMenu); ok {
			out = append(out, e.Texto)
		}
	})
	return out
}

// TestElMasDeLaFilaMueveUnCuadernoSinAbrirlo: el «…» de la fila de un cuaderno que NO
// está abierto ofrece lo mismo que la web —nuevo dentro, mover, ajustes— y «Mover
// cuaderno a…» manda el PUT sin pasar por abrirlo. VISTO EN ROJO con los dos botones
// de antes: la fila no tenía «…» ni menú.
func TestElMasDeLaFilaMueveUnCuadernoSinAbrirlo(t *testing.T) {
	falso := &servicioFalso{respuesta: map[string]string{
		"PUT /notebooks/hijo": `{"id":"hijo","title":"HIJO","owner_sub":"ada"}`,
	}}
	srv := httptest.NewServer(falso)
	t.Cleanup(srv.Close)
	p, w := pantallaDelArbol(t, srv)

	_, mas := masDeFila(t, p, "hijo")
	test.Tap(mas)
	tr := p.tr.T
	quiero := []string{tr("notebook.newChild", nil), tr("notebook.moveTo", nil), tr("notebook.settings", nil)}
	if got := elementosDelMenu(w); !slices.Equal(got, quiero) {
		t.Fatalf("el «…» de la fila ofrece %q; quiero %q", got, quiero)
	}
	recorrerInterno(w.Canvas().Overlays().Top(), func(x fyne.CanvasObject) {
		if e, ok := x.(*ui.ElementoMenu); ok && e.Texto == tr("notebook.moveTo", nil) {
			test.Tap(e)
		}
	})
	raiz := tr("notebook.toRoot", nil)
	if !slices.Contains(textosInternos(w.Canvas().Overlays().Top()), raiz) {
		t.Fatal("«Mover cuaderno a…» no ha abierto el diálogo de destinos")
	}
	recorrerInterno(w.Canvas().Overlays().Top(), func(x fyne.CanvasObject) {
		if o, ok := x.(*Opcion); ok && slices.Contains(textosInternos(o), raiz) {
			test.Tap(o)
		}
	})
	esperarVeces(t, falso, "PUT /notebooks/hijo", 1)
	if cuerpo := falso.ultima(t, "PUT /notebooks/hijo"); cuerpo["parent_id"] != nil {
		t.Fatalf("el PUT lleva %v; quiero el cuaderno en la raíz", cuerpo)
	}
	// Mover no abre: el abierto sigue siendo el que estaba, como en la web.
	esperarVeces(t, falso, "GET /notebooks", 1)
	var activo string
	fyne.DoAndWait(func() { activo = p.activo.ID })
	if activo != "a" {
		t.Fatalf("mover «hijo» ha abierto %q; el abierto tenía que seguir siendo «a»", activo)
	}
}

// TestElClicDerechoDeLaFilaAbreSuMenu: el clic derecho (y la pulsación larga, que Fyne
// entrega igual) abre el mismo menú que el «…». VISTO EN ROJO sin `AlPulsarSecundario`.
func TestElClicDerechoDeLaFilaAbreSuMenu(t *testing.T) {
	srv := httptest.NewServer(&servicioFalso{})
	t.Cleanup(srv.Close)
	p, w := pantallaDelArbol(t, srv)

	f, _ := masDeFila(t, p, "b")
	test.TapSecondary(f)
	if got := elementosDelMenu(w); !slices.Contains(got, p.tr.T("notebook.moveTo", nil)) {
		t.Fatalf("el clic derecho de la fila ofrece %q", got)
	}
}

// TestElMasDeLaFilaNoMueveNiUnDiaNiSinRed: con las mismas condiciones que el de la
// página, un día del diario y una pantalla sin red no ofrecen «Mover cuaderno a…»; las
// otras dos entradas siguen. VISTO EN ROJO ofreciéndolo siempre.
func TestElMasDeLaFilaNoMueveNiUnDiaNiSinRed(t *testing.T) {
	srv := httptest.NewServer(&servicioFalso{})
	t.Cleanup(srv.Close)
	p, w := pantallaDelArbol(t, srv)
	tr := p.tr.T

	// «a» es un día del diario CON hijos, que es como un día propio sigue en el árbol.
	p.cuadernos[0].Crudo.DailyDate = "2026-11-08"
	fyne.DoAndWait(p.Repintar)
	_, mas := masDeFila(t, p, "a")
	test.Tap(mas)
	got := elementosDelMenu(w)
	if slices.Contains(got, tr("notebook.moveTo", nil)) || !slices.Contains(got, tr("notebook.settings", nil)) {
		t.Fatalf("el «…» de un día del diario ofrece %q", got)
	}
	fyne.DoAndWait(func() { w.Canvas().Overlays().Remove(w.Canvas().Overlays().Top()) })

	p.red = &offline.Monitor{}
	p.red.Report(offline.Offline)
	fyne.DoAndWait(p.Repintar)
	_, mas = masDeFila(t, p, "hijo")
	test.Tap(mas)
	if got := elementosDelMenu(w); slices.Contains(got, tr("notebook.moveTo", nil)) {
		t.Fatalf("sin red el «…» ofrece %q", got)
	}
}

// TestLaFilaDelCuadernoNoSaltaAlPasarElRaton: el «…» cuenta para la medida aunque esté
// oculto, así que enseñarlo no cambia ni el alto ni el ancho de la cola. VISTO EN ROJO
// midiendo con `ui.Fila`, que se salta lo oculto.
func TestLaFilaDelCuadernoNoSaltaAlPasarElRaton(t *testing.T) {
	srv := httptest.NewServer(&servicioFalso{})
	t.Cleanup(srv.Close)
	p, _ := pantallaDelArbol(t, srv)

	f, _ := masDeFila(t, p, "b")
	var antes, cola fyne.Size
	fyne.DoAndWait(func() { antes, cola = ui.MedidaFilaLateral(f), f.Rama.Cola.MinSize() })
	if f.Rama.ColaAlPasar.Visible() && !ui.EsTactil() {
		t.Fatal("el «…» se ve en reposo")
	}
	f.MouseIn(&desktop.MouseEvent{})
	var despues, colaDespues fyne.Size
	fyne.DoAndWait(func() { despues, colaDespues = ui.MedidaFilaLateral(f), f.Rama.Cola.MinSize() })
	if !f.Rama.ColaAlPasar.Visible() {
		t.Fatal("al pasar el ratón no aparece el «…»")
	}
	if antes != despues || cola != colaDespues {
		t.Fatalf("la fila salta al pasar el ratón: %v → %v (cola %v → %v)", antes, despues, cola, colaDespues)
	}
}
