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
	"strconv"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// ExternalObjects persiste el mapa clave de blob → id del proveedor por tenant.
// GetExternalObject devuelve ErrNotFound para una clave ausente. El índice debe
// estar vinculado a la misma conexión que el TokenSource de Google Drive.
type ExternalObjects interface {
	PutExternalObject(ctx context.Context, tenant, key, providerID string) error
	GetExternalObject(ctx context.Context, tenant, key string) (string, error)
	DeleteExternalObject(ctx context.Context, tenant, key string) error
}

const (
	gdriveAPI    = "https://www.googleapis.com/drive/v3"
	gdriveUpload = "https://www.googleapis.com/upload/drive/v3/files?uploadType=resumable"
	gdriveFolder = "application/vnd.google-apps.folder"
	gdriveRaiz   = "Sytue"
)

// GoogleDrive guarda los blobs de un tenant en una carpeta «Sytue» de su Google
// Drive (alcance drive.file: solo ve lo que ha creado). Los nombres son las
// claves de blob; el id que Google asigna se guarda en ExternalObjects.
type GoogleDrive struct {
	tok  TokenSource
	objs ExternalObjects
	http *http.Client

	mu   sync.Mutex
	raiz string // id de la carpeta Sytue, una vez encontrada o creada
}

// NewGoogleDrive abre el adaptador con un token renovable y un índice persistente.
// El llamador fija un token y un índice del mismo propietario y conexión.
func NewGoogleDrive(tok TokenSource, objs ExternalObjects) *GoogleDrive {
	return &GoogleDrive{tok: tok, objs: objs, http: clienteDeTerceros(60 * time.Second)}
}

