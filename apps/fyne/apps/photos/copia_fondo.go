// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// # La copia de seguridad con la app cerrada
//
// La misma ronda que con la app abierta (copia.go), lanzada por el sistema: en Android,
// cuando hay fotos o vídeos nuevos en la galería y cada pocas horas (TareaFondo, con la
// red que pidan los ajustes); en iOS, en la tarea de refresco que el sistema da a la app.
// La registra este `init`: en Android ese trabajo carga la biblioteca de la app sin
// correr su `main`. Lo que la app abierta y esta comparten está en el registro de la
// copia (CopiaDe), así que una de las dos espera a la otra.
//
// Solo con el dispositivo desbloqueado: la sesión de la suite no se abre con la pantalla
// bloqueada (sesion, tareas_fondo.go).
//
// # Con el Drive en la bóveda (ADR 0086)
//
// Con el Drive de la persona en la bóveda post-cuántica, las fotos se suben cifradas al
// Drive privado, y para eso hay que abrir la bóveda. La clave de desbloqueo es la de
// Sytue ID: va con la sesión en el almacén seguro del sistema (Keystore, Keychain) y la
// lee la puerta de la bóveda (cifrado.PuertaBoveda) de la App viva, que es quien sabe
// abrirla con su diario y sus arrendamientos. Así que:
//
//   - **Con la App de esta cuenta viva** (el proceso de Android abierto, o la tarea de
//     refresco de iOS) y la clave en el dispositivo ([App.ClaveEnDispositivo]), la ronda
//     le pide abrir el Drive cifrado por esa misma puerta (sin nadie mirando no hay nada
//     que preguntar: si la clave no abre, la puerta se queda esperando a la persona) y
//     sube cifrado en cuanto está abierto, dentro del plazo de la tarea.
//   - **Sin la App viva** (Android despertando solo la biblioteca) no se abre nada: abrir
//     la bóveda sin la Suite sería una segunda forma de manejar sus claves, fuera de la
//     puerta, y la clave nunca se copia ni se guarda en ningún otro sitio. La ronda no
//     sube nada, dice al sistema que no hay cambios y, si hay fotos esperando, AVISA una
//     vez al día («Abre Sytue Fotos para terminar la copia cifrada»): esperar en silencio
//     dejaba fotos sin copia sin que nadie lo supiera. Al abrir la app, la copia sigue en
//     cuanto el Drive cifrado está abierto ([App.DriveCifradoAbierto]).

func init() { sesion.RegistrarTareaEnFondo("photos", copiaEnFondo) }

// galeriaFondo y rutaFondo son las de verdad; las pruebas las cambian.
var (
	galeriaFondo GaleriaSistema = CarreteDelSistema{}
	rutaFondo                   = RutaRegistroCopia
)

// copiaEnFondo es la TareaEnFondo de Photos.
func copiaEnFondo(ctx context.Context, f sesion.EnFondo) sesion.Resultado {
	ruta, err := rutaFondo()
	if err != nil {
		return sesion.Fallo
	}
	c := CopiaDe(galeriaFondo, ruta)
	if !c.Ajustes().Activa {
		return sesion.SinCambios
	}
	d, err := destinoEnFondo(ctx, f)
	if errors.Is(err, ErrCopiaEsperaBoveda) {
		if avisar, _ := c.EsperaBoveda(ctx, f.Cuenta); avisar {
			avisoEsperaBoveda()
		}
	}
	if err != nil {
		return resultadoDeCopia(ProgresoCopia{}, err)
	}
	p, err := c.Ronda(ctx, f.Cuenta, d, nil)
	return resultadoDeCopia(p, err)
}

// ErrCopiaEsperaBoveda: el Drive de la persona está en la bóveda (ADR 0086) y este
// proceso no la tiene abierta. La ronda no sube nada al Drive del servidor —que lo
// rechazaría con 409 una y otra vez— y lo deja para la copia con la app abierta, que
// sí sube cifrado (o para una ronda en fondo con la app viva y el Drive desbloqueado).
var ErrCopiaEsperaBoveda = errors.New("photos: la copia espera a que se abra el Drive cifrado")

// appEnFondo es la App de este proceso que sabe abrir el Drive de la bóveda (la
// registra CambiarSesion). Con la app viva (Android con el proceso abierto, o la
// tarea de refresco de iOS), la ronda en fondo sube cifrado por ella.
var appEnFondo atomic.Pointer[App]

// destinoEnFondo es a dónde sube la ronda en fondo: el Drive del servidor mientras su
// fuente siga en `legacy`; con el Drive en la bóveda, el Drive privado de la App viva
// de esta misma cuenta, abierto por su puerta si la clave está en el dispositivo
// ([App.driveCifradoEnFondo]), y si no, [ErrCopiaEsperaBoveda] sin tocar nada.
func destinoEnFondo(ctx context.Context, f sesion.EnFondo) (*destinoDrive, error) {
	servidor := api.NewDrive(f.URL("drive"), f.Sesion, nil)
	fotos := api.NewPhotos(f.URL("photos"), f.Sesion, nil)
	scope, ok := scopeDeCuenta(f.Cuenta)
	if !ok {
		return nuevoDestinoDrive(servidor, fotos), nil
	}
	c, fin := ctxCorto(ctx)
	state, err := servidor.PrivateStorageState(c, scope)
	fin()
	if err != nil || state.Mode == "legacy" {
		// Sin saberlo (un servidor anterior, sin red), como siempre: la ronda
		// cuenta sus fallos y los reintenta.
		return nuevoDestinoDrive(servidor, fotos), nil //nolint:nilerr // ver arriba
	}
	if a := appEnFondo.Load(); a != nil && a.GaleriaCifrada != nil {
		if cuenta, _ := a.cuenta.Load().(string); cuenta == f.Cuenta {
			if privado := a.driveCifradoEnFondo(ctx); privado != nil {
				return nuevoDestinoDrive(privado, fotos), nil
			}
		}
	}
	return nil, ErrCopiaEsperaBoveda
}

