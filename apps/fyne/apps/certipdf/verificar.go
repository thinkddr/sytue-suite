// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Las etiquetas JSON de este fichero son las del informe que contesta el motor
// (`/pdf/verify`, `domain.FirmaVerificada`), no las de la casa: ver el aviso de api.go.
//
//nolint:tagliatelle // las etiquetas son las del motor, no nuestras
package certipdf

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

/*
La VERIFICACIÓN de las firmas del panel «Firmas» (`Firmas` y `FirmaVerificadaFicha` de
`pdf/Paneles.tsx`, y `pdf/verificacion.ts`).

TODO el juicio —integridad, cambios posteriores, sello de tiempo, cadena contra las
anclas del despliegue— es del motor (`services/docs/internal/domain/pdfverificar.go`).
Aquí solo se traduce a claves del catálogo y a un tono, como en la web, para que una
firma que falla no se pinte nunca igual que una que no se sabe comprobar.

Lo que NO hace el motor, y la pantalla dice siempre debajo del informe (`pdf.verify.
revocation`): consultar si un certificado ha sido revocado (OCSP, CRL).

Verificar cuesta —cada firma resume el fichero entero y compara las revisiones—, así que
es un botón y no algo que corra al abrir el panel.
*/

// Veredicto es el de una firma, tal y como lo da el motor (`Veredicto*` de dominio).
type Veredicto string

// Los cuatro veredictos.
const (
	VeredictoValida        Veredicto = "valida"
	VeredictoIndeterminada Veredicto = "indeterminada"
	VeredictoModificada    Veredicto = "modificada"
	VeredictoInvalida      Veredicto = "invalida"
)

// CertificadoLeido es un certificado del informe.
type CertificadoLeido struct {
	Sujeto       string `json:"sujeto"`
	Organizacion string `json:"organizacion,omitempty"`
	Emisor       string `json:"emisor"`
	Serie        string `json:"serie"`
	Desde        string `json:"desde"`
	Hasta        string `json:"hasta"`
	Cualificado  bool   `json:"cualificado,omitempty"`
	// DispositivoSeguro es que la clave vive en un dispositivo seguro de creación.
	DispositivoSeguro bool `json:"dispositivo_seguro,omitempty"`
}

// SelloLeido es el sello de tiempo (RFC 3161) de una firma.
type SelloLeido struct {
	Fecha     string `json:"fecha,omitempty"`
	Autoridad string `json:"autoridad,omitempty"`
	Integro   bool   `json:"integro"`
	// Confianza es «confiable», «no_confiable» o «sin_anclas».
	Confianza string `json:"confianza"`
}

// CambiosPosteriores es lo que se añadió al fichero DESPUÉS de firmar.
type CambiosPosteriores struct {
	BytesFirmados int `json:"bytes_firmados"`
	BytesTotales  int `json:"bytes_totales"`
	Revisiones    int `json:"revisiones"`
	// Alcance es «ninguno», «permitidos» (otras firmas, datos de validación) u «otros».
	Alcance string `json:"alcance"`
	Paginas []int  `json:"paginas,omitempty"`
	Objetos int    `json:"objetos,omitempty"`
}

// FirmaVerificada es una firma con su veredicto y lo que se comprobó. Lleva lo que la
// firma declara ([FirmaLeida]) y, además, lo que el motor verificó.
type FirmaVerificada struct {
	FirmaLeida
	Tipo      string             `json:"tipo"`
	Veredicto Veredicto          `json:"veredicto"`
	Motivos   []string           `json:"motivos,omitempty"`
	Integra   bool               `json:"integra"`
	Firmante  *CertificadoLeido  `json:"firmante,omitempty"`
	Cadena    []CertificadoLeido `json:"cadena,omitempty"`
	// Confianza es la de la cadena contra las anclas del despliegue.
	Confianza      string             `json:"confianza"`
	HoraReferencia string             `json:"hora_referencia,omitempty"`
	Sello          *SelloLeido        `json:"sello,omitempty"`
	Cambios        CambiosPosteriores `json:"cambios"`
	Certificacion  int                `json:"certificacion,omitempty"`
}

// InformeVerificacion es la respuesta de `/pdf/verify`.
type InformeVerificacion struct {
	Firmas []FirmaVerificada `json:"firmas"`
	// Anclas es falso cuando el despliegue no tiene en quién confiar: la identidad no
	// se juzga, y la pantalla lo dice.
	Anclas bool `json:"anclas"`
}

// Verificar es `op: verificar`: el informe de cada firma del documento abierto.
func (d *Documento) Verificar(ctx context.Context) (InformeVerificacion, error) {
	var r InformeVerificacion
	err := d.Leer(ctx, "/pdf/verify", nil, &r)
	return r, err
}

