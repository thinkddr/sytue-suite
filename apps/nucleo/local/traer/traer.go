// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package traer es «Traer desde Sytue» (apps/fyne/PLAN-EDICION-LOCAL.md §5.13 y §6.15,
// HU-07): coger el ZIP de «Tus datos» que descarga la edición conectada y repartirlo en
// los almacenes de Sytue Office. Es un FICHERO, no una conexión: nada de aquí abre la
// red ni habla con Sytue.
//
// El ZIP lo arma el Hub del servicio (`services/hub/internal/http/export.go`): una
// carpeta raíz `sytue-one[-<org>]-AAAA-MM-DD/` con una carpeta por producto, que es la
// entrega de cada uno (`platform/pkg/portabilidad`):
//
//	calendar/calendars/<nombre>.ics   → un calendario por fichero, el mismo cada vez que
//	                                    se trae, evento a evento (repetir.go)
//	contacts/contacts.vcf (o .csv)    → la libreta «Sytue», la misma cada vez, contacto
//	                                    a contacto (repetir.go)
//	notes/notebooks/<título>.html     → un cuaderno por cuaderno, con sus notas, de su
//	                                    volcado (notes.TraerCuaderno; repartir.go); uno
//	                                    del diario, como el diario de su día
//	docs/documents/<título>.<ext>     → un documento de Docus, Calcus o Diapos
//	                                    (documentos.go)
//	mail/messages/<carpeta>/<x>.eml   → la carpeta «Importados» de Mail (correo.go)
//	photos/photos + photos/albums     → Photos, que enlaza los originales de drive/files
//	                                    (fotos.go) —fotos, vídeos y AVIF; una HEIC, como
//	                                    el JPEG del sistema—, con sus favoritas, sus fechas
//	                                    y sus álbumes
//	drive/files/<ruta>                → Drive, dentro de la carpeta de lo traído
//
// Lo demás —la cuenta, las tablas de cada producto, Rooms, Cast, Pages, los avisos y el
// resumen— no tiene almacén al que ir y se deja TAL CUAL en
// `<datos>/Drive/Traído de Sytue AAAA-MM-DD/<producto>/…`: nada del ZIP se pierde. Lo
// que no entra en su almacén (un cuaderno con notas cifradas, un .ics ilegible, una
// foto que Photos no sabe abrir en este equipo) también se deja allí.
//
// Traer otra vez la misma copia, o una posterior, no repite nada: las notas se
// reconocen por el identificador que tenían en Sytue; las fotos, por la huella de su
// original; los álbumes, por su identificador; los eventos y los contactos, por su UID,
// en el mismo calendario y la misma libreta; los documentos, por su ruta en la copia; y
// los correos, por su Message-ID (repetir.go). Lo que se dejó en Drive no se copia otra
// vez mientras siga allí, aunque se haya movido (memoria.go). Y no se pisa nunca lo que
// la persona cambió: un evento, un contacto o un documento que cambió en Sytue se pone
// al día solo si aquí no se ha tocado desde que se trajo (un documento, como versión
// nueva, con la de antes en su historial); si se tocó, se queda como está y lo de Sytue
// entra aparte. Lo que la persona quitó de aquí no vuelve. En la carpeta de lo traído,
// un nombre repetido sale como «x (2).ext». Los almacenes del motor no admiten dos
// escritores, así que quien llama tiene que asegurarse antes de que Calendar, Contacts,
// Notepads, Mail, Docus, Calcus, Diapos y Photos están cerradas ([Escriben]).
package traer

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
)

// Topes contra un ZIP dañado o malicioso: lo más que se descomprime de un fichero y del
// ZIP entero, y cuántos ficheros se aceptan.
const (
	MaxFichero  = 4 << 30
	MaxTotal    = 64 << 30
	MaxFicheros = 500000
)

// ErrNoEsDeSytue: el ZIP no tiene la forma de «Tus datos».
var ErrNoEsDeSytue = errors.New("traer: el fichero no es una copia de «Tus datos» de Sytue")

// Escriben son las apps (por su clave de producto) cuyos almacenes se escriben al traer:
// tienen que estar cerradas mientras tanto.
var Escriben = []string{"calendar", "contacts", "notes", "mail", "docs", "sheets", "slides", "photos"}

