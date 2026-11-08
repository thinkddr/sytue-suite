// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package apns es el aviso «SOLO DESPERTAR» a las apps nativas de iOS.
//
// # Por qué existe, y por qué no tiene equivalente en Android
//
// En Android la app mantiene ella misma el flujo SSE del Hub y se entera sin pedir
// permiso a nadie: el aviso nace y muere dentro de nuestra infraestructura europea. En
// iOS no: el sistema corta la conexión al pasar a segundo plano y el ÚNICO camino para
// despertar una app es APNs, que es de Apple y no tiene región europea. No hay
// alternativa técnica; solo cabe decidir QUÉ se le cuenta (ADR 0039).
//
// # Qué sale hacia Apple
//
// El token del aparato, el `apns-topic` —el identificador de la app, o sea qué app de
// la suite tiene algo nuevo— y el momento. El cuerpo es siempre el mismo
// (`cuerpoDespertar`) y va VACÍO de contenido: sin título, sin texto, sin autor, sin
// contador. La app despierta, pide lo suyo a nuestros servidores y repinta su widget.
// Apple puede saber que ese aparato recibió algo para esa app y cuándo; no qué, ni de
// quién.
//
// Por eso el cuerpo es una CONSTANTE y no se compone: que nadie pueda añadirle un
// campo «solo para depurar» sin darse cuenta de lo que está publicando.
// `TestAPNSNoMandaNadaMasQueElDespertar` lo fija.
//
// Hasta el 2026-11-07 vivía en `services/rooms/internal/service/apns.go`, con el topic
// fijo de Rooms. Se movió aquí, sin copia, cuando el registro de aparatos pasó al Hub
// para despertar a cualquiera de las quince apps por su widget (ADR 0078 y 0039).
package apns

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

const (
	// HostProduccion es el servidor de producción de Apple. El de pruebas
	// (HostPruebas) se elige por config: un token de una compilación de desarrollo NO
	// vale en producción y viceversa, y el error que da Apple (BadDeviceToken) no
	// distingue «token roto» de «servidor equivocado».
	HostProduccion = "https://api.push.apple.com"
	// HostPruebas es el servidor de Apple para las compilaciones de desarrollo.
	HostPruebas = "https://api.sandbox.push.apple.com"

	// vidaJWT renueva el JWT antes de que Apple lo rechace. Apple exige entre 20 y 60
	// minutos de antigüedad y devuelve TooManyProviderTokenUpdates si se pide uno nuevo
	// demasiado seguido, así que se reutiliza hasta cerca del límite.
	vidaJWT = 45 * time.Minute
	plazo   = 10 * time.Second
	// Caducidad es cuánto guarda Apple un despertar que no ha podido entregar (el
	// aparato apagado o sin red). Pasada, ya no sirve: lo que se iba a leer ha cambiado
	// otra vez o lo leerá la app al abrirse.
	Caducidad = time.Hour

	// maxLargoToken acota la fila. Un token APNs son 32 bytes en hex (64 caracteres),
	// pero Apple se reserva alargarlo, así que el tope es holgado y no exacto.
	maxLargoToken = 200
)

// cuerpoDespertar es lo ÚNICO que se manda a Apple: «hay algo nuevo». Ni un campo más.
var cuerpoDespertar = []byte(`{"aps":{"content-available":1}}`)

// ErrTokenCaduco es la respuesta de Apple de que el token ya no vale (410, o 400 con
// BadDeviceToken, Unregistered o DeviceTokenNotForTopic): quien lo guarda lo borra,
// igual que una suscripción Web Push caducada.
var ErrTokenCaduco = errors.New("apns: el token del aparato ya no vale")

// Config son las credenciales de proveedor que da Apple: la clave `.p8` ENTERA (PEM,
// PKCS#8), su identificador y el del equipo. Una clave de equipo vale para todas las
// apps del equipo, así que el topic va en cada envío y no aquí. Host vacío es
// HostProduccion.
type Config struct {
	KeyP8  string
	KeyID  string
	TeamID string
	Host   string
}

// Enviador firma los JWT de proveedor y entrega los despertares a Apple.
type Enviador struct {
	keyID  string
	teamID string
	host   string
	key    *ecdsa.PrivateKey
	client *http.Client

	// ahora se cambia en las pruebas para comprobar la caducidad del JWT sin esperar
	// tres cuartos de hora.
	ahora func() time.Time

	mu       sync.Mutex
	bearer   string
	bearerAt time.Time
}

