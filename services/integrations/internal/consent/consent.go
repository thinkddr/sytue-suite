// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package consent implementa el flujo de CONSENTIMIENTO (OAuth 2.0 authorization code
// + PKCE) con el que un admin conecta una app de terceros en un clic: pulsa «Conectar
// con X», aprueba en el proveedor y vuelve conectado, sin salir a fabricar un token en
// un panel ajeno ni pegar nada.
//
// Es COMPLEMENTARIO al paquete `oauth2` de la pasarela, no lo sustituye: aquél mintea
// tokens máquina-a-máquina (client_credentials, JWT bearer) para hablar con el SaaS una
// vez conectado; éste es la conversación de tres patas con el humano que autoriza.
//
// # POR QUÉ EL ESTADO VIAJA SELLADO Y NO EN UNA TABLA
//
// El flujo necesita recordar entre la ida y la vuelta tres cosas: quién empezó (tenant
// + instancia), el `code_verifier` de PKCE y hasta cuándo vale. La opción obvia es una
// tabla de estados pendientes, con su migración, su índice y su barrido de caducados.
// En vez de eso el estado va CIFRADO Y AUTENTICADO dentro del propio parámetro `state`:
// el AEAD de keycrypto lo hace ilegible e infalsificable para el navegador y para el
// proveedor, y la caducidad viaja dentro. Sin tabla, sin barrido y sin estado servidor
// que se desincronice entre réplicas.
//
// El `state` cumple así sus dos papeles a la vez: el anti-CSRF del RFC 6749 §10.12 (un
// callback con un state que no descifra o que ha caducado se rechaza) y el transporte
// del verificador. Que no se pueda leer desde fuera es lo que permite lo segundo.
package consent

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
)

// Errores del flujo. La capa HTTP los mapea a 400/500 y NUNCA los enseña con detalle al
// navegador: un callback manipulado no debe aprender por qué falló.
var (
	// ErrState: el state no descifra, no parsea o ha caducado. Cubre desde un enlace
	// viejo hasta un intento de CSRF; deliberadamente no se distinguen.
	ErrState = errors.New("consent: state inválido o caducado")
	// ErrDenied: el usuario dijo que no en la pantalla del proveedor. No es un fallo.
	ErrDenied = errors.New("consent: el usuario no autorizó la conexión")
	// ErrProvider: el proveedor devolvió un error propio en el redirect.
	ErrProvider = errors.New("consent: el proveedor rechazó la autorización")
)

// stateTTL acota la ventana entre pulsar «Conectar» y volver del proveedor. Diez
// minutos dan de sobra para autenticarse y leer la pantalla de permisos, y acotan
// cuánto vive un enlace de autorización si se filtra de un historial o un log.
const stateTTL = 10 * time.Minute

// Start es lo que hace falta para mandar al admin al proveedor.
type Start struct {
	// AuthorizeURL es la URL completa a la que redirigir (ya lleva state y PKCE).
	AuthorizeURL string
	// Scopes se enseñan al admin ANTES de redirigir: el paso «qué va a pasar» del
	// asistente explica en llano qué está autorizando. Un consentimiento que no se
	// entiende no es consentimiento.
	Scopes []string
	// Challenge es el code_challenge PKCE de este arranque. Es público (viaja en
	// AuthorizeURL) y quien arranca lo guarda como «consentimiento pendiente» de la
	// instancia: el canje lo consume, así que un mismo estado no se canjea dos veces
	// ni sirve el de un arranque anterior (ver Redeem.Challenge).
	Challenge string
}

// state es lo que sobrevive a la ida y vuelta, sellado dentro del parámetro `state`.
type state struct {
	Tenant   string `json:"t"`
	Instance string `json:"i"`
	// Sub es quién empezó el consentimiento. El estado no se puede falsificar, pero
	// sí llevar a OTRO navegador: sin esto, quien completara una URL de autorización
	// ajena dejaba su token del proveedor en la conexión de quien la generó. Quien
	// canjea el código comprueba que la sesión que vuelve es la de esta persona.
	Sub string `json:"u"`
	// Verifier es el `code_verifier` de PKCE (RFC 7636). Viaja aquí porque el
	// challenge que ve el proveedor es su SHA-256: sin el verificador original nadie
	// puede canjear el código, ni siquiera quien lo intercepte en el redirect.
	Verifier string `json:"v"`
	Expires  int64  `json:"e"`
	// Section es la sección de la consola a la que devolver al admin al terminar
	// ("apps", "migrations"). Viaja SELLADA con el resto del estado: si llegara en la
	// query del callback, cualquiera podría cambiar adónde aterriza la persona. Aun
	// así, quien la consume la vuelve a validar contra su lista (defensa en
	// profundidad: sellado no es lo mismo que válido).
	Section string `json:"s,omitempty"`
	// Redirect es el redirect_uri EXACTO que se envió en la ida. El canje obliga a
	// repetirlo idéntico (RFC 6749 §4.1.3), así que se guarda en vez de recalcularlo:
	// si la configuración del servicio cambiase entre ida y vuelta, recalcularlo daría
	// un valor distinto y el proveedor rechazaría el canje con un error opaco.
	Redirect string `json:"r"`
	// Requested scopes are sealed with the account choice. A later wizard edit
	// or URL query cannot enable services for an older consent.
	Scopes []string `json:"a,omitempty"`
}

