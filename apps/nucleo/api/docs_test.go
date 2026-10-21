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
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestDocs(t *testing.T) {
	var ops string // el cuerpo del último POST …/ops
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/documents":
			if got := r.URL.RawQuery; got != "cursor=c1&kind=document&limit=100&q=acta&starred=1" {
				t.Errorf("filtro: %s", got)
			}
			_, _ = io.WriteString(w, `{"documents":[{"id":"d1","kind":"document","title":"Acta","owner_sub":"u1","seq":7,"starred":true,"created_at":"2026-09-20T10:00:00Z","updated_at":"2026-09-21T10:00:00Z","source_ref":"drive:f1"}],"next_cursor":"c2"}`)
		case "POST /api/v1/documents":
			cuerpo, _ := io.ReadAll(r.Body)
			if string(cuerpo) != `{"kind":"document","title":"Nuevo"}` {
				t.Errorf("alta: %s", cuerpo)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"document":{"id":"d2","kind":"document","title":"Nuevo"},"role":"owner"}`)
		case "GET /api/v1/documents/d1":
			_, _ = io.WriteString(w, `{"document":{"id":"d1","kind":"document","title":"Acta"},"role":"viewer"}`)
		case "PATCH /api/v1/documents/d1":
			cuerpo, _ := io.ReadAll(r.Body)
			if string(cuerpo) != `{"title":"Acta de junio"}` {
				t.Errorf("renombrar: %s", cuerpo)
			}
			w.WriteHeader(http.StatusNoContent)
		case "PUT /api/v1/documents/d1/star":
			cuerpo, _ := io.ReadAll(r.Body)
			if string(cuerpo) != `{"starred":false}` {
				t.Errorf("destacar: %s", cuerpo)
			}
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /api/v1/documents/d1", "POST /api/v1/documents/d1/versions/v1/restore":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/v1/documents/d1/state":
			_, _ = io.WriteString(w, `{"seq":7,"state":{"seq":7,"lamport":12,"elements":[{"dot":{"a":"x","c":1},"value":"h"}],"registers":{"doc:html":{"value":"<p>Hola</p>","dot":{"a":"web","c":4},"lamport":9},"doc:html:lww":{"value":9,"dot":{"a":"web","c":4},"lamport":9}}}}`)
		case "POST /api/v1/documents/d1/ops":
			cuerpo, _ := io.ReadAll(r.Body)
			ops = string(cuerpo)
			_, _ = io.WriteString(w, `{"seq":9,"ops":[]}`)
		case "GET /api/v1/documents/d1/versions":
			_, _ = io.WriteString(w, `{"versions":[{"id":"v1","label":"Antes de la reunión","author_sub":"u1","seq":5,"auto":false,"created_at":"2026-09-20T10:00:00Z"},{"id":"v0","author_sub":"u1","seq":2,"auto":true,"created_at":"2026-09-19T10:00:00Z"}]}`)
		case "GET /api/v1/documents/d1/comments":
			_, _ = io.WriteString(w, `{"comments":[{"id":"c1","author_sub":"u2","anchor":null,"body":"¿Seguro?","resolved":false,"created_at":"2026-09-20T10:00:00Z"},{"id":"c2","parent_id":"c1","author_sub":"u1","anchor":null,"body":"Sí","resolved":false,"created_at":"2026-09-20T11:00:00Z"}]}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	d, ctx := NewDocs(srv.URL, nil, nil), context.Background()

	docs, sig, err := d.List(ctx, DocsFilter{Kind: DocText, Query: "acta", Starred: true, Cursor: "c1", Limit: 100})
	if err != nil || len(docs) != 1 || sig != "c2" || !docs[0].Starred || docs[0].Kind != DocText || docs[0].UpdatedAt.IsZero() || docs[0].SourceRef != "drive:f1" {
		t.Fatalf("lista: %+v, %q, %v", docs, sig, err)
	}
	if nuevo, err := d.Create(ctx, DocText, "Nuevo"); err != nil || nuevo.ID != "d2" {
		t.Fatalf("alta: %+v, %v", nuevo, err)
	}
	if doc, papel, err := d.Get(ctx, "d1"); err != nil || doc.Title != "Acta" || papel != "viewer" || CanWrite(papel) || !CanWrite("editor") || !CanWrite("owner") || CanWrite("commenter") {
		t.Fatalf("documento: %+v, %q, %v", doc, papel, err)
	}
	for nombre, err := range map[string]error{
		"renombrar": d.Rename(ctx, "d1", "Acta de junio"), "destacar": d.Star(ctx, "d1", false),
		"borrar": d.Delete(ctx, "d1"), "restaurar": d.RestoreVersion(ctx, "d1", "v1"),
	} {
		if err != nil {
			t.Errorf("%s: %v", nombre, err)
		}
	}

	c, err := d.Content(ctx, "d1", DocContentKey(DocText))
	if err != nil || c.Value != "<p>Hola</p>" || c.Lamport != 9 || c.MaxLamport != 12 || c.Dot != (DocDot{"web", 4}) {
		t.Fatalf("contenido: %+v, %v", c, err)
	}
	if vacio, err := d.Content(ctx, "d1", DocContentKey(DocSheet)); err != nil || vacio.Value != "" || vacio.MaxLamport != 12 {
		t.Fatalf("un registro que no existe es el cero con el reloj del documento: %+v, %v", vacio, err)
	}

	seq, err := d.SaveContent(ctx, "d1", "doc:html", "<p>Hola, Ada</p>", DocDot{"gio", 1}, 13, true)
	if err != nil || seq != 9 {
		t.Fatalf("guardar: %d, %v", seq, err)
	}
	// La marca va ANTES que el contenido y con su mismo punto y reloj, como en useDocument.ts.
	quiero := `{"ops":[{"kind":"set","dot":{"a":"gio","c":1},"key":"doc:html:lww","value":13,"lamport":13},{"kind":"set","dot":{"a":"gio","c":1},"key":"doc:html","value":"\u003cp\u003eHola, Ada\u003c/p\u003e","lamport":13}]}`
	if ops != quiero {
		t.Errorf("lote autoritativo:\n%s\nquería:\n%s", ops, quiero)
	}
	var lote struct {
		Ops []struct{ Value any } `json:"ops"`
	}
	if err := json.Unmarshal([]byte(ops), &lote); err != nil || lote.Ops[1].Value != "<p>Hola, Ada</p>" {
		t.Errorf("el HTML tiene que llegar intacto tras el JSON: %+v, %v", lote, err)
	}
	if _, err := d.SaveContent(ctx, "d1", "doc:sheet", "{}", DocDot{"gio", 2}, 14, false); err != nil || strings.Contains(ops, ":lww") {
		t.Errorf("hojas y presentaciones van sin marca: %s, %v", ops, err)
	}

	versiones, err := d.Versions(ctx, "d1")
	if err != nil || len(versiones) != 2 || versiones[0].Label != "Antes de la reunión" || versiones[1].Label != "" || !versiones[1].Auto {
		t.Fatalf("versiones: %+v, %v", versiones, err)
	}
	comentarios, err := d.Comments(ctx, "d1", false)
	if err != nil || len(comentarios) != 2 || comentarios[0].ParentID != "" || comentarios[1].ParentID != "c1" {
		t.Fatalf("comentarios: %+v, %v", comentarios, err)
	}
}

