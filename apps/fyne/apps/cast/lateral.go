// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"strconv"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
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

// Los glifos son los de Nav.tsx, del catálogo compartido. Eran tres propios (una casa
// rellena, un globo y unas ondas) que no eran los de la web.
var destinos = []struct {
	sec   Seccion
	clave string
	icono iconos.Icono
}{
	{SecInicio, "nav.home", iconos.Inicio},
	{SecExplorar, "nav.explore", iconos.Explorar},
	{SecEmitir, "nav.studio", iconos.Emitir},
	{SecSubir, "nav.upload", iconos.Subir},
	{SecModeracion, "nav.moderation", iconos.Escudo},
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
		glifo := iconos.Trazo(d.icono, ui.GlifoMenu, tinta, v.Variante).Objeto()
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
