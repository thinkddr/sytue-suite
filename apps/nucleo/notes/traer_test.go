// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

// cuadernoDeSytue es un cuaderno como lo exporta Sytue Notepads en «Tus datos»: su HTML
// nativo, con el volcado dentro (model.NotebookAHTML, el exportador del servicio).
func cuadernoDeSytue(t *testing.T, nb model.Notebook, notas []model.Note) []byte {
	t.Helper()
	volcado, ok := model.VolcadoDeHTML([]byte(model.NotebookAHTML(nb, notas)))
	if !ok {
		t.Fatal("el HTML exportado no lleva volcado")
	}
	return volcado
}

// HU-07: un cuaderno traído de Sytue conserva las fechas de sus notas, sus tareas hechas
// con su fecha, sus etiquetas con estados (creadas aquí por su nombre) y sus adjuntos; y
// traerlo otra vez no repite nada, aunque entre medias se haya editado una nota: solo
// entra la nota nueva. Un cuaderno hijo cuelga del que vino de su padre.
func TestTraerCuadernoConservaFechasEtiquetasYNoRepite(t *testing.T) {
	ctx := t.Context()
	repo, cerrar := abrirEnEquipo(t, t.TempDir())
	defer cerrar()
	// Una etiqueta que ya había aquí, con otro estado: se reutiliza y se le añade el que falta.
	previa, err := repo.SaveTag(ctx, "", api.TagInput{Name: "prioridad", States: []api.TagStateInput{{Name: "Baja", Color: "#00ff00"}}})
	if err != nil {
		t.Fatal(err)
	}

	creado := time.Date(2024, 2, 1, 8, 0, 0, 0, time.UTC)
	cambiado := time.Date(2024, 6, 3, 18, 30, 0, 0, time.UTC)
	hecha := time.Date(2024, 6, 2, 12, 0, 0, 0, time.UTC)
	adjunto := `{"attachments":[{"id":"1-abc","name":"presupuesto.txt","size":5,"type":"text/plain","dataUrl":"data:text/plain;base64,aG9sYQo="}]}`
	nb := model.Notebook{ID: uuid.New(), Title: "Reforma", Color: "#0f766e", Emoji: "🏠", Layout: model.LayoutTree, CreatedAt: creado, UpdatedAt: cambiado}
	texto := model.Note{
		ID: uuid.New(), NotebookID: nb.ID, Kind: model.KindText, Title: "Llamar al fontanero",
		Content: "<p>Por la <b>reforma</b> #obra</p>", Data: json.RawMessage(adjunto), CreatedAt: creado, UpdatedAt: cambiado,
		States: []model.NoteState{{TagID: uuid.New(), TagName: "Prioridad", StateID: uuid.New(), Name: "Alta", Emoji: "🔥", Color: "#dc2626"}},
	}
	tarea := model.Note{
		ID: uuid.New(), NotebookID: nb.ID, Kind: model.KindTodo, Title: "Comprar tejas", Checked: true, CheckedAt: &hecha,
		Position: 1, CreatedAt: creado, UpdatedAt: hecha,
		States: []model.NoteState{{TagID: uuid.New(), TagName: "Fase", StateID: uuid.New(), Name: "Tejado", Color: "#1d4ed8"}},
	}
	hijo := model.Notebook{ID: uuid.New(), ParentID: &nb.ID, Title: "Presupuestos", Color: "#86198f", Layout: model.LayoutTree, CreatedAt: creado, UpdatedAt: creado}
	nota3 := model.Note{ID: uuid.New(), NotebookID: hijo.ID, Kind: model.KindText, Title: "Tejado", Content: "<p>2.400 €</p>", CreatedAt: creado, UpdatedAt: creado}

	res, err := repo.TraerCuaderno(ctx, nil, cuadernoDeSytue(t, nb, []model.Note{texto, tarea}), "Reforma.html", "")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Nuevo || res.Importadas != 2 || res.Repetidas != 0 || res.Descartadas != 0 || res.Origen != nb.ID.String() {
		t.Fatalf("primera vez: %+v", res)
	}
	if !res.Notebook.UpdatedAt.Equal(cambiado) || res.Notebook.Title != "Reforma" || res.Notebook.Emoji != "🏠" {
		t.Errorf("cuaderno: %+v", res.Notebook)
	}
	traidos, err := repo.LoTraido(ctx)
	if err != nil {
		t.Fatal(err)
	}
	padre, ok := traidos.Cuaderno(nb.ID.String())
	if !ok || padre != res.Notebook.ID {
		t.Fatalf("el cuaderno traído no se reconoce por su origen: %q %v", padre, ok)
	}
	resHijo, err := repo.TraerCuaderno(ctx, traidos, cuadernoDeSytue(t, hijo, []model.Note{nota3}), "Presupuestos.html", padre)
	if err != nil || resHijo.Importadas != 1 {
		t.Fatal(resHijo, err)
	}
	if resHijo.Notebook.ParentID == nil || *resHijo.Notebook.ParentID != padre {
		t.Errorf("el hijo no cuelga de su padre: %+v", resHijo.Notebook)
	}

	notas, err := repo.ListNotes(ctx, res.Notebook.ID)
	if err != nil || len(notas) != 2 {
		t.Fatal(notas, err)
	}
	i := slices.IndexFunc(notas, func(n api.Note) bool { return n.Title == "Llamar al fontanero" })
	j := slices.IndexFunc(notas, func(n api.Note) bool { return n.Title == "Comprar tejas" })
	if i < 0 || j < 0 {
		t.Fatalf("faltan notas: %+v", notas)
	}
	n, tr := notas[i], notas[j]
	if !n.CreatedAt.Equal(creado) || !n.UpdatedAt.Equal(cambiado) {
		t.Errorf("la nota no conserva sus fechas: creada %v, cambiada %v", n.CreatedAt, n.UpdatedAt)
	}
	if !strings.Contains(n.Content, "<b>reforma</b>") || !strings.Contains(string(n.Data), "presupuesto.txt") {
		t.Errorf("contenido o adjunto perdidos: %q %s", n.Content, n.Data)
	}
	if len(n.States) != 1 || n.States[0].TagID != previa.ID || n.States[0].Name != "Alta" {
		t.Errorf("la etiqueta no se ha reutilizado por su nombre: %+v", n.States)
	}
	if tr.Kind != "todo" || !tr.Checked || tr.CheckedAt == nil || !tr.CheckedAt.Equal(hecha) || len(tr.States) != 1 || tr.States[0].Name != "Tejado" {
		t.Errorf("la tarea no llega hecha, con su fecha y su etiqueta: %+v", tr)
	}
	tags, err := repo.ListTags(ctx)
	if err != nil || len(tags) != 2 {
		t.Fatalf("etiquetas: %+v %v", tags, err)
	}
	if k := slices.IndexFunc(tags, func(x api.Tag) bool { return x.ID == previa.ID }); k < 0 || len(tags[k].States) != 2 {
		t.Errorf("a «prioridad» no se le ha añadido «Alta» manteniendo «Baja»: %+v", tags)
	}

	// Se edita una nota y se vuelve a traer la misma copia, con una nota más.
	in := n.Input()
	in.Title = "Llamar al fontanero (hecho)"
	if _, err := repo.UpdateNote(ctx, n.ID, res.Notebook.ID, in); err != nil {
		t.Fatal(err)
	}
	nueva := model.Note{ID: uuid.New(), NotebookID: nb.ID, Kind: model.KindText, Title: "Pedir licencia", CreatedAt: cambiado, UpdatedAt: cambiado}
	otra, err := repo.TraerCuaderno(ctx, nil, cuadernoDeSytue(t, nb, []model.Note{texto, tarea, nueva}), "Reforma.html", "")
	if err != nil {
		t.Fatal(err)
	}
	if otra.Nuevo || otra.Notebook.ID != res.Notebook.ID || otra.Importadas != 1 || otra.Repetidas != 2 {
		t.Fatalf("segunda vez: %+v", otra)
	}
	if libros, _ := repo.ListNotebooks(ctx); len(libros) != 2 {
		t.Errorf("cuadernos repetidos: %+v", libros)
	}
	if notas, _ := repo.ListNotes(ctx, res.Notebook.ID); len(notas) != 3 {
		t.Errorf("notas tras traer otra vez: %d", len(notas))
	}
	if tags, _ := repo.ListTags(ctx); len(tags) != 2 {
		t.Errorf("etiquetas repetidas: %+v", tags)
	}
}

