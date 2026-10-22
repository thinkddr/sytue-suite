// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// SYTUE DRIVE EN EL FINDER Y EN ARCHIVOS, sin pasos a mano: con sesión, la app deja una
// contraseña de aplicación de este equipo en el llavero que comparte con su extensión File
// Provider y da de alta el dominio; sin sesión, lo deshace. La extensión trabaja después
// por su cuenta, con la app cerrada (apps/nucleo/sistema/archivos.go).
//
// La contraseña se revoca al cerrar sesión desde cualquiera de las quince
// (sesion.RevocarArchivos en Salir); si entonces no hubo red, la revoca aquí el
// siguiente inicio de sesión antes de crear otra, así que no se acumulan.

// nombreArchivos es el del dominio en la barra lateral: la marca, igual en los 26 idiomas.
const nombreArchivos = "Sytue Drive"

// archivos sigue la sesión. Los campos son el sistema y Sytue ID; las pruebas ponen los suyos.
type archivos struct {
	hay       func() (bool, error)
	guardar   func(sistema.CredencialArchivos) error
	registrar func(nombre string) error
	olvidar   func() error
	// sujeto es quien ha entrado; apuntada y apuntar, la contraseña apuntada en
	// sesion.CuentaArchivos y de quién es; revocar revoca la que hubiera apuntada.
	sujeto   func() string
	apuntada func() (id, sujeto string, err error)
	apuntar  func(id, sujeto string) error
	revocar  func(context.Context) error
	crear    func(ctx context.Context, etiqueta string) (api.AppPasswordNueva, error)
	webdav   func(context.Context) (string, error)
	correo   func(context.Context) (string, error)

	// Una vuelta cada vez: dos avisos de sesión seguidos no crean dos contraseñas. hecho
	// es lo último que salió bien: el aviso de sesión llega también por cambiar el tema.
	mu    sync.Mutex
	hecho string
}

// conectarArchivos lo monta sobre la sesión de la suite. Fuera de iOS y de la Mac App
// Store no hay extensión y no hace nada.
func conectarArchivos(s *sesion.Suite) {
	if !sistema.ArchivosDisponible() {
		return
	}
	cuenta := api.NewAccount(s.URL("id"), s.Sesion, nil)
	a := &archivos{
		hay: sistema.HayCredencialArchivos, guardar: sistema.GuardarCredencialArchivos,
		registrar: sistema.RegistrarArchivos, olvidar: sistema.OlvidarArchivos,
		sujeto: s.Sesion.Subject, apuntada: sesion.ArchivosApuntada, apuntar: sesion.ApuntarArchivos,
		revocar: func(ctx context.Context) error { return sesion.RevocarArchivos(ctx, cuenta) },
		crear:   cuenta.CreateAppPassword,
		webdav: func(ctx context.Context) (string, error) {
			c, err := api.CoordenadasDe(ctx, s.URL("id"), nil)
			return c.WebDAVURL, err
		},
		correo: func(ctx context.Context) (string, error) {
			yo, err := cuenta.Me(ctx)
			return yo.Email, err
		},
	}
	// Fuera del hilo de la interfaz: el alta espera a NSFileProviderManager y a la red.
	s.AlCambiar(func(e sesion.Estado) { go a.seguir(e.Dentro()) })
	go a.seguir(s.Estado().Dentro())
}

// seguir deja el proveedor como dice la sesión. Los fallos se apuntan y se reintenta en
// el siguiente aviso o arranque: la app funciona igual sin él.
func (a *archivos) seguir(dentro bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	quiero := "fuera"
	if dentro {
		quiero = "dentro " + a.sujeto()
	}
	if a.hecho == quiero {
		return
	}
	ctx, parar := context.WithTimeout(context.Background(), time.Minute)
	defer parar()
	if err := a.paso(ctx, dentro); err != nil {
		slog.Warn("drive: Sytue Drive no queda en el Finder ni en Archivos", "error", err)
		return
	}
	a.hecho = quiero
}

func (a *archivos) paso(ctx context.Context, dentro bool) error {
	hay, err := a.hay()
	if err != nil {
		return err
	}
	if !dentro {
		if !hay {
			return nil
		}
		return a.olvidar()
	}
	// La credencial vale si es de quien ha entrado ahora. Si no —otra persona cerró sesión
	// desde otra app, que no ve este llavero—, fuera, y una nueva.
	_, de, err := a.apuntada()
	if err != nil {
		return err
	}
	if hay && de != a.sujeto() {
		if err := a.olvidar(); err != nil {
			return err
		}
		hay = false
	}
	if !hay {
		if err := a.alta(ctx); err != nil {
			return err
		}
	}
	// También con la credencial ya puesta: tras reinstalar la app el dominio no está, y
	// volver a darlo de alta no duplica nada.
	return a.registrar(nombreArchivos)
}

// alta crea la contraseña y la deja en el llavero con lo demás que pide la extensión.
func (a *archivos) alta(ctx context.Context) error {
	// La que quedara de un cierre de sesión sin red, antes de crear otra.
	if err := a.revocar(ctx); err != nil {
		return fmt.Errorf("revocar la anterior: %w", err)
	}
	dav, err := a.webdav(ctx)
	if err != nil {
		return err
	}
	if dav == "" {
		return errors.New("drive: Sytue ID no anuncia la dirección WebDAV de Drive")
	}
	usuario, err := a.correo(ctx)
	if err != nil {
		return err
	}
	clave, err := a.crear(ctx, "Archivos · "+sistema.EtiquetaEquipo())
	if err != nil {
		return err
	}
	// Apuntada ANTES de guardarla: si lo de después falla, cerrar sesión la revoca igual.
	if err := a.apuntar(clave.ID, a.sujeto()); err != nil {
		return err
	}
	return a.guardar(sistema.CredencialArchivos{URL: dav, Usuario: usuario, Clave: clave.Password})
}
