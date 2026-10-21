// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

/*
Lo que se hace SOBRE LA HOJA con la herramienta de la cinta: el `onUp` de la `Hoja` de
`PdfApp.tsx` y lo que llama —`anotar`, `censurar`, `marcarCampo`, `crearCampos` y
`recortar`—, con las dos franjas de lo marcado y sin aplicar.

Censurar y crear campos van en DOS pasos, como en la web: lo marcado se ve sobre el
documento y solo al aplicar se escribe. Lo que se censura no vuelve.
*/

// ZonaCensura es una zona marcada para censurar, en coordenadas del PDF. Pagina es
// 0-based, como la del motor.
type ZonaCensura struct {
	Pagina int
	Rect   Rect
}

// CampoMarcado es un campo de formulario dibujado y todavía sin crear.
type CampoMarcado struct {
	Pagina int
	Rect   Rect
	Tipo   string
	// Nombre es lo que escribió quien lo marcó; vacío = se numera al crearlo.
	Nombre string
}

// tiposCampo es `TIPOS_CAMPO`: los que se pueden dibujar, firma incluida.
var tiposCampo = []struct{ valor, clave string }{
	{"texto", "pdf.field.kind.text"},
	{"casilla", "pdf.field.kind.check"},
	{"desplegable", "pdf.field.kind.select"},
	{"firma", "pdf.field.kind.signature"},
}

// tintaElegida es el color de la cinta, en `#rrggbb` y ya leído.
func (p *Pantalla) tintaElegida() (string, color.NRGBA) {
	hex := ColoresAnotacion[0].Hex
	if i := p.barra.Color; i >= 0 && i < len(ColoresAnotacion) {
		hex = ColoresAnotacion[i].Hex
	}
	c, _ := tokens.ParseColor(hex)
	return hex, c
}

// sincronizarHojas lleva a las hojas la herramienta, la tinta y el grosor de la cinta,
// y lo marcado sin aplicar. Lo llama cada recomposición: es barato y no rehace nada que
// no haya cambiado.
func (p *Pantalla) sincronizarHojas() {
	_, tinta := p.tintaElegida()
	hm := p.barra.Herramienta
	if hm != Mano && !p.barra.PuedeAnotar {
		hm = Mano
	}
	for _, h := range p.hojas {
		h.AlGesto = p.alGesto
		h.PonerHerramienta(hm, tinta, float64(p.barra.Grosor))
	}
	if !seleccionaTexto(hm) {
		for _, h := range p.hojas {
			if len(h.seleccion) > 0 {
				h.PonerSeleccion(nil)
			}
		}
	}
	if marcaTexto(hm) {
		p.pedirTramos(p.PaginaActual)
	}
}

// refrescarMarcas vuelve a poner sobre cada hoja lo que le cae encima.
func (p *Pantalla) refrescarMarcas() {
	for _, h := range p.hojas {
		h.PonerMarcas(p.marcasDe(h.pagina))
	}
}

// paginaDe es la descripción de la página `n` (1-based).
func (p *Pantalla) paginaDe(n int) (Pagina, bool) {
	if p.Contenido == nil {
		return Pagina{}, false
	}
	for _, pg := range p.Contenido.Paginas {
		if pg.Numero == n {
			return pg, true
		}
	}
	return Pagina{}, false
}

// rectPDF es la caja de un arrastre en coordenadas del PDF, ordenada.
func rectPDF(pag Pagina, g Gesto) Rect {
	ax, ay := APDF(pag, g.Desde[0], g.Desde[1])
	bx, by := APDF(pag, g.Hasta[0], g.Hasta[1])
	return Rect{math.Min(ax, bx), math.Min(ay, by), math.Max(ax, bx), math.Max(ay, by)}
}

