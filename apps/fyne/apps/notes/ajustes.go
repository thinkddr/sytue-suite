// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"encoding/json"
	"image/color"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Paridad web: apps/frontend/apps/notes/src/components/NotebookSettings.tsx.

// LOS AJUSTES DE UN CUADERNO, en UNA pantalla: nombre, icono, color, disposición, quién
// puede verlo, si entra en la búsqueda de la suite y si va cifrado. Es también el
// diálogo de «nuevo cuaderno»: la web usa el mismo con `notebook = null`.
//
// Medidas, de la hoja con el rem de 14 (diálogo `md`, 560 de ancho máximo):
//
//	cuerpo            `grid gap-4`                                        14
//	icono + nombre    `grid-cols-[5rem_1fr] gap-3`                        70 | resto, 10,5
//	leyendas          `text-base font-medium mb-1.5`                      5,25 debajo
//	muestras          `h-7 w-7 rounded-full`, `gap-1.5`                    24,5, 5,25
//	disposiciones     `grid-cols-3 gap-2`, cada una `p-2.5 gap-1 border`   7, 10,5, 3,5
//	visibilidades     `grid gap-1.5`, cada una `p-2.5 gap-2.5 border`      5,25, 10,5, 8,75
//	caja del índice   `rounded-md border p-3 grid gap-3`                   10,5, 10,5
const (
	HuecoAjustes      = 4 * 0.25 * tema.Rem
	AnchoIconoAjustes = 5 * tema.Rem
	HuecoCamposAjust  = 3 * 0.25 * tema.Rem
	HuecoLeyenda      = 1.5 * 0.25 * tema.Rem
	LadoMuestra       = 7 * 0.25 * tema.Rem
	HuecoMuestras     = 1.5 * 0.25 * tema.Rem
	HuecoDisposicion  = 2 * 0.25 * tema.Rem
	RellenoOpcion     = 2.5 * 0.25 * tema.Rem
	HuecoVisibilidad  = 1.5 * 0.25 * tema.Rem
	RellenoCajaIndice = 3 * 0.25 * tema.Rem
	// SangriaAyudaCasilla y SangriaAyudaInterruptor son el `pl-6` y el `pl-12` que
	// ponen la explicación debajo del RÓTULO de la casilla y del interruptor.
	SangriaAyudaCasilla     = 6 * 0.25 * tema.Rem
	SangriaAyudaInterruptor = 12 * 0.25 * tema.Rem
)

// ColoresCuaderno es la paleta de NotebookSettings.tsx: doce tonos medidos para llevar
// texto blanco encima. El primero es el de un cuaderno nuevo.
var ColoresCuaderno = []string{
	"#86198f", "#be123c", "#c2410c", "#a16207", "#4d7c0f", "#146a47",
	"#0f766e", "#0e7490", "#1c58d9", "#4c1d95", "#7e22ce", "#3a4a5e",
}

// Disposiciones son las cuatro con su glifo, EL ÁRBOL PRIMERO: es la de serie y las
// demás son «funciones añadidas», como pide el producto.
var Disposiciones = []struct {
	D     Disposicion
	Glifo string
}{{VistaArbol, ArbolD}, {Libre, LibreD}, {Columnas, ColumnasD}, {Lista, ListaD}}

// Visibilidades son las cuatro con su consecuencia escrita al lado: «privado» y
// «personal» suenan igual, y la diferencia —una está en la búsqueda y la otra no— es
// justo lo que hay que saber antes de elegir.
var Visibilidades = []struct {
	Clave, Glifo string
}{{"private", CuadernoD}, {"personal", NotaD}, {"organization", EnlaceD}, {"public", EnlaceD}}

// formAjustes es lo que el diálogo va cambiando antes de guardar.
type formAjustes struct {
	color, disposicion, visibilidad string
	indexable, cifrado              bool
	// crypto son los parámetros de un cifrado recién encendido; nil, los que ya tenía.
	crypto json.RawMessage
}

// defectoIndexable es el `v !== 'private' && !encrypted` de la web: cambiar de
// visibilidad devuelve la casilla del índice a su valor por defecto.
func defectoIndexable(visibilidad string, cifrado bool) bool {
	return visibilidad != "private" && !cifrado
}

