// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La NOTA DE IMAGEN POR URL de Sytue Office (la edición local, §5.8 del plan): la imagen
// de su `data.url` NO se baja sola —el portero de red no deja salir nada que la persona
// no haya configurado—, sino solo cuando la persona lo pide, como las imágenes remotas de
// un correo en Mail:
//
//   - «Cargar la imagen» en la tarjeta de una nota de imagen con dirección y sin la imagen
//     dentro (la que llega importada o de la edición conectada);
//   - «Añadir una imagen de una dirección web…» en el «…» del cuaderno, con la dirección
//     que se acaba de escribir: la nota nace ya con la imagen;
//   - «Volver a cargar la imagen» en el «…» de la nota que ya la lleva dentro, por si ha
//     cambiado en su sitio: se baja otra vez, igual, y SOLO si llega bien sustituye a la
//     de dentro, que se suelta de la nota (y de la memoria) para que la compactación del
//     almacén la quite del disco; si no ha cambiado, no se guarda nada y se dice.
//
// La baja FicherosLocales.BajarImagen, por el permiso puntual del portero (propósito
// `imagen-remota`: solo https, solo direcciones públicas de Internet, nada con «Sin
// conexión»), y se guarda como adjunto de la nota (como N-06), con su dirección por origen
// (Adjunto.Origen): desde entonces se ve sin red. La dirección se queda en `data.url`.
//
// Sin BajarImagen (la edición conectada), nada de esto existe: la tarjeta pide la imagen
// como siempre (`imagenDe`).

// Los fallos de BajarImagen que la pantalla sabe decir (el resto es «no se ha podido
// bajar»).
var (
	// ErrImagenNoPermitida: la dirección no es https, o lleva a este equipo o a una red
	// privada; el permiso puntual no sale ahí.
	ErrImagenNoPermitida = errors.New("notes: solo se bajan imágenes de direcciones https públicas")
	// ErrImagenSinConexion: «Sin conexión» está encendido.
	ErrImagenSinConexion = errors.New("notes: «Sin conexión» está encendido")
)

// MaxImagenRemota es lo más que se baja de una imagen por URL: una HEIC de 48 MP o un
// TIFF escaneado caben; lo que pasa de ahí no es una foto para una nota.
const MaxImagenRemota = 32 << 20

// TextosImagenURL son los textos de la nota de imagen por URL, que solo dice Sytue Office
// (es, en; los otros veinticuatro idiomas, en `traduccionesImagenURL`,
// imagen_url_idiomas.go).
var TextosImagenURL = map[string][2]string{
	"anadir":      {"Añadir una imagen de una dirección web…", "Add an image from a web address…"},
	"titulo":      {"Imagen de una dirección web", "Image from a web address"},
	"campo":       {"Dirección de la imagen", "Image address"},
	"pista":       {"Se baja ahora, una sola vez, y se guarda dentro de la nota para verla sin conexión. Solo direcciones https públicas.", "It's downloaded now, just once, and saved inside the note so you can see it offline. Only public https addresses."},
	"cargar":      {"Cargar la imagen", "Load the image"},
	"sinBajar":    {"Imagen de %s, sin bajar: Sytue Office no sale a la red hasta que lo pidas.", "Image from %s, not downloaded: Sytue Office doesn't go online until you ask."},
	"noPermitida": {"Solo se cargan imágenes de direcciones https públicas de Internet.", "Only images from public https addresses on the internet can be loaded."},
	"sinConexion": {"No se carga: «Sin conexión» está puesto en Ajustes.", "Not loaded: «Offline» is on in Settings."},
	"noBaja":      {"No se ha podido bajar la imagen. Mira la dirección y la conexión.", "Couldn't download the image. Check the address and the connection."},
	"sinHEIC":     {"Este equipo no sabe abrir fotos HEIC: instala ffmpeg o libheif.", "This device can't open HEIC photos: install ffmpeg or libheif."},
	"recargar":    {"Volver a cargar la imagen", "Reload the image"},
	"igual":       {"La imagen no ha cambiado en su dirección: la nota se queda como estaba.", "The image hasn't changed at its address: the note stays as it was."},
}

// textoImagenURL es un texto de TextosImagenURL en ese idioma.
func textoImagenURL(idioma i18n.Locale, clave string) string {
	return i18n.Propios{Base: TextosImagenURL, Traducciones: traduccionesImagenURL}.En(idioma, clave)
}

