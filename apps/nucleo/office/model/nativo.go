// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

// EL FORMATO NATIVO de Docus, Calcus y Diapos en un fichero del disco (`.docus`, `.calcus`,
// `.diapos`; PLAN-EDICION-LOCAL O-05). Es un ZIP, como ODF y OOXML, con cuatro entradas:
//
//	mimetype          el tipo, la PRIMERA y sin comprimir (lo reconoce quien mira los
//	                  primeros bytes sin descomprimir, como en ODF)
//	meta.json         tipo, título, fechas y versión del formato
//	contenido         el valor del registro de contenido TAL CUAL lo guarda el editor:
//	                  el HTML saneado de un documento o el JSON del modelo de la hoja o de
//	                  la presentación
//	comentarios.json  los comentarios, con su ancla, autor, fechas y respuestas
//
// No es una conversión: es el modelo. Por eso escribirlo y volverlo a leer devuelve el
// mismo contenido byte a byte (TestNativoIdaYVueltaSinPerdida), y guardar en él nunca
// pierde nada, al contrario que guardar en `.docx`.

// FormatoNativo es la versión del formato que escribe este código. Un fichero con una
// mayor lo ha escrito una versión más nueva de la app: se rechaza en vez de leerlo a
// medias.
const FormatoNativo = 1

// MaxNativo es lo más que se descomprime de una entrada: la misma holgura que el tope de
// importación, multiplicada por lo que comprime el texto.
const MaxNativo = 8 * MaxImportBytes

// ErrNativo: el fichero no es un fichero nativo bien formado.
var ErrNativo = errors.New("docs: no es un fichero nativo de Sytue Office")

// ErrNativoFuturo: el fichero lo ha escrito una versión más nueva del formato.
var ErrNativoFuturo = errors.New("docs: el fichero es de una versión más nueva de Sytue Office")

// Nativo es lo que lleva un fichero nativo.
type Nativo struct {
	Kind   Kind
	Titulo string
	// Contenido es el valor del registro de contenido del tipo ([ContentKey]).
	Contenido   string
	Comentarios []Comment
	Creado      time.Time
	Modificado  time.Time
}

// metaNativo es `meta.json`.
type metaNativo struct {
	Formato    int       `json:"formato"`
	Tipo       Kind      `json:"tipo"`
	Titulo     string    `json:"titulo"`
	Creado     time.Time `json:"creado,omitzero"`
	Modificado time.Time `json:"modificado,omitzero"`
	Generador  string    `json:"generador,omitempty"`
}

// Entradas del ZIP.
const (
	entradaMime        = "mimetype"
	entradaMeta        = "meta.json"
	entradaContenido   = "contenido"
	entradaComentarios = "comentarios.json"
)

// MimeNativo es el tipo MIME del fichero nativo de ese tipo de documento.
func MimeNativo(k Kind) string {
	return "application/vnd.sytue." + strings.TrimPrefix(ExtensionNativa(k), ".")
}

// ExtensionNativa es la extensión (con punto) del fichero nativo de ese tipo.
func ExtensionNativa(k Kind) string {
	switch k {
	case KindSpreadsheet:
		return ".calcus"
	case KindPresentation:
		return ".diapos"
	default:
		return ".docus"
	}
}

// KindNativo es el tipo de un fichero por su extensión nativa; false si no es una.
func KindNativo(nombre string) (Kind, bool) {
	switch strings.ToLower(path.Ext(strings.ReplaceAll(nombre, "\\", "/"))) {
	case ".docus":
		return KindDocument, true
	case ".calcus":
		return KindSpreadsheet, true
	case ".diapos":
		return KindPresentation, true
	}
	return "", false
}

// EsNativo dice si los bytes empiezan como un fichero nativo: un ZIP cuya primera entrada
// es `mimetype` sin comprimir con uno de los tres tipos. No descomprime nada.
func EsNativo(datos []byte) bool {
	// Cabecera local de ZIP: firma, ..., nombre en 30, longitud del nombre en 26.
	const cab = 30
	if len(datos) < cab+len(entradaMime) || !bytes.HasPrefix(datos, []byte("PK\x03\x04")) {
		return false
	}
	nl := int(datos[26]) | int(datos[27])<<8
	el := int(datos[28]) | int(datos[29])<<8
	if nl != len(entradaMime) || string(datos[cab:cab+nl]) != entradaMime {
		return false
	}
	ini := cab + nl + el
	for _, k := range []Kind{KindDocument, KindSpreadsheet, KindPresentation} {
		if m := MimeNativo(k); len(datos) >= ini+len(m) && string(datos[ini:ini+len(m)]) == m {
			return true
		}
	}
	return false
}