func (g *GoogleDrive) carpeta(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.raiz != "" {
		return g.raiz, nil
	}
	q := url.Values{"q": {fmt.Sprintf("name = '%s' and mimeType = '%s' and trashed = false", gdriveRaiz, gdriveFolder)}, "fields": {"files(id)"}, "spaces": {"drive"}}
	res, err := conProveedor(ctx, g.http, g.tok, http.MethodGet, gdriveAPI+"/files?"+q.Encode(), http.NoBody, 0, nil)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", errorDe(res, "google drive: buscar carpeta")
	}
	var out struct {
		Files []struct{ ID string } `json:"files"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Files) > 0 {
		g.raiz = out.Files[0].ID
		return g.raiz, nil
	}
	meta, _ := json.Marshal(map[string]any{"name": gdriveRaiz, "mimeType": gdriveFolder})
	res2, err := conProveedor(ctx, g.http, g.tok, http.MethodPost, gdriveAPI+"/files?fields=id", bytes.NewReader(meta), int64(len(meta)), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return "", err
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		return "", errorDe(res2, "google drive: crear carpeta")
	}
	var creada struct{ ID string }
	if err := json.NewDecoder(res2.Body).Decode(&creada); err != nil {
		return "", err
	}
	g.raiz = creada.ID
	return g.raiz, nil
}

func (g *GoogleDrive) Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts PutOptions) error {
	raiz, err := g.carpeta(ctx)
	if err != nil {
		return err
	}
	body, n, cerrar, err := spool(r, size)
	if err != nil {
		return err
	}
	defer cerrar()
	ct := opts.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	// Subida reanudable en una sola petición: primero los metadatos, luego el cuerpo
	// entero en la URL de sesión. Sirve para cualquier tamaño.
	meta, _ := json.Marshal(map[string]any{"name": t.StoragePrefix() + key, "parents": []string{raiz}})
	res, err := conProveedor(ctx, g.http, g.tok, http.MethodPost, gdriveUpload+"&fields=id", bytes.NewReader(meta), int64(len(meta)),
		map[string]string{"Content-Type": "application/json; charset=UTF-8", "X-Upload-Content-Type": ct, "X-Upload-Content-Length": strconv.FormatInt(n, 10)})
	if err != nil {
		return err
	}
	sesion := res.Header.Get("Location")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || sesion == "" {
		return fmt.Errorf("google drive: iniciar subida: %s", res.Status)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, sesion, body)
	if err != nil {
		return err
	}
	req.ContentLength = n
	req.Header.Set("Content-Type", ct)
	res2, err := g.http.Do(req)
	if err != nil {
		return fmt.Errorf("google drive: subir: %w", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK && res2.StatusCode != http.StatusCreated {
		return errorDe(res2, "google drive: subir")
	}
	var creado struct{ ID string }
	if err := json.NewDecoder(res2.Body).Decode(&creado); err != nil || creado.ID == "" {
		return fmt.Errorf("google drive: la subida no devolvió id")
	}
	return g.objs.PutExternalObject(ctx, t.Slug, key, creado.ID)
}

func (g *GoogleDrive) id(ctx context.Context, t tenancy.Tenant, key string) (string, error) {
	id, err := g.objs.GetExternalObject(ctx, t.Slug, key)
	if errors.Is(err, ErrNotFound) {
		return "", ErrNotFound
	}
	return id, err
}

func (g *GoogleDrive) Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	id, err := g.id(ctx, t, key)
	if err != nil {
		return nil, err
	}
	res, err := conProveedor(ctx, g.http, g.tok, http.MethodGet, gdriveAPI+"/files/"+url.PathEscape(id)+"?alt=media", http.NoBody, 0, nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusNotFound {
		_ = res.Body.Close()
		return nil, ErrNotFound
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return nil, errorDe(res, "google drive: descargar")
	}
	return res.Body, nil
}

func (g *GoogleDrive) Delete(ctx context.Context, t tenancy.Tenant, key string) error {
	id, err := g.id(ctx, t, key)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	res, err := conProveedor(ctx, g.http, g.tok, http.MethodDelete, gdriveAPI+"/files/"+url.PathEscape(id), http.NoBody, 0, nil)
	if err != nil {
		return err
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusNotFound {
		return fmt.Errorf("google drive: borrar: %s", res.Status)
	}
	return g.objs.DeleteExternalObject(ctx, t.Slug, key)
}

func (g *GoogleDrive) Stat(ctx context.Context, t tenancy.Tenant, key string) (int64, error) {
	id, err := g.id(ctx, t, key)
	if err != nil {
		return 0, err
	}
	res, err := conProveedor(ctx, g.http, g.tok, http.MethodGet, gdriveAPI+"/files/"+url.PathEscape(id)+"?fields=size", http.NoBody, 0, nil)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return 0, ErrNotFound
	}
	if res.StatusCode != http.StatusOK {
		return 0, errorDe(res, "google drive: consultar")
	}
	var out struct{ Size string }
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return 0, err
	}
	n, _ := strconv.ParseInt(out.Size, 10, 64)
	return n, nil
}

func (g *GoogleDrive) Existe(ctx context.Context, t tenancy.Tenant, key string) (bool, error) {
	_, err := g.Stat(ctx, t, key)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (g *GoogleDrive) Copiar(ctx context.Context, t tenancy.Tenant, origen, destino string) error {
	id, err := g.id(ctx, t, origen)
	if err != nil {
		return err
	}
	raiz, err := g.carpeta(ctx)
	if err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]any{"name": t.StoragePrefix() + destino, "parents": []string{raiz}})
	res, err := conProveedor(ctx, g.http, g.tok, http.MethodPost, gdriveAPI+"/files/"+url.PathEscape(id)+"/copy?fields=id", bytes.NewReader(meta), int64(len(meta)), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errorDe(res, "google drive: copiar")
	}
	var copia struct{ ID string }
	if err := json.NewDecoder(res.Body).Decode(&copia); err != nil || copia.ID == "" {
		return fmt.Errorf("google drive: la copia no devolvió id")
	}
	return g.objs.PutExternalObject(ctx, t.Slug, destino, copia.ID)
}

func (g *GoogleDrive) PresignGet(context.Context, tenancy.Tenant, string, time.Duration) (*url.URL, error) {
	return nil, ErrPresignUnsupported
}

func (g *GoogleDrive) PresignPut(context.Context, tenancy.Tenant, string, time.Duration) (*url.URL, error) {
	return nil, ErrPresignUnsupported
}

// Probar comprueba que el token sirve: quién es el usuario.
func (g *GoogleDrive) Probar(ctx context.Context) error {
	res, err := conProveedor(ctx, g.http, g.tok, http.MethodGet, gdriveAPI+"/about?fields=user", http.NoBody, 0, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errorDe(res, "google drive")
	}
	return nil
}

var _ BlobStore = (*GoogleDrive)(nil)