// entradaAjustes arma el cuerpo que se manda: parte del cuaderno ENTERO (su cifrado, su
// sala) y cambia solo lo que el diálogo edita. `padre` es el del cuaderno nuevo.
func entradaAjustes(orig *Cuaderno, padre, titulo, emoji string, f formAjustes) api.NotebookInput {
	var in api.NotebookInput
	if orig != nil {
		in = orig.Crudo.Input()
	} else if padre != "" {
		in.ParentID = ptr(padre)
	}
	in.Title, in.Emoji, in.Color = strings.TrimSpace(titulo), emoji, f.color
	in.Layout, in.Visibility = f.disposicion, f.visibilidad
	in.Encrypted = f.cifrado
	if f.cifrado && f.crypto != nil {
		in.Crypto = f.crypto
	}
	// Cifrado y en el índice a la vez no existe: el índice guarda el texto en claro.
	in.Indexable = ptr(f.indexable && !f.cifrado)
	if !f.cifrado {
		in.Crypto = nil
	}
	return in
}

// esDuenoCuaderno conserva los cuadernos de muestra sin servicio ni sesión; en una
// sesión real, el identificador del dueño tiene que coincidir.
func (p *Pantalla) esDuenoCuaderno(c *Cuaderno) bool {
	if c == nil || (p.svc == nil && p.Yo == nil) {
		return true
	}
	return p.esMio(c.Crudo.OwnerSub)
}

// puedeAdministrarCuaderno sigue la regla del servicio: el dueño, o quien
// administra la organización si el cuaderno pertenece a esa organización.
func (p *Pantalla) puedeAdministrarCuaderno(c *Cuaderno) bool {
	return p.esDuenoCuaderno(c) || (c != nil && c.Crudo.Visibility == "organization" &&
		p.AdminOrg != nil && p.AdminOrg())
}

// dialogoAjustes abre los ajustes de `c`, o el de «nuevo cuaderno» con c == nil y, si
// cuelga de otro, su `padre`.
func (p *Pantalla) dialogoAjustes(c *Cuaderno, padre string) {
	if p.win == nil {
		return
	}
	t := p.tr.T
	f := formAjustes{color: ColoresCuaderno[0], disposicion: VistaArbol.Clave(), visibilidad: "personal", indexable: true}
	if c != nil {
		f = formAjustes{
			color: c.Color, disposicion: c.Vista.Clave(), visibilidad: c.Publico,
			indexable: c.Crudo.Indexable, cifrado: c.Cifrado,
		}
	}
	// Los dos campos se crean UNA vez: el resto del cuerpo se rehace al elegir un color
	// o una disposición, y rehacer un campo perdería lo tecleado y el foco.
	emoji := ui.NuevaEntrada("📓")
	titulo := ui.NuevaEntrada("")
	if c != nil {
		emoji.PonerTexto(c.Emoji)
		titulo.PonerTexto(c.Titulo)
	}

	cuerpo := container.New(ui.Columna{})
	guardar := ui.NuevoBoton(t("common.save", nil), nil)
	guardar.Inerte = strings.TrimSpace(titulo.Texto()) == ""
	cancelar := ui.NuevoBoton(t("common.cancel", nil), nil).Con(ui.Fantasma, ui.TallaMD)
	var borrar fyne.CanvasObject
	if c != nil && p.esDuenoCuaderno(c) {
		borrar = ui.NuevoBoton(t("notebook.delete", nil), func() {
			p.confirmar(t("notebook.delete", nil), t("notebook.deleteConfirm", i18n.Vars{"title": c.Titulo}),
				t("common.delete", nil), func() {
					p.cerrarAjustes()
					go p.borrarCuaderno(*c)
				})
		}).Con(ui.Peligro, ui.TallaMD)
	}
	// Archivar va junto a borrar, como en la web: solo su dueño y nunca un diario.
	acciones := sinNil([]fyne.CanvasObject{borrar})
	// Desarchivar solo la cabeza de la rama: un hijo devuelto con el padre aún archivado
	// colgaría de uno que no se ve. El servicio también lo niega.
	if c != nil && p.esMio(c.Crudo.OwnerSub) && c.Crudo.DailyDate == "" &&
		(!c.Archivado || slices.ContainsFunc(raicesArchivadas(p.cuadernos), func(r Cuaderno) bool { return r.ID == c.ID })) {
		clave := "archive.archive"
		if c.Archivado {
			clave = "archive.unarchive"
		}
		archivar := ui.NuevoBoton(t(clave, nil), func() {
			p.cerrarAjustes()
			go p.archivar(*c, !c.Archivado)
		}).Con(ui.Secundario, ui.TallaMD)
		acciones = append(acciones, archivar)
	}
	derecha := len(acciones)
	acciones = append(acciones, cancelar, guardar)
	// Eliminar y archivar a la izquierda en escritorio; en el teléfono, de dos en dos a
	// todo el ancho (`ui.PieAcciones`, el pie de Modal.tsx).
	pie := container.New(ui.PieAcciones{Izquierda: derecha}, acciones...)

	rotulo := t("sidebar.newNotebook", nil)
	if c != nil {
		rotulo = t("notebook.settings", nil)
	}
	d := ui.NuevoDialogo(rotulo, cuerpo).Con(ui.DialogoMD, pie)
	d.Foco = titulo
	d.AlCerrar = p.cerrarAjustes
	p.ajustes = d

	var pintar func()
	pintar = func() {
		cuerpo.Objects = []fyne.CanvasObject{p.cuerpoAjustes(c, emoji, titulo, &f, pintar)}
		cuerpo.Refresh()
		d.Refresh()
	}
	titulo.AlCambiar(func(s string) {
		guardar.Inerte = strings.TrimSpace(s) == ""
		guardar.Refresh()
	})
	cancelar.AlPulsar = p.cerrarAjustes
	guardar.AlPulsar = func() {
		if guardar.Inerte || guardar.Cargando {
			return
		}
		guardar.Cargando = true
		guardar.Refresh()
		id := ""
		if c != nil {
			id = c.ID
		}
		in := entradaAjustes(c, padre, titulo.Texto(), strings.TrimSpace(emoji.Texto()), f)
		go func() {
			err := p.guardarCuaderno(id, in)
			enHilo(func() {
				guardar.Cargando = false
				guardar.Refresh()
				if err == nil {
					p.cerrarAjustes()
				}
			})
		}()
	}
	pintar()
	p.mostrar(d)
}