// motivosDelCatalogo son las claves de `domain.Motivo*`, por su código estable.
var motivosDelCatalogo = map[string]string{
	"rango":              "pdf.verify.reason.rango",
	"formato":            "pdf.verify.reason.formato",
	"integridad":         "pdf.verify.reason.integridad",
	"certificacion":      "pdf.verify.reason.certificacion",
	"cambios":            "pdf.verify.reason.cambios",
	"sin_anclas":         "pdf.verify.reason.sin_anclas",
	"no_confiable":       "pdf.verify.reason.no_confiable",
	"caducado":           "pdf.verify.reason.caducado",
	"sello_invalido":     "pdf.verify.reason.sello_invalido",
	"sello_no_confiable": "pdf.verify.reason.sello_no_confiable",
	"limite":             "pdf.verify.reason.limite",
	"serie_negativa":     "pdf.verify.reason.serie_negativa",
	"sin_atributos":      "pdf.verify.reason.sin_atributos",
}

// claveDeMotivo es la clave del texto de un motivo; uno que el motor añada y aquí no se
// conozca cae en el genérico en vez de enseñar un código en bruto.
func claveDeMotivo(m string) string {
	if c, ok := motivosDelCatalogo[m]; ok {
		return c
	}
	return motivosDelCatalogo["formato"]
}

// claveDeVeredicto es la clave del rótulo de un veredicto; uno desconocido cae en
// «sin determinar», que ni acusa ni da el visto bueno.
func claveDeVeredicto(v Veredicto) string {
	switch v {
	case VeredictoValida, VeredictoModificada, VeredictoInvalida:
		return "pdf.verify.verdict." + string(v)
	case VeredictoIndeterminada:
	}
	return "pdf.verify.verdict.indeterminada"
}

// tintaDeVeredicto es el token de la tinta del veredicto. Solo «válida» es verde;
// «indeterminada» no es un fallo ni un visto bueno, así que se queda en la tinta normal
// (`tonoDeVeredicto` de la web).
func tintaDeVeredicto(v Veredicto) string {
	switch v {
	case VeredictoValida:
		return "success-text"
	case VeredictoModificada, VeredictoInvalida:
		return "danger-text"
	case VeredictoIndeterminada:
	}
	return "text"
}

// rangoDePaginas es «3, 5-7, 9»: las páginas cambiadas sin una lista interminable.
func rangoDePaginas(paginas []int) string {
	orden := slices.Compact(slices.Sorted(slices.Values(paginas)))
	var trozos []string
	for i := 0; i < len(orden); {
		j := i
		for j+1 < len(orden) && orden[j+1] == orden[j]+1 {
			j++
		}
		if j > i {
			trozos = append(trozos, strconv.Itoa(orden[i])+"-"+strconv.Itoa(orden[j]))
		} else {
			trozos = append(trozos, strconv.Itoa(orden[i]))
		}
		i = j + 1
	}
	return strings.Join(trozos, ", ")
}

// fechaCorta es la fecha ISO del motor como fecha y hora legibles (`dateStyle: 'medium',
// timeStyle: 'short'`, en la zona de quien mira); si no se puede leer, se deja tal cual.
func fechaCorta(iso string, l i18n.Locale) string {
	if iso == "" {
		return ""
	}
	var t time.Time
	var err error
	for _, forma := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err = time.Parse(forma, iso); err == nil {
			break
		}
	}
	if err != nil {
		return iso
	}
	if !i18n.IsLocale(string(l)) {
		l = i18n.Default
	}
	zona := zonaDeFechas
	if zona == nil {
		zona = time.Local
	}
	return i18n.FormatDateTime(l, t.In(zona))
}

// zonaDeFechas fija la zona de fechaCorta en las pruebas; nil es la del equipo. Antes
// la prueba reasignaba time.Local, y el detector de carreras lo pillaba contra el
// time.Now() de las conexiones HTTP que otras pruebas dejaban vivas.
var zonaDeFechas *time.Location

// ---- El panel ----

// estadoVerificacion es el `estado` del componente de la web.
type estadoVerificacion uint8

const (
	verificacionLista estadoVerificacion = iota
	verificacionTrabajando
	verificacionFallida
)

// panelFirmas es el estado del panel «Firmas»: sobrevive a cambiar de pestaña, como el
// componente montado de la web.
type panelFirmas struct {
	l *PanelLateral

	informe *InformeVerificacion
	estado  estadoVerificacion
	// version cambia cuando el documento deja de ser el que se verificó: un informe que
	// llegue de antes no vale, y uno ya recibido se tira.
	version int

	vista *fyne.Container
}

