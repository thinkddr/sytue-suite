// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

func TestElContornoMarcoSigueLaSeleccionSinCambiarLaFila(t *testing.T) {
	enMarco(t)
	conPetroleoClaro(t)
	sinAnimacionDelMenu(t)
	t.Cleanup(TactilDePrueba(true))
	for _, caso := range []struct {
		nombre   string
		rama     *RamaLateral
		compacta bool
	}{
		{nombre: "destino"},
		{nombre: "arbol", rama: &RamaLateral{Hoja: true}},
		{nombre: "pie", compacta: true},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			f := NuevaFilaLateral("Recibidos", nil, nil)
			f.Rama, f.Compacta = caso.rama, caso.compacta
			f.Resize(fyne.NewSize(228, 44))
			r := test.WidgetRenderer(f).(*filaRenderer)
			f.Refresh()
			if r.contorno.Visible() {
				t.Fatal("el destino en reposo no debe llevar contorno de selección")
			}
			for _, forma := range []string{"marco", "carril", "marco"} {
				tema.ElegirForma(forma)
				f.Activa = true
				f.Refresh()
				if r.contorno.Visible() != (forma == "marco") {
					t.Fatalf("%s: el contorno no sigue la forma y la selección", forma)
				}
				if f.Size() != fyne.NewSize(228, 44) || r.contorno.Position() != fyne.NewPos(0.5, 0.5) ||
					r.contorno.Size() != fyne.NewSize(227, 43) || r.contorno.StrokeWidth != 1 {
					t.Fatal("el contorno invade el exterior o cambia el área de la fila")
				}
				_, _, _, alfa := r.contorno.FillColor.RGBA()
				if alfa != 0 {
					t.Fatal("el contorno tapa el relleno de la selección")
				}
				if r.contorno.StrokeColor != tokens.Crudo("sidebar-border", tema.Claro) {
					t.Fatal("el contorno no usa el token compartido del lateral")
				}
				f.FocusGained()
				if !r.aro.visible || r.aro.Margen() != 2 || r.contorno.Visible() {
					t.Fatal("la selección sustituye el aro de foco externo de 2 px")
				}
				f.FocusLost()
				if r.contorno.Visible() != (forma == "marco") {
					t.Fatal("al perder foco no vuelve la selección de Marco")
				}
				f.Activa = false
				f.Refresh()
				if r.contorno.Visible() {
					t.Fatal("el contorno se queda después de cambiar de destino")
				}
			}
		})
	}
}

// TestMenuMarcoIntegrado deja originales de los componentes compartidos, sin servidor
// ni datos personales. Se ejecuta en la captura remota; sin opt-in no escribe PNG.
func TestMenuMarcoIntegrado(t *testing.T) {
	base := os.Getenv("SYTUE_CAPTURA_CROMO")
	if base == "" {
		t.Skip("sin SYTUE_CAPTURA_CROMO no se guardan las vistas del menú")
	}
	enMarco(t)
	sinAnimacionDelMenu(t)
	tema.ElegirColor("app")
	tema.ElegirApp("drive")
	t.Cleanup(func() { tema.Elegir(""); tema.ElegirColor(""); tema.ElegirApp("") })
	for _, variante := range []string{"claro", "oscuro", "traslucido"} {
		tema.Elegir(variante)
		for _, tam := range []fyne.Size{{Width: 320, Height: 640}, {Width: 640, Height: 320}, {Width: 1024, Height: 768}} {
			t.Run(fmt.Sprintf("%s/%gx%g", variante, tam.Width, tam.Height), func(t *testing.T) {
				t.Cleanup(TactilDePrueba(tam.Width < Estrecho))
				v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
				activa := NuevaFilaLateral("Mi unidad", iconos.Trazo(iconos.DriveCarpeta, GlifoMenu, "sidebar-fg", v).Objeto(), nil)
				activa.Activa = true
				menu := NuevoLateral("Navegación", NuevaSeccionLateral("Ubicaciones", activa,
					NuevaFilaLateral("Compartido", nil, nil)))
				menu.ConCabecera(NuevoBoton("Nuevo", nil))
				menu.Pie = NuevaFilaLateral("Papelera", iconos.Trazo(iconos.Papelera, GlifoMenu, "sidebar-fg", v).Objeto(), nil)
				a := NuevoArmazon(canvas.NewRectangle(tema.Color("bg", v)))
				a.Menu, a.Titulo = menu, "Sytue Drive"
				b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.Drive)
				t.Cleanup(b.Parar)
				montado := b.Montar(a)
				montado.Resize(tam)
				if tam.Width < Estrecho {
					a.AbrirCajon()
				}
				nombre := fmt.Sprintf("menu-marco-%s-%gx%g", variante, tam.Width, tam.Height)
				guardarEn(t, base, nombre+"-abierto.png", pintaA(t, 1, tam.Width, tam.Height, montado))
				if tam.Width < Estrecho {
					a.arrastrarCierreCajon(-a.Menu.Size().Width / 2)
					guardarEn(t, base, nombre+"-intermedio.png", pintaA(t, 1, tam.Width, tam.Height, montado))
				} else if variante == "claro" {
					activa.FocusGained()
					guardarEn(t, base, nombre+"-foco.png", pintaA(t, 1, tam.Width, tam.Height, montado))
					activa.FocusLost()
				}
			})
		}
	}
}

