// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js && !android && !ios && !(darwin && tienda)

package auth

import (
	"encoding/base64"
	"errors"
	"log/slog"

	"github.com/zalando/go-keyring"
)

// La sesión se guarda en el llavero del sistema: Keychain en macOS, Secret Service en
// Linux y Credential Manager en Windows.
//
// En un Linux SIN Secret Service (KWallet apagado, un gestor de ventanas sin llavero) va a
// un fichero cifrado, con la clave en otro sitio (store_fichero_linux.go, que explica de
// qué protege y de qué no), y se llama a [SinLlavero] la primera vez para que la app lo
// diga. En cuanto vuelve a haber llavero, la sesión se muda a él y el fichero se borra.
// Los otros secretos (LeerSecreto…) no tienen ese repuesto: sin llavero, no se guardan.
const (
	llaveroServicio = "eu.sytue.one"
	llaveroCuenta   = "sesion"
)

// cargar recupera la sesión guardada. Sin ninguna, devuelve Tokens vacíos y nil.
//
// En Linux puede haber sesión en el llavero y en el fichero cifrado a la vez (el fichero
// se escribió cuando el llavero no contestaba, o no se pudo borrar al volver): manda la
// que se guardó después. Si es la del fichero y hay llavero, se muda a él en el acto; si
// es la del llavero, el fichero sobra y se borra.
//
// ponytail: «después» es el reloj de pared de cada guardado. Solo decide si el fichero no
// se pudo borrar o se escribió con el llavero caído, y si además el reloj retrocedió
// justo entonces, gana la vieja: el token está rotado y se vuelve a entrar, sin fuga.
func cargar() (Tokens, error) { return cargarCuenta(llaveroCuenta) }

func cargarCuenta(cuenta string) (Tokens, error) {
	if cuentaCompartida(cuenta) {
		return cargarCuentaCompartida(cuenta)
	}
	crudo, errLlavero := keyring.Get(llaveroServicio, cuenta)
	llaveroVivo := errLlavero == nil || errors.Is(errLlavero, keyring.ErrNotFound)
	if !llaveroVivo && !sinLlavero() {
		return Tokens{}, errLlavero // hay llavero y no deja leer
	}
	var enLlavero Tokens
	var cuandoLlavero int64
	if errLlavero == nil {
		var err error
		if enLlavero, cuandoLlavero, err = recuperarConInstante([]byte(crudo)); err != nil {
			return Tokens{}, err
		}
	}
	enFichero, cuandoFichero, errFichero := cargarFichero(cuenta)
	switch {
	case enFichero.Refresh != "" && (enLlavero.Refresh == "" || cuandoFichero > cuandoLlavero):
		if llaveroVivo {
			mudarCuenta(cuenta, enFichero)
		}
		return enFichero, nil
	case enLlavero.Refresh != "":
		if enFichero.Refresh != "" {
			avisarBorrado(borrarFichero(cuenta)) // una copia vieja: no puede volver a salir
		}
		return enLlavero, nil
	case llaveroVivo:
		// Un sobre que no se abre todavía (su clave la está poniendo otra app) no es
		// ningún error para quien pregunta: con llavero y sin nada en él, no hay sesión.
		return Tokens{}, nil
	default:
		return Tokens{}, errFichero
	}
}

// mudar pasa la sesión del fichero al llavero, y borra el fichero si ha ido bien.
func mudarCuenta(cuenta string, t Tokens) {
	crudo, err := serializar(t)
	if err == nil {
		err = keyring.Set(llaveroServicio, cuenta, string(crudo))
	}
	if err == nil {
		avisarBorrado(borrarFichero(cuenta))
	}
}

// avisarBorrado apunta que no se ha podido borrar la copia cifrada. No es un error de
// quien guardó: la sesión ya está en el llavero.
func avisarBorrado(err error) {
	if err != nil {
		slog.Warn("auth: la sesión ya está en el llavero y la copia cifrada no se ha podido borrar", "error", err)
	}
}

// guardar persiste el refresh token, o borra la entrada cuando ya no hay sesión.
func guardar(t Tokens) error { return guardarCuenta(llaveroCuenta, t) }

func guardarCuenta(cuenta string, t Tokens) error {
	if cuentaCompartida(cuenta) {
		return guardarCuentaCompartida(cuenta, t)
	}
	if t.Refresh == "" {
		errFichero := borrarFichero(cuenta)
		if err := keyring.Delete(llaveroServicio, cuenta); err != nil &&
			!errors.Is(err, keyring.ErrNotFound) && !sinLlavero() {
			return err
		}
		return errFichero
	}
	crudo, err := serializar(t)
	if err != nil {
		return err
	}
	err = keyring.Set(llaveroServicio, cuenta, string(crudo))
	if err == nil {
		avisarBorrado(borrarFichero(cuenta)) // ya está en el llavero: la copia cifrada sobra
		return nil
	}
	if !sinLlavero() {
		return err // hay llavero y ha dicho que no: no se esquiva escribiendo a disco
	}
	nuevo, err := guardarFichero(t, cuenta)
	if err == nil && nuevo && SinLlavero != nil {
		SinLlavero()
	}
	return err
}

// Los demás secretos, en el mismo servicio del llavero con su propia cuenta. En base64:
// el llavero guarda texto.
func leerSecreto(cuenta string) ([]byte, error) {
	if cuentaCompartida(cuenta) {
		return leerSecretoCompartido(cuenta)
	}
	return leerSecretoLegado(cuenta)
}

func leerSecretoLegado(cuenta string) ([]byte, error) {
	crudo, err := keyring.Get(llaveroServicio, cuenta)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(crudo)
}

func guardarSecreto(cuenta string, datos []byte) error {
	if cuentaCompartida(cuenta) {
		return guardarSecretoCompartido(cuenta, datos)
	}
	return keyring.Set(llaveroServicio, cuenta, base64.StdEncoding.EncodeToString(datos))
}

func borrarSecreto(cuenta string) error {
	if cuentaCompartida(cuenta) {
		if err := borrarSecretoCompartido(cuenta); err != nil {
			return err
		}
	}
	if err := keyring.Delete(llaveroServicio, cuenta); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}
