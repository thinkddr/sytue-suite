// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// altaDoble es un Sytue ID de mentira que apunta cada llamada y guarda lo que se crea.
type altaDoble struct {
	mu        sync.Mutex
	llamadas  []string
	estado    api.AltaEstado
	sub       api.AltaSuscripcion
	falloOrg  error
	checkout  string
	guardados []api.AltaGuardado
}

func (a *altaDoble) apuntar(s string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.llamadas = append(a.llamadas, s)
}

func (a *altaDoble) Estado(context.Context) (api.AltaEstado, error) {
	a.apuntar("estado")
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.estado, nil
}

func (a *altaDoble) Guardar(_ context.Context, g api.AltaGuardado) error {
	a.apuntar("guardar " + g.Mode + " " + g.Step)
	a.mu.Lock()
	a.guardados = append(a.guardados, g)
	if g.Mode != "" {
		a.estado.Mode = g.Mode
	}
	a.estado.Step = g.Step
	a.mu.Unlock()
	return nil
}

func (a *altaDoble) Identificador(_ context.Context, slug string) (api.AltaIdentificador, error) {
	return api.AltaIdentificador{Slug: slug, Available: slug != "tomado", Subdomain: slug + ".sytue.eu", Reason: "Ya está en uso."}, nil
}

func (a *altaDoble) CrearOrganizacion(_ context.Context, nombre, slug string) (api.AltaOrg, error) {
	a.apuntar("crear " + nombre + " " + slug)
	if a.falloOrg != nil {
		return api.AltaOrg{}, a.falloOrg
	}
	o := api.AltaOrg{
		ID: "o1", Name: nombre, Slug: slug, Subdomain: slug + ".sytue.eu",
		DNS: &api.AltaSubdominio{Host: slug + ".sytue.eu", Status: "live"},
	}
	a.mu.Lock()
	a.estado.Organization = &o
	a.mu.Unlock()
	return o, nil
}

func (a *altaDoble) Suscripcion(context.Context) (api.AltaSuscripcion, error) {
	a.apuntar("suscripcion")
	return a.sub, nil
}

func (a *altaDoble) Pagar(_ context.Context, asientos int) (string, error) {
	a.apuntar("pagar")
	return a.checkout, nil
}

func (a *altaDoble) Dominio(_ context.Context, host string) (api.AltaDominio, error) {
	return api.AltaDominio{Host: host, RecordName: "_sytue." + host, RecordValue: "v", Status: "pending"}, nil
}

func (a *altaDoble) Cloudflare(context.Context, api.AltaCloudflarePeticion) (api.AltaCloudflare, error) {
	return api.AltaCloudflare{}, nil
}

func (a *altaDoble) Disponibilidad(_ context.Context, d string) (api.AltaDisponibilidad, error) {
	return api.AltaDisponibilidad{Domain: d, Status: "taken"}, nil
}

func (a *altaDoble) Invitar(_ context.Context, correos, rol string) (int, []api.AltaInvitacion, error) {
	a.apuntar("invitar " + rol)
	return 1, []api.AltaInvitacion{{Email: correos, Status: "sent"}}, nil
}

func (a *altaDoble) CrearRoles(_ context.Context, claves []string) error {
	a.apuntar("roles " + strings.Join(claves, ","))
	return nil
}

func nuevaBienvenidaDePrueba(t *testing.T, a *altaDoble) (*Bienvenida, *bienvenidaRenderer) {
	t.Helper()
	b := NuevaBienvenida(castellano(t))
	b.Alta = a
	// En el acto: la prueba sigue el orden de verdad, y el pintor de texto no es de
	// dos gorutinas a la vez (el driver de pruebas no serializa `fyne.Do`).
	b.lanzar = func(f func()) { f() }
	b.demorar = func(_ time.Duration, f func()) *time.Timer { f(); return nil }
	r := test.WidgetRenderer(b).(*bienvenidaRenderer)
	b.Resize(fyne.NewSize(1280, 800))
	return b, r
}

