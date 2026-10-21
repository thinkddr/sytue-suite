// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

// TipoAdjunto es el tipo de campo que guarda ficheros. Sus valores no van en
// `data`: son filas de base_attachments, como los enlaces lo son de base_links.
const TipoAdjunto Tipo = "attachment"

// Límites de los adjuntos.
const (
	MaxBytesAdjunto         = 20 << 20 // por fichero; se lee entero en memoria (ver http/adjuntos.go)
	MaxAdjuntosPorCampo     = 20       // por registro y campo
	MaxBytesAdjuntosPorBase = 2 << 30  // cuota de una base
	MaxLongNombreAdjunto    = 255
)

// Adjunto es la ficha de un fichero adjunto. Nunca lleva la clave del almacén.
type Adjunto struct {
	ID        string    `json:"id"`
	RecordID  string    `json:"record_id"`
	FieldID   string    `json:"field_id"`
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	HasThumb  bool      `json:"has_thumb"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// NombreDeAdjunto limpia el nombre con el que se guarda un fichero: sin ruta, sin
// caracteres de control y de longitud razonable. Es solo una etiqueta —la clave del
// almacén no sale de aquí—, pero acaba en un Content-Disposition y en un CSV.
func NombreDeAdjunto(s string) (string, error) {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		s = s[i+1:]
	}
	if s == "" || s == "." || s == ".." || !utf8.ValidString(s) || tieneControl(s) || utf8.RuneCountInString(s) > MaxLongNombreAdjunto {
		return "", fmt.Errorf("%w: nombre de fichero", ErrInvalido)
	}
	return s, nil
}

// extensionesVetadas: lo que un navegador o un sistema ejecutaría al abrirlo.
var extensionesVetadas = map[string]bool{
	".html": true, ".htm": true, ".xhtml": true, ".svg": true, ".js": true, ".mjs": true,
	".exe": true, ".dll": true, ".com": true, ".scr": true, ".msi": true, ".bat": true, ".cmd": true,
	".vbs": true, ".ps1": true, ".sh": true, ".jar": true, ".apk": true, ".lnk": true,
}

// tipoPorExtension: cuando el nombre promete uno de estos tipos, el contenido ha de serlo.
var tipoPorExtension = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif",
	".webp": "image/webp", ".pdf": "application/pdf", ".zip": "application/zip",
}

// TipoDeContenido decide el tipo de un adjunto MIRANDO SUS BYTES (nunca la cabecera
// Content-Type de quien sube) y rechaza lo que no se guarda: contenido activo (HTML,
// SVG, scripts), ejecutables y ficheros cuyo contenido no es lo que dice su nombre.
func TipoDeContenido(nombre string, datos []byte) (string, error) {
	malo := func(motivo string) (string, error) {
		return "", fmt.Errorf("%w: adjunto: %s", ErrInvalido, motivo)
	}
	if len(datos) == 0 {
		return malo("vacío")
	}
	tipo, _, _ := strings.Cut(http.DetectContentType(datos), ";")
	cabeza := bytes.ToLower(datos[:min(len(datos), 1024)])
	ext := strings.ToLower(path.Ext(nombre))
	switch {
	case extensionesVetadas[ext]:
		return malo("tipo de fichero no admitido")
	case tipo == "text/html", bytes.Contains(cabeza, []byte("<svg")), bytes.Contains(cabeza, []byte("<script")):
		return malo("contenido activo")
	case bytes.HasPrefix(datos, []byte("MZ")), bytes.HasPrefix(datos, []byte("\x7fELF")), bytes.HasPrefix(datos, []byte("#!")):
		return malo("ejecutable")
	}
	if esperado, ok := tipoPorExtension[ext]; ok && esperado != tipo {
		return malo("el contenido no es lo que dice el nombre")
	}
	return tipo, nil
}

// TipoAlServir es el Content-Type con el que se entrega un adjunto: el suyo si es una
// imagen o un PDF, y bytes sin más en cualquier otro caso. Se entrega SIEMPRE como
// descarga (attachment) y con nosniff; esto solo decide qué icono le pone el sistema.
func TipoAlServir(mime string) string {
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "application/pdf":
		return mime
	}
	return "application/octet-stream"
}