// EscribirNativo serializa n. El orden de las entradas y sus fechas son fijos, así que
// el mismo documento da siempre los mismos bytes.
func EscribirNativo(n Nativo) ([]byte, error) {
	if !n.Kind.Valid() {
		return nil, ErrInvalidKind
	}
	meta, err := json.MarshalIndent(metaNativo{
		Formato: FormatoNativo, Tipo: n.Kind, Titulo: n.Titulo,
		Creado: n.Creado.UTC(), Modificado: n.Modificado.UTC(), Generador: "Sytue Office",
	}, "", " ")
	if err != nil {
		return nil, err
	}
	comentarios := n.Comentarios
	if comentarios == nil {
		comentarios = []Comment{}
	}
	// Compacto y no sangrado: el ancla es un json.RawMessage, y sangrarlo lo reescribiría.
	com, err := json.Marshal(comentarios)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fecha := n.Modificado.UTC()
	if fecha.IsZero() || fecha.Year() < 1980 {
		fecha = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	for _, e := range []struct {
		nombre string
		metodo uint16
		datos  []byte
	}{
		{entradaMime, zip.Store, []byte(MimeNativo(n.Kind))},
		{entradaMeta, zip.Deflate, meta},
		{entradaContenido, zip.Deflate, []byte(n.Contenido)},
		{entradaComentarios, zip.Deflate, com},
	} {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.nombre, Method: e.metodo, Modified: fecha})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(e.datos); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// LeerNativo lee un fichero nativo. Lo que no se entiende es ErrNativo; un formato más
// nuevo, ErrNativoFuturo.
func LeerNativo(datos []byte) (Nativo, error) {
	zr, err := zip.NewReader(bytes.NewReader(datos), int64(len(datos)))
	if err != nil {
		return Nativo{}, fmt.Errorf("%w: %w", ErrNativo, err)
	}
	entradas := map[string][]byte{}
	for _, f := range zr.File {
		switch f.Name {
		case entradaMime, entradaMeta, entradaContenido, entradaComentarios:
		default:
			continue // lo que una versión futura añada sin cambiar el formato
		}
		if _, repetida := entradas[f.Name]; repetida {
			return Nativo{}, fmt.Errorf("%w: %s repetida", ErrNativo, f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return Nativo{}, fmt.Errorf("%w: %w", ErrNativo, err)
		}
		b, err := io.ReadAll(io.LimitReader(rc, MaxNativo+1))
		_ = rc.Close()
		if err != nil {
			return Nativo{}, fmt.Errorf("%w: %w", ErrNativo, err)
		}
		if len(b) > MaxNativo {
			return Nativo{}, fmt.Errorf("%w: %s demasiado grande", ErrNativo, f.Name)
		}
		entradas[f.Name] = b
	}
	var meta metaNativo
	raw, ok := entradas[entradaMeta]
	if !ok || json.Unmarshal(raw, &meta) != nil {
		return Nativo{}, fmt.Errorf("%w: sin %s", ErrNativo, entradaMeta)
	}
	if meta.Formato > FormatoNativo {
		return Nativo{}, ErrNativoFuturo
	}
	if meta.Formato < 1 || !meta.Tipo.Valid() {
		return Nativo{}, fmt.Errorf("%w: meta.json no válido", ErrNativo)
	}
	if mime, ok := entradas[entradaMime]; ok && string(mime) != MimeNativo(meta.Tipo) {
		return Nativo{}, fmt.Errorf("%w: el tipo de mimetype y el de meta.json no coinciden", ErrNativo)
	}
	contenido, ok := entradas[entradaContenido]
	if !ok {
		return Nativo{}, fmt.Errorf("%w: sin %s", ErrNativo, entradaContenido)
	}
	n := Nativo{Kind: meta.Tipo, Titulo: meta.Titulo, Contenido: string(contenido), Creado: meta.Creado, Modificado: meta.Modificado}
	if raw, ok := entradas[entradaComentarios]; ok && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &n.Comentarios); err != nil {
			return Nativo{}, fmt.Errorf("%w: comentarios.json: %w", ErrNativo, err)
		}
	}
	return n, nil
}
