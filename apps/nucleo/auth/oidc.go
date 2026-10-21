// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Los clientes OIDC de las apps nativas, los dos PÚBLICOS y sin secreto —un binario
// instalado no puede guardar uno—: lo que los protege es PKCE y la comprobación exacta
// del redirect que hace el IdP.
//
// Son DOS y no uno porque el camino de vuelta del navegador es distinto, que es lo que
// RFC 8252 separa: el escritorio vuelve a un servidor efímero del propio equipo (§7.3,
// `sytue-desktop`, migración 0069) y el móvil al esquema propio de cada app (§7.1,
// `sytue-mobile`, migración 0071, con los catorce esquemas registrados uno a uno).
const (
	ClientID      = "sytue-desktop"
	ClientIDMovil = "sytue-mobile"
	loopback      = "127.0.0.1:8921"
	redirectURI   = "http://" + loopback + "/auth/callback"
	scopes        = "openid profile email offline_access"
	esperaLogin   = 3 * time.Minute
	limiteCuerpo  = 1 << 20
)

// OIDC habla con Sytue ID en nombre de una app nativa.
type OIDC struct {
	// Issuer es https://id.<apex>.
	Issuer string
	// Cliente es el client_id registrado. Vacío: ClientID, el del escritorio.
	Cliente string
	// HTTP es el cliente para discovery y token; nil usa uno con 10 s de plazo.
	HTTP *http.Client
	// Open abre una URL en el navegador del sistema. Lo pone cada `cmd/`: no hay una
	// forma portable de hacerlo y aquí no se adivina.
	Open func(url string) error
	// MaxAge, si es > 0, va como `max_age` en el login: Sytue ID vuelve a pedir la
	// contraseña si la sesión del navegador es más vieja que eso. Lo pone el bloqueo
	// en un equipo que no sabe verificar al dueño (fyne/sesion/bloqueo.go).
	MaxAge int

	// descMu guarda el discovery: `endpoints` se escribe una vez, con él puesto, y solo
	// se lee después de que `descubrir` haya devuelto nil.
	descMu      sync.Mutex
	descubierto bool
	endpoints   struct{ Authorization, Token, Revocation string }

	mu      sync.Mutex
	espera  chan url.Values // login por esquema propio en marcha; nil si no hay ninguno
	destino string          // y su redirect_uri exacto
}

func (o *OIDC) clientID() string {
	if o.Cliente != "" {
		return o.Cliente
	}
	return ClientID
}

func (o *OIDC) cliente() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// descubrir lee el discovery y lo recuerda SOLO si ha salido bien. Sin rutas de repuesto
// compuestas a mano: si el IdP no contesta aquí, tampoco va a contestar en /token, y un
// error claro vale más.
//
// Antes iba en un `sync.Once`, que recuerda también el ERROR: una app que arrancaba sin
// red —o a la que se le cancelaba el contexto de esa primera llamada— se quedaba sin
// login ni renovación hasta reiniciarla, aunque la red volviera. Es el mismo fallo que la
// auditoría del 2026-09-21 corrigió en la web de Gio (G2), aquí en el cliente nativo. El
// cerrojo se mantiene durante la petición: quien llega mientras tanto espera y se
// encuentra el resultado, como con el `Once`.
func (o *OIDC) descubrir(ctx context.Context) error {
	o.descMu.Lock()
	defer o.descMu.Unlock()
	if o.descubierto {
		return nil
	}
	var doc struct {
		Issuer        string `json:"issuer"`
		Authorization string `json:"authorization_endpoint"`
		Token         string `json:"token_endpoint"`
		Revocation    string `json:"revocation_endpoint"`
	}
	base := strings.TrimRight(o.Issuer, "/")
	if err := o.json(ctx, http.MethodGet, base+"/.well-known/openid-configuration", nil, &doc); err != nil {
		return err
	}
	// OIDC Discovery §4.3: el issuer del documento tiene que ser el que se pidió.
	if strings.TrimRight(doc.Issuer, "/") != base || doc.Authorization == "" || doc.Token == "" {
		return fmt.Errorf("auth: el discovery de %s no es de ese emisor o viene incompleto", base)
	}
	o.endpoints.Authorization, o.endpoints.Token, o.endpoints.Revocation = doc.Authorization, doc.Token, doc.Revocation
	o.descubierto = true
	return nil
}

