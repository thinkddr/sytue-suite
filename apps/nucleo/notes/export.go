// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"context"
	"encoding/json"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var _ api.NotesExportData = (*Repository)(nil)

// ExportNotebook reads the authenticated private catalog and runs the original
// shared exporter on the device. The retained session gate lasts through the
// cancellable download, and the generated plaintext buffer is cleared on exit.
func (r *Repository) ExportNotebook(parent context.Context, book, note, format string) (io.ReadCloser, string, error) {
	f, ok := model.FormatoDe(format)
	if !ok {
		return nil, "", model.ErrInvalidInput
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, "", err
	}
	nb, notes, err := r.exportNotes(ctx, book, note)
	if err != nil {
		done()
		return nil, "", err
	}
	if err := model.ValidateEncryptedBackup(nb, notes); err != nil {
		done()
		return nil, "", err
	}
	if err := model.ValidateEncryptedBackupFormat(nb, f); err != nil {
		done()
		return nil, "", err
	}
	name := model.NombreDeFicheroExportado(nb.Title, f)
	body := storage.PipeRead(ctx, done, func(ctx context.Context, output io.Writer) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw := model.Exportar(nb, notes, f)
		defer clear(raw)
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := output.Write(raw)
		return err
	})
	return body, name, nil
}

func (r *Repository) exportNotes(ctx context.Context, book, only string) (model.Notebook, []model.Note, error) {
	nb, err := r.GetNotebook(ctx, book)
	if err != nil {
		return model.Notebook{}, nil, err
	}
	raw, err := json.Marshal(nb)
	if err != nil {
		return model.Notebook{}, nil, err
	}
	var notebook model.Notebook
	err = json.Unmarshal(raw, &notebook)
	clear(raw)
	if err != nil {
		return model.Notebook{}, nil, err
	}
	current, err := r.ListNotes(ctx, book)
	if err != nil {
		return model.Notebook{}, nil, err
	}
	notes := make([]model.Note, 0, len(current))
	for _, n := range current {
		if only != "" && n.ID != only {
			continue
		}
		raw, err := json.Marshal(n)
		if err != nil {
			return model.Notebook{}, nil, err
		}
		var note model.Note
		err = json.Unmarshal(raw, &note)
		clear(raw)
		if err != nil {
			return model.Notebook{}, nil, err
		}
		notes = append(notes, note)
	}
	if only != "" {
		if len(notes) != 1 {
			return model.Notebook{}, nil, ErrNotFound
		}
		notebook.Title = model.TituloDeNota(notes[0])
	}
	if err := ctx.Err(); err != nil {
		return model.Notebook{}, nil, err
	}
	return notebook, notes, nil
}
