// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import "context"

// Grupos opcionales de NotesData (PLAN-EDICION-LOCAL.md §5.7 y Anexo A). Una
// implementación privada que cumpla un grupo atiende sus métodos; la que no, sigue
// fallando cerrada con ErrPrivateNotesOperation, sin caer nunca al transporte HTTP.

// NotesDailyData es el diario: abrir (o crear) el cuaderno de un día, los días con
// notas de un tramo y «Este día».
type NotesDailyData interface {
	DailyNotebook(ctx context.Context, date, title string, carry bool) (Notebook, error)
	DailyRange(ctx context.Context, from, to string, notas bool) ([]DailyDay, error)
	ThisDay(ctx context.Context, date string) ([]DailyDay, error)
}

// NotesTasksData es la vista «Tareas» y «Posponer».
type NotesTasksData interface {
	ListTasks(ctx context.Context, estado string) ([]Tarea, error)
	PostponeTask(ctx context.Context, id, date, title string) (Notebook, error)
}

// NotesTagsData son las etiquetas con estados y su marca en una nota.
type NotesTagsData interface {
	SaveTag(ctx context.Context, id string, in TagInput) (Tag, error)
	DeleteTag(ctx context.Context, id string) error
	SetNoteState(ctx context.Context, nota, etiqueta, estado string) error
}

// NotesRemindersData pone y quita MI recordatorio de una nota.
type NotesRemindersData interface {
	SetReminder(ctx context.Context, nota string, in NoteReminderInput) (NoteReminder, error)
	DeleteReminder(ctx context.Context, nota string) error
}

// NotesMoveData coloca notas, las cambia de cuaderno, archiva cuadernos y borra notas
// de la papelera para siempre.
type NotesMoveData interface {
	MoveNote(ctx context.Context, id string, p NotePosition) error
	MoveNoteToNotebook(ctx context.Context, id, cuaderno string) error
	ArchiveNotebook(ctx context.Context, id string, archivar bool) error
	PurgeNote(ctx context.Context, id string) error
}

// NotesImportData es «Importar una copia…»: el ensayo y la importación, leídos en el
// dispositivo con el mismo lector que el servicio.
type NotesImportData interface {
	PreviewNotebook(ctx context.Context, volcado []byte, nombre string) (VistaPreviaVolcado, error)
	ImportNotebook(ctx context.Context, volcado []byte, nombre string) (Importacion, error)
}

// notesGrupo devuelve el grupo opcional T de la fachada privada. ok=false con
// privada=true: la fachada es privada y no lo cumple (falla cerrada). privada=false:
// es la fachada de siempre y sigue por HTTP.
func notesGrupo[T any](n *Notes) (grupo T, privada, ok bool) {
	if n == nil || n.data == nil {
		return grupo, false, false
	}
	grupo, ok = n.data.(T)
	return grupo, true, ok
}