// Login hace el flujo completo —Authorization Code + PKCE con redirect al loopback,
// como `gcloud auth login`— y BLOQUEA hasta que el navegador vuelve, ctx se cancela o
// pasan tres minutos. Se llama desde una goroutine, nunca desde Layout.
func (o *OIDC) Login(ctx context.Context) (Tokens, error) {
	if err := o.descubrir(ctx); err != nil {
		return Tokens{}, err
	}
	secreto := secretoDeDispositivo()
	autorizar, verifier, state := o.autorizar(redirectURI, secreto)

	// Keep the registered fixed OIDC redirect while sharing the listener lifecycle
	// and browser return handling with other native SSO clients.
	waitCtx, cancel := context.WithTimeout(ctx, esperaLogin)
	defer cancel()
	query, err := WaitLoopback(waitCtx, LoopbackCallback{
		Address: loopback, Path: "/auth/callback",
		Open:     func(string) error { return o.Open(autorizar) },
		Validate: func(q url.Values) error { _, err := comprobar(q, state); return err },
		Success:  "Sytue: sesión iniciada. Ya puedes cerrar esta pestaña y volver a la aplicación.",
		Failure:  "Sytue: no se ha podido iniciar sesión. Vuelve a la aplicación e inténtalo de nuevo.",
	})
	if err != nil {
		return Tokens{}, err
	}
	code, err := comprobar(query, state)
	if err != nil {
		return Tokens{}, err
	}
	return o.canjear(ctx, code, verifier, redirectURI, secreto)
}

// LoginEsquema es el login del MÓVIL: RFC 8252 §7.1. Ahí no hay puerto que escuchar —una
// app no puede reservarse uno en un teléfono—, así que el navegador del sistema vuelve al
// esquema propio de la app (`sytue-mail://auth/callback`), el sistema reabre la app con
// ese enlace, y ese enlace entra aquí por Vuelta. Como Login, BLOQUEA hasta que vuelve,
// ctx se cancela o pasan tres minutos: se llama desde una goroutine, nunca desde Layout.
//
// El navegador del SISTEMA y no una vista dentro de la app (§8.1): así la contraseña se
// escribe en una ventana cuyo dominio y cuyo candado se ven, y la sesión que ya hubiera
// con Sytue ID se reutiliza.
func (o *OIDC) LoginEsquema(ctx context.Context, redirect string) (Tokens, error) {
	if err := o.descubrir(ctx); err != nil {
		return Tokens{}, err
	}
	llegada := make(chan url.Values, 1)
	o.mu.Lock()
	if o.espera != nil {
		o.mu.Unlock()
		return Tokens{}, errors.New("auth: ya hay un inicio de sesión abierto en el navegador")
	}
	o.espera, o.destino = llegada, redirect
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		o.espera, o.destino = nil, ""
		o.mu.Unlock()
	}()

	secreto := secretoDeDispositivo()
	autorizar, verifier, state := o.autorizar(redirect, secreto)
	if err := o.Open(autorizar); err != nil {
		return Tokens{}, fmt.Errorf("auth: no se pudo abrir el navegador: %w", err)
	}
	select {
	case q := <-llegada:
		code, err := comprobar(q, state)
		if err != nil {
			return Tokens{}, err
		}
		return o.canjear(ctx, code, verifier, redirect, secreto)
	case <-ctx.Done():
		return Tokens{}, ctx.Err()
	case <-time.After(esperaLogin):
		return Tokens{}, errors.New("auth: tres minutos sin volver del navegador")
	}
}

