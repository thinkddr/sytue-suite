// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

/*
FIRMAR (`Firmar` de `Paneles.tsx`), de las cuatro maneras de la web: a mano, desde una
imagen, con un certificado EN FICHERO (.p12 o PEM, que firma el motor en el servidor) y
con la aplicación oficial del país.

Y lo que la web no puede: firmar con el certificado del SISTEMA —las tarjetas y el DNIe
por PKCS#11, y los almacenes del sistema según lleguen— en dos pasos, sin que la clave
salga del equipo (firmasistema.go, ADR 0051). «¿Tu certificado está en el sistema…?»
sigue explicando, como en la web, cómo firmarlo con la aplicación de tu país.
*/

// rectFirma es el recuadro donde cae la rúbrica, en puntos de la página.
var rectFirma = [4]float64{56, 56, 296, 130}

// La resolución interna del lienzo de firma (`width={480} height={148}`).
const anchoLienzo, altoLienzo = 480.0, 148.0

// modosDeFirma son las pestañas, en su orden.
var modosDeFirma = []struct{ id, clave string }{
	{"mano", "pdf.dialog.firmar.modo.mano"},
	{"imagen", "pdf.dialog.firmar.modo.imagen"},
	{"certificado", "pdf.dialog.firmar.modo.certificado"},
	{"oficial", "pdf.dialog.firmar.modo.oficial"},
}

// ficheroElegido es lo que dejó un `<input type="file">`: su nombre y sus bytes.
type ficheroElegido struct {
	nombre string
	datos  []byte
}

// dialogoFirmar es el estado del diálogo.
type dialogoFirmar struct {
	a      *dialogoAbierto
	modo   string
	lienzo *lienzoFirma
	imagen ficheroElegido
	// El certificado: un .p12, o el certificado y la clave en PEM.
	pkcs12, certificado, clavePrivada ficheroElegido
	claveP12, motivo, lugar, pagina   *ui.Entrada
	visible, todas, bloquear          bool
	sistemaAbierto                    bool
	// La aplicación oficial.
	lanzada, errorOficial, subiendo bool
	// sistema es la firma con el certificado del sistema (firmasistema.go).
	sistema firmaSistema
}

func nuevoDialogoFirmar(a *dialogoAbierto) *dialogoFirmar {
	d := &dialogoFirmar{a: a, modo: "mano", visible: true}
	d.lienzo = nuevoLienzoFirma(func() { a.rehacer() })
	d.claveP12 = entradaDialogo("", 0)
	d.claveP12.Contrasena(true)
	d.motivo, d.lugar = entradaDialogo("", 0), entradaDialogo("", 0)
	d.pagina = ui.NuevaEntrada("").ConPropia(ui.CampoPropio{MargenX: tema.Espacio(1.5), MargenY: tema.Espacio(1), Texto: tema.TextoXS, Borde: "border"})
	d.pagina.Radio, d.pagina.Ancho = tema.RadioMD, tema.Espacio(16)
	d.pagina.PonerTexto("1")
	return d
}

// listo es si hay algo que firmar: la oficial nunca, porque se firma fuera.
func (d *dialogoFirmar) listo() bool {
	switch d.modo {
	case "mano":
		for _, t := range d.lienzo.trazos {
			if len(t) > 1 {
				return true
			}
		}
		return false
	case "imagen":
		return d.imagen.datos != nil
	case "certificado":
		return d.pkcs12.datos != nil || (d.certificado.datos != nil && d.clavePrivada.datos != nil)
	}
	return false
}

func (d *dialogoFirmar) vista(ancho float32) fyne.CanvasObject {
	a := d.a
	partes := []fyne.CanvasObject{d.pestanas(ancho)}
	switch d.modo {
	case "mano":
		partes = append(partes, d.aMano(ancho))
	case "imagen":
		partes = append(partes, conRotulo(a.t("pdf.dialog.firmar.imagen"),
			d.campoFichero(&d.imagen, a.t("pdf.dialog.firmar.imagen"), extensionesPNGyJPEG...), notaDialogo(a.t("pdf.dialog.firmar.imagenHelp"), ancho)))
	case "certificado":
		partes = append(partes, d.conCertificado(ancho))
	case "oficial":
		partes = append(partes, d.oficial(ancho))
	}
	partes = append(partes, d.opciones(ancho))
	if d.modo == "certificado" {
		partes = append(partes, d.bloquearDespues(ancho))
	}
	verdad := "pdf.dialog.firmar.truthManuscrita"
	if d.modo == "certificado" {
		verdad = "pdf.dialog.firmar.truth"
	}
	partes = append(partes, avisoAmbar(a.t(verdad), ancho))
	a.botonesDialogo(d.firmar)
	// «Aplicar» apagado hasta que haya algo que firmar.
	if a.botonAplicar != nil {
		a.botonAplicar.Inerte = a.botonAplicar.Inerte || !d.listo()
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, partes...)
}