func (l *PanelLateral) firmas() fyne.CanvasObject {
	if l.firmasP == nil {
		l.firmasP = &panelFirmas{l: l, vista: container.New(ui.Columna{Hueco: tema.Espacio(2)})}
	}
	l.firmasP.rehacer()
	return l.firmasP.vista
}

// olvidar tira el informe: el documento ha cambiado y el veredicto era de otro fichero.
// (La web lo conserva mientras no cambie la ruta; aquí un cambio en el sitio es otro
// fichero, y enseñar «Válida» sobre bytes que ya no son los verificados es lo único que
// esta pantalla no puede permitirse.)
func (f *panelFirmas) olvidar() {
	f.informe, f.estado = nil, verificacionLista
	f.version++
}

func (f *panelFirmas) t(clave string) string { return f.l.t(clave, nil) }

// verificar manda el documento al motor y, al volver, enseña el informe.
func (f *panelFirmas) verificar() {
	p := f.l.pant
	if p == nil || p.doc == nil || f.estado == verificacionTrabajando {
		return
	}
	f.estado = verificacionTrabajando
	f.rehacer()
	doc, version := p.doc, f.version
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		informe, err := doc.Verificar(ctx)
		fyne.Do(func() {
			defer p.avisarRed()
			if p.doc != doc || f.version != version {
				return
			}
			if err != nil {
				f.estado = verificacionFallida
			} else {
				f.informe, f.estado = &informe, verificacionLista
			}
			f.rehacer()
		})
	}()
}

// rehacer recompone el panel con su estado; el contenedor es el MISMO objeto, así que
// el desplazamiento no vuelve arriba al llegar el informe o al ponerse a verificar.
func (f *panelFirmas) rehacer() {
	if f.informe != nil && len(f.informe.Firmas) > 0 {
		f.vista.Objects = f.vistaDelInforme()
	} else {
		f.vista.Objects = f.vistaDeLoDeclarado()
	}
	f.vista.Refresh()
}

// pie es `text-text-subtle border-border text-2xs mt-1 border-t pt-2`: el descargo, con
// su línea encima.
func (f *panelFirmas) pie(texto string) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	nota := ui.NuevoParrafo(texto, tema.Texto2XS, ui.Normal, "text-subtle", v).A(anchoUtil())
	return container.New(ui.Margen{Arr: tema.Espacio(1)},
		container.New(conLineaArriba{sep: tema.Espacio(2)}, canvas.NewRectangle(tema.Color("border", v)), nota))
}

// vistaDeLoDeclarado es lo que la firma DECLARA —quién, por qué, cuándo y si cubre el
// fichero entero—, con el botón que lo verifica y el descargo de lo que esto no
// comprueba. Sin firmas, solo dice que no las hay.
func (f *panelFirmas) vistaDeLoDeclarado() []fyne.CanvasObject {
	l := f.l
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	dentro := anchoUtil() - 2*tema.Borde - 2*tema.Espacio(2)
	if len(l.c.Firmas) == 0 {
		return []fyne.CanvasObject{parrafoVacio(l.t("pdf.signatures.empty", nil), anchoUtil())}
	}
	var partes []fyne.CanvasObject
	for _, fi := range l.c.Firmas {
		nombre := fi.Nombre
		if nombre == "" {
			nombre = l.t("pdf.signatures.unnamed", nil)
		}
		lineas := []fyne.CanvasObject{ui.NuevoParrafo(nombre, tema.TextoXS, ui.Medio, "text", v).A(dentro)}
		if fi.Motivo != "" {
			lineas = append(lineas, ui.NuevoParrafo(fi.Motivo, tema.Texto2XS, ui.Normal, "text-muted", v).A(dentro))
		}
		if fi.Fecha != "" {
			lineas = append(lineas, ui.NuevoParrafo(fi.Fecha, tema.Texto2XS, ui.Normal, "text-subtle", v).A(dentro))
		}
		// Lo que de verdad hay que mirar: si cubre el fichero ENTERO. Si no, alguien
		// añadió algo después de firmar, que es el truco para colar una cláusula.
		rotulo, tinta := "pdf.signatures.partial", "danger-text"
		if fi.CubreTodo {
			rotulo, tinta = "pdf.signatures.covers", "success-text"
		}
		veredicto := ui.NuevoParrafo(l.t(rotulo, nil), tema.Texto2XS, ui.Normal, tinta, v).A(dentro)
		lineas = append(lineas, container.New(ui.Margen{Arr: tema.Espacio(1)}, veredicto))
		partes = append(partes, nuevoRecuadro(container.New(ui.Columna{}, lineas...)))
	}

	rotulo := f.t("pdf.verify.run")
	if f.estado == verificacionTrabajando {
		rotulo = f.t("pdf.verify.running")
	}
	boton := ui.NuevoBoton(rotulo, f.verificar).Con(ui.Primario, ui.TallaSM)
	boton.Inerte = f.estado == verificacionTrabajando
	partes = append(partes, boton)
	if f.estado == verificacionFallida {
		// `role="alert"`: el párrafo se anuncia por su texto (ui.Parrafo es texto accesible).
		partes = append(partes, ui.NuevoParrafo(f.t("pdf.verify.error"), tema.TextoXS, ui.Normal, "danger-text", v).A(anchoUtil()))
	}
	return append(partes, f.pie(f.t("pdf.signatures.disclaimer")))
}