// Vuelta entrega el enlace con el que el sistema acaba de reabrir la app. Devuelve true
// si era la vuelta del login en marcha, y entonces NO es un deep link de producto: los
// dos llegan por el mismo sitio (app.URLEvent) y hay que separarlos una sola vez.
//
// La comparación es EXACTA —esquema, host y ruta— igual que la que hace el IdP con el
// redirect registrado. Sin login en marcha no se acepta ninguna: un enlace suelto a
// `…://auth/callback` no puede colar un código de otro.
func (o *OIDC) Vuelta(u *url.URL) bool {
	if u == nil {
		return false
	}
	o.mu.Lock()
	llegada, destino := o.espera, o.destino
	o.mu.Unlock()
	if llegada == nil || (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String() != destino {
		return false
	}
	select {
	case llegada <- u.Query():
	default: // ya había llegado una; la segunda se descarta, no se reparte como deep link
	}
	return true
}

// autorizar compone la URL de /authorize y devuelve con qué hay que comprobar la vuelta:
// el verificador de PKCE (RFC 7636) y el `state`. Con secreto de dispositivo, lleva su
// hash (NAT-02, ver secretoDeDispositivo).
func (o *OIDC) autorizar(redirect, secretoDispositivo string) (destino, verifier, state string) {
	verifier, state = aleatorio(32), aleatorio(16)
	reto := sha256.Sum256([]byte(verifier))
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {o.clientID()},
		"redirect_uri":          {redirect},
		"scope":                 {scopes},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(reto[:])},
		"code_challenge_method": {"S256"},
	}
	if o.MaxAge > 0 {
		q.Set("max_age", strconv.Itoa(o.MaxAge))
	}
	if secretoDispositivo != "" {
		suma := sha256.Sum256([]byte(secretoDispositivo))
		q.Set("sytue_device", base64.RawURLEncoding.EncodeToString(suma[:]))
	}
	destino = o.endpoints.Authorization + "?" + q.Encode()
	return destino, verifier, state
}

// comprobar valida la vuelta del navegador y saca el código.
func comprobar(q url.Values, state string) (string, error) {
	switch {
	case subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1:
		return "", errors.New("auth: la vuelta del navegador no es del login que abrió esta app")
	case q.Get("error") != "":
		return "", fmt.Errorf("auth: Sytue ID ha rechazado el login: %s", q.Get("error"))
	case q.Get("code") == "":
		return "", errors.New("auth: la vuelta del navegador no trae código")
	}
	return q.Get("code"), nil
}

// canjear cambia el código por tokens (RFC 6749 §4.1.3 con el verificador de PKCE).
// Con secreto de dispositivo, lo presenta: el código está atado a su hash y sin él
// Sytue ID no lo canjea.
func (o *OIDC) canjear(ctx context.Context, code, verifier, redirect, secretoDispositivo string) (Tokens, error) {
	forma := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirect},
		"client_id":     {o.clientID()},
		"code_verifier": {verifier},
	}
	if secretoDispositivo != "" {
		forma.Set("sytue_device_secret", secretoDispositivo)
	}
	return o.pedir(ctx, forma)
}

// cuentaDispositivo es la entrada del almacén seguro con el secreto de este dispositivo.
// No empieza por «notes-», así que cerrar sesión no la borra: es del equipo, no de la
// sesión, y por eso Sytue ID no vuelve a preguntar al entrar otra vez.
const cuentaDispositivo = "dispositivo"

// secretoDeDispositivo es lo que identifica a ESTE dispositivo ante Sytue ID (NAT-02):
// la primera vez que una app nativa entra desde un dispositivo, Sytue ID pide
// consentimiento, y después ya no. A /authorize solo viaja su hash; el secreto, solo en
// el canje, como el verificador de PKCE. Otra app del equipo no lo puede leer (el
// almacén seguro es por usuario y por app) y sin él no canjea un código atado a este
// dispositivo. Sin almacén seguro devuelve "" y se entra igual: Sytue ID pregunta cada
// vez.
func secretoDeDispositivo() string {
	guardado, err := LeerSecreto(cuentaDispositivo)
	if err != nil {
		return ""
	}
	if len(guardado) == 0 {
		guardado = make([]byte, 32)
		_, _ = rand.Read(guardado) // crypto/rand.Read no falla
		if GuardarSecreto(cuentaDispositivo, guardado) != nil {
			return ""
		}
	}
	return base64.RawURLEncoding.EncodeToString(guardado)
}