// pestanas es la tira `flex gap-1 border-b pb-2` de los cuatro modos. Si no caben
// (medido a 360: en 288), encogen como en CSS y su rótulo se parte en dos renglones.
func (d *dialogoFirmar) pestanas(ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	botones := make([]*Accion, len(modosDeFirma))
	for i, m := range modosDeFirma {
		id := m.id
		b := NuevaAccion(d.a.t(m.clave), func() { d.modo = id; d.a.rehacer() })
		b.PX, b.PY = tema.Espacio(2.5), tema.Espacio(1)
		if m.id == d.modo {
			b.Fondo, b.Tinta, b.FondoEncima = "selected", "primary-text", ""
		} else {
			b.Tinta, b.TintaEncima = "text-muted", "text"
		}
		botones[i] = b
	}
	return container.New(conLineaAbajo{}, canvas.NewRectangle(tema.Color("border", v)),
		container.New(ui.Margen{Abj: tema.Espacio(2)}, repartirFila(botones, -1, ancho, tema.Espacio(1))))
}

func (d *dialogoFirmar) aMano(ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	limpiar := NuevaAccion(d.a.t("pdf.dialog.firmar.limpiar"), func() {
		d.lienzo.trazos = nil
		d.lienzo.Refresh()
		d.a.rehacer()
	})
	limpiar.Peso, limpiar.PX, limpiar.PY, limpiar.Subrayado = ui.Normal, 0, 0, true
	limpiar.Tinta, limpiar.TintaEncima, limpiar.FondoEncima, limpiar.Radio = "text-muted", "text", "", tema.RadioSM
	d.lienzo.ancho = ancho
	return container.New(ui.Columna{Hueco: tema.Espacio(2)},
		ui.NuevoParrafo(d.a.t("pdf.dialog.firmar.dibuja"), tema.TextoXS, ui.Normal, "text-muted", v).A(ancho),
		d.lienzo, container.New(alPrincipio{}, limpiar))
}

// campoFichero es el `<input type="file" class="text-xs">`, que con el preflight de
// Tailwind se pinta como texto: el botón y el nombre del elegido.
// ponytail: el «ningún fichero elegido» del navegador no tiene clave en el catálogo; sin
// fichero solo se ve el botón.
func (d *dialogoFirmar) campoFichero(f *ficheroElegido, titulo string, extensiones ...string) fyne.CanvasObject {
	elegir := NuevaAccion(d.a.t("pdf.open.pick"), func() { d.elegir(f, titulo, extensiones) })
	elegir.Peso, elegir.PX, elegir.PY, elegir.Tinta, elegir.TintaEncima, elegir.FondoEncima = ui.Normal, 0, 0, "text-muted", "text", ""
	fila := []fyne.CanvasObject{elegir}
	if f.nombre != "" {
		fila = append(fila, ui.NuevoRotulo(f.nombre, tema.TextoXS, ui.Normal, "text-muted"))
	}
	return container.New(ui.Fila{Hueco: tema.Espacio(1)}, fila...)
}

// elegir abre el selector del sistema, filtrado como el `accept` de la web, y guarda el
// fichero elegido.
func (d *dialogoFirmar) elegir(f *ficheroElegido, titulo string, extensiones []string) {
	go func() {
		rutas, err := ficheros.ElegirConTipos(titulo, extensiones...)
		if err != nil || len(rutas) == 0 {
			return
		}
		datos, err := os.ReadFile(rutas[0])
		if err != nil {
			return
		}
		fyne.Do(func() {
			*f = ficheroElegido{nombre: filepath.Base(rutas[0]), datos: datos}
			d.a.rehacer()
		})
	}()
}

