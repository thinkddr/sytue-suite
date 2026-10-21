// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package portabilidad es el formato con el que cada producto entrega los datos
// de una persona para que se los lleve (RGPD art. 20): lo escribe el producto y
// lo lee Sytue Hub, que junta las entregas de todos en un único ZIP.
//
// Una entrega es una respuesta multipart/mixed con un fichero por parte. Hay dos
// clases de fichero:
//
//   - los que ya tienen formato estándar y van TAL CUAL: el .eml de un correo, el
//     .ics de un calendario, el .vcf de una agenda, los bytes de un archivo;
//   - las TABLAS (la lista de mensajes, de eventos, de archivos), que se escriben
//     en el formato que eligió la persona: JSON, CSV o XML.
//
// Por qué multipart y no tar ni ZIP: una tabla se escribe mientras se recorre la
// base, así que su tamaño no se sabe hasta el final, y tar lo exige en la
// cabecera; un ZIP no se puede leer de corrido sin su índice, que va al final. Una
// parte multipart se escribe y se lee en streaming con la librería estándar.
package portabilidad

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Formato es cómo se escriben las tablas.
type Formato string

// Formatos de tabla: los tres que abre cualquier herramienta sin instalar nada.
const (
	JSON Formato = "json"
	CSV  Formato = "csv"
	XML  Formato = "xml"
)

// ErrFormato es un formato que no es ninguno de los tres.
var ErrFormato = errors.New("portabilidad: el formato debe ser json, csv o xml")

// FormatoDe lee el formato pedido. Vacío es JSON.
func FormatoDe(s string) (Formato, error) {
	switch f := Formato(strings.ToLower(strings.TrimSpace(s))); f {
	case "":
		return JSON, nil
	case JSON, CSV, XML:
		return f, nil
	default:
		return "", ErrFormato
	}
}

// Cabeceras de cada parte. La ruta va escapada por segmentos: una cabecera solo
// admite ASCII y un nombre de archivo puede llevar cualquier cosa.
const (
	cabeceraRuta  = "X-Sytue-Path"
	cabeceraError = "X-Sytue-Error"
)

// Paquete es la entrega de UN producto.
type Paquete struct {
	mw      *multipart.Writer
	rc      *http.ResponseController
	formato Formato
}

// Nuevo empieza la entrega: lee ?format=, escribe las cabeceras y las manda YA.
//
// Mandarlas enseguida es parte del contrato: Hub abre a la vez la entrega de todos
// los productos mientras el token de la persona aún vale, y a partir de ahí las
// va leyendo de una en una. Por lo mismo se quita el plazo de escritura del
// servidor: la entrega dura lo que tarde en descargarse todo lo anterior.
func Nuevo(w http.ResponseWriter, r *http.Request) (*Paquete, error) {
	f, err := FormatoDe(r.URL.Query().Get("format"))
	if err != nil {
		return nil, err
	}
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	mw := multipart.NewWriter(w)
	w.Header().Set("Content-Type", mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": mw.Boundary()}))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = rc.Flush()
	return &Paquete{mw: mw, rc: rc, formato: f}, nil
}

// Formato es el formato de tabla que pidió la persona, para el producto que tenga
// una versión propia mejor que la genérica (el CSV de Contacts es el de Google).
func (p *Paquete) Formato() Formato { return p.formato }

// Fichero añade un fichero tal cual. ruta es relativa a la carpeta del producto.
func (p *Paquete) Fichero(ruta string, modificado time.Time, contenido io.Reader) error {
	return p.FicheroDeTamano(ruta, modificado, -1, contenido)
}

// FicheroDeTamano es Fichero cuando se sabe cuánto mide: quien lo lea para subirlo
// a otro servicio (Microsoft Graph exige Content-Length) no tiene que leerlo entero
// antes. -1 es «no se sabe».
func (p *Paquete) FicheroDeTamano(ruta string, modificado time.Time, tamano int64, contenido io.Reader) error {
	w, err := p.parte(ruta, modificado, tamano)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, contenido); err != nil {
		return fmt.Errorf("portabilidad: escribir %q: %w", ruta, err)
	}
	return nil
}

// Tabla añade <nombre>.<formato> con una fila por elemento. fila es el nombre de
// cada elemento en XML («contact» dentro de «contacts»).
func Tabla[T any](p *Paquete, nombre, fila string, filas iter.Seq2[T, error]) error {
	w, err := p.parte(nombre+"."+string(p.formato), time.Time{}, -1)
	if err != nil {
		return err
	}
	return EscribirTabla(w, p.formato, nombre, fila, filas)
}

