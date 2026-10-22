// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js && !android && !ios && !(darwin && tienda)

package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// La máquina de estados de la sesión, transición a transición, con reloj de mentira.
func TestSessionTransiciones(t *testing.T) {
	ahora := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	var (
		renovaciones int
		respuesta    Tokens
		fallo        error
		guardados    []Tokens
	)
	s := NewSession(Tokens{Refresh: "r0", Subject: "ada"}, func(_ context.Context, actuales Tokens) (Tokens, error) {
		renovaciones++
		if actuales.Refresh == "" {
			return Tokens{}, ErrLoginRequired
		}
		return respuesta, fallo
	}, func(tk Tokens) error { guardados = append(guardados, tk); return nil })
	s.ahora = func() time.Time { return ahora }
	pedir := func(quiero string, quieroErr error) {
		t.Helper()
		got, err := s.AccessToken(context.Background())
		if got != quiero || !errors.Is(err, quieroErr) {
			t.Fatalf("AccessToken = %q, %v; quiero %q, %v", got, err, quiero, quieroErr)
		}
	}

	// Arranque: hay refresh guardado y ningún access → renueva.
	respuesta = Tokens{Access: "a1", Refresh: "r1", Expiry: ahora.Add(5 * time.Minute)}
	pedir("a1", nil)
	// Vigente → no toca al IdP.
	pedir("a1", nil)
	if renovaciones != 1 {
		t.Fatalf("%d renovaciones con el token vigente, quiero 1", renovaciones)
	}
	// Dentro del margen (quedan 29 s) → renueva. El IdP no rota el refresh ni repite
	// el subject: se conservan.
	ahora = ahora.Add(5*time.Minute - 29*time.Second)
	respuesta = Tokens{Access: "a2", Expiry: ahora.Add(5 * time.Minute)}
	pedir("a2", nil)
	if u := guardados[len(guardados)-1]; u.Refresh != "r1" || u.Subject != "ada" {
		t.Fatalf("al renovar se ha perdido el refresh o el subject: %+v", u)
	}
	// Fallo de red con el token en el margen pero aún vivo → se sigue con él.
	ahora = ahora.Add(5*time.Minute - 10*time.Second)
	fallo = errors.New("sin red")
	pedir("a2", nil)
	// Fallo de red con el token caducado → error, pero la sesión NO se cierra.
	ahora = ahora.Add(time.Minute)
	pedir("", fallo)
	if s.Subject() != "ada" {
		t.Fatal("un fallo de red ha cerrado la sesión")
	}
	// Vuelve la red.
	fallo, respuesta = nil, Tokens{Access: "a3", Expiry: ahora.Add(5 * time.Minute)}
	pedir("a3", nil)
	// Un 401 sobre un token VIEJO no invalida el actual; sobre el actual, sí.
	antes := renovaciones
	s.Invalidate("a2")
	pedir("a3", nil)
	s.Invalidate("a3")
	respuesta = Tokens{Access: "a4", Expiry: ahora.Add(5 * time.Minute)}
	pedir("a4", nil)
	if renovaciones != antes+1 {
		t.Fatalf("%d renovaciones tras los dos Invalidate, quiero 1", renovaciones-antes)
	}
	// El IdP dice que el refresh ya no vale → login, y lo guardado se borra.
	s.Invalidate("a4")
	fallo = ErrLoginRequired
	pedir("", ErrLoginRequired)
	if u := guardados[len(guardados)-1]; u != (Tokens{}) || s.Subject() != "" {
		t.Fatalf("tras invalid_grant queda sesión: %+v", u)
	}
	// Y sin refresh no hay nada que renovar.
	fallo = nil
	pedir("", ErrLoginRequired)
}

func TestSessionUnaSolaRenovacionEnVuelo(t *testing.T) {
	var renovaciones int
	s := NewSession(Tokens{Refresh: "r"}, func(context.Context, Tokens) (Tokens, error) {
		renovaciones++ // sin cerrojo propio: si dos entraran a la vez, -race lo canta
		time.Sleep(20 * time.Millisecond)
		return Tokens{Access: "a", Expiry: time.Now().Add(time.Hour)}, nil
	}, nil)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if tok, err := s.AccessToken(context.Background()); tok != "a" || err != nil {
				t.Errorf("AccessToken = %q, %v", tok, err)
			}
		})
	}
	wg.Wait()
	if renovaciones != 1 {
		t.Fatalf("%d renovaciones para 8 peticiones simultáneas, quiero 1", renovaciones)
	}
}

