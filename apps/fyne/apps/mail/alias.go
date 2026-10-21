// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Alias de correo» (`AliasesModal.tsx`)
//
// Qué es un alias; la principal arriba (no se quita) y los alias con su aspa; y uno
// nuevo, con la dirección validada EN VIVO —vacía, sin dominio, mal escrita, repetida
// o con buena pinta— antes de llamar al servicio, que valida otra vez contra los
// dominios que sirve. Los alias alimentan el «De» del redactor.

// GestorAlias es lo que el diálogo le pide al servidor además de la lista (Aliases,
// que ya está en Remitente). Lo cumple *api.Mail.
type GestorAlias interface {
	CreateAlias(ctx context.Context, direccion string) (api.MailAlias, error)
	DeleteAlias(ctx context.Context, id string) error
}

var _ GestorAlias = (*api.Mail)(nil)

// pareceCorreo es `EMAIL_RE`: solo evita llamadas obviamente inválidas.
var pareceCorreo = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// PistaAlias es `draftHint`: el tono y la clave de la pista bajo la dirección.
func PistaAlias(borrador, principal string, alias []api.MailAlias) (ui.TonoAyuda, string) {
	d := strings.TrimSpace(borrador)
	repetida := d != "" && (strings.EqualFold(d, principal) ||
		slices.ContainsFunc(alias, func(a api.MailAlias) bool { return strings.EqualFold(a.Address, d) }))
	switch {
	case repetida:
		return ui.AyudaPeligro, "aliases.hintDuplicate"
	case d == "":
		return ui.AyudaNeutra, "aliases.hintEmpty"
	case !strings.Contains(d, "@"):
		return ui.AyudaPeligro, "aliases.hintNoDomain"
	case pareceCorreo.MatchString(d):
		return ui.AyudaExito, "aliases.hintValid"
	}
	return ui.AyudaPeligro, "aliases.hintInvalid"
}

type gestorAlias struct {
	b         *Buzon
	d         *ui.Dialogo
	caja      *fyne.Container
	ancho     float32
	estado    Estado
	principal string
	alias     []api.MailAlias
	borrador  *ui.Entrada
	anadiendo bool
}

// verAlias abre «Alias de correo». Devuelve el gestor para las pruebas.
func (b *Buzon) verAlias() *gestorAlias {
	lienzo := b.P.Lienzo()
	if lienzo == nil {
		return nil
	}
	t := b.P.T
	g := &gestorAlias{b: b, ancho: AnchoCuerpoDialogo(ui.DialogoMD, lienzo.Size().Width)}
	g.borrador = ui.NuevaEntrada(t.T("aliases.placeholder", nil))
	g.borrador.AlCambiar(func(string) { g.pintar() })
	g.borrador.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			g.anadir()
			return true
		}
		return false
	}
	g.caja = container.New(ui.Columna{Hueco: huecoSeccionesEtiq})
	cerrar := ui.NuevoBoton(t.T("common.close", nil), nil).Con(ui.Secundario, ui.TallaMD)
	g.d = ui.NuevoDialogo(t.T("aliases.title", nil), g.caja).Con(ui.DialogoMD, cerrar)
	cerrar.AlPulsar = g.d.Cerrar
	g.d.RotuloCerrar = t.T("ui.close", nil)
	g.d.Mostrar(lienzo)
	g.cargar()
	return g
}

func (g *gestorAlias) cargar() {
	g.estado = Cargando
	g.pintar()
	pedir(g.b, func(ctx context.Context) (listaAlias, error) {
		p, a, err := g.b.S.Aliases(ctx)
		return listaAlias{p, a}, err
	}, func(r listaAlias, err error) {
		if err != nil {
			g.estado = Error
		} else {
			g.principal, g.alias, g.estado = r.principal, r.alias, Listo
		}
		g.pintar()
	})
}

// listaAlias es lo que devuelve Aliases, en un valor para `pedir`.
type listaAlias struct {
	principal string
	alias     []api.MailAlias
}

// puede es `canAdd`.
func (g *gestorAlias) puede() bool {
	tono, _ := PistaAlias(g.borrador.Texto(), g.principal, g.alias)
	return tono == ui.AyudaExito && !g.anadiendo
}

