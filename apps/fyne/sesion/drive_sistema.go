// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/documentos"
	"git.sytue.eu/sytue/one/apps/nucleo/nube"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

type estadoDriveSistema struct {
	identidad  auth.IdentitySnapshot
	habilitado bool
}

func (e estadoDriveSistema) dentro() bool {
	return e.habilitado && e.identidad.Subject != "" && e.identidad.Organization != ""
}

// controladorDriveSistema serializa el cambio de cuenta con la retirada anterior.
// No sondea ni inicia nuevas peticiones mientras no cambie la sesión/preferencia.
type controladorDriveSistema struct {
	leer            func() estadoDriveSistema
	conectar        func(context.Context, estadoDriveSistema) error
	retirar         func(context.Context, estadoDriveSistema) error
	inicio          func(estadoDriveSistema) error
	conservada      func(estadoDriveSistema) bool
	mu              sync.Mutex
	anterior        estadoDriveSistema
	iniciado, hecho bool
	cancelar        context.CancelFunc
	termino         chan struct{}
}

func (c *controladorDriveSistema) seguir(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Se lee AHORA: dos notificaciones de UI pueden llegar a estas goroutines
	// en distinto orden; ninguna lleva consigo una identidad que ya haya salido.
	e := c.leer()
	if c.iniciado && e == c.anterior {
		if c.cancelar != nil {
			return
		}
		if c.hecho && (c.conservada == nil || c.conservada(e)) {
			return
		}
	}
	if c.cancelar != nil {
		c.cancelar()
		<-c.termino
		c.cancelar, c.termino = nil, nil
	}
	if c.inicio != nil {
		if err := c.inicio(e); err != nil {
			slog.Warn("sesion: Drive no se ha podido actualizar en el inicio de Windows", "error", err)
		}
	}
	vieja := c.anterior
	c.anterior, c.iniciado, c.hecho = e, true, false
	if !e.dentro() {
		plazo, parar := context.WithTimeout(ctx, time.Minute)
		defer parar()
		if err := c.retirar(plazo, vieja); err != nil && !errors.Is(err, documentos.ErrNoDisponible) {
			slog.Warn("sesion: Drive no se ha podido retirar del sistema", "error", err)
			return
		}
		c.hecho = true
		return
	}
	vida, cancelar := context.WithCancel(ctx)
	termino := make(chan struct{})
	c.cancelar, c.termino = cancelar, termino
	go func() {
		err := c.conectar(vida, e)
		close(termino) // antes del mutex: seguir espera con el cerrojo cogido
		c.mu.Lock()
		if c.termino == termino {
			c.cancelar, c.termino = nil, nil
			c.hecho = err == nil
		}
		c.mu.Unlock()
		cancelar()
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, documentos.ErrNoDisponible) && !errors.Is(err, nube.ErrNoDisponible) {
			slog.Warn("sesion: Drive no se ha podido conectar al sistema", "error", err)
		}
	}()
}

func (s *Suite) iniciarDriveSistemaNoApple(ctx context.Context) {
	if s.app == nil {
		return
	}
	m := s.app.Metadata()
	if !strings.HasPrefix(m.ID, "eu.sytue.") || m.Name == "" || !api.EsProducto(strings.TrimPrefix(m.Custom["esquema"], "sytue-")) {
		return
	}
	switch runtime.GOOS {
	case "windows", "linux":
	case "android":
		// El proveedor y su almacén privado pertenecen al UID de Drive.
		if s.app.Metadata().Custom["esquema"] != "sytue-drive" {
			return
		}
	default:
		return
	}
	c := &controladorDriveSistema{
		leer:     func() estadoDriveSistema { return estadoDriveSistema{s.Sesion.SnapshotIdentity(), s.DriveEnSistema()} },
		conectar: s.conectarDriveSistema,
		inicio:   s.inicioDriveSistema,
		conservada: func(e estadoDriveSistema) bool {
			if !e.dentro() || runtime.GOOS == "windows" {
				return true
			}
			return documentos.CredencialActiva(identidadDocumentos(e.identidad, s.almacen.Identificador()), s.URL("drive")+"/dav/")
		},
		retirar: func(ctx context.Context, anterior estadoDriveSistema) error {
			if runtime.GOOS != "windows" {
				if err := documentos.Habilitar(false); err != nil {
					return err
				}
				if anterior.identidad.Subject == "" {
					return documentos.OlvidarCon(ctx)
				}
				cuenta := api.NewAccount(s.URL("id"), s.autorizacionDriveSistema(anterior.identidad), nil)
				return documentos.RetirarIdentidad(ctx, cuenta, identidadDocumentos(anterior.identidad, s.almacen.Identificador()))
			}
			soltar, err := cerrojoDriveSistema(ctx)
			if err != nil {
				return err
			}
			defer soltar()
			i := anterior.identidad
			if dir, ok := nube.Conectada(nube.Cuenta{Emisor: i.Issuer, Sub: i.Subject, Org: i.Organization}); ok {
				return nube.Desconectar(dir)
			}
			return nil
		},
	}
	s.AlCambiar(func(Estado) { go c.seguir(ctx) })
	s.AlVolver(func() { go c.seguir(ctx) })
	go c.seguir(ctx)
}

func identidadDocumentos(i auth.IdentitySnapshot, cuenta string) documentos.Identidad {
	return documentos.Identidad{Emisor: i.Issuer, Cliente: i.Client, Sujeto: i.Subject, Organizacion: i.Organization, CuentaSesion: cuenta}
}