// TestSlugify: el identificador que se propone es el de la web.
func TestSlugify(t *testing.T) {
	for nombre, quiero := range map[string]string{
		"Acme, S.L.":            "acme-s-l",
		"  Álvaro Núñez Peña  ": "alvaro-nunez-pena",
		"Über—Café  &  Co":      "uber-cafe-co",
		"---":                   "",
		strings.Repeat("a", 45): strings.Repeat("a", 40),
		"una organización con un nombre muy largo de verdad": "una-organizacion-con-un-nombre-muy-largo",
	} {
		if got := Slugify(nombre); got != quiero {
			t.Errorf("Slugify(%q) = %q; quiero %q", nombre, got, quiero)
		}
	}
}

// TestElAltaRetomaDondeSeDejo: al entrar se lee el estado y se coloca en el paso
// guardado, con lo que ya existe en los campos: cerrar a medias no pierde nada.
func TestElAltaRetomaDondeSeDejo(t *testing.T) {
	a := &altaDoble{estado: api.AltaEstado{
		Mode: api.AltaOrganizacion, Step: api.AltaPasoFacturacion, SuiteDomain: "sytue.eu",
		Organization: &api.AltaOrg{ID: "o1", Name: "Acme", Slug: "acme", Subdomain: "acme.sytue.eu"},
		Billing:      &api.AltaFacturacion{LegalName: "Acme SL"},
		Account:      &api.AltaCuenta{Email: "ada@acme.eu", Name: "Ada"},
	}, sub: api.AltaSuscripcion{Seats: 2, ActiveMembers: 3, UnitPriceCents: 500}}
	b, r := nuevaBienvenidaDePrueba(t, a)
	b.Cargar(context.Background(), false)
	if b.paso != 3 || b.nombre != "Acme" || b.slug != "acme" || b.asientos != 3 || b.facturacion.LegalName != "Acme SL" {
		t.Fatalf("retoma en el paso %d con %q/%q, %d asientos y %q", b.paso, b.nombre, b.slug, b.asientos, b.facturacion.LegalName)
	}
	if r.c.legal.Texto() != "Acme SL" || r.c.asientos.Texto() != "3" {
		t.Errorf("los campos no traen lo guardado: %q, %q", r.c.legal.Texto(), r.c.asientos.Texto())
	}
	if quiero := b.T.T("ob.billing.pay", nil); r.c.siguiente.Texto != quiero {
		t.Errorf("sin derecho aún, el botón dice %q y tiene que decir %q", r.c.siguiente.Texto, quiero)
	}
}

