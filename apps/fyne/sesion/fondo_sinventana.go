// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"io/fs"
	"log/slog"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets/fuentes"
)

// # Sin ventana: Android despierta la app para su widget
//
// En Android no hay BGAppRefreshTask ni push sin Google (ADR 0078, enmienda del
// 2026-11-06, pieza 6). Un JobService (eu.sytue.nativo.RefrescoWidgets) carga la
// biblioteca de la app con System.loadLibrary —lo mismo que hace GoNativeActivity antes
// de crear la actividad— y llama a una entrada JNI (fondo_android.go). Cargar la
// biblioteca inicializa Go y corre los `init`, pero NO el `main`: no hay fyne.App, ni
// Suite, ni ventana. RefrescarSinVentana es lo que hace esa entrada:
//
//   - con la app abierta en este mismo proceso (su Suite ha arrancado), se le avisa a su
//     publicador, que ya sigue al servidor, y no se lee nada aquí;
//   - si no, lee la sesión guardada como Abrir (leerSesionGuardada, con el cliente de
//     Sytue ID del móvil), construye la Fuente registrada del producto
//     (fuentes.Registrada, la misma que monta el `cmd` con Suite.FuenteWidget) y publica
//     UNA ronda con un Publicador que se para al acabar;
//   - ni presencia, ni SSE, ni Hub, ni avisos: solo las lecturas de la Fuente y, si
//     hace falta, la renovación del token;
//   - sin widget de esta app, sin sesión o con el bloqueo de la suite, no lee nada. El
//     dispositivo bloqueado lo mira antes el JobService (no llama a Go).

// SinVentana es lo que la plataforma le da al refresco sin ventana.
type SinVentana struct {
	// Bundle es el paquete de la app (`eu.sytue.calendar`).
	Bundle string
	// Producto es su esquema sin «sytue-» (`calendar`, `sheets`, `one`).
	Producto string
	// Zona es la del sistema. Sin el `main`, nadie ha puesto time.Local (en Android Go
	// arranca en UTC y es el fork, al crear la actividad, quien lo cambia), y «hoy»
	// sería el de Greenwich.
	Zona *time.Location
}

var (
	// suiteViva es la Suite que ha arrancado en este proceso: la app está abierta.
	suiteViva atomic.Pointer[Suite]
	// sinVentanaMu: un refresco sin ventana a la vez (el periódico y el del relevo son
	// dos trabajos distintos para Android y pueden coincidir).
	sinVentanaMu sync.Mutex
	// apexSinVentana es el de Abrir(a, ""), con el que abren los quince `cmd`.
	apexSinVentana = api.ApexProd
	// fabricaSinVentana y urlSinVentana son los de producción; las pruebas los cambian.
	fabricaSinVentana = fuentes.Registrada
	urlSinVentana     = func(servicio string) string { return api.ServiceURL(apexSinVentana, servicio) }
	renovarSinVentana = func(o *auth.OIDC) auth.Renewer { return o.Renew }
)

var bundleSuite = regexp.MustCompile(`^eu\.sytue\.[a-z][a-z0-9]*$`)