func (d *dialogoFirmar) conCertificado(ancho float32) fyne.CanvasObject {
	a := d.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	partes := []fyne.CanvasObject{
		ui.NuevoParrafo(a.t("pdf.dialog.firmar.help"), tema.TextoXS, ui.Normal, "text-muted", v).A(ancho),
	}
	// En la app nativa, lo primero es el certificado del SISTEMA: el navegador no llega,
	// y es donde está el de casi todo el mundo.
	if a.p.Almacenes != nil {
		partes = append(partes, d.conSistema(ancho))
	}
	partes = append(partes,
		conRotulo(a.t("pdf.dialog.firmar.p12"), d.campoFichero(&d.pkcs12, a.t("pdf.dialog.firmar.p12"), "p12", "pfx"), notaDialogo(a.t("pdf.dialog.firmar.p12Help"), ancho)))
	if d.pkcs12.datos != nil {
		d.claveP12.Ancho = ancho
		partes = append(partes, conRotulo(a.t("pdf.dialog.firmar.p12Clave"), d.claveP12))
	} else {
		partes = append(partes,
			conRotulo(a.t("pdf.dialog.firmar.cert"), d.campoFichero(&d.certificado, a.t("pdf.dialog.firmar.cert"), "pem", "crt", "cer")),
			conRotulo(a.t("pdf.dialog.firmar.key"), d.campoFichero(&d.clavePrivada, a.t("pdf.dialog.firmar.key"), "pem", "key"), notaDialogo(a.t("pdf.dialog.firmar.keyHelp"), ancho)))
	}
	mitad := (ancho - tema.Espacio(2)) / 2
	if a.estrecha() {
		mitad = ancho // `max-sm:flex-col`: uno debajo de otro, como la web
	}
	d.motivo.Ancho, d.lugar.Ancho = mitad, mitad
	motivo := container.New(anchoFijo{ancho: mitad}, conRotulo(a.t("pdf.dialog.firmar.reason"), d.motivo))
	lugar := container.New(anchoFijo{ancho: mitad}, conRotulo(a.t("pdf.dialog.firmar.place"), d.lugar))
	var fila fyne.CanvasObject = container.New(ui.Fila{Hueco: tema.Espacio(2)}, motivo, lugar)
	if a.estrecha() {
		fila = container.New(ui.Columna{Hueco: tema.Espacio(2)}, motivo, lugar)
	}
	partes = append(partes, d.certificadoDelSistema(ancho), fila)
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, partes...)
}

// casilla es una `<label class="flex items-center gap-1.5 text-xs">` con su casilla.
func casillaDeFirma(rotulo string, marcada bool, alCambiar func(bool)) *casillaNativa {
	c := nuevaCasillaNativa(rotulo, marcada, alCambiar)
	c.esc, c.hueco, c.tinta = tema.TextoXS, tema.Espacio(1.5), "text"
	return c
}

// opciones es la fila `flex items-center gap-4` del recuadro: enseñarlo, en todas las
// páginas y en cuál. A lo estrecho (medido a 360) las tres `<label>` encogen como en CSS
// y sus rótulos se parten en renglones, con la casilla y el campo centrados en el alto.
func (d *dialogoFirmar) opciones(ancho float32) fyne.CanvasObject {
	a := d.a
	casillas := []*casillaNativa{casillaDeFirma(a.t("pdf.dialog.firmar.visible"), d.visible, func(v bool) { d.visible = v; a.rehacer() })}
	if d.visible {
		casillas = append(casillas, casillaDeFirma(a.t("pdf.dialog.firmar.allPages"), d.todas, func(v bool) { d.todas = v; a.rehacer() }))
	}
	e, hueco := tema.TextoXS, tema.Espacio(1.5)
	pagina := a.t("pdf.dialog.firmar.page")
	conPagina := d.visible && !d.todas
	// Las `<label>` no tienen relleno: su base interior es su ancho entero.
	maxc, minc := make([]float32, 0, 3), make([]float32, 0, 3)
	for _, c := range casillas {
		maxc = append(maxc, ladoCasillaNativa+c.hueco+ui.Medir(c.rotulo, e, ui.Normal).Width)
		minc = append(minc, ladoCasillaNativa+c.hueco+palabraMasLarga(c.rotulo, e, ui.Normal))
	}
	if conPagina {
		maxc = append(maxc, ui.Medir(pagina, e, ui.Normal).Width+hueco+d.pagina.Ancho)
		minc = append(minc, palabraMasLarga(pagina, e, ui.Normal)+hueco+d.pagina.Ancho)
	}
	gap := tema.Espacio(4)
	anchos := anchosFlex(maxc, minc, maxc, -1, ancho-gap*float32(len(maxc)-1))
	fila := make([]fyne.CanvasObject, 0, len(maxc))
	for i, c := range casillas {
		if anchos[i] < maxc[i] {
			c.ancho = anchos[i]
		}
		fila = append(fila, c)
	}
	if conPagina {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		texto := anchos[len(anchos)-1] - hueco - d.pagina.Ancho
		fila = append(fila, container.New(filaCentrada{hueco: hueco},
			ui.NuevoParrafo(pagina, e, ui.Normal, "text-muted", v).A(texto), d.pagina))
	}
	return container.New(filaCentrada{hueco: gap}, fila...)
}

