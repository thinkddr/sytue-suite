// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// DOS DISPOSITIVOS CON LA MISMA NOTA (conflicto-pantalla.test.tsx de la web). El servicio
// falso guarda una nota con su versión: un PUT que no trae esa versión recibe el 409
// `note_edit_conflict`, como el de verdad; uno que la trae, guarda y la adelanta.

var v0 = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

type servidorConVersion struct {
	mu    sync.Mutex
	nota  api.Note
	puts  []map[string]any
	posts []map[string]any
}

func (s *servidorConVersion) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	crudo, _ := io.ReadAll(r.Body)
	var c map[string]any
	_ = json.Unmarshal(crudo, &c)
	ruta := strings.TrimPrefix(r.URL.Path, "/api/v1")
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(ruta, "/notebooks/c1/notes"):
		_ = json.NewEncoder(w).Encode(map[string]any{"notes": []api.Note{s.nota}})
	case r.Method == http.MethodPut && ruta == "/notes/n1":
		s.puts = append(s.puts, c)
		esperada, _ := time.Parse(time.RFC3339Nano, anyString(c["expected_updated_at"]))
		if !esperada.Equal(s.nota.UpdatedAt) {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"note_edit_conflict"}}`)
			return
		}
		s.nota.Title, s.nota.Content = anyString(c["title"]), anyString(c["content"])
		s.nota.Pinned = c["pinned"] == true
		s.nota.UpdatedAt = s.nota.UpdatedAt.Add(time.Second)
		_ = json.NewEncoder(w).Encode(s.nota)
	case r.Method == http.MethodPost && ruta == "/notebooks/c1/notes":
		s.posts = append(s.posts, c)
		_ = json.NewEncoder(w).Encode(api.Note{
			ID: "copia", NotebookID: "c1", Kind: "text",
			Title: anyString(c["title"]), Content: anyString(c["content"]),
		})
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func anyString(v any) string { s, _ := v.(string); return s }

func (s *servidorConVersion) versionesEnviadas() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.puts))
	for _, c := range s.puts {
		out = append(out, anyString(c["expected_updated_at"]))
	}
	return out
}

// pantallaConVersion monta la pantalla con la nota tal como la leyó este dispositivo
// (`aqui`) y el servidor con la que tiene ahora (`servidor`).
func pantallaConVersion(t *testing.T, aqui, servidor api.Note) (*Pantalla, *servidorConVersion) {
	t.Helper()
	falso := &servidorConVersion{nota: servidor}
	srv := httptest.NewServer(falso)
	t.Cleanup(srv.Close)
	w := test.NewTempWindow(t, nil)
	p := NuevaPantalla(trInterno(t), "es", api.NewNotes(srv.URL, nil, nil), nil, w)
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	c1 := DeCuaderno(api.Notebook{ID: "c1", Title: "Mudanza", NoteCount: 1})
	p.cuadernos, p.activo = []Cuaderno{c1}, c1
	p.notas, p.lienzo.Fase = DeNotas([]api.Note{aqui}), Exito
	return p, falso
}

func notaLeche() api.Note {
	return api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Title: "Compra", Content: "<p>leche</p>", UpdatedAt: v0}
}

func TestCadaGuardadoLlevaLaVersionQueDejoElAnterior(t *testing.T) {
	p, falso := pantallaConVersion(t, notaLeche(), notaLeche())
	uno, dos := "<p>leche y pan</p>", "<p>leche, pan y huevos</p>"
	if err := p.editar("n1", Parche{Contenido: &uno}, true); err != nil {
		t.Fatal(err)
	}
	// La nota de pantalla vuelve a ser la de antes, como cuando el segundo autoguardado
	// sale antes de que el primero se pinte: aun así no choca consigo mismo.
	p.notas = DeNotas([]api.Note{notaLeche()})
	if err := p.editar("n1", Parche{Contenido: &dos}, true); err != nil {
		t.Fatal(err)
	}
	quiero := []string{v0.Format(time.RFC3339Nano), v0.Add(time.Second).Format(time.RFC3339Nano)}
	if got := falso.versionesEnviadas(); !slices.Equal(got, quiero) {
		t.Fatalf("versiones enviadas %v; quiero %v", got, quiero)
	}
	if p.conflicto != nil {
		t.Fatal("dos guardados seguidos desde aquí no son un conflicto")
	}
}

func TestSiElOtroSoloLaMovioSeGuardaSinPreguntar(t *testing.T) {
	movida := notaLeche()
	movida.X, movida.UpdatedAt = 400, v0.Add(time.Minute)
	p, falso := pantallaConVersion(t, notaLeche(), movida)
	if err := p.editar("n1", Parche{Fijada: ptr(true)}, true); err != nil {
		t.Fatal(err)
	}
	if got := len(falso.versionesEnviadas()); got != 2 {
		t.Fatalf("hacen falta dos PUT (el que choca y el bueno); ha habido %d", got)
	}
	if p.conflicto != nil || !falso.nota.Pinned {
		t.Fatalf("mover no es un conflicto: conflicto=%v fijada=%v", p.conflicto != nil, falso.nota.Pinned)
	}
}

func conflictoConPan(t *testing.T) (*Pantalla, *servidorConVersion) {
	t.Helper()
	otra := notaLeche()
	otra.Content, otra.UpdatedAt = "<p>pan</p>", v0.Add(time.Minute)
	p, falso := pantallaConVersion(t, notaLeche(), otra)
	mia := "<p>leche y miel</p>"
	if err := p.editar("n1", Parche{Contenido: &mia}, true); err != nil {
		t.Fatalf("un conflicto se pregunta, no es un fallo: %v", err)
	}
	if p.conflicto == nil || p.conflicto.dialog == nil {
		t.Fatal("el conflicto no ha abierto el diálogo")
	}
	textos := strings.Join(textosInternos(p.conflicto.dialog), "|")
	for _, quiero := range []string{
		p.tr.T("conflict.title", nil), "leche y miel", "pan",
		p.tr.T("conflict.keepMine", nil), p.tr.T("conflict.seeTheirs", nil), p.tr.T("conflict.keepBoth", nil),
	} {
		if !strings.Contains(textos, quiero) {
			t.Errorf("el diálogo no dice %q: %s", quiero, textos)
		}
	}
	if n, _ := p.nota("n1"); n.Contenido != "<p>leche y miel</p>" {
		t.Errorf("mientras se decide, lo escrito tiene que seguir en pantalla: %q", n.Contenido)
	}
	return p, falso
}

func TestQuedarmeConLaMiaLaGuardaSobreLaOtra(t *testing.T) {
	p, falso := conflictoConPan(t)
	p.quedarmeConLaMia(p.conflicto, func() {})
	if p.conflicto != nil {
		t.Fatal("el diálogo sigue abierto")
	}
	if falso.nota.Content != "<p>leche y miel</p>" {
		t.Fatalf("en el servidor ha quedado %q", falso.nota.Content)
	}
	if got := falso.versionesEnviadas(); got[len(got)-1] != v0.Add(time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("«Quedarme con la mía» no se manda sobre la versión del otro: %v", got)
	}
}

func TestVerLaOtraLaPoneEnLaNotaSinVolverAGuardar(t *testing.T) {
	p, falso := conflictoConPan(t)
	antes := len(falso.versionesEnviadas())
	p.adoptarLaOtra(p.conflicto)
	if n, _ := p.nota("n1"); n.Contenido != "<p>pan</p>" || p.conflicto != nil {
		t.Fatalf("tras «Ver la otra» la nota dice %q (conflicto abierto: %v)", n.Contenido, p.conflicto != nil)
	}
	if len(falso.versionesEnviadas()) != antes || falso.nota.Content != "<p>pan</p>" {
		t.Fatal("«Ver la otra» ha vuelto a mandar lo de aquí")
	}
}

func TestGuardarLasDosCreaUnaNotaConLaMia(t *testing.T) {
	p, falso := conflictoConPan(t)
	p.guardarLasDos(p.conflicto)
	esperar(t, func() bool {
		falso.mu.Lock()
		defer falso.mu.Unlock()
		return len(falso.posts) > 0
	})
	falso.mu.Lock()
	defer falso.mu.Unlock()
	if len(falso.posts) != 1 {
		t.Fatal("«Guardar las dos» no ha creado la nota con lo de aquí")
	}
	c := falso.posts[0]
	if c["title"] != p.tr.T("conflict.copyTitle", map[string]any{"title": "Compra"}) || c["content"] != "<p>leche y miel</p>" {
		t.Errorf("la copia viaja como %v", c)
	}
	if falso.nota.Content != "<p>pan</p>" {
		t.Errorf("la nota del otro ha cambiado: %q", falso.nota.Content)
	}
}

func TestMismoContenidoNoMiraLaPosicionNiElOrdenDeLosDatos(t *testing.T) {
	a := api.Note{Kind: "text", Content: "x", Data: json.RawMessage(`{"a":1,"b":2}`)}
	b := a
	b.X, b.Data, b.UpdatedAt = 300, json.RawMessage(`{"b":2,"a":1}`), v0
	if !mismoContenido(a, b) {
		t.Error("mover la nota o reordenar sus datos no cambia su contenido")
	}
	b.Content = "y"
	if mismoContenido(a, b) {
		t.Error("cambiar el texto sí")
	}
	if got := string(datosDeCopia(json.RawMessage(`{"client_id":"c","carry_count":2,"icon":"pin"}`))); got != `{"icon":"pin"}` {
		t.Errorf("datos de la copia: %s", got)
	}
}
