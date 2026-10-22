// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package compat traduce el modelo de ficheros de Sytue Drive (internal/domain)
// a los dialectos de la API de Google Drive v3 y de Microsoft Graph (files), y
// de vuelta en lo único que llega del cliente: el parámetro `q` de Google.
//
// Todo lo de aquí es PURO: entran estructuras de dominio o cadenas y salen
// estructuras serializables — sin red, sin base de datos, sin contexto. Los
// handlers finos que lo usan viven en internal/http/handlers_compat.go.
//
// PREFIJOS DE MONTAJE (no colisionan con la API nativa /api/v1):
//
//	/compat/google → Google Drive v3       (GET /drive/v3/about, /drive/v3/files, …)
//	/compat/graph  → Microsoft Graph v1.0  (GET /v1.0/me/drive, /v1.0/me/drive/items/{id}, …)
//
// Es lo que el tercero pega como «base URL» de su integración: la emulación sirve
// justo donde el cliente deja configurarla (ver docs/architecture.md §3).
//
// AUTORIZACIÓN: no vive aquí. Los handlers llaman al MISMO servicio que la API
// nativa con el contexto de la petición (Bearer de Sytue ID → tenant + RLS), así
// que la verja es idéntica. Un traductor nunca decide qué se ve.
package compat

import "time"

// FolderMIME es el mimeType con el que Google Drive marca una carpeta. Las
// librerías cliente lo usan para distinguir carpeta de fichero, así que es
// obligatorio emitirlo tal cual.
const FolderMIME = "application/vnd.google-apps.folder"

// RootAlias es el identificador que ambos dialectos aceptan para «la raíz de mi
// unidad» (Google: `'root' in parents`; Graph: /me/drive/root).
const RootAlias = "root"

// defaultMIME es el tipo que se emite cuando el fichero no tiene uno registrado.
const defaultMIME = "application/octet-stream"

// rfc3339 formatea un instante en UTC; ambos dialectos usan RFC 3339. El cero
// devuelve "" para que el campo se omita en vez de emitir una fecha inventada.
func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// mimeOr devuelve el tipo del fichero o el genérico si no consta.
func mimeOr(contentType string) string {
	if contentType == "" {
		return defaultMIME
	}
	return contentType
}
