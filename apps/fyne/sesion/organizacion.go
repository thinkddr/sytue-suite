// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Cambiar de organización es cosa de la SESIÓN y no de una app: el `tenant` va firmado
// dentro del token que comparten las trece, así que cambiarlo es volver a entrar, y al
// volver a entrar las doce restantes se enteran por la marca del llavero como con
// cualquier otro inicio de sesión.
//
// Es lo que hace el menú de cuenta de la web (`AccountMenu.tsx`) y «Tus datos» cuando
// se elige otra organización: `PUT /auth/organizations` y `location.assign('/auth/login')`.
// Con la sesión de Sytue ID viva en el navegador del sistema, volver a entrar es una
// redirección y no pedir la contraseña.

// organizador es lo que hace falta de Sytue ID. Lo cumple `*api.Account`; en las
// pruebas, un doble.
type organizador interface {
	Organizaciones(ctx context.Context) ([]api.Organizacion, error)
	CambiarOrganizacion(ctx context.Context, id string) error
}

var _ organizador = (*api.Account)(nil)

// cuenta es el cliente de Sytue ID con esta sesión, o el doble que haya puesto una
// prueba.
func (s *Suite) cuenta() organizador {
	if s.orgs != nil {
		return s.orgs
	}
	return api.NewAccount(s.URL("id"), s.Sesion, nil)
}

// Organizaciones son las de quien ha entrado, con la activa marcada.
func (s *Suite) Organizaciones(ctx context.Context) ([]api.Organizacion, error) {
	return s.cuenta().Organizaciones(ctx)
}

// CambiarOrganizacion hace activa otra de mis organizaciones y vuelve a entrar para que
// el token la lleve. BLOQUEA hasta que se vuelve del navegador, como Entrar: se llama
// desde una goroutine.
//
// Si el cambio falla no se abre nada y la sesión sigue en la de antes. Si falla la
// vuelta, el servicio ya ha revocado los refresh tokens —es a propósito, para que el
// tenant viejo no sobreviva días— y la sesión se acaba al caducar el token: es lo mismo
// que le pasa a la web si se cierra la pestaña a mitad de la redirección.
func (s *Suite) CambiarOrganizacion(ctx context.Context, id string) error {
	if err := s.cuenta().CambiarOrganizacion(ctx, id); err != nil {
		return err
	}
	if s.reentrar != nil {
		return s.reentrar(ctx)
	}
	return s.Entrar(ctx)
}