// Fallo deja constancia de que la entrega se ha quedado a medias y la cierra. Las
// cabeceras ya salieron con un 200, así que es la única forma de que Hub lo sepa
// y lo diga en el resumen en vez de entregar un producto incompleto como bueno.
func (p *Paquete) Fallo(err error) error {
	h := textproto.MIMEHeader{}
	h.Set(cabeceraError, strings.ToValidUTF8(strings.ReplaceAll(err.Error(), "\n", " "), "?"))
	if _, e := p.mw.CreatePart(h); e != nil {
		return e
	}
	return p.mw.Close()
}

// Cerrar termina la entrega. Sin el cierre, Hub la da por cortada.
func (p *Paquete) Cerrar() error { return p.mw.Close() }

func (p *Paquete) parte(ruta string, modificado time.Time, tamano int64) (io.Writer, error) {
	h := textproto.MIMEHeader{}
	h.Set(cabeceraRuta, escaparRuta(ruta))
	if tamano >= 0 {
		h.Set("Content-Length", strconv.FormatInt(tamano, 10))
	}
	if !modificado.IsZero() {
		h.Set("Last-Modified", modificado.UTC().Format(http.TimeFormat))
	}
	w, err := p.mw.CreatePart(h)
	if err != nil {
		return nil, err
	}
	// Cada fichero sale en cuanto se escribe: sin esto, el búfer del servidor
	// retiene las partes pequeñas y Hub espera sin motivo.
	_ = p.rc.Flush()
	return w, nil
}

// Todas recorre un slice como filas de una tabla.
func Todas[T any](filas []T) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for _, f := range filas {
			if !yield(f, nil) {
				return
			}
		}
	}
}

// Parte es un fichero de la entrega de un producto, tal como la lee Hub.
type Parte struct {
	Ruta       string
	Modificado time.Time
	// Tamano es lo que mide, si el producto lo dijo; -1 si no.
	Tamano int64
	io.Reader
}

// ErrProducto es un fallo que el producto declaró con Fallo.
type ErrProducto struct{ Motivo string }

func (e *ErrProducto) Error() string { return e.Motivo }

// Leer recorre la entrega de un producto. Termina con error si el producto la
// declaró fallida (ErrProducto) o si se cortó antes de cerrarla.
func Leer(resp *http.Response) iter.Seq2[Parte, error] {
	return func(yield func(Parte, error) bool) {
		_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if err != nil || params["boundary"] == "" {
			yield(Parte{}, fmt.Errorf("portabilidad: la entrega no es multipart: %q", resp.Header.Get("Content-Type")))
			return
		}
		mr := multipart.NewReader(resp.Body, params["boundary"])
		for {
			part, err := mr.NextRawPart()
			// io.EOF A SECAS es el cierre. Envuelto («multipart: NextPart: EOF») es
			// que el cuerpo acabó sin él: el producto se cayó a mitad de la entrega.
			if err == io.EOF { //nolint:errorlint // distinguir el EOF envuelto es justo lo que se busca.
				return
			}
			if err != nil {
				yield(Parte{}, fmt.Errorf("portabilidad: la entrega se ha cortado: %w", err))
				return
			}
			if motivo := part.Header.Get(cabeceraError); motivo != "" {
				yield(Parte{}, &ErrProducto{Motivo: motivo})
				return
			}
			ruta, err := desescaparRuta(part.Header.Get(cabeceraRuta))
			if err != nil {
				yield(Parte{}, err)
				return
			}
			mod, _ := http.ParseTime(part.Header.Get("Last-Modified"))
			tamano, err := strconv.ParseInt(part.Header.Get("Content-Length"), 10, 64)
			if err != nil {
				tamano = -1
			}
			if !yield(Parte{Ruta: ruta, Modificado: mod, Tamano: tamano, Reader: part}, nil) {
				return
			}
		}
	}
}

func escaparRuta(ruta string) string {
	segs := strings.Split(ruta, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

func desescaparRuta(v string) (string, error) {
	if v == "" {
		return "", errors.New("portabilidad: parte sin ruta")
	}
	segs := strings.Split(v, "/")
	for i, s := range segs {
		d, err := url.PathUnescape(s)
		if err != nil {
			return "", fmt.Errorf("portabilidad: ruta ilegible %q: %w", v, err)
		}
		segs[i] = d
	}
	return strings.Join(segs, "/"), nil
}
