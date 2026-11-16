// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"log/slog"
	"strings"
	"time"

	// Los cuatro formatos que el servicio admite (`ValidatePhotoDataURI`): con PNG y
	// JPEG van GIF y WebP, que se registran en `image` al importarlos.
	_ "image/gif"
	_ "image/png"

	"fyne.io/fyne/v2"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/externo"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La foto de un contacto: `ContactPhoto` de ContactDetail.tsx, la del formulario y la
// del avatar de cada fila de la lista (`avatarSrc` de ContactsApp.tsx). La lista pide
// solo la de quien la tiene —el `has_photo` del listado, lo que la web lee de la PHOTO
// de cada vCard— y solo al pintarse su fila, como la `<img>` del navegador.

// Los errores de imagen de `avatarImage.ts`, cada uno con su clave.
var (
	errFotoFormato = errors.New("photo.errorFormat")
	errFotoProceso = errors.New("photo.errorProcess")
	errFotoGrande  = errors.New("photo.errorTooLarge")
)

// MaxFotoBytes es MAX_PHOTO_BYTES: los 256 KiB que el servicio admite.
const MaxFotoBytes = 256 << 10

// FotoDeFichero es `fileToAvatarDataUri`: el cuadrado del centro de la imagen escalado a
// 256 y en JPEG, bajando la calidad hasta que quepa, como data URI listo para subir.
func FotoDeFichero(datos []byte) (string, error) {
	img, err := externo.Decodificar(datos) // con el tope de píxeles (FYC-02)
	if err != nil {
		return "", errFotoFormato
	}
	b := img.Bounds()
	lado := min(b.Dx(), b.Dy())
	if lado <= 0 {
		return "", errFotoProceso
	}
	origen := image.Rect(b.Min.X+(b.Dx()-lado)/2, b.Min.Y+(b.Dy()-lado)/2, 0, 0)
	origen.Max = origen.Min.Add(image.Pt(lado, lado))
	destino := image.NewRGBA(image.Rect(0, 0, ui.LadoAvatarPredefinido, ui.LadoAvatarPredefinido))
	xdraw.CatmullRom.Scale(destino, destino.Bounds(), img, origen, xdraw.Src, nil)
	for _, calidad := range []int{85, 70, 55, 40} {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, destino, &jpeg.Options{Quality: calidad}); err != nil {
			return "", errFotoProceso
		}
		if buf.Len() <= MaxFotoBytes {
			return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
		}
	}
	return "", errFotoGrande
}