// alGesto es el `onUp` de la `Hoja` (y, con las de texto, lo que pinta la selección
// mientras se arrastra).
func (p *Pantalla) alGesto(h *Hoja, g Gesto, fin bool) {
	hm := h.Herramienta()
	if seleccionaTexto(hm) {
		p.seleccionar(h, g, fin)
		return
	}
	if !fin {
		return
	}
	pag := h.pagina
	hex, _ := p.tintaElegida()
	rgb := ColorARGB(hex)
	grosor := float64(p.barra.Grosor)

	if esTrazo(hm) {
		if len(g.Trazo) < 2 {
			return
		}
		puntos := make([][2]float64, len(g.Trazo))
		for i, q := range g.Trazo {
			x, y := APDF(pag, q[0], q[1])
			puntos[i] = [2]float64{x, y}
		}
		p.anotar([]AnotacionNueva{{
			Pagina: pag.Numero - 1, Tipo: "tinta", Trazos: [][][2]float64{puntos},
			Color: rgb, Opacidad: 1, Grosor: grosor,
		}})
		return
	}

	ax, ay := APDF(pag, g.Desde[0], g.Desde[1])
	bx, by := APDF(pag, g.Hasta[0], g.Hasta[1])
	rect := rectPDF(pag, g)
	// La línea se decide ANTES que la caja: una línea en diagonal también tiene ancho y
	// alto, y mirando primero la caja las tres de línea dibujarían un rectángulo.
	if esLinea(hm) {
		largo := math.Hypot(bx-ax, by-ay)
		// Un clic sin arrastrar no es una línea: quedaría una anotación de longitud cero
		// imposible de ver.
		if largo < 2 {
			return
		}
		a := AnotacionNueva{
			Pagina: pag.Numero - 1, Tipo: tipoDeLinea(hm), Puntos: [][2]float64{{ax, ay}, {bx, by}},
			Color: rgb, Opacidad: 1, Grosor: grosor,
		}
		if hm == Medida {
			// Una medición sin la medida escrita no mide nada. El PDF cuenta en puntos
			// (1/72") y nadie piensa en puntos: se rotula en mm, con el `toFixed(1)` de
			// la web (punto decimal en todos los idiomas).
			a.Etiqueta = p.tv("pdf.measure.mm", i18n.Vars{"v": strconv.FormatFloat(largo*25.4/72, 'f', 1, 64)})
		}
		p.anotar([]AnotacionNueva{a})
		return
	}
	if rect[2]-rect[0] <= 2 || rect[3]-rect[1] <= 2 {
		return
	}
	switch hm {
	case Censura:
		p.zonas = append(p.zonas, ZonaCensura{Pagina: pag.Numero - 1, Rect: rect})
		p.refrescarMarcas()
		p.Recomponer()
	case CampoNuevo:
		p.marcarCampo(pag.Numero-1, rect)
	case Recorte:
		p.recortar(pag.Numero, rect)
	default:
		p.anotacionDeCaja(hm, pag.Numero-1, rect, rgb, grosor, p.anotar)
	}
}

// tipoDeLinea es el `tipo` que el motor espera para cada herramienta de dos puntos: el
// mismo nombre que la herramienta en la web.
func tipoDeLinea(hm Herramienta) string {
	switch hm {
	case Flecha:
		return "flecha"
	case Medida:
		return "medida"
	default:
		return "linea"
	}
}

// anotar es `anotar`: pone las anotaciones con quien las firma.
func (p *Pantalla) anotar(anots []AnotacionNueva) {
	if len(anots) == 0 {
		return
	}
	for i := range anots {
		anots[i].Autor = p.Autor
	}
	p.Aplicar(api.PDFCambio{Op: "anotar", Datos: anots}, p.t("pdf.saved.annotated"), nil)
}

// anotacionDeCaja es `anotacionDeCaja`: la herramienta de caja se traduce a la
// anotación que le toca. Nota, texto y sello piden su texto con el diálogo de la casa.
func (p *Pantalla) anotacionDeCaja(hm Herramienta, pagina int, rect Rect, rgb [3]float64, grosor float64, listo func([]AnotacionNueva)) {
	base := AnotacionNueva{Pagina: pagina, Rect: &rect, Color: rgb, Opacidad: 1, Grosor: grosor}
	pedir := func(clave, valor string, poner func(*AnotacionNueva, string)) {
		p.preguntar(p.t(clave), valor, true, func(s string) {
			if strings.TrimSpace(s) == "" {
				return
			}
			a := base
			poner(&a, s)
			listo([]AnotacionNueva{a})
		})
	}
	switch hm {
	case Nota:
		pedir("pdf.prompt.note", "", func(a *AnotacionNueva, s string) { a.Tipo, a.Texto = "nota", s })
	case TextoLibre:
		pedir("pdf.prompt.text", "", func(a *AnotacionNueva, s string) { a.Tipo, a.Texto, a.Tamano = "texto", s, 11 })
	case Sello:
		pedir("pdf.prompt.stamp", p.t("pdf.stamp.default"), func(a *AnotacionNueva, s string) { a.Tipo, a.Etiqueta = "sello", s })
	case Elipse:
		base.Tipo = "elipse"
		listo([]AnotacionNueva{base})
	default:
		base.Tipo = "rectangulo"
		listo([]AnotacionNueva{base})
	}
}

// preguntar es `preguntarEnEditor`: el diálogo de la casa con un campo. Cancelar no
// llama a nada. Sin `obligatorio`, vale dejarlo vacío (`permitirVacio`).
func (p *Pantalla) preguntar(mensaje, valor string, obligatorio bool, si func(string)) {
	c := fyne.CurrentApp().Driver().CanvasForObject(p.raiz)
	if c == nil {
		return
	}
	d := ui.PreguntaOpcional(mensaje, valor, p.t("ui.cancel"), p.t("ui.save"), si)
	if obligatorio {
		d = ui.Pregunta(mensaje, valor, p.t("ui.cancel"), p.t("ui.save"), p.t("ui.required"), si)
	}
	d.Mostrar(c)
}

