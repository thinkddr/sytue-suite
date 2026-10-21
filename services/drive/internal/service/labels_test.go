// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// confidencial es el snapshot de una etiqueta que prohíbe la compartición externa.
func confidencial() *domain.Label {
	return &domain.Label{
		ID: "22222222-2222-2222-2222-222222222222", Name: "Confidencial",
		Rank: 30, BlockExternalShare: true,
	}
}

// ENFORCEMENT: un fichero con etiqueta block_external_share NO puede obtener enlace
// público. Es el punto único por el que nace cualquier share kind=link.
func TestCreateShareLinkBlockedByLabel(t *testing.T) {
	fid := uuid.New()
	fs := &fakeStore{
		file:  domain.File{ID: fid, Label: confidencial()},
		share: domain.Share{ID: uuid.New(), Kind: domain.ShareLink},
	}
	s := newSvc(fs, &fakeBlobs{}, &capturePub{})

	_, err := s.CreateShare(tenantCtx(t), CreateShareInput{FileID: &fid, Kind: domain.ShareLink})
	if !errors.Is(err, domain.ErrLabelBlocksSharing) {
		t.Fatalf("se esperaba ErrLabelBlocksSharing, got %v", err)
	}
	if fs.newShare != nil {
		t.Fatal("no debió crearse ningún share")
	}
}

// El MISMO fichero SIN etiqueta bloqueante sí obtiene enlace (el bloqueo es de la
// etiqueta, no un «no» general).
func TestCreateShareLinkAllowedWithoutBlockingLabel(t *testing.T) {
	fid := uuid.New()
	for _, tc := range []struct {
		name  string
		label *domain.Label
	}{
		{"sin etiqueta", nil},
		{"etiqueta que solo informa", &domain.Label{ID: "1", Name: "Interno", Rank: 10}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeStore{
				file:  domain.File{ID: fid, Label: tc.label},
				share: domain.Share{ID: uuid.New(), Kind: domain.ShareLink},
			}
			s := newSvc(fs, &fakeBlobs{}, &capturePub{})
			if _, err := s.CreateShare(tenantCtx(t), CreateShareInput{FileID: &fid, Kind: domain.ShareLink}); err != nil {
				t.Fatalf("CreateShare: %v", err)
			}
			if fs.newShare == nil || fs.newShare.Token == "" {
				t.Fatal("debió crearse un enlace con token")
			}
		})
	}
}

// La etiqueta bloquea la salida FUERA de la organización, no el resto: compartir con
// una persona o un grupo del tenant sigue funcionando igual.
func TestLabelDoesNotBlockInternalShares(t *testing.T) {
	fid := uuid.New()
	for _, kind := range []domain.ShareKind{domain.ShareUser, domain.ShareGroup} {
		fs := &fakeStore{
			file:  domain.File{ID: fid, Label: confidencial()},
			share: domain.Share{ID: uuid.New(), Kind: kind},
		}
		s := newSvc(fs, &fakeBlobs{}, &capturePub{})
		if _, err := s.CreateShare(tenantCtx(t), CreateShareInput{
			FileID: &fid, Kind: kind, Subject: "otra@acme.eu",
		}); err != nil {
			t.Fatalf("share kind=%s: %v", kind, err)
		}
	}
}

// Etiquetar exige permiso de ESCRITURA: un viewer recibe ErrPermission y no se toca
// nada (mismo criterio que RestoreVersion/CopyFile).
func TestSetFileLabelRequiresWrite(t *testing.T) {
	fs := &fakeStore{file: domain.File{ID: uuid.New()}}
	s := newSvc(fs, &fakeBlobs{}, &capturePub{})
	lb := *confidencial()

	if _, err := s.SetFileLabel(tenantCtx(t), uuid.New(), lb, domain.PermRead); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("viewer: se esperaba ErrPermission, got %v", err)
	}
	if fs.labelSet != nil {
		t.Fatal("un viewer no debió escribir la etiqueta")
	}

	got, err := s.SetFileLabel(tenantCtx(t), uuid.New(), lb, domain.PermWrite)
	if err != nil {
		t.Fatalf("editor: %v", err)
	}
	if fs.labelSet == nil || fs.labelSet.Name != "Confidencial" || !fs.labelSet.BlockExternalShare {
		t.Fatalf("snapshot guardado incorrecto: %+v", fs.labelSet)
	}
	if got.Label == nil || !got.Label.BlockExternalShare {
		t.Fatalf("el fichero devuelto debería llevar la etiqueta: %+v", got.Label)
	}

	// Desetiquetar (snapshot vacío) también exige escritura y limpia la etiqueta.
	cleared, err := s.SetFileLabel(tenantCtx(t), uuid.New(), domain.Label{}, domain.PermWrite)
	if err != nil {
		t.Fatalf("desetiquetar: %v", err)
	}
	if cleared.Label != nil {
		t.Fatalf("desetiquetar debería dejar el fichero sin etiqueta: %+v", cleared.Label)
	}
}

// Enlace ya existente + etiqueta puesta DESPUÉS: el fichero deja de servirse por el
// enlace público (verja al servir, no solo al crear). Igual para un fichero
// etiquetado dentro de una carpeta compartida: no se lista ni se descarga.
func TestPublicBrowseHidesLabelBlockedFiles(t *testing.T) {
	folderID := uuid.New()
	visible := domain.File{ID: uuid.New(), Name: "publico.pdf"}
	oculto := domain.File{ID: uuid.New(), Name: "nomina.pdf", Label: confidencial()}

	// Enlace directo AL FICHERO etiquetado: 404 (no se sirve).
	fs := &fakeStore{file: oculto}
	s := newSvc(fs, &fakeBlobs{}, &capturePub{})
	fileID := oculto.ID
	if _, err := s.PublicBrowse(tenantCtx(t), domain.Share{FileID: &fileID, Kind: domain.ShareLink}, "acme", ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("enlace a fichero etiquetado: se esperaba ErrNotFound, got %v", err)
	}

	// Enlace A LA CARPETA: el etiquetado no aparece en el listado.
	fs2 := &fakeStore{
		folder: domain.Folder{ID: folderID, Name: "Compartida"},
		page:   store.FilePage{Files: []domain.File{visible, oculto}},
	}
	s2 := newSvc(fs2, &fakeBlobs{}, &capturePub{})
	node, err := s2.PublicBrowse(tenantCtx(t), domain.Share{FolderID: &folderID, Kind: domain.ShareLink}, "acme", "")
	if err != nil {
		t.Fatalf("PublicBrowse: %v", err)
	}
	if len(node.Files) != 1 || node.Files[0].Name != "publico.pdf" {
		t.Fatalf("el listado público no debe anunciar lo etiquetado: %+v", node.Files)
	}
}