// RefrescarSinVentana publica el widget de esta app sin la app (ver arriba). Es la
// entrada de todo lo que despierta a Android por su widget: el refresco periódico y el
// relevo de la señal de cambios que hace One. Vuelve como tarde a widgets.PlazoWidget.
func RefrescarSinVentana(ctx context.Context, o SinVentana) Resultado {
	if s := suiteViva.Load(); s != nil {
		// La app está abierta: su Publicador ya lee cuando hace falta (y con su
		// antirrebote, que agrupa esto con la señal que le llega por su propio SSE).
		s.AvisoDelHub(s.productoWidget())
		return SinCambios
	}
	fabrica, ok := fabricaSinVentana(o.Producto)
	if !ok || !bundleSuite.MatchString(o.Bundle) || !widgets.Disponible() {
		return SinCambios
	}
	if !sinVentanaMu.TryLock() {
		return SinCambios // ya hay uno en curso: publica lo mismo
	}
	defer sinVentanaMu.Unlock()
	// Con el bloqueo de la suite puesto no se lee: sin interfaz no se le puede pedir al
	// dueño que diga quién es (como en iOS, RefrescarWidgetEnFondo).
	e := ajustes.Leer()
	if bloqueoPuesto(e) {
		return SinCambios
	}
	oidc := oidcDe(apexSinVentana, true)
	almacen := oidc.Almacen()
	cargar := func() (auth.Tokens, error) { return cargarAlmacen(almacen) }
	guardados, marcas, err := leerSesionGuardada(ctx, cargar, almacen, true)
	if err != nil {
		slog.Info("sesion: sin ventana, la sesión guardada no se deja leer", "error", err)
		return Fallo
	}
	if guardados.Subject == "" {
		return SinCambios
	}
	ses := auth.NewSession(guardados, auth.Compartida(renovarSinVentana(oidc), cargar, almacen.Guardar), almacen.Guardar)
	fuente := fabrica(comunesSinVentana(o, e), urlSinVentana, ses)
	vista := widgets.NewView(o.Bundle, o.Producto, widgets.Capture(marcas.epoca))
	pub := widgets.NuevoPublicador(widgets.OpcionesPublicador{
		Vista:  func() widgets.View { return vista },
		Fuente: fuente,
	})
	// Una ronda y nada más: sin latido ni medianoche en un proceso que nadie mira.
	defer pub.Pausar()
	antes, _ := widgets.Read(o.Bundle)
	ctx, parar := context.WithTimeout(ctx, widgets.PlazoWidget)
	defer parar()
	return resultadoDeRonda(o.Bundle, antes, pub.RefrescarYa(ctx))
}

// comunesSinVentana son los Comunes de Suite.ComunesWidget sin Suite: el idioma de la
// suite (lo elegido y, si no, el del sistema), los catálogos que carga el `cmd` del
// producto y la zona del sistema.
func comunesSinVentana(o SinVentana, e ajustes.Estado) fuentes.Comunes {
	idioma := i18n.Negotiate(sistema.Idioma(), e.Idioma)
	zona := o.Zona
	if zona == nil {
		zona = time.Local
	}
	c := fuentes.Comunes{
		Zona:   func() *time.Location { return zona },
		Ahora:  time.Now,
		Idioma: func() i18n.Locale { return idioma },
	}
	if t, err := i18n.New(idioma, append([]fs.FS{i18n.Common()}, catalogos.DeProducto(o.Producto)...)...); err == nil {
		c.T = func(clave string) string { return t.T(clave, nil) }
	}
	return c
}

// ejecutorJava es la forma de sistema.EnAndroid.
type ejecutorJava = func(f func(vm, env, ctx uintptr) error) error

// conRespaldo es la puerta a Java del refresco sin ventana: la de la actividad si la
// hay (el toolkit la da con driver.RunNative en cuanto Android crea la actividad), y si
// no, la propia, con la máquina y el contexto de la app que dio la entrada JNI. Se
// pregunta en cada llamada: la actividad puede crearse con el proceso ya despierto, y
// desde entonces lo que pide una actividad (un diálogo, el selector) tiene que ir a ella.
func conRespaldo(actividad, propia ejecutorJava) ejecutorJava {
	return func(f func(vm, env, ctx uintptr) error) error {
		if actividad != nil && actividad(func(_, _, _ uintptr) error { return nil }) == nil {
			return actividad(f)
		}
		return propia(f)
	}
}

// zonaDe es la zona del sistema que da Android (`TimeZone.getDefault()`): la de nombre
// IANA si Go la encuentra (en Android, en el tzdata del sistema) y, si no, la fija del
// desfase de ahora, que sin el cambio de hora da al menos el día correcto.
func zonaDe(nombre string, desfaseSegundos int) *time.Location {
	if nombre != "" && nombre != "Local" {
		if z, err := time.LoadLocation(nombre); err == nil {
			return z
		}
	}
	return time.FixedZone(nombre, desfaseSegundos)
}
