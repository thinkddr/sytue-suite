// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package sesscrypto cifra los datos de contexto de una sesión —la IP de acceso
// y la ubicación que se deduce de ella— con una clave que Sytue NO custodia.
//
// EL PUNTO DEL QUE SALE TODO: en la tabla `sessions` no está el token de sesión,
// solo su SHA-256. El token vive únicamente en la cookie del navegador de su
// dueño. Si la clave de cifrado se DERIVA del token, entonces:
//
//	· con la base de datos entera delante, esos campos son ruido;
//	· con una copia de seguridad robada, también;
//	· quien administre el servidor no puede leerlos sin que el dueño presente
//	  su cookie.
//
// LO QUE ESTO NO ES, Y CONVIENE NO PROMETERLO. El servidor VE la IP en el
// instante de la conexión: es la dirección de origen del TCP, no un dato que el
// cliente elija enviar. Ningún cifrado cambia eso, ni aquí ni en ningún otro
// servicio del mundo. Lo que se garantiza es que no queda ALMACENADA de forma
// legible: existe en memoria el tiempo de atender la petición y a disco solo baja
// cifrada. La frase honesta es «no guardamos tu IP de forma que podamos leerla»,
// no «nunca la vemos».
//
// CONSECUENCIA VISIBLE, Y ES DELIBERADA: cada sesión cifra con SU token, así que
// desde una sesión solo se descifra su propia fila. La pantalla de la cuenta
// enseña la ubicación de la sesión desde la que miras y, de las demás, el
// dispositivo y las fechas pero no de dónde vinieron. Es el precio de que no
// exista ninguna llave maestra: si la hubiera para poder enseñarlas todas,
// Sytue la tendría, y entonces la promesa sería falsa.
package sesscrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
)

// ErrNoDescifrable indica que el texto cifrado no corresponde a esta clave. Es
// el caso NORMAL al mirar sesiones ajenas a la actual, no una anomalía.
var ErrNoDescifrable = errors.New("sesscrypto: no se puede descifrar con esta clave")

// info separa el propósito de la clave derivada. Si algún día se deriva otra
// cosa del mismo token, cambiar esta etiqueta evita que las dos claves coincidan.
const info = "sytue-id/sesion/contexto/v1"

// clave deriva la clave de cifrado del token de sesión con HKDF-SHA256.
//
// HKDF y no el token en crudo: el token es una cadena aleatoria de 32 bytes, pero
// pasarla por una derivación con etiqueta de propósito impide que un mismo valor
// sirva para dos cosas distintas, y deja el camino abierto a derivar más claves
// del mismo secreto sin que se relacionen entre sí.
func clave(token string) ([]byte, error) {
	k := make([]byte, 32)
	r := hkdf.New(sha256.New, []byte(token), nil, []byte(info))
	if _, err := io.ReadFull(r, k); err != nil {
		return nil, err
	}
	return k, nil
}

// Cifrar protege un valor con la clave derivada del token. Devuelve nonce||ct.
// Un valor vacío se guarda como nil: cifrar la cadena vacía dejaría un rastro
// del mismo tamaño que un dato real y haría distinguible «no había IP» de «hay
// una IP», que es justo lo que no interesa.
func Cifrar(token, valor string) ([]byte, error) {
	if valor == "" {
		return nil, nil
	}
	k, err := clave(token)
	if err != nil {
		return nil, err
	}
	bloque, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(bloque)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(valor), nil), nil
}

// Descifrar recupera el valor. Devuelve ErrNoDescifrable si el texto no fue
// cifrado con esta clave, que es lo que ocurre —por diseño— al mirar la fila de
// otra sesión.
func Descifrar(token string, ct []byte) (string, error) {
	if len(ct) == 0 {
		return "", nil
	}
	k, err := clave(token)
	if err != nil {
		return "", err
	}
	bloque, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(bloque)
	if err != nil {
		return "", err
	}
	if len(ct) < gcm.NonceSize() {
		return "", ErrNoDescifrable
	}
	nonce, cuerpo := ct[:gcm.NonceSize()], ct[gcm.NonceSize():]
	claro, err := gcm.Open(nil, nonce, cuerpo, nil)
	if err != nil {
		// GCM está autenticado: un fallo aquí es «no es tuyo» o «está manipulado»,
		// y las dos cosas se tratan igual desde fuera.
		return "", ErrNoDescifrable
	}
	return string(claro), nil
}
