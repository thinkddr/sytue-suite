// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pantalla

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # El muestrario de aros de foco
//
// Un control de cada familia con el foco encendido, para MEDIR su aro sobre píxeles: en
// los tests, con el pintor de software, y en una ventana de verdad, con el de GL
// (`cmd/demo` con `SYTUE_AROS`, que es lo que lanza `scripts/ventana-real.sh --aros`).
//
// Existe porque los dos pintores de Fyne no trazan igual —el de GL pinta el `StrokeWidth`
// de un rectángulo por DENTRO de su caja y el de software a caballo del contorno— y el
// aro abierto de `ui.Aro` era un trazo: los tests lo veían en su sitio y la ventana real
// un píxel más adentro, y medio tapado donde el control tiene caja. Un test verde no
// decía nada del aro real. La misma medida corre ahora en los dos sitios.

// AnchoAros y AltoAros son los del lienzo del muestrario.
const (
	AnchoAros = 360
	AltoAros  = 560
)

// MuestraAro es un control del muestrario y lo que hace falta saber para medir su aro.
type MuestraAro struct {
	Nombre string
	// Pos y Tam son la caja que rodea el aro.
	Pos fyne.Position
	Tam fyne.Size
	// Sep es el hueco entre la caja y el anillo: `ui.SinHueco` o `ui.ConHueco`.
	Sep float32
	// Token es el color del anillo.
	Token string
	// BordeAlFoco dice que la caja lleva un borde que con el foco toma el color del
	// anillo (un campo de texto): lo de dentro de la caja no se cuenta como aro.
	BordeAlFoco bool
}

// Aros construye el muestrario: una columna de controles sobre el papel, y la fila del
// menú lateral sobre su tinta.
func Aros(v tema.Variante) (fyne.CanvasObject, []MuestraAro) {
	const margen, paso, anchoFijo = 32, 56, 200
	fondo := canvas.NewRectangle(tema.Color("bg", v))
	fondo.Resize(fyne.NewSize(AnchoAros, AltoAros))
	objetos := []fyne.CanvasObject{fondo}
	var muestras []MuestraAro

	y := float32(margen)
	poner := func(nombre string, o enfocable, ancho, sep float32, token string) {
		tam := o.MinSize()
		if ancho > 0 {
			tam.Width = ancho
		}
		o.Resize(tam)
		o.Move(fyne.NewPos(margen, y))
		o.FocusGained() // el aro, encendido sin pasar por el foco del lienzo
		objetos = append(objetos, o)
		muestras = append(muestras, MuestraAro{Nombre: nombre, Pos: o.Position(), Tam: tam, Sep: sep, Token: token})
		y += paso
	}

	poner("enlace", ui.NuevoEnlace("Ver el calendario", tema.TextoBase, ui.Medio, "primary-text", nil), 0, ui.SinHueco, "focus-ring")
	poner("miga", ui.NuevaMiga("Documentos", nil), 0, ui.SinHueco, "focus-ring")
	poner("pestaña", ui.NuevaPestana("Actividad", nil), 0, ui.SinHueco, "focus-ring")
	poner("botón de icono", ui.NuevoBotonIcono(iconos.Ajustes, "Ajustes", nil), 0, ui.SinHueco, "focus-ring")
	poner("botón fantasma", ui.NuevoBoton("Más opciones", nil).Con(ui.Fantasma, ui.TallaMD), 0, ui.ConHueco, "focus-ring")
	poner("opción de menú", ui.NuevoElementoMenu("Cambiar el nombre", iconos.Ninguno, nil), anchoFijo, ui.SinHueco, "focus-ring")
	// Los dos de aro RELLENO, que son la referencia: su caja es opaca y lo tapa.
	poner("botón principal", ui.NuevoBoton("Guardar cambios", nil), 0, ui.ConHueco, "focus-ring")
	poner("campo de texto", ui.NuevaEntrada("Buscar…"), anchoFijo, ui.SinHueco, "focus-ring")
	muestras[len(muestras)-1].BordeAlFoco = true

	// La fila del menú lateral va sobre su tinta, con el aro de allí.
	tinta := canvas.NewRectangle(tema.Color("sidebar-bg", v))
	tinta.Move(fyne.NewPos(0, y-16))
	tinta.Resize(fyne.NewSize(AnchoAros, AltoAros-(y-16)))
	objetos = append(objetos, tinta)
	poner("fila del lateral", ui.NuevaFilaLateral("Mi unidad", nil, nil), anchoFijo, ui.SinHueco, "sidebar-focus")

	return container.NewWithoutLayout(objetos...), muestras
}

// enfocable es un control del muestrario: se coloca y se le enciende el foco.
type enfocable interface {
	fyne.CanvasObject
	FocusGained()
}

