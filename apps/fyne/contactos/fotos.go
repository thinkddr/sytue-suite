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

// fotoDe es la foto ya traída de ese contacto, o nil.
func (a *App) fotoDe(c api.Contact) image.Image { return a.fotos[api.CleanUID(c.UID)] }

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
func (a *App) pedirFoto(c api.Contact, lanzar func(func())) {
	uid := api.CleanUID(c.UID)
	if a.svc == nil || uid == "" {
		return
	}
	if etag, pedida := a.fotosPedidas[uid]; pedida && etag == c.ETag {
		return
	}
	a.fotosPedidas[uid] = c.ETag
	scope := a.capturar()
	lanzar(func() {
		ctx, fin := context.WithTimeout(scope.ctx, a.Plazo)
		datos, err := a.svc.Photo(ctx, uid)
		fin()
		var img image.Image
		if err == nil && datos != nil {
			// La foto la pone cualquiera de la libreta de la organización: con el
			// tope de píxeles (FYC-02).
			img, err = externo.Decodificar(datos)
		}
		if err != nil {
			slog.Info("contactos: la foto no ha llegado", "error", err)
		}
		fyne.Do(func() {
			if !a.vigente(scope) {
				return
			}
			switch {
			case img != nil:
				a.fotos[uid] = img
			case err == nil && a.fotos[uid] != nil:
				delete(a.fotos, uid) // ya no la tiene: se quitó en otro sitio
			default:
				return
			}
			if s := a.Pantalla.Seleccionado; s != nil && api.CleanUID(s.UID) == uid {
				a.repintar()
			} else {
				a.Pantalla.refrescarLista()
			}
		})
	})
}

// olvidarFoto hace que la próxima vez se vuelva a pedir.
func (a *App) olvidarFoto(uid string) {
	uid = api.CleanUID(uid)
	delete(a.fotos, uid)
	delete(a.fotosPedidas, uid)
}

// ponerFoto sube una foto (data URI) al contacto, avisa y recarga.
func (a *App) ponerFoto(c api.Contact, dataURI string) {
	uid := api.CleanUID(c.UID)
	a.enFondo(func() {
		ctx, fin := a.ctx()
		err := a.svc.SetPhoto(ctx, uid, dataURI)
		fin()
		fyne.Do(func() {
			if err != nil {
				slog.Warn("contactos: no se pudo poner la foto", "error", err)
				a.avisar(ui.AvisoPeligro, "photo.updateFailed", "")
				return
			}
			a.avisar(ui.AvisoExito, "photo.updated", "")
			a.olvidarFoto(uid)
			a.recargarActiva()
		})
	})
}

// quitarFoto la quita, avisa y recarga.
func (a *App) quitarFoto(c api.Contact) {
	uid := api.CleanUID(c.UID)
	a.enFondo(func() {
		ctx, fin := a.ctx()
		err := a.svc.RemovePhoto(ctx, uid)
		fin()
		fyne.Do(func() {
			if err != nil {
				slog.Warn("contactos: no se pudo quitar la foto", "error", err)
				a.avisar(ui.AvisoPeligro, "photo.removeFailed", "common.tryAgainSoon")
				return
			}
			a.avisar(ui.AvisoExito, "photo.removed", "")
			a.olvidarFoto(uid)
			a.recargarActiva()
		})
	})
}

// recargarActiva vuelve a traer la libreta que se ve (el `onChanged` de la ficha).
func (a *App) recargarActiva() {
	if a.Pantalla.Activa != nil {
		a.recargar(*a.Pantalla.Activa)
	}
}

// fotoDeUnFichero abre el selector de imágenes y la prepara; `listo` recibe el data
// URI, o nada si no se eligió o no se pudo leer (y entonces lo dice).
func (a *App) fotoDeUnFichero(listo func(dataURI string)) {
	if a.ElegirFichero == nil {
		return
	}
	a.ElegirFichero([]string{".jpg", ".jpeg", ".png", ".webp", ".gif"}, func(datos []byte) {
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
