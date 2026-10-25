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
	preparar  func()
	hay       func() (bool, error)
	guardar   func(sistema.CredencialArchivos) error
	registrar func(nombre string) error
	olvidar   func() error
	// sujeto es quien ha entrado; apuntada y apuntar, la contraseña apuntada en
	// sesion.CuentaArchivos y de quién es; revocar revoca la que hubiera apuntada.
	sujeto       func() string
	habilitado   func() bool
	dentro       func() bool
	organizacion func() string
	apuntada     func() (id, sujeto string, err error)
	apuntar      func(id, sujeto string) error
	revocar      func(context.Context) error
	crear        func(ctx context.Context, etiqueta string) (api.AppPasswordNueva, error)
	revocarNueva func(context.Context, string) error
	webdav       func(context.Context) (string, error)
	correo       func(context.Context) (string, error)

	// Una vuelta cada vez: dos avisos de sesión seguidos no crean dos contraseñas. hecho
	// es lo último que salió bien: el aviso de sesión llega también por cambiar el tema.
	mu    sync.Mutex
	hecho string
}

// conectarArchivos sigue la sesión y la elección local. Sólo el host firmado que
// incorpora File Provider puede guardar su credencial o registrar el dominio.
func conectarArchivos(s *sesion.Suite) *archivos {
	if !sistema.ArchivosDisponible() {
		return nil
	}
	cuenta := api.NewAccount(s.URL("id"), s.Sesion, nil)
	a := &archivos{
		hay: sistema.HayCredencialArchivos, guardar: sistema.GuardarCredencialArchivos,
		registrar: sistema.RegistrarArchivos, olvidar: sistema.OlvidarArchivos,
		sujeto:       func() string { return s.Sesion.SnapshotIdentity().IdentidadDrive() },
		habilitado:   s.DriveEnSistema,
		dentro:       func() bool { return s.Estado().Dentro() },
		organizacion: s.Sesion.Organizacion,
		apuntada:     sesion.ArchivosApuntada, apuntar: sesion.ApuntarArchivos,
		revocar: func(ctx context.Context) error { return sesion.RevocarArchivos(ctx, cuenta) },
		crear:   cuenta.CreateAppPassword, revocarNueva: cuenta.RevokeAppPassword,
		webdav: func(ctx context.Context) (string, error) {
			c, err := api.CoordenadasDe(ctx, s.URL("id"), nil)
			return c.WebDAVURL, err
		},
		correo: func(ctx context.Context) (string, error) {
			yo, err := cuenta.Me(ctx)
			return yo.Email, err
		},
	}
	// Cada operación queda ligada a la misma identidad, aunque la sesión compartida
	// cambie mientras hay una petición a Sytue ID en vuelo.
	a.preparar = func() {
		cuenta := s.CuentaDriveEnSistema()
		a.crear, a.revocarNueva = cuenta.CreateAppPassword, cuenta.RevokeAppPassword
		a.revocar = func(ctx context.Context) error { return sesion.RevocarArchivos(ctx, cuenta) }
		a.correo = func(ctx context.Context) (string, error) { yo, err := cuenta.Me(ctx); return yo.Email, err }
	}
	// Fuera del hilo de la interfaz: el alta espera a NSFileProviderManager y a la red.
	s.AlCambiar(func(e sesion.Estado) { go func() { _ = a.seguir(e.Dentro()) }() })
	go func() { _ = a.seguir(s.Estado().Dentro()) }()
	return a
}

// seguir deja el proveedor como dice la sesión. Los fallos se apuntan y se reintenta en
// el siguiente aviso o arranque: la app funciona igual sin él.
func (a *archivos) seguir(dentro bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.preparar != nil {
		a.preparar()
	}
	// Un aviso viejo que esperó al cerrojo no puede volver a abrir la cuenta anterior.
	if a.dentro != nil {
		dentro = a.dentro()
	}
	habilitado := a.habilitado == nil || a.habilitado()
	quiero := "fuera"
	if !habilitado {
		quiero = "desactivado"
	}
	if dentro && habilitado {
		quiero = "dentro " + a.sujeto()
	}
	if a.hecho == quiero {
		return nil
	}
	ctx, parar := context.WithTimeout(context.Background(), time.Minute)
	defer parar()
	if err := a.paso(ctx, dentro); err != nil {
		slog.Warn("drive: Sytue Drive no queda en el Finder ni en Archivos", "error", err)
		return err
	}
	a.hecho = quiero
	return nil
}

// abrir se une al alta en vuelo, o la reintenta. El enlace desde Hub no puede
// adelantarse al registro del dominio. Sólo se llama desde una goroutine de UI.
func (a *archivos) abrir(abrir func() error) error {
	if (a.dentro != nil && !a.dentro()) || (a.habilitado != nil && !a.habilitado()) {
		return errors.New("drive: la integración del sistema no está habilitada")
	}
	if err := a.seguir(true); err != nil {
		return err
	}
	a.mu.Lock()
	valida := a.hecho == "dentro "+a.sujeto() && (a.dentro == nil || a.dentro()) && (a.habilitado == nil || a.habilitado())
	a.mu.Unlock()
	if !valida {
		return errors.New("drive: la sesión cambió antes de abrir el sistema")
	}
	return abrir()
}

func (a *archivos) paso(ctx context.Context, dentro bool) error {
	hay, err := a.hay()
	if err != nil {
		return err
	}
	habilitado := a.habilitado == nil || a.habilitado()
	if !dentro || !habilitado {
		// La baja también se pide sin credencial: una instalación anterior pudo
		// registrar el dominio antes de que el llavero quedara accesible.
		err := a.olvidar()
		if dentro && !habilitado {
			err = errors.Join(err, a.revocar(ctx))
		}
		return err
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
	identidad := a.sujeto()
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
	// Apuntada ANTES de validarla y guardarla: incluso una respuesta antigua o
	// incompleta debe poder revocarse sin publicar la clave en el proveedor.
	if err := a.apuntar(clave.ID, identidad); err != nil {
		if a.revocarNueva != nil {
			err = errors.Join(err, a.revocarNueva(ctx, clave.ID))
		}
		return err
	}
	// Una respuesta de la cuenta anterior o sin tenant verificable nunca llega al SO.
	cambio := a.sujeto() != identidad || (a.dentro != nil && !a.dentro()) || (a.habilitado != nil && !a.habilitado())
	sinOrganizacion := a.organizacion != nil && (clave.OrgID == "" || clave.Tenant != a.organizacion())
	if cambio || sinOrganizacion {
		return errors.Join(errors.New("drive: la identidad de Archivos no coincide con la sesión"), a.revocar(ctx))
	}
	if err := a.guardar(sistema.CredencialArchivos{URL: dav, Usuario: usuario, Clave: clave.Password, Identidad: identidad}); err != nil {
		return errors.Join(err, a.revocar(ctx))
	}
	return nil
}
