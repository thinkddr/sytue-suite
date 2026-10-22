// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"context"
	"image/color"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El grupo «Tema» de la cinta (`TemaGroup.tsx`) y el diálogo de la marca de la
// organización (`MarcaDialog.tsx`): aplicar un tema común o la marca a un documento sin
// tocar su contenido y, para quien administra la organización, guardar una versión nueva
// de la marca. Qué puede cada quien lo decide el servicio; aquí solo se enseña.

// FuentesDeTema son las que ofrece la marca (`THEME_FONTS` de slides/temas.ts).
var FuentesDeTema = []string{
	"Georgia", "Calibri", "Arial", "Verdana", "Trebuchet MS", "Times New Roman", "Garamond", "Courier New",
}

// MaxNombreTema es el tope del nombre de la marca (`MAX_THEME_NAME`).
const MaxNombreTema = 60

var reHex = regexp.MustCompile(`^#?([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// NormalizarHex es `normalizeHex`: «#RRGGBB» en mayúsculas desde «#RGB», «RGB», «RRGGBB» o
// «#RRGGBB»; false si no es un color.
func NormalizarHex(raw string) (string, bool) {
	m := reHex.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return "", false
	}
	h := m[1]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	return "#" + strings.ToUpper(h), true
}

func colorDeHex(hex string) color.NRGBA {
	n, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		return color.NRGBA{}
	}
	return color.NRGBA{R: uint8(n >> 16 & 0xFF), G: uint8(n >> 8 & 0xFF), B: uint8(n & 0xFF), A: 0xFF}
}

// TemasDoc es lo que el grupo «Tema» necesita del producto y del documento abierto.
type TemasDoc struct {
	Svc *api.Docs
	// Lanzar corre una petición fuera del hilo de la interfaz; nil, una rutina nueva.
	Lanzar func(func())
	// DocID es el documento abierto.
	DocID string
	// Guardar vuelca lo pendiente ANTES de aplicar —el tema se aplica al estado del
	// servidor—; corre fuera del hilo de la interfaz. nil, nada pendiente.
	Guardar func()
	// Recargar reabre el documento con el tema ya aplicado.
	Recargar func()

	datos  *api.DocThemes
	pedido bool
}

func (t *TemasDoc) lanzar(f func()) {
	if t.Lanzar != nil {
		t.Lanzar(f)
		return
	}
	go f()
}

// cargar pide los temas una vez; `listo` repinta la cinta con ellos.
func (t *TemasDoc) cargar(listo func()) {
	if t.pedido || t.Svc == nil {
		return
	}
	t.pedido = true
	t.recargar(listo)
}

func (t *TemasDoc) recargar(listo func()) {
	t.lanzar(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelar()
		d, err := t.Svc.Themes(ctx)
		fyne.Do(func() {
			if err == nil {
				t.datos = &d
			}
			if listo != nil {
				listo()
			}
		})
	})
}

// editarMarca y actualizarMarca son los valores de las dos opciones del selector que no
// son un tema (`EDITAR_MARCA`, `ACTUALIZAR_MARCA`).
const (
	editarMarca     = "__editar-marca"
	actualizarMarca = "__actualizar-marca"
)

// Grupo es el grupo «Tema» con su separador por delante, o nil mientras los temas no han
// llegado (la web tampoco lo pinta sin datos).
func (t *TemasDoc) Grupo(c *Contexto) []fyne.CanvasObject {
	t.cargar(c.Refrescar)
	if t.datos == nil {
		return nil
	}
	tr := func(k string, v ...i18n.Vars) string { return c.Traducir(k, v...) }
	opciones := []Opcion{{Valor: "", Rotulo: tr("theme.placeholder")}}
	for _, o := range OpcionesDeTema(func(k string, v i18n.Vars) string { return c.Traducir(k, v) }, *t.datos) {
		opciones = append(opciones, Opcion{Valor: o.ID, Rotulo: o.Rotulo})
		if o.ID == api.IDTemaMarca { // justo tras la marca, como en la web
			opciones = append(opciones, Opcion{Valor: actualizarMarca, Rotulo: tr("theme.update")})
		}
	}
	if t.datos.CanManage {
		opciones = append(opciones, Opcion{Valor: editarMarca, Rotulo: tr("theme.editBrand")})
	}
	var sel *SelectorCinta
	sel = NuevoSelectorCinta(tr("theme.select"), 150, opciones, "", func(v string) {
		switch v {
		case "":
		case editarMarca:
			t.PedirMarca(c, fyne.CurrentApp().Driver().CanvasForObject(sel))
		case actualizarMarca:
			t.ActualizarMarca(c, fyne.CurrentApp().Driver().CanvasForObject(sel))
		default:
			t.Aplicar(c, v)
		}
	})
	sel.Inerte = c.SoloLectura || c.SinRed
	return []fyne.CanvasObject{SeparadorGrupos(), NuevoGrupo(tr("theme.group"), sel)}
}

// Aplicar aplica un tema al documento: guarda lo pendiente, lo pide al servicio y reabre
// el documento. Nunca falla en silencio: un aviso cuenta lo que pasó.
func (t *TemasDoc) Aplicar(c *Contexto, id string) {
	t.lanzar(func() {
		if t.Guardar != nil {
			t.Guardar()
		}
		ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelar()
		cambio, err := t.Svc.ApplyTheme(ctx, t.DocID, id, 0)
		fyne.Do(func() {
			switch {
			case err != nil:
				c.Notificar(ui.AvisoPeligro, c.Traducir("theme.msg.failed"))
			case cambio:
				if t.Recargar != nil {
					t.Recargar()
				}
				c.Notificar(ui.AvisoExito, c.Traducir("theme.msg.applied"))
			default:
				c.Notificar(ui.AvisoInfo, c.Traducir("theme.msg.unchanged"))
			}
		})
	})
}

// campoColor es una fila «etiqueta, muestra y #RRGGBB» de la marca. Lo que no es un color
// se marca como inválido y no deja guardar.
type campoColor struct {
	hex     string
	muestra *canvas.Rectangle
	campo   *ui.Entrada
}

func nuevoCampoColor(valor string, alCambiar func()) *campoColor {
	k := &campoColor{hex: valor, muestra: canvas.NewRectangle(colorDeHex(valor)), campo: ui.NuevaEntrada("")}
	// La muestra con su filete (el fondo #FFFFFF no se veía sobre el panel) y a la altura
	// del campo; el campo, el de la suite (`w-28`), no el de 24 de la cinta.
	k.muestra.CornerRadius = tema.Forma("radius-control")
	k.muestra.StrokeColor = tema.Color("border", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	k.muestra.StrokeWidth = tema.Borde
	k.muestra.SetMinSize(fyne.NewSize(tema.Espacio(11), ui.AltoCampo(ui.CampoMD)))
	k.campo.Talla, k.campo.Ancho = ui.CampoMD, tema.Espacio(28)
	k.campo.PonerTexto(valor)
	k.campo.AlCambiar(func(s string) {
		h, ok := NormalizarHex(s)
		k.campo.Invalida = !ok
		if ok {
			k.hex = h
			k.muestra.FillColor = colorDeHex(h)
			k.muestra.Refresh()
		}
		k.campo.Refresh()
		alCambiar()
	})
	return k
}

func (k *campoColor) valido() bool { return !k.campo.Invalida }

// PedirMarca abre el diálogo de la marca: nombre, tres colores y dos fuentes. Al guardar
// crea una versión NUEVA (las anteriores se conservan) y refresca los temas.
func (t *TemasDoc) PedirMarca(c *Contexto, lienzo fyne.Canvas) {
	if lienzo == nil {
		return
	}
	tr := func(k string, v ...i18n.Vars) string { return c.Traducir(k, v...) }
	var actual *api.DocBrand
	if t.datos != nil && len(t.datos.Versions) > 0 {
		actual = &t.datos.Versions[0]
	}
	m := api.DocBrand{Name: "", Bg: "#FFFFFF", Ink: "#0A0A0A", Accent: "#1F2937"}
	if actual != nil {
		m = *actual
	}
	var (
		dlg     *ui.Dialogo
		guardar *ui.Boton
	)
	nombre := ui.NuevaEntrada("")
	nombre.PonerTexto(m.Name)
	listo := func() {
		guardar.Inerte = strings.TrimSpace(nombre.Texto()) == ""
		guardar.Refresh()
	}
	bg, ink, acento := nuevoCampoColor(m.Bg, listo), nuevoCampoColor(m.Ink, listo), nuevoCampoColor(m.Accent, listo)
	fuente := func(actual string, alElegir func(string)) *ui.Selector {
		ops := append([]string{tr("brand.fontDefault")}, FuentesDeTema...)
		elegida := 0
		for i, f := range FuentesDeTema {
			if f == actual {
				elegida = i + 1
			}
		}
		if actual != "" && elegida == 0 { // una fuente que ya tenía la marca y no está en la lista
			ops = append(ops, actual)
			elegida = len(ops) - 1
		}
		s := ui.NuevoSelector(ops, func(i int) {
			if i == 0 {
				alElegir("")
				return
			}
			alElegir(ops[i])
		})
		s.Elegida = elegida
		return s
	}
	titulos := fuente(m.HeadingFont, func(f string) { m.HeadingFont = f })
	cuerpoF := fuente(m.BodyFont, func(f string) { m.BodyFont = f })
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// Como la web: el nombre y las fuentes en un `FieldGroup` a todo el ancho, y cada
	// color con su rótulo a la izquierda y la muestra y el hex en su columna a la derecha.
	grupo := func(rotulo string, control fyne.CanvasObject) fyne.CanvasObject {
		return ui.GrupoCampo(rotulo, false, control, "", "")
	}
	util := ui.AnchoCuerpoTalla(ui.DialogoSM, lienzo.Size())
	filaColor := func(rotulo string, k *campoColor) fyne.CanvasObject {
		controles := container.New(ui.Fila{Hueco: tema.Espacio(2)}, k.muestra, k.campo)
		// El rótulo, repartido a lo que dejan los controles (`gap-3`): «Acento (títulos y
		// cabeceras de tabla)» parte de línea en vez de empujar el campo hasta el canto.
		cabe := max(util-controles.MinSize().Width-tema.Espacio(3), 0)
		return container.NewBorder(nil, nil, nil, controles,
			ui.NuevoParrafo(rotulo, tema.TextoSM, ui.Medio, "text", v).A(cabe))
	}
	siguiente := 1
	if actual != nil {
		siguiente = actual.Version + 1
	}
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(4)},
		grupo(tr("brand.name"), nombre),
		container.New(ui.Columna{Hueco: tema.Espacio(2)},
			filaColor(tr("brand.bg"), bg), filaColor(tr("brand.ink"), ink), filaColor(tr("brand.accent"), acento)),
		grupo(tr("brand.headingFont"), titulos), grupo(tr("brand.bodyFont"), cuerpoF),
		ui.Texto(tr("brand.version", i18n.Vars{"version": siguiente}), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
	)
	guardar = ui.NuevoBoton(tr("brand.save"), func() {
		if guardar.Cargando || guardar.Inerte || !bg.valido() || !ink.valido() || !acento.valido() {
			return
		}
		guardar.Cargando = true
		guardar.Refresh()
		m.Name, m.Bg, m.Ink, m.Accent = strings.TrimSpace(nombre.Texto()), bg.hex, ink.hex, acento.hex
		if len([]rune(m.Name)) > MaxNombreTema {
			m.Name = string([]rune(m.Name)[:MaxNombreTema])
		}
		t.lanzar(func() {
			ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancelar()
			version, err := t.Svc.SaveBrand(ctx, m)
			fyne.Do(func() {
				guardar.Cargando = false
				guardar.Refresh()
				if err != nil {
					c.Notificar(ui.AvisoPeligro, tr("brand.msg.failed"))
					return
				}
				c.Notificar(ui.AvisoExito, tr("brand.msg.saved", i18n.Vars{"version": version}))
				dlg.Cerrar()
				t.recargar(c.Refrescar)
			})
		})
	})
	guardar.Inerte = strings.TrimSpace(nombre.Texto()) == ""
	nombre.AlCambiar(func(string) { listo() })
	cancelar := ui.NuevoBoton(tr("action.cancel"), func() { dlg.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, guardar)
	dlg = ui.NuevoDialogo(tr("brand.title"), cuerpo).Con(ui.DialogoSM, pie)
	dlg.RotuloCerrar = tr("ui.close")
	dlg.Mostrar(lienzo)
}

// claveDeCambio es `CLAVE_DE_CAMBIO` de ActualizarMarcaDialog.tsx.
var claveDeCambio = map[string]string{
	"headings":      "theme.change.headings",
	"table_headers": "theme.change.table_headers",
	"body_font":     "theme.change.body_font",
	"sheet_header":  "theme.change.sheet_header",
	"slide_theme":   "theme.change.slide_theme",
}

// LineaDeCambio es el texto de un cambio de la previa (`lineaDeCambio`); un valor de
// partida vacío es «predeterminado».
func LineaDeCambio(t func(string, i18n.Vars) string, k api.DocThemeChange) string {
	if k.Scope == "logo" {
		if k.To != "" {
			return t("theme.change.logoSet", nil)
		}
		return t("theme.change.logoRemoved", nil)
	}
	de := k.From
	if de == "" {
		de = t("theme.change.unset", nil)
	}
	clave, ok := claveDeCambio[k.Scope]
	if !ok {
		return k.Scope // un ámbito que este cliente aún no conoce: se dice tal cual, no se calla
	}
	return t(clave, i18n.Vars{"count": k.Count, "from": de, "to": k.To, "sheet": k.Sheet})
}

// ActualizarMarca es «Actualizar a la marca actual…»: pregunta al servicio qué cambiaría
// (sin escribir nada), lo enseña y, si hay algo, lo aplica.
func (t *TemasDoc) ActualizarMarca(c *Contexto, lienzo fyne.Canvas) {
	if lienzo == nil {
		return
	}
	tr := func(k string, v i18n.Vars) string { return c.Traducir(k, v) }
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	parrafo := func(s, token string) fyne.CanvasObject {
		return ui.NuevoParrafo(s, tema.TextoSM, ui.Normal, token, v)
	}
	var dlg *ui.Dialogo
	aplicar := ui.NuevoBoton(tr("theme.update.apply", nil), func() {
		dlg.Cerrar()
		t.Aplicar(c, api.IDTemaMarca)
	})
	aplicar.Inerte = true
	cancelar := ui.NuevoBoton(tr("action.cancel", nil), func() { dlg.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, aplicar)
	dlg = ui.NuevoDialogo(tr("theme.update.title", nil), parrafo(tr("theme.update.loading", nil), "text-muted")).Con(ui.DialogoSM, pie)
	dlg.RotuloCerrar = tr("ui.close", nil)
	dlg.Mostrar(lienzo)
	t.lanzar(func() {
		if t.Guardar != nil {
			t.Guardar() // la previa se calcula sobre el estado del servidor
		}
		ctx, cancelarCtx := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelarCtx()
		p, err := t.Svc.PreviewTheme(ctx, t.DocID, api.IDTemaMarca, 0)
		fyne.Do(func() {
			switch {
			case err != nil:
				dlg.CambiarCuerpo(parrafo(tr("theme.update.failed", nil), "text"))
			case !p.Changed:
				dlg.CambiarCuerpo(parrafo(tr("theme.update.none", nil), "text"))
			default:
				filas := make([]fyne.CanvasObject, 0, 1+len(p.Changes))
				filas = append(filas, parrafo(tr("theme.update.intro", i18n.Vars{"version": p.Version}), "text-muted"))
				for _, k := range p.Changes {
					filas = append(filas, parrafo("• "+LineaDeCambio(tr, k), "text"))
				}
				dlg.CambiarCuerpo(container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...))
				aplicar.Inerte = false
				aplicar.Refresh()
			}
		})
	})
}
