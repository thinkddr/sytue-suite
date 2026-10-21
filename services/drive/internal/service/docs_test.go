// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// TestDocsAccion: lo que Docs pide solo toca el fichero de un documento de esa
// persona, es idempotente y la purga no alcanza a un fichero vivo.
func TestDocsAccion(t *testing.T) {
	ayer := time.Now().Add(-24 * time.Hour)
	suyo := domain.File{ID: uuid.New(), Name: "Informe.odt", OwnerSub: "ana"}
	tirado := suyo
	tirado.DeletedAt = &ayer
	casos := []struct {
		nombre, accion, dueno, kind string
		fichero                     domain.File
		ficheroErr                  error
		quiero                      string
		evento                      string
	}{
		{"tirar el suyo", DocsTirar, "ana", "document", suyo, nil, DocsHecho, "drive.file.deleted"},
		{"comprobar no toca", DocsComprobar, "ana", "document", suyo, nil, DocsHecho, ""},
		{"comprobar uno tirado", DocsComprobar, "ana", "document", tirado, nil, DocsYaEstaba, ""},
		{"comprobar el de otra", DocsComprobar, "bea", "document", suyo, nil, DocsNoAplica, ""},
		{"tirar otra vez", DocsTirar, "ana", "document", tirado, nil, DocsYaEstaba, ""},
		{"restaurar", DocsRestaurar, "ana", "document", tirado, nil, DocsHecho, "drive.file.restored"},
		{"restaurar el vivo", DocsRestaurar, "ana", "document", suyo, nil, DocsYaEstaba, ""},
		{"purgar de la papelera", DocsPurgar, "ana", "document", tirado, nil, DocsHecho, "drive.file.purged"},
		{"purgar un vivo NO", DocsPurgar, "ana", "document", suyo, nil, DocsVivo, ""},
		{"de otra persona NO", DocsTirar, "bea", "document", suyo, nil, DocsNoAplica, ""},
		{"sin dueño NO", DocsTirar, "", "document", domain.File{Name: "x.odt"}, nil, DocsNoAplica, ""},
		{"otro formato NO", DocsTirar, "ana", "document", domain.File{Name: "Informe.docx", OwnerSub: "ana"}, nil, DocsNoAplica, ""},
		{"una hoja no toca un .odt", DocsTirar, "ana", "spreadsheet", suyo, nil, DocsNoAplica, ""},
		{"ya purgado", DocsPurgar, "ana", "document", domain.File{}, domain.ErrNotFound, DocsNoAplica, ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			fs := &fakeStore{file: c.fichero, fileErr: c.ficheroErr}
			pub := &capturePub{}
			s := newSvc(fs, &fakeBlobs{}, pub)
			got, err := s.DocsAccion(tenantCtx(t), uuid.New(), c.dueno, c.kind, c.accion, time.Time{})
			if err != nil {
				t.Fatalf("DocsAccion: %v", err)
			}
			if got != c.quiero {
				t.Fatalf("resultado = %q, quiero %q", got, c.quiero)
			}
			if c.evento == "" && len(pub.types) != 0 {
				t.Fatalf("no debía tocar el fichero y emitió %v", pub.types)
			}
			if c.evento != "" && (len(pub.types) != 1 || pub.types[0] != c.evento) {
				t.Fatalf("eventos = %v, quiero %s", pub.types, c.evento)
			}
		})
	}
	if _, err := newSvc(&fakeStore{file: suyo}, &fakeBlobs{}, &capturePub{}).
		DocsAccion(tenantCtx(t), uuid.New(), "ana", "document", "borrar", time.Time{}); !errors.Is(err, ErrAccionDocs) {
		t.Fatalf("acción desconocida = %v", err)
	}
	// Retención legal: ni con fichero en la papelera, ni sin fichero, se purga.
	retenida := newSvc(&fakeStore{file: tirado, retenido: true}, &fakeBlobs{}, &capturePub{})
	for _, id := range []uuid.UUID{uuid.New(), uuid.Nil} {
		if _, err := retenida.DocsAccion(tenantCtx(t), id, "ana", "document", DocsPurgar, time.Time{}); !errors.Is(err, domain.ErrLegalHold) {
			t.Fatalf("purgar con retención (fichero %v) = %v", id, err)
		}
	}
	if got, err := newSvc(&fakeStore{}, &fakeBlobs{}, &capturePub{}).
		DocsAccion(tenantCtx(t), uuid.Nil, "ana", "document", DocsPurgar, time.Time{}); err != nil || got != DocsNoAplica {
		t.Fatalf("purgar sin fichero y sin retención = %q, %v", got, err)
	}
	// Modificado después de borrarse el documento: ni se tira ni cuenta como a tirar.
	editado := suyo
	editado.UpdatedAt = time.Now()
	for _, a := range []string{DocsComprobar, DocsTirar} {
		pub := &capturePub{}
		got, err := newSvc(&fakeStore{file: editado}, &fakeBlobs{}, pub).
			DocsAccion(tenantCtx(t), uuid.New(), "ana", "document", a, ayer)
		if err != nil || got != DocsModificado || len(pub.types) != 0 {
			t.Fatalf("%s de uno modificado = %q, %v, eventos %v", a, got, err, pub.types)
		}
	}
}
