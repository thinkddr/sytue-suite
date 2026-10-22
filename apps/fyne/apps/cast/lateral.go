// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"strconv"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Seccion es un destino de la navegación lateral (Nav.tsx de la web).
type Seccion uint8

// Los cinco destinos de la web: Inicio, Explorar, Emitir, Subir y Denuncias y bloqueos.
const (
	SecInicio Seccion = iota
	SecExplorar
	SecEmitir
	SecSubir
	SecModeracion
)

var destinos = []struct {
	sec   Seccion
	clave string
	glifo string
	icono iconos.Icono
}{
	{SecInicio, "nav.home", glifoInicio, iconos.Ninguno},
	{SecExplorar, "nav.explore", "", iconos.Globo},
	{SecEmitir, "nav.studio", glifoDirecto, iconos.Ninguno},
	{SecSubir, "nav.upload", "", iconos.Subir},
	{SecModeracion, "nav.moderation", "", iconos.Escudo},
}

// Lateral arma el carril con los cinco destinos. `activa` fuera
// de rango (una página de vídeo) no marca ninguno.
func Lateral(v Vista, activa Seccion, alIr func(Seccion)) fyne.CanvasObject {
	filas := make([]fyne.CanvasObject, 0, len(destinos))
	for _, d := range destinos {
		tinta := "sidebar-fg-muted"
		if d.sec == activa {
			tinta = "sidebar-fg"
		}
		var glifo fyne.CanvasObject
		if d.glifo != "" {
			glifo = iconos.TrazoDeRuta(d.glifo, ui.GlifoMenu, tema.Color(tinta, v.Variante)).Objeto()
		} else {
			glifo = iconos.Trazo(d.icono, ui.GlifoMenu, tinta, v.Variante).Objeto()
		}
		sec := d.sec
		f := ui.NuevaFilaLateral(v.Traduce(d.clave), glifo, func() {
			if alIr != nil {
				alIr(sec)
			}
		})
		f.Activa = d.sec == activa
		if d.sec == SecModeracion && v.Denuncias > 0 {
			f.Contador = strconv.Itoa(v.Denuncias)
		}
		filas = append(filas, f)
	}
	return ui.NuevoLateral(v.Traduce("ui.mainNav"), ui.NuevaSeccionLateral("", filas...))
}
