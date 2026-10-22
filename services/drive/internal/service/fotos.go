// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// CarpetaFotos es donde van los originales de Sytue Photos: la misma que usa «Añadir
// fotos» de la galería (`pages/api/drive-subir.ts`).
const CarpetaFotos = "/Fotos"

// CopiaParaFotos deja en el Drive de `duenya` una copia del fichero `src`: es lo que pasa
// cuando alguien con edición en un álbum compartido de Photos le añade una foto suya, para
// que el álbum no dependa de que esa persona conserve su original. La copia es de la
// dueña (`owner_sub`), va a /Fotos, cuenta contra la cuota de la organización
// (ErrQuotaExceeded si no cabe) y no se repite: si ya tiene viva una con la misma huella,
// se devuelve esa (`reusada`).
//
// Es de SERVICIO A SERVICIO (la llama syphotos por /internal, con el secreto de la
// suite): quien la pide ya ha comprobado que `editora` puede editar ese álbum. Aquí solo
// se copia un fichero DE `editora` y dentro de la MISMA organización (RLS).
func (s *Service) CopiaParaFotos(ctx context.Context, src uuid.UUID, duenya, editora string) (f domain.File, reusada bool, err error) {
	if duenya == "" || editora == "" {
		return domain.File{}, false, domain.ErrInvalidName
	}
	// Primero que el origen existe: una organización que no existe no llega a crear ni
	// raíz, ni cuota, ni /Fotos.
	if _, err := s.store.GetFile(store.WithOwner(ctx, editora), src); err != nil {
		return domain.File{}, false, err
	}
	carpeta, err := s.carpetaFotos(store.WithOwner(ctx, duenya), duenya)
	if err != nil {
		return domain.File{}, false, err
	}
	f, reusada, err = s.store.CopiaParaDuenya(ctx, src, carpeta, duenya, editora)
	if err != nil {
		return domain.File{}, false, err
	}
	if !reusada {
		s.publish(ctx, "drive.file.copied", map[string]any{
			"file_id": f.ID, "source_id": src, "name": f.Name, "size": f.SizeBytes,
		})
	}
	return f, reusada, nil
}

// carpetaFotos es /Fotos, creándola a nombre de `duenya` si no está.
func (s *Service) carpetaFotos(ctx context.Context, duenya string) (uuid.UUID, error) {
	n, err := s.ResolvePath(ctx, CarpetaFotos)
	if err == nil && n.IsDir() {
		return n.Folder.ID, nil
	}
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return uuid.Nil, err
	}
	root, err := s.Root(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	c, err := s.CreateFolder(ctx, root.ID, CarpetaFotos[1:], duenya, false)
	if errors.Is(err, domain.ErrConflict) {
		// Otra petición la ha creado a la vez: la suya vale.
		if n, e := s.ResolvePath(ctx, CarpetaFotos); e == nil && n.IsDir() {
			return n.Folder.ID, nil
		}
	}
	return c.ID, err
}
