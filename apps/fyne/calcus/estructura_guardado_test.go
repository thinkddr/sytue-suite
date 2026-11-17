// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
)

// Los cambios de estructura (insertar o borrar filas o columnas, o deshacerlo) van cada
// uno en SU guardado, enseguida y con lo pendiente de antes en otro: en una hoja
// protegida el servidor solo reconoce un desplazamiento por guardado
// (apps/nucleo/office/model/proteccion_ajenas.go), y dos inserciones que caían en el
// mismo autoguardado se rechazaban. Es lo mismo que hace la web (`useDocument`).
//
// VISTO EN ROJO sin la cola de app.go: las dos inserciones esperaban al reloj y salían
// juntas en un guardado con dos desplazamientos, que el servidor rechaza.

// libroProtegidoT es «Datos», sin proteger, y «Resumen», protegida sin permisos, con
// dos fórmulas bloqueadas que citan los datos: al insertar filas en «Datos» siguen a su
// dato, y el servidor lo acepta solo si UN desplazamiento lo explica.
const libroProtegidoT = `{"active":0,"sheets":[` +
	`{"name":"Datos","cells":{"A1":"Zona","A2":"Norte","A3":"Sur","A4":"Este","A5":"Oeste","A6":"Centro"}},` +
	`{"name":"Resumen","cells":{"A1":"=Datos!A3","A2":"=Datos!A6"},"protect":{}}]}`

// servicioEstructura apunta, en orden, el registro entero de cada guardado. Un parche se
// contesta con 400, así SaveChecked manda el registro entero, que es lo que se mira.
type servicioEstructura struct {
	mu        sync.Mutex
	hoja      string
	lamport   uint64
	dot       string
	guardados []string
}

func (s *servicioEstructura) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/documents/h1":
		_, _ = io.WriteString(w, `{"document":{"id":"h1","kind":"spreadsheet","title":"Cuentas"},"role":"editor"}`)
	case "GET /api/v1/documents/h1/state":
		valor, _ := json.Marshal(s.hoja)
		_, _ = io.WriteString(w, `{"seq":3,"state":{"lamport":`+itoa(s.lamport)+`,"registers":{"doc:sheet":{"value":`+
			string(valor)+`,"dot":`+s.dot+`,"lamport":`+itoa(s.lamport)+`}}}}`)
	case "POST /api/v1/documents/h1/ops":
		var cuerpo struct {
			Ops []struct {
				Kind    string          `json:"kind"`
				Key     string          `json:"key"`
				Value   json.RawMessage `json:"value"`
				Lamport uint64          `json:"lamport"`
				Dot     json.RawMessage `json:"dot"`
			} `json:"ops"`
		}
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		for _, op := range cuerpo.Ops {
			if op.Kind == "patch" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var v string
			if op.Kind == "set" && op.Key == "doc:sheet" && json.Unmarshal(op.Value, &v) == nil {
				s.hoja, s.lamport, s.dot = v, op.Lamport, string(op.Dot)
				s.guardados = append(s.guardados, v)
			}
		}
		_, _ = io.WriteString(w, `{"seq":4}`)
	case "GET /api/v1/documents/h1/comments":
		_, _ = io.WriteString(w, `{"comments":[]}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// abrirProtegidaT abre la hoja protegida con las rutinas de fondo en espera: `correr`
// las lanza en el orden en que se pidieron, que es cuando la prueba quiere.
func abrirProtegidaT(t *testing.T) (s *servicioEstructura, ab *abierto, correr func()) {
	t.Helper()
	s = &servicioEstructura{hoja: libroProtegidoT, lamport: 4, dot: `{"a":"web","c":2}`}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	a := NuevaApp(traductorPlay(t), "es", api.NewDocs(srv.URL, sesionFija{}, srv.Client()))
	a.lanzar = func(f func()) { f() }
	a.portada.Lanzar = a.lanzar
	a.AbrirID("h1", "")
	ab = a.abierto
	if ab.ctx.SoloLectura || ab.auto == nil {
		t.Fatal("el libro se abre en solo lectura")
	}
	ab.auto.Parar()
	var pendientes []func()
	a.lanzar = func(f func()) { pendientes = append(pendientes, f) }
	correr = func() {
		for len(pendientes) > 0 {
			f := pendientes[0]
			pendientes = pendientes[1:]
			f()
		}
		ab.auto.Parar() // el reloj que haya armado AlCambiar no sobrevive a la prueba
	}
	return s, ab, correr
}

// celdasGuardadasT son las celdas de la hoja `i` de lo guardado.
func celdasGuardadasT(t *testing.T, modelo string, i int) map[string]string {
	t.Helper()
	var m struct {
		Sheets []struct {
			Cells map[string]string `json:"cells"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal([]byte(modelo), &m); err != nil || len(m.Sheets) <= i {
		t.Fatalf("lo guardado no es el libro: %v", err)
	}
	return m.Sheets[i].Cells
}

// formulasResumenT son las dos fórmulas de «Resumen» en lo guardado.
func formulasResumenT(t *testing.T, modelo string) string {
	t.Helper()
	r := celdasGuardadasT(t, modelo, 1)
	return r["A1"] + " " + r["A2"]
}