// bloquearDespues es la casilla de certificar el documento, con su ayuda debajo
// (`ml-5`).
func (d *dialogoFirmar) bloquearDespues(ancho float32) fyne.CanvasObject {
	a := d.a
	sangria := tema.Espacio(5)
	return container.New(ui.Columna{Hueco: tema.Espacio(0.5)},
		container.New(alPrincipio{}, casillaDeFirma(a.t("pdf.dialog.firmar.lock"), d.bloquear, func(v bool) { d.bloquear = v })),
		container.New(ui.Margen{Izq: sangria}, notaDialogo(a.t("pdf.dialog.firmar.lockHelp"), ancho-sangria)))
}

// certificadoDelSistema es `CertificadoDelSistema`: qué hacer cuando el certificado no
// está en un fichero. Se sugiere la aplicación del país de quien está delante, y debajo
// quedan las demás.
func (d *dialogoFirmar) certificadoDelSistema(ancho float32) fyne.CanvasObject {
	a := d.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	px, py := tema.Espacio(2.5), tema.Espacio(2)
	dentro := ancho - 2*tema.Borde - 2*px
	resumen := nuevoDetalles(a.t("pdf.dialog.firmar.sistema.title"), d.sistemaAbierto, func(abierto bool) {
		d.sistemaAbierto = abierto
		a.rehacer()
	})
	resumen.peso = ui.Medio
	lineas := []fyne.CanvasObject{resumen}
	if d.sistemaAbierto {
		propia, hay := HerramientaDe(sistema.Idioma())
		lineas = append(lineas, container.New(ui.Margen{Arr: tema.Espacio(1.5)},
			ui.NuevoParrafo(a.t("pdf.dialog.firmar.sistema.help"), tema.TextoXS, ui.Normal, "text-muted", v).A(dentro)))
		if hay {
			l := enlaceExterno(propia.Nombre, propia.URL, tema.TextoXS, ui.Medio, "primary-text")
			l.Icono, l.TamIcono, l.Hueco = iconos.Enlace, 12, tema.Espacio(1)
			// El icono va DETRÁS del nombre en la web; aquí, delante (ver Accion).
			lineas = append(lineas, container.New(ui.Margen{Arr: tema.Espacio(2)}, container.New(alPrincipio{}, l)))
		}
		var otras []fyne.CanvasObject
		for _, h := range HerramientasNacionales {
			if hay && h.Pais == propia.Pais {
				continue
			}
			otras = append(otras, enlaceExterno(h.Pais+" · "+h.Nombre, h.URL, tema.Texto2XS, ui.Normal, "text-muted"))
		}
		lineas = append(lineas,
			container.New(ui.Margen{Arr: tema.Espacio(2)}, container.New(&filaQueEnvuelve{hueco: tema.Espacio(3), huecoY: tema.Espacio(1), ancho: dentro}, otras...)),
			container.New(ui.Margen{Arr: tema.Espacio(2)}, container.New(ui.Fila{Hueco: 0.26 * tema.Texto2XS.Tam},
				ui.NuevoRotulo(a.t("pdf.dialog.firmar.sistema.otros"), tema.Texto2XS, ui.Normal, "text-subtle"),
				enlaceExterno(a.t("pdf.dialog.firmar.sistema.lista"), ListaDeConfianzaUE, tema.Texto2XS, ui.Normal, "text-subtle"))))
	}
	caja := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD)
	cuerpo := container.New(ui.Margen{Izq: tema.Borde + px, Arr: tema.Borde + py, Der: tema.Borde + px, Abj: tema.Borde + py},
		container.New(ui.Columna{}, lineas...))
	return container.New(anchoFijo{ancho: ancho}, container.New(conCaja{caja}, append(caja.Objetos(), cuerpo)...))
}

// enlaceExterno es un `<a target="_blank" class="underline">`: se abre en el navegador.
func enlaceExterno(texto, direccion string, e tema.Escalon, p ui.Peso, tinta string) *Accion {
	a := NuevaAccion(texto, func() {
		abrirFuera(direccion)
	})
	a.Escalon, a.Peso, a.PX, a.PY, a.Subrayado = e, p, 0, 0, true
	a.Tinta, a.FondoEncima, a.Radio = tinta, "", tema.RadioSM
	if tinta == "text-muted" {
		a.TintaEncima = "text"
	}
	return a
}

// ---- Con la aplicación oficial ----

// nombreDeRuta es el nombre del fichero a partir de su ruta, con «.pdf».
func nombreDeRuta(ruta string) string {
	n := "documento.pdf"
	for _, t := range strings.Split(ruta, "/") {
		if t != "" {
			n = t
		}
	}
	if !strings.HasSuffix(strings.ToLower(n), ".pdf") {
		n += ".pdf"
	}
	return n
}

