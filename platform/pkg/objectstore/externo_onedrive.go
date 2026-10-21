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
	graphAPI     = "https://graph.microsoft.com/v1.0"
	onedriveRaiz = "Sytue"
	// Graph admite PUT directo hasta 4 MiB; por encima, sesión de subida por trozos.
	onedriveSimpleMax = 4 << 20
	onedriveTrozo     = 10 << 20
)

// OneDrive guarda los blobs de un tenant bajo /Sytue/<tenant>/… en el
// OneDrive de quien dio el consentimiento. Va por RUTA: no necesita mapa de ids.
type OneDrive struct {
	tok  TokenSource
	http *http.Client
}

// NewOneDrive abre el adaptador con el token renovable de una conexión.
func NewOneDrive(tok TokenSource) *OneDrive {
	return &OneDrive{tok: tok, http: clienteDeTerceros(60 * time.Second)}
}

// item es la URL del elemento por ruta: /me/drive/root:/Sytue/<prefijo><clave>.
func (o *OneDrive) item(t tenancy.Tenant, key string) string {
	partes := strings.Split(onedriveRaiz+"/"+t.StoragePrefix()+strings.TrimLeft(key, "/"), "/")
	for i, p := range partes {
		partes[i] = url.PathEscape(p)
	}
	return graphAPI + "/me/drive/root:/" + strings.Join(partes, "/")
}

func (o *OneDrive) Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts PutOptions) error {
	body, n, cerrar, err := spool(r, size)
	if err != nil {
		return err
	}
	defer cerrar()
	ct := opts.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	if n <= onedriveSimpleMax {
		res, err := conProveedor(ctx, o.http, o.tok, http.MethodPut, o.item(t, key)+":/content?@microsoft.graph.conflictBehavior=replace", body, n, map[string]string{"Content-Type": ct})
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
			return errorDe(res, "onedrive: subir")
		}
		return nil
	}
	// Sesión de subida: trozos consecutivos con Content-Range; la URL de sesión ya
	// va autenticada y no lleva el token.
	meta, _ := json.Marshal(map[string]any{"item": map[string]any{"@microsoft.graph.conflictBehavior": "replace"}})
	res, err := conProveedor(ctx, o.http, o.tok, http.MethodPost, o.item(t, key)+":/createUploadSession", bytes.NewReader(meta), int64(len(meta)), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return err
	}
	var sesion struct {
		UploadURL string `json:"uploadUrl"` //nolint:tagliatelle // así lo llama Graph
	}
	err = json.NewDecoder(res.Body).Decode(&sesion)
	_ = res.Body.Close()
	if err != nil || res.StatusCode != http.StatusOK || sesion.UploadURL == "" {
		return fmt.Errorf("onedrive: abrir sesión de subida: %s", res.Status)
	}
	for desde := int64(0); desde < n; desde += onedriveTrozo {
		hasta := desde + onedriveTrozo
		if hasta > n {
			hasta = n
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, sesion.UploadURL, io.NewSectionReader(body, desde, hasta-desde))
		if err != nil {
			return err
		}
		req.ContentLength = hasta - desde
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", desde, hasta-1, n))
		rt, err := o.http.Do(req)
		if err != nil {
			return fmt.Errorf("onedrive: subir trozo: %w", err)
		}
		_, _ = io.Copy(io.Discard, rt.Body)
		_ = rt.Body.Close()
		if rt.StatusCode != http.StatusAccepted && rt.StatusCode != http.StatusOK && rt.StatusCode != http.StatusCreated {
			return fmt.Errorf("onedrive: subir trozo: %s", rt.Status)
		}
	}
	return nil
}

func (o *OneDrive) Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	// Graph responde 302 a una URL prefirmada; el cliente la sigue y Go no
	// reenvía el Authorization a otro host.
	res, err := conProveedor(ctx, o.http, o.tok, http.MethodGet, o.item(t, key)+":/content", http.NoBody, 0, nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusNotFound {
		_ = res.Body.Close()
		return nil, ErrNotFound
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return nil, errorDe(res, "onedrive: descargar")
	}
	return res.Body, nil
}

func (o *OneDrive) Delete(ctx context.Context, t tenancy.Tenant, key string) error {
	res, err := conProveedor(ctx, o.http, o.tok, http.MethodDelete, o.item(t, key), http.NoBody, 0, nil)
	if err != nil {
		return err
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusNotFound {
		return fmt.Errorf("onedrive: borrar: %s", res.Status)
	}
	return nil
}

func (o *OneDrive) Stat(ctx context.Context, t tenancy.Tenant, key string) (int64, error) {
	res, err := conProveedor(ctx, o.http, o.tok, http.MethodGet, o.item(t, key)+"?$select=size", http.NoBody, 0, nil)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return 0, ErrNotFound
	}
	if res.StatusCode != http.StatusOK {
		return 0, errorDe(res, "onedrive: consultar")
	}
	var out struct{ Size int64 }
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.Size, nil
}

func (o *OneDrive) Existe(ctx context.Context, t tenancy.Tenant, key string) (bool, error) {
	_, err := o.Stat(ctx, t, key)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Copiar lee y escribe: la copia server-side de Graph es asíncrona y obliga a
// esperar un monitor; para lo que se copia (deduplicación) no compensa.
func (o *OneDrive) Copiar(ctx context.Context, t tenancy.Tenant, origen, destino string) error {
	rc, err := o.Get(ctx, t, origen)
	if err != nil {
		return err
	}
	defer rc.Close()
	return o.Put(ctx, t, destino, rc, -1, PutOptions{})
}

func (o *OneDrive) PresignGet(context.Context, tenancy.Tenant, string, time.Duration) (*url.URL, error) {
	return nil, ErrPresignUnsupported
}

func (o *OneDrive) PresignPut(context.Context, tenancy.Tenant, string, time.Duration) (*url.URL, error) {
	return nil, ErrPresignUnsupported
}

// Probar comprueba que el token sirve: el drive del usuario.
func (o *OneDrive) Probar(ctx context.Context) error {
	res, err := conProveedor(ctx, o.http, o.tok, http.MethodGet, graphAPI+"/me/drive?$select=id", http.NoBody, 0, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errorDe(res, "onedrive")
	}
	return nil
}

var _ BlobStore = (*OneDrive)(nil)