// censurar es `censurar`: confirma (lo borrado no vuelve) y aplica. Las marcas solo se
// sueltan si el servidor las aplicó: borrarlas tras un fallo obligaría a marcarlo todo
// otra vez sin haber conseguido nada.
func (p *Pantalla) censurar() {
	if len(p.zonas) == 0 {
		return
	}
	p.confirmar(p.t("pdf.redact.confirm"), p.t("pdf.redact.apply"), func() {
		datos := make([]map[string]any, len(p.zonas))
		for i, z := range p.zonas {
			datos[i] = map[string]any{"pagina": z.Pagina, "x1": z.Rect[0], "y1": z.Rect[1], "x2": z.Rect[2], "y2": z.Rect[3]}
		}
		p.Aplicar(api.PDFCambio{Op: "redactar", Datos: datos, Sufijo: p.t("pdf.suffix.redacted")}, "", func(hecho bool) {
			if hecho {
				p.zonas = nil
				p.refrescarMarcas()
				p.Recomponer()
			}
		})
	})
}

// marcarCampo es `marcarCampo`: pregunta cómo se llama (se puede dejar vacío para marcar
// veinte casillas seguidas sin contestar veinte veces) y lo deja marcado.
func (p *Pantalla) marcarCampo(pagina int, rect Rect) {
	tipo := p.tipoCampo
	p.preguntar(p.t("pdf.field.name.ask"), "", false, func(nombre string) {
		p.camposNuevos = append(p.camposNuevos, CampoMarcado{Pagina: pagina, Rect: rect, Tipo: tipo, Nombre: strings.TrimSpace(nombre)})
		p.refrescarMarcas()
		p.Recomponer()
	})
}

// crearCampos es `crearCampos`: escribe los marcados en una copia. Sin nombre, uno por
// orden: un `texto3` en la exportación no dice nada, pero el motor los quiere distintos.
func (p *Pantalla) crearCampos() {
	if len(p.camposNuevos) == 0 {
		return
	}
	datos := make([]map[string]any, len(p.camposNuevos))
	for i, c := range p.camposNuevos {
		nombre := c.Nombre
		if nombre == "" {
			nombre = c.Tipo + strconv.Itoa(i+1)
		}
		datos[i] = map[string]any{"tipo": c.Tipo, "nombre": nombre, "pagina": c.Pagina, "rect": c.Rect}
	}
	p.Aplicar(api.PDFCambio{Op: "crear-campos", Datos: datos, Sufijo: p.t("pdf.suffix.form")},
		p.t("pdf.saved.fieldsCreated"), func(hecho bool) {
			if hecho {
				p.camposNuevos = nil
				p.refrescarMarcas()
				p.Recomponer()
			}
		})
}

// recortar es `recortar`: se lleva un trozo de la página como imagen, sin tocar el
// documento. Va al PORTAPAPELES del sistema, que es donde se quiere para pegarlo en un
// correo, y solo si eso no se puede se descarga —como la web cuando
// `navigator.clipboard.write` falla—. La web lo rasteriza del SVG a escala 2
// (`ESCALA_IMAGEN_POR_DEFECTO`); aquí se pide la página a esa escala al motor y se recorta.
func (p *Pantalla) recortar(numero int, rect Rect) {
	pag, ok := p.paginaDe(numero)
	doc := p.doc
	if !ok || doc == nil || p.Contenido == nil {
		return
	}
	c := RectEnSVG(pag, rect)
	escala := escalaDeImagen(pag.Ancho, pag.Alto)
	nombre := nombreDePagina(p.t("pdf.snapshot.file"), numero, p.Contenido.Total, "png")
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		aviso := "pdf.snapshot.failed"
		if datos, err := func() ([]byte, error) {
			imgs, err := doc.Imagenes(ctx, numero, numero, escala)
			if err != nil {
				return nil, err
			}
			return recorteEnPNG(imgs[numero], c, escala)
		}(); err == nil {
			aviso = llevarRecorte(datos, nombre)
		}
		fyne.Do(func() {
			defer p.avisarRed()
			p.Error, p.Aviso = "", p.t(aviso)
			p.Recomponer()
		})
	}()
}

// copiarImagen es el portapapeles del sistema; las pruebas lo cambian.
var copiarImagen = ficheros.CopiarImagen

