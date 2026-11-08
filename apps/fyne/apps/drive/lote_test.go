// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestEnTandasNoPasaDelCupoNiCortaPorUnFallo: lo que falla se cuenta aparte, el resto se
// hace, y nunca van más llamadas a la vez de las pedidas.
//
// VISTO EN ROJO (2026-11-05): con el cupo del tamaño de la lista, el pico sale 20; y
// apuntando el error como hecho, «b» no aparece entre los fallidos.
func TestEnTandasNoPasaDelCupoNiCortaPorUnFallo(t *testing.T) {
	hechos, fallidos := EnTandas([]string{"a", "b", "c", "d"}, func(x string) error {
		if x == "b" {
			return errors.New("403")
		}
		return nil
	}, 2)
	slices.Sort(hechos)
	if !slices.Equal(hechos, []string{"a", "c", "d"}) || !slices.Equal(fallidos, []string{"b"}) {
		t.Fatalf("hechos %q y fallidos %q", hechos, fallidos)
	}
	var enVuelo, pico atomic.Int32
	n := make([]int, 20)
	hechosN, _ := EnTandas(n, func(int) error {
		v := enVuelo.Add(1)
		for {
			p := pico.Load()
			if v <= p || pico.CompareAndSwap(p, v) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		enVuelo.Add(-1)
		return nil
	}, 3)
	if len(hechosN) != 20 || pico.Load() > 3 {
		t.Fatalf("hechos %d, pico %d (cupo 3)", len(hechosN), pico.Load())
	}
}

// TestLaPapeleraEnLoteSeDeshace: el aviso de «N elementos enviados a la papelera» dura
// los 10 s del de uno solo y trae su «Deshacer», que pide restaurar cada elemento con id
// por su clase; lo que el servicio niega se dice con su número.
//
// VISTO EN ROJO (2026-11-05): con el aviso de antes (`p.avisar` sin acción) el test dice
// que no hay «Deshacer»; y sin separar los fallidos, el aviso final es el de éxito.
func TestLaPapeleraEnLoteSeDeshace(t *testing.T) {
	var mu sync.Mutex
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta := r.Method + " " + r.URL.Path
		switch {
		case ruta == "GET /api/v1/folders/root":
			_, _ = io.WriteString(w, `{"id":"raiz","is_root":true}`)
			return
		case strings.HasPrefix(ruta, "GET "):
			_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
			return
		case ruta == "POST /api/v1/batch/trash":
			_, _ = io.WriteString(w, `{"trashed_files":2,"trashed_folders":1}`)
			return
		}
		mu.Lock()
		pedidas = append(pedidas, ruta)
		mu.Unlock()
		if ruta == "POST /api/v1/files/f2/restore" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	p := NuevaPantalla(traductor(t), api.NewDrive(srv.URL, sesionDoble{}, nil), nil, nil)
	sel := []Elemento{{ID: "f1", Nombre: "a.odt"}, {ID: "f2", Nombre: "b.odt"}, {ID: "c1", Nombre: "C", Carpeta: true}}

	p.mandarAPapelera(sel)
	esperar()
	avisos := p.avisos.Avisos()
	if len(avisos) == 0 {
		t.Fatal("la papelera en lote no avisa")
	}
	a := avisos[len(avisos)-1]
	if a.Titulo != p.tr.T("files.batch.trashDone", i18n.Vars{"count": 3}) || a.Duracion != ui.DuracionDeshacer ||
		a.RotuloAccion != p.tr.T("files.delete.undo", nil) || a.AlPulsarAccion == nil {
		t.Fatalf("el aviso del lote no trae su «Deshacer» de 10 s: %+v", a)
	}
	a.AlPulsarAccion()
	for i := 0; i < 200 && len(p.avisos.Avisos()) < 2; i++ {
		esperar()
	}
	mu.Lock()
	got := slices.Clone(pedidas)
	mu.Unlock()
	slices.Sort(got)
	quiero := []string{"POST /api/v1/files/f1/restore", "POST /api/v1/files/f2/restore", "POST /api/v1/folders/c1/restore"}
	if !slices.Equal(got, quiero) {
		t.Errorf("al servicio ha llegado %q, y se esperaba %q", got, quiero)
	}
	avisos = p.avisos.Avisos()
	if ultimo := avisos[len(avisos)-1]; ultimo.Titulo != p.tr.T("files.batch.restorePartial", i18n.Vars{"count": 1}) {
		t.Errorf("tras deshacer con uno negado se avisa «%s»", ultimo.Titulo)
	}
}

// TestVaciarLaPapeleraPreguntaYPurgaTodo: «Vaciar papelera» solo sale con algo que
// vaciar, pregunta antes de tocar nada y, al confirmar, purga cada elemento por su id y
// su clase; lo que el servicio niega se dice con su número.
//
// VISTO EN ROJO (2026-11-05): llamando a `vaciarPapelera` desde `AlVaciar` sin el
// diálogo, el test dice que se purgó sin preguntar; y con el botón también en una
// papelera vacía, que sale donde no hay nada.
// VISTO EN ROJO (2026-11-06): confirmar ya se hace pulsando el botón del diálogo; con su
// callback sin llamar a `vaciarPapelera`, el test dice que no se avisa de lo que queda, y
// pasándole solo el primer elemento, que al servicio no llega la purga de la carpeta.
func TestVaciarLaPapeleraPreguntaYPurgaTodo(t *testing.T) {
	var mu sync.Mutex
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta := r.Method + " " + r.URL.Path
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
			return
		}
		mu.Lock()
		pedidas = append(pedidas, ruta)
		mu.Unlock()
		if ruta == "DELETE /api/v1/folders/c1/purge" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	lista := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := slices.Clone(pedidas)
		slices.Sort(out)
		return out
	}
	w := test.NewWindow(nil)
	defer w.Close()
	p := NuevaPantalla(traductor(t), api.NewDrive(srv.URL, sesionDoble{}, nil), nil, w)
	w.SetContent(p.Objeto())
	v := p.variante()

	pap := p.papelera
	pap.Fase = Vacia
	if pap.BotonVaciar(v) != nil {
		t.Error("una papelera vacía ofrece «Vaciar papelera»")
	}
	pap.Fase = Exito
	pap.Elementos = []Elemento{{ID: "f1", Nombre: "a.odt"}, {ID: "c1", Nombre: "C", Carpeta: true}}
	b := pap.BotonVaciar(v)
	if b == nil || b.Texto != p.tr.T("trash.emptyAll", nil) || b.Inerte {
		t.Fatalf("con dos elementos no sale «Vaciar papelera» activo: %+v", b)
	}
	pap.Desconectado = true
	if !pap.BotonVaciar(v).Inerte {
		t.Error("sin red «Vaciar papelera» se puede pulsar")
	}
	pap.Desconectado = false

	test.Tap(b)
	esperar()
	if got := lista(); len(got) != 0 {
		t.Fatalf("pulsar «Vaciar papelera» purgó sin preguntar: %q", got)
	}
	dialogo := w.Canvas().Overlays().Top()
	if dialogo == nil {
		t.Fatal("pulsar «Vaciar papelera» no abre la confirmación")
	}

	// Se confirma pulsando el botón de peligro del diálogo, como lo haría la persona: así
	// se vigila también el cableado entre la confirmación y la purga.
	var confirmar *ui.Boton
	recorrer(dialogo, func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.Boton); ok && x.Texto == p.tr.T("trash.emptyAll", nil) {
			confirmar = x
		}
	})
	if confirmar == nil {
		t.Fatal("la confirmación no trae el botón de «Vaciar papelera»")
	}
	test.Tap(confirmar)
	quiero := []string{"DELETE /api/v1/files/f1/purge", "DELETE /api/v1/folders/c1/purge"}
	parcial := p.tr.T("trash.emptyAllPartial", i18n.Vars{"count": 1})
	ultimo := func() string {
		if a := p.avisos.Avisos(); len(a) > 0 {
			return a[len(a)-1].Titulo
		}
		return ""
	}
	for i := 0; i < 200 && ultimo() != parcial; i++ {
		esperar()
	}
	if got := lista(); !slices.Equal(got, quiero) {
		t.Errorf("al servicio ha llegado %q, y se esperaba %q", got, quiero)
	}
	if ultimo() != parcial {
		t.Error("vaciar con una purga negada no dice cuántos se quedan")
	}
}

