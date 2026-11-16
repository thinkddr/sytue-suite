// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"fmt"
	"image/color"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// pasosDeCuentaNueva son los de una cuenta recién creada vistos desde la app instalada:
// los cuatro que no son «instalar», con el mismo cálculo que la web.
func pasosDeCuentaNueva() []api.PasoPendiente {
	falso := false
	return api.PrimerosPasos(api.DatosPrimerosPasos{
		Me:             &api.PerfilRevision{HasAvatar: &falso},
		Passkeys:       []api.Passkey{},
		ContrasenasApp: []api.AppPassword{},
		Instalada:      true,
	})
}

// Las medidas de la tarjeta de la web a 1280 y a 390, sacadas de sus clases (rem de 16,
// Figtree, inglés) con la aritmética de CSS: son el contrato, y el guardián compara
// contra ESTOS números, no contra lo que calcula `AltoPara`.
//
//	1280: borde 1 + `pt-3` 12 + cabecera 32 (`h-8` del botón `sm`) + `p-2` 8
//	      + dos filas de 53,875 (`p-2` 8 + título 18 + `mt-0.5` 2 + cuerpo 17,875 + 8)
//	      con `gap-1` 4 + `p-2` 8 + borde 1                                   = 173,75
//	 390: lo mismo en una columna de 324 (342 − 2 − 16), con el «Ocultar» a `min-h-11`
//	      (44) y los cuerpos repartidos a 250 de texto (324 − 16 − 18 − 16 − 2 × 12)
const (
	altoPasos1280 float32 = 173.75
	colPasos1280  float32 = 605 // (1232 − 2 − 16 − 4) / 2
	altoPasos390  float32 = 373 // cabecera 44 y cuatro filas de 71,75 (cuerpos en dos líneas)
)

// TestLosPrimerosPasosMidenLoQueLaWeb pinta la portada a 1280 y a 390 y mira dónde cae la
// tarjeta, cuánto mide, y que empuja las apps su alto más el `mb-4`. Y en los PÍXELES:
// el borde de la tarjeta está donde dice la medida.
//
// Visto en ROJO: con el `sm:grid-cols-2` olvidado (una columna siempre) la tarjeta mide
// 301,5 a 1280; sin el `max-sm:min-h-11` del botón, 361 a 390.
func TestLosPrimerosPasosMidenLoQueLaWeb(t *testing.T) {
	for _, c := range []struct {
		ventana    float32
		alto       float32
		columnas   int
		altoBoton  float32
		anchoColum float32
	}{
		{ventana, altoPasos1280, 2, tema.Espacio(8), colPasos1280},
		{390, altoPasos390, 1, AltoMinimoPaso, 324},
	} {
		sin := lanzadorDePrueba(t)
		_, rSin := pinta(t, sin, c.ventana)
		apps := rSin.rotulo.Objetos()[0].Position().Y

		l := lanzadorDePrueba(t)
		l.Pasos = pasosDeCuentaNueva()
		img, r := pinta(t, l, c.ventana)
		if r.pasos == nil {
			t.Fatalf("a %v: con pasos pendientes no hay tarjeta", c.ventana)
		}
		x, ancho := CajaContenido(c.ventana)
		pos, tam := r.pasos.Position(), r.pasos.Size()
		if !cerca(pos.X, x) || !cerca(pos.Y, MargenY+r.cabecera.alto(ancho)+MargenSeccion) {
			t.Errorf("a %v: la tarjeta está en %v y va bajo la cabecera, en (%v, %v)",
				c.ventana, pos, x, MargenY+r.cabecera.alto(ancho)+MargenSeccion)
		}
		if !cerca(tam.Width, ancho) || !cerca(tam.Height, c.alto) {
			t.Errorf("a %v: la tarjeta mide %v y en la web %v × %v", c.ventana, tam, ancho, c.alto)
		}
		if n, col := r.pasos.columnas(ancho); n != c.columnas || !cerca(col, c.anchoColum) {
			t.Errorf("a %v: %d columnas de %v; en la web %d de %v", c.ventana, n, col, c.columnas, c.anchoColum)
		}
		if got := r.pasos.ocultar.Size().Height; !cerca(got, c.altoBoton) {
			t.Errorf("a %v: «Ocultar» mide %v de alto y en la web %v", c.ventana, got, c.altoBoton)
		}
		if got := r.rotulo.Objetos()[0].Position().Y; !cerca(got, apps+c.alto+MargenPasos) {
			t.Errorf("a %v: las apps empiezan en %v; sin tarjeta en %v, y la tarjeta las baja %v + `mb-4`",
				c.ventana, got, apps, c.alto)
		}
		if !cerca(r.alto(c.ventana), rSin.alto(c.ventana)+c.alto+MargenPasos) {
			t.Errorf("a %v: la portada crece %v y debe crecer lo que la tarjeta y su margen",
				c.ventana, r.alto(c.ventana)-rSin.alto(c.ventana))
		}

		// Los píxeles: a media altura, justo dentro de la tarjeta está la superficie
		// blanca de Card, y justo fuera el papel de la página.
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		medio := int(pos.Y + tam.Height/2)
		if got := nrgba(img.At(int(pos.X)+3, medio)); !parecido(got, tema.Color("surface", v)) {
			t.Errorf("a %v: dentro de la tarjeta hay %v y no la superficie", c.ventana, got)
		}
		if got := nrgba(img.At(int(pos.X)-4, medio)); parecido(got, tema.Color("surface", v)) {
			t.Errorf("a %v: fuera de la tarjeta también hay superficie: no se despega del papel", c.ventana)
		}
		if got := nrgba(img.At(int(pos.X+tam.Width/2), int(pos.Y+tam.Height)+6)); parecido(got, tema.Color("surface", v)) {
			t.Errorf("a %v: bajo la tarjeta, en su `mb-4`, sigue la superficie: mide más de lo que dice", c.ventana)
		}
		guardar(t, img, fmt.Sprintf("primeros-pasos-%v", c.ventana))
	}
}

