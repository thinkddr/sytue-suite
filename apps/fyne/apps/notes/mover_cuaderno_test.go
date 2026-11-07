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
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
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