// plazoImagenRemota es lo más que se espera a una imagen por URL, conversión incluida
// (el cliente del portero corta antes la descarga: 30 s).
const plazoImagenRemota = 2 * time.Minute

// puedeImagenURL dice si en el cuaderno abierto se pueden crear y cargar notas de imagen
// por URL: con BajarImagen y donde se pueden añadir ficheros del equipo.
func (p *Pantalla) puedeImagenURL() bool {
	return p.FicherosLocales != nil && p.FicherosLocales.BajarImagen != nil && p.puedeFicherosLocales()
}

// direccionImagen es la dirección de una imagen tal como se escribió, si es https con
// host; si no, ErrImagenNoPermitida (el permiso puntual no saldría).
func direccionImagen(direccion string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(direccion))
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" {
		return nil, ErrImagenNoPermitida
	}
	return u, nil
}

// nombreDeDireccion es el nombre del fichero de una dirección (lo último de su ruta), o
// su host si no lo tiene.
func nombreDeDireccion(u *url.URL) string {
	if n := path.Base(u.Path); n != "" && n != "/" && n != "." {
		return n
	}
	return strings.ToLower(u.Hostname())
}

// adjuntoDeDireccion baja la imagen de una dirección y la deja lista para ir dentro de
// la nota (adjuntoDeDatos), con la dirección por origen, tal cual se escribió.
func (f *FicherosLocales) adjuntoDeDireccion(ctx context.Context, direccion string, ahora time.Time) (Adjunto, error) {
	if f == nil || f.BajarImagen == nil {
		return Adjunto{}, ErrImagenNoPermitida
	}
	direccion = strings.TrimSpace(direccion)
	u, err := direccionImagen(direccion)
	if err != nil {
		return Adjunto{}, err
	}
	datos, err := f.BajarImagen(ctx, direccion)
	if err != nil {
		return Adjunto{}, err
	}
	a, err := f.adjuntoDeDatos(ctx, nombreDeDireccion(u), datos, ahora)
	if err != nil {
		return Adjunto{}, err
	}
	a.Origen = direccion
	return a, nil
}

// notaDeImagenRemota es la nota de imagen de una dirección que se acaba de escribir: su
// `data.url` y la imagen ya dentro, para verla sin red.
func (f *FicherosLocales) notaDeImagenRemota(ctx context.Context, direccion string, ahora time.Time) (api.NoteInput, error) {
	a, err := f.adjuntoDeDireccion(ctx, direccion, ahora)
	if err != nil {
		return api.NoteInput{}, err
	}
	con, err := json.Marshal(map[string]string{"url": a.Origen})
	if err != nil {
		return api.NoteInput{}, err
	}
	data, err := conAdjuntos(con, []Adjunto{a})
	if err != nil {
		return api.NoteInput{}, err
	}
	u, _ := direccionImagen(a.Origen)
	nombre := nombreDeDireccion(u)
	titulo := strings.TrimSuffix(nombre, path.Ext(nombre))
	if titulo == "" {
		titulo = strings.ToLower(u.Hostname())
	}
	return api.NoteInput{Kind: "image", Title: titulo, Data: data}, nil
}

// textoFalloImagen es lo que se dice cuando una imagen no entra en una nota: la de una
// dirección (remota) o la de un fichero del equipo.
func (p *Pantalla) textoFalloImagen(err error, remota bool) string {
	t := func(clave string) string { return textoImagenURL(p.tr.Locale(), clave) }
	switch {
	case errors.Is(err, ErrImagenNoPermitida):
		return t("noPermitida")
	case errors.Is(err, ErrImagenSinConexion):
		return t("sinConexion")
	case errors.Is(err, errSinHEIC):
		return t("sinHEIC")
	case remota && !errors.Is(err, ErrNoEsImagen):
		return t("noBaja")
	case p.FicherosLocales != nil:
		return p.FicherosLocales.NoEsImagen
	}
	return ""
}

// cargarImagenRemota es «Cargar la imagen» en la tarjeta de una nota de imagen con
// dirección: la baja ahora, por el permiso puntual, y la guarda como adjunto de la nota
// (la que hubiera de esa misma dirección deja su sitio). Mientras, el botón da vueltas.
func (p *Pantalla) cargarImagenRemota(n Nota) { p.bajarImagenDeNota(n, "cargar") }