// llevarRecorte deja el PNG en el portapapeles o, si no se puede, en descargas, y dice
// con qué clave avisar de lo que pasó.
func llevarRecorte(datos []byte, nombre string) string {
	if copiarImagen(datos) == nil {
		return "pdf.snapshot.copied"
	}
	carpeta, err := ficheros.Carpeta()
	if err == nil {
		_, err = ficheros.Guardar(carpeta, nombre, bytes.NewReader(datos))
	}
	if err != nil {
		return "pdf.snapshot.failed"
	}
	return "pdf.snapshot.downloaded"
}

// errSinDibujo es que el motor no supo dibujar la página que se quería recortar.
var errSinDibujo = errors.New("certipdf: el motor no ha dibujado la página del recorte")

// recorteEnPNG es `paginaComoImagen` con `recorte`: un lienzo del tamaño de la zona, con
// fondo blanco (un PNG transparente se ve negro en un visor con tema oscuro), y la página
// dibujada desplazada dentro.
func recorteEnPNG(img image.Image, c CajaSVG, escala float64) ([]byte, error) {
	if img == nil {
		return nil, errSinDibujo
	}
	w := max(1, int(math.Round(c.Ancho*escala)))
	h := max(1, int(math.Round(c.Alto*escala)))
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	b := img.Bounds()
	origen := image.Pt(b.Min.X+int(math.Round(c.X*escala)), b.Min.Y+int(math.Round(c.Y*escala)))
	draw.Draw(dst, dst.Bounds(), img, origen, draw.Over)
	var out bytes.Buffer
	err := png.Encode(&out, dst)
	return out.Bytes(), err
}

// franjaCampos es la de los campos marcados: `bg-bg-subtle text-text`, el número, el
// desplegable del tipo, «Quitar» y «Crear campos».
func (p *Pantalla) franjaCampos() fyne.CanvasObject {
	if len(p.camposNuevos) == 0 {
		return nil
	}
	opciones := make([]string, len(tiposCampo))
	elegida := 0
	for i, tc := range tiposCampo {
		opciones[i] = p.t(tc.clave)
		if tc.valor == p.tipoCampo {
			elegida = i
		}
	}
	// `<label class="flex items-center gap-1.5 text-xs">` con el `<select>` de `border-border
	// bg-surface text-text rounded-md border px-2 py-1 text-xs`.
	s := ui.NuevoSelector(opciones, func(i int) { p.tipoCampo = tiposCampo[i].valor })
	s.Elegida = elegida
	campo := campoPanel(tema.Espacio(1))
	campo.Radio = tema.RadioMD
	s.Propio = &ui.SelectorPropio{Campo: campo, Alto: ui.AltoSelectNativoXS}
	tipo := container.New(filaCentrada{hueco: tema.Espacio(1.5)},
		ui.NuevoRotulo(p.t("pdf.field.type"), tema.TextoXS, ui.Normal, "text"), s)

	quitar := accionDeFranja(p.t("pdf.field.clear"), "text", false, func() {
		p.camposNuevos = nil
		p.refrescarMarcas()
		p.Recomponer()
	})
	quitar.PY = tema.Espacio(1)
	crear := NuevaAccion(p.t("pdf.field.apply"), p.crearCampos)
	crear.PX, crear.PY = tema.Espacio(3), tema.Espacio(1)
	crear.Fondo, crear.FondoEncima, crear.Tinta = "primary", "primary-hover", "on-primary"
	crear.Inerte = p.Trabajando
	f := nuevaFranja("bg-subtle", "text", p.tv("pdf.field.pending", i18n.Vars{"count": len(p.camposNuevos)}),
		p.anchoVentana, tipo, quitar, crear)
	f.entreAcciones = tema.Espacio(2)
	return f
}

// franjaCensura es la de las zonas marcadas: `bg-warning-bg text-warning-text`, el
// número, «Quitar marcas» y «Censurar» con `bg-danger-bg text-danger-text
// border-danger-text/30`.
func (p *Pantalla) franjaCensura() fyne.CanvasObject {
	if len(p.zonas) == 0 {
		return nil
	}
	quitar := accionDeFranja(p.t("pdf.redact.clear"), "warning-text", false, func() {
		p.zonas = nil
		p.refrescarMarcas()
		p.Recomponer()
	})
	quitar.PY = tema.Espacio(1)
	aplicar := NuevaAccion(p.t("pdf.redact.apply"), p.censurar)
	aplicar.PX, aplicar.PY = tema.Espacio(3), tema.Espacio(1)
	aplicar.Fondo, aplicar.FondoEncima, aplicar.Tinta = "danger-bg", "danger-bg", "danger-text"
	aplicar.Borde, aplicar.BordeAlfa = "danger-text", 0.3
	aplicar.Inerte = p.Trabajando
	f := nuevaFranja("warning-bg", "warning-text", p.tv("pdf.redact.pending", i18n.Vars{"count": len(p.zonas)}),
		p.anchoVentana, quitar, aplicar)
	f.entreAcciones = tema.Espacio(2)
	return f
}
