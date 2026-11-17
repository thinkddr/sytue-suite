// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
)

// La contraseña de protección de hoja y libro la cumple el servidor, como en la web: el
// modelo lleva solo `hasPassword` y lo tecleado viaja aparte, en el `proteccion` del
// guardado. Estas pruebas hablan con un servicio de mentira que pasa cada guardado por
// la MISMA regla del servidor (model.AplicarProteccion), con su estado y sus hashes.
//
// VISTO EN ROJO con el arreglo deshecho: `hasPassword` se perdía al leer (la hoja de la
// web se veía sin contraseña y el servidor rechazaba el guardado), la contraseña salía
// en claro dentro de la operación, y desproteger se cotejaba —y fallaba— en el cliente.

// servicioClave es la hoja «h1» detrás de model.AplicarProteccion.
type servicioClave struct {
	mu      sync.Mutex
	rol     string
	hoja    string
	lamport uint64
	dot     model.Dot
	est     model.EstadoProteccion
	cuerpos []string // los de cada POST …/ops que llega
}

// nuevoServicioClave protege `inicial` como lo haría la web: el propietario la guarda
// con las contraseñas `nuevas` en el sobre, y el servidor deja solo su hash.
func nuevoServicioClave(t *testing.T, rol, inicial string, nuevas map[string]string) *servicioClave {
	t.Helper()
	b, _ := json.Marshal(inicial)
	dot := model.Dot{Actor: "web", Counter: 1}
	res, err := model.AplicarProteccion(model.EstadoProteccion{}, []model.Op{{Kind: model.OpSet, Key: "doc:sheet", Value: b, Dot: dot, Lamport: 1}},
		true, model.Proteccion{Nuevas: nuevas})
	if err != nil {
		t.Fatalf("la web no protege la hoja: %v", err)
	}
	var hoja string
	_ = json.Unmarshal(res.Ops[0].Value, &hoja)
	return &servicioClave{rol: rol, hoja: hoja, lamport: 1, dot: dot, est: res.Estado}
}

