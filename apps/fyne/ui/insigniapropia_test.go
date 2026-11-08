// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// TestLaInsigniaPropiaMandaSobreLaDelHub: mientras la app no dice nada, el icono lleva
// lo que cuentan los avisos del Hub; desde que lo dice (Rooms, sus no leídos), lleva
// lo suyo, y un aviso nuevo del Hub ya no lo pisa.
func TestLaInsigniaPropiaMandaSobreLaDelHub(t *testing.T) {
	test.NewTempApp(t)
	var puestos []int
	antes := insigniaSistema
	insigniaSistema = func(_ string, n int) error { puestos = append(puestos, n); return nil }
	t.Cleanup(func() { insigniaSistema = antes })

	a := nuevoAvisosBarra(traductorPrueba(t), nil)
	b := &BarraSuite{avisos: a}
	// Sin metadatos la app es «»: cuenta los avisos de origen «», que es este.
	a.items = []api.HubNotification{{ID: "n1"}}
	a.ponerInsignia()
	b.PonerInsignia(7)
	a.items = append(a.items, api.HubNotification{ID: "n2"})
	a.ponerInsignia()
	b.PonerInsignia(0)
	if quiero := []int{1, 7, 0}; !slices.Equal(puestos, quiero) {
		t.Errorf("el icono llevó %v; quiero %v", puestos, quiero)
	}
}

// appRooms es la app de prueba con la identidad de Rooms, que es la que decide qué
// avisos son suyos.
type appRooms struct{ fyne.App }

func (appRooms) Metadata() fyne.AppMetadata {
	return fyne.AppMetadata{ID: "eu.sytue.rooms", Name: "Sytue Rooms"}
}

// TestLaPausaCallaLosAvisosDeLaApp: mientras la app dice que está en pausa, sus avisos
// no salen al sistema y se dan por anunciados —al acabar la pausa no suenan de golpe—;
// sin pausa, salen.
func TestLaPausaCallaLosAvisosDeLaApp(t *testing.T) {
	antes := fyne.CurrentApp()
	fyne.SetCurrentApp(appRooms{test.NewTempApp(t)})
	t.Cleanup(func() { fyne.SetCurrentApp(antes) })
	var salidos []string
	avisarAntes := avisarSistema
	avisarSistema = func(a escritorio.Aviso) error { salidos = append(salidos, a.Titulo); return nil }
	t.Cleanup(func() { avisarSistema = avisarAntes })

	a := nuevoAvisosBarra(traductorPrueba(t), nil)
	b := &BarraSuite{avisos: a}
	enPausa := true
	b.CallarAvisosSi(func() bool { return enPausa })
	a.anunciar(api.HubNotification{ID: "n1", Source: "rooms", Title: "Ana"})
	if len(salidos) != 0 || !a.anunciados["n1"] {
		t.Fatalf("en pausa salió %v (anunciado: %v)", salidos, a.anunciados["n1"])
	}
	enPausa = false
	a.anunciar(api.HubNotification{ID: "n1", Source: "rooms", Title: "Ana"})
	a.anunciar(api.HubNotification{ID: "n2", Source: "rooms", Title: "Beto"})
	if len(salidos) != 1 {
		t.Errorf("sin pausa salieron %v; quiero solo el nuevo", salidos)
	}
}
