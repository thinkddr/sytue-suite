// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"bytes"
	"image"
	_ "image/gif"  // las imágenes de una nota: GIF,
	_ "image/jpeg" // JPEG,
	_ "image/png"  // PNG
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/webp" // y WebP, las cuatro que deja el saneador
	"golang.org/x/net/html"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// IMÁGENES Y FICHEROS DENTRO DEL TEXTO de una nota. Se guardan como los demás adjuntos
// —en `data.attachments`, como URL de datos (adjuntos.go)— y el cuerpo los nombra por su
// `id` con el esquema `adjunto:`, que es lo único que el saneador del servicio deja en un
// `src` o en un `href` además de las direcciones web:
//
//	imagen   <img src="adjunto:1732540000000-ab12cd" alt="plano.png">
//	fichero  <a href="adjunto:1732540000000-ab12cd" …>informe.pdf</a>
//
// Así la imagen no viaja dos veces (en el cuerpo y en `data`), un cuaderno cifrado la
// sella con el resto de `data`, y una nota pública no puede hacer que otro navegador pida
// nada a ningún sitio: `adjunto:` no es una dirección (ver `apps/nucleo/notes/model/html.go`).
//
// Pulsar un fichero lo descarga; una imagen se pinta con la imagen del adjunto, y si el
// adjunto ya no está, con el hueco de una imagen rota.

// EsquemaAdjunto es el prefijo de un `src` o un `href` que nombra un adjunto de la nota.
const EsquemaAdjunto = "adjunto:"

// reIDAdjunto es la forma de un id de adjunto: la de `leerAdjunto` y la de la web
// (`Date.now()-aleatorio`), y nada que pueda salirse de una URL.
var reIDAdjunto = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// IDDeAdjunto es el id del adjunto que nombra un `src` o un `href` (`adjunto:ID`).
func IDDeAdjunto(ref string) (string, bool) {
	id, ok := strings.CutPrefix(ref, EsquemaAdjunto)
	return id, ok && reIDAdjunto.MatchString(id)
}

// TiposImagenEnLinea son los tipos de imagen que se pintan dentro del texto: los que el
// navegador y Fyne leen sin más (un HEIC o un TIFF van como fichero).
var TiposImagenEnLinea = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

// esImagenEnLinea dice si un adjunto se pone como imagen y no como fichero.
func esImagenEnLinea(a Adjunto) bool { return slices.Contains(TiposImagenEnLinea, a.Type) }

// InsertarAdjunto pone en el cursor un adjunto ya guardado en la nota: la imagen dentro
// del texto, o el enlace al fichero con su nombre y un espacio para seguir escribiendo
// fuera de él (como `escribirEnlace`). Un paso de deshacer.
func (e *EditorNota) InsertarAdjunto(a Adjunto) {
	if !reIDAdjunto.MatchString(a.ID) {
		return
	}
	if !esImagenEnLinea(a) {
		e.escribirEnlace(EsquemaAdjunto+a.ID, a.Name)
		return
	}
	a0, b0 := e.seleccion()
	e.hacer(func() {
		if a0 != b0 {
			e.cur = e.doc.Borrar(a0, b0)
		}
		im := Tramo{Imagen: []html.Attribute{{Key: "src", Val: EsquemaAdjunto + a.ID}, {Key: "alt", Val: a.Name}}}
		e.cur = e.doc.InsertarDocumento(e.cur, &Documento{Bloques: []Bloque{{Tipo: Parrafo, Tramos: []Tramo{im}}}})
		e.ancla = e.cur
	})
}

// imagenesDeAdjuntos da la imagen de un `src="adjunto:ID"` sacada de los adjuntos que
// diga `lista` (los de la nota AHORA: uno recién puesto se ve en seguida). Cada imagen se
// descodifica una vez; lo que no es un adjunto, o no se puede leer, no tiene imagen.
func imagenesDeAdjuntos(lista func() []Adjunto) func(src string) image.Image {
	var mu sync.Mutex
	hechas := map[string]image.Image{}
	return func(src string) image.Image {
		id, ok := IDDeAdjunto(src)
		if !ok {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		if im, ok := hechas[id]; ok {
			return im
		}
		as := lista()
		i := slices.IndexFunc(as, func(a Adjunto) bool { return a.ID == id })
		if i < 0 {
			// Todavía no está (se acaba de poner y la nota aún no ha vuelto con él): no se
			// apunta, para que se vea en cuanto llegue.
			return nil
		}
		var im image.Image
		if b, err := as[i].contenido(); err == nil && esImagenEnLinea(as[i]) {
			im, _, _ = image.Decode(bytes.NewReader(b))
		}
		hechas[id] = im
		return im
	}
}

// adjuntarEnCuerpo es «Imagen…» y «Archivo…» del menú «+» y de «/»: el selector del
// sistema (solo imágenes, en el primero), los ficheros a los adjuntos de la nota y, ya
// guardados, cada uno en el cursor del editor.
func (p *Pantalla) adjuntarEnCuerpo(id string, ed *EditorNota, imagen bool) {
	p.iniciar(func() {
		elegir := ficheros.Elegir
		if imagen {
			elegir = func() ([]string, error) { return ficheros.ElegirImagenes(p.tr.T("editor.image", nil)) }
		}
		rutas, err := elegir()
		if err != nil || len(rutas) == 0 {
			return // cancelar el selector no es un error
		}
		nuevos := make([]Adjunto, 0, len(rutas))
		for _, r := range rutas {
			a, err := leerAdjunto(r, time.Now())
			if err != nil {
				p.avisar(ui.AvisoPeligro, p.tr.T("editor.attachTooLarge", nil), "")
				return
			}
			nuevos = append(nuevos, a)
		}
		if p.cambiarAdjuntos(id, func(as []Adjunto) []Adjunto { return append(as, nuevos...) }) != nil {
			return
		}
		p.enHilo(func() {
			for _, a := range nuevos {
				ed.InsertarAdjunto(a)
			}
			ed.reenfocar()
		})
	})
}

// adjuntoDeEnlace es el adjunto que nombra un enlace `adjunto:ID`, buscado en las notas
// cargadas (un id es único: la hora en milisegundos y seis letras al azar).
func (p *Pantalla) adjuntoDeEnlace(href string) (Adjunto, bool) {
	id, ok := IDDeAdjunto(href)
	if !ok {
		return Adjunto{}, false
	}
	for _, n := range p.notas {
		if i := slices.IndexFunc(n.Adjuntos, func(a Adjunto) bool { return a.ID == id }); i >= 0 {
			return n.Adjuntos[i], true
		}
	}
	return Adjunto{}, false
}