// idpFalso es un Sytue ID de mentira que SÍ comprueba PKCE, redirect y cliente.
type idpFalso struct {
	*httptest.Server
	reto        string // code_challenge del último /authorize
	refreshVale string
	tokenStatus int
	issuerFalso string
	refrescadas int
	// cliente y redirect son los REGISTRADOS: el IdP compara la cadena entera, que es
	// justo lo que impide que otra app se lleve el código. Por defecto, los del
	// escritorio.
	cliente  string
	redirect string
	// lo último que llegó de dispositivo: el hash a /authorize y el secreto al canje.
	dispositivo, secretoVisto string
}

func nuevoIDP(t *testing.T) *idpFalso {
	t.Helper()
	idp := &idpFalso{refreshVale: "r1", tokenStatus: http.StatusOK, cliente: ClientID, redirect: redirectURI}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		issuer := idp.URL
		if idp.issuerFalso != "" {
			issuer = idp.issuerFalso
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": issuer, "authorization_endpoint": idp.URL + "/authorize", "token_endpoint": idp.URL + "/token",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if idp.tokenStatus != http.StatusOK {
			w.WriteHeader(idp.tokenStatus)
			return
		}
		_ = r.ParseForm()
		ok := r.PostForm.Get("client_id") == idp.cliente
		switch r.PostForm.Get("grant_type") {
		case "authorization_code":
			idp.secretoVisto = r.PostForm.Get("sytue_device_secret")
			suma := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
			ok = ok && r.PostForm.Get("code") == "c0d1g0" && r.PostForm.Get("redirect_uri") == idp.redirect &&
				base64.RawURLEncoding.EncodeToString(suma[:]) == idp.reto
		case "refresh_token":
			idp.refrescadas++
			ok = ok && r.PostForm.Get("refresh_token") == idp.refreshVale
		default:
			ok = false
		}
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token inválido o expirado"}`))
			return
		}
		reclamos := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"ada"}`))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "acceso", "refresh_token": "r2", "expires_in": 300, "id_token": "x." + reclamos + ".y",
		})
	})
	idp.Server = httptest.NewServer(mux)
	t.Cleanup(idp.Close)
	return idp
}

// navegador hace de persona: recibe la URL de /authorize y vuelve al loopback.
func (idp *idpFalso) navegador(t *testing.T, cambiar func(vuelta url.Values)) func(string) error {
	return func(autorizar string) error {
		u, err := url.Parse(autorizar)
		if err != nil {
			return err
		}
		q := u.Query()
		if q.Get("client_id") != idp.cliente || q.Get("redirect_uri") != idp.redirect ||
			q.Get("code_challenge_method") != "S256" || q.Get("response_type") != "code" ||
			!strings.Contains(q.Get("scope"), "offline_access") {
			t.Errorf("/authorize mal formado: %v", q)
		}
		idp.reto = q.Get("code_challenge")
		idp.dispositivo = q.Get("sytue_device")
		vuelta := url.Values{"code": {"c0d1g0"}, "state": {q.Get("state")}}
		if cambiar != nil {
			cambiar(vuelta)
		}
		go func() {
			resp, err := http.Get(redirectURI + "?" + vuelta.Encode())
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
		return nil
	}
}

func TestLoginPorLoopback(t *testing.T) {
	idp := nuevoIDP(t)
	o := &OIDC{Issuer: idp.URL, Open: idp.navegador(t, nil)}
	tok, err := o.Login(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Access != "acceso" || tok.Refresh != "r2" || tok.Subject != "ada" || time.Until(tok.Expiry) < 4*time.Minute {
		t.Fatalf("tokens = %+v", tok)
	}
}

func TestLoginRechazaUnaVueltaAjena(t *testing.T) {
	for nombre, cambiar := range map[string]func(url.Values){
		"state de otro flujo": func(v url.Values) { v.Set("state", "el-de-otro") },
		"error del IdP":       func(v url.Values) { v.Set("error", "access_denied") },
		"sin código":          func(v url.Values) { v.Del("code") },
	} {
		idp := nuevoIDP(t)
		o := &OIDC{Issuer: idp.URL, Open: idp.navegador(t, cambiar)}
		if tok, err := o.Login(context.Background()); err == nil {
			t.Errorf("%s: ha dado tokens: %+v", nombre, tok)
		}
	}
}

func TestLoginSeCancela(t *testing.T) {
	idp := nuevoIDP(t)
	ctx, cancelar := context.WithCancel(context.Background())
	o := &OIDC{Issuer: idp.URL, Open: func(string) error { cancelar(); return nil }}
	if _, err := o.Login(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, quiero context.Canceled", err)
	}
	// Y suelta el puerto: el siguiente login tiene que poder escuchar.
	o = &OIDC{Issuer: idp.URL, Open: idp.navegador(t, nil)}
	if _, err := o.Login(context.Background()); err != nil {
		t.Fatalf("el login cancelado se ha quedado con el puerto: %v", err)
	}
}

func TestRenew(t *testing.T) {
	idp := nuevoIDP(t)
	o := &OIDC{Issuer: idp.URL}
	ctx := context.Background()
	if tok, err := o.Renew(ctx, Tokens{Refresh: "r1"}); err != nil || tok.Access != "acceso" {
		t.Fatalf("Renew = %+v, %v", tok, err)
	}
	// Refresh revocado o ya rotado: invalid_grant → login.
	// Y dice POR QUÉ: un canje rechazado y una sesión caducada no se leen igual.
	if _, err := o.Renew(ctx, Tokens{Refresh: "caducado"}); !errors.Is(err, ErrLoginRequired) ||
		!strings.Contains(err.Error(), "refresh token inválido o expirado") {
		t.Fatalf("invalid_grant ha dado %v, quiero ErrLoginRequired con el porqué de Sytue ID", err)
	}
	// El IdP caído NO es «inicia sesión otra vez».
	idp.tokenStatus = http.StatusBadGateway
	if _, err := o.Renew(ctx, Tokens{Refresh: "r1"}); err == nil || errors.Is(err, ErrLoginRequired) {
		t.Fatalf("un 502 ha dado %v, quiero un error pasajero", err)
	}
	// Sin refresh no se llega ni a preguntar.
	antes := idp.refrescadas
	if _, err := o.Renew(ctx, Tokens{}); !errors.Is(err, ErrLoginRequired) || idp.refrescadas != antes {
		t.Fatalf("sin refresh: %v, %d peticiones", err, idp.refrescadas-antes)
	}
}

func TestDiscoveryDeOtroEmisor(t *testing.T) {
	idp := nuevoIDP(t)
	idp.issuerFalso = "https://id.no-somos-nosotros.example"
	if _, err := (&OIDC{Issuer: idp.URL}).Renew(context.Background(), Tokens{Refresh: "r1"}); err == nil {
		t.Fatal("se ha aceptado un discovery cuyo issuer no es el pedido")
	}
}

// TestUnTropiezoDelDiscoveryNoDejaSinSesion: el discovery iba en un `sync.Once`, que
// recuerda también el error. Un primer intento fallido —arrancar sin red, un contexto
// cancelado, un IdP a medio desplegar— dejaba a la app sin login ni renovación hasta
// reiniciarla. Es el G2 de la auditoría del 2026-09-21, que se corrigió en la web de Gio
// y seguía en el cliente nativo (NAT-10).
func TestUnTropiezoDelDiscoveryNoDejaSinSesion(t *testing.T) {
	idp := nuevoIDP(t)
	o := &OIDC{Issuer: idp.URL}
	cancelado, cancelar := context.WithCancel(context.Background())
	cancelar()
	if _, err := o.Renew(cancelado, Tokens{Refresh: "r1"}); err == nil {
		t.Fatal("con el contexto cancelado no puede haber renovado")
	}
	idp.issuerFalso = "https://id.no-somos-nosotros.example"
	if _, err := o.Renew(context.Background(), Tokens{Refresh: "r1"}); err == nil {
		t.Fatal("se ha aceptado un discovery cuyo issuer no es el pedido")
	}
	idp.issuerFalso = ""
	nuevos, err := o.Renew(context.Background(), Tokens{Refresh: "r1"})
	if err != nil || nuevos.Access != "acceso" {
		t.Fatalf("pasado el tropiezo, la misma app sigue sin renovar: %+v, %v", nuevos, err)
	}
}

func TestLlavero(t *testing.T) {
	// En Linux, sin sesión en el llavero se mira el fichero cifrado: que no sea el de
	// quien ejecuta los tests.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	keyring.MockInit()
	if tok, err := Load(); err != nil || tok != (Tokens{}) {
		t.Fatalf("llavero vacío: %+v, %v", tok, err)
	}
	if err := Save(Tokens{Access: "NO-SE-GUARDA", Refresh: "r", Subject: "ada", Expiry: time.Now()}); err != nil {
		t.Fatal(err)
	}
	crudo, _ := keyring.Get(llaveroServicio, llaveroCuenta)
	if strings.Contains(crudo, "NO-SE-GUARDA") {
		t.Fatal("el access token ha llegado al llavero")
	}
	if tok, err := Load(); err != nil || tok != (Tokens{Refresh: "r", Subject: "ada"}) {
		t.Fatalf("Load = %+v, %v", tok, err)
	}
	// Cerrar sesión borra la entrada; borrar dos veces no es un error.
	for range 2 {
		if err := Save(Tokens{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := keyring.Get(llaveroServicio, llaveroCuenta); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("tras cerrar sesión queda entrada: %v", err)
	}
}

// abreYVuelve hace de navegador DEL SISTEMA en un teléfono: lee la URL de /authorize y
// reabre la app con el enlace de vuelta, que es lo que hace Android con el intent-filter
// del esquema (RFC 8252 §7.1).
func (idp *idpFalso) abreYVuelve(t *testing.T, o *OIDC, cambiar func(url.Values)) func(string) error {
	t.Helper()
	return func(autorizar string) error {
		u, err := url.Parse(autorizar)
		if err != nil {
			return err
		}
		q := u.Query()
		if q.Get("client_id") != idp.cliente || q.Get("redirect_uri") != idp.redirect ||
			q.Get("code_challenge_method") != "S256" || q.Get("response_type") != "code" ||
			!strings.Contains(q.Get("scope"), "offline_access") {
			t.Errorf("/authorize mal formado: %v", q)
		}
		idp.reto = q.Get("code_challenge")
		vuelta := url.Values{"code": {"c0d1g0"}, "state": {q.Get("state")}}
		if cambiar != nil {
			cambiar(vuelta)
		}
		enlace, err := url.Parse(idp.redirect + "?" + vuelta.Encode())
		if err != nil {
			return err
		}
		if !o.Vuelta(enlace) {
			t.Errorf("Vuelta ha rechazado la vuelta del propio login: %s", enlace)
		}
		return nil
	}
}

// El login del MÓVIL de punta a punta: PKCE contra el cliente `sytue-mobile` y vuelta
// por el esquema propio de la app, no por un puerto del equipo.
func TestLoginPorEsquemaPropio(t *testing.T) {
	const redirect = "sytue-mail://auth/callback"
	idp := nuevoIDP(t)
	idp.cliente, idp.redirect = ClientIDMovil, redirect
	o := &OIDC{Issuer: idp.URL, Cliente: ClientIDMovil}
	o.Open = idp.abreYVuelve(t, o, nil)

	tok, err := o.LoginEsquema(context.Background(), redirect)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Access != "acceso" || tok.Refresh != "r2" || tok.Subject != "ada" || time.Until(tok.Expiry) < 4*time.Minute {
		t.Fatalf("tokens = %+v", tok)
	}
	// Y el flujo se cierra: la vuelta ya no vale para nada, y se puede volver a entrar.
	enlace, _ := url.Parse(redirect + "?code=c0d1g0&state=x")
	if o.Vuelta(enlace) {
		t.Fatal("la vuelta sigue aceptándose con el login ya terminado")
	}
}

// Vuelta tiene que separar la vuelta del login de un deep link de producto: los dos
// llegan por app.URLEvent y solo hay un sitio donde decidirlo.
func TestVueltaSoloEsLaDelLoginEnMarcha(t *testing.T) {
	const redirect = "sytue-drive://auth/callback"
	idp := nuevoIDP(t)
	idp.cliente, idp.redirect = ClientIDMovil, redirect
	o := &OIDC{Issuer: idp.URL, Cliente: ClientIDMovil}

	// Sin login en marcha no se acepta ni el redirect exacto, ni un enlace vacío —que
	// es lo que quedaría si la comparación fuera lo único que decide.
	enlace, _ := url.Parse(redirect + "?code=c0d1g0&state=x")
	vacio, _ := url.Parse("")
	if o.Vuelta(enlace) || o.Vuelta(vacio) {
		t.Fatal("se ha aceptado una vuelta sin ningún login en marcha")
	}
	ajenos := []string{
		"sytue-mail://auth/callback?code=c0d1g0",   // el esquema de OTRA app
		"sytue-drive://carpeta/7",                  // un deep link de producto
		"sytue-drive://auth/callback/mas?code=c0d", // una ruta que solo se parece
		"https://drive.sytue.eu/auth/callback",     // un enlace web
	}
	bueno := idp.abreYVuelve(t, o, nil)
	o.Open = func(autorizar string) error {
		for _, crudo := range ajenos {
			u, err := url.Parse(crudo)
			if err != nil {
				return err
			}
			if o.Vuelta(u) {
				t.Errorf("%s se ha tragado como vuelta del login", crudo)
			}
		}
		return bueno(autorizar)
	}
	if _, err := o.LoginEsquema(context.Background(), redirect); err != nil {
		t.Fatal(err)
	}
}

func TestLoginPorEsquemaRechazaUnaVueltaAjena(t *testing.T) {
	const redirect = "sytue-notes://auth/callback"
	for nombre, cambiar := range map[string]func(url.Values){
		"state de otro flujo": func(v url.Values) { v.Set("state", "el-de-otro") },
		"error del IdP":       func(v url.Values) { v.Set("error", "access_denied") },
		"sin código":          func(v url.Values) { v.Del("code") },
	} {
		idp := nuevoIDP(t)
		idp.cliente, idp.redirect = ClientIDMovil, redirect
		o := &OIDC{Issuer: idp.URL, Cliente: ClientIDMovil}
		o.Open = idp.abreYVuelve(t, o, cambiar)
		if tok, err := o.LoginEsquema(context.Background(), redirect); err == nil {
			t.Errorf("%s: ha dado tokens: %+v", nombre, tok)
		}
	}
}

// El sobre con el que Android guarda la sesión cifrada: ida y vuelta, y que un fichero
// tocado se note en vez de devolver basura. Es lo único del Keystore que se puede
// probar sin un teléfono; lo demás son llamadas a Android.
func TestSobreDeLaSesionGuardada(t *testing.T) {
	iv, cifrado := []byte("123456789012"), []byte("lo que sea, ya cifrado")
	sobre, err := empaquetar(iv, cifrado)
	if err != nil {
		t.Fatal(err)
	}
	vueltaIV, vueltaCifrado, err := desempaquetar(sobre)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(vueltaIV, iv) || !bytes.Equal(vueltaCifrado, cifrado) {
		t.Fatalf("ida y vuelta: IV %q, criptograma %q", vueltaIV, vueltaCifrado)
	}
	// Cortado antes de tener IV entero y al menos un byte de criptograma: error, no un
	// IV inventado a partir de lo que haya. Más allá de ahí el sobre SÍ es válido —su
	// longitud no dice cuánto criptograma debería haber— y quien lo caza es la etiqueta
	// de autenticación de GCM al descifrar, que es su trabajo.
	for n := range 1 + len(iv) + 1 {
		if _, _, err := desempaquetar(sobre[:n]); err == nil {
			t.Errorf("un sobre de %d bytes (de %d) se ha aceptado", n, len(sobre))
		}
	}
	// Y no se guarda un sobre que no se podría abrir.
	if _, err := empaquetar(nil, cifrado); err == nil {
		t.Error("se ha empaquetado sin IV")
	}
	if _, err := empaquetar(make([]byte, 256), cifrado); err == nil {
		t.Error("se ha empaquetado un IV que no cabe en el byte de longitud")
	}
	if _, err := empaquetar(iv, nil); err == nil {
		t.Error("se ha empaquetado sin criptograma")
	}
}

// Lo que llega al almacén seguro es el refresh token y de quién es, NUNCA el access
// token: es el mismo formato en el llavero, el Keystore y el Keychain.
func TestFormatoDeLoGuardado(t *testing.T) {
	crudo, err := serializar(Tokens{Access: "NO-SE-GUARDA", Refresh: "r", Subject: "ada", Expiry: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(crudo), "NO-SE-GUARDA") {
		t.Fatalf("el access token ha llegado a lo guardado: %s", crudo)
	}
	if tok, err := recuperar(crudo); err != nil || tok != (Tokens{Refresh: "r", Subject: "ada"}) {
		t.Fatalf("recuperar = %+v, %v", tok, err)
	}
}

// TestLosGruposSalenDelAccessTokenSinLaOrganizacion: el selector de «Compartir con un
// grupo» ofrece los grupos del reclamo `groups`, y no el pseudo-grupo `org/<slug>`, con
// el que no se comparte nada.
func TestLosGruposSalenDelAccessTokenSinLaOrganizacion(t *testing.T) {
	carga := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"u1","groups":["/org/acme","Ventas","org/acme","Dirección"]}`))
	s := NewSession(Tokens{Access: "cabecera." + carga + ".firma"}, nil, nil)
	if got := s.Grupos(); strings.Join(got, ",") != "Ventas,Dirección" {
		t.Errorf("los grupos son %q", got)
	}
	if got := NewSession(Tokens{Access: "opaco"}, nil, nil).Grupos(); got != nil {
		t.Errorf("un token que no es JWT no tiene grupos: %q", got)
	}
}

