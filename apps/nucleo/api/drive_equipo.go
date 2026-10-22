// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"
)

// # Unidades de equipo
//
// Una unidad de equipo es de la ORGANIZACIÓN, no de una persona: sus miembros entran con
// un rol —ver, editar o gestionar— y el contenido está aislado por unidad, así que se
// navega y se descarga por sus propias rutas (`/team-drives/{id}/…`) y no por las de «Mi
// unidad». Es lo que usa `TeamDrives.tsx` a través del BFF.

// Los tres roles de una unidad.
const (
	RolVer       = "viewer"
	RolEditar    = "editor"
	RolGestionar = "manager"
)

// UnidadEquipo es una unidad y el rol de quien la consulta.
type UnidadEquipo struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Role       string     `json:"role"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
}

// PuedeEscribir: editar y gestionar suben, crean y borran; ver solo lee.
func (u UnidadEquipo) PuedeEscribir() bool { return u.Role == RolEditar || u.Role == RolGestionar }

// MiembroEquipo es una persona de la unidad: su `sub` y su rol.
type MiembroEquipo struct {
	Subject string `json:"subject"`
	Role    string `json:"role"`
}

// ContenidoEquipo es una página de una carpeta de la unidad, con el rol de quien mira.
type ContenidoEquipo struct {
	Role       string   `json:"role"`
	CanWrite   bool     `json:"can_write"`
	Folders    []Folder `json:"folders"`
	Files      []File   `json:"files"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

func equipo(id, resto string) string { return "/team-drives/" + url.PathEscape(id) + resto }

// TeamDrives son las unidades de las que la persona es miembro.
func (d *Drive) TeamDrives(ctx context.Context) ([]UnidadEquipo, error) {
	r, err := JSON[struct {
		Items []UnidadEquipo `json:"items"`
	}](ctx, d.c, Request{Path: "/team-drives"})
	return r.Items, err
}

// CreateTeamDrive crea una unidad; quien la crea queda como gestor.
func (d *Drive) CreateTeamDrive(ctx context.Context, nombre string) (UnidadEquipo, error) {
	return PostJSON[UnidadEquipo](ctx, d.c, "/team-drives", map[string]string{"name": nombre})
}

// cambiarUnidad es el PATCH de renombrar y de archivar, que el servicio exige por
// separado.
func (d *Drive) cambiarUnidad(ctx context.Context, id string, cuerpo any) (UnidadEquipo, error) {
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return UnidadEquipo{}, err
	}
	return JSON[UnidadEquipo](ctx, d.c, Request{
		Method: http.MethodPatch, Path: equipo(id, ""), Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// RenameTeamDrive le cambia el nombre. Solo un gestor; si no, 403.
func (d *Drive) RenameTeamDrive(ctx context.Context, id, nombre string) (UnidadEquipo, error) {
	return d.cambiarUnidad(ctx, id, map[string]string{"name": nombre})
}

// ArchiveTeamDrive la archiva o la reactiva. Archivar es organizativo: no borra nada
// ni quita el acceso.
func (d *Drive) ArchiveTeamDrive(ctx context.Context, id string, archivada bool) (UnidadEquipo, error) {
	return d.cambiarUnidad(ctx, id, map[string]bool{"archived": archivada})
}

// TeamDriveMembers son los miembros de la unidad. Solo un gestor.
func (d *Drive) TeamDriveMembers(ctx context.Context, id string) ([]MiembroEquipo, error) {
	r, err := JSON[struct {
		Members []MiembroEquipo `json:"members"`
	}](ctx, d.c, Request{Path: equipo(id, "/members")})
	return r.Members, err
}

// PutTeamDriveMember añade a alguien o le cambia el rol. 409 es que se quedaría la
// unidad sin gestor.
func (d *Drive) PutTeamDriveMember(ctx context.Context, id, sujeto, rol string) error {
	return d.mandar(ctx, http.MethodPut, equipo(id, "/members"), map[string]string{"subject": sujeto, "role": rol})
}

// RemoveTeamDriveMember saca a alguien de la unidad.
func (d *Drive) RemoveTeamDriveMember(ctx context.Context, id, sujeto string) error {
	return d.mandar(ctx, http.MethodDelete, equipo(id, "/members/"+url.PathEscape(sujeto)), nil)
}

// TeamDriveChildren es una página de una carpeta de la unidad; `carpeta` vacía es su raíz.
func (d *Drive) TeamDriveChildren(ctx context.Context, id, carpeta, cursor string) (ContenidoEquipo, error) {
	q := url.Values{}
	if carpeta != "" {
		q.Set("folder", carpeta)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	return JSON[ContenidoEquipo](ctx, d.c, Request{Path: equipo(id, "/children"), Query: q})
}

// CreateTeamDriveFolder crea una carpeta en la unidad; `padre` vacío es su raíz.
func (d *Drive) CreateTeamDriveFolder(ctx context.Context, id, padre, nombre string) (Folder, error) {
	cuerpo := map[string]string{"name": nombre}
	if padre != "" {
		cuerpo["parent_id"] = padre
	}
	return PostJSON[Folder](ctx, d.c, equipo(id, "/folders"), cuerpo)
}

// UploadTeamDrive sube un fichero a una carpeta de la unidad, sin pasarlo por memoria.
func (d *Drive) UploadTeamDrive(ctx context.Context, id, carpeta, nombre, tipo string, cuerpo io.Reader, tamano int64) (File, error) {
	if tipo == "" {
		tipo = "application/octet-stream"
	}
	q := url.Values{"name": {nombre}}
	if carpeta != "" {
		q.Set("folder", carpeta)
	}
	resp, err := d.c.Stream(ctx, http.MethodPost, equipo(id, "/files"), q, http.Header{"Content-Type": {tipo}}, cuerpo, tamano)
	if err != nil {
		return File{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var f File
	err = json.NewDecoder(resp.Body).Decode(&f)
	return f, err
}

// DownloadTeamDrive abre el contenido de un fichero de la unidad.
func (d *Drive) DownloadTeamDrive(ctx context.Context, id, fichero string) (io.ReadCloser, error) {
	resp, err := d.c.Do(ctx, Request{Path: equipo(id, "/files/"+url.PathEscape(fichero)+"/content")})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// DeleteTeamDriveItem manda a la papelera un fichero o una carpeta de la unidad.
func (d *Drive) DeleteTeamDriveItem(ctx context.Context, id string, k DriveKind, elemento string) error {
	return d.mandar(ctx, http.MethodDelete, equipo(id, "/"+string(k)+"/"+url.PathEscape(elemento)), nil)
}
