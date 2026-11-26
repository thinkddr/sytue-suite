// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/bases/dominio"
	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// Adjuntos de un registro (B-04). Los BYTES son un fichero retenido del motor de
// registros: pasan por el puerto (en Sytue Office, `local/almacen`, en claro y con su
// huella por nombre) y se comprueban al leerlos. La FICHA (nombre, tipo, tamaño) es una
// fila `file` que cita ese fichero; el registro no cambia al adjuntar.

const (
	binarioAdjunto   = "attachment"
	operacionAdjunto = "bases-attachment-v1"
	ladoMiniatura    = 512
)

// fichaAdjunto es la fila `file`: la ficha que ven las pantallas y el fichero retenido.
type fichaAdjunto struct {
	api.BasesAttachment
	Retenido records.RetainedFile `json:"retained"`
}

func (r *Repository) UploadAttachment(ctx context.Context, registro, campo, nombre string, datos []byte) (api.BasesAttachment, error) {
	nombre, err := dominio.NombreDeAdjunto(nombre)
	if err != nil {
		return api.BasesAttachment{}, errorDe(err)
	}
	if len(datos) > dominio.MaxBytesAdjunto {
		return api.BasesAttachment{}, errorDe(dominio.ErrLimite)
	}
	tipo, err := dominio.TipoDeContenido(nombre, datos)
	if err != nil {
		return api.BasesAttachment{}, errorDe(err)
	}
	rec, err := r.GetRecord(ctx, registro)
	if err != nil {
		return api.BasesAttachment{}, err
	}
	campos, err := r.camposDeTabla(ctx, rec.TableID)
	if err != nil {
		return api.BasesAttachment{}, err
	}
	if !slices.ContainsFunc(campos, func(f api.BasesField) bool { return f.ID == campo && f.Kind == api.BasesKindAttachment }) {
		return api.BasesAttachment{}, errorDe(dominio.ErrInvalido)
	}
	suma := sha256.Sum256(datos)
	e := entradaAdjunto{Registro: registro, Campo: campo, Nombre: nombre, Tipo: tipo, Tamano: int64(len(datos)), SHA256: hex.EncodeToString(suma[:])}
	contenido := r.contenidoDeAdjunto(e, datos)
	raw, err := r.mutarCon(ctx, "attachment-create", e, prepararNueva("file", func(ctx context.Context) (string, fichaAdjunto, error) {
		// El tope, con lo que hay al preparar (una operación repetida no se prepara:
		// su ficha ya está entre las que cuentan).
		fichas, err := leerTodos[fichaAdjunto](ctx, r, "file")
		if err != nil {
			return "", fichaAdjunto{}, err
		}
		n := 0
		for _, f := range fichas {
			if f.RecordID == registro && f.FieldID == campo {
				n++
			}
		}
		if n >= dominio.MaxAdjuntosPorCampo {
			return "", fichaAdjunto{}, errorDe(dominio.ErrLimite)
		}
		f := fichaAdjunto{BasesAttachment: api.BasesAttachment{
			ID: uuid.NewString(), RecordID: registro, FieldID: campo, Name: nombre, Mime: tipo, Size: e.Tamano,
			HasThumb: conMiniatura(tipo), CreatedBy: r.scope.Subject, CreatedAt: time.Now().UTC(),
		}}
		return f.ID, f, nil
	}), &contenido)
	ficha, err := escrita[fichaAdjunto](raw, err)
	if err != nil {
		return api.BasesAttachment{}, err
	}
	return ficha.BasesAttachment, nil
}

