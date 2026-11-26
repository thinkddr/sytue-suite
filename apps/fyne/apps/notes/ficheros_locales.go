// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"bytes"
	"context"
	"errors"
	"html"
	"image/jpeg"
	"image/png"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
)

// Los FICHEROS DE ESTE EQUIPO en una nota, solo en Sytue Office (la edición local, §6.5
// del plan), que no tiene Drive donde subirlos:
//
//   - N-06, la nota de IMAGEN desde un fichero: se elige con el selector del sistema
//     («Añadir una imagen del equipo…» en el «…» del cuaderno) o se suelta sobre el
//     cuaderno. La imagen va DENTRO de la nota, como su adjunto (`data.attachments`, una
//     URL de datos) y sin `data.url`: vive en el almacén en claro y se ve sin red. Una
//     foto grande se reduce antes (miniatura.Reducir) para que quepa en los 5 MB de
//     `data`; una pequeña de la web (PNG, JPEG, GIF, WebP) va tal cual. AVIF, TIFF y BMP
//     se leen con los decodificadores que registra la edición (`local/imagenes`, los de
//     Photos) y se guardan en JPEG o PNG (BMP, siempre en PNG); una HEIC se pasa antes a
//     JPEG con el sistema (FicherosLocales.HEIC). Así la nota se ve en cualquier edición.
//   - La nota de imagen por URL (imagen_url.go): la imagen de una dirección se baja solo
//     cuando la persona lo pide, por el permiso puntual del portero, y se guarda igual,
//     como adjunto de la nota.
//   - N-07, el ENLACE a un fichero: soltarlo sobre una nota de texto o de tarea le añade
//     su enlace `file:///…` —donde está el cursor, si es la que se escribe—, y soltarlo
//     sobre el cuaderno crea una nota con él. Se abre con su app de la edición o la del
//     sistema (AbrirLocal). El repositorio local guarda esos enlaces (`notes.NewLocal`);
//     el de la edición conectada los quita.
//
// Sin FicherosLocales (la edición conectada), nada de esto existe y todo es como siempre.

// FicherosLocales es lo que la edición local da a la pantalla para los ficheros del equipo.
type FicherosLocales struct {
	// Elegir abre el selector de imágenes del sistema (`ficheros.ElegirImagenes`) y
	// devuelve las rutas elegidas. Nil: no se ofrece elegir, solo soltar.
	Elegir func() ([]string, error)
	// Rotulo es la entrada del «…» del cuaderno que lo abre.
	Rotulo string
	// NoEsImagen es el aviso de un fichero elegido que no se puede leer como imagen.
	NoEsImagen string
	// HEIC, si no es nil, pasa a JPEG una foto HEIC o HEIF con lo que tenga el sistema
	// (`imagenes.AJPEG`, el mismo camino que Photos): no hay decodificador que la lea.
	// Nil: una HEIC no es una imagen que se sepa leer.
	HEIC func(ctx context.Context, nombre string, datos []byte) (string, []byte, error)
	// BajarImagen, si no es nil, baja la imagen de una dirección https para la nota de
	// imagen por URL (imagen_url.go): SOLO cuando la persona lo pide, por el permiso
	// puntual del portero (`red.Portero.Puntual`, propósito `imagen-remota`). Nil: la
	// nota de imagen por URL no se crea ni se carga aquí.
	BajarImagen func(ctx context.Context, direccion string) ([]byte, error)
}

// Medidas de la imagen de una nota.
const (
	// LadoImagenNota es el lado más largo con que se guarda una foto reducida: de sobra
	// para una tarjeta (`max-h-64`) y para la hoja del árbol, en pantallas densas.
	LadoImagenNota = 2048
	// imagenTalCual es lo más que ocupa una imagen que se guarda sin tocar (si no pasa
	// además de LadoImagenNota).
	imagenTalCual = 1 << 20
	// maxFicheroImagen es lo más que se lee de un fichero de imagen: más, ni se intenta.
	maxFicheroImagen = 64 << 20
	// cabeEnNota es lo más que puede ocupar la imagen en bytes para que su URL de datos
	// (base64: cuatro tercios) quepa en `data` con lo demás.
	cabeEnNota = (MaxDatosNota-(16<<10))/4*3 - 1
)

