// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// «Combinar con…» (`MergeModal`) y «Posibles duplicados» (`DuplicatesModal`), de
// ContactsApp.tsx. Los dos son diálogos `md`.
//
//	paso 1   buscador (lupa de 16) y la lista `rounded-md border max-h-80` de ListItem  hoja
//	         con avatar `sm`; sin nadie, `py-6 text-center text-sm text-muted`
//	paso 2   la frase `text-base`, los `fieldset` de nombre y organización (si difieren) hoja
//	         con su leyenda `text-sm font-medium text-muted mb-1.5` y radios a `gap-1.5`,
//	         y el resultado `border-t pt-3 text-sm text-muted`
//	grupos   cajas `rounded-md border p-3`: hasta tres avatares `sm` solapados         hoja
//	         (`-space-x-2`), los nombres y la meta, y «Combinar» `secondary sm`
var (
	// AltoMaxCandidatos es el `max-h-80` de la lista de candidatos.
	AltoMaxCandidatos = tema.Espacio(80)
	// HuecoCombinar es el `gap-3` del paso 1 y el `gap-4` del paso 2.
	HuecoCombinar1 = tema.Espacio(3)
	HuecoCombinar2 = tema.Espacio(4)
	// MargenLeyendaFusion es el `mb-1.5` de la leyenda y HuecoRadios el `gap-1.5`.
	MargenLeyendaFusion = tema.Espacio(1.5)
	HuecoRadios         = tema.Espacio(1.5)
	// RellenoResultado es el `pt-3` del resultado.
	RellenoResultado = tema.Espacio(3)
	// SolapeAvatares es el `-space-x-2` de los avatares de un grupo.
	SolapeAvatares = tema.Espacio(2)
)

// fusion es el estado de MergeModal mientras está abierto.
type fusion struct {
	p             *Pantalla
	lienzo        fyne.Canvas
	superviviente api.Contact
	candidatos    []api.Contact
	d             *ui.Dialogo
	cuerpo        *fyne.Container
	pie           *fyne.Container
	buscador      *ui.Entrada

	perdedor                    *api.Contact
	nombrePerdedor, orgPerdedor bool
	ocupado                     bool

	// alCombinar hace la fusión con lo elegido y llama a `listo` con el resultado.
	alCombinar func(perdedor api.Contact, nombrePerdedor, orgPerdedor bool, listo func(error))
}

func (p *Pantalla) nuevaFusion(lienzo fyne.Canvas, superviviente api.Contact, candidatos []api.Contact) *fusion {
	f := &fusion{p: p, lienzo: lienzo, superviviente: superviviente, candidatos: candidatos}
	f.cuerpo = container.New(ui.Columna{})
	f.pie = container.New(ui.Fila{Hueco: ui.HuecoPieDialogo})
	f.buscador = ui.NuevaEntrada(p.t("merge.searchPlaceholder", nil))
	f.buscador.Icono = iconos.Buscar
	f.buscador.AlCambiar(func(string) { f.pintar() })
	f.d = ui.NuevoDialogo(p.t("merge.selectTitle", nil), f.cuerpo).Con(ui.DialogoMD, f.pie)
	f.d.RotuloCerrar = p.t("ui.close", nil)
	f.d.AlCerrar = func() {
		if !f.ocupado {
			f.d.Cerrar()
		}
	}
	f.pintar()
	return f
}

// opciones es lo que deja el buscador: por nombre o por correo.
func (f *fusion) opciones() []api.Contact {
	q := buscar.Plegar(strings.TrimSpace(f.buscador.Texto()))
	if q == "" {
		return f.candidatos
	}
	var fuera []api.Contact
	for _, c := range f.candidatos {
		if strings.Contains(buscar.Plegar(c.DisplayName()), q) {
			fuera = append(fuera, c)
			continue
		}
		for _, e := range c.Emails {
			if strings.Contains(buscar.Plegar(e.Email), q) {
				fuera = append(fuera, c)
				break
			}
		}
	}
	return fuera
}