// Contenido es lo que trae el ZIP, para enseñarlo antes de traerlo.
type Contenido struct {
	// Raiz es la carpeta de arriba (`sytue-one-acme-2026-11-20`).
	Raiz string
	// Ficheros son los ficheros de cada producto, por su carpeta en el ZIP.
	Ficheros map[string]int
}

// Productos son los productos que trae, en orden.
func (c Contenido) Productos() []string {
	out := make([]string, 0, len(c.Ficheros))
	for p := range c.Ficheros {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// entrada es un fichero del ZIP con su ruta ya limpia, sin la raíz: `calendar/calendars/x.ics`.
type entrada struct {
	ruta string
	f    *zip.File
}

func (e entrada) producto() string { p, _, _ := strings.Cut(e.ruta, "/"); return p }

// leer abre el contenido del fichero con su tope.
func (e entrada) leer() ([]byte, error) {
	if e.f.UncompressedSize64 > MaxFichero {
		return nil, fmt.Errorf("traer: %s es demasiado grande", e.ruta)
	}
	rc, err := e.f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	datos, err := io.ReadAll(io.LimitReader(rc, int64(e.f.UncompressedSize64)+1))
	if err == nil && uint64(len(datos)) > e.f.UncompressedSize64 {
		err = fmt.Errorf("traer: %s no mide lo que dice", e.ruta)
	}
	return datos, err
}

// ruta limpia: relativa, sin «..» ni raíz, con barras normales. Vacía, no vale.
func limpia(nombre string) string {
	nombre = strings.ReplaceAll(nombre, `\`, "/")
	if strings.HasPrefix(nombre, "/") || strings.Contains(nombre, "\x00") {
		return ""
	}
	limpio := path.Clean(nombre)
	if limpio == "." || limpio == ".." || strings.HasPrefix(limpio, "../") || path.IsAbs(limpio) {
		return ""
	}
	for s := range strings.SplitSeq(limpio, "/") {
		if s == ".." || len(s) == 2 && s[1] == ':' { // ni subir ni una unidad de Windows
			return ""
		}
	}
	return limpio
}

// abrir lee el índice del ZIP y comprueba que es de «Tus datos»: una sola carpeta raíz
// con, al menos, una carpeta de producto conocida o el resumen.
func abrir(ruta string) (*zip.ReadCloser, Contenido, []entrada, error) {
	z, err := zip.OpenReader(ruta)
	if err != nil {
		return nil, Contenido{}, nil, err
	}
	c := Contenido{Ficheros: map[string]int{}}
	var es []entrada
	var total uint64
	falla := func(err error) (*zip.ReadCloser, Contenido, []entrada, error) {
		_ = z.Close()
		return nil, Contenido{}, nil, err
	}
	if len(z.File) > MaxFicheros {
		return falla(fmt.Errorf("traer: el ZIP tiene más de %d ficheros", MaxFicheros))
	}
	conocido := false
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		r := limpia(f.Name)
		raiz, resto, ok := strings.Cut(r, "/")
		if r == "" || !ok || resto == "" {
			return falla(ErrNoEsDeSytue)
		}
		if c.Raiz == "" {
			c.Raiz = raiz
		} else if raiz != c.Raiz {
			return falla(ErrNoEsDeSytue)
		}
		total += f.UncompressedSize64
		if total > MaxTotal {
			return falla(fmt.Errorf("traer: el ZIP descomprimido pasa de %d GiB", MaxTotal>>30))
		}
		e := entrada{ruta: resto, f: f}
		p := e.producto()
		if !strings.Contains(resto, "/") {
			p = "" // el resumen y lo suelto
		}
		if _, ok := productosConocidos[p]; ok || strings.HasPrefix(resto, "summary.") {
			conocido = true
		}
		if p != "" {
			c.Ficheros[p]++
		}
		es = append(es, e)
	}
	if !conocido {
		return falla(ErrNoEsDeSytue)
	}
	return z, c, es, nil
}

// productosConocidos son las carpetas que pone el Hub del servicio.
var productosConocidos = map[string]struct{}{
	"account": {}, "mail": {}, "drive": {}, "calendar": {}, "contacts": {}, "notes": {}, "docs": {},
	"photos": {}, "rooms": {}, "cast": {}, "pages": {}, "hub": {},
}

// Mirar dice lo que trae el ZIP sin traer nada.
func Mirar(ruta string) (Contenido, error) {
	z, c, _, err := abrir(ruta)
	if err != nil {
		return Contenido{}, err
	}
	_ = z.Close()
	return c, nil
}
