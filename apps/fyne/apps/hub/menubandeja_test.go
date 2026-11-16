// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// rotulos son las etiquetas del menú, con «—» por cada separador.
func rotulos(m *fyne.Menu) []string {
	var out []string
	for _, e := range m.Items {
		if e.IsSeparator {
			out = append(out, "—")
			continue
		}
		out = append(out, e.Label)
	}
	return out
}

func elemento(t *testing.T, m *fyne.Menu, rotulo string) *fyne.MenuItem {
	t.Helper()
	for _, e := range m.Items {
		if e.Label == rotulo {
			return e
		}
	}
	t.Fatalf("no hay %q en %v", rotulo, rotulos(m))
	return nil
}

// TestLaBandejaCuentaLosAvisosSinLeer: «Notifications (3)» con avisos por leer y el
// rótulo a secas sin ninguno, y lleva a la bandeja de avisos del Hub.
func TestLaBandejaCuentaLosAvisosSinLeer(t *testing.T) {
	tr := traductor(t)
	var idas []Destino
	m := MenuBandeja(tr, 3, nil, nil, AccionesBandeja{Ir: func(d Destino) { idas = append(idas, d) }})
	if got := rotulos(m); fmt.Sprint(got) != "[Notifications (3)]" {
		t.Fatalf("con 3 sin leer = %v", got)
	}
	m.Items[0].Action()
	if len(idas) != 1 || idas[0] != AAvisos {
		t.Errorf("«Notifications (3)» lleva a %v, quiero la bandeja de avisos", idas)
	}
	if got := rotulos(MenuBandeja(tr, 0, nil, nil, AccionesBandeja{})); fmt.Sprint(got) != "[Notifications]" {
		t.Errorf("sin nada por leer = %v", got)
	}
}

// TestLaBandejaLlevaLasAccionesDelDia: las cuatro de las píldoras, en su orden, y cada
// una salta a su producto con la ruta de la web.
func TestLaBandejaLlevaLasAccionesDelDia(t *testing.T) {
	tr := traductor(t)
	var saltos []string
	m := MenuBandeja(tr, 0, AccesosBandeja(nil), nil, AccionesBandeja{
		Saltar:   func(p Producto, ruta string) { saltos = append(saltos, p.Marca.Clave()+ruta) },
		AbrirURL: func(u string) { t.Errorf("sin sustitutos no se abre ninguna dirección: %s", u) },
	})
	quiero := "[Notifications — Write an email New document Upload to Drive Create an event]"
	if got := rotulos(m); fmt.Sprint(got) != quiero {
		t.Fatalf("menú = %v, quiero %v", got, quiero)
	}
	for _, e := range m.Items[2:] {
		e.Action()
	}
	if got := fmt.Sprint(saltos); got != "[mail/compose docs drive/?upload=1 calendar/new]" {
		t.Errorf("saltos = %s", got)
	}
}

// TestLosAccesosSustituidosNoLlevanLaRutaDeSytue: con Mail en otra aplicación,
// «Escribir un correo» abre ESA aplicación, sin «/compose», y las demás siguen en Sytue.
func TestLosAccesosSustituidosNoLlevanLaRutaDeSytue(t *testing.T) {
	tr := traductor(t)
	accesos := AccesosBandeja(map[string]api.Sustituto{"mail": {Name: "Outlook", Href: "https://outlook.example"}})
	var abiertas, saltos []string
	m := MenuBandeja(tr, 0, accesos, nil, AccionesBandeja{
		Saltar:   func(p Producto, ruta string) { saltos = append(saltos, p.Marca.Clave()+ruta) },
		AbrirURL: func(u string) { abiertas = append(abiertas, u) },
	})
	elemento(t, m, "Write an email").Action()
	elemento(t, m, "Create an event").Action()
	if fmt.Sprint(abiertas) != "[https://outlook.example]" {
		t.Errorf("el correo sustituido abre %v, quiero solo la dirección de la otra aplicación", abiertas)
	}
	if fmt.Sprint(saltos) != "[calendar/new]" {
		t.Errorf("saltos = %v, quiero solo el del calendario", saltos)
	}
	if accesos[0].Ruta != "" || accesos[0].Producto != "" {
		t.Errorf("el acceso sustituido conserva el destino de Sytue: %+v", accesos[0])
	}
}

// TestLaBandejaEnseñaComoMuchoCincoRecientes: en un submenú, los cinco primeros, con
// el título largo recortado, y cada uno abre su dirección.
func TestLaBandejaEnseñaComoMuchoCincoRecientes(t *testing.T) {
	tr := traductor(t)
	hits := make([]api.HubHit, 0, 8)
	for i := range 8 {
		hits = append(hits, api.HubHit{Source: "docs", Title: fmt.Sprintf("Documento %d", i), URL: fmt.Sprintf("https://docs/%d", i)})
	}
	hits[1].Title = strings.Repeat("Presupuesto ", 10)
	hits[2].Title = ""
	var abiertas []string
	m := MenuBandeja(tr, 0, nil, hits, AccionesBandeja{AbrirURL: func(u string) { abiertas = append(abiertas, u) }})
	r := elemento(t, m, "Recent")
	if r.ChildMenu == nil || len(r.ChildMenu.Items) != MaxRecientesBandeja {
		t.Fatalf("recientes = %v, quiero %d en un submenú", r.ChildMenu, MaxRecientesBandeja)
	}
	sub := r.ChildMenu.Items
	if l := []rune(sub[1].Label); len(l) != LargoRotuloBandeja || l[len(l)-1] != '…' {
		t.Errorf("el título largo no se recorta: %q", sub[1].Label)
	}
	if sub[2].Label == "" {
		t.Error("un reciente sin título sale en blanco: lleva el nombre del producto")
	}
	sub[4].Action()
	if fmt.Sprint(abiertas) != "[https://docs/4]" {
		t.Errorf("el quinto abre %v", abiertas)
	}
	// Sin recientes no hay submenú vacío.
	if got := rotulos(MenuBandeja(tr, 0, nil, nil, AccionesBandeja{})); len(got) != 1 {
		t.Errorf("sin recientes = %v", got)
	}
}