func TestElContratoDeDocsEsElDelServicio(t *testing.T) {
	for fichero, literales := range map[string][]string{
		"services/docs/internal/http/handler.go": {
			`pr.Route("/api/v1/documents"`, `dr.Get("/", h.listDocuments)`, `dr.Post("/", h.createDocument)`,
			`idr.Get("/", h.getDocument)`, `idr.Patch("/", h.renameDocument)`, `idr.Delete("/", h.deleteDocument)`,
			`idr.Put("/star", h.starDocument)`, `idr.Get("/state", h.getState)`, `idr.Post("/ops", h.postOps)`,
			`idr.Get("/versions", h.listVersions)`, `idr.Post("/versions/{versionId}/restore", h.restoreVersion)`,
			`idr.Get("/comments", h.listComments)`,
		},
		"services/docs/internal/http/documents.go": {
			`q.Get("kind")`, `q.Get("q")`, `q.Get("starred") == "1"`, `q.Get("cursor")`, `q.Get("limit")`,
			`"documents": docs`, `resp["next_cursor"]`, "`json:\"document\"`", "`json:\"role,omitempty\"`",
			"`json:\"kind\"`", "`json:\"title\"`", "`json:\"starred\"`",
		},
		"services/docs/internal/http/collab.go":   {`"state": state`, "`json:\"ops\"`", `"seq": newSeq`},
		"services/docs/internal/http/versions.go": {`"versions": versions`},
		"services/docs/internal/http/comments.go": {`"comments": comments`},
		"services/docs/internal/domain/doc.go": {
			`KindDocument     Kind = "document"`, `KindSpreadsheet  Kind = "spreadsheet"`, `KindPresentation Kind = "presentation"`,
			`RoleEditor    Role = "editor"`, `RoleOwner     Role = "owner"`,
			"`json:\"owner_sub\"`", "`json:\"seq\"`", "`json:\"updated_at\"`", "`json:\"source_ref,omitempty\"`",
			"`json:\"label,omitempty\"`", "`json:\"auto\"`", "`json:\"parent_id,omitempty\"`", "`json:\"body\"`", "`json:\"resolved\"`",
		},
		"services/docs/internal/domain/crdt.go": {
			"`json:\"a\"`", "`json:\"c\"`", `OpSet OpKind = "set"`, "`json:\"key,omitempty\"`", "`json:\"value,omitempty\"`",
			"`json:\"lamport\"`", "`json:\"registers\"`",
		},
		// Dónde vive el contenido y cómo se marca un guardado que manda lo decide la WEB.
		"apps/frontend/packages/office/src/editor/types.ts":           {`document: 'doc:html'`, `spreadsheet: 'doc:sheet'`, `presentation: 'doc:slides'`},
		"apps/frontend/packages/office/src/editor/state/coedicion.ts": {"`${claveContenido}:lww`"},
		"apps/frontend/packages/office/src/editor/state/useDocument.ts": {
			`const marca = { kind: 'set', dot, key: claveLww(keyRef.current), value: lamport, lamport };`,
			`marcado ? [...chars, marca, set]`,
		},
	} {
		fuente := webref.Read(t, fichero)
		for _, literal := range literales {
			if !strings.Contains(fuente, literal) {
				t.Errorf("%s ya no dice %s: revisar internal/api/docs.go", fichero, literal)
			}
		}
	}
}

