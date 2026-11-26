// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
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
)

// proveedorFalso es un servidor OAuth de mentira, como el de Google o Microsoft: la
// página de permiso redirige con un código al redirect_uri que se le dio, y el de tokens
// solo lo canjea con ese mismo redirect_uri y el verificador de PKCE de su reto.
type proveedorFalso struct {
	srv *httptest.Server
	mu  sync.Mutex
	// por código: el reto y el redirect_uri con que se dio.
	codigos map[string][2]string
	canjes  int
}

func nuevoProveedor(t *testing.T) *proveedorFalso {
	p := &proveedorFalso{codigos: map[string][2]string{}}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/authorize":
			q := r.URL.Query()
			if q.Get("code_challenge_method") != "S256" || q.Get("response_type") != "code" || q.Get("client_id") != "cli" {
				http.Error(w, "petición mala", http.StatusBadRequest)
				return
			}
			codigo := "c-" + q.Get("state")[:8]
			p.mu.Lock()
			p.codigos[codigo] = [2]string{q.Get("code_challenge"), q.Get("redirect_uri")}
			p.mu.Unlock()
			destino, _ := url.Parse(q.Get("redirect_uri"))
			v := url.Values{"code": {codigo}, "state": {q.Get("state")}}
			destino.RawQuery = v.Encode()
			http.Redirect(w, r, destino.String(), http.StatusFound)
		case "/token":
			_ = r.ParseForm()
			p.mu.Lock()
			dado, ok := p.codigos[r.Form.Get("code")]
			delete(p.codigos, r.Form.Get("code")) // un código vale una vez
			p.canjes++
			p.mu.Unlock()
			h := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if !ok || base64.RawURLEncoding.EncodeToString(h[:]) != dado[0] || r.Form.Get("redirect_uri") != dado[1] || r.Form.Get("grant_type") != "authorization_code" {
				t.Errorf("canje malo: %v (dado %v)", r.Form, dado)
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "a", "refresh_token": "r-" + r.Form.Get("code"), "expires_in": 3600})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *proveedorFalso) oauth(v *Vuelta) OAuth {
	return OAuth{Autorizar: p.srv.URL + "/authorize", Token: p.srv.URL + "/token", Cliente: "cli", Alcances: []string{"Files.ReadWrite"}, Vuelta: v}
}

// navegadorMovil hace de navegador del teléfono: abre la página de permiso, NO sigue la
// redirección (es a un esquema que el navegador no sabe abrir) y le da al sistema el
// enlace de vuelta, que este entrega a la app (aquí, a enlace).
func navegadorMovil(t *testing.T, p *proveedorFalso, enlace func(*url.URL)) func(string) error {
	return func(destino string) error {
		c := p.srv.Client()
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		resp, err := c.Get(destino)
		if err != nil {
			return err
		}
		resp.Body.Close()
		vuelta, err := url.Parse(resp.Header.Get("Location"))
		if err != nil || resp.StatusCode != http.StatusFound {
			t.Errorf("sin redirección: %d %v", resp.StatusCode, err)
			return nil
		}
		go enlace(vuelta)
		return nil
	}
}

// En el móvil, la vuelta es por el esquema de la app: el redirect_uri es
// `sytueoffice-docs:/oauth`, el enlace llega como llegaría por AlAbrirEnlace y se
// canjea con PKCE.
func TestVueltaPorElEsquemaDeLaApp(t *testing.T) {
	p := nuevoProveedor(t)
	v := NuevaVuelta("sytueoffice-docs")
	if v.Redireccion != "sytueoffice-docs:/oauth" {
		t.Fatalf("redirect_uri %q", v.Redireccion)
	}
	recibido := make(chan bool, 1)
	navegador := navegadorMovil(t, p, func(u *url.URL) {
		if u.String()[:len("sytueoffice-docs:/oauth?")] != "sytueoffice-docs:/oauth?" {
			t.Errorf("vuelta a %s", u)
		}
		recibido <- v.Recibir(u)
	})
	got, err := p.oauth(v).Conectar(t.Context(), p.srv.Client(), navegador)
	if err != nil || got.Refresco == "" || !<-recibido {
		t.Fatalf("%+v, %v", got, err)
	}
	if v.pendientes() != 0 {
		t.Errorf("quedan %d altas esperando", v.pendientes())
	}
}

