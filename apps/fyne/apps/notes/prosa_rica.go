// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// CÓMO SE PINTAN LOS BLOQUES RICOS (bloques_ricos.go), con los tokens del tema en claro y
// en oscuro:
//
//	panel        `rounded-md px-3 py-2` sobre el `*-bg` de su tipo, el icono de 16 en su
//	             `*-text` a la izquierda y el texto a 28 del borde
//	desplegable  `rounded-md border px-3 py-2`, con su fila de título (chevron y título en
//	             `font-medium`) encima de lo de dentro
//	estado       mayúsculas en negrita al 80 %, en el `*-text` de su color sobre su `*-bg`,
//	             `rounded-sm px-1`
//	fecha        sobre `bg-subtle`, `rounded-sm px-1`
//	decisión     la marca de decisión en `success-text` en vez del punto
//	código       el lenguaje en `text-xs text-subtle` encima de las líneas
//
// El color de cada valor es un token y no un hexadecimal: así el panel «aviso» es el
// mismo ámbar que los avisos de la suite, y en oscuro, el suyo.

// Medidas de los bloques ricos.
var (
	pxCaja               = tema.Espacio(3)
	pyCaja               = tema.Espacio(2)
	margenCaja           = tema.Espacio(3)
	ladoIconoPanel       = float32(16)
	huecoIconoPanel      = tema.Espacio(2)
	pxEtiqueta           = tema.Espacio(1)
	margenEtiquetaCodigo = tema.Espacio(1)
	altoCabeceraCaja     = escalonCuerpo.Linea + tema.Espacio(1)
)

// coloresPanel son el fondo, la tinta y el icono de cada tipo de panel.
func coloresPanel(valor string) (fondo, tinta string, icono iconos.Icono) {
	switch valor {
	case "nota":
		return "violet-bg", "violet-text", iconos.Lapiz
	case "exito":
		return "success-bg", "success-text", iconos.Tic
	case "aviso":
		return "warning-bg", "warning-text", iconos.Aviso
	case "error":
		return "danger-bg", "danger-text", iconos.Peligro
	}
	return "info-bg", "info-text", iconos.Informacion
}

// coloresEstado son el fondo y la tinta de un color de la lista cerrada (los de las
// etiquetas de estado y los fondos de celda).
func coloresEstado(valor string) (fondo, tinta string) {
	switch valor {
	case "azul":
		return "info-bg", "info-text"
	case "verde":
		return "success-bg", "success-text"
	case "amarillo":
		return "warning-bg", "warning-text"
	case "rojo":
		return "danger-bg", "danger-text"
	case "morado":
		return "violet-bg", "violet-text"
	}
	return "bg-subtle", "text-muted"
}

// cajas lleva la cuenta de la caja abierta mientras se maqueta.
type cajas struct {
	m       *Maquetado
	ancho   float32
	rotulo  string
	abierta *Caja
	inicio  float32
	fondo   int
	// primero dice que el bloque que viene es el primero de la caja: no lleva margen
	// de arriba (`first:mt-0`).
	primero bool
	// cerrada dice que la caja abierta es un desplegable cerrado que se está leyendo: de
	// él solo se ve el título. d es el documento, que sabe cuáles se han abierto al leer.
	cerrada bool
	d       *Documento
}

// x es lo que se corre a la derecha el texto dentro de la caja.
func (c *cajas) x(k *Caja) float32 {
	if k.Tipo == CajaPanel {
		return pxCaja + ladoIconoPanel + huecoIconoPanel
	}
	return pxCaja
}

// abrir empieza la caja `k` en el bloque `i`: su margen de fuera, su fondo (que se mide al
// cerrar), su icono o su fila de título y el relleno de arriba.
func (c *cajas) abrir(i int, k *Caja, y, margenPrevio float32) float32 {
	if i > 0 {
		y += max(margenPrevio, margenCaja)
	}
	c.abierta, c.inicio, c.fondo, c.primero = k, y, len(c.m.Adornos), true
	c.cerrada = k.Tipo == CajaExpandir && !c.d.desplegado && !c.d.abiertaAlLeer(k)
	c.m.Adornos = append(c.m.Adornos, Adorno{Tipo: adornoPanel, Bloque: i, X: 0, Y: y, An: c.ancho, caja: k})
	y += pyCaja
	if k.Tipo == CajaPanel {
		c.m.Adornos = append(c.m.Adornos, Adorno{
			Tipo: adornoIcono, Bloque: i, X: pxCaja, Y: y + (escalonCuerpo.Linea-ladoIconoPanel)/2,
			An: ladoIconoPanel, Al: ladoIconoPanel, caja: k,
		})
		return y
	}
	titulo := k.Titulo
	if titulo == "" {
		titulo = c.rotulo
	}
	c.m.Adornos = append(c.m.Adornos, Adorno{
		Tipo: adornoCabecera, Bloque: i, X: pxCaja, Y: y, An: c.ancho - 2*pxCaja, Al: escalonCuerpo.Linea,
		Texto: titulo, caja: k, escalon: escalonCuerpo, Marcada: k.Titulo != "",
		// La flecha mira a la derecha con el desplegable cerrado: escribiendo se ve lo de
		// dentro igual, pero se sabe cómo se leerá.
		plegada: !k.Abierta && c.d.desplegado || c.cerrada,
	})
	if c.cerrada {
		return y + escalonCuerpo.Linea // sin el `mb-1.5` del título de uno abierto
	}
	return y + altoCabeceraCaja
}

