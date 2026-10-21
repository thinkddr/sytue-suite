// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestServiceURL(t *testing.T) {
	for producto, quiero := range map[string]string{
		"notes": "https://notes.sytue.eu", "sheets": "https://docs.sytue.eu",
		"console": "https://admin.sytue.eu", "meet": "https://rooms.sytue.eu",
	} {
		if got := ServiceURL(ApexProd, producto); got != quiero {
			t.Errorf("ServiceURL(%q) = %q, quiero %q", producto, got, quiero)
		}
	}
	// El reparto de hosts es el de tokens.ts: cada `clave: at('host')` tiene que estar.
	tokens := webref.Read(t, "apps/frontend/packages/config/src/tokens.ts")
	for producto, host := range hostDe {
		switch producto {
		case "integrations":
			continue // va aparte en tokens.ts, como `integrationsDomain`
		case "ai":
			continue // la web no la llama por host: va por su BFF, dentro del clúster
		case "photos":
			// El servicio NO es la web: `photos.` es el frontal y su BFF, y el servicio
			// Go publica su propio host. Se comprueba contra el chart que lo publica.
			// VISTO EN ROJO con "photos": "photos", que es lo que tenía.
			if chart := webref.Read(t, "deploy/helm/charts/syphotos/values.yaml"); !strings.Contains(chart, "subdomain: "+host+"\n") {
				t.Errorf("el servicio de fotos está en %q y el chart syphotos no publica ese subdominio", host)
			}
			continue
		}
		if web, ok := webDe[producto]; ok {
			host = web // la ofimática: la web en su host, la API en `docs.` (ADR 0076)
		}
		if !strings.Contains(tokens, producto+": at('"+host+"')") {
			t.Errorf("tokens.ts ya no dice %s: at('%s')", producto, host)
		}
	}
}

// Actualizar una nota manda la nota ENTERA: el PUT del servicio sustituye tipo, título,
// contenido, datos y enlaces, y un cliente que mandase solo el título borraría el resto.
func TestUpdateNoteNoPierdeNada(t *testing.T) {
	original := `{"id":"n1","notebook_id":"c1","kind":"todo","title":"Compra","content":"<p>pan</p>",
		"data":{"due":"2026-10-01"},"x":40,"y":80,"col":2,"position":5,"width":320,"group_id":"g1",
		"collapsed":true,"checked":true,"pinned":true,"visibility":"organization","indexable":false,
		"links":[{"target_kind":"drive_file","target_id":"f9","label":"ticket"}],
		"created_at":"2026-09-01T10:00:00Z","updated_at":"2026-09-02T10:00:00Z"}`
	var recibido map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/notebooks/c1/notes/page":
			_, _ = io.WriteString(w, `{"notes":[`+original+`]}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/notes/n1":
			_ = json.NewDecoder(r.Body).Decode(&recibido)
			_, _ = io.WriteString(w, original)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	notas := NewNotes(srv.URL, nil, nil)
	lista, err := notas.ListNotes(context.Background(), "c1")
	if err != nil || len(lista) != 1 {
		t.Fatalf("ListNotes = %v, %v", lista, err)
	}
	in := lista[0].Input()
	in.Title = "Compra del sábado" // lo ÚNICO que cambia
	if _, err := notas.UpdateNote(context.Background(), "n1", "c1", in); err != nil {
		t.Fatal(err)
	}
	var quiero map[string]any
	_ = json.Unmarshal([]byte(original), &quiero)
	quiero["title"] = "Compra del sábado"
	for _, soloLectura := range []string{"id", "created_at", "updated_at"} {
		delete(quiero, soloLectura)
	}
	for campo, valor := range quiero {
		a, _ := json.Marshal(valor)
		b, _ := json.Marshal(recibido[campo])
		if !bytes.Equal(a, b) {
			t.Errorf("al actualizar, %q viaja como %s y era %s", campo, b, a)
		}
	}
}

// Los nombres JSON son los del SERVICIO, no los que se recuerden: el escritorio Fyne leía
// `body` y el servicio escribe `content`.
func TestLosCamposSonLosDelServicio(t *testing.T) {
	dominio := webref.Read(t, "services/notes/internal/domain/domain.go")
	for _, campo := range []string{
		`json:"content,omitempty"`, `json:"content_cipher,omitempty"`, `json:"notebook_id"`, `json:"note_count"`,
		`json:"group_id,omitempty"`, `json:"target_kind"`, `json:"pinned"`, `json:"checked"`,
	} {
		if !strings.Contains(dominio, campo) {
			t.Errorf("services/notes ya no declara %s: revisar internal/api/notes.go", campo)
		}
	}
	if strings.Contains(dominio, `json:"body`) {
		t.Error("services/notes ha pasado a `body`: revisar internal/api/notes.go")
	}
}

