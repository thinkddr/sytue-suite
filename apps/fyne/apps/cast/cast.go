// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package cast es Sytue Cast en Fyne: vídeo bajo demanda y directo, con paridad de
// pantallas con `apps/frontend/apps/cast` (ADR 0061, §10 y §11 del encargo).
//
// # Cómo está organizado
//
// Como Photos: cada pantalla es una FUNCIÓN que recibe sus datos ya resueltos y
// devuelve un `fyne.CanvasObject` (inicio.go, explorar.go, ver.go, subir.go,
// gestionar.go, emitir.go). App (app.go) guarda el estado, llama a services/cast por
// `apps/nucleo/api` y vuelve a construir la pantalla cuando algo cambia. Así los tests
// pintan cualquiera de los seis estados de cualquier pantalla sin red.
//
// Lo que se IMPORTA en vez de copiarse (CONVENTIONS §13.2): la pila de bloques, el
// alto a un ancho y la fila partible de `ui` (PilaBloques, Bloque, AltoA, AlAncho,
// FilaAuto); de `apps/photos`, solo su caché de imágenes (Fototeca) para los pósters;
// el reproductor es `video` y el directo es `llamada`.
package cast

import (
	"errors"
	"fmt"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Traductor es el de la app: el catálogo común más el de Cast, en ese orden.
func Traductor(l i18n.Locale) (*i18n.Translator, error) {
	return i18n.New(l, i18n.Common(), catalogos.Cast())
}

// Estado son los seis en los que puede estar una pantalla, más «sin sesión». El cero
// es el bueno, que es el que más se pinta.
type Estado uint8

// Los siete.
const (
	// Lista: hay datos y se enseñan.
	Lista Estado = iota
	// Cargando: se han pedido y no han llegado.
	Cargando
	// Vacia: han llegado y no hay ninguno. No es un fallo: tiene su texto y su acción.
	Vacia
	// Error: el servicio ha contestado que no.
	Error
	// SinConexion: no se ha podido ni preguntar.
	SinConexion
	// SinPermiso: la sesión no alcanza (403), o el vídeo no existe para quien pregunta
	// (el servicio contesta 404 a propósito: un 403 diría que existe).
	SinPermiso
	// SinSesion: nadie ha entrado y la pantalla la necesita. Se ofrece el botón.
	SinSesion
)

// Vista es el contexto común de las pantallas.
type Vista struct {
	T *i18n.Translator
	// Ventana es el ancho de la VENTANA: los cortes de la rejilla son media queries.
	Ventana  float32
	Variante tema.Variante
	Estado   Estado
	// Detalle es el texto del error, cuando el servicio lo da.
	Detalle string
	// Teca es la caché de pósters, por id de vídeo. nil: las tarjetas van sin imagen.
	Teca *photos.Fototeca
	// Ahora es el reloj de las fechas relativas; cero, time.Now.
	Ahora time.Time
	// Denuncias son las que hay sin resolver, para el contador del menú de quien
	// administra la organización. Cero: no se pinta.
	Denuncias int
	// Movil: la app corre en un teléfono. Cambia lo que no tiene sentido allí: el
	// reproductor va de borde a borde y la zona de subir no habla de arrastrar.
	Movil bool
	// AlEntrar es el botón del estado SinSesion.
	AlEntrar func()
	// AlReintentar es el botón de los estados de fallo. nil: no se ofrece.
	AlReintentar func()
}

// Traduce es el `t()` de la web.
func (v Vista) Traduce(clave string, vars ...i18n.Vars) string {
	if len(vars) > 0 {
		return v.T.T(clave, vars[0])
	}
	return v.T.T(clave, nil)
}

func (v Vista) ahora() time.Time {
	if v.Ahora.IsZero() {
		return time.Now()
	}
	return v.Ahora
}

// estadoDeFallo traduce un error del servicio a uno de los estados de la pantalla.
func estadoDeFallo(err error) Estado {
	var e *api.Error
	switch {
	case errors.Is(err, auth.ErrLoginRequired), errors.As(err, &e) && e.Status == 401:
		return SinSesion
	case errors.As(err, &e) && (e.Status == 403 || e.Status == 404):
		return SinPermiso
	case errors.As(err, &e):
		return Error
	}
	return SinConexion
}

// textoDeFallo es el aviso de un fallo que NO vacía la pantalla (una acción que no
// salió). Si el servicio explica por qué en su `message`, se dice eso: suele ser mejor
// que un «inténtalo de nuevo» («el título está vacío»).
func textoDeFallo(v Vista, err error) string {
	var e *api.Error
	switch estadoDeFallo(err) {
	case SinConexion:
		return v.Traduce("offline.description")
	case SinPermiso:
		return v.Traduce("forbidden.description")
	case SinSesion:
		return v.Traduce("account.signedOutDescription")
	default:
	}
	if errors.As(err, &e) && e.Status == 400 && e.Message() != "" {
		return e.Message()
	}
	return v.Traduce("error.connection")
}

// ---- Formatos ----

// Duracion es el «1:02:03» o «4:05» de la esquina de una tarjeta.
func Duracion(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// Tamano es un número de bytes en KB, MB o GB, con el separador del idioma.
func Tamano(v Vista, b int64) string { return i18n.FormatBytes(v.T.Locale(), float64(b)) }

// Hace es la fecha relativa: «hace 3 días».
func Hace(v Vista, t time.Time) string { return i18n.FormatTimeAgo(v.T.Locale(), t, v.ahora()) }

// Numero es un recuento con el separador de miles del idioma.
func Numero(v Vista, n int64) string { return i18n.FormatNumber(v.T.Locale(), float64(n)) }

// claveVisibilidad y claveEstado son los rótulos de cada valor.
func claveVisibilidad(vis string) string {
	switch vis {
	case api.CastPublic:
		return "visibility.public"
	case api.CastUnlisted:
		return "visibility.unlisted"
	case api.CastPeople:
		return "visibility.people"
	}
	return "visibility.org"
}

func claveEstado(s string) string {
	switch s {
	case api.CastStatusUploading:
		return "status.uploading"
	case api.CastStatusProcessing:
		return "status.processing"
	case api.CastStatusFailed:
		return "status.failed"
	case api.CastStatusScheduled:
		return "status.scheduled"
	case api.CastStatusLive:
		return "status.live"
	case api.CastStatusEnded:
		return "status.ended"
	}
	return "status.ready"
}

// tonoEstado es el color de la insignia de estado: el directo en peligro (rojo), el
// fallo también, lo que se está cociendo en aviso y lo programado en info.
func tonoEstado(s string) ui.Tono {
	switch s {
	case api.CastStatusLive, api.CastStatusFailed:
		return ui.InsigniaPeligro
	case api.CastStatusUploading, api.CastStatusProcessing:
		return ui.InsigniaAviso
	case api.CastStatusScheduled:
		return ui.InsigniaInfo
	}
	return ui.InsigniaNeutra
}

// varianteActual es la de la suite ahora mismo.
func varianteActual() tema.Variante { return tema.De(fyne.CurrentApp().Settings().ThemeVariant()) }