// Lo que la ronda en fondo espera a que la puerta abra el Drive cifrado: dentro del
// plazo de la tarea, y mirando cada [pasoBovedaFondo]. Las pruebas los acortan.
var (
	esperaBovedaFondo = time.Minute
	pasoBovedaFondo   = 500 * time.Millisecond
)

// driveCifradoEnFondo es el Drive privado de esta App para la ronda en fondo: el ya
// abierto o, si está cerrado y la clave de desbloqueo está en el dispositivo, el que
// abre su puerta con ella. nil: sigue cerrado (sin clave, la clave no abre, o no da
// tiempo). Nunca toca la clave: solo pregunta si está ([App.ClaveEnDispositivo]).
func (a *App) driveCifradoEnFondo(ctx context.Context) *api.Drive {
	privado, err := a.GaleriaCifrada(ctx, false)
	if err == nil && privado != nil {
		return privado
	}
	if !errors.Is(err, ErrDriveCifradoCerrado) || a.ClaveEnDispositivo == nil || !a.ClaveEnDispositivo() {
		return nil
	}
	// Pedirlo con `abrir` es lo mismo que «Mostrar» en la galería: la puerta de la
	// bóveda de la App lo abre con la clave de la sesión, fuera de esta goroutine.
	if privado, err := a.GaleriaCifrada(ctx, true); err == nil && privado != nil {
		return privado
	} else if !errors.Is(err, ErrDriveCifradoCerrado) {
		return nil
	}
	c, fin := context.WithTimeout(ctx, esperaBovedaFondo)
	defer fin()
	t := time.NewTicker(pasoBovedaFondo)
	defer t.Stop()
	for {
		select {
		case <-c.Done():
			return nil
		case <-t.C:
		}
		privado, err := a.GaleriaCifrada(c, false)
		switch {
		case err == nil && privado != nil:
			return privado
		case !errors.Is(err, ErrDriveCifradoCerrado):
			return nil
		}
	}
}

// avisoEsperaBoveda avisa en el sistema de que la copia cifrada espera a que se abra
// la app. Las pruebas lo cambian.
var avisoEsperaBoveda = avisarEsperaBoveda

func avisarEsperaBoveda() {
	t, err := Traductor(i18n.Negotiate(sistema.Idioma(), ajustes.Leer().Idioma))
	if err != nil {
		return
	}
	titulo, cuerpo := t.T("backup.encryptedWaitTitle", nil), t.T("backup.encryptedWaitBody", nil)
	aviso := escritorio.Aviso{
		ID: "eu.sytue.photos", Nombre: "Sytue Photos", Esquema: "sytue-photos",
		Titulo: titulo, Cuerpo: cuerpo, Destino: escritorio.Destino{Origen: "photos", Ruta: "/"},
	}
	if app := fyne.CurrentApp(); app != nil {
		m := app.Metadata()
		if m.ID != "" {
			aviso.ID, aviso.Nombre = m.ID, m.Name
		}
		if e := m.Custom["esquema"]; e != "" {
			aviso.Esquema = e
		}
	}
	if escritorio.Avisar(aviso) != nil {
		if app := fyne.CurrentApp(); app != nil {
			sesion.AvisoDeRespaldo(app, titulo, cuerpo)
		}
	}
}

// scopeDeCuenta saca el ámbito del Drive de una identidad de cuenta
// ([sesion.IdentidadDeCuenta]: emisor, persona y organización).
func scopeDeCuenta(cuenta string) (storage.Scope, bool) {
	partes := strings.Split(cuenta, "\x00")
	if len(partes) != 3 || partes[1] == "" || partes[2] == "" {
		return storage.Scope{}, false
	}
	s := storage.Scope{Tenant: partes[2], Subject: partes[1], Product: "drive"}
	return s, s.Valid()
}

func ctxCorto(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 30*time.Second)
}

// resultadoDeCopia traduce cómo acabó una ronda a lo que se le dice al sistema: lo que hay
// que reintentar pronto (sin Wi-Fi, sin tiempo) es Aplazado; lo que no se arregla
// reintentando (sin permiso, sin galería, apagada) es SinCambios.
func resultadoDeCopia(p ProgresoCopia, err error) sesion.Resultado {
	switch {
	case err == nil && p.Subidas > 0:
		return sesion.Nuevos
	case err == nil, errors.Is(err, ErrCopiaEnMarcha), errors.Is(err, ErrCopiaEsperaBoveda),
		errors.Is(err, carrete.ErrSinPermiso), errors.Is(err, carrete.ErrNoDisponible):
		return sesion.SinCambios
	case errors.Is(err, ErrCopiaEsperaWifi), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return sesion.Aplazado
	default:
		return sesion.Fallo
	}
}