// cerrarAjustes retira el diálogo de ajustes si está abierto.
func (p *Pantalla) cerrarAjustes() {
	if p.ajustes != nil {
		p.ajustes.Cerrar()
		p.ajustes = nil
	}
}

// anchoCuerpoAjustes sigue siendo el ancho inicial del editor de etiquetas.
func anchoCuerpoAjustes(ventana fyne.Size) float32 {
	return ui.AnchoCuerpoTalla(ui.DialogoMD, ventana)
}

// rejillaAjustes solo elige el corte de VENTANA de NotebookSettings.tsx. El reparto
// de columnas y el alto envuelto siguen siendo los de la Rejilla compartida.
type rejillaAjustes struct{ ventana func() float32 }

func (r rejillaAjustes) rejilla() ui.Rejilla {
	columnas := 2
	if r.ventana() >= ui.CorteSM {
		columnas = 3
	}
	return ui.Rejilla{Fijas: columnas, Hueco: HuecoDisposicion}
}

func (r rejillaAjustes) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return r.rejilla().MinSize(objs)
}

func (r rejillaAjustes) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return r.rejilla().MinSizeCon(objs, ancho).Height
}

func (r rejillaAjustes) Layout(objs []fyne.CanvasObject, tam fyne.Size) {
	r.rejilla().Layout(objs, tam)
}