// Flow describe el flujo de un proveedor concreto, resuelto ya con los valores de la
// instancia (las plantillas {campo} del manifiesto vienen sustituidas).
type Flow struct {
	ScopeParameter string
	AuthorizeURL   string
	TokenURL       string
	Scopes         []string
	// ClientID es con lo que Sytue se presenta ante el proveedor. Sale de la config del
	// servicio (app registrada por Sytue ⇒ 1 clic de verdad) o de la instancia (el
	// tenant trae la suya). Nunca del manifiesto, que es una semilla del repo.
	ClientID string
	// Extra son parámetros fijos del proveedor en la URL de autorización (ver
	// manifest.OAuthFlow.AuthorizeParams). No pueden pisar los del estándar.
	Extra map[string]string
}

// Begin construye la URL de autorización y sella el estado. `redirectURI` debe ser
// exactamente la registrada ante el proveedor.
//
// `now` se inyecta en vez de leer el reloj: la caducidad es lógica de seguridad y tiene
// que poder probarse sin dormir el test.
func Begin(c *keycrypto.Cipher, f Flow, tenant, sub, instanceID, redirectURI, section string, now time.Time) (Start, error) {
	if strings.TrimSpace(f.ClientID) == "" {
		return Start{}, fmt.Errorf("consent: falta el client_id con el que presentarse ante el proveedor")
	}
	if strings.TrimSpace(redirectURI) == "" {
		return Start{}, fmt.Errorf("consent: falta redirect_uri")
	}
	sealed, verifier, err := beginState(c, tenant, sub, instanceID, redirectURI, section, f.Scopes, now)
	if err != nil {
		return Start{}, err
	}
	u, err := url.Parse(f.AuthorizeURL)
	if err != nil {
		return Start{}, fmt.Errorf("consent: authorize_url inválida: %w", err)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", f.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", sealed)
	q.Set("code_challenge", challenge(verifier))
	q.Set("code_challenge_method", "S256")
	for _, scope := range f.Scopes {
		if scope == "openid" {
			q.Set("nonce", challenge(verifier))
			break
		}
	}
	for k, val := range f.Extra {
		if q.Get(k) == "" {
			q.Set(k, val)
		}
	}
	if len(f.Scopes) > 0 {
		if f.ScopeParameter == "user_scope" {
			q.Set("user_scope", strings.Join(f.Scopes, ","))
		} else {
			q.Set("scope", strings.Join(f.Scopes, " "))
		}
	}
	u.RawQuery = q.Encode()
	return Start{AuthorizeURL: u.String(), Scopes: f.Scopes, Challenge: challenge(verifier)}, nil
}

// BeginEmbedded protege un consentimiento iniciado por un SDK: el nonce sellado
// se liga a la sesión y se consume como los demás, sin inventar parámetros PKCE
// que el protocolo del SDK no declara. El código se entrega por POST al BFF.
func BeginEmbedded(c *keycrypto.Cipher, tenant, sub, instanceID, section string, now time.Time) (sealed, nonce string, err error) {
	sealed, verifier, err := beginState(c, tenant, sub, instanceID, "", section, nil, now)
	return sealed, challenge(verifier), err
}

func beginState(c *keycrypto.Cipher, tenant, sub, instanceID, redirectURI, section string, scopes []string, now time.Time) (sealed, verifier string, err error) {
	if !c.Enabled() {
		// Sin cifrador no hay forma de sellar el estado, y firmarlo «a medias» sería
		// peor que no ofrecerlo: se prefiere no arrancar el flujo.
		return "", "", fmt.Errorf("consent: no hay cifrador configurado para sellar el state")
	}
	if strings.TrimSpace(sub) == "" {
		return "", "", fmt.Errorf("consent: falta quién empieza el consentimiento")
	}
	verifier, err = randomVerifier()
	if err != nil {
		return "", "", err
	}
	sealed, err = seal(c, state{
		Tenant: tenant, Sub: sub, Instance: instanceID, Verifier: verifier,
		Expires: now.Add(stateTTL).Unix(), Redirect: redirectURI, Section: section, Scopes: append([]string(nil), scopes...),
	})
	return sealed, verifier, err
}