func TestElPieDeMarcoSeAlineaConElInteriorDelMenu(t *testing.T) {
	enMarco(t)
	conPetroleoClaro(t)
	l := NuevoLateral("Navegación", nil)
	pie := contenidoDePrueba()
	pie.SetMinSize(fyne.NewSize(20, 20))
	l.Pie = pie
	l.Resize(fyne.NewSize(260, 500))
	r := test.WidgetRenderer(l).(*lateralRenderer)
	for _, forma := range []string{"marco", "carril", "marco"} {
		tema.ElegirForma(forma)
		l.Refresh()
		x, ancho := float32(0), float32(260)
		if forma == "marco" {
			x, ancho = l.Pie.Position().X, l.Pie.Size().Width
		}
		if r.filetePie.Position().X != x || r.filetePie.Size().Width != ancho {
			t.Fatalf("%s: el separador del pie no sigue el canalón del menú", forma)
		}
	}
}

func TestElLateralSoloConservaFondoHastaIntegrarseEnMarco(t *testing.T) {
	enMarco(t)
	conPetroleoClaro(t)
	l := NuevoLateral("Navegación", nil)
	l.Resize(fyne.NewSize(260, 500))
	r := test.WidgetRenderer(l).(*lateralRenderer)
	l.Refresh()
	_, _, _, alfa := r.fondo.FillColor.RGBA()
	if !r.fondo.Visible() || alfa != 0xffff {
		t.Fatal("un lateral standalone de Marco queda sin suelo antes de integrarlo el armazón")
	}
	l.SinFondo = true
	l.Refresh()
	if r.fondo.Visible() {
		t.Fatal("el lateral sigue tapando el marco después de integrarlo")
	}
}

func TestElFondoDelCajonMarcoQuedaAncladoALaVentana(t *testing.T) {
	enMarco(t)
	sinAnimacionDelMenu(t)
	tema.Elegir("traslucido")
	tema.ElegirColor("app")
	tema.ElegirApp("notes")
	t.Cleanup(func() { tema.Elegir(""); tema.ElegirColor(""); tema.ElegirApp("") })
	for _, tam := range []fyne.Size{{Width: 320, Height: 640}, {Width: 640, Height: 320}, {Width: 1024, Height: 768}} {
		fondo := nuevoFondoMarco()
		fondo.pintar(tema.Translucido)
		fondo.colocar(fyne.Position{}, tam)
		referencia := pinta(t, tam.Width, tam.Height, fondo.objetos()...)
		a := NuevoArmazon(contenidoDePrueba())
		a.Menu = NuevoLateral("Navegación", nil)
		if tam.Width >= Estrecho { // Cajón del ERP en escritorio.
			carril := contenidoDePrueba()
			carril.SetMinSize(fyne.NewSize(64, 0))
			a.Carril = carril
		}
		a.Resize(tam)
		a.Refresh()
		a.AbrirCajon()
		ancho := a.Menu.Size().Width
		for _, desplazamiento := range []float32{0, -ancho / 2} {
			if desplazamiento < 0 {
				a.arrastrarCierreCajon(desplazamiento)
			}
			img := pinta(t, tam.Width, tam.Height, a)
			for _, x := range []float32{12, (ancho + desplazamiento) / 2} {
				pxX, pxY := int(x*escala), int(tam.Height/2*escala)
				got, quiero := px(img, pxX, pxY), px(referencia, pxX, pxY)
				if !cercaDe(got, quiero, 2) {
					t.Fatalf("%v, desplazamiento %g: fondo del cajón=%v; mismo punto del marco=%v", tam, desplazamiento, got, quiero)
				}
			}
		}
		a.CerrarCajon()
		r := test.WidgetRenderer(a).(*armazonRenderer)
		if r.fondoCajon.liso.Visible() || r.fondoCajon.degradado.Visible() {
			t.Fatal("el fondo del cajón sigue encima de la hoja después de cerrarlo")
		}
	}
}

func TestCarrilConservaLaBaseDelCajonAlCambiarDeForma(t *testing.T) {
	enMarco(t)
	sinAnimacionDelMenu(t)
	tema.Elegir("traslucido")
	t.Cleanup(func() { tema.Elegir("") })
	a := NuevoArmazon(contenidoDePrueba())
	l := NuevoLateral("Navegación", nil)
	a.Menu = l
	a.Resize(fyne.NewSize(390, 600))
	a.Refresh()
	a.AbrirCajon()
	r := test.WidgetRenderer(a).(*armazonRenderer)
	for _, forma := range []string{"carril", "marco", "carril"} {
		tema.ElegirForma(forma)
		a.Refresh()
		if r.base.Visible() != (forma == "carril") || l.SinFondo != (forma == "marco") {
			t.Fatalf("%s: el cajón mezcla el suelo de Carril con el fondo de Marco", forma)
		}
		if forma == "carril" && (r.fondoCajon.liso.Visible() || r.fondoCajon.degradado.Visible()) {
			t.Fatal("el fondo de Marco se queda sobre el cajón de Carril")
		}
	}
}
