// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

// # Nubes personales
//
// El Google Drive, OneDrive o Dropbox de la propia persona, conectado con solo su
// consentimiento (`handlers_personal_storage.go` de integraciones). La credencial
// del proveedor se queda en la pasarela: aquí solo viajan los nombres y los bytes.

// NubeMaxBytes es el tope por fichero de la pasarela: más se rechaza antes de mandarlo.
const NubeMaxBytes = 100 << 20

// NubesProveedores son los proveedores, en el orden de la web, con su nombre de marca.
var NubesProveedores = []string{"google-drive", "onedrive", "dropbox"}

// NombreNube es la marca del proveedor, sin traducir.
var NombreNube = map[string]string{"google-drive": "Google Drive", "onedrive": "OneDrive", "dropbox": "Dropbox"}

// hostNube es el host exacto de la pantalla de consentimiento de cada proveedor.
var hostNube = map[string]string{
	"google-drive": "accounts.google.com",
	"onedrive":     "login.microsoftonline.com",
	"dropbox":      "www.dropbox.com",
}

type NubePersonal struct {
	InstanceID string `json:"instance_id"`
	Provider   string `json:"provider"`
	Name       string `json:"name"`
	Connected  bool   `json:"connected"`
}

type ProveedorNube struct {
	Provider      string `json:"provider"`
	Available     bool   `json:"available"`
	DisabledByOrg bool   `json:"disabled_by_org"`
}

type NubesPersonales struct {
	Items     []NubePersonal  `json:"items"`
	Providers []ProveedorNube `json:"providers"`
}

// Conectadas son las nubes listas para usar.
func (n NubesPersonales) Conectadas() []NubePersonal {
	var out []NubePersonal
	for _, c := range n.Items {
		if c.Connected {
			out = append(out, c)
		}
	}
	return out
}

// Libres son los proveedores que se pueden conectar y no lo están: un «Conectar» cada uno.
func (n NubesPersonales) Libres() []string {
	var out []string
	for _, p := range n.Providers {
		if !p.Available {
			continue
		}
		conectada := false
		for _, c := range n.Items {
			conectada = conectada || (c.Connected && c.Provider == p.Provider)
		}
		if !conectada {
			out = append(out, p.Provider)
		}
	}
	return out
}

type ElementoNube struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Folder   bool   `json:"folder"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
	MimeType string `json:"mime_type"`
}

type PaginaNube struct {
	Items      []ElementoNube `json:"items"`
	NextCursor string         `json:"next_cursor"`
}

func (i *Integraciones) NubesPersonales(ctx context.Context) (NubesPersonales, error) {
	return JSON[NubesPersonales](ctx, i.c, Request{Path: "/personal-storage/accounts"})
}

// NubeConectar pide el consentimiento con vuelta a la app nativa de Drive.
func (i *Integraciones) NubeConectar(ctx context.Context, provider, instance string) (OAuthAuthorization, error) {
	host := hostNube[provider]
	if host == "" {
		return OAuthAuthorization{}, ErrOAuthState
	}
	result, err := PostJSON[struct {
		InstanceID   string `json:"instance_id"`
		AuthorizeURL string `json:"authorize_url"`
	}](ctx, i.c, "/personal-storage/connect", struct {
		Provider string `json:"provider"`
		Instance string `json:"instance_id,omitempty"`
		ReturnTo string `json:"return_to"`
	}{provider, instance, "native-drive"})
	if err != nil {
		return OAuthAuthorization{}, err
	}
	return autorizacionPersonal(result.AuthorizeURL, result.InstanceID, host, "native-personal-storage")
}

func (i *Integraciones) NubeDesconectar(ctx context.Context, instance string) error {
	res, err := i.c.Do(ctx, Request{Method: http.MethodDelete, Path: ruta("/personal-storage/", instance, "")})
	if err != nil {
		return err
	}
	return res.Body.Close()
}

// NubeElementos es una página de la carpeta; carpeta vacía es la raíz de la nube.
func (i *Integraciones) NubeElementos(ctx context.Context, instance, carpeta, cursor string) (PaginaNube, error) {
	q := url.Values{}
	if carpeta != "" {
		q.Set("folder", carpeta)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	return JSON[PaginaNube](ctx, i.c, Request{Path: ruta("/personal-storage/", instance, "/items"), Query: q})
}

// NubeDescargar abre el contenido de un fichero; cerrarlo es cosa de quien llama.
func (i *Integraciones) NubeDescargar(ctx context.Context, instance, item string) (io.ReadCloser, error) {
	res, err := i.c.Do(ctx, Request{Path: ruta("/personal-storage/", instance, "/content"), Query: url.Values{"item": {item}}})
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

// NubeSubir manda un fichero a la carpeta, sin pasar por la memoria entera.
func (i *Integraciones) NubeSubir(ctx context.Context, instance, carpeta, nombre string, cuerpo io.Reader, tamano int64) (ElementoNube, error) {
	q := url.Values{"name": {nombre}}
	if carpeta != "" {
		q.Set("folder", carpeta)
	}
	res, err := i.c.Stream(ctx, http.MethodPost, ruta("/personal-storage/", instance, "/content"), q,
		http.Header{"Content-Type": {"application/octet-stream"}}, cuerpo, tamano)
	if err != nil {
		return ElementoNube{}, err
	}
	defer func() { _ = res.Body.Close() }()
	var out ElementoNube
	err = json.NewDecoder(res.Body).Decode(&out)
	return out, err
}
