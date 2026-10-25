// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package documentos

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// Identidad ata la conexión del sistema a la sesión que la autorizó. CuentaSesion
// es sólo el identificador del almacén OIDC, nunca un token ni una contraseña.
type Identidad struct {
	Emisor       string `json:"issuer,omitempty"`
	Cliente      string `json:"client,omitempty"`
	Sujeto       string `json:"subject,omitempty"`
	Organizacion string `json:"org,omitempty"`
	CuentaSesion string `json:"account,omitempty"`
}

func (i Identidad) completa() bool {
	return i.Emisor != "" && i.Cliente != "" && i.Sujeto != "" && i.Organizacion != ""
}

// Pendiente guarda sólo lo necesario para revocar una contraseña de otra sesión
// cuando vuelva su dueño; cambiar de cuenta nunca necesita su contraseña en claro.
type Pendiente struct {
	ID    string    `json:"id"`
	Dueno Identidad `json:"owner"`
}

// Una sola operación entre procesos: dos apps que arrancan juntas no crean dos
// contraseñas ni pierden la revocación que dejó pendiente un cierre sin conexión.
func excluirCredencial(ctx context.Context) (func(), error) {
	dir, err := ajustes.Carpeta()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "drive-documentos.cerrojo"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	return ajustes.CerrojoFichero(ctx, f)
}

var excluir = excluirCredencial

// AsegurarIdentidad es Asegurar para el ciclo de sesión de la suite. No reutiliza
// una credencial de otro emisor, usuario, organización ni cliente OIDC.
func AsegurarIdentidad(ctx context.Context, c Cuenta, url, etiqueta string, identidad Identidad) error {
	if !identidad.completa() {
		return errors.New("documentos: falta la identidad de la sesión")
	}
	return asegurar(ctx, c, url, etiqueta, identidad)
}

// RetirarIdentidad retira la conexión local y revoca sólo las contraseñas del
// dueño presente. Las de otra cuenta quedan sin contraseña y pendientes para ella.
func RetirarIdentidad(ctx context.Context, c Cuenta, identidad Identidad) error {
	return retirar(ctx, c, &identidad)
}

// CredencialActiva comprueba sólo el almacén local. Una retirada desde otra app
// invalida el resultado previo del controlador aunque el usuario vuelva a entrar
// con la misma identidad antes de que se observe el cambio intermedio.
func CredencialActiva(identidad Identidad, url string) bool {
	c, err := cargarCredencial()
	return err == nil && c.Clave != "" && c.Dueno == identidad && c.URL == url
}
