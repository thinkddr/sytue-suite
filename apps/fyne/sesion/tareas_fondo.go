// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Trabajo de una app con la app cerrada
//
// Algunas apps tienen trabajo que hacer aunque nadie las abra: la copia de seguridad de
// Photos sube las fotos nuevas de la galería. Ese trabajo es una TareaEnFondo, que el
// paquete de la app registra en su `init` —en Android, el trabajo en segundo plano carga
// la biblioteca de la app y NO corre su `main`— y que corre por dos caminos:
//
//   - **Android**: un JobService (eu.sytue.nativo.TareaFondo) que la app programa con
//     [Suite.ProgramarTareaEnFondo] y que Android lanza cuando hay fotos nuevas en la
//     galería, o cada pocas horas, y solo con la red que pida la app. Entra por la JNI
//     (fondo_android.go) en [TareaSinVentana], que lee la sesión guardada como el refresco
//     del widget (leerSinVentana).
//   - **iOS**: la tarea de refresco que el sistema ya da a la app por su widget
//     (BGAppRefreshTask, fondo.go), tras publicar el widget y con el tiempo que quede.
//
// Las dos reglas del refresco del widget valen aquí: con el bloqueo de la suite puesto
// no se lee nada, y con el dispositivo bloqueado tampoco —la sesión de la suite no se
// abre con la pantalla bloqueada—: la tarea se aplaza. Con la app abierta en el mismo
// proceso, corre con la sesión de la app.

// EnFondo es lo que una TareaEnFondo recibe para hablar con los servicios.
type EnFondo struct {
	// Sesion es la de la cuenta abierta, que se renueva sola.
	Sesion api.Session
	// URL es la de cada servicio de la suite (api.ServiceURL del apex).
	URL func(servicio string) string
	// Cuenta identifica la cuenta como la identidad que la app usa con la ventana
	// abierta ([IdentidadDeCuenta]): lo que la tarea recuerde por cuenta es lo mismo.
	Cuenta string
}

// TareaEnFondo es el trabajo de una app sin la app. Corre en una goroutine y tiene
// el plazo de `ctx`, que el sistema acorta a su gusto: lo que no termine se retoma en la
// siguiente.
type TareaEnFondo func(ctx context.Context, f EnFondo) Resultado

var (
	tareasMu sync.Mutex
	tareas   = map[string]TareaEnFondo{}
	// tareaSinVentanaMu: una tarea sin ventana a la vez (el trabajo por fotos nuevas y el
	// periódico pueden coincidir).
	tareaSinVentanaMu sync.Mutex
)

// RegistrarTareaEnFondo registra la tarea de un producto (el esquema de su app sin
// «sytue-», como en FyneApp.toml). Va en el `init` del paquete de la app.
func RegistrarTareaEnFondo(producto string, t TareaEnFondo) {
	tareasMu.Lock()
	defer tareasMu.Unlock()
	tareas[producto] = t
}

func tareaDe(producto string) (TareaEnFondo, bool) {
	tareasMu.Lock()
	defer tareasMu.Unlock()
	t, ok := tareas[producto]
	return t, ok && t != nil
}

// IdentidadDeCuenta es la clave de una cuenta en este dispositivo: el emisor (la URL de
// Sytue ID), la persona y su organización. Una renovación de tokens no la cambia; otra
// cuenta u otra organización, sí.
func IdentidadDeCuenta(urlID, sujeto, organizacion string) string {
	return urlID + "\x00" + sujeto + "\x00" + organizacion
}

// IdentidadCuenta es [IdentidadDeCuenta] de la sesión de esta Suite; vacía sin sesión.
func (s *Suite) IdentidadCuenta(e Estado) string {
	if !e.Dentro() || s.Sesion == nil {
		return ""
	}
	return IdentidadDeCuenta(s.URL("id"), e.Sujeto, s.Sesion.Organizacion())
}

// TareaSinVentana es la entrada de Android: corre la tarea de `o.Producto` leyendo la
// sesión guardada, sin Suite ni ventana. Con la app abierta en este proceso, con la suya.
func TareaSinVentana(ctx context.Context, o SinVentana) Resultado {
	t, ok := tareaDe(o.Producto)
	if !ok || !bundleSuite.MatchString(o.Bundle) {
		return SinCambios
	}
	if s := suiteViva.Load(); s != nil {
		return s.TareaEnFondo(ctx)
	}
	if !tareaSinVentanaMu.TryLock() {
		return SinCambios // ya hay una en curso
	}
	defer tareaSinVentanaMu.Unlock()
	leida, r := leerSinVentana(ctx)
	if leida.ses == nil {
		return r
	}
	return t(ctx, EnFondo{
		Sesion: leida.ses,
		URL:    urlSinVentana,
		Cuenta: IdentidadDeCuenta(urlSinVentana("id"), leida.ses.Subject(), leida.ses.Organizacion()),
	})
}

// TareaEnFondo corre la tarea de esta app con la sesión de la Suite: la de iOS en su
// tarea de refresco, y la de Android con el proceso ya vivo.
func (s *Suite) TareaEnFondo(ctx context.Context) Resultado {
	if s.app == nil {
		return SinCambios
	}
	t, ok := tareaDe(s.productoWidget())
	if !ok {
		return SinCambios
	}
	// Las mismas puertas que RefrescarWidgetEnFondo: datos protegidos, bloqueo, sesión.
	if !s.conductor().ProtectedDataAvailable() {
		return Aplazado
	}
	if bloqueoPuesto(ajustes.Leer()) {
		return SinCambios
	}
	cuenta := s.IdentidadCuenta(s.Estado())
	if cuenta == "" {
		return SinCambios
	}
	return t(ctx, EnFondo{Sesion: s.Sesion, URL: s.URL, Cuenta: cuenta})
}

// ProgramarTareaEnFondo dice al sistema cuándo despertar a esta app para su tarea: solo
// Android lo necesita (TareaFondo, tareas_fondo_android.go); en iOS la lanza la tarea de
// refresco, y en el escritorio no hay app cerrada que despertar. `conDatos`: también con
// datos móviles; si no, solo con una red que no sea de pago.
func (s *Suite) ProgramarTareaEnFondo(activa, conDatos bool) {
	if s.app == nil {
		return
	}
	programarTareaEnFondo(s.productoWidget(), activa, conDatos)
}
