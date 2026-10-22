// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func cajaCast(id, html string) Caja { return Caja{ID: id, W: 10, H: 10, HTML: html} }

// Los mismos vectores que `test/slides-cast.test.ts` de la web: el título que se envía a
// Cast es el mismo en las dos superficies.
func TestTituloCastEsElDeLaWeb(t *testing.T) {
	if got := textoPlano("<h1>Cuentas &amp; <b>pozo</b></h1><p>a&nbsp;b</p>"); got != "Cuentas & pozo a b" {
		t.Errorf("textoPlano: %q", got)
	}
	if got := textoPlano("<p>uno<br>dos</p>"); got != "uno dos" {
		t.Errorf("textoPlano br: %q", got)
	}
	casos := []struct {
		nombre string
		d      Diapo
		quiere string
	}{
		{"encabezado manda", Diapo{Cajas: []Caja{cajaCast("a", "<p>cuerpo</p>"), cajaCast("b", "<h1>Presupuesto</h1>")}}, "Presupuesto"},
		{"primera línea", Diapo{Cajas: []Caja{cajaCast("a", "<p>Primera línea</p><p>segunda</p>")}}, "Primera línea"},
		{"una forma no es título", Diapo{Cajas: []Caja{{ID: "a", Tipo: "shape", HTML: "<h1>Forma</h1>"}}}, ""},
		{"encabezado vacío", Diapo{Cajas: []Caja{cajaCast("a", "<h1></h1>"), cajaCast("b", "<p>Resto</p>")}}, "Resto"},
		{"vacía", Diapo{}, ""},
	}
	for _, c := range casos {
		if got := TituloCast(c.d); got != c.quiere {
			t.Errorf("%s: %q, quiere %q", c.nombre, got, c.quiere)
		}
	}
	largo := Diapo{Cajas: []Caja{cajaCast("a", "<h1>"+strings.Repeat("ñ", 500)+"</h1>")}}
	if n := len([]rune(TituloCast(largo))); n != MaxTituloCast {
		t.Errorf("el título mide %d runas", n)
	}
}

// La posición lleva número, total, título y origen: nunca notas ni contenido.
func TestPosicionCastSoloLlevaLaPosicion(t *testing.T) {
	m := &Modelo{Diapos: []Diapo{
		{ID: "a", Cajas: []Caja{cajaCast("a", "<h1>Portada</h1><p>secreto</p>")}, Notas: "decir que cuesta 40.000"},
		{ID: "b"},
	}}
	p, ok := PosicionCast(m, 0, "abc-1")
	if !ok || p != (api.CastPosicion{Index: 0, Total: 2, Title: "Portada", Ref: "docs:abc-1"}) {
		t.Fatalf("posición: %+v %v", p, ok)
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), "40.000") || strings.Contains(string(b), "secreto") {
		t.Errorf("sale más que la posición: %s", b)
	}
	if _, ok := PosicionCast(m, 2, "x"); ok {
		t.Error("una diapositiva que no existe no tiene posición")
	}
	if p, _ := PosicionCast(m, 1, "con espacio"); p.Ref != "" {
		t.Errorf("una referencia que Cast rechazaría no se envía: %q", p.Ref)
	}
}

// El avisador manda de una en una y, si se pasa deprisa, solo la última.
func TestAvisadorNoAmontona(t *testing.T) {
	var cola []func()
	var enviadas []int
	fallos := 0
	fallar := true
	av := NuevoAvisador(
		func(p api.CastPosicion) error {
			enviadas = append(enviadas, p.Index)
			if fallar {
				return io.ErrUnexpectedEOF
			}
			return nil
		},
		func() { fallos++ },
		func(f func()) { cola = append(cola, f) }, // la petición queda «en vuelo» hasta correrla
	)
	av(api.CastPosicion{Index: 0})
	av(api.CastPosicion{Index: 1})
	av(api.CastPosicion{Index: 2})
	if len(cola) != 1 || len(enviadas) != 0 {
		t.Fatalf("con una petición en vuelo no sale otra: cola %d, enviadas %v", len(cola), enviadas)
	}
	correr := func() {
		f := cola[0]
		cola = cola[1:]
		f()
	}
	correr() // sale la 0; al acabar, pendiente es la 2 y no la 1
	correr()
	if len(enviadas) != 2 || enviadas[0] != 0 || enviadas[1] != 2 || len(cola) != 0 {
		t.Errorf("enviadas %v, cola %d", enviadas, len(cola))
	}
	if fallos != 1 {
		t.Errorf("un fallo por racha, no uno por petición: %d", fallos)
	}
	fallar = false
	av(api.CastPosicion{Index: 3})
	correr()
	fallar = true
	av(api.CastPosicion{Index: 4})
	correr()
	if fallos != 2 {
		t.Errorf("tras un éxito, una racha nueva avisa otra vez: %d", fallos)
	}
}

// De punta a punta: presentar con Cast abre la emisión, manda la diapositiva de salida y
// cada cambio de diapositiva (no los pasos), con la referencia del documento.
func TestPresentarConCastAvisaDeCadaDiapositiva(t *testing.T) {
	var mu sync.Mutex
	var vistas []api.CastPosicion
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/lives/v9/slide" {
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
		var p api.CastPosicion
		_ = json.NewDecoder(r.Body).Decode(&p)
		mu.Lock()
		vistas = append(vistas, p)
		mu.Unlock()
		_, _ = io.WriteString(w, `{"index":0,"total":1,"title":"","marks":[]}`)
	}))
	defer srv.Close()

	e, _, _ := montar(t, `{"slides":[{"id":"a","boxes":[{"id":"x","x":0,"y":0,"w":50,"h":20,"html":"<h1>Uno</h1>"}]},{"id":"b","boxes":[]}]}`)
	abierto := ""
	e.Cast = &Cast{
		Cliente: api.NewCast(srv.URL, nil, nil), DocID: "d1",
		Abrir:  func(id string) { abierto = id },
		Lanzar: func(f func()) { f() }, // sin rutinas: la prueba es determinista
	}
	p := e.empezarCast("v9")
	if abierto != "v9" {
		t.Errorf("no se abrió la página de emisión: %q", abierto)
	}
	if len(vistas) != 1 || vistas[0] != (api.CastPosicion{Index: 0, Total: 2, Title: "Uno", Ref: "docs:d1"}) {
		t.Fatalf("la salida no avisó de la primera diapositiva: %+v", vistas)
	}
	p.Avanzar(1)
	if len(vistas) != 2 || vistas[1].Index != 1 || vistas[1].Total != 2 || vistas[1].Title != "" {
		t.Fatalf("el cambio de diapositiva no se avisó: %+v", vistas)
	}
	p.Ir(p.Diapo, p.Paso) // quedarse donde está no avisa
	if len(vistas) != 2 {
		t.Errorf("avisó sin cambiar de diapositiva: %+v", vistas)
	}
}

// Sin cliente de Cast no hay botón: la web tampoco lo ofrece sin Cast detrás.
func TestSinCastNoHayBotonDeCast(t *testing.T) {
	e, _, _ := montar(t, `{"slides":[{"id":"a","boxes":[]}]}`)
	e.PresentarConCast() // no hace nada ni paraliza
	if e.Cast != nil {
		t.Fatal("sin cliente no hay Cast")
	}
}