// oficial es `ConLaAplicacionOficial`: en la app de escritorio, «Firmar con mi
// aplicación» abre el documento en la aplicación del sistema; y siempre, los tres pasos
// a la vista y la zona para volver con el firmado.
func (d *dialogoFirmar) oficial(ancho float32) fyne.CanvasObject {
	a := d.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p := tema.Espacio(2.5)
	dentro := ancho - 2*tema.Borde - 2*p
	rotulo := "pdf.dialog.firmar.oficial.conMiApp"
	if d.subiendo {
		rotulo = "pdf.working"
	}
	conMiApp := botonPrimario(a.t(rotulo), tema.TextoXS, tema.Espacio(4), 0, d.firmarConLaApp)
	conMiApp.Peso, conMiApp.Ancho, conMiApp.Alto, conMiApp.Inerte, conMiApp.Opacidad = ui.SemiNegrita, dentro, tema.Espacio(9), d.subiendo, 0.6
	ayuda := "pdf.dialog.firmar.oficial.conMiAppHelp"
	if d.lanzada {
		ayuda = "pdf.dialog.firmar.oficial.lanzada"
	}
	caja := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD)
	nativo := container.New(conCaja{caja}, append(caja.Objetos(),
		container.New(ui.Margen{Izq: tema.Borde + p, Arr: tema.Borde + p, Der: tema.Borde + p, Abj: tema.Borde + p},
			container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, conMiApp, notaDialogo(a.t(ayuda), dentro))))...)

	descargar := NuevaAccion(a.t("pdf.dialog.firmar.oficial.paso1"), d.descargar)
	descargar.Peso, descargar.PX, descargar.PY, descargar.Subrayado, descargar.Tinta, descargar.FondoEncima, descargar.Radio =
		ui.Normal, 0, 0, true, "primary-text", "", tema.RadioSM
	paso2 := []fyne.CanvasObject{ui.NuevoRotulo(a.t("pdf.dialog.firmar.oficial.paso2"), tema.TextoXS, ui.Normal, "text-muted")}
	if propia, hay := HerramientaDe(sistema.Idioma()); hay {
		paso2 = append(paso2, enlaceExterno(propia.Nombre, propia.URL, tema.TextoXS, ui.Medio, "primary-text"))
	}
	pasos := container.New(ui.Columna{Hueco: tema.Espacio(2)},
		pasoNumerado(1, descargar),
		pasoNumerado(2, container.New(ui.Fila{Hueco: 0.26 * tema.TextoXS.Tam}, paso2...)),
		pasoNumerado(3, ui.NuevoRotulo(a.t("pdf.dialog.firmar.oficial.paso3"), tema.TextoXS, ui.Normal, "text-muted")))

	partes := []fyne.CanvasObject{nativo, pasos, d.zonaDevolver(ancho)}
	if ui.EsTactil() {
		// En la web solo lo ofrece la app de escritorio (`sytueFirmarNativo`): en un
		// móvil no hay aplicación de firma que señalar ni carpeta que vigilar.
		partes = partes[1:]
	}
	if d.errorOficial {
		partes = append(partes, ui.NuevoParrafo(a.t("pdf.dialog.firmar.oficial.error"), tema.Texto2XS, ui.Normal, "danger-text", v).A(ancho))
	}
	partes = append(partes, d.certificadoDelSistema(ancho))
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, partes...)
}

// pasoNumerado es un `<li>` de `list-decimal pl-4`: el número a la izquierda del texto.
func pasoNumerado(n int, contenido fyne.CanvasObject) fyne.CanvasObject {
	marca := strconv.Itoa(n) + ". "
	sangria := tema.Espacio(4)
	w := ui.Medir(marca, tema.TextoXS, ui.Normal).Width
	return container.New(ui.Fila{}, container.New(ui.Margen{Izq: sangria - w},
		ui.NuevoRotulo(marca, tema.TextoXS, ui.Normal, "text-muted")), contenido)
}

