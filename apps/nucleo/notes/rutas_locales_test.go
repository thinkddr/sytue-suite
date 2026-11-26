// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes_test

import (
	"encoding/json"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func ptr[T any](v T) *T { return &v }

// Las rutas que antes solo tenía el servicio (Anexo A) funcionan sobre el almacén en
// claro y sobreviven a reabrir: diario con traslado, «Este día», tareas, posponer,
// etiquetas con estados, recordatorios, mover, archivar y purgar.
func TestRutasLocalesDeNotas(t *testing.T) {
	dir := t.TempDir()
	repo, cerrar := abrirLocal(t, dir)
	ctx := t.Context()

	// Diario de ayer con una tarea pendiente que lleva una subtarea, y una hecha.
	ayer, err := repo.DailyNotebook(ctx, "2026-11-22", "22 nov", false)
	if err != nil || ayer.DailyDate != "2026-11-22" {
		t.Fatal("abrir diario", ayer, err)
	}
	if otra, err := repo.DailyNotebook(ctx, "2026-11-22", "22 nov", false); err != nil || otra.ID != ayer.ID {
		t.Fatal("abrirlo otra vez no lo duplica", otra, err)
	}
	padre, err := repo.CreateNote(ctx, ayer.ID, api.NoteInput{Kind: "todo", Title: "Llamar"})
	if err != nil {
		t.Fatal(err)
	}
	hija, err := repo.CreateNote(ctx, ayer.ID, api.NoteInput{Kind: "todo", Title: "Buscar el número", GroupID: &padre.ID})
	if err != nil {
		t.Fatal(err)
	}
	hecha, err := repo.CreateNote(ctx, ayer.ID, api.NoteInput{Kind: "todo", Title: "Hecha", Checked: ptr(true)})
	if err != nil {
		t.Fatal(err)
	}
	// Hace un año, el mismo día.
	antes, err := repo.DailyNotebook(ctx, "2025-11-23", "23 nov 2025", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateNote(ctx, antes.ID, api.NoteInput{Kind: "text", Title: "Hace un año"}); err != nil {
		t.Fatal(err)
	}

	hoy, err := repo.DailyNotebook(ctx, "2026-11-23", "23 nov", true)
	if err != nil {
		t.Fatal("abrir hoy con traslado", err)
	}
	enHoy, err := repo.ListNotes(ctx, hoy.ID)
	if err != nil || len(enHoy) != 2 {
		t.Fatal("la tarea pendiente y su subtarea pasan a hoy", enHoy, err)
	}
	for _, n := range enHoy {
		if n.ID == hija.ID && (n.GroupID == nil || *n.GroupID != padre.ID) {
			t.Fatal("la subtarea sigue colgando de su tarea", n)
		}
		if n.ID == padre.ID {
			var d struct {
				From  string `json:"carried_from"`
				Count int    `json:"carry_count"`
			}
			if json.Unmarshal(n.Data, &d) != nil || d.From != "2026-11-22" || d.Count != 1 {
				t.Fatal("el traslado se apunta", string(n.Data))
			}
		}
	}
	if quedan, _ := repo.ListNotes(ctx, ayer.ID); len(quedan) != 1 || quedan[0].ID != hecha.ID {
		t.Fatal("la hecha se queda en su día", quedan)
	}

	dias, err := repo.DailyRange(ctx, "2026-11-20", "2026-11-26", false)
	if err != nil || len(dias) != 2 || dias[0].Date != "2026-11-22" || dias[1].Pending != 2 {
		t.Fatal("días del tramo", dias, err)
	}
	este, err := repo.ThisDay(ctx, "2026-11-23")
	if err != nil || len(este) != 1 || este[0].Date != "2025-11-23" || len(este[0].Notes) != 1 {
		t.Fatal("este día", este, err)
	}

	abiertas, err := repo.ListTasks(ctx, api.TareaAbierta)
	if err != nil || len(abiertas) != 2 {
		t.Fatal("tareas abiertas", abiertas, err)
	}
	if hechas, _ := repo.ListTasks(ctx, api.TareaHecha); len(hechas) != 1 || hechas[0].Notebook != "22 nov" {
		t.Fatal("tareas hechas", hechas)
	}
	manana, err := repo.PostponeTask(ctx, padre.ID, "2026-11-24", "24 nov")
	if err != nil {
		t.Fatal("posponer", err)
	}
	if ns, _ := repo.ListNotes(ctx, manana.ID); len(ns) != 2 {
		t.Fatal("pospuesta con su subtarea", ns)
	}

	// Etiquetas con estados y su marca.
	tag, err := repo.SaveTag(ctx, "", api.TagInput{Name: "Prioridad", States: []api.TagStateInput{{Name: "Alta", Color: "#ff0000"}, {Name: "Baja"}}})
	if err != nil || len(tag.States) != 2 {
		t.Fatal("crear etiqueta", tag, err)
	}
	if err := repo.SetNoteState(ctx, hecha.ID, tag.ID, tag.States[0].ID); err != nil {
		t.Fatal("marcar", err)
	}
	// Quitar «Baja» y renombrar: la marca de «Alta» sigue.
	tag, err = repo.SaveTag(ctx, tag.ID, api.TagInput{Name: "Urgencia", States: []api.TagStateInput{{ID: &tag.States[0].ID, Name: "Alta", Color: "#ff0000"}}})
	if err != nil || len(tag.States) != 1 || tag.Name != "Urgencia" {
		t.Fatal("cambiar etiqueta", tag, err)
	}

	// Recordatorio.
	cuando := time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)
	if _, err := repo.SetReminder(ctx, hecha.ID, api.NoteReminderInput{DueAt: cuando, TZID: "Europe/Madrid"}); err != nil {
		t.Fatal("recordatorio", err)
	}
	if _, err := repo.SetReminder(ctx, hecha.ID, api.NoteReminderInput{DueAt: cuando, RRule: "FREQ=HOURLY"}); err == nil {
		t.Fatal("cada hora no se admite")
	}

	// Mover a otro cuaderno, archivar y purgar.
	libro, err := repo.CreateNotebook(ctx, api.NotebookInput{Title: "Proyectos"})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := repo.CreateNotebook(ctx, api.NotebookInput{Title: "Sub", ParentID: &libro.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MoveNoteToNotebook(ctx, hecha.ID, libro.ID); err != nil {
		t.Fatal("mover", err)
	}
	if err := repo.ArchiveNotebook(ctx, libro.ID, true); err != nil {
		t.Fatal("archivar", err)
	}
	if err := repo.ArchiveNotebook(ctx, hoy.ID, true); err == nil {
		t.Fatal("un diario no se archiva")
	}
	borrar, err := repo.CreateNote(ctx, manana.ID, api.NoteInput{Title: "Sobra"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.PurgeNote(ctx, borrar.ID); err == nil {
		t.Fatal("solo se purga lo de la papelera")
	}
	if err := repo.DeleteNote(ctx, borrar.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.PurgeNote(ctx, borrar.ID); err != nil {
		t.Fatal("purgar", err)
	}
	if err := repo.RestoreNote(ctx, borrar.ID); err == nil {
		t.Fatal("lo purgado no se restaura")
	}
	cerrar()

	repo, cerrar = abrirLocal(t, dir)
	defer cerrar()
	ns, err := repo.ListNotes(ctx, libro.ID)
	if err != nil || len(ns) != 1 || len(ns[0].States) != 1 || ns[0].States[0].TagName != "Urgencia" || ns[0].States[0].Name != "Alta" {
		t.Fatal("tras reabrir: nota movida con su estado", ns, err)
	}
	recs, err := repo.ListReminders(ctx)
	if err != nil || len(recs) != 1 || !recs[0].DueAt.Equal(cuando) || recs[0].NotebookID != libro.ID {
		t.Fatal("recordatorio tras reabrir", recs, err)
	}
	if err := repo.DeleteReminder(ctx, hecha.ID); err != nil {
		t.Fatal(err)
	}
	if recs, _ := repo.ListReminders(ctx); len(recs) != 0 {
		t.Fatal("recordatorio quitado", recs)
	}
	libros, _ := repo.ListNotebooks(ctx)
	for _, b := range libros {
		if (b.ID == libro.ID || b.ID == sub.ID) && b.ArchivedAt == nil {
			t.Fatal("la rama entera queda archivada", b)
		}
	}
	if papelera, _ := repo.ListTrash(ctx, 50); len(papelera) != 0 {
		t.Fatal("lo purgado no sale en la papelera", papelera)
	}
	if err := repo.DeleteTag(ctx, tag.ID); err != nil {
		t.Fatal(err)
	}
	if tags, _ := repo.ListTags(ctx); len(tags) != 0 {
		t.Fatal("etiqueta borrada", tags)
	}
	if ns, _ := repo.ListNotes(ctx, libro.ID); len(ns[0].States) != 0 {
		t.Fatal("sin etiqueta no hay estado", ns[0].States)
	}
}

// La fachada privada enruta los grupos nuevos al repositorio: «Importar notas…» de la
// pantalla ya no falla cerrado.
func TestLaFachadaPrivadaEnrutaLosGruposNuevos(t *testing.T) {
	repo, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	svc, err := api.NewNotes("", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	vp, err := svc.PreviewNotebook(ctx, []byte(enexDePrueba), "Evernote.enex")
	if err != nil || len(vp.Notas) != 2 {
		t.Fatal("ensayo por la fachada", vp, err)
	}
	imp, err := svc.ImportNotebook(ctx, []byte(enexDePrueba), "Evernote.enex")
	if err != nil || imp.Importadas != 2 {
		t.Fatal("importar por la fachada", imp, err)
	}
	if _, err := svc.DailyNotebook(ctx, "2026-11-23", "Hoy", true); err != nil {
		t.Fatal("diario por la fachada", err)
	}
	if _, err := svc.ListTasks(ctx, api.TareaAbierta); err != nil {
		t.Fatal("tareas por la fachada", err)
	}
	if _, err := svc.SaveTag(ctx, "", api.TagInput{Name: "Estado", States: []api.TagStateInput{{Name: "Hecho"}}}); err != nil {
		t.Fatal("etiqueta por la fachada", err)
	}
}