// ErrNoEsImagen: no es una imagen que se sepa leer, o no cabe ni reducida (también lo
// que baja BajarImagen y no es una imagen o pasa de MaxImagenRemota).
var ErrNoEsImagen = errors.New("notes: el fichero no es una imagen que se pueda guardar en una nota")

// errSinHEIC: una HEIC (o HEIF) y nada con qué pasarla a JPEG: sin FicherosLocales.HEIC,
// o el sistema no sabe.
var errSinHEIC = errors.New("notes: este equipo no sabe abrir fotos HEIC")

// extensionesImagenNota son las que se leen: las de la web (PNG, JPEG, GIF y WebP) y las
// que registra la edición (AVIF, TIFF y BMP, `local/imagenes`). Las de extensionesHEIC, si
// hay con qué pasarlas a JPEG (FicherosLocales.HEIC).
var (
	extensionesImagenNota = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".tif", ".tiff", ".bmp"}
	extensionesHEIC       = []string{".heic", ".heif"}
)

// formatosWeb son los que una nota guarda tal cual, si caben: los que lee cualquier
// edición (`miniatura`).
var formatosWeb = []string{"png", "jpeg", "gif", "webp"}

// esImagenLeible dice, por la extensión, si un fichero soltado se toma por imagen.
func (f *FicherosLocales) esImagenLeible(ruta string) bool {
	ext := strings.ToLower(filepath.Ext(ruta))
	return slices.Contains(extensionesImagenNota, ext) || f != nil && f.HEIC != nil && slices.Contains(extensionesHEIC, ext)
}