// zonaDevolver es la `<label>` discontinua para volver con el PDF firmado: lo que se
// elija se abre por el MISMO camino que «Abrir un archivo».
func (d *dialogoFirmar) zonaDevolver(ancho float32) fyne.CanvasObject {
	a := d.a
	texto := "pdf.dialog.firmar.oficial.devolver"
	if d.subiendo {
		texto = "pdf.working"
	}
	boton := NuevaAccion("", func() {
		if a.p.AlElegir != nil {
			a.cerrar()
			a.p.AlElegir()
		}
	})
	boton.Ancho, boton.Alto, boton.FondoEncima, boton.Radio = ancho, 2*tema.Borde+2*tema.Espacio(4)+18+tema.Espacio(1)+tema.TextoXS.Linea, "bg-subtle", tema.Forma("radius-control")
	borde := ui.NuevoBordeDiscontinuo(1, tema.RadioMD, "border") // `border border-dashed rounded-md`
	dibujo := iconos.Trazo(iconos.Descargar, 18, "text-muted", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	dibujo.Colocar(0, 0, 18)
	glifo := container.New(fijo{fyne.NewSize(18, 18)}, dibujo.Objeto())
	contenido := container.New(ui.Margen{Arr: tema.Borde + tema.Espacio(4)}, container.New(ui.Columna{Hueco: tema.Espacio(1)},
		container.NewCenter(glifo), container.NewCenter(ui.NuevoRotulo(a.t(texto), tema.TextoXS, ui.Normal, "text-muted"))))
	return container.New(anchoFijo{ancho: ancho}, container.NewStack(boton, borde, contenido))
}

// descargar deja el documento en descargas: es el primer paso.
func (d *dialogoFirmar) descargar() {
	if _, err := d.guardarDocumento(); err != nil {
		d.errorOficial = true
		d.a.rehacer()
	}
}

// guardarDocumento escribe el PDF tal como está en descargas y devuelve su ruta.
func (d *dialogoFirmar) guardarDocumento() (string, error) {
	doc := d.a.p.doc
	datos, _ := doc.paraElMotor()
	carpeta, err := ficheros.Carpeta()
	if err != nil {
		return "", err
	}
	return ficheros.Guardar(carpeta, nombreDeRuta(doc.Ruta()), bytes.NewReader(datos.Datos))
}

// firmarConLaApp es el puente de la app de escritorio (`sytueFirmarNativo`): el
// documento se abre con la aplicación de firma de la persona —la PRIMERA vez se le
// pregunta cuál es, con el selector del sistema, y se recuerda— y, cuando esa
// aplicación guarda el firmado junto al original, vuelve solo a CertiPDF.
//
// El documento va a una carpeta temporal nueva y exclusiva: es donde casi todas las
// aplicaciones de firma guardan el firmado (junto al original, con un sufijo), y así lo
// único nuevo que aparece ahí es él.
func (d *dialogoFirmar) firmarConLaApp() {
	p := d.a.p
	doc := p.doc
	if doc == nil {
		return
	}
	d.subiendo, d.errorOficial = true, false
	d.a.rehacer()
	titulo := d.a.t("pdf.dialog.firmar.oficial.conMiApp")
	prefs := fyne.CurrentApp().Preferences()
	app := prefs.String(prefAppDeFirma)
	datos, _ := doc.paraElMotor()
	nombre := nombreDeRuta(doc.Ruta())
	// Si alguien había pedido firmar este documento, el firmado que vuelva solo (y solo
	// ese: es el único fichero nuevo de su carpeta) se le entrega al abrirse.
	pend := p.peticion
	go func() {
		lanzada, err := func() (bool, error) {
			if _, err := os.Stat(app); app == "" || err != nil {
				elegida, err := ficheros.ElegirAplicacion(titulo)
				if err != nil || elegida == "" {
					return false, err // cancelar no es un fallo
				}
				app = elegida
				fyne.Do(func() { prefs.SetString(prefAppDeFirma, elegida) })
			}
			carpeta, err := os.MkdirTemp("", "sytue-firma-")
			if err != nil {
				return false, err
			}
			original := filepath.Join(carpeta, nombre)
			if err := os.WriteFile(original, datos.Datos, 0o600); err != nil {
				_ = os.RemoveAll(carpeta)
				return false, err
			}
			if err := ficheros.AbrirCon(app, original); err != nil {
				_ = os.RemoveAll(carpeta)
				return false, err
			}
			go vigilarFirmado(carpeta, original, func(firmado string) {
				fyne.Do(func() {
					if p.AlAbrirLocal != nil {
						p.porEntregar = pend
						p.AlAbrirLocal(firmado)
					}
				})
			})
			return true, nil
		}()
		fyne.Do(func() {
			d.subiendo = false
			d.lanzada, d.errorOficial = lanzada, err != nil
			d.a.rehacer()
		})
	}()
}

// prefAppDeFirma es dónde se recuerda la aplicación de firma elegida, en las
// preferencias de Fyne de esta app.
const prefAppDeFirma = "appDeFirma"

// Cuánto se espera al firmado y cada cuánto se mira. Diez minutos: firmar con una
// tarjeta es teclear un PIN y a veces ir a buscarla a un cajón; más, y el vigilante es
// una rutina y una carpeta temporal que nadie va a usar.
var (
	esperaFirmado = 10 * time.Minute
	pasoFirmado   = 2 * time.Second
)

// vigilarFirmado espera a que aparezca en `carpeta` un PDF que no sea el original y,
// cuando deja de crecer, se lo entrega a `abrir`. Se rinde pasado `esperaFirmado` y
// borra la carpeta.
func vigilarFirmado(carpeta, original string, abrir func(string)) {
	limite := time.Now().Add(esperaFirmado)
	tamanos := map[string]int64{}
	for time.Now().Before(limite) {
		time.Sleep(pasoFirmado)
		entradas, err := os.ReadDir(carpeta)
		if err != nil {
			return
		}
		for _, e := range entradas {
			ruta := filepath.Join(carpeta, e.Name())
			if e.IsDir() || ruta == original || !strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
				continue
			}
			info, err := e.Info()
			if err != nil || info.Size() == 0 {
				continue
			}
			// A medio escribir no se abre: un PDF truncado se leería como corrupto. Se
			// da por acabado cuando mide lo mismo en dos vueltas seguidas.
			if antes, visto := tamanos[ruta]; !visto || antes != info.Size() {
				tamanos[ruta] = info.Size()
				continue
			}
			abrir(ruta)
			return
		}
	}
	_ = os.RemoveAll(carpeta)
}

