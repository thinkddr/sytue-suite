// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package despertar

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/apns"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/hub/internal/store"
)

type almacenFalso struct {
	mu        sync.Mutex
	reservar  []store.Aparato
	pedidos   [][]string // bundles de cada ReservarDespertar
	tenants   []string
	borrados  []store.Aparato
	pendiente []store.Aparato
}

func (a *almacenFalso) ReservarDespertar(ctx context.Context, _ string, bs []string, v time.Duration) ([]store.Aparato, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if v != Ventana {
		return nil, errors.New("ventana inesperada")
	}
	t, _ := tenancy.From(ctx)
	a.tenants = append(a.tenants, t.Slug)
	a.pedidos = append(a.pedidos, bs)
	return a.reservar, nil
}

func (a *almacenFalso) BarrerPendientes(context.Context, time.Duration, int) ([]store.Aparato, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := a.pendiente
	a.pendiente = nil
	return p, nil
}

func (a *almacenFalso) BorrarAparato(_ context.Context, ap store.Aparato) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.borrados = append(a.borrados, ap)
	return nil
}

type enviadorFalso struct {
	mu       sync.Mutex
	enviados []string // topic/token
	caducos  map[string]bool
}

func (e *enviadorFalso) Despertar(_ context.Context, topic, token string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enviados = append(e.enviados, topic+"/"+token)
	if e.caducos[token] {
		return apns.ErrTokenCaduco
	}
	return nil
}

func sincrono(a Almacen, e Enviador) *Despertador {
	d := Nuevo(a, e)
	d.lanzar = func(f func()) { f() }
	return d
}

func TestUnCambioDespiertaLaAppDeSuProducto(t *testing.T) {
	al := &almacenFalso{reservar: []store.Aparato{{Tenant: "acme", Bundle: "eu.sytue.calendar", Token: "t1"}}}
	en := &enviadorFalso{}
	sincrono(al, en).Cambio(context.Background(), "acme", "ana", "calendar")
	if len(al.pedidos) != 1 || !slices.Equal(al.pedidos[0], []string{"eu.sytue.calendar", BundleHub}) {
		t.Fatalf("bundles pedidos = %v: Calendar despierta a Calendar y a One", al.pedidos)
	}
	if al.tenants[0] != "acme" {
		t.Fatalf("la reserva va con la organización del cambio, no con %q", al.tenants[0])
	}
	if !slices.Equal(en.enviados, []string{"eu.sytue.calendar/t1"}) {
		t.Fatalf("enviados = %v", en.enviados)
	}
}

