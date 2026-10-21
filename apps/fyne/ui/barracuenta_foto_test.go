// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"github.com/zalando/go-keyring"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// El avatar de la barra es la foto elegida en Sytue ID, en las trece apps: la barra es
// común. Hasta el 2026-09-24 solo se le ponía el NOMBRE, así que salían las iniciales
// aunque la persona tuviera foto —en la web, un icono de imagen rota—.

// idDeMentira es Sytue ID: `GET /api/v1/me` con la versión de la foto y
// `GET /api/v1/me/avatar` con un PNG liso del color que toque.
type idDeMentira struct {
	mu      sync.Mutex
	version string // "" = sin foto
	color   color.NRGBA
	falla   bool // la foto contesta 500
	bajadas int
}

func (m *idDeMentira) cuantas() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bajadas
}

func (m *idDeMentira) poner(version string, c color.NRGBA) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.version, m.color, m.falla = version, c, false
}

func (m *idDeMentira) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer tok" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case "/api/v1/me":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "Ada Lovelace", "email": "ada@sytue.eu",
			"has_avatar": m.version != "", "avatar_version": m.version,
		})
	case "/api/v1/me/avatar":
		switch {
		case m.version == "":
			w.WriteHeader(http.StatusNotFound)
		case m.falla:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			m.bajadas++
			liso := image.NewNRGBA(image.Rect(0, 0, 32, 32))
			for i := 0; i < len(liso.Pix); i += 4 {
				liso.Pix[i], liso.Pix[i+1], liso.Pix[i+2], liso.Pix[i+3] = m.color.R, m.color.G, m.color.B, 255
			}
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("ETag", `"`+m.version+`"`)
			_ = png.Encode(w, liso)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

type sesionDePrueba struct{}

func (sesionDePrueba) AccessToken(context.Context) (string, error) { return "tok", nil }
func (sesionDePrueba) Invalidate(string)                           {}

// suiteConSesion es una `sesion.Suite` de verdad con alguien dentro, aislada del
// llavero y de los ajustes de quien ejecuta las pruebas.
func suiteConSesion(t *testing.T) *sesion.Suite {
	t.Helper()
	casa := t.TempDir()
	t.Setenv("HOME", casa)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(casa, ".config"))
	keyring.MockInit()
	s := sesion.Abrir(fyne.CurrentApp(), "sytue.eu")
	s.Sesion.Adoptar(auth.Tokens{Access: "tok", Refresh: "r", Subject: "ada", Expiry: time.Now().Add(time.Hour)})
	return s
}

