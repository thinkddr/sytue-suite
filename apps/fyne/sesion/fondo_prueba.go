// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"sync"
	"time"
)

// EscenaDePrueba es la plataforma de iOS para las pruebas: la app lanzada sin escena,
// como la lanza el sistema para la tarea de refresco del widget. Hace lo que hace el
// driver del fork (mobile.BackgroundDriver), sin UIKit.
type EscenaDePrueba struct {
	mu        sync.Mutex
	conectada bool
	esperan   []func()
	bloqueado bool
	cada      time.Duration
	refrescar func(context.Context) (bool, time.Duration)
	// El aviso silencioso (mobile.RemoteNotificationDriver, parche 0017): con token, la
	// app declara el modo de fondo y el sistema se lo da.
	tokenAvisos string
	avisar      func(context.Context) (bool, bool)
	pedidos     int
}

// ProbarSinEscena es SOLO para pruebas: deja esta suite como la lanza iOS para su
// widget, sin escena, antes de que los `cmd` y la barra pidan nada. Se llama justo
// después de Abrir (o de montar la suite a mano).
//
// Como en iOS, la presencia y el SSE de la barra siguen al primer plano (PausaEnFondo):
// ProbarPrimerPlano es la app que pasa al fondo o vuelve.
func (s *Suite) ProbarSinEscena() *EscenaDePrueba {
	e := &EscenaDePrueba{}
	s.mu.Lock()
	s.pausaEnFondo = true
	s.mu.Unlock()
	s.prepararFondo(e)
	return e
}

// ProbarPrimerPlano es SOLO para pruebas: la app pasa a primer plano (true) o sale de él
// (false), como cuando Fyne avisa a la sesión (engancharVida).
func (s *Suite) ProbarPrimerPlano(si bool) { s.repartir(si) }

// Conectar es la primera escena: corre lo que esperaba, en orden.
func (e *EscenaDePrueba) Conectar() {
	e.mu.Lock()
	e.conectada = true
	esperan := e.esperan
	e.esperan = nil
	e.mu.Unlock()
	for _, f := range esperan {
		f()
	}
}

// Bloquear es el dispositivo bloqueado (sin datos protegidos) o desbloqueado.
func (e *EscenaDePrueba) Bloquear(si bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.bloqueado = si
}

// Refrescar es el sistema lanzando la tarea de refresco: lo que la app contesta, o
// false y 0 si no se apuntó a ninguna.
func (e *EscenaDePrueba) Refrescar(ctx context.Context) (ok bool, reintento, cada time.Duration) {
	e.mu.Lock()
	f, cada := e.refrescar, e.cada
	e.mu.Unlock()
	if f == nil {
		return false, 0, 0
	}
	ok, reintento = f(ctx)
	return ok, reintento, cada
}

// LaunchedInBackground implementa conductorFondo.
func (e *EscenaDePrueba) LaunchedInBackground() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return !e.conectada
}

// OnFirstScene implementa conductorFondo.
func (e *EscenaDePrueba) OnFirstScene(f func()) {
	e.mu.Lock()
	if !e.conectada {
		e.esperan = append(e.esperan, f)
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()
	f()
}

// ProtectedDataAvailable implementa conductorFondo.
func (e *EscenaDePrueba) ProtectedDataAvailable() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return !e.bloqueado
}

// OnBackgroundRefresh implementa conductorFondo.
func (e *EscenaDePrueba) OnBackgroundRefresh(every time.Duration, f func(context.Context) (bool, time.Duration)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cada, e.refrescar = every, f
}

// ProbarAvisos es la app compilada con SYTUE_DESPERTAR_APPLE: el modo de fondo
// `remote-notification` declarado, y el sistema da `token` al registrarse.
func (e *EscenaDePrueba) ProbarAvisos(token string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tokenAvisos = token
}

// AvisoSilencioso es un aviso del Hub llegando por APNs: lo que la app contesta, o
// false y false si no se apuntó a ninguno.
func (e *EscenaDePrueba) AvisoSilencioso(ctx context.Context) (newData, ok bool) {
	e.mu.Lock()
	f := e.avisar
	e.mu.Unlock()
	if f == nil {
		return false, false
	}
	return f(ctx)
}

// PedidosDeToken es cuántas veces la app ha pedido el token al sistema.
func (e *EscenaDePrueba) PedidosDeToken() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pedidos
}

// RemoteNotificationsAvailable implementa conductorAvisos.
func (e *EscenaDePrueba) RemoteNotificationsAvailable() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tokenAvisos != ""
}

// RegisterForRemoteNotifications implementa conductorAvisos.
func (e *EscenaDePrueba) RegisterForRemoteNotifications(f func(token string, err error)) {
	e.mu.Lock()
	e.pedidos++
	tok := e.tokenAvisos
	e.mu.Unlock()
	go f(tok, nil)
}

// OnRemoteNotification implementa conductorAvisos.
func (e *EscenaDePrueba) OnRemoteNotification(f func(context.Context) (bool, bool)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.avisar = f
}