// nrgba pasa un píxel de la captura a NRGBA, que es como se comparan.
func nrgba(c color.Color) color.NRGBA { return color.NRGBAModel.Convert(c).(color.NRGBA) }

// TestSinPasosNoHayTarjeta: la cuenta ya lista no ve nada, ni un hueco.
func TestSinPasosNoHayTarjeta(t *testing.T) {
	sin := lanzadorDePrueba(t)
	_, rSin := pinta(t, sin, ventana)
	l := lanzadorDePrueba(t)
	l.Pasos = []api.PasoPendiente{}
	_, r := pinta(t, l, ventana)
	if r.pasos != nil || r.alto(ventana) != rSin.alto(ventana) {
		t.Errorf("sin pasos hay tarjeta (%v) o la portada mide %v en vez de %v", r.pasos != nil, r.alto(ventana), rSin.alto(ventana))
	}
}

// TestLaTarjetaDiceCadaPasoConSuTexto: los textos son los del catálogo del portal, los
// mismos de la web, y el de conectar los dispositivos es el del alta.
func TestLaTarjetaDiceCadaPasoConSuTexto(t *testing.T) {
	tr := traductor(t)
	l := lanzadorDePrueba(t)
	l.Pasos = pasosDeCuentaNueva()
	_, r := pinta(t, l, ventana)
	quiero := []string{
		tr.T("firstSteps.mfa.title", nil), tr.T("firstSteps.recovery.title", nil),
		tr.T("firstSteps.photo.title", nil), tr.T("ob.next.devices.title", nil),
	}
	if len(r.pasos.filas) != len(quiero) {
		t.Fatalf("hay %d pasos y son %d", len(r.pasos.filas), len(quiero))
	}
	for i, f := range r.pasos.filas {
		if f.AccessibilityLabel() != quiero[i] {
			t.Errorf("el paso %d se anuncia como %q y es %q", i, f.AccessibilityLabel(), quiero[i])
		}
	}
	if got := r.pasos.titulo.Text; got != tr.T("firstSteps.title", nil) {
		t.Errorf("el título es %q", got)
	}
	if got := r.pasos.ocultar.AccessibilityLabel(); got != tr.T("firstSteps.hideAria", nil) {
		t.Errorf("«Ocultar» se anuncia como %q y es %q", got, tr.T("firstSteps.hideAria", nil))
	}
}

// pasosDoble trae los pasos y apunta lo que se oculta, sin tocar los ajustes del equipo.
type pasosDoble struct {
	pasos    []api.PasoPendiente
	pedidos  int
	ocultas  map[string]bool
	ocultada chan string
}