func (s *servicioClave) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/documents/h1":
		_, _ = io.WriteString(w, `{"document":{"id":"h1","kind":"spreadsheet","title":"Cuentas"},"role":"`+s.rol+`"}`)
	case "GET /api/v1/documents/h1/state":
		valor, _ := json.Marshal(s.hoja)
		dot, _ := json.Marshal(s.dot)
		_, _ = io.WriteString(w, `{"seq":3,"state":{"lamport":`+itoa(s.lamport)+`,"registers":{"doc:sheet":{"value":`+
			string(valor)+`,"dot":`+string(dot)+`,"lamport":`+itoa(s.lamport)+`}}}}`)
	case "POST /api/v1/documents/h1/ops":
		crudo, _ := io.ReadAll(r.Body)
		s.cuerpos = append(s.cuerpos, string(crudo))
		var cuerpo struct {
			Ops        []model.Op        `json:"ops"`
			Proteccion *model.Proteccion `json:"proteccion"`
		}
		_ = json.Unmarshal(crudo, &cuerpo)
		if slices.ContainsFunc(cuerpo.Ops, func(o model.Op) bool { return o.Kind == model.OpPatch }) {
			w.WriteHeader(http.StatusBadRequest) // sin parches: el registro entero
			return
		}
		var p model.Proteccion
		if cuerpo.Proteccion != nil {
			p = *cuerpo.Proteccion
		}
		res, err := model.AplicarProteccion(s.est, cuerpo.Ops, s.rol == "owner", p)
		switch {
		case errors.Is(err, model.ErrClaveProteccion):
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"code":"protection_password"}}`)
			return
		case errors.Is(err, model.ErrProtegido):
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"code":"protected"}}`)
			return
		}
		s.est = res.Estado
		for _, op := range res.Ops {
			if op.Kind == model.OpSet && op.Key == "doc:sheet" {
				_ = json.Unmarshal(op.Value, &s.hoja)
				s.lamport, s.dot = op.Lamport, op.Dot
			}
		}
		_, _ = io.WriteString(w, `{"seq":4}`)
	case "GET /api/v1/documents/h1/comments":
		_, _ = io.WriteString(w, `{"comments":[]}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// abrirClave abre «h1» con el guardado a mano: `guardar` es el autoguardado de ahora.
func abrirClave(t *testing.T, s *servicioClave) (ab *abierto, avisos *[]string, guardar func()) {
	t.Helper()
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
	return ab, avisosDe(ab.lz), func() {
		ab.auto.Ahora()
		ab.auto.Parar()
	}
}

// libroConClaveT es una hoja «H» con B1 desbloqueada y el libro, los dos protegidos.
const libroConClaveT = `{"active":0,"protect":{},"sheets":[{"name":"H","cells":{"A1":"dato"},` +
	`"styles":{"B1":{"locked":false}},"protect":{"sort":true}}]}`

// opsSinClave falla si alguna operación de lo enviado lleva `clave` o un `password`.
func opsSinClave(t *testing.T, cuerpos []string, clave string) {
	t.Helper()
	for _, c := range cuerpos {
		var cuerpo struct {
			Ops json.RawMessage `json:"ops"`
		}
		_ = json.Unmarshal([]byte(c), &cuerpo)
		if strings.Contains(string(cuerpo.Ops), clave) || strings.Contains(string(cuerpo.Ops), "password") {
			t.Errorf("la contraseña viaja dentro de las operaciones: %s", cuerpo.Ops)
		}
	}
}

// TestLaHojaConClaveDeLaWebConservaSuClave: una hoja y un libro protegidos con
// contraseña desde la web se abren CON ella —`hasPassword` se lee, se enseña y vuelve a
// salir tal cual— y quien no es el propietario sigue guardando lo que le dejan: el
// servidor no lee un `hasPassword` perdido como «quitar la contraseña».
func TestLaHojaConClaveDeLaWebConservaSuClave(t *testing.T) {
	s := nuevoServicioClave(t, "editor", libroConClaveT, map[string]string{"hoja:H": "buena", "libro": "otra"})
	if !strings.Contains(s.hoja, `"hasPassword":true`) || strings.Contains(s.hoja, "buena") {
		t.Fatalf("el servidor de mentira no guarda la hoja como la web: %s", s.hoja)
	}
	ab, _, guardar := abrirClave(t, s)
	st := ab.lz.Estado()
	if !st.Protegida || !st.ConClave || !st.ProtegidoLibro || !st.ConClaveLibro {
		t.Fatalf("la hoja de la web se abre sin su contraseña: %+v", st)
	}
	if g := ab.lz.Serializar(); !strings.Contains(g, `"protect":{"hasPassword":true,"sort":true}`) ||
		!strings.Contains(g, `"protect":{"hasPassword":true}`) {
		t.Errorf("`hasPassword` no vuelve a salir tal cual: %s", g)
	}
	ab.lz.Rejilla.Seleccionar(1, 0, false)
	if !ab.lz.Rejilla.Escribir("nuevo") {
		t.Fatal("B1 está desbloqueada y no se escribe")
	}
	guardar()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cuerpos) == 0 || !strings.Contains(s.hoja, "nuevo") {
		t.Fatalf("el servidor no acepta el guardado de quien no es el propietario: %v", s.cuerpos)
	}
	if h, ok := s.est.Claves["hoja:H"]; !ok || !h.Coincide("buena") || !strings.Contains(s.hoja, `"hasPassword":true`) {
		t.Error("el guardado ha quitado la contraseña de la hoja")
	}
}

// TestProtegerConClaveNoLaGuardaEnElModelo: la contraseña tecleada al proteger no entra
// nunca en el modelo —ni en el del lienzo ni en el que se envía—; sale aparte, en el
// sobre, el servidor la acepta (model.AplicarProteccion) y guarda solo su hash; y una
// vez recibida no se vuelve a mandar.
func TestProtegerConClaveNoLaGuardaEnElModelo(t *testing.T) {
	s := nuevoServicioClave(t, "editor", `{"active":0,"sheets":[{"name":"H","cells":{"A1":"dato"}}]}`, nil)
	ab, _, guardar := abrirClave(t, s)
	ab.lz.Mandar(CmdProteger, `{"password":"secreta","sort":true}`)
	ab.lz.Mandar(CmdProtegerLibro, `{"password":"delibro"}`)
	if g := ab.lz.Serializar(); strings.Contains(g, "secreta") || strings.Contains(g, "delibro") || strings.Contains(g, "password") ||
		!strings.Contains(g, `"protect":{"hasPassword":true,"sort":true}`) || !strings.Contains(g, `"protect":{"hasPassword":true}`) {
		t.Errorf("el modelo protegido no es el de la web: %s", g)
	}
	guardar()
	s.mu.Lock()
	cuerpos, hoja, claves := append([]string(nil), s.cuerpos...), s.hoja, s.est.Claves
	s.mu.Unlock()
	if len(cuerpos) != 1 {
		t.Fatalf("quiero un guardado y son %d", len(cuerpos))
	}
	opsSinClave(t, cuerpos, "secreta")
	opsSinClave(t, cuerpos, "delibro")
	var cuerpo struct {
		Proteccion model.Proteccion `json:"proteccion"`
	}
	_ = json.Unmarshal([]byte(cuerpos[0]), &cuerpo)
	if cuerpo.Proteccion.Nuevas["hoja:H"] != "secreta" || cuerpo.Proteccion.Nuevas["libro"] != "delibro" {
		t.Errorf("las contraseñas nuevas no van en el sobre: %s", cuerpos[0])
	}
	if h, ok := claves["hoja:H"]; !ok || !h.Coincide("secreta") || h.Coincide("otra") {
		t.Error("el servidor no guarda el hash de la contraseña de la hoja")
	}
	if h, ok := claves["libro"]; !ok || !h.Coincide("delibro") {
		t.Error("el servidor no guarda el hash de la contraseña del libro")
	}
	if strings.Contains(hoja, "secreta") || !strings.Contains(hoja, `"hasPassword":true`) {
		t.Errorf("lo guardado no es lo de la web: %s", hoja)
	}
	if p := ab.lz.Libro.Sobre(); p.Clave != "" || len(p.Nuevas) != 0 {
		t.Errorf("la contraseña se queda en el cliente después de guardar: %+v", p)
	}
}

// TestDesprotegerConClaveLaVerificaElServidor: quitar la protección con contraseña la
// presenta con el guardado y la verifica el servidor, como en la web. Con una que no es,
// el servidor lo rechaza, el lienzo vuelve a lo que hay y se dice «Contraseña
// incorrecta»; sin teclearla, ni se intenta; con la buena, queda quitada y sin hash.
func TestDesprotegerConClaveLaVerificaElServidor(t *testing.T) {
	s := nuevoServicioClave(t, "editor", libroConClaveT, map[string]string{"hoja:H": "buena"})
	ab, avisos, guardar := abrirClave(t, s)

	ab.lz.Mandar(CmdDesproteger, "")
	if ab.lz.Libro.Proteccion() == nil || len(*avisos) != 1 || (*avisos)[0] != "warning:sheet.msg.wrongPassword" {
		t.Fatalf("sin contraseña no se quita, y se dice: avisos %v", *avisos)
	}

	ab.lz.Mandar(CmdDesproteger, "mala")
	guardar()
	if p := ab.lz.Libro.Proteccion(); p == nil || !p.HasPassword {
		t.Fatal("con la contraseña equivocada la hoja queda desprotegida")
	}
	if len(*avisos) != 2 || (*avisos)[1] != "warning:sheet.msg.wrongPassword" {
		t.Errorf("la contraseña equivocada no se dice: avisos %v", *avisos)
	}
	if p := ab.lz.Libro.Sobre(); p.Clave != "" {
		t.Error("la contraseña rechazada se queda en el cliente")
	}
	s.mu.Lock()
	if _, ok := s.est.Claves["hoja:H"]; !ok || !strings.Contains(s.hoja, `"protect":{"hasPassword":true`) {
		t.Error("el servidor ha dejado quitar la protección con la contraseña equivocada")
	}
	s.mu.Unlock()

	ab.lz.Mandar(CmdDesproteger, "buena")
	guardar()
	s.mu.Lock()
	defer s.mu.Unlock()
	var guardada struct {
		Sheets []map[string]json.RawMessage `json:"sheets"`
	}
	_ = json.Unmarshal([]byte(s.hoja), &guardada)
	if _, sigue := guardada.Sheets[0]["protect"]; ab.lz.Libro.Proteccion() != nil || sigue {
		t.Errorf("con la contraseña buena no se quita: %s", s.hoja)
	}
	if _, ok := s.est.Claves["hoja:H"]; ok {
		t.Error("el servidor guarda el hash de una protección quitada")
	}
	opsSinClave(t, s.cuerpos, "buena")
	if !strings.Contains(s.cuerpos[len(s.cuerpos)-1], `"proteccion":{"password":"buena"}`) {
		t.Errorf("la contraseña no viaja en el sobre: %s", s.cuerpos[len(s.cuerpos)-1])
	}
}

// TestSinClaveSoloElPropietarioDesprotege: una protección sin contraseña solo la quita
// el propietario (`esPropietario` de la web); a los demás se les dice sin mandar nada.
func TestSinClaveSoloElPropietarioDesprotege(t *testing.T) {
	s := nuevoServicioClave(t, "editor", libroConClaveT, nil)
	ab, avisos, _ := abrirClave(t, s)
	ab.lz.Mandar(CmdDesproteger, "")
	ab.lz.Mandar(CmdDesprotegerLibro, "")
	if ab.lz.Libro.Proteccion() == nil || ab.lz.Libro.ProteccionLibro() == nil ||
		!slices.Equal(*avisos, []string{"warning:sheet.msg.protectedSheet", "warning:sheet.msg.protectedBook"}) {
		t.Errorf("quien no es el propietario quita una protección sin contraseña: avisos %v", *avisos)
	}

	s = nuevoServicioClave(t, "owner", libroConClaveT, map[string]string{"hoja:H": "buena"})
	ab, _, guardar := abrirClave(t, s)
	ab.lz.Mandar(CmdDesproteger, "")
	guardar()
	s.mu.Lock()
	defer s.mu.Unlock()
	if ab.lz.Libro.Proteccion() != nil {
		t.Error("el propietario no quita la protección")
	}
	if _, ok := s.est.Claves["hoja:H"]; ok {
		t.Error("el servidor no deja al propietario quitar la protección sin contraseña")
	}
}