// entradaAdjunto es la operación de adjuntar tal como la pide quien llama (sin el id de
// la ficha ni la hora): repetirla tras un corte es la MISMA mutación del motor, que la
// retoma de su diario y da la misma ficha con el mismo fichero, sin otra ficha ni otra
// subida.
type entradaAdjunto struct {
	Registro string `json:"record"`
	Campo    string `json:"field"`
	Nombre   string `json:"name"`
	Tipo     string `json:"mime"`
	Tamano   int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

// fila es el fichero retenido de la ficha id.
func (e entradaAdjunto) fila(id string) records.SnapshotRow {
	return records.SnapshotRow{Kind: binarioAdjunto, ID: id, Size: e.Tamano, SHA256: e.SHA256}
}

// ficha descodifica la ficha que prepara (o que retoma) la mutación de e.
func (e entradaAdjunto) ficha(raw []byte) (fichaAdjunto, error) {
	var f fichaAdjunto
	if json.Unmarshal(raw, &f) != nil || f.RecordID != e.Registro || f.FieldID != e.Campo || f.Name != e.Nombre || f.Mime != e.Tipo || f.Size != e.Tamano || !storage.Identifier(f.ID) {
		return fichaAdjunto{}, ErrBases
	}
	return f, nil
}

// contenidoDeAdjunto sube los bytes como fichero retenido del motor DESPUÉS de dejar la
// ficha en su diario (`MutateWithContent`, como las fotos de Photos): un corte en la
// subida o en la publicación se retoma con la misma ficha y el mismo fichero. La ficha
// publicada cita el fichero, y solo entonces la subida queda cerrada.
func (r *Repository) contenidoDeAdjunto(e entradaAdjunto, datos []byte) records.MutationContent {
	return records.MutationContent{
		Capture: func(_ context.Context, raw []byte, _ storage.PublicationTarget) ([]byte, error) {
			if _, err := e.ficha(raw); err != nil {
				return nil, err
			}
			return json.Marshal(e)
		},
		Prepare: func(ctx context.Context, raw, _ []byte, destino storage.PublicationTarget) ([]byte, error) {
			f, err := e.ficha(raw)
			if err != nil {
				return nil, err
			}
			f.Retenido, err = r.core.StageRetainedFile(ctx, operacionAdjunto, destino, e.fila(f.ID), func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(datos)), nil
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(f)
		},
		Validate: func(ctx context.Context, raw, _ []byte, destino storage.PublicationTarget) error {
			f, err := e.ficha(raw)
			if err != nil {
				return err
			}
			if f.Retenido.Target != destino || f.Retenido.Row != e.fila(f.ID) {
				return ErrBases
			}
			return r.core.ValidateRetainedFile(ctx, f.Retenido)
		},
		Acknowledge: func(ctx context.Context, raw, _ []byte, _ storage.PublicationTarget, aceptada storage.ObjectVersion) error {
			f, err := e.ficha(raw)
			if err != nil {
				return err
			}
			return r.core.AcknowledgeRetainedFiles(ctx, operacionAdjunto, []records.RetainedFile{f.Retenido}, aceptada, func(publicada []byte) ([]records.RetainedFile, error) {
				var x fichaAdjunto
				if json.Unmarshal(publicada, &x) != nil || x.Retenido != f.Retenido {
					return nil, ErrBases
				}
				return []records.RetainedFile{x.Retenido}, nil
			})
		},
	}
}

func conMiniatura(tipo string) bool {
	switch tipo {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

func (r *Repository) ficha(ctx context.Context, registro, adjunto string) (fichaAdjunto, error) {
	f, err := leerUno[fichaAdjunto](ctx, r, "file", adjunto)
	if err != nil {
		return f, err
	}
	if f.RecordID != registro {
		return f, errorDe(dominio.ErrNoExiste)
	}
	return f, nil
}

func (r *Repository) leerAdjunto(ctx context.Context, f fichaAdjunto) ([]byte, error) {
	var b bytes.Buffer
	if err := r.core.ReadRetainedFile(ctx, f.Retenido, &b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (r *Repository) DownloadAttachment(ctx context.Context, registro, adjunto string) (io.ReadCloser, error) {
	f, err := r.ficha(ctx, registro, adjunto)
	if err != nil {
		return nil, err
	}
	datos, err := r.leerAdjunto(ctx, f)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(datos)), nil
}

// DownloadThumb es la miniatura de un adjunto de imagen, hecha al pedirla.
func (r *Repository) DownloadThumb(ctx context.Context, registro, adjunto string) (io.ReadCloser, error) {
	f, err := r.ficha(ctx, registro, adjunto)
	if err != nil {
		return nil, err
	}
	if !f.HasThumb {
		return nil, errorDe(dominio.ErrNoExiste)
	}
	datos, err := r.leerAdjunto(ctx, f)
	if err != nil {
		return nil, err
	}
	salida := "image/jpeg"
	if strings.HasSuffix(f.Mime, "png") || strings.HasSuffix(f.Mime, "gif") {
		salida = "image/png"
	}
	// Con el contexto: quien ya no la quiere deja de esperar el turno de decodificar.
	mini, err := miniatura.RenderConContexto(ctx, func(context.Context) ([]byte, error) { return datos, nil }, ladoMiniatura, salida)
	if err != nil {
		if cancelado := ctx.Err(); cancelado != nil {
			return nil, cancelado
		}
		return nil, errorDe(dominio.ErrNoExiste)
	}
	return io.NopCloser(bytes.NewReader(mini)), nil
}

// DeleteAttachment quita la ficha; el fichero sigue en el almacén (no hay recolector).
func (r *Repository) DeleteAttachment(ctx context.Context, registro, adjunto string) error {
	if _, err := r.ficha(ctx, registro, adjunto); err != nil {
		return err
	}
	if r.SoltarAdjuntos {
		// Sin `retained` nada cita ya el fichero: la compactación lo quita del disco.
		return r.trashRow(ctx, "attachment-delete", "file", adjunto, "retained")
	}
	return r.trashRow(ctx, "attachment-delete", "file", adjunto)
}

var _ api.BasesAttachmentData = (*Repository)(nil)