// aceptaElServidor pasa los guardados, uno tras otro, por la validación de la protección
// del servidor, como lo haría quien no es el propietario. Devuelve el primer rechazo.
func aceptaElServidor(t *testing.T, inicial string, guardados ...string) error {
	t.Helper()
	op := func(v string, n uint64) []model.Op {
		b, _ := json.Marshal(v)
		return []model.Op{{Kind: model.OpSet, Key: "doc:sheet", Value: b, Dot: model.Dot{Actor: "c", Counter: n}, Lamport: n}}
	}
	res, err := model.AplicarProteccion(model.EstadoProteccion{Claves: map[string]model.ClaveHash{}}, op(inicial, 1), true, model.Proteccion{})
	if err != nil {
		t.Fatalf("el propietario protege la hoja: %v", err)
	}
	est := res.Estado
	for i, g := range guardados {
		res, err := model.AplicarProteccion(est, op(g, uint64(i+2)), false, model.Proteccion{Quien: model.Subject{Sub: "luis"}})
		if err != nil {
			return err
		}
		est = res.Estado
	}
	return nil
}

func TestDosInsercionesSeguidasVanEnDosGuardados(t *testing.T) {
	s, ab, correr := abrirProtegidaT(t)
	ab.lz.Rejilla.Seleccionar(0, 1, false) // A2
	ab.lz.Mandar(CmdInsFila, "above")
	// La segunda llega antes de que la primera se haya guardado, y en otro sitio: espera
	// su turno sin parar la interfaz.
	ab.lz.Rejilla.Seleccionar(0, 5, false) // A6, donde ha bajado «Oeste»
	ab.lz.Mandar(CmdInsFila, "above")
	if got := ab.lz.Libro.Texto(0, 7); got != "Centro" {
		t.Fatalf("tras las dos inserciones, A8 es %q: la interfaz no las ha aplicado", got)
	}
	correr()
	ab.auto.Ahora() // y lo que quedara, por el camino de siempre
	s.mu.Lock()
	guardados := append([]string(nil), s.guardados...)
	s.mu.Unlock()
	if len(guardados) != 2 {
		t.Fatalf("quiero dos guardados, uno por inserción, y son %d", len(guardados))
	}
	// Un desplazamiento en cada uno: las fórmulas de «Resumen» siguen a su dato.
	for i, quiero := range []string{"=Datos!A4 =Datos!A7", "=Datos!A4 =Datos!A8"} {
		if got := formulasResumenT(t, guardados[i]); got != quiero {
			t.Errorf("el guardado %d deja «Resumen» en %q; quiero %q", i+1, got, quiero)
		}
	}
	if ab.ctx.Guardado != editor.Limpio {
		t.Errorf("tras guardar las dos, el chip dice %v", ab.ctx.Guardado)
	}
	// Y el servidor acepta la secuencia con «Resumen» protegida; las dos juntas, no: no
	// hay un desplazamiento que mueva A3 una fila y A6 dos.
	if err := aceptaElServidor(t, libroProtegidoT, guardados...); err != nil {
		t.Errorf("el servidor rechaza los guardados de uno en uno: %v", err)
	}
	if err := aceptaElServidor(t, libroProtegidoT, guardados[1]); !errors.Is(err, model.ErrProtegido) {
		t.Errorf("las dos inserciones en un guardado: quiero ErrProtegido y es %v", err)
	}
}

// Lo escrito antes de insertar sale en su propio guardado, y la inserción en otro.
func TestLoPendienteSeGuardaAntesDeInsertar(t *testing.T) {
	s, ab, correr := abrirProtegidaT(t)
	ab.lz.Rejilla.Seleccionar(2, 0, false) // C1
	ab.lz.Rejilla.Escribir("nota")
	ab.lz.Rejilla.Seleccionar(0, 1, false)
	ab.lz.Mandar(CmdInsFila, "above")
	correr()
	s.mu.Lock()
	guardados := append([]string(nil), s.guardados...)
	s.mu.Unlock()
	if len(guardados) != 2 {
		t.Fatalf("quiero dos guardados, lo escrito y la inserción, y son %d", len(guardados))
	}
	if d := celdasGuardadasT(t, guardados[0], 0); d["C1"] != "nota" || d["A2"] != "Norte" ||
		formulasResumenT(t, guardados[0]) != "=Datos!A3 =Datos!A6" {
		t.Errorf("el primer guardado no es solo lo escrito: %v", d)
	}
	if d := celdasGuardadasT(t, guardados[1], 0); d["C1"] != "nota" || d["A3"] != "Norte" ||
		formulasResumenT(t, guardados[1]) != "=Datos!A4 =Datos!A7" {
		t.Errorf("el segundo guardado no es la inserción: %v", d)
	}
	if err := aceptaElServidor(t, libroProtegidoT, guardados...); err != nil {
		t.Errorf("el servidor rechaza los guardados: %v", err)
	}
}

// Deshacer y rehacer una inserción también son cambios de estructura.
func TestDeshacerUnaInsercionVaEnSuGuardado(t *testing.T) {
	s, ab, correr := abrirProtegidaT(t)
	ab.lz.Rejilla.Seleccionar(0, 1, false)
	ab.lz.Mandar(CmdInsFila, "above")
	ab.lz.Mandar(editor.Deshacer, "")
	ab.lz.Mandar(editor.Rehacer, "")
	correr()
	s.mu.Lock()
	guardados := append([]string(nil), s.guardados...)
	s.mu.Unlock()
	donde := make([]string, 0, len(guardados))
	for _, g := range guardados {
		donde = append(donde, celdasGuardadasT(t, g, 1)["A1"])
	}
	if strings.Join(donde, " ") != "=Datos!A4 =Datos!A3 =Datos!A4" {
		t.Errorf("Resumen!A1 en cada guardado: %v; quiero uno por paso", donde)
	}
	if err := aceptaElServidor(t, libroProtegidoT, guardados...); err != nil {
		t.Errorf("el servidor rechaza los guardados: %v", err)
	}
}
