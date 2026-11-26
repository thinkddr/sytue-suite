// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

// Importar un volcado (`.onex`, Markdown suelto o en ZIP, Evernote `.enex`) SIN servicio:
// la misma lectura (`model.LeerImportable`), el mismo `Normalize` y las mismas reglas que
// `services/notes` (`Store.ImportNotebook` y `Handlers.PreviewNotebook`), pero escritas en
// este repositorio. Es lo que usa Sytue Office, que no tiene servidor, y lo gana también la
// edición conectada si su fachada lo enruta (Anexo A de PLAN-EDICION-LOCAL.md).
//
// Siempre crea un cuaderno NUEVO: no se restaura encima de nada. Las notas que no pasan la
// validación no abortan la importación: se cuentan en `Descartadas`.
//
// Un volcado de un cuaderno CIFRADO se rechaza entero con
// `model.ErrEncryptedMigrationRequired`: este repositorio no sella contenido, y saltarse
// esas notas sería perderlas en silencio.

// PreviewNotebook es el ensayo: lo que entraría, ya saneado, sin crear nada.
func (r *Repository) PreviewNotebook(_ context.Context, volcado []byte, nombre string) (api.VistaPreviaVolcado, error) {
	libro, entradas, descartadas, err := leerVolcado(volcado, nombre)
	if err != nil {
		return api.VistaPreviaVolcado{}, err
	}
	vp := api.VistaPreviaVolcado{Descartadas: descartadas}
	vp.Notebook = api.Notebook{Title: libro.Title, Emoji: libro.Emoji, Color: libro.Color, Layout: libro.Layout, Visibility: "personal"}
	for _, in := range entradas {
		n := api.Note{Kind: in.Kind, Title: in.Title, Content: in.Content, Data: in.Data, Links: in.Links}
		vp.Notas = append(vp.Notas, n)
	}
	return vp, nil
}

// ImportNotebook crea el cuaderno nuevo con las notas del volcado.
func (r *Repository) ImportNotebook(ctx context.Context, volcado []byte, nombre string) (api.Importacion, error) {
	libro, entradas, descartadas, err := leerVolcado(volcado, nombre)
	if err != nil {
		return api.Importacion{}, err
	}
	creado, err := r.CreateNotebook(ctx, libro)
	if err != nil {
		return api.Importacion{}, err
	}
	puestas := 0
	for _, in := range entradas {
		if _, err := r.CreateNote(ctx, creado.ID, in); err != nil {
			descartadas++
			continue
		}
		puestas++
	}
	return api.Importacion{Notebook: creado, Importadas: puestas, Descartadas: descartadas}, nil
}

// leerVolcado lee y sanea el volcado: el cuaderno a crear y las notas que pasan, como
// entradas de la fachada, y cuántas se quedan fuera.
func leerVolcado(volcado []byte, nombre string) (api.NotebookInput, []api.NoteInput, int, error) {
	libro, _, leidas, descartadas, err := leerVolcadoEntero(volcado, nombre)
	if err != nil {
		return api.NotebookInput{}, nil, 0, err
	}
	var salida []api.NoteInput
	for _, l := range leidas {
		salida = append(salida, l.entrada)
	}
	return libro, salida, descartadas, nil
}

// notaLeida es una nota del volcado que pasa: la entrada que se crea y la nota tal como
// venía (su identificador, sus fechas y sus estados, que la entrada no lleva).
type notaLeida struct {
	entrada api.NoteInput
	fuente  model.Note
}

// leerVolcadoEntero es leerVolcado con el cuaderno y cada nota tal como venían.
func leerVolcadoEntero(volcado []byte, nombre string) (api.NotebookInput, model.Notebook, []notaLeida, int, error) {
	if len(volcado) > api.MaxVolcado {
		return api.NotebookInput{}, model.Notebook{}, nil, 0, model.ErrInvalidInput
	}
	nb, notas, err := model.LeerImportable(volcado, nombre)
	if err != nil {
		return api.NotebookInput{}, model.Notebook{}, nil, 0, err
	}
	if err := model.ValidateBackupUpload(nb, notas); err != nil {
		return api.NotebookInput{}, model.Notebook{}, nil, 0, err
	}
	entrada := nb.AImportar()
	if entrada.Encrypted {
		return api.NotebookInput{}, model.Notebook{}, nil, 0, model.ErrEncryptedMigrationRequired
	}
	if err := entrada.Normalize(); err != nil {
		return api.NotebookInput{}, model.Notebook{}, nil, 0, err
	}
	// Lo publicado o compartido en otra instalación entra como personal: aquí no hay a
	// quién enseñárselo, y el repositorio rechaza cualquier otra visibilidad.
	entrada.Visibility, entrada.Crypto = "", nil
	var libro api.NotebookInput
	if err := recodificar(entrada, &libro); err != nil {
		return api.NotebookInput{}, model.Notebook{}, nil, 0, err
	}
	var salida []notaLeida
	descartadas := 0
	for _, n := range notas {
		in := n.AImportar()
		// En claro: ni versión de cifrado ni criptograma.
		in.CipherVersion, in.ContentCipher = 0, ""
		if err := in.Normalize(false); err != nil {
			descartadas++
			continue
		}
		in.Visibility = ""
		var entradaNota api.NoteInput
		if err := recodificar(in, &entradaNota); err != nil {
			descartadas++
			continue
		}
		salida = append(salida, notaLeida{entrada: entradaNota, fuente: n})
	}
	return libro, nb, salida, descartadas, nil
}

// recodificar pasa de la entrada del modelo a la de la fachada: tienen las mismas claves
// JSON, que es el contrato del servicio.
func recodificar(de, a any) error {
	raw, err := json.Marshal(de)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, a)
}