// ---- firmar ----

// trazosAPagina lleva los trazos del lienzo a la página, con el eje vertical al derecho:
// en un PDF crece hacia ARRIBA y en el lienzo hacia abajo.
func trazosAPagina(trazos [][][2]float64) [][][2]float64 {
	x0, y0, x1, y1 := rectFirma[0], rectFirma[1], rectFirma[2], rectFirma[3]
	r := make([][][2]float64, len(trazos))
	for i, t := range trazos {
		r[i] = make([][2]float64, len(t))
		for j, p := range t {
			r[i][j] = [2]float64{x0 + p[0]/anchoLienzo*(x1-x0), y1 - p[1]/altoLienzo*(y1-y0)}
		}
	}
	return r
}

// cambio es lo que manda «Aplicar», según el modo.
func (d *dialogoFirmar) cambio() api.PDFCambio {
	rect := rectFirma
	if !d.visible {
		rect = [4]float64{}
	}
	pagina, _ := strconv.Atoi(strings.TrimSpace(d.pagina.Texto()))
	sufijo := d.a.t("pdf.suffix.signed")
	if d.modo == "certificado" {
		c := api.PDFCambio{Op: "firmar-certificado", Sufijo: sufijo, Datos: map[string]any{
			"Motivo": d.motivo.Texto(), "Lugar": d.lugar.Texto(), "Pagina": pagina - 1, "Rect": rect, "Bloquear": d.bloquear,
		}}
		if d.pkcs12.datos != nil {
			c.Partes = []api.PDFParte{{Campo: "pkcs12", Nombre: "certificado.p12", Datos: d.pkcs12.datos}}
			c.Campos = map[string]string{"clave_p12": d.claveP12.Texto()}
		} else {
			c.Partes = []api.PDFParte{
				{Campo: "certificado", Nombre: "certificado.pem", Datos: d.certificado.datos},
				{Campo: "clave_priv", Nombre: "clave.pem", Datos: d.clavePrivada.datos},
			}
		}
		return c
	}
	datos := map[string]any{
		"pagina": pagina - 1, "rect": rect, "color": [3]float64{0.12, 0.16, 0.22}, "grosor": 1.6, "todas_las_paginas": d.todas,
	}
	c := api.PDFCambio{Op: "firmar", Datos: datos, Sufijo: sufijo}
	switch d.modo {
	case "mano":
		datos["trazos"] = trazosAPagina(d.lienzo.trazos)
	case "imagen":
		c.Partes = []api.PDFParte{{Campo: "imagen", Nombre: "firma.png", Datos: d.imagen.datos}}
	}
	return c
}

func (d *dialogoFirmar) firmar() {
	if d.listo() {
		d.a.aplicar(d.cambio())
	}
}

// ---- El lienzo de firma ----

// lienzoFirma es `LienzoDeFirma`: 480×148 por dentro, estirado a lo ancho, con los
// trazos en coordenadas internas. Se firma arrastrando: ratón, dedo o lápiz.
type lienzoFirma struct {
	widget.BaseWidget
	trazos    [][][2]float64
	dibujando bool
	ancho     float32
	alCambiar func()
}

var (
	_ fyne.Draggable     = (*lienzoFirma)(nil)
	_ desktop.Cursorable = (*lienzoFirma)(nil)
)

func nuevoLienzoFirma(alCambiar func()) *lienzoFirma {
	l := &lienzoFirma{alCambiar: alCambiar}
	l.ExtendBaseWidget(l)
	return l
}

// MinSize es `w-full` con el alto de la proporción del lienzo, más su borde.
func (l *lienzoFirma) MinSize() fyne.Size {
	return fyne.NewSize(l.ancho, (l.ancho-2*tema.Borde)*altoLienzo/anchoLienzo+2*tema.Borde)
}