// La de la app (UsarVuelta) vale para todas las altas, también las de agenda, correo y
// mensajería, que construyen su OAuth sin vuelta.
func TestUsarVueltaValeParaTodas(t *testing.T) {
	p := nuevoProveedor(t)
	v := NuevaVuelta("sytueoffice-calendar")
	UsarVuelta(v)
	t.Cleanup(func() { UsarVuelta(nil) })
	if VueltaDeLaApp() != v {
		t.Fatal("UsarVuelta")
	}
	o := p.oauth(nil)
	if _, err := o.Conectar(t.Context(), p.srv.Client(), navegadorMovil(t, p, func(u *url.URL) { v.Recibir(u) })); err != nil {
		t.Fatal(err)
	}
}

// Un enlace de vuelta con otro state (una web que intenta colar su código) se descarta
// y el alta sigue esperando la buena; la buena, repetida, ya no vale; y uno que no es de
// vuelta (o de otro esquema) se deja para la app.
func TestVueltaSoloLaDelAltaYUnaVez(t *testing.T) {
	p := nuevoProveedor(t)
	v := NuevaVuelta("sytueoffice-mail")
	var buena *url.URL
	navegador := navegadorMovil(t, p, func(u *url.URL) {
		colada, _ := url.Parse("sytueoffice-mail:/oauth?code=robado&state=otro")
		if !v.Recibir(colada) {
			t.Error("una vuelta con otro state no es de la app: no se le debe pasar")
		}
		buena = u
		v.Recibir(u)
	})
	if _, err := p.oauth(v).Conectar(t.Context(), p.srv.Client(), navegador); err != nil {
		t.Fatal(err)
	}
	// La misma vuelta otra vez: se descarta (y no llega a canjearse).
	canjes := p.canjes
	if !v.Recibir(buena) || p.canjes != canjes {
		t.Error("la segunda vuelta con el mismo state no se ha descartado")
	}
	for _, s := range []string{"sytueoffice-mail:/redactar?para=a@b.c", "mailto:a@b.c", "sytueoffice-docs:/oauth?code=x&state=y", "sytueoffice-mail:/abrir?clase=hilo&id=1"} {
		u, _ := url.Parse(s)
		if v.Recibir(u) {
			t.Errorf("%s no es una vuelta de OAuth de Mail", s)
		}
	}
	// `sytueoffice-mail://oauth?…` también lo es.
	u, _ := url.Parse("sytueoffice-mail://oauth?code=x&state=y")
	if !v.Recibir(u) {
		t.Error("la forma con // también es una vuelta")
	}
}

// Sin vuelta a tiempo, el alta caduca; cancelada, también para; y el «no» del proveedor
// se dice.
func TestVueltaCaducaSeCancelaYDiceQueNo(t *testing.T) {
	p := nuevoProveedor(t)
	v := NuevaVuelta("sytueoffice-drive")
	v.Caducidad = 50 * time.Millisecond
	tarde := make(chan *url.URL, 1)
	_, err := p.oauth(v).Conectar(t.Context(), p.srv.Client(), navegadorMovil(t, p, func(u *url.URL) { tarde <- u }))
	if !errors.Is(err, ErrVueltaCaducada) {
		t.Fatalf("caducada: %v", err)
	}
	// Lo que llegue después se descarta.
	if !v.Recibir(<-tarde) || v.pendientes() != 0 {
		t.Error("la vuelta tardía no se descarta")
	}

	ctx, cancelar := context.WithCancel(t.Context())
	v.Caducidad = time.Minute
	_, err = p.oauth(v).Conectar(ctx, p.srv.Client(), func(string) error { cancelar(); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("cancelada: %v", err)
	}

	no := func(destino string) error {
		d, _ := url.Parse(destino)
		u, _ := url.Parse("sytueoffice-drive:/oauth?error=access_denied&state=" + url.QueryEscape(d.Query().Get("state")))
		go v.Recibir(u)
		return nil
	}
	_, err = p.oauth(v).Conectar(t.Context(), p.srv.Client(), no)
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Errorf("el no del proveedor: %v", err)
	}
	if p.canjes != 0 {
		t.Errorf("se ha canjeado %d veces", p.canjes)
	}
}

// En el escritorio, sin vuelta puesta, sigue por 127.0.0.1.
func TestSinVueltaSigueElLoopback(t *testing.T) {
	UsarVuelta(nil)
	p := nuevoProveedor(t)
	navegador := func(destino string) error {
		d, _ := url.Parse(destino)
		if r := d.Query().Get("redirect_uri"); !strings.HasPrefix(r, "http://127.0.0.1:") {
			t.Errorf("redirect_uri %q", r)
		}
		go func() {
			resp, err := p.srv.Client().Get(destino)
			if err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	}
	if got, err := p.oauth(nil).Conectar(t.Context(), p.srv.Client(), navegador); err != nil || got.Refresco == "" {
		t.Fatal(got, err)
	}
}
