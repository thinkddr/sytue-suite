// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// # Traer los datos del programa anterior (`ImportarDesde.tsx`)
//
// Holded, Odoo, Salesforce o Business Central, por las rutas de administración del
// servicio (`services/syerp/internal/http/handlers_conectores.go`): las conexiones
// guardadas, probar una instancia de la pasarela antes de guardarla y la importación de
// los objetos elegidos. Las claves del tercero NUNCA pasan por aquí: viven selladas en el
// Integration Gateway, y el ERP solo guarda qué instancia de la pasarela usar.

// ERPConexion es una conexión guardada, sin secretos.
type ERPConexion struct {
	Proveedor string `json:"proveedor"`
	Instancia string `json:"instancia,omitempty"`
	Activo    bool   `json:"activo"`
}

// ERPImportado es lo que trajo la importación de UN objeto del proveedor.
type ERPImportado struct {
	Objeto       string   `json:"objeto"`
	Modelo       string   `json:"modelo"`
	Creados      int      `json:"creados"`
	Actualizados int      `json:"actualizados"`
	Fallos       []string `json:"fallos,omitempty"`
}

// Conexiones son las conexiones guardadas de la organización (`GET /conexiones`).
func (e *ERP) Conexiones(ctx context.Context) ([]ERPConexion, error) {
	r, err := JSON[struct {
		Conexiones []ERPConexion `json:"conexiones"`
	}](ctx, e.c, e.pedir("/conexiones", nil))
	return r.Conexiones, err
}

// ProbarConexion pide al proveedor UNA fila de su primer objeto por esa instancia de
// la pasarela, sin escribir nada (`POST /conexiones/{proveedor}/probar`). Una instancia
// que no conecta contesta 422 con el motivo de la pasarela.
func (e *ERP) ProbarConexion(ctx context.Context, proveedor, instancia string) error {
	cuerpo, err := json.Marshal(map[string]string{"instancia": strings.TrimSpace(instancia)})
	if err != nil {
		return err
	}
	r := e.pedir("/conexiones/"+url.PathEscape(proveedor)+"/probar", nil)
	r.Method, r.Body = http.MethodPost, cuerpo
	r.Header.Set("Content-Type", "application/json")
	_, err = JSON[json.RawMessage](ctx, e.c, r)
	return err
}

// GuardarConexion deja esa instancia como la conexión activa del proveedor
// (`POST /conexiones/{proveedor}`).
func (e *ERP) GuardarConexion(ctx context.Context, proveedor, instancia string) error {
	cuerpo, err := json.Marshal(map[string]any{"instancia": strings.TrimSpace(instancia), "activo": true})
	if err != nil {
		return err
	}
	return e.sinCuerpo(ctx, http.MethodPost, "/conexiones/"+url.PathEscape(proveedor), cuerpo)
}

// ImportarDesde trae del proveedor los objetos elegidos, con las claves de los mapeos
// del servicio (`POST /importar/{proveedor}`). Es idempotente: cada fila se reconoce
// por su id de origen, así que importar dos veces actualiza en vez de duplicar.
func (e *ERP) ImportarDesde(ctx context.Context, proveedor string, objetos []string) ([]ERPImportado, error) {
	cuerpo, err := json.Marshal(map[string][]string{"objetos": objetos})
	if err != nil {
		return nil, err
	}
	r := e.pedir("/importar/"+url.PathEscape(proveedor), nil)
	r.Method, r.Body = http.MethodPost, cuerpo
	r.Header.Set("Content-Type", "application/json")
	res, err := JSON[struct {
		Importado []ERPImportado `json:"importado"`
	}](ctx, e.c, r)
	return res.Importado, err
}