// SaveChecked es el guardado con la mirada de delante. Lo comparten Docus, Calcus y
// Diapos, así que el conflicto se prueba UNA vez, aquí.
func TestDocsSaveChecked(t *testing.T) {
	estado := `{"seq":5,"state":{"lamport":12,"registers":{"doc:sheet":{"value":"{}","dot":{"a":"web","c":4},"lamport":9}}}}`
	var ops string
	carrera := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/documents/d1/state":
			_, _ = io.WriteString(w, estado)
		case "POST /api/v1/documents/d1/ops":
			cuerpo, _ := io.ReadAll(r.Body)
			if strings.Contains(string(cuerpo), `"kind":"patch"`) { // un servicio sin parches: cae al registro entero
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":{"code":"bad_request","message":"Invalid editing operation."}}`)
				return
			}
			ops = string(cuerpo)
			if carrera { // otro escribió entre la lectura y el POST
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"error":{"code":"stale_seq","message":"stale"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"seq":9,"ops":[]}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	d, ctx := NewDocs(srv.URL, nil, nil), context.Background()

	base, err := d.Content(ctx, "d1", "doc:sheet")
	if err != nil {
		t.Fatal(err)
	}
	dot := DocDot{Actor: "gio-1", Counter: 1}
	res, err := d.SaveChecked(ctx, "d1", "doc:sheet", `{"active":0}`, base, dot, false)
	if err != nil {
		t.Fatalf("guardar: %v", err)
	}
	// El reloj tiene que GANAR al más alto que el documento ha visto (12), o el `set` no
	// se aplica y el guardado se pierde en silencio.
	if res.Lamport != 13 || res.Dot != dot || res.Value != `{"active":0}` || res.Seq != 9 {
		t.Errorf("registro resultante: %+v", res)
	}
	if !strings.Contains(ops, `"base_seq":5`) {
		t.Errorf("el guardado tiene que ir con base_seq de lo leído: %s", ops)
	}
	if !strings.Contains(ops, `"lamport":13`) || strings.Contains(ops, "doc:sheet:lww") {
		t.Errorf("ops = %s", ops)
	}

	// Ahora otro ha escrito entretanto: NO se manda nada y se dice por qué.
	estado = `{"state":{"lamport":20,"registers":{"doc:sheet":{"value":"{}","dot":{"a":"otro","c":1},"lamport":20}}}}`
	ops = ""
	if _, err := d.SaveChecked(ctx, "d1", "doc:sheet", `{"active":1}`, base, dot, false); !errors.Is(err, ErrDocConflicto) {
		t.Errorf("con el registro cambiado quiero ErrDocConflicto, no %v", err)
	}
	if ops != "" {
		t.Errorf("se ha escrito encima del trabajo ajeno: %s", ops)
	}

	// Y la carrera entre la lectura y la escritura: la ve el servicio y es el mismo conflicto.
	estado = `{"seq":5,"state":{"lamport":12,"registers":{"doc:sheet":{"value":"{}","dot":{"a":"web","c":4},"lamport":9}}}}`
	carrera = true
	if _, err := d.SaveChecked(ctx, "d1", "doc:sheet", `{"active":2}`, base, dot, false); !errors.Is(err, ErrDocConflicto) {
		t.Errorf("con 409 del servicio quiero ErrDocConflicto, no %v", err)
	}
}

// Los comentarios se escriben, se resuelven y se borran por las rutas del servicio
// (services/docs/internal/http/comments.go). Se afirma el método y la ruta.
func TestDocsComentarios(t *testing.T) {
	var visto []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		visto = append(visto, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+string(cuerpo))
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"comments":[{"id":"c1","author_sub":"u2","body":"¿Seguro?","resolved":false,"created_at":"2026-09-20T10:00:00Z"}]}`)
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"comment":{"id":"c2","parent_id":"c1","author_sub":"u1","body":"Sí","created_at":"2026-09-20T11:00:00Z"}}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	d, ctx := NewDocs(srv.URL, nil, nil), context.Background()

	if cs, err := d.Comments(ctx, "d1", true); err != nil || len(cs) != 1 || cs[0].Body != "¿Seguro?" {
		t.Fatalf("lista: %+v, %v", cs, err)
	}
	if c, err := d.CreateComment(ctx, "d1", "c1", "  Sí  "); err != nil || c.ID != "c2" || c.ParentID != "c1" {
		t.Fatalf("crear: %+v, %v", c, err)
	}
	// Un cuerpo en blanco no llega a salir: el servicio lo rechazaría igual.
	if _, err := d.CreateComment(ctx, "d1", "", "   "); !errors.Is(err, ErrComentarioVacio) {
		t.Errorf("un comentario vacío tiene que pararse aquí, no en el servidor: %v", err)
	}
	if _, err := d.CreateAnchoredComment(ctx, "d1", "Ojo", "m7", "la cita"); err != nil {
		t.Fatalf("crear anclado: %v", err)
	}
	if err := d.ResolveComment(ctx, "d1", "c1", true); err != nil {
		t.Fatalf("resolver: %v", err)
	}
	if err := d.DeleteComment(ctx, "d1", "c2"); err != nil {
		t.Fatalf("borrar: %v", err)
	}
	quiero := []string{
		"GET /api/v1/documents/d1/comments?resolved=true ",
		`POST /api/v1/documents/d1/comments? {"body":"Sí","parent_id":"c1"}`,
		`POST /api/v1/documents/d1/comments? {"anchor":{"mark":"m7","text":"la cita"},"body":"Ojo"}`,
		`PUT /api/v1/documents/d1/comments/c1/resolve? {"resolved":true}`,
		"DELETE /api/v1/documents/d1/comments/c2? ",
	}
	if !slices.Equal(visto, quiero) {
		t.Errorf("peticiones:\n%q\nquiero:\n%q", visto, quiero)
	}
}

// Exportar es de dos pasos, como en el BFF de la web: materializar y bajar en línea.
func TestDocsExportYVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/documents/d1/exports":
			if string(cuerpo) != `{"format":"ooxml"}` {
				t.Errorf("exportar: %s", cuerpo)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"export":{"id":"b1","format":"ooxml","content_type":"application/zip","size_bytes":3}}`)
		case "GET /api/v1/documents/d1/exports/b1":
			if r.URL.Query().Get("inline") != "true" {
				t.Error("la descarga tiene que ser en línea: la URL prefirmada es interna")
			}
			_, _ = io.WriteString(w, "PK!")
		case "POST /api/v1/documents/d1/versions":
			if string(cuerpo) != `{"label":"Antes de enviar"}` {
				t.Errorf("versión: %s", cuerpo)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"version":{"id":"v9","label":"Antes de enviar","seq":7}}`)
		default:
			t.Errorf("inesperado: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	d := NewDocs(srv.URL, nil, nil)
	datos, err := d.Export(context.Background(), "d1", "ooxml")
	if err != nil || string(datos) != "PK!" {
		t.Fatalf("exportar: %q, %v", datos, err)
	}
	v, err := d.CreateVersion(context.Background(), "d1", "  Antes de enviar ")
	if err != nil || v.ID != "v9" {
		t.Fatalf("versión: %+v, %v", v, err)
	}
	for _, c := range []struct {
		titulo string
		k      DocKind
		f      string
		quiero string
	}{
		{"Acta: junio/julio", DocText, "ooxml", "Acta_ junio_julio.docx"},
		{"  ", DocSheet, "odf", "documento.ods"},
		{"Plan", DocSlides, "markdown", "Plan.md"},
		{"Plan", DocText, "rar", "Plan.bin"},
	} {
		if got := ExportFilename(c.titulo, c.k, c.f); got != c.quiero {
			t.Errorf("ExportFilename(%q, %s, %s) = %q, quiero %q", c.titulo, c.k, c.f, got, c.quiero)
		}
	}
	if !slices.Equal(ExportFormats, []string{"pdf", "ooxml", "odf", "html", "markdown"}) ||
		!strings.Contains(webref.Read(t, "apps/frontend/packages/office/src/editor/types.ts"),
			`EXPORT_FORMATS = OFFICE_EXPORT_FORMATS`) ||
		webref.Read(t, "apps/frontend/packages/config/src/office-catalog.json") != webref.Read(t, "apps/nucleo/office/catalog.json") {
		t.Error("los formatos de exportación ya no son los de la web")
	}
}

// Exportar como plantilla conserva los dos pasos y pide template:true, como el BFF.
func TestDocsExportTemplateYPrefijosDeNombre(t *testing.T) {
	for _, formato := range []string{"ooxml", "odf"} {
		t.Run(formato, func(t *testing.T) {
			var peticiones atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				peticiones.Add(1)
				switch r.Method + " " + r.URL.Path {
				case "POST /api/v1/documents/d1/exports":
					cuerpo, _ := io.ReadAll(r.Body)
					if string(cuerpo) != `{"format":"`+formato+`","template":true}` {
						t.Errorf("plantilla: %s", cuerpo)
					}
					w.WriteHeader(http.StatusCreated)
					_, _ = io.WriteString(w, `{"export":{"id":"p1"}}`)
				case "GET /api/v1/documents/d1/exports/p1":
					if r.URL.Query().Get("inline") != "true" {
						t.Error("la plantilla debe descargarse en línea")
					}
					_, _ = io.WriteString(w, "PK!plantilla")
				default:
					t.Errorf("inesperado: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()
			datos, err := NewDocs(srv.URL, nil, srv.Client()).ExportTemplate(context.Background(), "d1", formato)
			if err != nil || string(datos) != "PK!plantilla" || peticiones.Load() != 2 {
				t.Fatalf("plantilla %q, peticiones %d: %v", datos, peticiones.Load(), err)
			}
		})
	}
	for _, caso := range []struct {
		tipo         DocKind
		formato, ext string
	}{
		{DocText, "ooxml", "dotx"},
		{DocSheet, "ooxml", "xltx"},
		{DocSlides, "ooxml", "potx"},
		{DocText, "odf", "ott"},
		{DocSheet, "odf", "ots"},
		{DocSlides, "odf", "otp"},
	} {
		if ext := ExportTemplateExt(caso.tipo, caso.formato); ext != caso.ext {
			t.Errorf("ext %s/%s: %q", caso.tipo, caso.formato, ext)
		}
		if nombre := ExportTemplateFilename("Acta: junio/julio", caso.tipo, caso.formato); nombre != "Acta_ junio_julio."+caso.ext {
			t.Errorf("nombre %s/%s: %q", caso.tipo, caso.formato, nombre)
		}
		if nombre := ExportTemplateFilename("  ", caso.tipo, caso.formato); nombre != "documento."+caso.ext {
			t.Errorf("nombre vacío: %q", nombre)
		}
	}
}

// El ancla la escribe la web de dos formas: el texto a secas o `{text, mark}`.
func TestAnclaDeComentario(t *testing.T) {
	for crudo, quiero := range map[string]string{
		`"la frase"`:                    "la frase",
		`{"text":" otra ","mark":"c1"}`: "otra",
		`null`:                          "",
		`{"mark":"c2"}`:                 "",
	} {
		if got := (DocComment{Anchor: json.RawMessage(crudo)}).AnchorText(); got != quiero {
			t.Errorf("AnchorText(%s) = %q, quiero %q", crudo, got, quiero)
		}
	}
}

// «Gestionar acceso» habla con las rutas del servicio que usa el BFF de la web.
func TestDocsSharing(t *testing.T) {
	var vistos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		vistos = append(vistos, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+string(cuerpo))
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/documents/d1/permissions":
			_, _ = io.WriteString(w, `{"permissions":[{"principal_kind":"user","principal_id":"ana@x.eu","role":"owner"},{"principal_kind":"tenant","principal_id":"","role":"viewer"}]}`)
		case "GET /api/v1/documents/d1/shares":
			_, _ = io.WriteString(w, `{"shares":[{"id":"s1","role":"viewer","expires_at":"2026-12-31T00:00:00Z"}]}`)
		case "POST /api/v1/documents/d1/shares":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"share":{"id":"s2","role":"editor","token":"tok"}}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	d, ctx := NewDocs(srv.URL, nil, nil), context.Background()
	perms, links, err := d.Sharing(ctx, "d1")
	if err != nil || len(perms) != 2 || perms[1].PrincipalKind != "tenant" || len(links) != 1 || links[0].ExpiresAt == nil {
		t.Fatalf("Sharing: %+v %+v %v", perms, links, err)
	}
	if err := d.Grant(ctx, "d1", "user", "luis@x.eu", "editor"); err != nil {
		t.Fatal(err)
	}
	if err := d.Revoke(ctx, "d1", "tenant", ""); err != nil {
		t.Fatal(err)
	}
	s, err := d.CreateShare(ctx, "d1", "editor")
	if err != nil || s.Token != "tok" {
		t.Fatalf("CreateShare: %+v %v", s, err)
	}
	if err := d.RevokeShare(ctx, "d1", "s1"); err != nil {
		t.Fatal(err)
	}
	quiero := []string{
		"GET /api/v1/documents/d1/permissions? ",
		"GET /api/v1/documents/d1/shares? ",
		`PUT /api/v1/documents/d1/permissions? {"principal_id":"luis@x.eu","principal_kind":"user","role":"editor"}`,
		"DELETE /api/v1/documents/d1/permissions?principal_id=&principal_kind=tenant ",
		`POST /api/v1/documents/d1/shares? {"role":"editor"}`,
		"DELETE /api/v1/documents/d1/shares/s1? ",
	}
	if !slices.Equal(vistos, quiero) {
		t.Errorf("peticiones:\n%s\nquería:\n%s", strings.Join(vistos, "\n"), strings.Join(quiero, "\n"))
	}
	for fichero, literales := range map[string][]string{
		"services/docs/internal/http/handler.go": {
			`idr.Get("/permissions", h.listPermissions)`, `idr.Put("/permissions", h.upsertPermission)`,
			`idr.Delete("/permissions", h.revokePermission)`, `idr.Get("/shares", h.listShares)`,
			`idr.Post("/shares", h.createShare)`, `idr.Delete("/shares/{linkId}", h.revokeShare)`,
		},
		"services/docs/internal/http/permissions.go": {`"permissions": perms`, `"shares": links`, `"share": link`, `Query().Get("principal_kind")`},
	} {
		fuente := webref.Read(t, fichero)
		for _, l := range literales {
			if !strings.Contains(fuente, l) {
				t.Errorf("%s ya no dice %s: revisar Sharing/Grant/Revoke", fichero, l)
			}
		}
	}
}

// «PDF accesible» apagado: el cliente pide el PDF simple con accessible:false; Export, que
// es el accesible por defecto, no manda el campo.
func TestDocsExportPDFSimplePideAccessibleFalse(t *testing.T) {
	var cuerpos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			cuerpos = append(cuerpos, string(b))
			_, _ = w.Write([]byte(`{"export":{"id":"b1"}}`))
			return
		}
		_, _ = w.Write([]byte("%PDF-"))
	}))
	defer srv.Close()
	d := NewDocs(srv.URL, nil, srv.Client())
	if _, err := d.ExportPDFSimple(context.Background(), "d1"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Export(context.Background(), "d1", "pdf"); err != nil {
		t.Fatal(err)
	}
	if len(cuerpos) != 2 || cuerpos[0] != `{"format":"pdf","accessible":false}` || cuerpos[1] != `{"format":"pdf"}` {
		t.Errorf("cuerpos = %q", cuerpos)
	}
}
