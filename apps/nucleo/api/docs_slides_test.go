// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// slidesServidor es un Docs de mentira: guarda UN registro `doc:slides` y aplica los
// `set` que le llegan, como el CRDT del servicio. `visto` apunta cada petición.
type slidesServidor struct {
	t          *testing.T
	visto      []string
	valor      json.RawMessage // el valor del registro, tal cual (una cadena JSON)
	dot        SlidesDot
	lamport    uint64
	ultimaOp   map[string]any
	baseSeq    any  // el base_seq del último POST
	escrituras int  // POST que sí escribieron (los parches rechazados no cuentan)
	carrera    bool // el POST contesta 409: otro escribió entre la lectura y la escritura
}

func (f *slidesServidor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.visto = append(f.visto, r.Method+" "+r.URL.Path)
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/documents":
		if q := r.URL.Query(); q.Get("kind") != "presentation" || q.Get("limit") != "100" {
			f.t.Errorf("listado sin kind=presentation o sin limit: %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"documents":[{"id":"d1","kind":"presentation","title":"Resultados","updated_at":"2026-09-20T10:00:00Z"}],"next_cursor":"2026-09-20T10:00:00Z~d1"}`)
	case "POST /api/v1/documents":
		var cuerpo map[string]string
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		if cuerpo["kind"] != "presentation" {
			f.t.Errorf("se crea con kind=%q", cuerpo["kind"])
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"document":{"id":"d2","kind":"presentation","title":"`+cuerpo["title"]+` (2)"},"role":"owner"}`)
	case "GET /api/v1/documents/d1":
		_, _ = io.WriteString(w, `{"document":{"id":"d1","kind":"presentation","title":"Resultados"},"role":"viewer"}`)
	case "PATCH /api/v1/documents/d1", "DELETE /api/v1/documents/d1":
		w.WriteHeader(http.StatusNoContent)
	case "GET /api/v1/documents/d1/state":
		registros := map[string]any{}
		if f.valor != nil {
			registros["doc:slides"] = map[string]any{"value": f.valor, "dot": f.dot, "lamport": f.lamport}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"seq": 7, "state": map[string]any{"seq": 7, "lamport": f.lamport, "elements": nil, "registers": registros}})
	case "POST /api/v1/documents/d1/ops":
		var cuerpo struct {
			Ops []map[string]any `json:"ops"`
		}
		crudo, _ := io.ReadAll(r.Body)
		if strings.Contains(string(crudo), `"kind":"patch"`) { // un servicio sin parches: cae al registro entero
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"code":"bad_request","message":"Invalid editing operation."}}`)
			return
		}
		if err := json.Unmarshal(crudo, &cuerpo); err != nil || len(cuerpo.Ops) != 1 {
			f.t.Fatalf("cuerpo de ops ilegible: %s", crudo)
		}
		if strings.Contains(string(crudo), `\u003c`) {
			f.t.Error("el cuerpo escapa el HTML (\\u003c): infla el guardado contra el tope de 1 MiB")
		}
		// El guardado es condicional: base_seq es la seq que se cargó, y el servicio rechaza
		// con 409 si otro escribió la clave después (ApplyOps con baseSeq).
		var base struct {
			BaseSeq any `json:"base_seq"`
		}
		_ = json.Unmarshal(crudo, &base)
		f.baseSeq = base.BaseSeq
		if f.carrera {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"stale_seq","message":"stale"}}`)
			return
		}
		f.escrituras++
		f.ultimaOp = cuerpo.Ops[0]
		f.valor, _ = json.Marshal(f.ultimaOp["value"])
		_, _ = io.WriteString(w, `{"seq":8,"ops":[]}`)
	default:
		f.t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestSlidesRutas(t *testing.T) {
	falso := &slidesServidor{t: t}
	srv := httptest.NewServer(falso)
	defer srv.Close()
	s, ctx := NewSlides(srv.URL, nil, nil), context.Background()

	docs, siguiente, err := s.List(ctx, "", "")
	if err != nil || len(docs) != 1 || docs[0].Title != "Resultados" || siguiente == "" {
		t.Fatalf("List = %v, %q, %v", docs, siguiente, err)
	}
	if d, err := s.Create(ctx, "Charla"); err != nil || d.ID != "d2" || d.Title != "Charla (2)" {
		t.Fatalf("Create = %+v, %v: el título bueno es el que DEVUELVE el servicio", d, err)
	}
	if _, rol, err := s.Get(ctx, "d1"); err != nil || rol != "viewer" || SlidesPuedeEscribir(rol) {
		t.Fatalf("Get: rol %q, %v", rol, err)
	}
	if err := s.Rename(ctx, "d1", "Otro"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	quiero := []string{
		"GET /api/v1/documents", "POST /api/v1/documents", "GET /api/v1/documents/d1",
		"PATCH /api/v1/documents/d1", "DELETE /api/v1/documents/d1",
	}
	if strings.Join(falso.visto, ",") != strings.Join(quiero, ",") {
		t.Errorf("rutas = %v, quiero %v", falso.visto, quiero)
	}
}

// El contenido es una CADENA JSON dentro del registro (doble codificación, como
// JSON.stringify en la web), y guardar es UN `set` de ese registro por encima del reloj.
func TestSlidesGuardarPorElCaminoDeLaWeb(t *testing.T) {
	modelo := `{"slides":[{"id":"s1","boxes":[{"id":"b1","x":10,"y":10,"w":80,"h":20,"html":"<h1>Hola & adiós</h1>"}]}]}`
	envuelto, _ := json.Marshal(modelo)
	falso := &slidesServidor{t: t, valor: envuelto, dot: SlidesDot{Actor: "web", Counter: 3}, lamport: 41}
	srv := httptest.NewServer(falso)
	defer srv.Close()
	s, ctx := NewSlides(srv.URL, nil, nil), context.Background()

	base, err := s.Content(ctx, "d1")
	if err != nil || base.Modelo != modelo || base.Dot.Actor != "web" || base.Lamport != 41 || base.Seq != 7 {
		t.Fatalf("Content = %+v, %v", base, err)
	}
	nuevo := strings.Replace(modelo, "Hola", "Buenas", 1)
	guardado, err := s.Save(ctx, "d1", base, "gio-1", 1, nuevo)
	if err != nil {
		t.Fatal(err)
	}
	if falso.baseSeq != float64(7) {
		t.Errorf("base_seq = %v, quiero 7 (la seq que se cargó)", falso.baseSeq)
	}
	op := falso.ultimaOp
	if op["kind"] != "set" || op["key"] != "doc:slides" || op["value"] != nuevo || op["lamport"] != float64(42) {
		t.Errorf("op = %v", op)
	}
	if dot, _ := op["dot"].(map[string]any); dot["a"] != "gio-1" || dot["c"] != float64(1) {
		t.Errorf("dot = %v", op["dot"])
	}
	if guardado.Dot.Actor != "gio-1" || guardado.Lamport != 42 || guardado.Seq != 8 || guardado.Modelo != nuevo {
		t.Errorf("guardado = %+v", guardado)
	}

	// Otro escribe entretanto: el guardado siguiente NO sale y devuelve lo que hay.
	falso.dot, falso.lamport = SlidesDot{Actor: "web", Counter: 4}, 50
	antes := falso.escrituras
	ahora, err := s.Save(ctx, "d1", guardado, "gio-1", 2, modelo)
	if !errors.Is(err, ErrSlidesConflict) || ahora.Dot.Counter != 4 {
		t.Fatalf("Save con el registro cambiado = %+v, %v; quiero ErrSlidesConflict", ahora, err)
	}
	if falso.escrituras != antes {
		t.Error("ha escrito encima de un cambio ajeno")
	}

	// La carrera de milisegundos entre mirar y escribir: la ve el servicio (409) y es el
	// mismo conflicto, no un error de red.
	falso.dot, falso.lamport, falso.carrera = guardado.Dot, guardado.Lamport, true
	if _, err := s.Save(ctx, "d1", guardado, "gio-1", 3, modelo); !errors.Is(err, ErrSlidesConflict) {
		t.Fatalf("Save con 409 = %v; quiero ErrSlidesConflict", err)
	}
}

func TestSlidesSinContenidoYContenidoRaro(t *testing.T) {
	falso := &slidesServidor{t: t}
	srv := httptest.NewServer(falso)
	defer srv.Close()
	s := NewSlides(srv.URL, nil, nil)
	if c, err := s.Content(context.Background(), "d1"); err != nil || c.Modelo != "" || c.Dot != (SlidesDot{}) {
		t.Errorf("recién creada: %+v, %v", c, err)
	}
	// Un registro que no es una cadena no es «vacío»: tratarlo así dejaría guardar encima.
	falso.valor = json.RawMessage(`{"slides":[]}`)
	if _, err := s.Content(context.Background(), "d1"); err == nil {
		t.Error("un registro que no es cadena se ha dado por bueno")
	}
	// Y lo que no cabe en el cuerpo del servicio no se manda.
	falso.valor = nil
	if _, err := s.Save(context.Background(), "d1", SlidesContent{}, "a", 1, strings.Repeat("x", slidesMaxCuerpo)); !errors.Is(err, ErrSlidesTooBig) {
		t.Errorf("Save de 1 MiB = %v, quiero ErrSlidesTooBig", err)
	}
}

// Rutas, campos y la clave del registro son los del SERVICIO y los de la web.
func TestSlidesContratoConElServicio(t *testing.T) {
	rutas := webref.Read(t, "services/docs/internal/http/handler.go")
	for _, literal := range []string{
		`pr.Route("/api/v1/documents"`, `dr.Get("/", h.listDocuments)`, `dr.Post("/", h.createDocument)`,
		`idr.Get("/", h.getDocument)`, `idr.Patch("/", h.renameDocument)`, `idr.Delete("/", h.deleteDocument)`,
		`idr.Get("/state", h.getState)`, `idr.Post("/ops", h.postOps)`,
	} {
		if !strings.Contains(rutas, literal) {
			t.Errorf("services/docs ya no declara %s", literal)
		}
	}
	for fichero, literales := range map[string][]string{
		"services/docs/internal/http/documents.go": {
			`json:"kind"`, `json:"title"`, `json:"document"`, `json:"role,omitempty"`,
			`q.Get("kind")`, `q.Get("q")`, `q.Get("cursor")`, `q.Get("limit")`, `resp["next_cursor"]`, `"documents": docs`,
		},
		"services/docs/internal/http/collab.go": {`json:"ops"`, `"seq":   seq`, `"state": state`, `"seq": newSeq`},
		"services/docs/internal/domain/crdt.go": {
			`json:"a"`, `json:"c"`, `json:"dot"`, `json:"key,omitempty"`, `json:"value,omitempty"`, `json:"lamport"`,
			`json:"registers"`, `OpSet OpKind = "set"`,
		},
		"services/docs/internal/domain/doc.go": {
			`json:"owner_sub"`, `json:"starred"`, `json:"updated_at"`, `json:"source_ref,omitempty"`,
			`KindPresentation Kind = "presentation"`, `RoleEditor    Role = "editor"`, `RoleOwner     Role = "owner"`,
		},
		"apps/frontend/packages/office/src/editor/types.ts": {`presentation: 'doc:slides'`},
	} {
		texto := webref.Read(t, fichero)
		for _, l := range literales {
			if !strings.Contains(texto, l) {
				t.Errorf("%s ya no dice %s: revisar internal/api/docs_slides.go", fichero, l)
			}
		}
	}
}
