// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// «Subir a Cast» desde Sytue Drive: DesdeDrive.tsx, la variante de /upload?drive=<id>.
// Es una COPIA explícita —el fichero sigue en Drive y la copia ocupa su espacio—, y lo
// que el servicio rechazaría (no es un vídeo, pasa de 8 GiB, no cabe, no se ve) se sabe
// al llegar, por `GET /videos:from-drive`, y no después de pulsar.

// Glifos de los estados de DesdeDrive (`lock` y `warning` de Icon.tsx).
const (
	glifoCandado = "M8 10V7a4 4 0 0 1 8 0v3M5 11a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-8ZM12 14.5v2.5"
	glifoAviso   = "M10.3 4.2 2.2 18a2 2 0 0 0 1.7 3h16.2a2 2 0 0 0 1.7-3L13.7 4.2a2 2 0 0 0-3.4 0ZM12 9.5v4.5M12 17.5h.01"
)

// MotivoDrive es por qué no se puede subir (`MotivoDeDrive` de lib/desde-drive.ts).
type MotivoDrive uint8

// Los motivos. El cero es «se puede».
const (
	DriveBien MotivoDrive = iota
	DriveSinConexion
	DriveSinPermiso
	DriveNoVideo
	DriveGrande
	DriveCuota
	DriveCaido
	DriveError
)

// motivoDrive traduce el fallo del servicio. «No existe» y «no puedes leerlo» se dicen
// igual: distinguirlos diría a quien no tiene permiso que el fichero existe.
func motivoDrive(err error) MotivoDrive {
	var e *api.Error
	if !errors.As(err, &e) {
		if estadoDeFallo(err) == SinConexion {
			return DriveSinConexion
		}
		return DriveError
	}
	switch e.Status {
	case http.StatusForbidden, http.StatusNotFound:
		return DriveSinPermiso
	case http.StatusRequestEntityTooLarge:
		return DriveGrande
	case http.StatusUnsupportedMediaType:
		return DriveNoVideo
	case http.StatusInsufficientStorage:
		return DriveCuota
	case http.StatusServiceUnavailable:
		return DriveCaido
	}
	return DriveError
}

// reintentable: solo tiene sentido volver a pedir lo mismo si el fallo no es del fichero.
func reintentable(m MotivoDrive) bool {
	return m == DriveCaido || m == DriveError || m == DriveSinConexion
}