func vistaConPasos(t *testing.T, d *pasosDoble) (*Vista, *abridorDoble) {
	t.Helper()
	a := &abridorDoble{}
	v := NuevaVista(traductor(t), nil, time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)).
		ConCuenta(cuentaDoble{auth.Reclamos{Iss: "https://id.sytue.test", Sub: "u-ana", Tenant: "acme"}}).
		ConPrimerosPasos("https://id.sytue.test", func(context.Context) ([]api.PasoPendiente, bool) {
			d.pedidos++
			return d.pasos, true
		})
	v.abrir = a
	v.pasosOcultos = func(h string) bool { return d.ocultas[h] }
	v.ocultarPasos = func(h string) error { d.ocultada <- h; return nil }
	return v, a
}

// TestCadaPasoLlevaADondeSeArregla: los de la cuenta abren «Mi cuenta» en su sección del
// navegador (solo es web), y conectar los dispositivos es la pantalla del Hub.
func TestCadaPasoLlevaADondeSeArregla(t *testing.T) {
	d := &pasosDoble{pasos: pasosDeCuentaNueva(), ocultada: make(chan string, 1)}
	v, a := vistaConPasos(t, d)
	v.cargarCuenta()
	esperar(t, "que lleguen los pasos", func() bool {
		v.mu.Lock()
		defer v.mu.Unlock()
		return len(v.lanzador.Pasos) == 4
	})
	r := test.WidgetRenderer(v.lanzador).(*lanzadorRenderer)
	test.WidgetRenderer(r.pasos)
	for _, f := range r.pasos.filas {
		test.Tap(f)
	}
	quiero := []string{
		"https://id.sytue.test/account#seguridad",
		"https://id.sytue.test/account#recuperacion",
		"https://id.sytue.test/account#perfil",
	}
	if len(a.abiertas) != len(quiero) {
		t.Fatalf("se ha abierto %v; son %v", a.abiertas, quiero)
	}
	for i := range quiero {
		if a.abiertas[i] != quiero[i] {
			t.Errorf("el paso %d abre %s y es %s", i, a.abiertas[i], quiero[i])
		}
	}
	if v.Destino() != ADispositivos {
		t.Errorf("conectar los dispositivos deja la pantalla en %s", v.Destino().Ruta())
	}
}

// TestOcultarQuitaLaTarjetaYSeRecuerda: «Ocultar» la quita al momento y lo guarda con la
// huella de emisor, persona y organización; con esa huella guardada no se piden siquiera.
func TestOcultarQuitaLaTarjetaYSeRecuerda(t *testing.T) {
	d := &pasosDoble{pasos: pasosDeCuentaNueva(), ocultada: make(chan string, 1), ocultas: map[string]bool{}}
	v, _ := vistaConPasos(t, d)
	v.cargarCuenta()
	esperar(t, "que lleguen los pasos", func() bool {
		v.mu.Lock()
		defer v.mu.Unlock()
		return len(v.lanzador.Pasos) > 0
	})
	tarjeta := test.WidgetRenderer(v.lanzador).(*lanzadorRenderer).pasos
	test.WidgetRenderer(tarjeta)
	test.Tap(tarjeta.ocultar)
	esperar(t, "que se vaya la tarjeta", func() bool {
		v.mu.Lock()
		defer v.mu.Unlock()
		return v.lanzador.Pasos == nil && test.WidgetRenderer(v.lanzador).(*lanzadorRenderer).pasos == nil
	})
	var huella string
	select {
	case huella = <-d.ocultada:
	case <-time.After(time.Second):
		t.Fatal("«Ocultar» no se ha guardado")
	}
	if huella == "" || huella == "u-ana" {
		t.Fatalf("se guarda %q: ni vacía ni el sujeto en claro", huella)
	}

	d.ocultas[huella] = true
	d.pedidos = 0
	v2, _ := vistaConPasos(t, d)
	v2.cargarCuenta()
	fyne.DoAndWait(func() {})
	if d.pedidos != 0 || len(v2.lanzador.Pasos) != 0 {
		t.Errorf("ocultada, se piden %d veces y salen %d pasos", d.pedidos, len(v2.lanzador.Pasos))
	}
	// En otra organización, la misma persona sí los ve.
	v3 := NuevaVista(traductor(t), nil, time.Now()).
		ConCuenta(cuentaDoble{auth.Reclamos{Iss: "https://id.sytue.test", Sub: "u-ana", Tenant: "beta"}}).
		ConPrimerosPasos("", func(context.Context) ([]api.PasoPendiente, bool) { d.pedidos++; return d.pasos, true })
	v3.pasosOcultos = func(h string) bool { return d.ocultas[h] }
	v3.cargarCuenta()
	if d.pedidos != 1 {
		t.Error("ocultar en Acme la oculta también en otra organización")
	}
}
