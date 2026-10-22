// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// Node identifica un recurso resuelto por ruta dentro del árbol del tenant.
// Exactamente uno de Folder/File está poblado.
type Node struct {
	Folder *domain.Folder
	File   *domain.File
}

// IsDir indica si el nodo es una carpeta.
func (n Node) IsDir() bool { return n.Folder != nil }

// splitPath normaliza una ruta WebDAV ("/a/b/c") en segmentos limpios.
func splitPath(p string) []string {
	p = path.Clean("/" + strings.TrimSpace(p))
	if p == "/" || p == "." {
		return nil
	}
	segs := strings.Split(strings.Trim(p, "/"), "/")
	out := segs[:0]
	for _, s := range segs {
		if s != "" && s != "." {
			out = append(out, s)
		}
	}
	return out
}

// ResolvePath resuelve una ruta absoluta a un Node (carpeta o fichero),
// partiendo de la raíz del tenant. La ruta vacía o "/" devuelve la raíz.
func (s *Service) ResolvePath(ctx context.Context, p string) (Node, error) {
	root, err := s.Root(ctx)
	if err != nil {
		return Node{}, err
	}
	segs := splitPath(p)
	if len(segs) == 0 {
		r := root
		return Node{Folder: &r}, nil
	}

	// El descenso entero lo hace el store en UNA transacción. Antes se iba tramo a
	// tramo: una transacción por segmento, cada una trayendo TODAS las subcarpetas
	// del padre para quedarse con una, y encima `ListChildren` traía además todas
	// las filas de ficheros de un tramo intermedio para descartarlas enteras.
	// WebDAV resuelve por ruta en cada PROPFIND, GET y PUT, así que se pagaba
	// siempre y crecía con la profundidad de la ruta.
	carpeta, fichero, err := s.store.ResolveUnder(ctx, root.ID, segs)
	if err != nil {
		return Node{}, err
	}
	switch {
	case fichero != nil:
		return Node{File: fichero}, nil
	case carpeta != nil:
		return Node{Folder: carpeta}, nil
	}
	return Node{}, domain.ErrNotFound
}

// resolveParent devuelve la carpeta padre de una ruta y el nombre del último
// segmento. Útil para crear/escribir/borrar.
func (s *Service) resolveParent(ctx context.Context, p string) (parentID uuid.UUID, name string, err error) {
	segs := splitPath(p)
	if len(segs) == 0 {
		return uuid.Nil, "", domain.ErrInvalidName
	}
	name = segs[len(segs)-1]
	parentPath := "/" + strings.Join(segs[:len(segs)-1], "/")
	parent, err := s.ResolvePath(ctx, parentPath)
	if err != nil {
		return uuid.Nil, "", err
	}
	if !parent.IsDir() {
		return uuid.Nil, "", domain.ErrInvalidParent
	}
	return parent.Folder.ID, name, nil
}

// MkdirPath crea una carpeta en la ruta dada (su padre debe existir).
func (s *Service) MkdirPath(ctx context.Context, p string) error {
	parentID, name, err := s.resolveParent(ctx, p)
	if err != nil {
		return err
	}
	owner := store.OwnerFrom(ctx)
	_, err = s.CreateFolder(ctx, parentID, name, owner, false)
	return err
}

// WritePath sube/sustituye el fichero de la ruta dada con el contenido leído.
func (s *Service) WritePath(ctx context.Context, p, contentType string, body io.Reader) (domain.File, error) {
	parentID, name, err := s.resolveParent(ctx, p)
	if err != nil {
		return domain.File{}, err
	}
	return s.UploadFile(ctx, parentID, name, contentType, store.OwnerFrom(ctx), body)
}

// RemovePath borra el recurso (fichero o carpeta) de la ruta dada.
func (s *Service) RemovePath(ctx context.Context, p string) error {
	node, err := s.ResolvePath(ctx, p)
	if err != nil {
		return err
	}
	if node.IsDir() {
		if node.Folder.IsRoot {
			return domain.ErrRootImmutable
		}
		return s.DeleteFolder(ctx, node.Folder.ID)
	}
	return s.DeleteFile(ctx, node.File.ID)
}

// RenamePath mueve/renombra un recurso de oldPath a newPath dentro del tenant.
func (s *Service) RenamePath(ctx context.Context, oldPath, newPath string) error {
	node, err := s.ResolvePath(ctx, oldPath)
	if err != nil {
		return err
	}
	newParentID, newName, err := s.resolveParent(ctx, newPath)
	if err != nil {
		return err
	}
	if node.IsDir() {
		if _, err := s.MoveFolder(ctx, node.Folder.ID, newParentID); err != nil {
			return err
		}
		if !strings.EqualFold(node.Folder.Name, newName) {
			_, err = s.RenameFolder(ctx, node.Folder.ID, newName)
		}
		return err
	}
	if _, err := s.MoveFile(ctx, node.File.ID, newParentID); err != nil {
		return err
	}
	if !strings.EqualFold(node.File.Name, newName) {
		_, err = s.RenameFile(ctx, node.File.ID, newName)
	}
	return err
}

// OpenContentByFile abre el blob de un fichero ya resuelto.
func (s *Service) OpenContentByFile(ctx context.Context, f domain.File) (io.ReadCloser, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	rc, err := s.blobs.Get(ctx, t, f.StorageKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return rc, nil
}