func (f *fusion) pintar() {
	if f.perdedor == nil {
		f.pintarEleccion()
	} else {
		f.pintarConfirmacion()
	}
	f.cuerpo.Refresh()
	f.pie.Refresh()
	recolocar(f.lienzo, f.d)
}

// pintarEleccion es el paso 1: elegir con quién.
func (f *fusion) pintarEleccion() {
	p := f.p
	f.d.Titulo = p.t("merge.selectTitle", nil)
	f.d.Refresh()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var lista fyne.CanvasObject
	if ops := f.opciones(); len(ops) == 0 {
		clave := "merge.noMatches"
		if len(f.candidatos) == 0 {
			clave = "merge.noCandidates"
		}
		lista = avisoCentrado(p.t(clave, nil))
	} else {
		filas := make([]fyne.CanvasObject, 0, len(ops))
		for _, c := range ops {
			contacto := c
			fila := ui.NuevaFilaLista(c.DisplayName(), subtituloDe(c))
			fila.Delantero = ui.NuevoAvatar(c.DisplayName()).Con(ui.AvatarSM)
			fila.Fondo = "surface-raised"
			fila.AlPulsar = func() {
				f.perdedor = &contacto
				f.nombrePerdedor, f.orgPerdedor = false, false
				f.pintar()
			}
			filas = append(filas, fila)
		}
		desplaza := container.NewVScroll(container.New(ui.Columna{}, filas...))
		lista = enCajaConBorde(container.New(&altoMaximo{Max: AltoMaxCandidatos}, desplaza), 0, v)
	}
	f.cuerpo.Objects = []fyne.CanvasObject{container.New(ui.Columna{Hueco: HuecoCombinar1}, f.buscador, lista)}
	cancelar := ui.NuevoBoton(p.t("common.cancel", nil), func() { f.d.Descartar() }).Con(ui.Secundario, ui.TallaMD)
	f.pie.Objects = []fyne.CanvasObject{cancelar}
}

// pintarConfirmacion es el paso 2: qué nombre y qué organización se quedan, y en qué
// queda la ficha.
func (f *fusion) pintarConfirmacion() {
	p, s, l := f.p, f.superviviente, *f.perdedor
	f.d.Titulo = p.t("merge.confirmTitle", nil)
	f.d.Refresh()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := anchoCuerpo(f.lienzo.Size(), 560)
	hijos := []fyne.CanvasObject{
		ui.NuevoParrafo(p.t("merge.explain", i18n.Vars{"loser": l.DisplayName(), "survivor": s.DisplayName()}),
			tema.TextoBase, ui.Normal, "text", v).A(ancho),
	}
	eleccion := func(leyenda, suyo, otro string, delOtro *bool) fyne.CanvasObject {
		radio := func(texto string, este bool) *ui.Eleccion {
			r := ui.NuevoRadio(texto, func(bool) { *delOtro = este; f.pintar() })
			r.Marcada = *delOtro == este
			return r
		}
		return container.New(ui.Columna{Hueco: MargenLeyendaFusion},
			ui.NuevoRotulo(p.t(leyenda, nil), tema.TextoSM, ui.Medio, "text-muted"),
			container.New(ui.Columna{Hueco: HuecoRadios}, radio(suyo, false), radio(otro, true)))
	}
	if strings.TrimSpace(s.DisplayName()) != strings.TrimSpace(l.DisplayName()) {
		hijos = append(hijos, eleccion("merge.keepName", s.DisplayName(), l.DisplayName(), &f.nombrePerdedor))
	}
	if s.Organization != "" && l.Organization != "" && s.Organization != l.Organization {
		hijos = append(hijos, eleccion("merge.keepOrganization", s.Organization, l.Organization, &f.orgPerdedor))
	}
	in := Fusion(s, l, f.nombrePerdedor, f.orgPerdedor)
	resultado := p.t("merge.result", i18n.Vars{
		"emails": p.t("merge.resultEmails", i18n.Vars{"count": len(in.Emails)}),
		"phones": p.t("merge.resultPhones", i18n.Vars{"count": len(in.Phones)}),
	})
	hijos = append(hijos, container.New(&filetArriba{}, container.New(ui.Margen{Arr: RellenoResultado},
		ui.NuevoParrafo(resultado, tema.TextoSM, ui.Normal, "text-muted", v).A(ancho)),
		filete(tema.Color("border", v), 1)))
	f.cuerpo.Objects = []fyne.CanvasObject{container.New(ui.Columna{Hueco: HuecoCombinar2}, hijos...)}

	atras := ui.NuevoBoton(p.t("common.back", nil), func() { f.perdedor = nil; f.pintar() }).Con(ui.Secundario, ui.TallaMD)
	combinar := ui.NuevoBoton(p.t("merge.confirm", nil), nil).Con(ui.Primario, ui.TallaMD)
	combinar.AlPulsar = func() {
		if f.alCombinar == nil || f.ocupado {
			return
		}
		f.ocupado, combinar.Cargando, atras.Inerte = true, true, true
		combinar.Refresh()
		atras.Refresh()
		f.alCombinar(l, f.nombrePerdedor, f.orgPerdedor, func(err error) {
			f.ocupado = false
			if err == nil {
				f.d.Cerrar()
				return
			}
			combinar.Cargando, atras.Inerte = false, false
			combinar.Refresh()
			atras.Refresh()
		})
	}
	f.pie.Objects = []fyne.CanvasObject{atras, combinar}
}