// La organización de los enlaces sale del reclamo `tenant` del access token, leído sin
// verificar: nombra el destino (ADR 0056), no da permiso. Un token que no es JWT no la
// tiene, y sin sesión tampoco.
func TestLaOrganizacionSaleDelAccessToken(t *testing.T) {
	carga := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"u1","tenant":"acme"}`))
	s := NewSession(Tokens{Access: "cabecera." + carga + ".firma"}, nil, nil)
	if got := s.Organizacion(); got != "acme" {
		t.Errorf("la organización es %q, quiero acme", got)
	}
	if got := NewSession(Tokens{Access: "opaco"}, nil, nil).Organizacion(); got != "" {
		t.Errorf("un token que no es JWT no tiene organización: %q", got)
	}
	if got := NewSession(Tokens{}, nil, nil).Organizacion(); got != "" {
		t.Errorf("sin sesión no hay organización: %q", got)
	}
	// Se guarda con la sesión: sin red al arrancar no hay access token y la cola de
	// Notepads necesita saber de qué organización es.
	crudo, _ := serializar(Tokens{Access: "cabecera." + carga + ".firma", Refresh: "r", Subject: "u1"})
	tok, err := recuperar(crudo)
	if err != nil || NewSession(tok, nil, nil).Organizacion() != "acme" {
		t.Errorf("la organización guardada se pierde: %+v, %v", tok, err)
	}
}

// TestUnSecretoSeGuardaSeLeeYSeBorra: la clave recordada de un cuaderno va en el mismo
// llavero que la sesión, con su cuenta, y no puede pisar la de la sesión ni salirse del
// directorio en Android. VISTO EN ROJO dejando pasar la cuenta «sesion».
func TestUnSecretoSeGuardaSeLeeYSeBorra(t *testing.T) {
	keyring.MockInit()
	const cuenta = "notes-5f0c9a1e-7d2b-4c1e-9f3a-2b8e6d4c1a07"
	if d, err := LeerSecreto(cuenta); err != nil || d != nil {
		t.Fatalf("sin nada guardado: %q, %v", d, err)
	}
	clave := []byte{0, 1, 2, 250, 255, '\n'}
	if err := GuardarSecreto(cuenta, clave); err != nil {
		t.Fatal(err)
	}
	if d, err := LeerSecreto(cuenta); err != nil || !bytes.Equal(d, clave) {
		t.Fatalf("leído %v, %v", d, err)
	}
	for range 2 {
		if err := BorrarSecreto(cuenta); err != nil {
			t.Fatal(err)
		}
	}
	if d, _ := LeerSecreto(cuenta); d != nil {
		t.Error("borrado y sigue")
	}
	for _, mala := range []string{"", "sesion", "../sesion", "notes/x", "a b"} {
		if err := GuardarSecreto(mala, clave); err == nil {
			t.Errorf("la cuenta %q se ha aceptado", mala)
		}
	}
}

// TestLaAparienciaDeLaOrganizacionVieneEnElIDToken: el color y la forma por defecto de
// la organización (reclamos `org_color` y `org_forma`) solo viajan en el ID token. Se
// leen al entrar, se guardan con la sesión para pintar bien sin red, y una renovación
// SIN ID token los conserva; una que lo trae sin ellos los borra (la organización ha
// vuelto a los de la casa). Lo que no es un nombre —letras minúsculas— no pasa.
//
// VISTO EN ROJO sin leer los reclamos en `pedir`: la sesión no trae apariencia.
func TestLaAparienciaDeLaOrganizacionVieneEnElIDToken(t *testing.T) {
	idToken := func(carga string) string {
		return "x." + base64.RawURLEncoding.EncodeToString([]byte(carga)) + ".y"
	}
	var respuesta map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			base := "http://" + r.Host
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": base, "authorization_endpoint": base + "/authorize", "token_endpoint": base + "/token",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(respuesta)
	}))
	t.Cleanup(srv.Close)
	o := &OIDC{Issuer: srv.URL}

	respuesta = map[string]any{
		"access_token": "a", "refresh_token": "r2", "expires_in": 300,
		"id_token": idToken(`{"sub":"ada","org_color":"cobalto","org_forma":"carril"}`),
	}
	primeros, err := o.Renew(context.Background(), Tokens{Refresh: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if primeros.OrgColor != "cobalto" || primeros.OrgForma != "carril" || primeros.Subject != "ada" {
		t.Fatalf("del ID token sale %+v; quiero cobalto, carril y ada", primeros)
	}
	// Se guarda y se recupera con la sesión.
	crudo, err := serializar(primeros)
	if err != nil {
		t.Fatal(err)
	}
	if rec, err := recuperar(crudo); err != nil || rec.OrgColor != "cobalto" || rec.OrgForma != "carril" {
		t.Errorf("lo guardado devuelve %+v (%v): la apariencia de la organización se ha perdido", rec, err)
	}
	s := NewSession(primeros, nil, nil)
	if c, f := s.Apariencia(); c != "cobalto" || f != "carril" {
		t.Errorf("la sesión dice %q y %q", c, f)
	}

	// Renovación sin ID token: no se sabe nada nuevo, se conserva.
	respuesta = map[string]any{"access_token": "a", "refresh_token": "r3", "expires_in": 300}
	sinID, err := o.Renew(context.Background(), primeros)
	if err != nil {
		t.Fatal(err)
	}
	if got := conservar(sinID, primeros); got.OrgColor != "cobalto" || got.OrgForma != "carril" || got.Subject != "ada" {
		t.Errorf("sin ID token la renovación deja %+v y tenía que conservar la apariencia", got)
	}
	// Con ID token y sin los reclamos: la organización ha vuelto a los de la casa. Y un
	// valor que no es un nombre no pasa.
	respuesta["id_token"] = idToken(`{"sub":"ada","org_forma":"Carril; path=/"}`)
	conID, err := o.Renew(context.Background(), primeros)
	if err != nil {
		t.Fatal(err)
	}
	if got := conservar(conID, primeros); got.OrgColor != "" || got.OrgForma != "" {
		t.Errorf("con un ID token sin apariencia queda %+v y tenía que quedar la de la casa", got)
	}
}

// NAT-02: la app se identifica ante Sytue ID con un secreto de ESTE dispositivo, que
// vive en el almacén seguro y sobrevive a cerrar sesión. A /authorize va su hash; el
// secreto, solo en el canje. Es lo que deja a Sytue ID no volver a preguntar en este
// dispositivo sin que otra app pueda hacerse pasar por él.
func TestElLoginLlevaElDispositivo(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	keyring.MockInit()
	idp := nuevoIDP(t)
	o := &OIDC{Issuer: idp.URL, Open: idp.navegador(t, nil)}
	if _, err := o.Login(context.Background()); err != nil {
		t.Fatal(err)
	}
	primero := idp.dispositivo
	suma := sha256.Sum256([]byte(idp.secretoVisto))
	if primero == "" || idp.secretoVisto == "" || base64.RawURLEncoding.EncodeToString(suma[:]) != primero {
		t.Fatalf("dispositivo %q y secreto %q: el hash tiene que ser el del secreto", primero, idp.secretoVisto)
	}
	if strings.Contains(primero, idp.secretoVisto) {
		t.Fatal("el secreto ha viajado a /authorize")
	}
	// Cerrar sesión no cambia de dispositivo: Sytue ID no tiene por qué volver a preguntar.
	if err := Save(Tokens{}); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Login(context.Background()); err != nil {
		t.Fatal(err)
	}
	if idp.dispositivo != primero {
		t.Fatalf("tras cerrar sesión el dispositivo ha cambiado: %q → %q", primero, idp.dispositivo)
	}
}