// esperarFoto deja pasar lo encolado hasta que la foto del botón cumpla `listo`.
func esperarFoto(t *testing.T, b *BarraSuite, que string, listo func(image.Image) bool) {
	t.Helper()
	limite := time.Now().Add(5 * time.Second)
	for {
		var foto image.Image
		fyne.DoAndWait(func() { foto = b.cuenta.boton.avatar.Foto })
		if listo(foto) {
			return
		}
		if time.Now().After(limite) {
			t.Fatalf("%s: la foto del botón no llegó", que)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// enLaBarra cuenta los píxeles de un color en la barra ENTERA pintada: la foto de
// prueba es de un color liso que la barra no usa en ningún otro sitio, así que si está,
// es el avatar.
func enLaBarra(t *testing.T, b *BarraSuite, c color.NRGBA) int {
	t.Helper()
	tam := fyne.NewSize(anchoPrueba, AltoBarraSuite())
	b.Resize(tam)
	test.WidgetRenderer(b).Layout(tam)
	b.Refresh()
	return cuantos(pinta(t, tam.Width, tam.Height, b), c)
}

// TestLaBarraPintaLaFotoDeSytueID es el guardián del encargo, de principio a fin con la
// barra de verdad, una `sesion.Suite` y un Sytue ID de mentira: sin foto, las iniciales;
// con foto, la foto (en el botón y en la cabecera del menú); con la misma versión no se
// vuelve a bajar; al volver a la app con otra versión, la nueva; y si no se puede
// bajar, otra vez las iniciales.
//
// VISTO EN ROJO sin `c.boton.avatar.Foto = foto` en `pintar`: con foto, el botón sigue
// en las iniciales.
func TestLaBarraPintaLaFotoDeSytueID(t *testing.T) {
	id := &idDeMentira{}
	srv := httptest.NewServer(id)
	defer srv.Close()

	s := suiteConSesion(t)
	b := NuevaBarraSuite(traductorPrueba(t), s, iconos.Mail)
	t.Cleanup(b.Parar)
	t.Cleanup(func() { fyne.CurrentApp().Lifecycle().SetOnEnteredForeground(nil) })
	b.cuenta.cuenta = api.NewAccount(srv.URL, sesionDePrueba{}, nil)
	av := b.cuenta.boton.avatar
	iniciales := ColorAvatar("Ada Lovelace", tema.Claro)

	// Sin foto: entra, pide la ficha y se queda en las iniciales.
	b.cuenta.arrancar()
	limite := time.Now().Add(5 * time.Second)
	for av.Nombre != "Ada Lovelace" {
		if time.Now().After(limite) {
			t.Fatal("la ficha no llegó")
		}
		fyne.DoAndWait(func() {})
		time.Sleep(10 * time.Millisecond)
	}
	if av.Foto != nil {
		t.Fatal("sin foto en Sytue ID, el botón tiene una")
	}
	if enLaBarra(t, b, iniciales) == 0 {
		t.Errorf("sin foto la barra no pinta el círculo de las iniciales (%v)", iniciales)
	}

	// Con foto: se pinta la foto, y también en la cabecera del menú.
	rojo := color.NRGBA{R: 200, G: 30, B: 30, A: 255}
	id.poner("v1", rojo)
	b.cuenta.alVolver()
	esperarFoto(t, b, "con foto", func(f image.Image) bool { return f != nil })
	if enLaBarra(t, b, rojo) == 0 || enLaBarra(t, b, iniciales) != 0 {
		t.Errorf("con foto la barra pinta %d px de la foto y %d de las iniciales",
			enLaBarra(t, b, rojo), enLaBarra(t, b, iniciales))
	}
	if a := avatarDe(b.cuenta.contenido()[0]); a == nil || a.Foto == nil {
		t.Error("la cabecera del menú de cuenta no lleva la foto")
	}

	// La misma versión no se vuelve a bajar.
	b.cuenta.pedirFicha()
	fyne.DoAndWait(func() {})
	if n := id.cuantas(); n != 1 {
		t.Errorf("con la misma versión se ha bajado %d veces", n)
	}

	// Otra versión (otro ETag) al volver a la ventana: el aviso de Fyne, como lo da el
	// sistema, y la foto nueva.
	azul := color.NRGBA{R: 30, G: 60, B: 200, A: 255}
	id.poner("v2", azul)
	volver := fyne.CurrentApp().Lifecycle().(interface{ OnEnteredForeground() func() }).OnEnteredForeground()
	if volver == nil {
		t.Fatal("la barra no se apunta a la vuelta a la app")
	}
	volver()
	esperarFoto(t, b, "versión nueva", func(f image.Image) bool {
		return f != nil && cerca(px(f, 16, 16), azul)
	})
	if enLaBarra(t, b, azul) == 0 || enLaBarra(t, b, rojo) != 0 {
		t.Errorf("tras cambiar la foto la barra pinta %d px de la nueva y %d de la vieja",
			enLaBarra(t, b, azul), enLaBarra(t, b, rojo))
	}

	// Si la foto no se puede bajar, las iniciales, nunca un hueco.
	id.poner("v3", rojo)
	id.mu.Lock()
	id.falla = true
	id.mu.Unlock()
	b.cuenta.alVolver()
	esperarFoto(t, b, "foto que falla", func(f image.Image) bool { return f == nil })
	if enLaBarra(t, b, iniciales) == 0 || enLaBarra(t, b, azul) != 0 {
		t.Error("con la foto fallando la barra no vuelve a las iniciales")
	}
}

// TestLaFotoCambiaAlLlegarElEventoDelHub: cambiar la foto en Sytue ID —en la web o en
// otra app— se ve AL MOMENTO en la barra abierta, sin volver a la ventana: el evento
// `avatar` llega por el flujo de la campana y la cuenta vuelve a pedir la ficha. Y no
// entra en la campana, que no es un aviso.
//
// VISTO EN ROJO sin `b.avisos.alFoto` en NuevaBarraSuite: la foto se queda en la vieja.
func TestLaFotoCambiaAlLlegarElEventoDelHub(t *testing.T) {
	id := &idDeMentira{}
	ids := httptest.NewServer(id)
	defer ids.Close()
	hub := &hubDeMentira{enviar: make(chan string, 4), conexion: make(chan struct{}, 4)}
	hs := httptest.NewServer(hub)
	t.Cleanup(hs.Close)

	s := suiteConSesion(t)
	s.Hub = api.NewHub(hs.URL, sesionDePrueba{}, nil)
	b := NuevaBarraSuite(traductorPrueba(t), s, iconos.Mail)
	t.Cleanup(b.Parar)
	t.Cleanup(func() { fyne.CurrentApp().Lifecycle().SetOnEnteredForeground(nil) })
	b.cuenta.cuenta = api.NewAccount(ids.URL, sesionDePrueba{}, nil)

	rojo := color.NRGBA{R: 200, G: 30, B: 30, A: 255}
	id.poner("v1", rojo)
	b.Arrancar()
	esperarConexion(t, hub)
	esperarFoto(t, b, "al entrar", func(f image.Image) bool { return f != nil && cerca(px(f, 16, 16), rojo) })

	azul := color.NRGBA{R: 30, G: 60, B: 200, A: 255}
	id.poner("v2", azul)
	hub.enviar <- "event: avatar\ndata: {\"version\":\"v2\"}"
	esperarFoto(t, b, "tras el evento", func(f image.Image) bool { return f != nil && cerca(px(f, 16, 16), azul) })

	// La quitó: las iniciales, por el mismo camino.
	id.poner("", azul)
	hub.enviar <- "event: avatar\ndata: {\"version\":\"\"}"
	esperarFoto(t, b, "al quitarla", func(f image.Image) bool { return f == nil })
	if n := sinLeerDe(b.avisos); n != 0 {
		t.Fatalf("el evento de la foto cuenta como aviso: %d sin leer", n)
	}
}

// avatarDe busca el avatar dentro de una pieza del menú.
func avatarDe(o fyne.CanvasObject) *Avatar {
	switch v := o.(type) {
	case *Avatar:
		return v
	case *fyne.Container:
		for _, h := range v.Objects {
			if a := avatarDe(h); a != nil {
				return a
			}
		}
	}
	return nil
}

// TestRetratoDeLaBarraConFoto guarda la barra con la foto y el menú abierto a 1280 y a
// 360, en claro y en oscuro, para MIRARLOS. No afirma nada. Con
// `SYTUE_CAPTURA_AVATAR=/directorio go test -run RetratoDeLaBarraConFoto ./ui`.
func TestRetratoDeLaBarraConFoto(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURA_AVATAR")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURA_AVATAR")
	}
	t.Cleanup(func() { tema.Elegir("") })
	// Una «foto» con algo de forma: mitad de arriba ámbar y de abajo verde azulado.
	foto := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	for y := range 96 {
		for x := range 96 {
			c := color.NRGBA{R: 225, G: 150, B: 40, A: 255}
			if y >= 48 {
				c = color.NRGBA{R: 30, G: 130, B: 140, A: 255}
			}
			foto.SetNRGBA(x, y, c)
		}
	}
	for _, modo := range []string{"claro", "oscuro"} {
		tema.Elegir(modo)
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		for _, ancho := range []float32{1280, 360} {
			b := barraPruebaA(t, ancho)
			b.cuenta.ficha = api.Profile{Name: "Ada Lovelace", Email: "ada@sytue.eu", ActiveOrg: "corporate"}
			b.cuenta.estado = sesion.Estado{Sujeto: "ada", Presencia: api.EnLinea}
			b.cuenta.foto = foto
			// Por `pintar`, que es el camino de verdad: poner los campos a mano dejaba
			// el botón en «?» porque nadie lo repintaba.
			b.cuenta.pintar()
			fyne.DoAndWait(func() {})

			m := NuevoMenu(b.cuenta.contenido()...)
			tm := MedidaMenu(m, fyne.NewSize(400, 600))
			m.Resize(tm)
			test.WidgetRenderer(m).Layout(tm)
			m.Move(fyne.NewPos(ancho-tm.Width-8, AltoBarraSuite()+4))

			alto := AltoBarraSuite() + tm.Height + 12
			fondo := fcanvas.NewRectangle(tema.Color("bg", v))
			fondo.Resize(fyne.NewSize(ancho, alto))
			img := pintaA(t, 2, ancho, alto, fondo, b, m)
			ruta := filepath.Join(dir, fmt.Sprintf("barra-avatar-%s-%d.png", modo, int(ancho)))
			f, err := os.Create(ruta)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, img); err != nil {
				t.Fatal(err)
			}
			_ = f.Close()
		}
	}
}