// recargarImagenRemota es «Volver a cargar la imagen» en el «…» de una nota de imagen que
// ya la lleva dentro: la misma bajada, por si ha cambiado en su sitio.
func (p *Pantalla) recargarImagenRemota(n Nota) { p.bajarImagenDeNota(n, "recargar") }

// puedeRecargarImagen dice si el «…» de la nota ofrece «Volver a cargar la imagen»: una
// nota de imagen con su dirección y la imagen de esa dirección ya dentro, donde se puede
// cargar (como CargarImagen en la tarjeta: con escritura, ni cerrada ni cifrada).
func (p *Pantalla) puedeRecargarImagen(n Nota) bool {
	if !p.puedeImagenURL() || n.Tipo != "image" || !esWeb(n.URL) || p.lienzo.SoloLectura || p.bloqueado || n.Cifrada {
		return false
	}
	_, ok := imagenIncrustada(n)
	return ok
}

// bajarImagenDeNota baja la imagen de la dirección de la nota, por el permiso puntual, y la
// guarda como su adjunto. Sustituye a la que hubiera de esa dirección SOLO si llega bien;
// esa deja la nota (y su imagen decodificada, la memoria), así que la compactación del
// almacén la quitará del disco. Si la que llega es la misma que ya estaba, no se escribe
// una versión nueva de la nota y se dice. accion es la clave del texto que encabeza el
// aviso de un fallo («cargar» o «recargar»).
func (p *Pantalla) bajarImagenDeNota(n Nota, accion string) {
	f := p.FicherosLocales
	if f == nil || f.BajarImagen == nil || p.bajandoImagen[n.ID] {
		return
	}
	if p.bajandoImagen == nil {
		p.bajandoImagen = map[string]bool{}
	}
	p.bajandoImagen[n.ID] = true
	p.Repintar()
	id, direccion := n.ID, n.URL
	loc := p.tr.Locale()
	p.iniciar(func() {
		base, _ := p.contextoTrabajo()
		ctx, cancelar := context.WithTimeout(base, plazoImagenRemota)
		defer cancelar()
		a, err := f.adjuntoDeDireccion(ctx, direccion, time.Now())
		var soltadas []Adjunto
		switch {
		case err != nil:
			if base.Err() == nil { // con la pantalla cerrada, nadie a quien decírselo
				p.avisar(ui.AvisoPeligro, textoImagenURL(loc, accion), p.textoFalloImagen(err, true))
			}
		case p.imagenDentroEs(id, a):
			p.avisar(ui.AvisoInfo, textoImagenURL(loc, "igual"), "")
		default:
			// cambiarAdjuntos y editar dicen ellos si no se puede guardar.
			err = p.cambiarAdjuntos(id, func(as []Adjunto) []Adjunto {
				soltadas = nil
				quedan := slices.DeleteFunc(slices.Clone(as), func(x Adjunto) bool {
					if x.Origen == direccion {
						soltadas = append(soltadas, x)
						return true
					}
					return false
				})
				return append(quedan, a)
			})
			if err != nil {
				soltadas = nil // la nota sigue con la de antes
			}
		}
		p.enHilo(func() {
			delete(p.bajandoImagen, id)
			for _, x := range soltadas {
				delete(p.imagenes, "adjunto:"+x.ID)
			}
			p.Repintar()
		})
	})
}

// imagenDentroEs dice si la nota id ya lleva dentro, de la misma dirección, esa imagen
// tal cual (los mismos bytes con el mismo tipo). Va fuera del hilo de la interfaz.
func (p *Pantalla) imagenDentroEs(id string, a Adjunto) bool {
	igual := false
	p.enHiloEsperar(func() {
		if n, ok := p.nota(id); ok {
			viejo, dentro := imagenIncrustada(n)
			igual = dentro && viejo.Origen == a.Origen && viejo.Type == a.Type && viejo.DataURL == a.DataURL
		}
	})
	return igual
}