// Renew es el Renewer nativo: cambia el refresh token por tokens nuevos.
func (o *OIDC) Renew(ctx context.Context, actuales Tokens) (Tokens, error) {
	if actuales.Refresh == "" {
		return Tokens{}, ErrLoginRequired
	}
	if (actuales.Issuer != "" && strings.TrimRight(actuales.Issuer, "/") != strings.TrimRight(o.Issuer, "/")) ||
		(actuales.Cliente != "" && actuales.Cliente != o.clientID()) {
		return Tokens{}, ErrSesionAjena
	}
	if err := o.descubrir(ctx); err != nil {
		return Tokens{}, err
	}
	return o.pedir(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {actuales.Refresh},
		"client_id":     {o.clientID()},
	})
}

// Revocar anula en Sytue ID ese refresh token (RFC 7009) y, con él, su familia entera. Es
// lo que hace que «Cerrar sesión» cierre de verdad: sin esto el token se borraba de este
// equipo y seguía valiendo para quien tuviera una copia. Un IdP que no publica
// `revocation_endpoint` no es un error: no hay a quién decírselo.
func (o *OIDC) Revocar(ctx context.Context, refresh string) error {
	if refresh == "" {
		return nil
	}
	if err := o.descubrir(ctx); err != nil {
		return err
	}
	if o.endpoints.Revocation == "" {
		return nil
	}
	return o.json(ctx, http.MethodPost, o.endpoints.Revocation, url.Values{
		"token":           {refresh},
		"token_type_hint": {"refresh_token"},
		"client_id":       {o.clientID()},
	}, nil)
}

func (o *OIDC) pedir(ctx context.Context, forma url.Values) (Tokens, error) {
	var doc struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int64  `json:"expires_in"`
		ID      string `json:"id_token"`
	}
	if err := o.json(ctx, http.MethodPost, o.endpoints.Token, forma, &doc); err != nil {
		return Tokens{}, err
	}
	if doc.Access == "" {
		return Tokens{}, errors.New("auth: el IdP ha contestado sin access_token")
	}
	// Del ID token, sin verificar la firma (ver `Reclamos`): quién es y la apariencia por
	// defecto de su organización, que solo viaja ahí.
	var id struct {
		Sub      string `json:"sub"`
		OrgColor string `json:"org_color"`
		OrgForma string `json:"org_forma"`
	}
	conID := reclamosDe(doc.ID, &id)
	return Tokens{
		Issuer:     strings.TrimRight(o.Issuer, "/"),
		Cliente:    o.clientID(),
		Access:     doc.Access,
		Refresh:    doc.Refresh,
		Subject:    id.Sub,
		Expiry:     time.Now().Add(time.Duration(doc.Expires) * time.Second),
		OrgColor:   nombreDeApariencia(id.OrgColor),
		OrgForma:   nombreDeApariencia(id.OrgForma),
		conIDToken: conID,
	}, nil
}

func (o *OIDC) json(ctx context.Context, metodo, destino string, forma url.Values, en any) error {
	var cuerpo io.Reader
	if forma != nil {
		cuerpo = strings.NewReader(forma.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, metodo, destino, cuerpo)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if forma != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := o.cliente().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, limiteCuerpo))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		// RFC 6749 §5.2: invalid_grant es «ese refresh token ya no vale» —caducado,
		// revocado o ya rotado—. Todo lo demás (5xx, red) es pasajero y NO cierra la
		// sesión.
		var fallo struct {
			Error       string `json:"error"`
			Descripcion string `json:"error_description"`
		}
		if json.Unmarshal(datos, &fallo) == nil && fallo.Error == "invalid_grant" {
			// Con el porqué de Sytue ID («código inválido, expirado o ya usado», «PKCE»…):
			// sin él, un canje rechazado y una sesión caducada se leían igual.
			if fallo.Descripcion != "" {
				return fmt.Errorf("%w (%s)", ErrLoginRequired, fallo.Descripcion)
			}
			return ErrLoginRequired
		}
		return fmt.Errorf("auth: %s ha respondido HTTP %d", destino, resp.StatusCode)
	}
	if en == nil {
		return nil // quien llama no espera cuerpo (la revocación contesta vacío)
	}
	return json.Unmarshal(datos, en)
}

func aleatorio(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand.Read no falla: aborta el proceso si no hay entropía
	return base64.RawURLEncoding.EncodeToString(b)
}
