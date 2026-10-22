// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"net/url"
	"slices"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// conEnlaces es el doble del servicio con las vistas previas.
type conEnlaces struct {
	*servicioFalso
	vistas  map[string]api.RoomsUnfurl
	pedidas []string
}

func (s *conEnlaces) Unfurl(_ context.Context, e string) (api.RoomsUnfurl, error) {
	s.pedidas = append(s.pedidas, e)
	v, ok := s.vistas[e]
	if !ok {
		return api.RoomsUnfurl{}, errors.New("sin vista")
	}
	return v, nil
}

func (s *conEnlaces) UnfurlImage(_ context.Context, ruta string) (io.ReadCloser, error) {
	s.pedidas = append(s.pedidas, "imagen "+ruta)
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 8, 8)))
	return io.NopCloser(&b), nil
}

// `extractLinks` y `proveedorDeReunion`: dos enlaces distintos como mucho, y reunión
// solo con https, el host y la forma de ruta de una.
func TestLosEnlacesDeUnMensaje(t *testing.T) {
	got := EnlacesDe("mira https://a.eu y https://a.eu, https://b.eu y https://c.eu", 2)
	if !slices.Equal(got, []string{"https://a.eu", "https://b.eu"}) {
		t.Errorf("EnlacesDe = %v", got)
	}
	for e, quiero := range map[string]string{
		"https://meet.google.com/abc-defg-hij":        "meet",
		"https://meet.google.com/landing":             "",
		"http://meet.google.com/abc-defg-hij":         "",
		"https://acme.zoom.us/j/123":                  "zoom",
		"https://teams.microsoft.com/l/meetup-join/x": "teams",
		"https://acme.webex.com/meet/ana":             "webex",
		"https://zoom.us.evil.eu/j/1":                 "",
	} {
		if p := ProveedorDeReunion(e); p != quiero {
			t.Errorf("ProveedorDeReunion(%q) = %q; quiero %q", e, p, quiero)
		}
	}
}

// Bajo el mensaje, la reunión sale en el acto; la vista previa se pide UNA vez y, al
// llegar, el historial la pinta; lo que no tiene título ni imagen no deja tarjeta.
// Pulsarla abre el enlace.
//
// VISTO EN ROJO (2026-09-24, en el frontal, saboteado a mano): sin apuntar el enlace
// como pedido en pedirVista, la vista vacía se pide en cada repintado del historial.
func TestLasTarjetasDeLosEnlaces(t *testing.T) {
	s := servicioConDos()
	s.mensajes["c1"][0].Body = "https://meet.google.com/abc-defg-hij y https://sytue.eu/blog y https://vacio.eu"
	// La vacía va en un mensaje suyo: de cada uno solo se miran los dos primeros enlaces.
	s.mensajes["c1"][3].Body = "https://vacio.eu"
	s.mensajes["c1"] = append(s.mensajes["c1"][:1], s.mensajes["c1"][3])
	sa, relojes := salaDePrueba(t, s)
	en := &conEnlaces{servicioFalso: s, vistas: map[string]api.RoomsUnfurl{
		"https://sytue.eu/blog": {URL: "https://sytue.eu/blog", Title: "Blog", Description: "Novedades", ImageURL: "/api/v1/unfurl/imagen/ab"},
		"https://vacio.eu":      {URL: "https://vacio.eu"},
	}}
	sa.S = en
	var abierto string
	sa.AbrirEnlace = func(u *url.URL) error { abierto = u.String(); return nil }
	sa.Arrancar(context.Background())
	h := sa.P.Canal.Historial
	test.WidgetRenderer(filaDe(t, h, "m1"))
	if r := relojes[0]; r != nil {
		r()
	}
	h.Refresh()
	h.Refresh()
	f := filaDe(t, h, "m1")
	test.WidgetRenderer(f)
	if len(f.enlaces) != 2 {
		t.Fatalf("la fila lleva %d tarjetas; quiero la reunión y el blog", len(f.enlaces))
	}
	reunion, blog := f.enlaces[0].(*TarjetaEnlace), f.enlaces[1].(*TarjetaEnlace)
	if reunion.Proveedor != "meet" || blog.Vista.Title != "Blog" || blog.Imagen == nil {
		t.Errorf("tarjetas: %+v / %+v", reunion, blog)
	}
	// La vacía cuenta igual: no deja tarjeta ni se apunta en la caché, y sin la marca de
	// «pedida» se volvería a pedir en cada repintado. La del blog llega y se queda.
	veces := map[string]int{}
	for _, p := range en.pedidas {
		veces[p]++
	}
	if veces["https://sytue.eu/blog"] != 1 || veces["https://vacio.eu"] != 1 || veces["https://meet.google.com/abc-defg-hij"] != 0 {
		t.Errorf("se piden %v", en.pedidas)
	}
	blog.AlPulsar()
	if abierto != "https://sytue.eu/blog" {
		t.Errorf("pulsar abre %q", abierto)
	}
	if m := MedidaTarjetaEnlace(blog, 2000); m.Width != AnchoTarjetaEnlace || m.Height < LadoImagenEnlace {
		t.Errorf("la tarjeta mide %v", m)
	}
}
