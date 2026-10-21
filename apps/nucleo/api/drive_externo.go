// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// # Almacenamiento de terceros (ADR 0038)
//
// La organización —o la persona— conecta su propio almacén y, desde entonces, los
// ficheros nuevos van allí y no gastan su espacio en Sytue: un bucket S3, un WebDAV, una
// cuenta de Google Drive, OneDrive o Dropbox autorizada por consentimiento, o su NAS por el túnel.
// Solo lo administra owner/admin: al resto el servicio le contesta 403.
//
// La clave secreta se manda una vez y no vuelve: lo que se lee enseña bucket y endpoint.

// AlmacenExterno es lo conectado, o `Connected: false`. Sin `Available`, la instalación
// no tiene con qué cifrar las claves y la función está apagada.
type AlmacenExterno struct {
	Connected  bool   `json:"connected"`
	Available  bool   `json:"available"`
	Kind       string `json:"kind"`
	InstanceID string `json:"instance_id"`
	BasePath   string `json:"base_path"`
	Endpoint   string `json:"endpoint"`
	Bucket     string `json:"bucket"`
	Region     string `json:"region"`
	Label      string `json:"label"`
	AccessKey  string `json:"access_key"`
}

// ConfigAlmacen es lo que se manda para guardar una conexión.
type ConfigAlmacen struct {
	Kind       string `json:"kind"`
	InstanceID string `json:"instance_id"`
	Endpoint   string `json:"endpoint"`
	BasePath   string `json:"base_path"`
	Region     string `json:"region"`
	Bucket     string `json:"bucket"`
	AccessKey  string `json:"access_key"`
	SecretKey  string `json:"secret_key"`
	Label      string `json:"label"`
	// AcceptTerms es el aviso de responsabilidad, que se acepta en CADA conexión.
	AcceptTerms bool `json:"accept_terms"`
}

// InstanciaExterna es una cuenta de Google Drive, OneDrive o Dropbox por consentimiento. Sin `Connected`, falta que alguien la autorice.
type InstanciaExterna struct {
	ID             string `json:"id"`
	TemplateKey    string `json:"template_key"`
	Name           string `json:"name"`
	Connected      bool   `json:"connected"`
	OAuthAvailable bool   `json:"oauth_available"`
}

type CuentasAlmacenExterno struct {
	Items     []InstanciaExterna `json:"items"`
	Providers map[string]bool    `json:"providers"`
}

// ExternalStorageAccounts includes server readiness; absent legacy fields fail closed.
func (d *Drive) ExternalStorageAccounts(ctx context.Context) (CuentasAlmacenExterno, error) {
	return JSON[CuentasAlmacenExterno](ctx, d.c, Request{Path: "/storage/external/instances"})
}

// EstadoNAS es el túnel del NAS: el comando que el cliente ejecuta en su NAS y si el
// agente ya está conectado y el WebDAV responde a través de él.
type EstadoNAS struct {
	Endpoint  string `json:"endpoint"`
	Command   string `json:"command"`
	Online    bool   `json:"online"`
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
}

// ExternalStorage lee lo conectado.
func (d *Drive) ExternalStorage(ctx context.Context) (AlmacenExterno, error) {
	return JSON[AlmacenExterno](ctx, d.c, Request{Path: "/storage/external"})
}

// SaveExternalStorage prueba y guarda la conexión: desde ese momento lo nuevo va allí.
// No hay «probar sin guardar» en el cliente: si no responde, el error trae el motivo.
func (d *Drive) SaveExternalStorage(ctx context.Context, c ConfigAlmacen) error {
	return d.mandar(ctx, http.MethodPut, "/storage/external", c)
}

// DeleteExternalStorage la desconecta. 409 es que todavía hay ficheros allí.
func (d *Drive) DeleteExternalStorage(ctx context.Context) error {
	return d.mandar(ctx, http.MethodDelete, "/storage/external", nil)
}

// ConnectNAS crea el túnel del NAS y guarda el almacén, apagado hasta que el túnel
// responda. Devuelve el comando que hay que ejecutar en el NAS.
func (d *Drive) ConnectNAS(ctx context.Context, c ConfigAlmacen) (EstadoNAS, error) {
	return PostJSON[EstadoNAS](ctx, d.c, "/storage/external/nas", map[string]any{
		"local_endpoint": c.Endpoint, "user": c.AccessKey, "password": c.SecretKey,
		"base_path": c.BasePath, "label": c.Label, "accept_terms": c.AcceptTerms,
	})
}

// NASStatus dice si el agente del NAS ya está conectado.
func (d *Drive) NASStatus(ctx context.Context) (EstadoNAS, error) {
	return JSON[EstadoNAS](ctx, d.c, Request{Path: "/storage/external/nas/status"})
}

// MotivoDe es el mensaje que da el servicio en un error, o nada. Es lo que la web pone
// detrás de «no se ha podido conectar:».
func MotivoDe(err error) string {
	var e *Error
	if !errors.As(err, &e) {
		return ""
	}
	if m := e.Message(); m != "" {
		return m
	}
	var suelto struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(e.Body), &suelto) == nil {
		return suelto.Error
	}
	return ""
}