// ---- Posibles duplicados ----

// dialogoDuplicados es DuplicatesModal: cada grupo con su «Combinar», que abre la
// fusión acotada a ese grupo (superviviente el primero).
func (p *Pantalla) dialogoDuplicados(lienzo fyne.Canvas, grupos [][]api.Contact, alElegir func([]api.Contact)) *ui.Dialogo {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var d *ui.Dialogo
	var cuerpo fyne.CanvasObject
	if len(grupos) == 0 {
		cuerpo = avisoCentrado(p.t("duplicates.empty", nil))
	} else {
		cajas := make([]fyne.CanvasObject, 0, len(grupos))
		for _, g := range grupos {
			grupo := g
			nombres := make([]string, len(g))
			for i, c := range g {
				nombres[i] = c.DisplayName()
			}
			meta := p.t("duplicates.cards", i18n.Vars{"count": len(g)})
			switch {
			case len(g[0].Emails) > 0:
				meta = g[0].Emails[0].Email
			case len(g[0].Phones) > 0:
				meta = g[0].Phones[0].Phone
			case g[0].Organization != "":
				meta = g[0].Organization
			}
			combinar := ui.NuevoBoton(p.t("duplicates.merge", nil), func() {
				d.Cerrar()
				alElegir(grupo)
			}).Con(ui.Secundario, ui.TallaSM)
			fila := container.New(filaEntre{}, grupoDePersonas(g, strings.Join(nombres, ", "), meta, v), combinar)
			cajas = append(cajas, enCajaConBorde(fila, RellenoCajaPapelera, v))
		}
		cuerpo = container.New(ui.Columna{Hueco: HuecoCombinar1},
			ui.NuevoParrafo(p.t("duplicates.description", nil), tema.TextoSM, ui.Normal, "text-muted", v).A(anchoCuerpo(lienzo.Size(), 560)),
			container.New(ui.Columna{Hueco: HuecoPapelera}, cajas...))
	}
	cerrar := ui.NuevoBoton(p.t("common.close", nil), nil).Con(ui.Secundario, ui.TallaMD)
	d = ui.NuevoDialogo(p.t("duplicates.title", nil), cuerpo).Con(ui.DialogoMD, cerrar)
	cerrar.AlPulsar = d.Cerrar
	d.RotuloCerrar = p.t("ui.close", nil)
	return d
}

