// SPDX-License-Identifier: Apache-2.0
package model

import "encoding/json"

// AImportar convierte lo leído de un `.onex` en lo que las funciones de creación
// esperan.
//
// Va aquí y no en el store porque es una traducción entre dos formas del mismo
// dato, sin base de datos de por medio: así se puede probar sin Postgres, que es
// lo que hace que se pruebe.
//
// Todo lo que decide el SERVIDOR se deja fuera —identificadores, dueño, fechas—:
// el fichero describe un cuaderno, no reclama un sitio concreto en la base de
// datos de nadie.
func (nb Notebook) AImportar() NotebookInput {
	return NotebookInput{
		Title:      nb.Title,
		Emoji:      nb.Emoji,
		Color:      nb.Color,
		Layout:     nb.Layout,
		Visibility: nb.Visibility,
		Indexable:  &nb.Indexable,
		Encrypted:  nb.Encrypted,
		Crypto:     nb.Crypto,
		// ParentID y RoomID NO se restauran: apuntan a un cuaderno padre y a una
		// sala de ESTA instalación, y un fichero traído de otra parte los tendría
		// colgando de identificadores que aquí no existen. El cuaderno entra a la
		// raíz y quien lo importa lo coloca.
	}
}

// AImportar hace lo propio con una nota, conservando lo que la sitúa en el
// lienzo: sin `Col`, `Position` y `Width` la restauración recoloca todo, y eso no
// es restaurar.
func (n Note) AImportar() NoteInput {
	data := n.Data
	if n.ContentCipher != "" {
		// Task execution controls refer to the original objects, not the new IDs
		// allocated by an import. They are not writable archive payload.
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) == nil && fields != nil {
			delete(fields, "repeated_into")
			delete(fields, "carried_from")
			delete(fields, "carry_count")
			data, _ = json.Marshal(fields)
		}
	}
	x, y, col, pos := n.X, n.Y, n.Col, n.Position
	// UN ANCHO A CERO NO ES UN ANCHO: es una nota que el fichero trae sin él, y se
	// deja a nil para que la creación ponga el de siempre. Con `&w` a cero, la
	// validación (80..1200) tiraba la nota entera —y con ella cualquier `.onex`
	// escrito por un script que no supiera de anchos— contándola como
	// «descartada» sin más explicación.
	var w *int
	if n.Width > 0 {
		ancho := n.Width
		w = &ancho
	}
	cancelada := n.CancelledAt != nil
	return NoteInput{
		CipherVersion: EncryptedNoteWriteVersion,
		Cancelled:     &cancelada,
		Kind:          n.Kind,
		Title:         n.Title,
		Content:       n.Content,
		ContentCipher: n.ContentCipher,
		Data:          data,
		X:             &x,
		Y:             &y,
		Col:           &col,
		Position:      &pos,
		Width:         w,
		Collapsed:     &n.Collapsed,
		Checked:       &n.Checked,
		Pinned:        &n.Pinned,
		Visibility:    n.Visibility,
		Indexable:     n.Indexable,
		Links:         n.Links,
		// GroupID se deja fuera por lo mismo que ParentID: agrupa notas por su
		// identificador, y los de un fichero ajeno no existen aquí.
	}
}