// cuerpoAjustes arma todo lo de dentro con el estado de ahora.
func (p *Pantalla) cuerpoAjustes(c *Cuaderno, emoji, titulo *ui.Entrada, f *formAjustes,
	repintar func(),
) fyne.CanvasObject {
	v, t := p.variante(), p.tr.T
	emoji.Ancho = AnchoIconoAjustes
	titulo.Ancho = 0
	campos := container.New(ui.FilaAuto{Hueco: HuecoCamposAjust, Derecha: 2, Partir: true, SaltoFila: HuecoCamposAjust, Elastico: 2},
		grupoCampo(t("notebook.emoji", nil), emoji, v),
		grupoCampo(t("notebook.title", nil), titulo, v))

	// Color.
	muestras := make([]fyne.CanvasObject, 0, len(ColoresCuaderno))
	for _, hex := range ColoresCuaderno {
		m := NuevaMuestra(hexO(hex, tema.Color("primary", v)), LadoMuestra, 2, hex,
			func() { f.color = hex; repintar() })
		m.Activa = strings.EqualFold(f.color, hex)
		muestras = append(muestras, m)
	}
	hm := conCeldaTactil(muestras, HuecoMuestras)
	paleta := leyenda(t("notebook.color", nil), container.New(ui.FilaAuto{Hueco: hm, Derecha: len(muestras), Partir: true, SaltoFila: hm}, muestras...), v)

	// Disposición: dos por fila en móvil, tres desde sm, como la web.
	dispos := make([]fyne.CanvasObject, 0, len(Disposiciones))
	for _, d := range Disposiciones {
		clave := d.D.Clave()
		dispos = append(dispos, opcionAjuste(
			container.New(ui.Columna{Hueco: 1 * 0.25 * tema.Rem},
				// En una `Fila`, que coloca a su medida: la columna ESTIRA a sus hijos
				// al ancho entero y el glifo salía deformado.
				container.New(ui.Fila{}, Glifo(d.Glifo, 20, "text-muted", v)),
				ui.NuevoParrafo(t(d.D.ClaveI18n(), nil), tema.TextoBase, ui.Medio, "text", v),
				ui.NuevoParrafo(t(d.D.ClaveI18n()+".hint", nil), tema.TextoXS, ui.Normal, "text-subtle", v)),
			t(d.D.ClaveI18n(), nil), f.disposicion == clave, func() { f.disposicion = clave; repintar() }, v))
	}
	disposicion := leyenda(t("layout.label", nil),
		container.New(rejillaAjustes{ventana: func() float32 { return p.win.Canvas().Size().Width }}, dispos...), v)

	piezas := []fyne.CanvasObject{campos, paleta, disposicion}
	if p.puedeAdministrarCuaderno(c) {
		// Visibilidad.
		vis := make([]fyne.CanvasObject, 0, len(Visibilidades))
		for _, x := range Visibilidades {
			clave := x.Clave
			texto := container.New(ui.Columna{},
				ui.NuevoParrafo(t("visibility."+clave, nil), tema.TextoBase, ui.Medio, "text", v),
				ui.NuevoParrafo(t("visibility."+clave+".hint", nil), tema.TextoXS, ui.Normal, "text-subtle", v))
			glifo := container.New(&margenY{arriba: 0.5 * 0.25 * tema.Rem}, Glifo(x.Glifo, 18, "text-muted", v))
			vis = append(vis, opcionAjuste(
				container.New(ui.FilaAuto{Hueco: 2.5 * 0.25 * tema.Rem, Derecha: 2, Partir: true, Elastico: 2}, glifo, texto),
				t("visibility."+clave, nil), f.visibilidad == clave, func() {
					f.visibilidad = clave
					f.indexable = defectoIndexable(clave, f.cifrado)
					repintar()
				}, v))
		}
		visibilidad := leyenda(t("visibility.label", nil),
			container.New(ui.Columna{Hueco: HuecoVisibilidad}, vis...), v)
		piezas = append(piezas, visibilidad)
	}
	if enlace := p.enlacePublico(c, f.visibilidad); enlace != "" {
		piezas = append(piezas, p.filaEnlacePublico(enlace, v))
	}
	piezas = append(piezas, p.cajaIndice(c, f, repintar, v))
	return container.New(ui.Columna{Hueco: HuecoAjustes}, piezas...)
}

// grupoCampo es FieldGroup de Form.tsx: el rótulo `text-sm font-semibold` y, a 5,25,
// el campo.
func grupoCampo(rotulo string, campo fyne.CanvasObject, v tema.Variante) fyne.CanvasObject {
	return container.New(ui.Columna{Hueco: 1.5 * 0.25 * tema.Rem},
		lineaTok(rotulo, tema.TextoSM, ui.SemiNegrita, "text", v), campo)
}

// leyenda es un `<fieldset>` con su `<legend>` `text-base font-medium mb-1.5`.
func leyenda(texto string, dentro fyne.CanvasObject, v tema.Variante) fyne.CanvasObject {
	return container.New(ui.Columna{Hueco: HuecoLeyenda},
		lineaTok(texto, tema.TextoBase, ui.Medio, "text", v), dentro)
}

// opcionAjuste es una tarjeta elegible de disposición o de visibilidad: `rounded-md
// border p-2.5`, y la elegida con `border-primary bg-primary/5`.
func opcionAjuste(dentro fyne.CanvasObject, rotulo string, activa bool, alPulsar func(), v tema.Variante) *Opcion {
	o := NuevaOpcion(rotulo, dentro, alPulsar)
	o.PX, o.PY, o.Borde, o.Activa = RellenoOpcion, RellenoOpcion, true, activa
	o.BordeActivo = tema.Color("primary", v)
	o.FondoActivo = velo(tema.Color("primary", v), 0.05)
	return o
}

// enlacePublico es la dirección del cuaderno publicado, si lo está y se sigue viendo
// como público. `public_token` solo se lo sirve el servicio a su dueño.
func (p *Pantalla) enlacePublico(c *Cuaderno, visibilidad string) string {
	if c == nil || c.Crudo.PublicToken == "" || visibilidad != "public" || p.AppURL == nil {
		return ""
	}
	return p.AppURL("/p/" + c.Crudo.PublicToken)
}