// grupoDePersonas son los avatares solapados de un grupo (hasta tres) con los nombres y
// la meta al lado.
func grupoDePersonas(g []api.Contact, nombres, meta string, v tema.Variante) fyne.CanvasObject {
	n := min(len(g), 3)
	avatares := make([]fyne.CanvasObject, n)
	for i := range n {
		avatares[i] = ui.NuevoAvatar(g[i].DisplayName()).Con(ui.AvatarSM)
	}
	return personaCon(container.New(solapados{}, avatares...), nombres, meta, v)
}

// solapados es el `flex -space-x-2`: cada avatar monta 7 px sobre el anterior.
type solapados struct{}

func (solapados) MinSize(objs []fyne.CanvasObject) fyne.Size {
	av := ui.MedidaAvatar(ui.AvatarSM)
	if len(objs) == 0 {
		return fyne.Size{}
	}
	return fyne.NewSize(float32(len(objs))*av.Width-float32(len(objs)-1)*SolapeAvatares, av.Height)
}

func (solapados) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	av := ui.MedidaAvatar(ui.AvatarSM)
	for i, o := range objs {
		o.Move(fyne.NewPos(float32(i)*(av.Width-SolapeAvatares), (t.Height-av.Height)/2))
		o.Resize(av)
	}
}

// ---- En la App ----

// candidatosDe son los de la libreta menos él.
func (a *App) candidatosDe(c api.Contact) []api.Contact {
	fuera := make([]api.Contact, 0, len(a.todos))
	for _, x := range a.todos {
		if x.ID != c.ID {
			fuera = append(fuera, x)
		}
	}
	return fuera
}

// abrirFusion abre MergeModal con esos candidatos.
func (a *App) abrirFusion(superviviente api.Contact, candidatos []api.Contact) *fusion {
	lienzo := a.lienzo()
	if lienzo == nil || a.svc == nil {
		return nil
	}
	f := a.Pantalla.nuevaFusion(lienzo, superviviente, candidatos)
	f.alCombinar = func(perdedor api.Contact, np, op bool, listo func(error)) {
		a.enFondo(func() { a.combinar(superviviente, perdedor, np, op, listo) })
	}
	a.mostrarEscritura("combinar", f.d, lienzo)
	return f
}

// combinar es `performMerge`, en este orden y nunca al revés:
//
//  1. guardar el superviviente (PUT con su etag); si falla —un 412 porque otro
//     dispositivo lo ha cambiado—, se vuelve a leer la libreta y se reintenta UNA vez
//     con los datos frescos de los dos;
//  2. pasar las etiquetas del perdedor al superviviente ANTES de borrar, para no dejar
//     pertenencias huérfanas;
//  3. borrar el perdedor.
//
// Si falla 2 o 3, el perdedor queda duplicado —estado seguro, sin perder nada— y se
// avisa de fusión incompleta. Si falla 1 no se ha tocado nada. Pase lo que pase, la
// libreta y las etiquetas se vuelven a cargar. Bloquea: en segundo plano.
func (a *App) combinar(s, l api.Contact, nombrePerdedor, orgPerdedor bool, listo func(error)) {
	var libreta api.AddressBook
	var etiquetas []api.ContactGroup
	fyne.DoAndWait(func() {
		if a.Pantalla.Activa != nil {
			libreta = *a.Pantalla.Activa
		}
		etiquetas = a.Pantalla.Etiquetas
	})
	ctx, fin := context.WithTimeout(context.Background(), 3*a.Plazo)
	defer fin()
	in := Fusion(s, l, nombrePerdedor, orgPerdedor)
	if _, err := a.svc.Update(ctx, s.ID, in, s.ETag); err != nil {
		in, err = a.reintentarFusion(ctx, libreta.ID, s, l, in, nombrePerdedor, orgPerdedor)
		if err != nil {
			a.finFusion(libreta, s, l, in, err, false, listo)
			return
		}
	}
	parcial := a.repuntar(ctx, etiquetas, s, l) != nil || a.svc.Delete(ctx, l.ID) != nil
	a.finFusion(libreta, s, l, in, nil, parcial, listo)
}

