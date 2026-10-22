// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// ErrSesionAjena impide presentar un refresh token a otro emisor o cliente OIDC.
var ErrSesionAjena = errors.New("auth: la sesión pertenece a otro emisor o cliente OIDC")

// ErrSesionNoDisponible exige olvidar también la sesión en memoria. Un broker
// perdido, bloqueado o rechazado nunca autoriza renovar con una copia antigua.
var ErrSesionNoDisponible = ajustes.ErrSesionNoDisponible

// AlmacenSesion es la entrada estable de una sesión nativa. El nombre depende sólo
// del emisor y cliente OIDC; nunca de la app, su versión ni su ruta de instalación.
// Los productos comparten el refresh cuando usan el mismo cliente público, sin
// compartirlo con otro entorno o con clientes de audiencias distintas.
type AlmacenSesion struct {
	emisor, cliente, cuenta string
}

func (o *OIDC) Almacen() *AlmacenSesion {
	emisor, cliente := strings.TrimRight(o.Issuer, "/"), o.clientID()
	suma := sha256.Sum256([]byte(emisor + "\x00" + cliente))
	return &AlmacenSesion{emisor: emisor, cliente: cliente, cuenta: fmt.Sprintf("sesion-%x", suma[:16])}
}

func (a *AlmacenSesion) pertenece(t Tokens) bool {
	return (t.Issuer == "" || strings.TrimRight(t.Issuer, "/") == a.emisor) &&
		(t.Cliente == "" || t.Cliente == a.cliente)
}

// Cargar no renueva ni abre un navegador. Los tokens sólo salen del almacén seguro
// de la plataforma, y nunca se aceptan si pertenecen a otro emisor o cliente.
func (a *AlmacenSesion) Cargar() (Tokens, error) {
	if t, compartida, err := cargarCompartida(a); compartida {
		if err == nil && t.Refresh != "" && (t.Issuer != a.emisor || t.Cliente != a.cliente) {
			return Tokens{}, errors.Join(ErrSesionNoDisponible, ErrSesionAjena)
		}
		return t, err
	}
	ruta, err := a.marcaCierre()
	if err != nil {
		return Tokens{}, err
	}
	if _, err := os.Stat(ruta); err == nil {
		return Tokens{}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Tokens{}, err
	}
	t, err := cargarCuenta(a.cuenta)
	if err == nil && t.Refresh != "" && (t.Issuer != a.emisor || t.Cliente != a.cliente) {
		return Tokens{}, ErrSesionAjena
	}
	return t, err
}

func (a *AlmacenSesion) marcaMigracion() (string, error) {
	dir, err := ajustes.Carpeta()
	return filepath.Join(dir, a.cuenta+".migrada"), err
}

func (a *AlmacenSesion) marcaCierre() (string, error) {
	dir, err := ajustes.Carpeta()
	return filepath.Join(dir, a.cuenta+".cerrada"), err
}

func (a *AlmacenSesion) marcarMigracion() error {
	ruta, err := a.marcaMigracion()
	if err != nil {
		return err
	}
	// Sólo un testigo sin secretos. Impide resucitar una copia por app después
	// del cierre compartido cuando otra app se actualiza más tarde.
	return ajustes.EscribirEntero(ruta, []byte("1\n"))
}

// Guardar es el callback de Session y Compartida. Los tokens y secretos permanecen
// en el llavero/Keystore; la marca compartida sólo avisa a los demás procesos.
func (a *AlmacenSesion) Guardar(t Tokens) error {
	if !a.pertenece(t) {
		return ErrSesionAjena
	}
	if compartida, err := guardarCompartida(a, t); compartida {
		return err
	}
	var errMarca error
	rutaCierre, errCierre := a.marcaCierre()
	if errCierre != nil {
		return errCierre
	}
	if t.Refresh == "" {
		// El cierre sigue siendo efectivo si el llavero no deja borrar. Una
		// copia antigua del refresh no vuelve a abrir la sesión en otra app.
		errCierre = ajustes.EscribirEntero(rutaCierre, []byte("1\n"))
		errMarca = a.marcarMigracion()
	} else {
		t.Issuer, t.Cliente = a.emisor, a.cliente
	}
	err := guardarCuenta(a.cuenta, t)
	if t.Refresh != "" && err == nil {
		errMarca = a.marcarMigracion()
		if errQuitar := os.Remove(rutaCierre); errQuitar != nil && !errors.Is(errQuitar, os.ErrNotExist) {
			errCierre = errQuitar
		}
	}
	ajustes.MarcarSesion()
	return errors.Join(err, errMarca, errCierre)
}

// Migrar conserva la sesión anterior al actualizar, una sola vez y bajo exclusión.
// Una entrada antigua sin emisor sólo se importa si quien configura la app sabe
// que éste es su entorno y cliente históricos. Nunca se prueba el token contra un
// servidor para adivinar a quién pertenece.
//
//nolint:contextcheck // Los callbacks del llavero no reciben contexto; su consulta D-Bus tiene plazo propio.
func (a *AlmacenSesion) Migrar(ctx context.Context, permitirSinEmisor bool) error {
	soltar, err := ajustes.CerrojoSesion(ctx)
	if err != nil {
		return err
	}
	defer soltar()
	if compartida, err := migrarCompartida(a, permitirSinEmisor); compartida {
		return err
	}
	t, err := a.Cargar()
	if err != nil {
		return err
	}
	if t.Refresh != "" {
		return nil
	}
	ruta, err := a.marcaMigracion()
	if err != nil {
		return err
	}
	if _, err = os.Stat(ruta); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	legada, err := cargar()
	if err != nil || legada.Refresh == "" {
		return err
	}
	if !a.pertenece(legada) || (legada.Issuer == "" && !permitirSinEmisor) {
		return nil
	}
	if err := a.Guardar(legada); err != nil {
		return err
	}
	return guardar(Tokens{})
}
