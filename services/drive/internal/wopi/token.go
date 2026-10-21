// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package wopi implementa el lado HOST del protocolo WOPI (Web Application Open Platform
// Interface): el que permite que un editor externo —Office Online, Collabora Online,
// OnlyOffice— abra y guarde un fichero que vive en Sytue Drive.
//
// Es el mismo papel que hacen Nextcloud, ownCloud o Seafile: Sytue guarda el fichero y el
// editor lo pide. Sytue NO es aquí el editor (para eso está Sytue Docs); es el almacén.
//
// # POR QUÉ EL TOKEN NO ES EL BEARER DE SYTUE ID
//
// El flujo de WOPI es a tres bandas: el navegador abre el editor, y es el EDITOR —desde su
// servidor— quien llama de vuelta a Sytue con un `access_token` que el host le entregó.
// Ese token viaja en la query string, queda en los logs del editor y lo maneja software de
// un tercero, así que NO puede ser el access token de la sesión: daría a ese tercero la
// identidad completa del usuario sobre toda la suite.
//
// Por eso se acuña un token PROPIO, acotado a lo mínimo: un fichero, un usuario, un
// permiso y una caducidad corta. Si se filtra, lo que se puede hacer con él es abrir ese
// fichero —nada más, y no por mucho tiempo.
package wopi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
)

// TTL por defecto de un token WOPI. Corto a propósito: el editor lo renueva con
// `PutRelativeFile`/refresco de sesión, y una fuga caduca sola.
const DefaultTTL = 10 * time.Hour

// ErrToken indica un token ausente, corrupto, de otro fichero o caducado. Es UNO solo a
// propósito: distinguir «caducado» de «falsificado» hacia el editor no le sirve de nada y
// sí le dice a quien sondea si acertó con el formato.
var ErrToken = errors.New("wopi: token de acceso inválido")

// Claims es lo que el token transporta. Todo lo necesario para autorizar la llamada del
// editor SIN volver a preguntar quién es: el editor no tiene sesión de Sytue.
type Claims struct {
	// Tenant es la organización. Viaja DENTRO del token y no como parámetro: si se
	// tomara de la petición, el editor podría pedir el mismo id de fichero en otra.
	Tenant string `json:"t"`
	// Subject es el usuario en cuyo nombre actúa el editor. Las escrituras se atribuyen
	// a él, no a «el editor».
	Subject string `json:"s"`
	// Name es su nombre visible: WOPI lo pide en CheckFileInfo para pintar la presencia.
	Name string `json:"n,omitempty"`
	// FileID acota el token a UN fichero. Se comprueba contra el de la ruta.
	FileID uuid.UUID `json:"f"`
	// Write indica si el editor puede guardar. Sale del permiso REAL del usuario sobre
	// el fichero en el momento de acuñarlo.
	Write bool `json:"w,omitempty"`
	// Exp es el vencimiento (unix). Sin él, un token filtrado valdría para siempre.
	Exp int64 `json:"e"`
}

// Minter acuña y verifica tokens WOPI. El sellado es AES-256-GCM (keycrypto), así que el
// token es opaco Y autenticado: nadie puede leerlo ni fabricar uno sin la clave.
type Minter struct {
	cipher *keycrypto.Cipher
	ttl    time.Duration
	now    func() time.Time
}

// NewMinter construye el acuñador. `ttl` a cero usa DefaultTTL.
func NewMinter(c *keycrypto.Cipher, ttl time.Duration) *Minter {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Minter{cipher: c, ttl: ttl, now: time.Now}
}

// Enabled indica si hay clave configurada. Sin ella NO se acuñan tokens y la capa WOPI se
// apaga entera: es preferible que el editor no pueda abrir nada a emitir tokens sin cifrar.
func (m *Minter) Enabled() bool { return m != nil && m.cipher != nil && m.cipher.Enabled() }

// Mint sella un token para (fichero, usuario, permiso).
func (m *Minter) Mint(c Claims) (string, error) {
	if !m.Enabled() {
		return "", ErrToken
	}
	c.Exp = m.now().Add(m.ttl).Unix()
	raw, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("wopi: serializar claims: %w", err)
	}
	// base64 url-safe ANTES de cifrar no: se cifra el JSON y se codifica el resultado,
	// que es lo que viaja en la query string.
	sealed, err := m.cipher.Encrypt(string(raw))
	if err != nil {
		return "", fmt.Errorf("wopi: sellar token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(sealed)), nil
}

// Verify abre el token y comprueba que sigue vigente y que es DE ESTE fichero.
//
// La comprobación del fichero se hace aquí y no en el handler a propósito: es la que
// impide que un token legítimo de un fichero se reutilice contra otro, y dejarla fuera
// sería confiar en que todos los handlers se acuerden de hacerla.
func (m *Minter) Verify(token string, fileID uuid.UUID) (Claims, error) {
	if !m.Enabled() || strings.TrimSpace(token) == "" {
		return Claims{}, ErrToken
	}
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Claims{}, ErrToken
	}
	plain, err := m.cipher.Decrypt(string(sealed))
	if err != nil {
		return Claims{}, ErrToken
	}
	// Decrypt devuelve el valor tal cual si NO lleva el prefijo `encv1:` (compatibilidad
	// con valores en claro heredados en columnas). Aquí eso sería aceptar un token sin
	// cifrar, así que se exige el prefijo antes de mirar el contenido.
	if !strings.HasPrefix(string(sealed), "encv1:") {
		return Claims{}, ErrToken
	}
	var c Claims
	if err := json.Unmarshal([]byte(plain), &c); err != nil {
		return Claims{}, ErrToken
	}
	if c.FileID != fileID || c.Tenant == "" || c.Subject == "" {
		return Claims{}, ErrToken
	}
	if m.now().Unix() >= c.Exp {
		return Claims{}, ErrToken
	}
	return c, nil
}