// Las rutas de papelera, mover, etiquetas y retroenlaces son las del servicio, y el gesto
// que no devuelve nada (204) no se queda con el cuerpo sin cerrar. Este test afirma el
// método Y la ruta: un `/notes/{id}/trash` inventado daría 404 en producción y aquí sale
// como «petición inesperada».
func TestGestosDeNotaPeganDondeToca(t *testing.T) {
	var visto []string
	var cuerpoEstado, cuerpoMover map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visto = append(visto, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/notes/n1/state":
			_ = json.NewDecoder(r.Body).Decode(&cuerpoEstado)
		case "/api/v1/notes/n1/move":
			_ = json.NewDecoder(r.Body).Decode(&cuerpoMover)
		case "/api/v1/notes/trash":
			_, _ = io.WriteString(w, `{"notes":[{"id":"n9","title":"Vieja"}]}`)
			return
		case "/api/v1/tags":
			_, _ = io.WriteString(w, `{"tags":[{"id":"t1","name":"Prioridad","states":[{"id":"s1","tag_id":"t1","name":"Alta","color":"#f00"}]}]}`)
			return
		case "/api/v1/notes/linked":
			if q := r.URL.Query(); q.Get("target_kind") != "note" || q.Get("target_id") != "n1" {
				t.Errorf("retroenlaces con %v", q)
			}
			_, _ = io.WriteString(w, `{"notes":[{"id":"n2","title":"Apunta aquí"}]}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	n, ctx := NewNotes(srv.URL, nil, nil), context.Background()

	if err := n.DeleteNote(ctx, "n1"); err != nil {
		t.Fatal(err)
	}
	if err := n.RestoreNote(ctx, "n1"); err != nil {
		t.Fatal(err)
	}
	if err := n.PurgeNote(ctx, "n1"); err != nil {
		t.Fatal(err)
	}
	if err := n.MoveNoteToNotebook(ctx, "n1", "c2"); err != nil {
		t.Fatal(err)
	}
	if err := n.SetNoteState(ctx, "n1", "t1", "s1"); err != nil {
		t.Fatal(err)
	}
	if err := n.SetNoteState(ctx, "n1", "t1", ""); err != nil {
		t.Fatal(err)
	}
	papelera, err := n.ListTrash(ctx, 100)
	if err != nil || len(papelera) != 1 || papelera[0].ID != "n9" {
		t.Fatalf("ListTrash = %v, %v", papelera, err)
	}
	etiquetas, err := n.ListTags(ctx)
	if err != nil || len(etiquetas) != 1 || len(etiquetas[0].States) != 1 || etiquetas[0].States[0].Name != "Alta" {
		t.Fatalf("ListTags = %v, %v", etiquetas, err)
	}
	retro, err := n.LinkedNotes(ctx, "note", "n1", 50)
	if err != nil || len(retro) != 1 || retro[0].ID != "n2" {
		t.Fatalf("LinkedNotes = %v, %v", retro, err)
	}

	quiero := []string{
		"DELETE /api/v1/notes/n1", "POST /api/v1/notes/n1/restore", "DELETE /api/v1/notes/n1/purge",
		"POST /api/v1/notes/n1/move", "POST /api/v1/notes/n1/state", "POST /api/v1/notes/n1/state",
		"GET /api/v1/notes/trash", "GET /api/v1/tags", "GET /api/v1/notes/linked",
	}
	for i, q := range quiero {
		if i >= len(visto) || visto[i] != q {
			t.Fatalf("petición %d fue %q y esperaba %q (todas: %v)", i, visto[min(i, len(visto)-1)], q, visto)
		}
	}
	// `state_id` nulo es «quita la etiqueta»; omitirlo dejaría el estado puesto.
	if cuerpoEstado["tag_id"] != "t1" || cuerpoEstado["state_id"] != nil {
		t.Errorf("quitar el estado manda %v", cuerpoEstado)
	}
	// Mover de cuaderno NO manda posición: la decide el destino.
	if cuerpoMover["notebook_id"] != "c2" || len(cuerpoMover) != 1 {
		t.Errorf("mover de cuaderno manda %v", cuerpoMover)
	}
}

// Los nombres de las etiquetas y de la papelera son los del servicio.
func TestEtiquetasYPapeleraSiguenAlServicio(t *testing.T) {
	dominio := webref.Read(t, "services/notes/internal/domain/domain.go")
	for _, campo := range []string{`json:"tag_id"`, `json:"state_id"`, `json:"states,omitempty"`, `json:"shortcut,omitempty"`} {
		if !strings.Contains(dominio, campo) {
			t.Errorf("services/notes ya no declara %s: revisar internal/api/notes.go", campo)
		}
	}
	rutas := webref.Read(t, "services/notes/internal/http/server.go")
	for _, ruta := range []string{
		`Get("/notes/trash"`, `Get("/notes/linked"`, `Post("/notes/{id}/move"`,
		`Post("/notes/{id}/state"`, `Post("/notes/{id}/restore"`, `Delete("/notes/{id}/purge"`, `Get("/tags"`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/notes ya no sirve %s: revisar internal/api/notes.go", ruta)
		}
	}
}

// Guardar los ajustes de un cuaderno manda el cuaderno ENTERO, y lo que más importa de
// «entero» es el cifrado: el PUT del servicio sustituye, así que un cliente que no
// devolviera `encrypted` y `crypto` DESCIFRARÍA el cuaderno en el servidor y sus notas,
// que siguen siendo criptogramas, se quedarían sin poder abrirse.
//
// VISTO EN ROJO (2026-09-22, en el frontal): quitando `Encrypted` y `Crypto` de
// `Notebook.Input()`, que es como estaba, el test dice que `encrypted` viaja como false
// y `crypto` como null. Deshecho.
func TestLosAjustesNoDescifranElCuaderno(t *testing.T) {
	original := `{"id":"c1","parent_id":"p1","title":"Diario","emoji":"📓","color":"#86198f",
		"layout":"tree","visibility":"private","indexable":false,"encrypted":true,
		"crypto":{"v":1,"kdf":"PBKDF2-SHA256","salt":"c2FsdHNhbHRzYWx0c2FsdA","iterations":210000,"check":"abc"},
		"room_id":"sala-1","position":3,"note_count":4,"updated_at":"2026-09-02T10:00:00Z"}`
	var recibido map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/notebooks":
			_, _ = io.WriteString(w, `{"notebooks":[`+original+`]}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/notebooks/c1":
			_ = json.NewDecoder(r.Body).Decode(&recibido)
			_, _ = io.WriteString(w, original)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	notas := NewNotes(srv.URL, nil, nil)
	lista, err := notas.ListNotebooks(context.Background())
	if err != nil || len(lista) != 1 {
		t.Fatalf("ListNotebooks = %v, %v", lista, err)
	}
	in := lista[0].Input()
	in.Title = "Diario íntimo" // lo ÚNICO que cambia
	if _, err := notas.UpdateNotebook(context.Background(), "c1", in); err != nil {
		t.Fatal(err)
	}
	var quiero map[string]any
	_ = json.Unmarshal([]byte(original), &quiero)
	quiero["title"] = "Diario íntimo"
	// Lo que el servicio calcula o no deja fijar desde aquí.
	for _, soloLectura := range []string{"id", "note_count", "updated_at", "position"} {
		delete(quiero, soloLectura)
	}
	for campo, valor := range quiero {
		a, _ := json.Marshal(valor)
		b, _ := json.Marshal(recibido[campo])
		if !bytes.Equal(a, b) {
			t.Errorf("al guardar los ajustes, %q viaja como %s y era %s", campo, b, a)
		}
	}
}

// Colocar una nota, y crear, cambiar y borrar etiquetas, pegan en las rutas del
// servicio. Sacar una nota de su grupo es `group_id` a null EXPLÍCITO: es lo que la web
// manda al «sacar de su nota padre».
func TestColocarYEtiquetarPeganDondeToca(t *testing.T) {
	var visto []string
	cuerpos := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clave := r.Method + " " + r.URL.Path
		visto = append(visto, clave)
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		cuerpos[clave] = c
		switch r.Method {
		case http.MethodPost, http.MethodPut:
			if strings.HasPrefix(r.URL.Path, "/api/v1/tags") {
				_, _ = io.WriteString(w, `{"id":"t1","name":"Prioridad","states":[]}`)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	n, ctx := NewNotes(srv.URL, nil, nil), context.Background()

	if err := n.MoveNote(ctx, "n1", NotePosition{X: 24, Y: 48, Col: 1, Position: 2}); err != nil {
		t.Fatal(err)
	}
	s1 := "s1"
	in := TagInput{Name: "Prioridad", States: []TagStateInput{
		{ID: &s1, Name: "Alta", Color: "#b0413a", Shortcut: "1"},
		{Name: "Baja", Color: "#16794a"},
	}}
	if _, err := n.SaveTag(ctx, "", in); err != nil {
		t.Fatal(err)
	}
	if _, err := n.SaveTag(ctx, "t1", in); err != nil {
		t.Fatal(err)
	}
	if err := n.DeleteTag(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	quiero := []string{
		"POST /api/v1/notes/n1/move", "POST /api/v1/tags", "PUT /api/v1/tags/t1", "DELETE /api/v1/tags/t1",
	}
	if strings.Join(visto, "|") != strings.Join(quiero, "|") {
		t.Fatalf("peticiones %v, esperaba %v", visto, quiero)
	}
	mover := cuerpos["POST /api/v1/notes/n1/move"]
	if v, esta := mover["group_id"]; !esta || v != nil {
		t.Errorf("colocar sin grupo tiene que mandar group_id null explícito: %v", mover)
	}
	if mover["x"] != float64(24) || mover["col"] != float64(1) || mover["position"] != float64(2) {
		t.Errorf("colocar manda %v", mover)
	}
	estados, _ := cuerpos["POST /api/v1/tags"]["states"].([]any)
	if len(estados) != 2 {
		t.Fatalf("la etiqueta viaja con %v estados", estados)
	}
	// El estado nuevo NO lleva id: un "" no es un UUID y el servicio lo rechazaría.
	if nuevo, _ := estados[1].(map[string]any); nuevo["id"] != nil {
		t.Errorf("el estado nuevo manda id %v", nuevo["id"])
	}
	rutas := webref.Read(t, "services/notes/internal/http/server.go")
	for _, ruta := range []string{`Post("/tags"`, `Put("/tags/{id}"`, `Delete("/tags/{id}"`} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/notes ya no sirve %s: revisar notes.go", ruta)
		}
	}
}

// TestImportarUnVolcadoMiraAntesYCreaDespues: la vista previa y la importación mandan el
// fichero TAL CUAL a sus dos rutas y entienden lo que contestan.
func TestImportarUnVolcadoMiraAntesYCreaDespues(t *testing.T) {
	const volcado = `{"formato":"sytue-notepads","notebook":{"title":"Mudanza"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		if string(cuerpo) != volcado || r.Method != http.MethodPost {
			t.Errorf("llega %s %q", r.Method, cuerpo)
		}
		switch r.URL.Path {
		case "/api/v1/notebooks/preview":
			_, _ = io.WriteString(w, `{"notebook":{"id":"","title":"Mudanza"},"notas":[{"id":"x","title":"Cajas"}],"descartadas":1}`)
		case "/api/v1/notebooks/import":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"notebook":{"id":"c9","title":"Mudanza"},"importadas":3,"descartadas":1}`)
		default:
			t.Errorf("ruta inesperada %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	notas := NewNotes(srv.URL, nil, nil)
	vp, err := notas.PreviewNotebook(context.Background(), []byte(volcado))
	if err != nil || vp.Notebook.Title != "Mudanza" || len(vp.Notas) != 1 || vp.Descartadas != 1 {
		t.Errorf("la vista previa es %+v, %v", vp, err)
	}
	im, err := notas.ImportNotebook(context.Background(), []byte(volcado))
	if err != nil || im.Notebook.ID != "c9" || im.Importadas != 3 || im.Descartadas != 1 {
		t.Errorf("la importación es %+v, %v", im, err)
	}
}

// Archivar, desarchivar, «Fijadas» y el filtro de archivados de la búsqueda hablan el
// idioma del servicio: el método y la ruta deciden si archiva o devuelve, y sin
// `archived=1` lo archivado no sale.
func TestArchivoYFijadasPeganDondeToca(t *testing.T) {
	var vistas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// QUERY lleva la consulta en el cuerpo (`Consulta`); el resto, en la URL.
		valores := r.URL.Query()
		if r.Method == "QUERY" {
			b, _ := io.ReadAll(r.Body)
			valores, _ = url.ParseQuery(string(b))
		}
		vistas = append(vistas, r.Method+" "+r.URL.Path+"?"+valores.Get("archived"))
		if r.Method == http.MethodGet || r.Method == "QUERY" {
			_, _ = io.WriteString(w, `{"notes":[{"id":"n1","notebook_id":"c1","notebook":"Ideas","title":"Fijada"}]}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	n, ctx := NewNotes(srv.URL, nil, nil), context.Background()
	if err := n.ArchiveNotebook(ctx, "c1", true); err != nil {
		t.Fatal(err)
	}
	if err := n.ArchiveNotebook(ctx, "c1", false); err != nil {
		t.Fatal(err)
	}
	fijadas, err := n.PinnedNotes(ctx)
	if err != nil || len(fijadas) != 1 || fijadas[0].Title != "Fijada" {
		t.Fatalf("PinnedNotes = %+v, %v", fijadas, err)
	}
	if _, err := n.Search(ctx, "x", 5, true); err != nil {
		t.Fatal(err)
	}
	quiero := []string{
		"POST /api/v1/notebooks/c1/archive?", "DELETE /api/v1/notebooks/c1/archive?",
		"GET /api/v1/notes/pinned?",
	}
	if len(vistas) < 4 || strings.Join(vistas[:3], "|") != strings.Join(quiero, "|") {
		t.Fatalf("peticiones = %q", vistas)
	}
	if !strings.HasSuffix(vistas[len(vistas)-1], "?1") {
		t.Errorf("la búsqueda con archivados no manda archived=1: %q", vistas)
	}
	rutas := webref.Read(t, "services/notes/internal/http/server.go")
	for _, ruta := range []string{`Post("/notebooks/{id}/archive"`, `Delete("/notebooks/{id}/archive"`, `Get("/notes/pinned"`} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/notes ya no sirve %s: revisar api/notes.go", ruta)
		}
	}
}

// Hashtags pide /hashtags, que el servicio sigue sirviendo.
func TestHashtagsPegaDondeToca(t *testing.T) {
	var vista string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vista = r.Method + " " + r.URL.Path
		_, _ = io.WriteString(w, `{"hashtags":[{"tag":"plan","count":3}]}`)
	}))
	defer srv.Close()
	tags, err := NewNotes(srv.URL, nil, nil).Hashtags(context.Background())
	if err != nil || len(tags) != 1 || tags[0] != (Hashtag{Tag: "plan", Count: 3}) || vista != "GET /api/v1/hashtags" {
		t.Fatalf("Hashtags = %+v, %v (%s)", tags, err, vista)
	}
	if !strings.Contains(webref.Read(t, "services/notes/internal/http/server.go"), `Get("/hashtags"`) {
		t.Error("services/notes ya no sirve /hashtags: revisar api/notes.go")
	}
}

// El diario por tramos y «Este día» piden las rutas y parámetros del servicio.
func TestDiasDelDiarioSiguenAlServicio(t *testing.T) {
	var visto []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visto = append(visto, r.URL.Path+"?"+r.URL.RawQuery)
		_, _ = io.WriteString(w, `{"days":[{"date":"2026-10-01","notebook_id":"c1","total":3,"pending":1,"notes":[{"id":"n1"}]}]}`)
	}))
	defer srv.Close()
	n, ctx := NewNotes(srv.URL, nil, nil), context.Background()
	dias, err := n.DailyRange(ctx, "2026-09-28", "2026-10-04", true)
	if err != nil || len(dias) != 1 || dias[0].Total != 3 || dias[0].Pending != 1 || len(dias[0].Notes) != 1 {
		t.Fatalf("DailyRange = %+v, %v", dias, err)
	}
	if _, err := n.DailyRange(ctx, "2026-09-28", "2026-10-04", false); err != nil {
		t.Fatal(err)
	}
	if _, err := n.ThisDay(ctx, "2026-10-01"); err != nil {
		t.Fatal(err)
	}
	quiero := []string{
		"/api/v1/daily?from=2026-09-28&notes=1&to=2026-10-04",
		"/api/v1/daily?from=2026-09-28&to=2026-10-04",
		"/api/v1/daily/this-day?date=2026-10-01",
	}
	if !slices.Equal(visto, quiero) {
		t.Fatalf("peticiones %v, quería %v", visto, quiero)
	}
	dominio := webref.Read(t, "services/notes/internal/domain/diario.go")
	for _, campo := range []string{`json:"date"`, `json:"notebook_id"`, `json:"total"`, `json:"pending"`, `json:"notes,omitempty"`} {
		if !strings.Contains(dominio, campo) {
			t.Errorf("el servicio ya no tiene %s en DailyDay", campo)
		}
	}
}
