// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// Lo que Sytue Docs pide hacer con el fichero de un documento (DocsAccion) y lo
// que pasó.
const (
	DocsTirar     = "trash"
	DocsRestaurar = "restore"
	DocsPurgar    = "purge"
	// DocsComprobar dice qué haría DocsTirar sin hacerlo: la pasada en seco de la
	// reconciliación (`docs reconciliar-papelera`).
	DocsComprobar = "check"

	// DocsHecho: el fichero cambió de estado.
	DocsHecho = "done"
	// DocsYaEstaba: ya estaba donde se pedía (idempotencia).
	DocsYaEstaba = "unchanged"
	// DocsNoAplica: no hay fichero, no es de esa persona o no es el formato en que
	// nace un documento. Docs sigue con lo suyo y no toca la unidad.
	DocsNoAplica = "not_applicable"
	// DocsVivo: al purgar, el fichero es del documento pero sigue VIVO en la unidad,
	// y no se toca. La purga automática de Docs deja entonces el documento.
	DocsVivo = "live"
	// DocsModificado: al tirar o comprobar con `borradoEn`, el fichero cambió después
	// de borrarse el documento: alguien lo sigue usando y no va a la papelera.
	DocsModificado = "modified"
)

// ErrAccionDocs es una acción que Docs no puede pedir.
var ErrAccionDocs = errors.New("drive: acción de Docs desconocida")

// extensionDeDocs es el formato en que nace cada tipo de documento de Docs
// (services/docs/internal/http/hogar_en_drive.go). La mig 0027 encola los mismos.
var extensionDeDocs = map[string]string{"document": ".odt", "spreadsheet": ".ods", "presentation": ".odp"}

// EsFicheroDeDocs dice si un fichero de `ownerSub` es el hogar de uno de sus
// documentos de tipo `kind`. Se exige que el fichero sea SUYO —un documento abierto
// desde el fichero de otra persona no manda ese fichero a ninguna papelera— y que su
// formato sea el de ese tipo: una hoja no se lleva por delante un .odt.
func EsFicheroDeDocs(f domain.File, ownerSub, kind string) bool {
	ext, ok := extensionDeDocs[kind]
	return ok && ownerSub != "" && f.OwnerSub == ownerSub &&
		strings.ToLower(path.Ext(f.Name)) == ext
}

// DocsAccion aplica al fichero `id` lo que Docs ha hecho con su documento. La
// autorización la ha hecho Docs (solo el dueño del documento borra, restaura o
// purga) y aquí se acota a lo que ese permiso cubre: el fichero tiene que ser de
// esa persona y de un formato de Docs. La purga solo toca lo que ya está en la
// papelera: un fichero vivo no se borra definitivamente desde Docs.
//
// La purga comprueba ANTES la retención legal de esa persona, haya o no fichero: el
// contenido de un documento también es suyo, y Docs no tiene otra forma de saberlo.
//
// `borradoEn` (opcional) es cuándo se borró el documento: con él, tirar o comprobar
// no tocan un fichero modificado después (la reconciliación de la papelera heredada).
func (s *Service) DocsAccion(ctx context.Context, id uuid.UUID, ownerSub, kind, accion string, borradoEn time.Time) (string, error) {
	if accion == DocsPurgar {
		retenida, err := s.store.SujetoRetenido(ctx, ownerSub)
		if err != nil {
			return "", err
		}
		if retenida {
			return "", domain.ErrLegalHold
		}
	}
	if id == uuid.Nil {
		// Un documento sin fichero: solo se venía a preguntar por la retención.
		if accion != DocsPurgar {
			return "", ErrAccionDocs
		}
		return DocsNoAplica, nil
	}
	f, err := s.store.FicheroEnCualquierEstado(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return DocsNoAplica, nil
	}
	if err != nil {
		return "", err
	}
	if !EsFicheroDeDocs(f, ownerSub, kind) {
		return DocsNoAplica, nil
	}
	enPapelera := f.DeletedAt != nil
	modificado := !borradoEn.IsZero() && f.UpdatedAt.After(borradoEn)
	switch accion {
	case DocsComprobar, DocsTirar:
		switch {
		case enPapelera:
			return DocsYaEstaba, nil
		case modificado:
			return DocsModificado, nil
		case accion == DocsComprobar:
			return DocsHecho, nil
		}
		return DocsHecho, s.DeleteFile(ctx, id)
	case DocsRestaurar:
		if !enPapelera {
			return DocsYaEstaba, nil
		}
		_, err := s.RestoreFile(ctx, id)
		return DocsHecho, err
	case DocsPurgar:
		if !enPapelera {
			return DocsVivo, nil
		}
		return DocsHecho, s.PurgeFile(ctx, id)
	}
	return "", ErrAccionDocs
}