// dialogoImagenURL es «Añadir una imagen de una dirección web…»: la dirección, la imagen
// que se baja al pulsar «Cargar la imagen» y, con ella dentro, la nota de imagen en el
// cuaderno abierto. Si no se puede, el campo dice por qué y no se crea nada.
func (p *Pantalla) dialogoImagenURL() {
	f := p.FicherosLocales
	if p.win == nil || f == nil || f.BajarImagen == nil {
		return
	}
	t := func(clave string) string { return textoImagenURL(p.tr.Locale(), clave) }
	v := p.variante()
	campo := ui.NuevaEntrada("https://")
	ok := ui.NuevoBoton(t("cargar"), nil)
	ok.Inerte = true
	cancelar := ui.NuevoBoton(p.tr.T("common.cancel", nil), nil).Con(ui.Fantasma, ui.TallaMD)
	cuerpo := container.New(ui.Columna{})
	var d *ui.Dialogo
	cerrado := false
	pintar := func(fallo string) {
		grupo := ui.GrupoCampo(t("campo"), false, campo, "", "")
		if fallo != "" {
			// Partido al ancho del cuerpo, como el de una pregunta: es una frase larga.
			grupo.Objects = append(grupo.Objects, ui.LineaErrorA(fallo, ui.AnchoCuerpoTalla(ui.DialogoSM, p.win.Canvas().Size())))
		}
		campo.Invalida = fallo != ""
		campo.Refresh()
		cuerpo.Objects = []fyne.CanvasObject{container.New(ui.Columna{Hueco: 3 * 0.25 * tema.Rem},
			ui.NuevoParrafo(t("pista"), tema.TextoBase, ui.Normal, "text-muted", v), grupo)}
		cuerpo.Refresh()
		if d != nil {
			d.Recolocar()
		}
	}
	campo.AlCambiar(func(s string) { ok.Inerte = strings.TrimSpace(s) == ""; ok.Refresh() })
	cuaderno := p.activo.ID
	enviar := func() {
		if ok.Inerte || ok.Cargando {
			return
		}
		direccion := strings.TrimSpace(campo.Texto())
		ok.Cargando = true
		ok.Refresh()
		p.iniciar(func() {
			base, _ := p.contextoTrabajo()
			ctx, fin := context.WithTimeout(base, plazoImagenRemota)
			defer fin()
			in, err := f.notaDeImagenRemota(ctx, direccion, time.Now())
			p.enHilo(func() {
				ok.Cargando = false
				ok.Refresh()
				if cerrado {
					return // se canceló mientras bajaba: no se crea nada
				}
				if err != nil {
					pintar(p.textoFalloImagen(err, true))
					return
				}
				cerrado = true
				d.Cerrar()
				p.lanzar(func() error { return p.crearEn(in, cuaderno, nil) })
			})
		})
	}
	ok.AlPulsar = enviar
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			enviar()
			return true
		}
		return false
	}
	pintar("")
	d = ui.NuevoDialogo(t("titulo"), cuerpo).Con(ui.DialogoSM, container.New(ui.Fila{Hueco: HuecoFilaDialogo}, cancelar, ok))
	d.Foco = campo
	// Cancelar, el aspa, Escape o el velo mientras baja: no se crea nada al llegar.
	d.AlCerrar = func() {
		cerrado = true
		d.Cerrar()
	}
	cancelar.AlPulsar = d.AlCerrar
	p.mostrar(d)
}

// cuerpoSinBajar es la tarjeta de una nota de imagen con dirección cuya imagen no está
// dentro, en Sytue Office: no se pide sola; se dice de dónde es y, si se puede escribir,
// «Cargar la imagen».
func (t *Tarjeta) cuerpoSinBajar(v tema.Variante) fyne.CanvasObject {
	host := t.nota.URL
	if u, err := url.Parse(t.nota.URL); err == nil && u.Hostname() != "" {
		host = strings.ToLower(u.Hostname())
	}
	loc := t.tr.Locale()
	piezas := []fyne.CanvasObject{
		ui.NuevoParrafo(fmt.Sprintf(textoImagenURL(loc, "sinBajar"), host), tema.TextoSM, ui.Normal, "text-muted", v),
	}
	if t.CargarImagen != nil {
		b := ui.NuevoBoton(textoImagenURL(loc, "cargar"), t.CargarImagen).Con(ui.Secundario, ui.TallaSM)
		b.Cargando = t.CargandoImagen
		piezas = append(piezas, container.New(ui.FilaAuto{Derecha: 1}, b))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, piezas...)
}
