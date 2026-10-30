// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package documentos

import (
	"context"
	"errors"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

var leerAccesoApertura = ajustes.LeerAccesoDrive

var errAperturaAjena = errors.New("documentos: la ubicación no pertenece a la autorización mostrada")

// Apertura conserva una URI y un dueño ya resueltos, nunca la contraseña. Mantiene
// la exclusión del proveedor hasta Abrir/Close: otro montaje no puede reutilizar
// la misma URI de GVfs para una organización distinta mientras se abre ésta.
type Apertura struct {
	uri       string
	dueno     Identidad
	acceso    ajustes.AccesoDrive
	abrir     func(context.Context, string) error
	soltar    func()
	cerrar    sync.Once
	mu        sync.Mutex
	consumida bool
}

// PrepararApertura resuelve la ubicación de un proveedor existente para la cuenta
// mostrada. Quien recibe un destino debe llamar Close, incluso si no llega a abrirlo.
func PrepararApertura(ctx context.Context, dueno Identidad, url string) (*Apertura, error) {
	return prepararApertura(ctx, dueno, url)
}

func comprobarDuenoApertura(ctx context.Context, dueno Identidad) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	i := auth.RequestAuthorizationIntent(ctx)
	if i == nil || !i.Current() {
		return auth.ErrAuthorizationChanged
	}
	esperada := i.Identity()
	if !dueno.completa() || dueno.CuentaSesion == "" || dueno.Emisor != esperada.Issuer || dueno.Cliente != esperada.Client ||
		dueno.Sujeto != esperada.Subject || dueno.Organizacion != esperada.Organization {
		return errAperturaAjena
	}
	return nil
}

func accesoDeApertura(ctx context.Context, dueno Identidad) (ajustes.AccesoDrive, error) {
	a, err := leerAccesoApertura()
	if err != nil {
		return ajustes.AccesoDrive{}, err
	}
	if err := comprobarDuenoApertura(ctx, dueno); err != nil {
		return ajustes.AccesoDrive{}, err
	}
	i := auth.RequestAuthorizationIntent(ctx).Identity()
	if a.Version != 1 || !a.Activo || a.Identidad != i.IdentidadDrive() || a.Revision == "" {
		return ajustes.AccesoDrive{}, errAperturaAjena
	}
	return a, nil
}

// Abrir consume la URI preparada; no vuelve a buscar credenciales ni otra ubicación.
func (a *Apertura) Abrir(ctx context.Context) error {
	if a == nil || a.abrir == nil {
		return ErrNoDisponible
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.consumida {
		return ErrNoDisponible
	}
	a.consumida = true
	defer a.cerrar.Do(func() {
		if a.soltar != nil {
			a.soltar()
		}
	})
	actual, err := accesoDeApertura(ctx, a.dueno)
	if err != nil {
		return err
	}
	if actual != a.acceso {
		return errAperturaAjena
	}
	return a.abrir(ctx, a.uri)
}

func (a *Apertura) Close() error {
	if a != nil {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.consumida = true
		a.cerrar.Do(func() {
			if a.soltar != nil {
				a.soltar()
			}
		})
	}
	return nil
}