// HU-07: un cuaderno del diario de Sytue entra como el diario de su día, el mismo que
// abre Notepads: nuevo y con su título si aquí no hay diario de ese día, y si lo hay, sus
// notas se suman a él. Traerlos otra vez no repite nada.
func TestTraerUnCuadernoDelDiarioEntraComoDiario(t *testing.T) {
	ctx := t.Context()
	repo, cerrar := abrirEnEquipo(t, t.TempDir())
	defer cerrar()
	creado := time.Date(2025, 6, 2, 7, 0, 0, 0, time.UTC)
	// El diario del 3 de junio ya está aquí, con una nota.
	propio, err := repo.DailyNotebook(ctx, "2025-06-03", "Martes", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateNote(ctx, propio.ID, api.NoteInput{Kind: "text", Title: "Escrita aquí"}); err != nil {
		t.Fatal(err)
	}
	lunes := model.Notebook{ID: uuid.New(), Title: "Lunes, 2 de junio", Color: "#86198f", Layout: model.LayoutList, DailyDate: "2025-06-02", CreatedAt: creado, UpdatedAt: creado}
	martes := model.Notebook{ID: uuid.New(), Title: "Martes, 3 de junio", Color: "#86198f", Layout: model.LayoutList, DailyDate: "2025-06-03", CreatedAt: creado, UpdatedAt: creado}
	notaLunes := model.Note{ID: uuid.New(), NotebookID: lunes.ID, Kind: model.KindText, Title: "Lo del lunes", Content: "<p>lunes</p>", CreatedAt: creado, UpdatedAt: creado}
	notaMartes := model.Note{ID: uuid.New(), NotebookID: martes.ID, Kind: model.KindText, Title: "Lo del martes", Content: "<p>martes</p>", CreatedAt: creado, UpdatedAt: creado}
	for vuelta := range 2 {
		traidos, err := repo.LoTraido(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct {
			nb   model.Notebook
			nota model.Note
		}{{lunes, notaLunes}, {martes, notaMartes}} {
			res, err := repo.TraerCuaderno(ctx, traidos, cuadernoDeSytue(t, c.nb, []model.Note{c.nota}), c.nb.Title+".html", "")
			if err != nil {
				t.Fatal(err)
			}
			if vuelta == 1 && (res.Importadas != 0 || res.Repetidas != 1 || res.Nuevo) {
				t.Errorf("otra vez, %s: %+v", c.nb.DailyDate, res)
			}
		}
	}
	diaLunes, err := repo.DailyNotebook(ctx, "2025-06-02", "otro título", false)
	if err != nil {
		t.Fatal(err)
	}
	if diaLunes.Title != "Lunes, 2 de junio" || diaLunes.DailyDate != "2025-06-02" || diaLunes.ParentID != nil {
		t.Errorf("el diario del lunes: %+v", diaLunes)
	}
	diaMartes, err := repo.DailyNotebook(ctx, "2025-06-03", "otro título", false)
	if err != nil {
		t.Fatal(err)
	}
	if diaMartes.ID != propio.ID {
		t.Errorf("el martes no ha ido al diario que ya estaba: %+v", diaMartes)
	}
	for id, quiero := range map[string][]string{diaLunes.ID: {"Lo del lunes"}, propio.ID: {"Escrita aquí", "Lo del martes"}} {
		ns, err := repo.ListNotes(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		titulos := make([]string, 0, len(ns))
		for _, n := range ns {
			titulos = append(titulos, n.Title)
		}
		slices.Sort(titulos)
		if !slices.Equal(titulos, quiero) {
			t.Errorf("notas de %s: %q", id, titulos)
		}
	}
	libros, err := repo.ListNotebooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(libros) != 2 {
		t.Errorf("cuadernos (solo los dos diarios): %+v", libros)
	}
}
