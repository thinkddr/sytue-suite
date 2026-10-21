// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// CuentaArchivos es dónde apunta Drive el id de la contraseña de aplicación de su
// proveedor de archivos (Finder, Archivos de iOS: apps/fyne/cmd/drive/archivos.go). Va
// en el almacén de la SUITE y no en el de Drive para que cerrar sesión desde cualquiera
// de las quince la revoque: la extensión trabaja con la app cerrada, y una contraseña
// viva después de «cerrar sesión» sería una puerta que la persona cree cerrada.
//
// Se apunta «<id> <sujeto>»: de quién es dice si la credencial que encuentre Drive al
// arrancar es de quien ha entrado ahora o de otra persona que salió desde otra app.
const CuentaArchivos = "archivos-clave-id"

// ApuntarArchivos apunta la contraseña `id` de `sujeto`.
func ApuntarArchivos(id, sujeto string) error {
	return auth.GuardarSecreto(CuentaArchivos, []byte(id+" "+sujeto))
}

// ArchivosApuntada devuelve la contraseña apuntada y de quién es, o "" si no hay.
func ArchivosApuntada() (id, sujeto string, err error) {
	crudo, err := auth.LeerSecreto(CuentaArchivos)
	id, sujeto, _ = strings.Cut(string(crudo), " ")
	return id, sujeto, err
}

// RevocarArchivos revoca con `cuenta` la contraseña apuntada en CuentaArchivos y olvida
// su id. Sin ninguna apuntada no hace nada; una que ya no existe (404) cuenta como
// revocada. Si falla, el id se queda: lo vuelve a intentar el siguiente que entre.
func RevocarArchivos(ctx context.Context, cuenta *api.Account) error {
	crudo, err := auth.LeerSecreto(CuentaArchivos)
	if err != nil || len(crudo) == 0 {
		return err
	}
	id, _, _ := strings.Cut(string(crudo), " ")
	var e *api.Error
	if err := cuenta.RevokeAppPassword(ctx, id); err != nil && (!errors.As(err, &e) || e.Status != http.StatusNotFound) {
		return err
	}
	// Solo si sigue siendo la misma: mientras tanto otra app puede haber creado otra.
	if ahora, _ := auth.LeerSecreto(CuentaArchivos); bytes.Equal(ahora, crudo) {
		return auth.BorrarSecreto(CuentaArchivos)
	}
	return nil
}

// revocarArchivos es la parte de Salir: el token se coge AHORA, antes de olvidar la
// sesión, y la revocación va aparte y con plazo, como la del refresh token.
func (s *Suite) revocarArchivos() {
	if id, err := auth.LeerSecreto(CuentaArchivos); err != nil || len(id) == 0 {
		return
	}
	ctx, parar := context.WithTimeout(context.Background(), 2*time.Second)
	tok, err := s.Sesion.AccessToken(ctx)
	parar()
	if err != nil {
		slog.Warn("sesion: sin token para revocar la contraseña de Archivos; la revoca el siguiente inicio de sesión en Drive", "error", err)
		return
	}
	cuenta := api.NewAccount(s.URL("id"), tokenFijo(tok), nil)
	go func() {
		ctx, parar := context.WithTimeout(context.Background(), 10*time.Second)
		defer parar()
		if err := RevocarArchivos(ctx, cuenta); err != nil {
			slog.Warn("sesion: la contraseña de Archivos no se ha podido revocar; la revoca el siguiente inicio de sesión en Drive", "error", err)
		}
	}()
}