// TestElAltaDeUnaOrganizacion recorre el alta entera contra el doble: elegir, crear la
// organización con el identificador propuesto, invitar, pagar en el navegador y, al
// volver con el cobro confirmado, roles y final con la vuelta a entrar.
//
// Visto en ROJO a mano (en el frontal): sin el `guardarYSeguir` del paso de identidad,
// el alta se queda en el paso 2 y el test lo dice.
func TestElAltaDeUnaOrganizacion(t *testing.T) {
	a := &altaDoble{
		estado:   api.AltaEstado{SuiteDomain: "sytue.eu", Account: &api.AltaCuenta{Email: "ada@acme.eu", Name: "Ada"}},
		checkout: "https://pago.ejemplo/c/1",
	}
	b, r := nuevaBienvenidaDePrueba(t, a)
	var abiertas []string
	reentrado := false
	b.AbrirURL = func(u string) { abiertas = append(abiertas, u) }
	b.Reentrar = func(context.Context) error { reentrado = true; return nil }
	b.Cargar(context.Background(), false)

	if !r.c.siguiente.Inerte {
		t.Error("sin elegir para quién es, «Continuar» no puede estar activo")
	}
	r.c.modoOrg.Tapped(nil)
	r.c.siguiente.Tapped(nil)
	if b.paso != 1 {
		t.Fatalf("tras elegir, el paso es %d", b.paso)
	}

	test.Type(r.c.nombre, "Acme Ibérica")
	if b.slug != "acme-iberica" {
		t.Fatalf("el identificador propuesto es %q", b.slug)
	}
	if b.comprobado == nil || !b.comprobado.Available {
		t.Fatalf("el identificador no se ha comprobado: %+v", b.comprobado)
	}
	if r.c.siguiente.Inerte {
		t.Fatal("con nombre e identificador libre, «Continuar» tiene que estar activo")
	}
	r.c.siguiente.Tapped(nil)
	if b.paso != 2 || b.org() == nil {
		t.Fatalf("tras crear la organización, el paso es %d y la organización %v", b.paso, b.org())
	}

	test.Type(r.c.correos, "bea@acme.eu")
	r.c.enviar.Tapped(nil)
	if len(b.invitadas) != 1 || b.invitadas[0].Status != "sent" {
		t.Fatalf("invitaciones: %+v", b.invitadas)
	}
	r.c.siguiente.Tapped(nil) // a facturación
	r.c.siguiente.Tapped(nil) // sin derecho: abre el cobro
	if len(abiertas) != 1 || abiertas[0] != a.checkout || !b.pagoVuelto || b.paso != 3 {
		t.Fatalf("el cobro abre %v, vuelto=%v, paso %d", abiertas, b.pagoVuelto, b.paso)
	}
	a.mu.Lock()
	a.sub.Entitled = true
	a.mu.Unlock()
	r.c.siguiente.Tapped(nil) // «Comprobar el pago»: ya hay derecho
	if b.paso != 4 {
		t.Fatalf("con el pago confirmado, el paso es %d", b.paso)
	}
	r.c.siguiente.Tapped(nil) // roles, sin elegir ninguno
	r.c.siguiente.Tapped(nil) // terminar
	if !reentrado {
		t.Error("al terminar no se ha vuelto a entrar: el token seguiría sin organización")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	quiero := []string{
		"estado", "guardar organization identidad", "crear Acme Ibérica acme-iberica", "suscripcion",
		"guardar organization personas", "invitar member", "estado", "guardar organization facturacion",
		"guardar organization facturacion", "suscripcion", "pagar", "guardar organization facturacion", "suscripcion",
		"guardar organization roles", "guardar organization listo", "guardar  listo",
	}
	if !slices.Equal(a.llamadas, quiero) {
		t.Errorf("llamadas:\n%v\nquiero:\n%v", a.llamadas, quiero)
	}
}

// TestUnFalloDelAltaSeDice: si Sytue ID rechaza algo, se enseña su mensaje —que ya viene
// traducido— y el paso no avanza.
func TestUnFalloDelAltaSeDice(t *testing.T) {
	a := &altaDoble{
		estado: api.AltaEstado{
			Mode: api.AltaIndividual, Step: api.AltaPasoIdentidad, SuiteDomain: "sytue.eu",
			Account: &api.AltaCuenta{Email: "ada@acme.eu", Name: "Ada"},
		},
		falloOrg: &api.Error{Status: 409, Body: `{"error":{"code":"conflict","message":"Ese identificador está reservado."}}`},
	}
	b, r := nuevaBienvenidaDePrueba(t, a)
	var avisos []string
	b.Avisar = func(_ ui.TonoAviso, titulo, cuerpo string) { avisos = append(avisos, titulo+" | "+cuerpo) }
	b.Cargar(context.Background(), false)
	b.mu.Lock()
	b.nombre, b.slug = "Ada", "ada"
	b.comprobado = &api.AltaIdentificador{Slug: "ada", Available: true}
	b.mu.Unlock()
	b.Refresh()
	r.c.siguiente.Tapped(nil)
	if b.paso != 1 || len(avisos) != 1 || !strings.HasSuffix(avisos[0], "Ese identificador está reservado.") {
		t.Errorf("paso %d, avisos %v", b.paso, avisos)
	}
}

// TestElAltaMideLoQueMideLaWeb ata el primer paso al marcado de `bienvenida.astro` con
// `Onboarding.tsx` dentro, con la hoja del portal, medido con Chromium en el frontal a
// 1280 y a 360 (sin la topbar: la columna empieza en y = 0). «Continuar» va a la
// derecha, así que se compara su borde derecho. Remedido en Chromium 2026-10-04
// sobre Onboarding real, Figtree y la hoja actual con raíz16, sin topbar.
//
// Visto en ROJO a mano (en el frontal): con el `sm:py-12` quitado de `colocar` —el
// `py-8` a todos los anchos—, a 1280 todo sube 14 y el test lo dice pieza a pieza.
func TestElAltaMideLoQueMideLaWeb(t *testing.T) {
	type caja struct{ x, y, w, h float32 }
	casos := []struct {
		ventana                            float32
		titulo, entrada, pasos, encabezado caja
		descripcion, persona, organizacion caja
		pieY, derecha                      float32
	}{
		{
			ventana: 1280,
			titulo:  caja{280, 48, 720, 36}, entrada: caja{280, 88, 720, 22},
			pasos: caja{280, 142, 720, 28}, encabezado: caja{280, 194, 720, 30},
			descripcion: caja{280, 230, 672, 22},
			persona:     caja{280, 276, 672, 93.75}, organizacion: caja{280, 381.75, 672, 75.875},
			pieY: 498.625, derecha: 1000.0,
		},
		{
			ventana: 360,
			titulo:  caja{16, 32, 328, 72}, entrada: caja{16, 108, 328, 66},
			pasos: caja{16, 206, 328, 28}, encabezado: caja{16, 258, 328, 30},
			descripcion: caja{16, 294, 328, 44},
			persona:     caja{16, 362, 328, 111.625}, organizacion: caja{16, 485.625, 328, 111.625},
			pieY: 638.25, derecha: 344.0,
		},
	}
	cerca := func(a, b, tol float32) bool { return a-b <= tol && b-a <= tol }
	for _, c := range casos {
		a := &altaDoble{estado: api.AltaEstado{SuiteDomain: "sytue.eu", Account: &api.AltaCuenta{Email: "ada@acme.eu"}}}
		b, r := nuevaBienvenidaDePrueba(t, a)
		b.Cargar(context.Background(), false)
		b.mu.Lock()
		b.modo = api.AltaOrganizacion
		b.mu.Unlock()
		ven := test.NewWindow(b)
		ven.SetPadded(false)
		b.Resize(fyne.NewSize(c.ventana, r.alto(c.ventana)))
		ven.Resize(fyne.NewSize(c.ventana, r.alto(c.ventana)))
		guardar(t, ven.Canvas().Capture(), "alta-"+map[float32]string{1280: "ancho", 360: "movil"}[c.ventana])
		mira := func(nombre string, o fyne.CanvasObject, quiero caja) {
			p, s := o.Position(), o.Size()
			if !cerca(p.X, quiero.x, 0.1) || !cerca(p.Y, quiero.y, 0.1) || !cerca(s.Width, quiero.w, 0.5) || !cerca(s.Height, quiero.h, 0.1) {
				t.Errorf("a %v, %s está en (%v, %v) y mide %v × %v; en la web (%v, %v) y %v × %v",
					c.ventana, nombre, p.X, p.Y, s.Width, s.Height, quiero.x, quiero.y, quiero.w, quiero.h)
			}
		}
		mira("el título", r.titulo, c.titulo)
		mira("la entrada", r.entrada, c.entrada)
		mira("el progreso", r.c.pasos, c.pasos)
		mira("el encabezado del paso", r.encabezado, c.encabezado)
		mira("su frase", r.descripcion, c.descripcion)
		mira("«Para mí»", r.c.modoPersona, c.persona)
		mira("«Para mi organización»", r.c.modoOrg, c.organizacion)
		p, s := r.c.siguiente.Position(), r.c.siguiente.Size()
		if !cerca(p.Y, c.pieY, 0.1) || !cerca(p.X+s.Width, c.derecha, 0.1) {
			t.Errorf("a %v, «Continuar» está en y = %v y acaba en x = %v; en la web %v y %v", c.ventana, p.Y, p.X+s.Width, c.pieY, c.derecha)
		}
		ven.Close()
	}
}