// Callback valida lo que devuelve el proveedor y extrae lo necesario para canjear el
// código: a qué instancia pertenece y con qué verificador. NO hace la petición de canje
// —eso sale por el cliente de egress de la pasarela, que es el único autorizado a
// hablar con terceros— sino que devuelve el material ya validado.
func Callback(c *keycrypto.Cipher, rawState, code, providerErr string, now time.Time) (Redeem, error) {
	st, err := open(c, rawState)
	if err != nil {
		return Redeem{}, err
	}
	// El destino y el actor autenticados se conservan también al caducar para
	// mostrar el error en la app de origen. Nunca se devuelve material de canje
	// (código/verificador) si el estado ha caducado.
	rd := Redeem{Tenant: st.Tenant, Sub: st.Sub, Instance: st.Instance, Section: st.Section}
	if now.Unix() > st.Expires {
		return rd, fmt.Errorf("%w: caducado", ErrState)
	}
	// También al denegar se conserva la vuelta sellada, después de validar el
	// estado. La capa HTTP comprueba además quién inició el consentimiento.
	if e := strings.TrimSpace(providerErr); e != "" {
		if e == "access_denied" {
			return rd, ErrDenied
		}
		return rd, fmt.Errorf("%w: %s", ErrProvider, e)
	}
	if strings.TrimSpace(code) == "" {
		return rd, fmt.Errorf("%w: sin código de autorización", ErrState)
	}
	return Redeem{
		Tenant: st.Tenant, Sub: st.Sub, Instance: st.Instance, Section: st.Section,
		Code: code, Verifier: st.Verifier, RedirectURI: st.Redirect, RequestedScopes: append([]string(nil), st.Scopes...),
	}, nil
}

// Redeem es el material validado con el que canjear el código por tokens.
type Redeem struct {
	Tenant string
	// Sub es quién empezó; ver state.Sub.
	Sub      string
	Instance string
	// Section es la sección de la consola desde la que se empezó (ver state.Section).
	Section         string
	Code            string
	Verifier        string
	RedirectURI     string
	RequestedScopes []string
}

// Challenge es el code_challenge del arranque que selló este estado (Start.Challenge).
func (r Redeem) Challenge() string { return challenge(r.Verifier) }

// Form construye el cuerpo del POST al token endpoint. El client_secret se pasa aparte
// (vacío para clientes públicos) porque no todos los proveedores lo exigen y meterlo
// siempre convertiría un cliente público en uno mal configurado.
func (r Redeem) Form(clientID, clientSecret string) url.Values {
	v := url.Values{}
	v.Set("grant_type", "authorization_code")
	v.Set("code", r.Code)
	v.Set("redirect_uri", r.RedirectURI)
	v.Set("client_id", clientID)
	v.Set("code_verifier", r.Verifier)
	if clientSecret != "" {
		v.Set("client_secret", clientSecret)
	}
	return v
}

// randomVerifier genera un code_verifier del RFC 7636 §4.1: 43 caracteres base64url sin
// relleno a partir de 32 bytes de entropía, el máximo que recomienda el RFC.
func randomVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("consent: generar code_verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// challenge deriva el code_challenge S256 del verificador (RFC 7636 §4.2).
func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// seal cifra el estado y lo deja apto para viajar en una query string.
func seal(c *keycrypto.Cipher, st state) (string, error) {
	raw, err := json.Marshal(st)
	if err != nil {
		return "", fmt.Errorf("consent: serializar state: %w", err)
	}
	enc, err := c.Encrypt(string(raw))
	if err != nil {
		return "", fmt.Errorf("consent: sellar state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(enc)), nil
}

// open revierte seal. Cualquier fallo es ErrState sin más detalle: a quien manipula un
// callback no se le explica en qué paso ha fallado.
func open(c *keycrypto.Cipher, raw string) (state, error) {
	if !c.Enabled() {
		return state{}, fmt.Errorf("%w: no hay cifrador", ErrState)
	}
	dec, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return state{}, fmt.Errorf("%w: base64", ErrState)
	}
	plain, err := c.Decrypt(string(dec))
	if err != nil {
		return state{}, fmt.Errorf("%w: no autentica", ErrState)
	}
	var st state
	if err := json.Unmarshal([]byte(plain), &st); err != nil {
		return state{}, fmt.Errorf("%w: JSON", ErrState)
	}
	if st.Tenant == "" || st.Sub == "" || st.Instance == "" || st.Verifier == "" {
		return state{}, fmt.Errorf("%w: incompleto", ErrState)
	}
	return st, nil
}
