// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

const (
	dropboxAPI     = "https://api.dropboxapi.com/2"
	dropboxContent = "https://content.dropboxapi.com/2"
	// Cada petición queda muy por debajo del límite de 150 MiB de Dropbox.
	dropboxTrozo = 8 << 20
)

// Dropbox conserva los objetos de cada organización bajo /Sytue/tenants/….
// El token lo custodia y renueva el servicio de integraciones.
type Dropbox struct {
	tok  TokenSource
	http *http.Client
}

// NewDropbox abre el adaptador con el token renovable de una conexión.
func NewDropbox(tok TokenSource) *Dropbox {
	return &Dropbox{tok: tok, http: clienteDeTerceros(60 * time.Second)}
}

func dropboxPath(t tenancy.Tenant, key string) string {
	return "/Sytue/" + t.StoragePrefix() + strings.TrimLeft(key, "/")
}

// content transmite los argumentos en la cabecera que exige la API de contenido;
// el JSON escapado también conserva rutas Unicode sin cabeceras no ASCII.
func (d *Dropbox) content(ctx context.Context, route string, args any, body io.Reader, size int64) (*http.Response, error) {
	arg, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	var escaped bytes.Buffer
	for _, r := range string(arg) {
		if r > 0x7f {
			// Los nombres de objeto son claves internas ASCII, pero los pares UTF-16
			// mantienen el contrato también para una ruta Unicode futura.
			if r > 0xffff {
				r -= 0x10000
				_, _ = fmt.Fprintf(&escaped, "\\u%04x\\u%04x", 0xd800+(r>>10), 0xdc00+(r&0x3ff))
			} else {
				_, _ = fmt.Fprintf(&escaped, "\\u%04x", r)
			}
		} else {
			_, _ = escaped.WriteRune(r)
		}
	}
	return conProveedor(ctx, d.http, d.tok, http.MethodPost, dropboxContent+route, body, size, map[string]string{
		"Content-Type": "application/octet-stream", "Dropbox-API-Arg": escaped.String(),
	})
}

func (d *Dropbox) rpc(ctx context.Context, route string, args any) (*http.Response, error) {
	body, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	return conProveedor(ctx, d.http, d.tok, http.MethodPost, dropboxAPI+route, bytes.NewReader(body), int64(len(body)), map[string]string{"Content-Type": "application/json"})
}

func (d *Dropbox) Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, _ PutOptions) error {
	body, n, cerrar, err := spool(r, size)
	if err != nil {
		return err
	}
	defer cerrar()
	commit := map[string]any{"path": dropboxPath(t, key), "mode": "overwrite", "autorename": false, "mute": true}
	if n <= dropboxTrozo {
		return d.upload(ctx, "/files/upload", commit, body, n)
	}
	res, err := d.content(ctx, "/files/upload_session/start", map[string]bool{"close": false}, io.NewSectionReader(body, 0, dropboxTrozo), dropboxTrozo)
	if err != nil {
		return err
	}
	var session struct {
		ID string `json:"session_id"`
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return errorDe(res, "dropbox: abrir subida")
	}
	err = json.NewDecoder(res.Body).Decode(&session)
	_ = res.Body.Close()
	if err != nil {
		return err
	}
	if session.ID == "" {
		return errors.New("dropbox: la sesión de subida no tiene identificador")
	}
	for offset := int64(dropboxTrozo); offset < n; {
		length := min(int64(dropboxTrozo), n-offset)
		args := map[string]any{"cursor": map[string]any{"session_id": session.ID, "offset": offset}}
		route := "/files/upload_session/append_v2"
		if offset+length == n {
			route = "/files/upload_session/finish"
			args["commit"] = commit
		}
		if err := d.upload(ctx, route, args, io.NewSectionReader(body, offset, length), length); err != nil {
			return err
		}
		offset += length
	}
	return nil
}

func (d *Dropbox) upload(ctx context.Context, route string, args any, body io.Reader, size int64) error {
	res, err := d.content(ctx, route, args, body, size)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errorDe(res, "dropbox: subir")
	}
	_, err = io.Copy(io.Discard, res.Body)
	return err
}

// Dropbox expresa «no existe» como un conflicto de ruta (HTTP 409), no un 404.
func dropboxError(res *http.Response, op string) error {
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
	var out struct {
		Summary string `json:"error_summary"`
	}
	_ = json.Unmarshal(body, &out)
	if res.StatusCode == http.StatusConflict && strings.HasPrefix(out.Summary, "path/not_found/") {
		return ErrNotFound
	}
	return fmt.Errorf("dropbox: %s: %s: %s", op, res.Status, strings.TrimSpace(string(body)))
}

func (d *Dropbox) Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	res, err := d.content(ctx, "/files/download", map[string]string{"path": dropboxPath(t, key)}, http.NoBody, 0)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return nil, dropboxError(res, "descargar")
	}
	return res.Body, nil
}

func (d *Dropbox) Delete(ctx context.Context, t tenancy.Tenant, key string) error {
	res, err := d.rpc(ctx, "/files/delete_v2", map[string]string{"path": dropboxPath(t, key)})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusOK {
		return nil
	}
	err = dropboxError(res, "borrar")
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func (d *Dropbox) Stat(ctx context.Context, t tenancy.Tenant, key string) (int64, error) {
	res, err := d.rpc(ctx, "/files/get_metadata", map[string]string{"path": dropboxPath(t, key)})
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, dropboxError(res, "consultar")
	}
	var out struct {
		Size int64 `json:"size"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.Size, nil
}

func (d *Dropbox) Existe(ctx context.Context, t tenancy.Tenant, key string) (bool, error) {
	_, err := d.Stat(ctx, t, key)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (d *Dropbox) Copiar(ctx context.Context, t tenancy.Tenant, origen, destino string) error {
	rc, err := d.Get(ctx, t, origen)
	if err != nil {
		return err
	}
	defer rc.Close()
	return d.Put(ctx, t, destino, rc, -1, PutOptions{})
}

func (d *Dropbox) PresignGet(context.Context, tenancy.Tenant, string, time.Duration) (*url.URL, error) {
	return nil, ErrPresignUnsupported
}

func (d *Dropbox) PresignPut(context.Context, tenancy.Tenant, string, time.Duration) (*url.URL, error) {
	return nil, ErrPresignUnsupported
}

func (d *Dropbox) Probar(ctx context.Context) error {
	// list_folder comprueba el acceso a archivos incluso con permiso App Folder;
	// no exige leer el perfil de la persona ni escribe archivos de prueba.
	res, err := d.rpc(ctx, "/files/list_folder", map[string]any{"path": "", "limit": 1})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return dropboxError(res, "comprobar")
	}
	return nil
}

var _ BlobStore = (*Dropbox)(nil)