// vistaDelInforme es una ficha por firma verificada, la nota de que no hay en quién
// confiar si es el caso, y SIEMPRE la de la revocación.
func (f *panelFirmas) vistaDelInforme() []fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	partes := make([]fyne.CanvasObject, 0, len(f.informe.Firmas)+2)
	for _, fi := range f.informe.Firmas {
		partes = append(partes, nuevoRecuadro(f.ficha(fi)))
	}
	if !f.informe.Anclas {
		partes = append(partes, ui.NuevoParrafo(f.t("pdf.verify.noAnchors"), tema.Texto2XS, ui.Normal, "text-muted", v).A(anchoUtil()))
	}
	return append(partes, f.pie(f.t("pdf.verify.revocation")))
}

// ficha es `FirmaVerificadaFicha`: el veredicto, en TEXTO y no solo en color, encabeza la
// ficha; después, el nombre, el porqué, el certificado, el sello y los cambios.
func (f *panelFirmas) ficha(fi FirmaVerificada) fyne.CanvasObject {
	l := f.l
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	dentro := anchoUtil() - 2*tema.Borde - 2*tema.Espacio(2)
	loc := l.idioma()
	linea := func(texto, tinta string, arriba float32) fyne.CanvasObject {
		p := ui.NuevoParrafo(texto, tema.Texto2XS, ui.Normal, tinta, v).A(dentro)
		if arriba == 0 {
			return p
		}
		return container.New(ui.Margen{Arr: arriba}, p)
	}

	nombre := fi.Nombre
	if nombre == "" && fi.Firmante != nil {
		nombre = fi.Firmante.Sujeto
	}
	if nombre == "" {
		nombre = l.t("pdf.signatures.unnamed", nil)
	}
	lineas := []fyne.CanvasObject{
		ui.NuevoParrafo(l.t(claveDeVeredicto(fi.Veredicto), nil), tema.TextoXS, ui.Medio, tintaDeVeredicto(fi.Veredicto), v).A(dentro),
		ui.NuevoParrafo(nombre, tema.TextoXS, ui.Medio, "text", v).A(dentro),
	}
	for _, m := range fi.Motivos {
		lineas = append(lineas, linea(l.t(claveDeMotivo(m), nil), "text-muted", tema.Espacio(0.5)))
	}
	if c := fi.Firmante; c != nil {
		lineas = append(lineas,
			linea(l.t("pdf.verify.certificate", i18n.Vars{"subject": c.Sujeto, "issuer": c.Emisor}), "text-muted", tema.Espacio(1)),
			linea(l.t("pdf.verify.validity", i18n.Vars{"from": fechaCorta(c.Desde, loc), "to": fechaCorta(c.Hasta, loc)}), "text-subtle", 0))
	}
	if s := fi.Sello; s != nil && s.Integro {
		lineas = append(lineas, linea(l.t("pdf.verify.timestamp",
			i18n.Vars{"authority": s.Autoridad, "date": fechaCorta(s.Fecha, loc)}), "text-subtle", 0))
	} else {
		lineas = append(lineas, linea(l.t("pdf.verify.noTimestamp", nil), "text-subtle", 0))
	}
	if fi.Cambios.Alcance == "permitidos" {
		lineas = append(lineas, linea(l.t("pdf.verify.changesAllowed", nil), "text-subtle", 0))
	}
	if len(fi.Cambios.Paginas) > 0 {
		lineas = append(lineas, linea(l.t("pdf.verify.changesPages",
			i18n.Vars{"pages": rangoDePaginas(fi.Cambios.Paginas)}), "danger-text", 0))
	}
	if fi.Cambios.Objetos > 0 {
		lineas = append(lineas, linea(l.t("pdf.verify.changesObjects", nil), "danger-text", 0))
	}
	return container.New(ui.Columna{}, lineas...)
}