// One se despierta con lo que sale en su widget: los contadores de Mail y Calendar y
// la actividad reciente (Drive y lo que vive en él, Bases, ERP, Notes). Un cambio de
// Contacts, Rooms o Cast no mueve ese widget. Visto en rojo con la regla vieja (solo
// Mail y Calendar).
func TestOneSeDespiertaConLoQueEnsenaSuWidget(t *testing.T) {
	for _, p := range []string{"mail", "calendar", "drive", "docs", "sheets", "slides", "pdf", "photos", "bases", "erp", "notes"} {
		if bs := BundlesDe(p); !slices.Contains(bs, BundleHub) {
			t.Errorf("un cambio de %q despierta %v y no a One", p, bs)
		}
	}
	for _, p := range []string{"contacts", "rooms", "cast"} {
		if bs := BundlesDe(p); slices.Contains(bs, BundleHub) {
			t.Errorf("un cambio de %q despierta a One (%v), y su widget no lo enseña", p, bs)
		}
	}
	for p := range deOne {
		if _, ok := bundles[p]; !ok {
			t.Errorf("%q en deOne no es un producto de la señal", p)
		}
	}
	// Y son los mismos con que One republica con la app abierta: otro módulo, así que
	// se lee del fuente.
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "apps", "nucleo", "widgets", "fuentes", "hub.go"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^var ProductosDeHub = \[\]string\{([^}]*)\}`).FindSubmatch(b)
	if m == nil {
		t.Fatal("no encuentro fuentes.ProductosDeHub")
	}
	comillas := regexp.MustCompile(`"([a-z]+)"`).FindAllSubmatch(m[1], -1)
	deLaApp := make([]string, 0, len(comillas))
	for _, q := range comillas {
		deLaApp = append(deLaApp, string(q[1]))
	}
	slices.Sort(deLaApp)
	delHub := slices.Sorted(maps.Keys(deOne))
	if !slices.Equal(deLaApp, delHub) {
		t.Fatalf("One republica con %v y el Hub la despierta con %v", deLaApp, delHub)
	}
}

func TestLoQueNoSePuedeDespertarNoSeReserva(t *testing.T) {
	al := &almacenFalso{}
	d := sincrono(al, &enviadorFalso{})
	d.Cambio(context.Background(), "acme", hubnotify.PorCorreo("ana@acme.eu"), "calendar")
	d.Cambio(context.Background(), "acme", "ana", "inventado")
	d.Cambio(context.Background(), "", "ana", "mail")
	d.Cambio(context.Background(), "acme", " ", "mail")
	if len(al.pedidos) != 0 {
		t.Fatalf("pedidos = %v: ni por correo, ni otro producto, ni sin organización o sujeto", al.pedidos)
	}
	var nada *Despertador
	nada.Cambio(context.Background(), "acme", "ana", "mail") // sin credenciales: no-op
	nada.Barrer(context.Background())
	if Nuevo(al, nil) != nil {
		t.Fatal("sin enviador no hay despertador")
	}
}

func TestUnTokenCaducoSeBorraYLosDemasSiguen(t *testing.T) {
	al := &almacenFalso{pendiente: []store.Aparato{
		{Tenant: "acme", Bundle: "eu.sytue.mail", Token: "viejo"},
		{Tenant: "acme", Bundle: "eu.sytue.mail", Token: "bueno"},
	}}
	en := &enviadorFalso{caducos: map[string]bool{"viejo": true}}
	sincrono(al, en).Barrer(context.Background())
	if len(en.enviados) != 2 {
		t.Fatalf("enviados = %v: el caducado no corta el lote", en.enviados)
	}
	if len(al.borrados) != 1 || al.borrados[0].Token != "viejo" {
		t.Fatalf("borrados = %v", al.borrados)
	}
}

func TestUnCambioDespiertaComoMuchoCincuenta(t *testing.T) {
	al := &almacenFalso{}
	for range maxPorCambio + 10 {
		al.reservar = append(al.reservar, store.Aparato{Tenant: "acme", Bundle: "eu.sytue.rooms", Token: "t"})
	}
	en := &enviadorFalso{}
	sincrono(al, en).Cambio(context.Background(), "acme", "ana", "rooms")
	if len(en.enviados) != maxPorCambio {
		t.Fatalf("enviados = %d, quiero %d", len(en.enviados), maxPorCambio)
	}
}

// Cada producto de la señal de cambios tiene su app, y la app es la del `ID` de su
// FyneApp.toml: un bundle mal escrito sería un despertar que Apple rechaza siempre.
func TestLosBundlesSonLosDeLasApps(t *testing.T) {
	tomls, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "apps", "fyne", "cmd", "*", "FyneApp.toml"))
	if err != nil || len(tomls) < 10 {
		t.Fatalf("no encuentro las apps nativas: %v (%d)", err, len(tomls))
	}
	esquema := regexp.MustCompile(`(?m)^\s*esquema\s*=\s*"sytue-([a-z-]+)"`)
	id := regexp.MustCompile(`(?m)^\s*ID\s*=\s*"([^"]+)"`)
	deLasApps := map[string]string{}
	for _, p := range tomls {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		e, i := esquema.FindSubmatch(b), id.FindSubmatch(b)
		if e == nil || i == nil {
			t.Fatalf("%s sin esquema o sin ID", p)
		}
		deLasApps[string(e[1])] = string(i[1])
	}
	if deLasApps["one"] != BundleHub {
		t.Fatalf("la app One es %q, no %q", deLasApps["one"], BundleHub)
	}
	for _, p := range hubnotify.ProductosDeDatos() {
		bs := BundlesDe(p)
		if len(bs) == 0 || bs[0] != deLasApps[p] {
			t.Errorf("el producto %q despierta %v y su app es %q", p, bs, deLasApps[p])
		}
		for _, b := range bs {
			if !BundleConocido(b) {
				t.Errorf("%q no se puede registrar", b)
			}
		}
	}
	if len(bundles) != len(hubnotify.ProductosDeDatos()) {
		t.Fatalf("hay %d bundles para %d productos", len(bundles), len(hubnotify.ProductosDeDatos()))
	}
	if BundleConocido("eu.sytue.fyne.demo") || BundleConocido("com.ejemplo.app") {
		t.Fatal("solo se registran las apps de la suite")
	}
}
