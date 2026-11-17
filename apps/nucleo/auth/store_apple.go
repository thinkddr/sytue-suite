// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios || (darwin && tienda)

package auth

import (
	"log/slog"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

const cuentaLlavero = "sesion"

// Las sesiones usan el grupo firmado de la suite. Una entrada histórica privada
// se transfiere atómicamente sólo si no existe ya una sesión compartida.
func cargar() (Tokens, error) { return cargarCuenta(cuentaLlavero) }

func cargarCuenta(cuenta string) (Tokens, error) {
	datos, err := leerDatosApple(cuenta, grupoCuenta(cuenta))
	if err != nil {
		return Tokens{}, err
	}
	if len(datos) == 0 && cuenta == cuentaLlavero {
		own, err := grupoPrivadoApple()
		if err != nil {
			return Tokens{}, err
		}
		exists, err := existeDatosApple(cuenta, own)
		if err != nil {
			return Tokens{}, err
		}
		if exists {
			datos, err = migrarSecretoPrivado(keychainMigration{}, cuenta, ajustes.GrupoSuite, own, func(raw []byte) error { _, err := recuperar(raw); return err })
			if err != nil {
				return Tokens{}, err
			}
		}
	}
	defer clear(datos)
	if len(datos) == 0 {
		return Tokens{}, nil
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

// Los secretos privados usan siempre el grupo de esta app. Un valor histórico
// compartido requiere la migración validada por el SDK o la caché que lo posee.
func leerSecreto(cuenta string) ([]byte, error) {
	datos, err := leerDatosApple(cuenta, grupoCuenta(cuenta))
	if err == nil && len(datos) == 0 && grupoCuenta(cuenta) == "" {
		if err = denegarLegadoCompartido(cuenta); err != nil {
			return nil, err
		}
	}
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
	if err := denegarLegadoCompartido(cuenta); err != nil {
		return err
	}
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
	if cuenta == cuentaLlavero || enGrupoSuite(cuenta) || cuentaRecuperacionAlmacen(cuenta) {
		return ajustes.GrupoSuite
	}
	return ""
}

func denegarLegadoCompartido(cuenta string) error {
	if grupoCuenta(cuenta) != "" {
		return nil
	}
	owned, err := existeDatosApple(cuenta, "")
	if err != nil || owned {
		return err
	}
	legacy, err := existeDatosApple(cuenta, ajustes.GrupoSuite)
	if err != nil {
		return err
	}
	if legacy {
		return ErrSecretoLegadoCompartido
	}
	return nil
}