// Nuevo construye el enviador. Los tres valores vacíos => (nil, nil): iOS sin
// despertar, como el Web Push sin VAPID. A medias sí es un error de configuración y
// aborta el arranque, porque «creía que estaba puesto» es peor que «no está».
func Nuevo(c Config) (*Enviador, error) {
	puestos := 0
	for _, v := range []string{c.KeyP8, c.KeyID, c.TeamID} {
		if strings.TrimSpace(v) != "" {
			puestos++
		}
	}
	if puestos == 0 {
		return nil, nil //nolint:nilnil // iOS sin despertar: ausencia intencional, no error.
	}
	if puestos < 3 {
		return nil, errors.New("apns: define SYTUE_APNS_KEY_P8, _KEY_ID y _TEAM_ID juntas (o ninguna)")
	}
	key, err := leerClave(c.KeyP8)
	if err != nil {
		return nil, err
	}
	host := strings.TrimRight(strings.TrimSpace(c.Host), "/")
	if host == "" {
		host = HostProduccion
	}
	return &Enviador{
		keyID:  strings.TrimSpace(c.KeyID),
		teamID: strings.TrimSpace(c.TeamID),
		host:   host,
		key:    key,
		// Sin redirecciones y con la barrera SSRF: el host lo pone la configuración,
		// pero un error ahí no debe convertir el enviador en un cliente de la red interna.
		client: ssrfhttp.NewClient(plazo, 0),
		ahora:  time.Now,
	}, nil
}

// leerClave lee la clave .p8 que descarga Apple (PKCS#8, PEM, ECDSA P-256).
func leerClave(p8 string) (*ecdsa.PrivateKey, error) {
	blk, _ := pem.Decode([]byte(p8))
	if blk == nil {
		return nil, errors.New("apns: SYTUE_APNS_KEY_P8 no es un PEM (el fichero .p8 entero, con sus cabeceras)")
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("apns: clave ilegible: %w", err)
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("apns: la clave no es ECDSA (Apple emite P-256)")
	}
	return ec, nil
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// token devuelve el JWT de proveedor, reutilizándolo mientras siga vigente.
func (e *Enviador) token() (string, error) {
	now := e.ahora()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.bearer != "" && now.Sub(e.bearerAt) < vidaJWT {
		return e.bearer, nil
	}
	cab, err := json.Marshal(map[string]string{"alg": "ES256", "kid": e.keyID, "typ": "JWT"})
	if err != nil {
		return "", err
	}
	cla, err := json.Marshal(map[string]any{"iss": e.teamID, "iat": now.Unix()})
	if err != nil {
		return "", err
	}
	firmado := b64url(cab) + "." + b64url(cla)
	sum := sha256.Sum256([]byte(firmado))
	r, s, err := ecdsa.Sign(rand.Reader, e.key, sum[:])
	if err != nil {
		return "", err
	}
	// ES256 quiere R||S en 32 bytes cada uno; el DER de ecdsa.SignASN1 no vale.
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	e.bearer = firmado + "." + b64url(sig)
	e.bearerAt = now
	return e.bearer, nil
}

// Despertar manda el aviso vacío al aparato `token` de la app `topic` (su bundle id).
// Devuelve ErrTokenCaduco si Apple dice que el token ya no vale.
func (e *Enviador) Despertar(ctx context.Context, topic, token string) error {
	if !TokenValido(token) {
		return ErrTokenCaduco
	}
	jwt, err := e.token()
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, plazo)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		e.host+"/3/device/"+token, bytes.NewReader(cuerpoDespertar))
	if err != nil {
		return sinURL(err)
	}
	req.Header.Set("authorization", "bearer "+jwt)
	req.Header.Set("apns-topic", topic)
	// `background` + prioridad 5: es un «mira si tienes algo», no una alerta. Apple
	// exige esa combinación para un push sin contenido y castiga lo contrario.
	req.Header.Set("apns-push-type", "background")
	req.Header.Set("apns-priority", "5")
	req.Header.Set("apns-expiration", strconv.FormatInt(e.ahora().Add(Caducidad).Unix(), 10))
	req.Header.Set("content-type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return sinURL(err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusGone {
		return ErrTokenCaduco
	}
	var r struct {
		Reason string `json:"reason"`
	}
	cuerpo, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = json.Unmarshal(cuerpo, &r)
	switch r.Reason {
	case "BadDeviceToken", "Unregistered", "DeviceTokenNotForTopic":
		return ErrTokenCaduco
	}
	return fmt.Errorf("apns: %d %s", resp.StatusCode, r.Reason)
}

// sinURL quita la URL de un error de transporte. El de `http.Client.Do` es un
// `*url.Error` cuyo texto lleva la URL entera, `…/3/device/<token>`, y quien registra
// el error lo haría con el token del aparato en claro: el redactor de los registros
// tapa por nombre de clave, no por contenido (security.md#apns-despertar).
func sinURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("apns: %s: %w", ue.Op, ue.Err)
	}
	return err
}

// TokenValido valida la forma de un token de aparato: hexadecimal, no vacío y acotado.
// Apple no publica más estructura que ésa.
func TokenValido(token string) bool {
	if len(token) < 32 || len(token) > maxLargoToken {
		return false
	}
	for _, r := range token {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