// TestDeshacerConLaPapeleraDelanteLaRecarga: el «Deshacer» llega hasta 10 s después; si
// en ese rato se ha ido a la Papelera, lo que se vuelve a pedir es la papelera —que ya no
// tiene lo restaurado—, no la carpeta de la que se tiró.
//
// VISTO EN ROJO (2026-11-06): sin la rama de la Papelera en `recargarVista`, tras deshacer
// se pide la carpeta y la papelera no, y sigue enseñando como borrado lo restaurado.
func TestDeshacerConLaPapeleraDelanteLaRecarga(t *testing.T) {
	var mu sync.Mutex
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta := r.Method + " " + r.URL.Path
		mu.Lock()
		pedidas = append(pedidas, ruta)
		mu.Unlock()
		switch {
		case ruta == "GET /api/v1/trash":
			_, _ = io.WriteString(w, `{"files":[],"folders":[]}`)
		case strings.HasPrefix(ruta, "GET "):
			_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	p := NuevaPantalla(traductor(t), api.NewDrive(srv.URL, sesionDoble{}, nil), nil, nil)
	p.irA(Papelera)
	mu.Lock()
	pedidas = nil
	mu.Unlock()

	p.deshacerLote([]Elemento{{ID: "f1", Nombre: "a.odt"}})
	esperar()
	mu.Lock()
	got := slices.Clone(pedidas)
	mu.Unlock()
	if !slices.Contains(got, "POST /api/v1/files/f1/restore") || !slices.Contains(got, "GET /api/v1/trash") {
		t.Errorf("deshacer con la Papelera delante pidió %q: la papelera no se recarga", got)
	}
}
