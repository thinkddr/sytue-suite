// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
)

// Integraciones es el cliente de la pasarela de integraciones (`services/integrations`):
// lo que el lanzador del Hub le pregunta —qué aplicaciones de terceros ha conectado la
// organización y a cuáles ha dado un producto de la suite—.
type Integraciones struct{ c *Client }

// NewIntegraciones crea el cliente sobre la raíz del servicio (ServiceURL(apex,
// "integrations")).
func NewIntegraciones(servicio string, s Session, h *http.Client) *Integraciones {
	return &Integraciones{c: New(servicio+"/api/v1", s, h)}
}

// AppConectada es una tesela de aplicación de terceros, con la MISMA forma que un
// producto del lanzador (`myApp` en `handlers_myapps.go`). `IconURL` es un `data:` con
// el SVG del manifiesto, no una dirección de fuera.
type AppConectada struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Href        string `json:"href"`
	//nolint:tagliatelle // así lo escribe el servicio (camelCase, la forma de SuiteApp del lanzador).
	IconURL string `json:"iconUrl"`
	Color   string `json:"color"`
	Status  string `json:"status"`
}

// MisApps son las conectadas y ACTIVAS de la organización (`GET /my-apps`).
func (i *Integraciones) MisApps(ctx context.Context) ([]AppConectada, error) {
	r, err := JSON[struct {
		Apps []AppConectada `json:"apps"`
	}](ctx, i.c, Request{Path: "/my-apps"})
	return r.Apps, err
}

// Sustituto es adónde abrir un producto que la organización ha dado a una aplicación de
// terceros: «Hojas de cálculo» con Google, por ejemplo.
type Sustituto struct {
	Name string `json:"name"`
	Href string `json:"href"`
}

// Sustituciones es el mapa producto → sustituto (`GET /substitutions/resolved`). Solo
// trae lo sustituido: lo que no está lo sirve Sytue. Se descarta lo que venga sin
// dirección, como hace el BFF de la web.
func (i *Integraciones) Sustituciones(ctx context.Context) (map[string]Sustituto, error) {
	r, err := JSON[struct {
		Products map[string]Sustituto `json:"products"`
	}](ctx, i.c, Request{Path: "/substitutions/resolved"})
	if err != nil {
		return nil, err
	}
	out := make(map[string]Sustituto, len(r.Products))
	for producto, s := range r.Products {
		if s.Href == "" {
			continue
		}
		if s.Name == "" {
			s.Name = producto
		}
		out[producto] = s
	}
	return out, nil
}

// # «Traer mis datos»
//
// El puerto de `packages/api-clients/src/core/importData.ts`: la puerta de cada producto
// al motor de migración de la pasarela, que antes solo tenía la consola de
// administración. La pasarela ignora cualquier identidad que se le mande —origen y
// destino son siempre quien llama—, así que por aquí no viaja ninguna.

// OrigenImportacion es una conexión que la organización ya autorizó y de la que se
// pueden traer datos personales.
type OrigenImportacion struct {
	InstanceID string
	// Name es el que le puso la organización («el Google de ventas»).
	Name string
	// Source es `google` o `m365`.
	Source string
}

// TrabajoImportacion es una importación en curso o terminada.
type TrabajoImportacion struct {
	ID        string `json:"id"`
	Domain    string `json:"domain"`
	Source    string `json:"source"`
	Status    string `json:"status"` // queued, running, done, error o canceled
	LastError string `json:"last_error"`
	Counts    struct {
		Migrated int `json:"migrated"`
		Skipped  int `json:"skipped"`
		Errors   int `json:"errors"`
	} `json:"counts"`
}

// EnCurso dice si sigue trabajando: mientras lo haga se vuelve a preguntar.
func (m TrabajoImportacion) EnCurso() bool { return m.Status == "queued" || m.Status == "running" }

// origenDe reconoce la clave de la plantilla, tolerando sufijos («google-workspace-eu»).
// Se mira la CLAVE y no el nombre: el nombre lo pone quien conecta y no dice de qué
// producto se trata. Cualquier otra conexión —Jira, Salesforce…— no tiene contenido
// personal que traer y no aparece.
func origenDe(plantilla string) string {
	k := strings.ToLower(strings.TrimSpace(plantilla))
	switch {
	case strings.HasPrefix(k, "google"), k == "gws":
		return "google"
	case strings.HasPrefix(k, "m365"), strings.HasPrefix(k, "microsoft"):
		return "m365"
	}
	return ""
}

// OrigenesImportacion son las conexiones ACTIVAS de la organización con Google o
// Microsoft. Quien llama trata un error como «no hay»: en una organización que no ha
// conectado ninguno, enseñar un fallo solo asustaría.
func (i *Integraciones) OrigenesImportacion(ctx context.Context) ([]OrigenImportacion, error) {
	r, err := JSON[struct {
		Instances []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			TemplateKey string `json:"template_key"`
			Status      string `json:"status"`
		} `json:"instances"`
	}](ctx, i.c, Request{Path: "/instances"})
	if err != nil {
		return nil, err
	}
	var out []OrigenImportacion
	for _, x := range r.Instances {
		if s := origenDe(x.TemplateKey); x.Status == "active" && s != "" {
			out = append(out, OrigenImportacion{InstanceID: x.ID, Name: x.Name, Source: s})
		}
	}
	return out, nil
}

// MisImportaciones son las de quien llama con el contenido de `dominios`. Se filtra aquí
// y no en la pasarela porque el mismo trabajo le importa a dos sitios: una migración de
// contactos la enseñan Contacts y Mail, cada uno con sus palabras.
func (i *Integraciones) MisImportaciones(ctx context.Context, dominios []string) ([]TrabajoImportacion, error) {
	r, err := JSON[struct {
		Migrations []TrabajoImportacion `json:"migrations"`
	}](ctx, i.c, Request{Path: "/migrations/self"})
	if err != nil {
		return nil, err
	}
	var out []TrabajoImportacion
	for _, m := range r.Migrations {
		if slices.Contains(dominios, m.Domain) {
			out = append(out, m)
		}
	}
	return out, nil
}

// Importar lanza la importación de `dominios` desde un origen y devuelve los trabajos
// que se han puesto en cola.
func (i *Integraciones) Importar(ctx context.Context, o OrigenImportacion, dominios []string) ([]TrabajoImportacion, error) {
	r, err := PostJSON[struct {
		Migrations []TrabajoImportacion `json:"migrations"`
	}](ctx, i.c, "/migrations/self", map[string]any{
		"instance_id": o.InstanceID, "source": o.Source, "domains": dominios,
	})
	return r.Migrations, err
}