func (g *gestorAlias) pintar() {
	t := g.b.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ayuda := ui.NuevaAyudaEnLinea(t.T("aliases.whatBody", nil)).ConTono(ui.AyudaInfo).ConTitulo(t.T("aliases.whatTitle", nil))

	var tuyas fyne.CanvasObject
	switch g.estado {
	case Cargando:
		// `flex items-center gap-2 py-4`.
		rueda := ui.NuevaRueda(16, "text-subtle", v)
		rueda.Rotulo = t.T("aliases.loading", nil)
		tuyas = container.New(ui.Margen{Arr: tema.Espacio(4), Abj: tema.Espacio(4)},
			container.New(ui.Fila{Hueco: tema.Espacio(2)}, rueda,
				ui.NuevoRotulo(t.T("common.loading", nil), tema.TextoSM, ui.Normal, "text-subtle")))
	case Error:
		icono := iconos.Trazo(iconos.Aviso, 16, "text", v)
		tuyas = container.New(ui.Margen{Arr: tema.Espacio(2), Abj: tema.Espacio(2)},
			container.New(ui.Columna{Hueco: tema.Espacio(2)},
				container.New(ui.Fila{Hueco: tema.Espacio(2)}, container.New(ui.Margen{Arr: tema.Espacio(0.5)}, icono.Objeto()),
					ui.NuevoRotulo(t.T("aliases.loadError", nil), tema.TextoSM, ui.Normal, "text")),
				container.New(ui.Fila{}, ui.NuevoBoton(t.T("common.retry", nil), g.cargar).Con(ui.Secundario, ui.TallaSM))))
	default:
		var filas []fyne.CanvasObject
		if g.principal != "" {
			filas = append(filas, filaAlias(ui.NuevoRotulo(g.principal, tema.TextoSM, ui.Medio, "text"),
				ui.NuevaInsignia(t.T("aliases.primary", nil), ui.InsigniaNeutra)))
		}
		for _, a := range g.alias {
			x := ui.NuevoBoton(t.T("aliases.deleteAlias", i18n.Vars{"address": a.Address}), func() { g.borrar(a) }).
				Con(ui.Fantasma, ui.TallaSM)
			x.SoloIcono, x.Icono, x.TamIcono = true, iconos.Cerrar, 16
			filas = append(filas, filaAlias(ui.NuevoRotulo(a.Address, tema.TextoSM, ui.Normal, "text"), x))
		}
		if len(g.alias) == 0 {
			vacio := ui.NuevoVacio(t.T("aliases.emptyTitle", nil), t.T("aliases.emptyDescription", nil))
			vacio.Ilustracion = iconos.Trazo(iconos.Usuario, 32, "text-subtle", v).Objeto()
			vacio.Accion = ui.NuevoBoton(t.T("aliases.emptyAction", nil), func() {
				if c := g.b.P.Lienzo(); c != nil {
					c.Focus(g.borrador)
				}
			}).Con(ui.Secundario, ui.TallaMD)
			filas = append(filas, vacio)
		}
		tuyas = enCajaConFiletes(filas)
	}

	// La dirección y «Añadir» en fila (`flex items-start gap-2`), y la pista debajo.
	tono, clave := PistaAlias(g.borrador.Texto(), g.principal, g.alias)
	g.borrador.Invalida = tono == ui.AyudaPeligro
	g.borrador.Refresh()
	anadir := ui.NuevoBoton(t.T("common.add", nil), g.anadir)
	anadir.Icono, anadir.Cargando, anadir.Inerte = iconos.Mas, g.anadiendo, !g.puede()
	pista := ui.NuevaPistaCampo(t.T(clave, nil)).ConTono(tono)
	pista.Ancho = g.ancho
	campo := ui.GrupoCampo(t.T("common.address", nil), false,
		container.New(extremos{hueco: tema.Espacio(2)}, g.borrador, anadir), "", "")
	campo.Add(pista)
	nueva := seccion(t.T("aliases.addTitle", nil), t.T("aliases.addDescription", nil), g.ancho, campo)

	g.caja.Objects = []fyne.CanvasObject{
		ayuda,
		seccion(t.T("aliases.yoursTitle", nil), t.T("aliases.yoursDescription", nil), g.ancho, tuyas),
		container.New(ui.Columna{Hueco: aireSeccionNueva}, canvasFilete(v), nueva),
	}
	g.caja.Refresh()
	if g.d != nil {
		g.d.Recolocar() // el diálogo se vuelve a medir con su contenido nuevo
	}
}

// filaAlias es `flex items-center justify-between gap-3 px-3 py-2.5`.
func filaAlias(texto, fin fyne.CanvasObject) fyne.CanvasObject {
	return container.New(ui.Margen{Izq: margenXFilaEtiqueta, Der: margenXFilaEtiqueta, Arr: margenYFilaEtiqueta, Abj: margenYFilaEtiqueta},
		container.New(extremos{centro: true}, texto, fin))
}

// anadir es `handleAdd`: el motivo de un rechazo del servicio va en la descripción.
func (g *gestorAlias) anadir() {
	if !g.puede() {
		return
	}
	d := strings.TrimSpace(g.borrador.Texto())
	t := g.b.P.T
	g.anadiendo = true
	g.pintar()
	pedir(g.b, func(ctx context.Context) (api.MailAlias, error) {
		return g.b.S.CreateAlias(ctx, d)
	}, func(_ api.MailAlias, err error) {
		g.anadiendo = false
		if err != nil {
			desc := "aliases.createFailedDescription"
			if e := (*api.Error)(nil); errors.As(err, &e) {
				switch e.Code() {
				case "alias_reserved":
					desc = "aliases.reservedDescription"
				case "alias_limit":
					desc = "aliases.limitDescription"
				}
			}
			g.b.P.Avisar(ui.AvisoPeligro, t.T("aliases.createFailed", nil), t.T(desc, nil))
			g.pintar()
			return
		}
		g.borrador.PonerTexto("")
		g.b.P.Avisar(ui.AvisoExito, t.T("aliases.added", nil), t.T("aliases.addedDescription", i18n.Vars{"address": d}))
		g.b.cargarIdentidad()
		g.cargar()
	})
}

// borrar es `handleDelete`: optimista, y de vuelta si el servicio dice que no.
func (g *gestorAlias) borrar(a api.MailAlias) {
	antes := slices.Clone(g.alias)
	g.alias = slices.DeleteFunc(g.alias, func(x api.MailAlias) bool { return x.ID == a.ID })
	g.pintar()
	pedir(g.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, g.b.S.DeleteAlias(ctx, a.ID)
	}, func(_ struct{}, err error) {
		if err != nil {
			g.alias = antes
			g.pintar()
			g.b.avisar(ui.AvisoPeligro, "aliases.deleteFailed", nil)
			return
		}
		g.b.cargarIdentidad()
	})
}