var uuidDrive = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IDDeDrive es el fichero que pide el enlace: vacío si no es un identificador (el
// estado vacío, no una petición al servicio con cualquier cosa).
func IDDeDrive(s string) string {
	s = strings.TrimSpace(s)
	if !uuidDrive.MatchString(s) {
		return ""
	}
	return strings.ToLower(s)
}

// DesdeDrive son los datos de la pantalla.
type DesdeDrive struct {
	// ID vacío: el enlace no nombra un fichero.
	ID      string
	Fichero *api.CastDriveFile
	Motivo  MotivoDrive
	// Libres es lo que dijo el servicio al rechazar por cuota; -1 si no lo dijo.
	Libres int64
	// Titulo es el campo; vive en App.
	Titulo      *ui.Entrada
	Visibilidad string
	Enviando    bool
	// ErrorEnvio es el `role="alert"` de un envío rechazado.
	ErrorEnvio string
	// Calidades es el selector, con lo que ocupará cada una.
	Calidades SelCalidades

	AlVisibilidad         func(string)
	AlSubir, AlReintentar func()
	AlAbrirDrive, AlOtro  func()
}

// textoMotivo es la descripción de un motivo de error (`textoDelMotivo`).
func textoMotivo(v Vista, m MotivoDrive, libres int64) string {
	switch m {
	case DriveNoVideo:
		return v.Traduce("upload.error.tipo")
	case DriveGrande:
		return v.Traduce("upload.error.grande")
	case DriveCuota:
		if libres >= 0 {
			return v.Traduce("upload.error.cuotaQuedan", i18n.Vars{"available": Tamano(v, libres)})
		}
		return v.Traduce("upload.error.cuota")
	case DriveCaido:
		return v.Traduce("upload.drive.error.drive")
	case DriveSinConexion:
		return v.Traduce("errors.offline.title")
	case DriveSinPermiso:
		return v.Traduce("upload.drive.forbidden.description")
	case DriveBien, DriveError:
	}
	return v.Traduce("upload.drive.error.servicio")
}

// PantallaDesdeDrive monta «Subir desde Sytue Drive» en sus seis estados: vacío (el
// enlace no nombra un fichero), cargando, sin conexión, sin permiso, error (con el
// motivo y reintentar si tiene sentido) y el formulario.
func PantallaDesdeDrive(v Vista, d DesdeDrive) *fyne.Container {
	otro := ui.NuevoBoton(v.Traduce("upload.drive.other"), d.AlOtro).Con(ui.Fantasma, ui.TallaMD)
	bloques := []ui.Bloque{{Obj: Cabecera(v.Traduce("upload.drive.title"), "", otro), Margen: tema.Espacio(6)}}
	if v.Estado == SinSesion {
		return Pagina(v, append(bloques, ui.Bloque{Obj: CuerpoDeEstado(v, nil)})...)
	}
	abrirDrive := func() fyne.CanvasObject {
		return ui.NuevoBoton(v.Traduce("upload.drive.openDrive"), d.AlAbrirDrive).Con(ui.Secundario, ui.TallaMD)
	}
	var cuerpo fyne.CanvasObject
	switch {
	case d.ID == "":
		cuerpo = Vacio(v, glifoVideo, v.Traduce("upload.drive.empty.title"), v.Traduce("upload.drive.empty.description"), abrirDrive())
	case d.Motivo == DriveSinConexion && d.Fichero == nil:
		v.Estado = SinConexion
		cuerpo = CuerpoDeEstado(v, nil)
	case d.Motivo == DriveSinPermiso && d.Fichero == nil:
		cuerpo = Vacio(v, glifoCandado, v.Traduce("upload.drive.forbidden.title"), v.Traduce("upload.drive.forbidden.description"), abrirDrive())
	case d.Motivo != DriveBien && d.Fichero == nil:
		accion := abrirDrive()
		if reintentable(d.Motivo) {
			accion = ui.NuevoBoton(v.Traduce("errors.retry"), d.AlReintentar).Con(ui.Secundario, ui.TallaMD)
		}
		cuerpo = Vacio(v, glifoAviso, v.Traduce("upload.drive.error.title"), textoMotivo(v, d.Motivo, d.Libres), accion)
	case d.Fichero == nil:
		cuerpo = esqueletoDrive(v)
	default:
		cuerpo = formularioDrive(v, d)
	}
	return Pagina(v, append(bloques, ui.Bloque{Obj: cuerpo})...)
}

// esqueletoDrive es la tarjeta del fichero en esqueleto y, al lado, el hueco de la
// visibilidad.
func esqueletoDrive(v Vista) fyne.CanvasObject {
	barra := func(alto float32) fyne.CanvasObject {
		return container.New(&altoFijo{alto: alto}, ui.NuevoEsqueleto(0, 0))
	}
	tarjeta := Recuadro(v, "", ui.PilaBloques(
		ui.Bloque{Obj: barra(1.25 * tema.Rem), Margen: tema.Espacio(4)},
		ui.Bloque{Obj: barra(2.25 * tema.Rem), Margen: tema.Espacio(4)},
		ui.Bloque{Obj: barra(0.75 * tema.Rem)},
	))
	return enDosDrive(v, tarjeta, barra(16*tema.Rem))
}

// enDosDrive es `lg:grid-cols-[minmax(0,1fr)_20rem] gap-6`; debajo de lg, en columna.
func enDosDrive(v Vista, izq, der fyne.CanvasObject) fyne.CanvasObject {
	if v.Ventana < ui.CorteLG {
		return ui.PilaBloques(ui.Bloque{Obj: izq, Margen: tema.Espacio(6)}, ui.Bloque{Obj: der})
	}
	return container.New(&dosColumnas{derecha: 20 * tema.Rem}, izq, der)
}

// formularioDrive es el fichero (marca de Drive, nombre, tamaño y tipo), el título,
// la nota de que es una copia, el error del envío y «Subir a Cast»; al lado, quién
// podrá verlo.
func formularioDrive(v Vista, d DesdeDrive) fyne.CanvasObject {
	f := d.Fichero
	nombre := Parrafo(v, f.Name, tema.TextoBase, ui.SemiNegrita, "text")
	nombre.Lineas = 1
	datos := Parrafo(v, Tamano(v, f.SizeBytes)+" · "+f.ContentType, tema.TextoXS, ui.Normal, "text-muted")
	cabeza := container.New(ui.FilaAuto{Hueco: tema.Espacio(3), Derecha: 2, Elastico: 2, Partir: true},
		iconos.Marca(iconos.Drive, 32, v.Variante).Objeto(),
		ui.PilaBloques(ui.Bloque{Obj: nombre}, ui.Bloque{Obj: datos}))
	subir := ui.NuevoBoton(v.Traduce("upload.drive.submit"), d.AlSubir).Con(ui.Primario, ui.TallaMD)
	subir.Cargando = d.Enviando
	subir.Inerte = d.Enviando || d.Titulo == nil || strings.TrimSpace(d.Titulo.Texto()) == ""
	bloques := []ui.Bloque{
		{Obj: cabeza, Margen: tema.Espacio(5)},
		{Obj: ui.GrupoCampo(v.Traduce("upload.titleField"), true, d.Titulo, "", ""), Margen: tema.Espacio(5)},
		{Obj: Parrafo(v, v.Traduce("upload.drive.note"), tema.TextoSM, ui.Normal, "text-muted"), Margen: tema.Espacio(5)},
		{Obj: SelectorCalidades(v, d.Calidades), Margen: tema.Espacio(5)},
	}
	if d.ErrorEnvio != "" {
		caja := ui.NuevaCaja(tema.Color("danger-bg", v.Variante), transparente(), tema.RadioMD)
		alerta := container.New(&recuadroFino{caja: caja}, append(caja.Objetos(), Parrafo(v, d.ErrorEnvio, tema.TextoSM, ui.Normal, "danger-text"))...)
		bloques = append(bloques, ui.Bloque{Obj: alerta, Margen: tema.Espacio(5)})
	}
	bloques = append(bloques, ui.Bloque{Obj: Fila(subir)})
	return enDosDrive(v, Recuadro(v, "", ui.PilaBloques(bloques...)), SelectorVisibilidad(v, d.Visibilidad, d.AlVisibilidad))
}