// adjuntoDeFichero lee una imagen del equipo y la deja lista para ir dentro de una nota
// (adjuntoDeDatos).
func (f *FicherosLocales) adjuntoDeFichero(ctx context.Context, ruta string, ahora time.Time) (Adjunto, error) {
	info, err := os.Stat(ruta)
	if err != nil {
		return Adjunto{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxFicheroImagen {
		return Adjunto{}, ErrNoEsImagen
	}
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return Adjunto{}, err
	}
	return f.adjuntoDeDatos(ctx, filepath.Base(ruta), datos, ahora)
}

// adjuntoDeDatos deja una imagen lista para ir dentro de una nota: una de la web (PNG,
// JPEG, GIF, WebP) tal cual si es pequeña; si no, o si es AVIF, TIFF o BMP, decodificada y
// vuelta a guardar ([recodificar]). Una HEIC (un ISOBMFF que no lee ningún
// decodificador) pasa antes a JPEG con el sistema.
func (f *FicherosLocales) adjuntoDeDatos(ctx context.Context, nombre string, datos []byte, ahora time.Time) (Adjunto, error) {
	if len(datos) > maxFicheroImagen {
		return Adjunto{}, ErrNoEsImagen
	}
	cfg, formato, err := miniatura.Cabecera(datos)
	if err != nil && esISOBMFF(datos) {
		if f == nil || f.HEIC == nil {
			return Adjunto{}, errSinHEIC
		}
		if nombre, datos, err = f.HEIC(ctx, nombre, datos); err != nil {
			if ctx.Err() != nil {
				return Adjunto{}, ctx.Err()
			}
			return Adjunto{}, errSinHEIC
		}
		cfg, formato, err = miniatura.Cabecera(datos)
	}
	if err != nil || !miniatura.Cabe(cfg) {
		return Adjunto{}, ErrNoEsImagen
	}
	if slices.Contains(formatosWeb, formato) && len(datos) <= imagenTalCual && max(cfg.Width, cfg.Height) <= LadoImagenNota {
		return nuevoAdjunto(nombre, "image/"+formato, datos, ahora), nil
	}
	salida, guardada, err := recodificar(ctx, datos, formato)
	if err != nil {
		return Adjunto{}, err
	}
	ext := ".png"
	if salida == "image/jpeg" {
		ext = ".jpg"
	}
	nombre = strings.TrimSuffix(nombre, filepath.Ext(nombre)) + ext
	return nuevoAdjunto(nombre, salida, guardada, ahora), nil
}

// recodificar decodifica la imagen (con el turno y la guarda de `miniatura`) y la vuelve
// a guardar reducida a LadoImagenNota, y más aún si ni así cabe en `data`: una de la web
// en el tipo de `miniatura.TipoSalida` (JPEG la que lo era; PNG la que puede tener
// transparencia), AVIF o TIFF en JPEG si es opaca y en PNG si no, y BMP siempre en PNG:
// es un mapa de bits sin pérdida (capturas, dibujos, iconos) y el JPEG le pondría
// manchas; el PNG lo guarda igual y ocupa mucho menos. Cancelado ctx mientras espera el
// turno, no decodifica nada y devuelve su error.
func recodificar(ctx context.Context, datos []byte, formato string) (string, []byte, error) {
	liberar, err := miniatura.TurnoConContexto(ctx)
	if err != nil {
		return "", nil, err
	}
	defer liberar()
	img, err := miniatura.Decodificar(datos)
	if err != nil {
		return "", nil, ErrNoEsImagen
	}
	salida, ok := miniatura.TipoSalida("image/" + formato)
	if formato == "bmp" {
		salida, ok = "image/png", true
	}
	if !ok {
		salida = "image/png"
		if o, es := img.(interface{ Opaque() bool }); es && o.Opaque() {
			salida = "image/jpeg"
		}
	}
	for _, lado := range []int{LadoImagenNota, LadoImagenNota * 5 / 8, LadoImagenNota / 2} {
		var b bytes.Buffer
		r := miniatura.Reducir(img, lado)
		if salida == "image/jpeg" {
			err = jpeg.Encode(&b, r, &jpeg.Options{Quality: miniatura.CalidadJPEG})
		} else {
			err = png.Encode(&b, r)
		}
		if err == nil && b.Len() <= cabeEnNota {
			return salida, b.Bytes(), nil
		}
	}
	return "", nil, ErrNoEsImagen
}

// esISOBMFF dice si unos bytes empiezan como un fichero ISOBMFF (la caja `ftyp`), que es
// como empiezan HEIC, HEIF y AVIF.
func esISOBMFF(datos []byte) bool { return len(datos) >= 12 && string(datos[4:8]) == "ftyp" }

// notaDeImagen es la nota de imagen de un fichero del equipo, con la imagen dentro.
func (f *FicherosLocales) notaDeImagen(ctx context.Context, ruta string, ahora time.Time) (api.NoteInput, error) {
	a, err := f.adjuntoDeFichero(ctx, ruta, ahora)
	if err != nil {
		return api.NoteInput{}, err
	}
	data, err := conAdjuntos(nil, []Adjunto{a})
	if err != nil {
		return api.NoteInput{}, err
	}
	titulo := strings.TrimSuffix(filepath.Base(ruta), filepath.Ext(ruta))
	return api.NoteInput{Kind: "image", Title: titulo, Data: data}, nil
}

// imagenIncrustada es la imagen que una nota de imagen lleva dentro: sin dirección
// (`data.url`), su primer adjunto de imagen (N-06); con ella, la de esa dirección bajada
// a petición, que la lleva por origen (imagen_url.go).
func imagenIncrustada(n Nota) (Adjunto, bool) {
	if n.Tipo != "image" {
		return Adjunto{}, false
	}
	for _, a := range n.Adjuntos {
		if strings.HasPrefix(a.DataURL, "data:image/") && a.Origen == n.URL {
			return a, true
		}
	}
	return Adjunto{}, false
}

// imagenAdjuntaDe es la imagen de un adjunto tal como vaya, como imagenDe la de una
// dirección: la primera vez se decodifica en su rutina y, al tenerla, se repinta.
func (p *Pantalla) imagenAdjuntaDe(a Adjunto) imagenRemota {
	clave := "adjunto:" + a.ID
	if r, ok := p.imagenes[clave]; ok {
		return r
	}
	if p.imagenes == nil {
		p.imagenes = map[string]imagenRemota{}
	}
	p.imagenes[clave] = imagenRemota{}
	p.iniciar(func() { p.cargarImagenAdjunta(a) })
	return imagenRemota{}
}

// cargarImagenAdjunta la decodifica (con la guarda de tamaño de miniatura) y repinta.
func (p *Pantalla) cargarImagenAdjunta(a Adjunto) {
	var r imagenRemota
	if b, err := a.contenido(); err == nil {
		r.img, r.err = miniatura.Decodificar(b)
	} else {
		r.err = err
	}
	r.lista = true
	p.enHilo(func() {
		if p.imagenes == nil {
			p.imagenes = map[string]imagenRemota{}
		}
		p.imagenes["adjunto:"+a.ID] = r
		p.Repintar()
	})
}

// puedeFicherosLocales dice si ahora se pueden añadir ficheros del equipo al cuaderno
// abierto: con FicherosLocales, escritura y un cuaderno que no sea cifrado ni cerrado.
func (p *Pantalla) puedeFicherosLocales() bool {
	return p.FicherosLocales != nil && p.puedeEscribir() && !p.activo.Cifrado && !p.bloqueado
}

// elegirImagenLocal es «Añadir una imagen del equipo…»: el selector del sistema y una
// nota de imagen por cada una que se elija.
func (p *Pantalla) elegirImagenLocal() {
	f := p.FicherosLocales
	if f == nil || f.Elegir == nil {
		return
	}
	cuaderno := p.activo.ID
	p.iniciar(func() {
		rutas, err := f.Elegir()
		if err != nil || len(rutas) == 0 {
			return // cancelar el selector no es un error
		}
		p.crearImagenes(cuaderno, rutas, nil)
	})
}

// crearImagenes crea una nota de imagen por ruta en el cuaderno, si sigue abierto; en el
// lienzo libre, a partir de en y escalonadas. Va fuera del hilo de la interfaz.
func (p *Pantalla) crearImagenes(cuaderno string, rutas []string, en *fyne.Position) {
	ctx, _ := p.contextoTrabajo()
	for i, r := range rutas {
		in, err := p.FicherosLocales.notaDeImagen(ctx, r, time.Now())
		if err != nil {
			p.avisar(ui.AvisoPeligro, filepath.Base(r), p.textoFalloImagen(err, false))
			continue
		}
		escalonar(&in, en, i)
		if err := p.crearEn(in, cuaderno, nil); err != nil {
			return // ya lo ha avisado crear
		}
	}
}

// escalonar pone la nota i de las soltadas en el lienzo libre: la primera donde se soltó
// y las demás un poco más abajo y a la derecha, como baseNueva.
func escalonar(in *api.NoteInput, en *fyne.Position, i int) {
	if en == nil {
		return
	}
	in.X = ptr(max(0, int(math.Round(float64(en.X)))+i*28))
	in.Y = ptr(max(0, int(math.Round(float64(en.Y)))+i*28))
}

// enlaceAFichero es el `file:///…` de una ruta del equipo, con lo que haga falta
// escapado. Una ruta de Windows (`C:\x`) queda `file:///C:/x`.
func enlaceAFichero(ruta string) string {
	r := filepath.ToSlash(ruta)
	if !strings.HasPrefix(r, "/") {
		r = "/" + r
	}
	return (&url.URL{Scheme: "file", Path: r}).String()
}

// rutaDeEnlace es la ruta del equipo de un enlace `file:`, al revés que enlaceAFichero.
func rutaDeEnlace(u *url.URL) string {
	r := u.Path
	if len(r) >= 3 && r[0] == '/' && r[2] == ':' {
		r = r[1:] // `/C:/x` es `C:/x` en Windows
	}
	return filepath.FromSlash(r)
}

// parrafoEnlace es el párrafo con el enlace a un fichero, con su nombre por texto.
func parrafoEnlace(ruta string) string {
	return `<p><a href="` + html.EscapeString(enlaceAFichero(ruta)) + `">` + html.EscapeString(filepath.Base(ruta)) + `</a></p>`
}

// SoltarFicheros atiende ficheros de este equipo soltados en la ventana, en el hilo de la
// interfaz (Sytue Office): sobre una nota de texto o de tarea, su enlace en ella; sobre el
// cuaderno, una nota por fichero —de imagen si lo es, con la imagen dentro, y si no, con
// su enlace—, en el lienzo libre donde se soltó. Dice si eran para ella: sin
// FicherosLocales, sin escritura o fuera del cuaderno, no.
func (p *Pantalla) SoltarFicheros(rutas []string, pos fyne.Position) bool {
	if len(rutas) == 0 || p.win == nil || p.vista != VistaCuaderno || !p.puedeFicherosLocales() ||
		p.win.Canvas().Overlays().Top() != nil || p.desplazador == nil {
		return false
	}
	if t := p.tarjetaBajo(pos); t != nil {
		p.enlazarEnNota(t.nota, rutas)
		return true
	}
	if !contienePunto(p.desplazador, pos) {
		return false
	}
	var en *fyne.Position
	if p.activo.Vista == Libre && p.desplazador.Content != nil {
		o := fyne.CurrentApp().Driver().AbsolutePositionForObject(p.desplazador.Content)
		x := fyne.NewPos(pos.X-o.X, pos.Y-o.Y)
		en = &x
	}
	cuaderno := p.activo.ID
	var imagenes, otros []string
	for _, r := range rutas {
		if p.FicherosLocales.esImagenLeible(r) {
			imagenes = append(imagenes, r)
		} else {
			otros = append(otros, r)
		}
	}
	p.iniciar(func() {
		p.crearImagenes(cuaderno, imagenes, en)
		for i, r := range otros {
			in := api.NoteInput{Kind: "text", Title: strings.TrimSuffix(filepath.Base(r), filepath.Ext(r)), Content: parrafoEnlace(r)}
			escalonar(&in, en, len(imagenes)+i)
			if p.crearEn(in, cuaderno, nil) != nil {
				return
			}
		}
	})
	return true
}

// tarjetaBajo es la tarjeta de una nota de texto o de tarea, que se pueda escribir, que
// está bajo ese punto de la ventana.
func (p *Pantalla) tarjetaBajo(pos fyne.Position) *Tarjeta {
	for _, t := range p.lienzo.vivas {
		if t.SoloLectura || (t.nota.Tipo != "text" && t.nota.Tipo != "todo") || !contienePunto(t, pos) {
			continue
		}
		return t
	}
	return nil
}

// enlazarEnNota pone los enlaces a esos ficheros en la nota: en el editor, donde está el
// cursor, si es la que se escribe; si no, en párrafos nuevos al final de su texto.
func (p *Pantalla) enlazarEnNota(n Nota, rutas []string) {
	if s := p.edicion; s != nil && s.nota == n.ID && s.editor != nil {
		for _, r := range rutas {
			s.editor.Enlazar(enlaceAFichero(r), filepath.Base(r))
		}
		p.enfocar(s.editor.Zona())
		return
	}
	var b strings.Builder
	b.WriteString(n.Contenido)
	for _, r := range rutas {
		b.WriteString(parrafoEnlace(r))
	}
	contenido := b.String()
	p.lanzar(func() error { return p.editar(n.ID, Parche{Contenido: &contenido}, true) })
}

// nuevoAdjunto es un adjunto con sus bytes en una URL de datos, como leerAdjunto.
func nuevoAdjunto(nombre, tipo string, datos []byte, ahora time.Time) Adjunto {
	a := Adjunto{Name: nombre, Size: int64(len(datos)), Type: tipo}
	a.ID, a.DataURL = idAdjunto(ahora), urlDeDatos(tipo, datos)
	return a
}