// MedidaAro es lo que se ve del aro de una muestra, medido en su borde de arriba y a
// media anchura: lejos de las esquinas y de lo que el control lleva dentro.
type MedidaAro struct {
	Nombre string
	// Grosor es la tinta del anillo, en píxeles lógicos. La web pinta 2.
	Grosor float32
	// Opacidad es la del píxel más cubierto, de 0 a 1. La web pinta el token entero.
	Opacidad float32
	// Centro es dónde cae el anillo respecto al borde de la caja, en píxeles lógicos y
	// negativo hacia fuera. La web lo pone en −1 sin hueco y en −2 con él.
	Centro float32
	// Contraste es el del píxel más cubierto contra lo que el control tiene detrás.
	Contraste float64
}

// MedirAros mide el aro de cada muestra en una captura del muestrario, a la escala que
// traiga la imagen.
func MedirAros(img image.Image, muestras []MuestraAro, v tema.Variante) []MedidaAro {
	escala := float32(img.Bounds().Dx()) / AnchoAros
	en := func(x, y int) color.NRGBA { // la captura es opaca
		r, g, b, _ := img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y).RGBA()
		return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 0xff} //nolint:gosec // G115: 16 bits a 8
	}
	out := make([]MedidaAro, 0, len(muestras))
	for _, m := range muestras {
		x := int((m.Pos.X + m.Tam.Width/2) * escala)
		borde := m.Pos.Y * escala
		// Lo de detrás, bien fuera del aro; y el interior, más allá de donde llega.
		fuera, dentro := en(x, int(borde-6*escala)), en(x, int(borde+3*escala))
		anillo := tema.Color(m.Token, v)
		med := MedidaAro{Nombre: m.Nombre}
		var tinta, momento float32
		hasta := borde + 3*escala
		if m.BordeAlFoco {
			hasta = borde
		}
		for py := int(borde - 6*escala); py < int(hasta); py++ {
			base := fuera
			if float32(py)+0.5 > borde {
				base = dentro
			}
			c := cobertura(en(x, py), base, anillo)
			tinta += c
			momento += c * (float32(py) + 0.5)
			if c > med.Opacidad {
				med.Opacidad, med.Contraste = c, contraste(en(x, py), fuera)
			}
		}
		med.Grosor = tinta / escala
		if tinta > 0 {
			med.Centro = (momento/tinta - borde) / escala
		}
		out = append(out, med)
	}
	return out
}

// ComprobarAros dice qué muestras no pintan el aro de la web: 2 px del token, pegados a
// la caja (o a 1 px, con hueco) y a 3:1 como poco contra lo de detrás.
//
// Las holguras son las del suavizado de borde del pintor de GL a ×1, que deja el píxel
// de un relleno redondeado al 84 %: es de todas las formas de la suite, no del aro.
func ComprobarAros(medidas []MedidaAro, muestras []MuestraAro) []string {
	var fallos []string
	for i, m := range medidas {
		centro := -(muestras[i].Sep + 1)
		if math.Abs(float64(m.Grosor-2)) > 0.3 || m.Opacidad < 0.8 || math.Abs(float64(m.Centro-centro)) > 0.3 || m.Contraste < 3 {
			fallos = append(fallos, fmt.Sprintf("%s: %.2f px al %.0f %%, centrado en %.2f (quiero %.0f) y a %.2f:1",
				m.Nombre, m.Grosor, 100*m.Opacidad, m.Centro, centro, m.Contraste))
		}
	}
	return fallos
}

// TablaAros es la tabla de las medidas, para la terminal y para el informe.
func TablaAros(medidas []MedidaAro) string {
	s := fmt.Sprintf("%-18s %7s %9s %7s %10s\n", "muestra", "grosor", "opacidad", "centro", "contraste")
	for _, m := range medidas {
		s += fmt.Sprintf("%-18s %7.2f %8.0f%% %7.2f %9.2f:1\n", m.Nombre, m.Grosor, 100*m.Opacidad, m.Centro, m.Contraste)
	}
	return s
}

// cobertura es cuánto del anillo hay en un píxel: su proyección en el camino que va del
// color de base al del anillo.
func cobertura(p, base, anillo color.NRGBA) float32 {
	var num, den float32
	for _, c := range [][3]uint8{{p.R, base.R, anillo.R}, {p.G, base.G, anillo.G}, {p.B, base.B, anillo.B}} {
		d := float32(c[2]) - float32(c[1])
		num += (float32(c[0]) - float32(c[1])) * d
		den += d * d
	}
	if den == 0 {
		return 0
	}
	return min(max(num/den, 0), 1)
}

// contraste es el de WCAG entre dos colores opacos.
func contraste(a, b color.NRGBA) float64 {
	luz := func(c color.NRGBA) float64 {
		canal := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= 0.04045 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*canal(c.R) + 0.7152*canal(c.G) + 0.0722*canal(c.B)
	}
	la, lb := luz(a), luz(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}
