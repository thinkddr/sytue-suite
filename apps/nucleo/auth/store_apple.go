// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios || (darwin && tienda)

package auth

import (
	"log/slog"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

const cuentaLlavero = "sesion"

// La entrada histórica se busca sin grupo explícito para conservarla al actualizar.
// Las nuevas sesiones se escriben siempre en el grupo firmado de la suite.
func cargar() (Tokens, error) { return cargarCuenta(cuentaLlavero) }

func cargarCuenta(cuenta string) (Tokens, error) {
	datos, err := leerDatosApple(cuenta, grupoCuenta(cuenta))
	if err != nil || datos == nil {
		return Tokens{}, err
	}
	return recuperar(datos)
}

func guardar(t Tokens) error { return guardarCuenta(cuentaLlavero, t) }

func guardarCuenta(cuenta string, t Tokens) error {
	if t.Refresh == "" {
		return borrarDatosApple(cuenta, grupoCuenta(cuenta))
	}
	datos, err := serializar(t)
	if err != nil {
		return err
	}
	return guardarDatosApple(cuenta, grupoCuenta(cuenta), datos)
}

// Los demás secretos conservan su ubicación anterior. La extensión de archivos
// tiene su propio grupo y no recibe acceso al refresh token de la suite.
func leerSecreto(cuenta string) ([]byte, error) {
	datos, err := leerDatosApple(cuenta, grupoCuenta(cuenta))
	if cuenta != CuentaBloqueo || err != nil || datos != nil {
		return datos, err
	}
	// Una actualización conserva la política privada anterior al primer acceso.
	datos, err = leerDatosApple(cuenta, "")
	if err == nil && len(datos) > 0 {
		if guardar := guardarDatosApple(cuenta, ajustes.GrupoSuite, datos); guardar != nil {
			slog.Warn("auth: se conserva el bloqueo anterior; no se pudo migrar al grupo de la suite", "error", guardar)
		}
	}
	return datos, err
}

func guardarSecreto(cuenta string, datos []byte) error {
	return guardarDatosApple(cuenta, grupoCuenta(cuenta), datos)
}

func borrarSecreto(cuenta string) error {
	if err := borrarDatosApple(cuenta, grupoCuenta(cuenta)); err != nil {
		return err
	}
	if cuenta == CuentaBloqueo {
		return borrarDatosApple(cuenta, "")
	}
	return nil
}

func grupoCuenta(cuenta string) string {
	if strings.HasPrefix(cuenta, "sesion-") || cuenta == CuentaBloqueo {
		return ajustes.GrupoSuite
	}
	return ""
}
