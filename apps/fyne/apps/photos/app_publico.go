// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/photos/src/pages/p/[token].astro

package photos

import (
	"context"
	"io"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El álbum compartido, vivo: lo que en la web es `/p/{token}`, una página SIN sesión que
// abre gente de fuera. Aquí llega por el enlace —`sytue-photos:/p/{token}` o la dirección
// web, cuando el sistema se la da a la app— y se pide por `/api/v1/public/…`, que es lo
// que sale por el ingress: la web pregunta por `/public/…` desde su BFF, dentro del
// clúster, y una app no tiene BFF.

// TokenDeEnlace saca el token de un enlace a un álbum compartido, o "" si no lo es. Vale
// `sytue-photos:/p/{token}` y la dirección web, con o sin el prefijo `/photos` de la app.
func TokenDeEnlace(u *url.URL) string {
	ruta := strings.TrimPrefix(u.Path, "/photos")
	token, ok := strings.CutPrefix(ruta, "/p/")
	if !ok || token == "" || strings.Contains(token, "/") {
		return ""
	}
	return token
}

// AbrirPublico enseña el álbum compartido de ese token.
func (a *App) AbrirPublico(token string) {
	a.cancelarFotos()
	teca, grande := NuevaFototeca(), NuevaFototeca()
	grande.Variante = VarianteGrande
	if a.cli != nil {
		cli := a.cli
		pedirVariante := func(ctx context.Context, id, variante string) ([]byte, error) {
			r, err := cli.PublicVariant(ctx, token, id, variante)
			if err != nil {
				return nil, err
			}
			defer func() { _ = r.Close() }()
			return io.ReadAll(r)
		}
		teca.Pedir, grande.Pedir = pedirVariante, pedirVariante
	}
	a.pub = estadoPublico{token: token, teca: teca, grande: grande}
	a.Ir(RutaPublica)
}

// pedirPublico trae el álbum. Un 404 es un enlace que no existe o que ya se cerró, y la
// web contesta con su página «no encontrada»: aquí también, si hay quien la enseñe.
func (a *App) pedirPublico() {
	a.cerrarPublica()
	a.pub.estadoCarga = estadoCarga{cargando: true}
	a.pintar()
	token := a.pub.token
	pedir(a, func(x context.Context, c *api.Photos) (api.PublicAlbum, error) { return c.PublicAlbum(x, token) },
		func(al api.PublicAlbum, err error) {
			a.pub.cargando, a.pub.fallo = false, err
			if api.EsNoEncontrado(err) && a.NoEncontrado != nil && a.NoEncontrado() {
				return
			}
			if err == nil {
				a.pub.album = al
			}
		})
}

func (a *App) pantallaPublica() *fyne.Container {
	p := a.pub
	v := a.vistaDe(p.estadoCarga, p.album.Items == nil, len(p.album.Items) == 0)
	v.Teca, v.TecaGrande = p.teca, p.grande
	// Sin sesión no hay a quién pedirle que entre: el enlace es la credencial.
	if v.Estado == SinSesion {
		v.Estado = Error
	}
	return PantallaPublica(v, p.album.Title, p.album.Count, p.album.Items, a.verPublica)
}

// verPublica abre una foto del álbum sola, como el navegador al seguir el enlace de la
// tesela. Es una capa sobre la ventana entera, igual que el visor.
func (a *App) verPublica(f api.Photo) {
	if a.pub.grande != nil && (a.pub.abierta == nil || a.pub.abierta.ID != f.ID) {
		a.pub.grande.Cancelar()
	}
	a.pub.abierta, a.pub.real = &f, false
	if a.pub.capa == nil {
		a.pub.capa = nuevaCapaVisor(a.cerrarPublica)
		if a.w != nil {
			c := a.w.Canvas()
			c.Overlays().Add(a.pub.capa)
			a.pub.capa.Resize(c.Size())
			c.Unfocus()
		}
	}
	a.pintarPublica()
}

func (a *App) pintarPublica() {
	p := a.pub
	if p.capa == nil || p.abierta == nil {
		return
	}
	p.grande.Olvidar()
	v := a.v
	v.Teca, v.TecaGrande = p.teca, p.grande
	zoom := func() {
		a.pub.real = !a.pub.real
		a.pintarPublica()
	}
	if a.ruta != RutaCompartido {
		p.capa.poner(VisorPublico(v, *p.abierta, p.real, zoom))
		return
	}
	// La de un álbum compartido conmigo lleva la cabecera de la web: anterior,
	// siguiente, cerrar y, si puedo editarlo, «Quitar del álbum».
	anterior, siguiente := a.vecinaCompartida(-1), a.vecinaCompartida(1)
	var quitar func(api.Photo)
	if a.cmp.editable {
		quitar = a.quitarCompartida
	}
	p.capa.poner(VisorCompartido(v, *p.abierta, p.real, zoom, anterior, siguiente, a.cerrarPublica, quitar))
}

// vecinaCompartida abre la foto de al lado (-1, 1) del álbum compartido, o nil si no hay.
func (a *App) vecinaCompartida(paso int) func() {
	if a.pub.abierta == nil {
		return nil
	}
	fotos := a.pub.album.Items
	for i, f := range fotos {
		if f.ID != a.pub.abierta.ID {
			continue
		}
		if j := i + paso; j >= 0 && j < len(fotos) {
			return func() { a.verPublica(fotos[j]) }
		}
		return nil
	}
	return nil
}

func (a *App) cerrarPublica() {
	if a.pub.grande != nil {
		a.pub.grande.Cancelar()
	}
	c := a.pub.capa
	if c == nil {
		return
	}
	a.pub.capa, a.pub.abierta = nil, nil
	if a.w != nil {
		a.w.Canvas().Overlays().Remove(c)
	}
}
