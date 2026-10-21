// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"errors"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// Load y Save son la sesión vista desde fuera: UNA entrada en el almacén seguro del
// sistema para las trece apps de la suite. Dónde está esa entrada lo pone cada
// plataforma en su `store_*.go` (`cargar`/`guardar`); lo que se añade aquí, y es igual
// en las tres, es la MARCA: dejar dicho que la sesión guardada ha cambiado para que las
// otras doce apps abiertas se enteren sin tener que preguntarle al llavero cada dos
// segundos —que en macOS es una llamada al demonio del Keychain, y en Linux una vuelta
// por D-Bus—.
//
// La marca NO lleva nada de la sesión: es un número que solo sirve para ser distinto del
// anterior (apps/nucleo/ajustes). Quien ve que ha cambiado vuelve aquí, a Load.

// SinLlavero, si alguien lo pone, se llama cuando la sesión se acaba de guardar en el
// fichero cifrado porque no hay llavero, y solo cuando el fichero es nuevo: la app lo
// cuenta una vez por inicio de sesión, no en cada renovación del token. Se llama en la
// rutina que guardó. Solo pasa en un Linux sin Secret Service (store_fichero_linux.go).
var SinLlavero func()

// Load recupera la sesión guardada. Sin ninguna, devuelve Tokens vacíos y nil.
func Load() (Tokens, error) { return cargar() }

// Save es el `guardar` de NewSession: persiste el refresh token, o borra la entrada
// cuando ya no hay sesión, y avisa a las demás apps de que algo ha cambiado.
//
// Se avisa TAMBIÉN cuando el almacén falla: lo que la otra app tiene que hacer es
// volver a leer, y si aquí ha fallado el guardado, leer es exactamente lo que le dirá
// la verdad.
func Save(t Tokens) error {
	err := guardar(t)
	ajustes.MarcarSesion()
	return err
}

// # Otros secretos en el mismo almacén
//
// Lo que no es la sesión pero tampoco puede ir en claro al disco: hoy, la clave derivada
// de un cuaderno cifrado de Notepads que su dueño ha pedido recordar con Touch ID. Va en
// el mismo almacén seguro de cada sistema, con su propia cuenta, y leerlo NO pide nada:
// quien lo usa verifica antes al dueño con `escritorio.Verificar`, como el bloqueo de la
// sesión.

// errCuenta es una cuenta con caracteres que no valen como nombre de fichero (Android) o
// que pisaría la entrada de la sesión.
// CuentaBloqueo es el testigo seguro de la política de desbloqueo de la suite.
// También se comparte entre las variantes firmadas Developer ID y Mac App Store.
const CuentaBloqueo = "bloqueo-sesion"

var errCuenta = errors.New("auth: cuenta de secreto no válida")

func cuentaValida(cuenta string) bool {
	if cuenta == "" || cuenta == "sesion" || strings.HasPrefix(cuenta, "sesion-") || cuenta == cuentaIndice || len(cuenta) > 128 {
		return false
	}
	for _, c := range cuenta {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' && c != '.' {
			return false
		}
	}
	return true
}

// LeerSecreto devuelve el secreto de `cuenta`, o nil sin error si no hay ninguno.
func LeerSecreto(cuenta string) ([]byte, error) {
	if !cuentaValida(cuenta) {
		return nil, errCuenta
	}
	return leerSecreto(cuenta)
}

// GuardarSecreto guarda `datos` en `cuenta`, sustituyendo lo que hubiera. La cuenta
// queda apuntada en un índice para que OlvidarSecretos la encuentre: el almacén de
// cada sistema no deja listar lo que guarda.
func GuardarSecreto(cuenta string, datos []byte) error {
	if !cuentaValida(cuenta) || len(datos) == 0 {
		return errCuenta
	}
	if err := guardarSecreto(cuenta, datos); err != nil {
		return err
	}
	return apuntar(cuenta, true)
}

// BorrarSecreto quita el secreto de `cuenta`. No haber ninguno no es un error.
func BorrarSecreto(cuenta string) error {
	if !cuentaValida(cuenta) {
		return errCuenta
	}
	if err := borrarSecreto(cuenta); err != nil {
		return err
	}
	return apuntar(cuenta, false)
}

// cuentaIndice es la entrada con la lista de cuentas que hay guardadas.
const cuentaIndice = "indice-secretos"

// apuntar mete o saca `cuenta` del índice.
func apuntar(cuenta string, esta bool) error {
	crudo, err := leerSecreto(cuentaIndice)
	if err != nil {
		return err
	}
	lista := strings.Fields(string(crudo))
	lista = slices.DeleteFunc(lista, func(c string) bool { return c == cuenta })
	if esta {
		lista = append(lista, cuenta)
	}
	if len(lista) == 0 {
		return borrarSecreto(cuentaIndice)
	}
	return guardarSecreto(cuentaIndice, []byte(strings.Join(lista, " ")))
}

// OlvidarSecretos borra los secretos guardados cuyas cuentas empiezan por `prefijo`.
// Al cerrar sesión, las claves recordadas de los cuadernos cifrados de Notepads
// (`notes-…`) sobrevivían en el almacén y abrían los cuadernos a quien entrara después
// con la misma cuenta del sistema (FYS-09).
func OlvidarSecretos(prefijo string) error {
	crudo, err := leerSecreto(cuentaIndice)
	if err != nil {
		return err
	}
	var errs []error
	for _, c := range strings.Fields(string(crudo)) {
		if strings.HasPrefix(c, prefijo) {
			errs = append(errs, BorrarSecreto(c))
		}
	}
	return errors.Join(errs...)
}
