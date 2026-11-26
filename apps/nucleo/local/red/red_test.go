// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package red

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
)

// Recién instalada no hay ningún destino: ni siquiera el propio equipo (R-RED).
func TestSinConfigurarNadaNoSaleNada(t *testing.T) {
	p := Abrir("")
	if ds := p.Destinos(); len(ds) != 0 {
		t.Fatalf("destinos sin configurar: %v", ds)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("la petición llegó al servidor")
	}))
	defer srv.Close()
	_, err := p.Cliente(IA).Get(srv.URL)
	if !errors.Is(err, ErrDenegado) {
		t.Fatalf("sin destinos: %v; quiero ErrDenegado", err)
	}
	if _, err := p.Marcar(context.Background(), Correo, "tcp", "imap.ejemplo.eu:993"); !errors.Is(err, ErrDenegado) {
		t.Fatalf("marcar sin destinos: %v", err)
	}
}

// Una fuente abre su destino, solo para su propósito; y lo que deja de configurarse se
// cierra en el acto, sin reiniciar.
func TestLaListaSaleDeLaConfiguracion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	puerto, _ := strconv.Atoi(u.Port())
	hay := true
	p := Abrir("")
	p.Fuente(func() []Destino {
		if !hay {
			return nil
		}
		return []Destino{{Proposito: IA, Host: "127.0.0.1", Puerto: puerto, Motivo: "Modelo en este equipo"}}
	})
	res, err := p.Cliente(IA).Get(srv.URL)
	if err != nil {
		t.Fatalf("destino configurado: %v", err)
	}
	_ = res.Body.Close()
	if _, err := p.Cliente(Nube).Get(srv.URL); !errors.Is(err, ErrDenegado) {
		t.Fatalf("otro propósito: %v; quiero ErrDenegado", err)
	}
	hay = false
	if _, err := p.Cliente(IA).Get(srv.URL); !errors.Is(err, ErrDenegado) {
		t.Fatalf("quitada la configuración: %v; quiero ErrDenegado", err)
	}
}

// Fuera de este equipo, solo TLS; salvo lo que la persona marcó a mano.
func TestSoloTLSSalvoLocalOMarcado(t *testing.T) {
	p := Abrir("")
	p.Fuente(func() []Destino {
		return []Destino{
			{Proposito: Chat, Host: "matrix.ejemplo.eu"},
			{Proposito: Correo, Host: "casa.lan", Puerto: 143, Claro: true},
		}
	})
	if err := p.Permitido(Chat, "matrix.ejemplo.eu", 80, false); !errors.Is(err, ErrDenegado) {
		t.Errorf("http fuera del equipo: %v", err)
	}
	if err := p.Permitido(Chat, "matrix.ejemplo.eu", 443, true); err != nil {
		t.Errorf("https configurado: %v", err)
	}
	if err := p.Permitido(Correo, "casa.lan", 143, false); err != nil {
		t.Errorf("marcado en claro: %v", err)
	}
	if err := p.Permitido(Correo, "casa.lan", 25, false); !errors.Is(err, ErrDenegado) {
		t.Errorf("otro puerto: %v", err)
	}
	if _, err := DestinoDeURL(IA, "http://ia.ejemplo.eu/v1", ""); err == nil {
		t.Error("una dirección http fuera del equipo dio destino")
	}
	if d, err := DestinoDeURL(IA, "http://localhost:11434/v1", ""); err != nil || !d.Claro || d.Puerto != 11434 {
		t.Errorf("localhost: %+v, %v", d, err)
	}
}

// «Sin conexión» lo corta todo menos el propio equipo, y se recuerda.
func TestSinConexion(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "red.json")
	p := Abrir(ruta)
	p.Fuente(func() []Destino {
		return []Destino{{Proposito: Nube, Host: "dav.ejemplo.eu"}, {Proposito: IA, Host: "localhost"}}
	})
	if err := p.PonerSinConexion(true); err != nil {
		t.Fatal(err)
	}
	if err := p.Permitido(Nube, "dav.ejemplo.eu", 443, true); !errors.Is(err, ErrDenegado) {
		t.Errorf("sin conexión, nube: %v", err)
	}
	if err := p.Permitido(IA, "localhost", 11434, false); err != nil {
		t.Errorf("sin conexión, el propio equipo: %v", err)
	}
	if !Abrir(ruta).SinConexion() {
		t.Error("«Sin conexión» no se recordó")
	}
}

// Lo que abre un alta se cierra al acabar.
func TestPermisoTemporal(t *testing.T) {
	p := Abrir("")
	cerrar := p.PermitirURL(OAuth, "https://login.ejemplo.eu/token", "Alta")
	if err := p.Permitido(OAuth, "login.ejemplo.eu", 443, true); err != nil {
		t.Fatalf("durante el alta: %v", err)
	}
	if ds := p.Destinos(); len(ds) != 1 || !ds[0].Temporal {
		t.Fatalf("destinos durante el alta: %+v", ds)
	}
	cerrar()
	cerrar()
	if err := p.Permitido(OAuth, "login.ejemplo.eu", 443, true); !errors.Is(err, ErrDenegado) {
		t.Fatalf("después del alta: %v", err)
	}
}

// Una redirección a otro host vuelve a pasar por el portero.
func TestLasRedireccionesPasanPorElPortero(t *testing.T) {
	otro := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("la redirección llegó a un host no configurado")
	}))
	defer otro.Close()
	ou, _ := url.Parse(otro.URL)
	// Mismo equipo, otro nombre: «localhost» en vez de 127.0.0.1.
	destinoOtro := "http://localhost:" + ou.Port()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destinoOtro, http.StatusFound)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	p := Abrir("")
	p.Fuente(func() []Destino { return []Destino{{Proposito: IA, Host: u.Hostname()}} })
	if _, err := p.Cliente(IA).Get(srv.URL); !errors.Is(err, ErrDenegado) {
		t.Fatalf("redirección: %v; quiero ErrDenegado", err)
	}
}

// Marcar abre la conexión cuando el destino está en la lista.
func TestMarcar(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, err := l.Accept()
		if err == nil {
			_ = c.Close()
		}
	}()
	_, puertoTxt, _ := net.SplitHostPort(l.Addr().String())
	puerto, _ := strconv.Atoi(puertoTxt)
	p := Abrir("")
	p.Fuente(func() []Destino { return []Destino{{Proposito: Correo, Host: "127.0.0.1", Puerto: puerto}} })
	c, err := p.Marcador(Correo)(context.Background(), "tcp", l.Addr().String())
	if err != nil {
		t.Fatalf("marcar: %v", err)
	}
	_ = c.Close()
}

// Denegar es lo que queda debajo de todo cliente heredado.
func TestDenegar(t *testing.T) {
	_, err := (&http.Client{Transport: Denegar{}}).Get("https://ejemplo.eu")
	if !errors.Is(err, ErrDenegado) {
		t.Fatalf("Denegar: %v", err)
	}
}
