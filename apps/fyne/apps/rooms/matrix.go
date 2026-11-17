// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

// Cifrado adds Rooms peers and calls to the shared native verification panel.
type Cifrado struct {
	*cifrado.Panel
	API *api.Rooms
}

func NuevoCifrado(sala *Sala, service *api.Rooms, base mx.Config, abrir func(string) error) *Cifrado {
	c := &Cifrado{API: service}
	var bootstrap func(context.Context) (mx.Bootstrap, error)
	if service != nil {
		bootstrap = service.MatrixBootstrap
	}
	c.Panel = &cifrado.Panel{
		Base: base, Abrir: abrir, Bootstrap: bootstrap,
		Custodio:   c.custodio(),
		AlContexto: func(ctx context.Context) { sala.ctx = ctx },
		Lienzo: func() fyne.Canvas {
			if canvas := fyne.CurrentApp().Driver().CanvasForObject(sala.P.Objeto()); canvas != nil {
				return canvas
			}
			for _, win := range fyne.CurrentApp().Driver().AllWindows() {
				if win != nil && win.Canvas() != nil {
					return win.Canvas()
				}
			}
			return nil
		},
		Traducir: func(key string) string { return sala.P.T.T(key, nil) },
		Copiar:   func(value string) { sala.Copiar(value) },
		Avisar:   func(message string) { sala.P.Avisar(ui.AvisoPeligro, message) },
		Personas: func() []cifrado.Persona {
			peers := make([]cifrado.Persona, 0, len(sala.roster))
			for _, member := range sala.roster {
				if member.ID != sala.Yo {
					peers = append(peers, cifrado.Persona{Subject: member.ID, Name: member.Nombre})
				}
			}
			return peers
		},
		Licencias: "\n" + llamada.ThirdPartyNotices,
	}
	return c
}

type custodioClave = cifrado.CustodioClave

// custodio evita la trampa de la interfaz con un *api.Rooms nil dentro.
func (c *Cifrado) custodio() custodioClave {
	if c.API == nil {
		return nil
	}
	return c.API
}

func (c *Cifrado) restablecer()        { c.Restablecer() }
func (c *Cifrado) t(key string) string { return c.Traducir(key) }