// filaEnlacePublico es el enlace con «Copiar enlace público»: `rounded-md border
// bg-bg-subtle p-2.5 gap-2`, el enlace en `text-sm` recortado.
func (p *Pantalla) filaEnlacePublico(enlace string, v tema.Variante) fyne.CanvasObject {
	t := p.tr.T
	copiar := ui.NuevoBoton(t("public.copyLink", nil), func() {
		fyne.CurrentApp().Clipboard().SetContent(enlace)
		p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, t("public.copied", nil), ""))
	}).Con(ui.Secundario, ui.TallaSM)
	texto := lineaTok(enlace, tema.TextoSM, ui.Normal, "text-muted", v)
	return enCajaDe(tema.Color("bg-subtle", v), RellenoOpcion,
		container.New(&filaElastica{hueco: HuecoFilaDialogo}, texto, copiar), v)
}

// enCajaDe mete un hijo en una caja con borde, radio `md`, ese fondo y ese relleno (el
// borde va por dentro y ocupa sitio, como con `box-sizing: border-box`).
func enCajaDe(fondo color.NRGBA, relleno float32, dentro fyne.CanvasObject, v tema.Variante) fyne.CanvasObject {
	caja := ui.NuevaCaja(fondo, tema.Color("border", v), tema.RadioMD)
	return container.New(&enCaja{caja: caja, margen: relleno + tema.Borde},
		append(caja.Objetos(), dentro)...)
}

// cajaIndice es la caja del índice y del cifrado, con la consecuencia de cada uno
// DEBAJO y no en una pista: decidir a ciegas si algo se indexa o se cifra no vale.
func (p *Pantalla) cajaIndice(c *Cuaderno, f *formAjustes, repintar func(), v tema.Variante) fyne.CanvasObject {
	t := p.tr.T

	casilla := ui.NuevaCasilla(t("index.label", nil), func(si bool) { f.indexable = si })
	casilla.AnchoEtiqueta = ui.Medir(casilla.Etiqueta, tema.TextoBase, ui.Normal).Width
	casilla.Marcada = f.indexable && !f.cifrado
	ayudaIndice := "index.hint"
	if f.cifrado {
		casilla.Disable()
		ayudaIndice = "index.lockedByEncryption"
	}
	indice := container.New(ui.Columna{Hueco: 1 * 0.25 * tema.Rem}, casilla,
		sangrado2(SangriaAyudaCasilla, ui.NuevoParrafo(t(ayudaIndice, nil), tema.TextoXS, ui.Normal,
			"text-subtle", v)))

	piezas := []fyne.CanvasObject{indice}
	if p.puedeAdministrarCuaderno(c) {
		interruptor := ui.NuevoConmutador(t("crypto.title", nil), func(si bool) {
			if !si {
				f.cifrado, f.crypto = false, nil
				f.indexable = defectoIndexable(f.visibilidad, false)
				repintar()
				return
			}
			// Encender pide la contraseña dos veces (`PassphraseDialog modo="crear"`); hasta
			// que se da, el interruptor sigue apagado.
			repintar()
			p.cifrarCuaderno(func(crypto json.RawMessage) {
				f.cifrado, f.crypto, f.indexable = true, crypto, false
				repintar()
			})
		})
		interruptor.Encendido = f.cifrado
		interruptor.AnchoEtiqueta = ui.Medir(interruptor.Etiqueta, tema.TextoBase, ui.Normal).Width
		cifrado := container.New(ui.Columna{Hueco: 1 * 0.25 * tema.Rem}, interruptor,
			sangrado2(SangriaAyudaInterruptor, ui.NuevoParrafo(t("crypto.hint", nil), tema.TextoXS, ui.Normal,
				"text-subtle", v)))
		piezas = append(piezas, cifrado)
		if c != nil && c.Cifrado && !f.cifrado {
			piezas = append(piezas, ui.NuevoParrafo(t("crypto.disableWarning", nil), tema.TextoBase, ui.Normal,
				"warning-text", v))
		}
	}
	return enCajaDe(tema.Color("surface-raised", v), RellenoCajaIndice,
		container.New(ui.Columna{Hueco: 3 * 0.25 * tema.Rem}, piezas...), v)
}

// sangrado2 es un `pl-N` sobre un hijo.
func sangrado2(lado float32, o fyne.CanvasObject) fyne.CanvasObject {
	return container.New(ui.Margen{Izq: lado}, o)
}