// fotoPredefinida es `presetAvatarDataUri` como data URI.
func fotoPredefinida(nombre, token string) (string, error) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	png, err := ui.AvatarPredefinido(nombre, tema.Color(token, v))
	if err != nil {
		return "", errFotoProceso
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

// ---- En la App ----

// Las fotos y solicitudes están vinculadas al ID y revisión, no al UID, que
// puede repetirse en libretas distintas. serial impide que una respuesta tardía
// reemplace otra solicitud de la misma revisión tras invalidar la caché.
type fotoGuardada struct {
	libreta string
	etag    string
	imagen  image.Image
}
type peticionFoto struct {
	libreta    string
	etag       string
	serial     uint64
	reintentar time.Time
}

const esperaReintentoFoto = 2 * time.Second

func debePedirFoto(p peticionFoto, libreta, etag string, ahora time.Time) bool {
	return p.libreta != libreta || p.etag != etag || (!p.reintentar.IsZero() && !ahora.Before(p.reintentar))
}

// fotoDe devuelve únicamente la imagen de la revisión visible de ese contacto.
func (a *App) fotoDe(c api.Contact) image.Image {
	if c.ID == "" || a.Pantalla.Activa == nil {
		return nil
	}
	foto, ok := a.fotos[c.ID]
	if !ok || foto.libreta != a.Pantalla.Activa.ID || foto.etag != c.ETag || (c.AddressBookID != "" && c.AddressBookID != foto.libreta) {
		return nil
	}
	return foto.imagen
}

func (a *App) aceptarFoto(id string, pedido peticionFoto, img image.Image, err error) bool {
	if actual, ok := a.fotosPedidas[id]; !ok || actual != pedido {
		return false
	}
	if img != nil {
		a.fotos[id] = fotoGuardada{libreta: pedido.libreta, etag: pedido.etag, imagen: img}
		return true
	}
	if err == nil {
		_, existed := a.fotos[id]
		delete(a.fotos, id)
		return existed
	}
	// No refrescar la lista aquí: un fallo repetido no debe generar un bucle.
	// La próxima presentación después del margen puede volver a intentarlo.
	pedido.reintentar = time.Now().Add(esperaReintentoFoto)
	a.fotosPedidas[id] = pedido
	return false
}

// fotoDeFila es la foto de una fila de la lista: la que ya está, pidiéndola si no ha
// llegado. Sin `has_photo`, las iniciales, sin pedir nada: miles de 404 si no.
func (a *App) fotoDeFila(c api.Contact) image.Image {
	if !c.HasPhoto {
		return nil
	}
	a.pedirFoto(c, a.enFondoFila)
	return a.fotoDe(c)
}

// traerFoto pide la foto del contacto de la ficha. Esta se pide aunque el listado no
// diga que la tiene: es una sola, y así la ficha la enseña también contra un servicio
// que todavía no manda `has_photo`.
func (a *App) traerFoto(c api.Contact) { a.pedirFoto(c, a.enFondo) }

// pedirFoto trae la foto si no se ha pedido ya para ESA versión de la tarjeta —el
// `?v=etag` con que la web la pide—, y al llegar repinta lo que la enseña.
//
//nolint:gocognit,gocyclo // la petición, su versión (`?v=etag`) y la vigencia de la sesión se comprueban en el mismo cierre.
func (a *App) pedirFoto(c api.Contact, lanzar func(func())) {
	if a.svc == nil || c.ID == "" || a.Pantalla.Activa == nil {
		return
	}
	libreta := a.Pantalla.Activa.ID
	if c.AddressBookID != "" && c.AddressBookID != libreta {
		return
	}
	if pedida, ok := a.fotosPedidas[c.ID]; ok && !debePedirFoto(pedida, libreta, c.ETag, time.Now()) {
		return
	}
	a.secuenciaFoto++
	pedido := peticionFoto{libreta: libreta, etag: c.ETag, serial: a.secuenciaFoto}
	a.fotosPedidas[c.ID] = pedido
	scope := a.capturar()
	lanzar(func() {
		if !a.vigente(scope) {
			return
		}
		// Each private projection reads the catalog. Limit concurrent reads;
		// queued work must leave promptly when the presenter is invalidated.
		select {
		case a.turnosFoto <- struct{}{}:
			defer func() { <-a.turnosFoto }()
		case <-scope.ctx.Done():
			return
		}
		if !a.vigente(scope) {
			return
		}
		ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
		defer fin()
		deadline, _ := ctx.Deadline()
		datos, err := a.svc.PhotoForContact(ctx, c)
		defer clear(datos)
		var img image.Image
		if err == nil && ctx.Err() == nil && datos != nil {
			// La foto la pone cualquiera de la libreta de la organización: con el
			// tope de píxeles (FYC-02).
			img, err = externo.Decodificar(datos)
		}
		if err != nil {
			slog.Info("contactos: la foto no ha llegado", "error", err)
		}
		finished := ctx.Err()
		a.enUIFoto(func() {
			if !a.vigente(scope) {
				return
			}
			// The catalog can change while the read is pending. Adopt only if
			// this exact contact and revision remain in the active book's view.
			actual := buscarPorID(a.todos, c.ID)
			if a.Pantalla.Activa == nil || a.Pantalla.Activa.ID != libreta || actual == nil || actual.ETag != c.ETag || actual.AddressBookID != c.AddressBookID || (actual.AddressBookID != "" && actual.AddressBookID != libreta) {
				if a.fotosPedidas[c.ID] == pedido {
					delete(a.fotosPedidas, c.ID)
				}
				return
			}
			if finished != nil || !time.Now().Before(deadline) {
				a.aceptarFoto(c.ID, pedido, nil, context.DeadlineExceeded)
				return
			}
			if !a.aceptarFoto(c.ID, pedido, img, err) {
				return
			}
			if s := a.Pantalla.Seleccionado; s != nil && s.ID == c.ID {
				a.repintar()
			} else {
				a.Pantalla.refrescarLista()
			}
		})
	})
}

// olvidarFoto hace que la próxima vez se vuelva a pedir.
func (a *App) olvidarFoto(id string) {
	delete(a.fotos, id)
	delete(a.fotosPedidas, id)
}

// ponerFoto sube una foto (data URI) al contacto, avisa y recarga.
func (a *App) ponerFoto(c api.Contact, dataURI string) {
	a.ponerFotoEn(a.capturar(), c, dataURI)
}

func (a *App) ponerFotoEn(scope cargaContacts, c api.Contact, dataURI string) {
	uid := api.CleanUID(c.UID)
	a.programarEn(scope, func(scope cargaContacts) {
		ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
		err := a.svc.SetPhoto(ctx, uid, dataURI)
		fin()
		fyne.Do(func() {
			if !a.vigente(scope) {
				return
			}
			if err != nil {
				slog.Warn("contactos: no se pudo poner la foto", "error", err)
				a.avisar(ui.AvisoPeligro, "photo.updateFailed", "")
				return
			}
			a.avisar(ui.AvisoExito, "photo.updated", "")
			a.olvidarFoto(c.ID)
			a.recargarActivaEn(scope)
		})
	})
}

// quitarFoto la quita, avisa y recarga.
func (a *App) quitarFoto(c api.Contact) {
	uid := api.CleanUID(c.UID)
	scope := a.capturar()
	a.programarEn(scope, func(scope cargaContacts) {
		ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
		err := a.svc.RemovePhoto(ctx, uid)
		fin()
		fyne.Do(func() {
			if !a.vigente(scope) {
				return
			}
			if err != nil {
				slog.Warn("contactos: no se pudo quitar la foto", "error", err)
				a.avisar(ui.AvisoPeligro, "photo.removeFailed", "common.tryAgainSoon")
				return
			}
			a.avisar(ui.AvisoExito, "photo.removed", "")
			a.olvidarFoto(c.ID)
			a.recargarActivaEn(scope)
		})
	})
}

// recargarActiva vuelve a traer la libreta que se ve (el `onChanged` de la ficha).
func (a *App) recargarActiva() { a.recargarActivaEn(a.capturar()) }

func (a *App) recargarActivaEn(scope cargaContacts) {
	if a.vigente(scope) && a.Pantalla.Activa != nil {
		a.recargarEn(scope, *a.Pantalla.Activa)
	}
}

// fotoDeUnFichero abre el selector de imágenes y la prepara; `listo` recibe el data
// URI, o nada si no se eligió o no se pudo leer (y entonces lo dice).
func (a *App) fotoDeUnFicheroEn(scope cargaContacts, listo func(dataURI string)) {
	if a.ElegirFichero == nil || !a.vigente(scope) {
		return
	}
	a.ElegirFichero([]string{".jpg", ".jpeg", ".png", ".webp", ".gif"}, func(datos []byte) {
		if !a.vigente(scope) {
			return
		}
		uri, err := FotoDeFichero(datos)
		if err != nil {
			a.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, a.Pantalla.t("photo.updateFailed", nil),
				a.Pantalla.t(err.Error(), nil)))
			return
		}
		listo(uri)
	})
}

// imagenDeURI decodifica un data URI de imagen (el que se va a subir) para enseñarlo ya.
func imagenDeURI(uri string) image.Image {
	_, datos, ok := strings.Cut(uri, ";base64,")
	if !ok {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(datos)
	if err != nil {
		return nil
	}
	img, err := externo.Decodificar(b) // con el tope de píxeles (FYC-02)
	if err != nil {
		return nil
	}
	return img
}