// interno lleva un punto de la caja (la de BORDE, como `getBoundingClientRect`) a la
// resolución interna.
func (l *lienzoFirma) interno(p fyne.Position) [2]float64 {
	s := l.Size()
	return [2]float64{float64(p.X/max(s.Width, 1)) * anchoLienzo, float64(p.Y/max(s.Height, 1)) * altoLienzo}
}

// Dragged empieza un trazo o lo alarga.
func (l *lienzoFirma) Dragged(e *fyne.DragEvent) {
	if !l.dibujando {
		l.dibujando = true
		l.trazos = append(l.trazos, [][2]float64{l.interno(e.Position.Subtract(e.Dragged))})
	}
	i := len(l.trazos) - 1
	l.trazos[i] = append(l.trazos[i], l.interno(e.Position))
	l.Refresh()
}

// DragEnd cierra el trazo; lo que se dibujó ya puede firmarse.
func (l *lienzoFirma) DragEnd() {
	l.dibujando = false
	if l.alCambiar != nil {
		l.alCambiar()
	}
}

// Cursor es el `cursor-crosshair`.
func (l *lienzoFirma) Cursor() desktop.Cursor { return desktop.CrosshairCursor }

func (l *lienzoFirma) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &lienzoFirmaRenderer{l: l, caja: ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)}
	r.raster = canvas.NewRaster(r.pintar)
	r.raster.ScaleMode = canvas.ImageScalePixels
	return r
}

type lienzoFirmaRenderer struct {
	l      *lienzoFirma
	caja   *ui.Caja
	raster *canvas.Raster
}

func (r *lienzoFirmaRenderer) MinSize() fyne.Size { return r.l.MinSize() }
func (r *lienzoFirmaRenderer) Destroy()           {}
func (r *lienzoFirmaRenderer) Objects() []fyne.CanvasObject {
	return append(r.caja.Objetos(), r.raster)
}

func (r *lienzoFirmaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.raster.Move(fyne.NewPos(tema.Borde, tema.Borde))
	r.raster.Resize(fyne.NewSize(t.Width-2*tema.Borde, t.Height-2*tema.Borde))
}

func (r *lienzoFirmaRenderer) Refresh() {
	r.Layout(r.l.Size())
	r.raster.Refresh()
}

// tintaFirma es el `strokeStyle = '#1f2937'` del lienzo: el color de la tinta, que no es
// un token porque es el de la firma, no el de la interfaz.
var tintaFirma = color.NRGBA{R: 0x1f, G: 0x29, B: 0x37, A: 255}

// pintar dibuja los trazos a la resolución del dispositivo: `lineWidth 2,5` con puntas y
// uniones redondas, escalado con el lienzo.
func (r *lienzoFirmaRenderer) pintar(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	sx, sy := float64(w)/anchoLienzo, float64(h)/altoLienzo
	radio := 2.5 * sx / 2
	for _, t := range r.l.trazos {
		for i := 1; i < len(t); i++ {
			trazoRedondo(img, t[i-1][0]*sx, t[i-1][1]*sy, t[i][0]*sx, t[i][1]*sy, radio)
		}
	}
	return img
}

// trazoRedondo pinta un segmento de grosor 2·radio con los extremos redondos, con
// cobertura por distancia (antialias).
func trazoRedondo(img *image.NRGBA, x0, y0, x1, y1, radio float64) {
	b := img.Bounds()
	minX, maxX := int(math.Floor(math.Min(x0, x1)-radio-1)), int(math.Ceil(math.Max(x0, x1)+radio+1))
	minY, maxY := int(math.Floor(math.Min(y0, y1)-radio-1)), int(math.Ceil(math.Max(y0, y1)+radio+1))
	dx, dy := x1-x0, y1-y0
	largo2 := dx*dx + dy*dy
	for y := max(minY, b.Min.Y); y < min(maxY, b.Max.Y); y++ {
		for x := max(minX, b.Min.X); x < min(maxX, b.Max.X); x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			t := 0.0
			if largo2 > 0 {
				t = math.Max(0, math.Min(1, ((px-x0)*dx+(py-y0)*dy)/largo2))
			}
			ex, ey := px-(x0+t*dx), py-(y0+t*dy)
			cobertura := math.Max(0, math.Min(1, radio+0.5-math.Hypot(ex, ey)))
			if cobertura == 0 {
				continue
			}
			a := uint8(cobertura * 255)
			if a > img.NRGBAAt(x, y).A {
				c := tintaFirma
				c.A = a
				img.SetNRGBA(x, y, c)
			}
		}
	}
}