// sesionDriveSistema impide que una petición de la conexión anterior adopte el
// token de la cuenta nueva, incluso antes de que llegue la notificación de UI.
type sesionDriveSistema struct {
	sesion    *auth.Session
	identidad auth.IdentitySnapshot
	vigente   func(context.Context) bool
}

func (s sesionDriveSistema) AccessToken(ctx context.Context) (string, error) {
	token, _, err := s.AccessTokenScoped(ctx)
	return token, err
}

func (s sesionDriveSistema) AccessTokenScoped(ctx context.Context) (string, *auth.AuthorizationScope, error) {
	token, alcance, err := s.sesion.AccessTokenScoped(ctx)
	if err != nil {
		return "", nil, err
	}
	if s.sesion.SnapshotIdentity() != s.identidad || (s.vigente != nil && !s.vigente(ctx)) || !alcance.Current("Bearer "+token) {
		return "", nil, nube.ErrOtraCuenta
	}
	return token, alcance, nil
}
func (s sesionDriveSistema) Invalidate(token string) { s.sesion.Invalidate(token) }

func (s *Suite) conectarDriveSistema(ctx context.Context, e estadoDriveSistema) error {
	fuente := s.autorizacionDriveSistema(e.identidad)
	if runtime.GOOS == "windows" {
		return conectarCarpetaDriveWindows(ctx, s, e, func(ctx context.Context, dir string, fuente sesionDriveSistema) error {
			return nube.Conectar(ctx, dir, api.NewDrive(s.URL("drive"), fuente, nil))
		})
	}
	if err := documentos.Habilitar(true); err != nil {
		return err
	}
	plazo, parar := context.WithTimeout(ctx, time.Minute)
	defer parar()
	return documentos.AsegurarIdentidad(plazo, api.NewAccount(s.URL("id"), fuente, nil), s.URL("drive")+"/dav/", "Sytue Drive · "+sistemaDriveEquipo(), identidadDocumentos(e.identidad, s.almacen.Identificador()))
}

func cuentaNube(i auth.IdentitySnapshot) nube.Cuenta {
	return nube.Cuenta{Emisor: i.Issuer, Sub: i.Subject, Org: i.Organization}
}

func conectarCarpetaDriveWindows(ctx context.Context, s *Suite, e estadoDriveSistema, conectar func(ctx context.Context, dir string, fuente sesionDriveSistema) error) error {
	// CFAPI admite una sola conexión por raíz. El mismo cerrojo permite
	// que otra app continúe al cerrar la que estaba atendiendo Windows.
	soltar, err := cerrojoDriveSistema(ctx)
	if err != nil {
		return err
	}
	defer soltar()
	if s.Sesion.SnapshotIdentity() != e.identidad || !s.DriveEnSistema() {
		return context.Canceled
	}
	dir, err := nube.CarpetaCon(ctx, cuentaNube(e.identidad))
	if err != nil {
		return err
	}
	return conectar(ctx, dir, s.autorizacionDriveSistema(e.identidad))
}

func cerrojoDriveSistema(ctx context.Context) (func(), error) {
	dir, err := ajustes.Carpeta()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "drive-sistema.cerrojo"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	return ajustes.CerrojoFichero(ctx, f)
}

func sistemaDriveEquipo() string {
	if runtime.GOOS == "android" {
		return "Android"
	}
	nombre, _ := os.Hostname()
	if strings.TrimSpace(nombre) == "" {
		return runtime.GOOS
	}
	return nombre
}

// CuentaDriveEnSistema captura la autorización de esta cuenta para una operación
// del proveedor. El cliente no adoptará la identidad del siguiente inicio de sesión.
func (s *Suite) CuentaDriveEnSistema() *api.Account {
	return api.NewAccount(s.URL("id"), s.autorizacionDriveSistema(s.Sesion.SnapshotIdentity()), nil)
}

func (s *Suite) autorizacionDriveSistema(i auth.IdentitySnapshot) sesionDriveSistema {
	return sesionDriveSistema{s.Sesion, i, func(ctx context.Context) bool {
		e := s.Estado()
		if !e.Dentro() {
			return false
		}
		if e.GeneracionSesion != "" && e.GeneracionSesion == ajustes.Leer().Sesion {
			return true
		}
		// La primera renovación puede haber cambiado la marca antes de que el
		// vigilante repinte la UI. Consultar el almacén existente distingue esa
		// renovación de un cierre o cambio de cuenta, sin una petición nueva.
		if s.almacen == nil {
			return false
		}
		t, _, err := s.leerSesionCompartida(ctx)
		return err == nil && t.Refresh != "" && t.Issuer == i.Issuer && t.Cliente == i.Client && t.Subject == i.Subject && t.Organizacion == i.Organization
	}}
}

func (s *Suite) inicioDriveSistema(e estadoDriveSistema) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	m := s.app.Metadata()
	if m.Custom["esquema"] != "sytue-drive" {
		// Otra app puede cerrar la sesión o desactivar Drive mientras el host
		// está cerrado: no debe volver a arrancar en el siguiente inicio del SO.
		if e.dentro() || !escritorio.ArrancaAlIniciarSesion("eu.sytue.drive") {
			return nil
		}
		m.ID = "eu.sytue.drive"
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return escritorio.AlIniciarSesion(m.ID, "Sytue Drive", exe, e.dentro())
}
