// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"encoding/base64"
	"fmt"
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// LAS TESELAS DE APPS CONECTADAS, IGUALES QUE LAS PROPIAS.
//
// Vistos en ROJO con la tesela de antes: caja `rounded-lg border-border` con un cuadro
// de 42 y su propio alto (en Marco medía 117 contra los 62 de las propias), fuera de
// las flechas —desde la rejilla de producto no se llegaba— y, sin icono, un cuadro gris
// vacío.

// conectadasDePrueba son dos apps de terceros: una con icono y otra sin él.
func conectadasDePrueba() []api.AppConectada {
	svg := base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="4" fill="#e03"/></svg>`))
	return []api.AppConectada{
		{
			Key: "i1", Name: "Holded", Description: "Facturación y contabilidad",
			Href: "https://app.holded.com", IconURL: "data:image/svg+xml;base64," + svg,
		},
		{Key: "i2", Name: "factorial", Description: "Personas y nóminas", Href: "https://factorial.test"},
	}
}

// TestLasConectadasMidenLoQueLasPropias: en las dos formas y a 1280 y 390, en claro y
// oscuro, la tesela conectada ocupa la misma caja que la de producto de su columna.
func TestLasConectadasMidenLoQueLasPropias(t *testing.T) {
	t.Cleanup(func() {
		tema.Elegir("")
		tema.FormaDeLosGuardianes()
	})
	for _, forma := range []string{"marco", "carril"} {
		for _, modo := range []string{"claro", "oscuro"} {
			for _, ancho := range []float32{1280, 390} {
				nombre := fmt.Sprintf("conectadas-%s-%s-%g", forma, modo, ancho)
				t.Run(nombre, func(t *testing.T) {
					tema.ElegirForma(forma)
					tema.Elegir(modo)
					l := lanzadorDePrueba(t)
					l.Conectadas = conectadasDePrueba()
					captura, r := pinta(t, l, ancho)
					guardar(t, captura, nombre)
					if len(r.conectadas) != 2 {
						t.Fatalf("hay %d conectadas pintadas y son dos", len(r.conectadas))
					}
					propia := r.teselas[0]
					for i, c := range r.conectadas {
						if c.Size().Width != propia.Size().Width {
							t.Errorf("la conectada mide %v de ancho y la propia %v", c.Size().Width, propia.Size().Width)
						}
						if x := r.teselas[i].Position().X; c.Position().X != x {
							t.Errorf("la conectada %d empieza en x=%v y su columna en %v", i, c.Position().X, x)
						}
						// En Marco, la compacta: una línea de descripción y, en el
						// teléfono, las que pida hasta dos, con la misma regla que las propias.
						quiero := AltoTeselaCompactaCon(LineasCompacta(c.App.Description, c.Size().Width, ancho < ui.CorteSM))
						if forma == "marco" && !cerca(c.Size().Height, quiero) {
							t.Errorf("en Marco la conectada mide %v de alto y la propia %v", c.Size().Height, quiero)
						}
					}
					// El borde es el de la de producto (`border-border-subtle`).
					v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
					if got, quiero := r.conectadas[0].borde(v), propia.borde(v); got != quiero {
						t.Errorf("el borde de la conectada es %v y el de la propia %v", got, quiero)
					}
				})
			}
		}
	}
}

// TestLasFlechasPasanDeUnaRejillaALaOtra: desde la última tesela propia, → lleva a la
// primera conectada y ← vuelve; ↓ desde la última fila propia baja a las conectadas, y
// Fin va a la última conectada.
func TestLasFlechasPasanDeUnaRejillaALaOtra(t *testing.T) {
	// La web junta las dos rejillas en una galería: si deja de hacerlo, esto ya no es
	// paridad.
	web := webref.Read(t, "apps/frontend/apps/portal/src/components/Launcher.tsx")
	if !strings.Contains(web, "data-hub-galeria") {
		t.Fatal("el lanzador web ya no junta las dos rejillas en `[data-hub-galeria]`")
	}
	l := lanzadorDePrueba(t)
	l.Conectadas = conectadasDePrueba()
	var r *lanzadorRenderer
	var w fyne.Window
	fyne.DoAndWait(func() {
		w = test.NewWindow(l)
		w.SetPadded(false)
		r = test.WidgetRenderer(l).(*lanzadorRenderer)
		l.Resize(fyne.NewSize(ventana, r.alto(ventana)))
		w.Resize(fyne.NewSize(ventana, r.alto(ventana)))
	})
	t.Cleanup(w.Close)
	c := w.Canvas()
	pulsa := func(k fyne.KeyName) {
		fyne.DoAndWait(func() { c.Focused().TypedKey(&fyne.KeyEvent{Name: k}) })
	}
	espera := func(tecla string, want fyne.Focusable) {
		t.Helper()
		if got := c.Focused(); got != want {
			t.Errorf("tras %s el foco está en %v y debía ir a %v", tecla, got, want)
		}
	}
	ultima := r.teselas[len(r.teselas)-1]
	con := r.conectadas
	fyne.DoAndWait(func() { c.Focus(ultima) })
	pulsa(fyne.KeyRight)
	espera("→ desde la última propia", con[0])
	pulsa(fyne.KeyRight)
	espera("→", con[1])
	pulsa(fyne.KeyRight) // borde: no se mueve
	espera("→ en el borde", con[1])
	pulsa(fyne.KeyLeft)
	pulsa(fyne.KeyLeft)
	espera("← desde la primera conectada", ultima)
	pulsa(fyne.KeyEnd)
	espera("Fin", con[1])
	pulsa(fyne.KeyHome)
	espera("Inicio", r.teselas[0])

	// ↓ desde la primera tesela de la última fila propia baja a la primera conectada,
	// que está en su misma columna.
	cols := r.rejillaApps(ventana).Columnas(cajaAnchoDe(ventana))
	primeraUltimaFila := r.teselas[(len(r.teselas)-1)/cols*cols]
	fyne.DoAndWait(func() { c.Focus(primeraUltimaFila) })
	pulsa(fyne.KeyDown)
	espera("↓ desde la última fila propia", con[0])
	pulsa(fyne.KeyUp)
	espera("↑ desde la primera conectada", primeraUltimaFila)
}

// cajaAnchoDe es el ancho de contenido de la portada a esa ventana.
func cajaAnchoDe(ventana float32) float32 {
	_, ancho := CajaContenido(ventana)
	return ancho
}

// TestLaConectadaSinIconoPintaSuInicialSobreNeutro es el test de píxeles: sin icono, el
// distintivo pinta `bg-subtle` (no el acento ni el color de la app) y, en su centro, la
// tinta `text-muted` de la inicial. Antes el cuadro quedaba vacío: todo `bg-subtle`.
func TestLaConectadaSinIconoPintaSuInicialSobreNeutro(t *testing.T) {
	// Las coordenadas son las de Carril en claro: no dependen de `SYTUE_FORMA` ni del
	// modo que haya dejado otro test.
	t.Cleanup(func() {
		tema.Elegir("")
		tema.FormaDeLosGuardianes()
	})
	tema.ElegirForma("carril")
	tema.Elegir("claro")
	a := conectadasDePrueba()[1]
	if InicialDe(a.Name) != "F" || InicialDe("  écija") != "É" || InicialDe("") != "?" {
		t.Fatalf("InicialDe: %q %q %q", InicialDe(a.Name), InicialDe("  écija"), InicialDe(""))
	}
	tesela := NuevaTeselaConectada(a, "factorial — Personas y nóminas", nil)
	img := pintaWidget(t, tesela, func(float32) float32 { return AltoTesela(LineasDescripcion) })
	guardar(t, img, "conectada-sin-icono")
	px := func(x, y float32) color.NRGBA {
		c, g, b, _ := img.At(int(x), int(y)).RGBA()
		return color.NRGBA{R: uint8(c >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
	}
	// La marca va donde la de producto: su caja de 40 en (borde + p-4).
	x0 := tema.Borde + MargenTesela
	lado := iconos.MarcaLG
	neutro := tema.Color("bg-subtle", tema.Claro)
	// Una esquina por dentro del distintivo, lejos de la letra y del redondeo.
	if got := px(x0+lado*0.3, y0(x0)+lado*0.18); !parecido(got, neutro) {
		t.Errorf("el distintivo pinta %v y el neutro `bg-subtle` es %v", got, neutro)
	}
	// Fuera del distintivo (en su margen de 4) va la superficie de la tesela.
	if got, quiero := px(x0+1, y0(x0)+1), tema.Color("surface", tema.Claro); !parecido(got, quiero) {
		t.Errorf("fuera del distintivo pinta %v y la superficie es %v", got, quiero)
	}
	// En el centro hay tinta de la inicial: algún píxel bastante más oscuro que el neutro.
	tinta := false
	for dx := -4; dx <= 4 && !tinta; dx++ {
		for dy := -5; dy <= 5 && !tinta; dy++ {
			p := px(x0+lado/2+float32(dx), y0(x0)+lado/2+float32(dy))
			tinta = int(p.R)+int(p.G)+int(p.B) < int(neutro.R)+int(neutro.G)+int(neutro.B)-120
		}
	}
	if !tinta {
		t.Error("el distintivo de una app sin icono está vacío: falta su inicial")
	}
}

// y0 es la altura de la marca en Carril: la misma que su x (borde + p-4).
func y0(x float32) float32 { return x }