// reintentarFusion es el segundo intento de `performMerge`: la libreta otra vez, y la
// fusión hecha sobre los datos frescos de los dos, con los correos y teléfonos del
// primer intento también (unidos, sin repetir).
func (a *App) reintentarFusion(ctx context.Context, libreta string, s, l api.Contact, antes api.ContactInput,
	nombrePerdedor, orgPerdedor bool,
) (api.ContactInput, error) {
	frescos, err := a.svc.InBook(ctx, libreta)
	if err != nil {
		return antes, err
	}
	fs := buscarPorID(frescos, s.ID)
	if fs == nil {
		return antes, errSinSuperviviente
	}
	if fl := buscarPorID(frescos, l.ID); fl != nil {
		l = *fl
	}
	in := Fusion(*fs, l, nombrePerdedor, orgPerdedor)
	in.Emails = unirCorreos(antes.Emails, in.Emails)
	in.Phones = unirTelefonos(antes.Phones, in.Phones)
	_, err = a.svc.Update(ctx, fs.ID, in, fs.ETag)
	return in, err
}

// finFusion avisa del resultado en el hilo de la interfaz y vuelve a cargar.
func (a *App) finFusion(libreta api.AddressBook, _, l api.Contact, in api.ContactInput, err error, parcial bool,
	listo func(error),
) {
	fyne.Do(func() {
		p := a.Pantalla
		vars := i18n.Vars{"loser": l.DisplayName(), "survivor": in.FormattedName}
		switch {
		case err != nil:
			slog.Warn("contactos: no se pudo combinar", "error", err)
			a.avisar(ui.AvisoPeligro, "merge.failed", "merge.failedHint")
		case parcial:
			a.avisos.Notificar(ui.NuevoAviso(ui.AvisoAdvertencia, p.t("merge.partial", nil), p.t("merge.partialDescription", vars)))
		default:
			a.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, p.t("merge.done", nil), p.t("merge.doneDescription", vars)))
		}
		listo(err)
		a.recargar(libreta)
		a.enFondo(a.cargarEtiquetas)
	})
}

// errSinSuperviviente es que, al releer para reintentar, el superviviente ya no está.
var errSinSuperviviente = errors.New("contactos: el contacto superviviente ya no existe")

// repuntar pasa al superviviente las etiquetas del perdedor.
func (a *App) repuntar(ctx context.Context, etiquetas []api.ContactGroup, s, l api.Contact) error {
	for _, g := range etiquetas {
		miembros := make([]string, 0, len(g.Members))
		tenia, ya := false, false
		for _, u := range g.Members {
			switch u {
			case l.UID:
				tenia = true
				continue
			case s.UID:
				ya = true
			}
			miembros = append(miembros, u)
		}
		if !tenia {
			continue
		}
		if !ya {
			miembros = append(miembros, s.UID)
		}
		if _, err := a.svc.RenameGroup(ctx, g.ID, g.Name, miembros); err != nil {
			return err
		}
	}
	return nil
}

// verDuplicados abre «Posibles duplicados».
func (a *App) verDuplicados() *ui.Dialogo {
	lienzo := a.lienzo()
	if lienzo == nil {
		return nil
	}
	d := a.Pantalla.dialogoDuplicados(lienzo, GruposDuplicados(a.todos), func(g []api.Contact) {
		a.abrirFusion(g[0], g[1:])
	})
	a.mostrarEscritura("duplicados", d, lienzo)
	return d
}