// cerrar acaba la caja abierta en `y`: el relleno de abajo y el alto del fondo. Devuelve
// dónde sigue y el margen de debajo.
func (c *cajas) cerrar(y float32) (float32, float32) {
	y += pyCaja
	c.m.Adornos[c.fondo].Al = y - c.inicio
	c.abierta, c.primero, c.cerrada = nil, false, false
	return y, margenCaja
}

// abiertaAlLeer dice si un desplegable se ve abierto en la lectura: su `open`, salvo que
// se haya abierto o cerrado pulsando su título (eso no se guarda, como en la web).
func (d *Documento) abiertaAlLeer(c *Caja) bool { return c.Abierta != d.alternadas[c] }

// alternarAlLeer abre o cierra un desplegable en la lectura, sin tocar la nota.
func (d *Documento) alternarAlLeer(c *Caja) {
	if d.alternadas == nil {
		d.alternadas = map[*Caja]bool{}
	}
	d.alternadas[c] = !d.alternadas[c]
}

// pintarAdornoRico pinta los adornos de los bloques ricos.
func pintarAdornoRico(a Adorno, v tema.Variante) []fyne.CanvasObject {
	switch a.Tipo {
	case adornoPanel:
		if a.caja != nil && a.caja.Tipo == CajaExpandir {
			r := rect(tema.Color("surface", v), a.X, a.Y, a.An, a.Al, tema.RadioMD)
			r.StrokeColor, r.StrokeWidth = tema.Color("border", v), tema.Borde
			return []fyne.CanvasObject{r}
		}
		fondo, _, _ := coloresPanel(a.caja.Valor)
		return []fyne.CanvasObject{rect(tema.Color(fondo, v), a.X, a.Y, a.An, a.Al, tema.RadioMD)}
	case adornoIcono:
		_, tinta, icono := coloresPanel(a.caja.Valor)
		d := iconos.Trazo(icono, a.An, tinta, v)
		d.Colocar(a.X, a.Y, a.An)
		return []fyne.CanvasObject{d.Objeto()}
	case adornoCabecera:
		flecha := iconos.ChevronAbajo
		if a.plegada {
			flecha = iconos.ChevronDerecha
		}
		d := iconos.Trazo(flecha, 14, "text-muted", v)
		d.Colocar(a.X, a.Y+(a.Al-14)/2, 14)
		tinta := "text"
		if !a.Marcada {
			tinta = "text-subtle"
		}
		t := ui.Texto(a.Texto, a.escalon, ui.Medio, tema.Color(tinta, v))
		ui.Truncar(t, a.Texto, a.escalon, ui.Medio, max(a.An-14-tema.Espacio(1.5), 0))
		ui.ColocarTexto(t, a.escalon, a.X+14+tema.Espacio(1.5), a.Y)
		return []fyne.CanvasObject{d.Objeto(), t}
	case adornoDecision:
		lado := a.escalon.Tam * 1.05
		d := iconos.TrazoDeRuta(DecisionD, lado, tema.Color("success-text", v))
		d.Colocar(a.X-a.escalon.Tam*0.75-lado/2, a.Y+(a.Al-lado)/2, lado)
		return []fyne.CanvasObject{d.Objeto()}
	case adornoEtiqueta:
		t := ui.Texto(a.Texto, a.escalon, ui.Medio, tema.Color("text-subtle", v))
		ui.ColocarTexto(t, a.escalon, a.X, a.Y)
		return []fyne.CanvasObject{t}
	case adornoVineta, adornoNumero, adornoCasilla, adornoFilo, adornoFondo, adornoRegla, adornoCaja,
		adornoGuia, adornoNumeroFila, adornoLineas:
	}
	return nil
}

// DecisionD es la marca de una decisión: el camino que se parte en dos y uno sigue.
const DecisionD = "M12 21v-8M12 13 7 8M12 13l5-5M7 8V4.5M7 8h3.5M17 8V4.5M17 8h-3.5"

// estiloEspecial pinta una etiqueta de estado (mayúsculas en negrita al 80 % sobre su
// color suave) o una fecha (sobre `bg-subtle`).
func estiloEspecial(s *estiloTrozo, t Tramo, e estiloBloque, v tema.Variante) {
	if t.Estado != "" {
		fondo, tinta := coloresEstado(t.Estado)
		s.fondo, s.tinta, s.pxFondo = tema.Color(fondo, v), tema.Color(tinta, v), pxEtiqueta
		s.peso, s.mayus, s.codigoLn = ui.Negrita, true, true
		s.escalon = tema.Escalon{Tam: e.escalon.Tam * 0.8, Linea: e.escalon.Linea}
	}
	if t.Fecha != "" {
		s.fondo, s.pxFondo, s.codigoLn = tema.Color("bg-subtle", v), pxEtiqueta, true
	}
}

// nombreLenguaje es el nombre de un lenguaje como se escribe («JavaScript», «C++»).
func nombreLenguaje(l string) string {
	nombres := map[string]string{
		"texto": "Texto", "bash": "Bash", "c": "C", "cpp": "C++", "csharp": "C#", "css": "CSS",
		"go": "Go", "html": "HTML", "java": "Java", "javascript": "JavaScript", "json": "JSON",
		"kotlin": "Kotlin", "markdown": "Markdown", "php": "PHP", "python": "Python", "ruby": "Ruby",
		"rust": "Rust", "sql": "SQL", "swift": "Swift", "typescript": "TypeScript", "xml": "XML", "yaml": "YAML",
	}
	if n, ok := nombres[l]; ok {
		return n
	}
	return l
}
