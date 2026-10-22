// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"image/color"
	"net/http"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Versiones
//
// `VersionsDialog.tsx`: el historial de un fichero, con qué es una versión y cuánto dura
// ANTES de la lista, la descarga de cualquiera y «Restaurar» con su confirmación EN LA
// FILA —no un diálogo encima de otro— que dice qué pasa a estar vivo y qué NO se pierde.
// Restaurar no es destructivo: lo que había pasa a ser una versión más.
//
// ponytail: la fecha es la de la lista con su hora («06 sept 2026, 9:00»); la web pide la
// hora también a dos cifras («09:00»), que `i18n` no tiene todavía. Y el autor es el `sub`
// tal cual, que es lo que la web enseña: el servicio no manda el nombre.

// dialogoVersiones es el estado del diálogo.
type dialogoVersiones struct {
	p  *Pantalla
	el Elemento
	d  *ui.Dialogo

	versiones  []api.DriveVersion
	fallo      bool
	cursor     string
	cargando   bool
	confirmar  int
	restaurada int

	cuerpo *fyne.Container
	// anchoCuerpo y anchoPanel son los últimos anchos colocados (ver pilaSeparada).
	anchoCuerpo, anchoPanel float32
}

// pedirVersiones abre el historial de un fichero y lo devuelve (para el guardián).
func (p *Pantalla) pedirVersiones(el Elemento) *dialogoVersiones {
	if p.win == nil || p.svc == nil || el.Carpeta || el.ID == "" {
		return nil
	}
	dv := &dialogoVersiones{p: p, el: el, cuerpo: container.NewStack()}
	dv.anchoCuerpo = AnchoCuerpoDialogo(ui.DialogoMD, p.explor.Ventana)
	dv.anchoPanel = dv.anchoCuerpo - 2*rellenoXVersion - 2*(tema.Espacio(3)+tema.Borde)
	cerrar := ui.NuevoBoton(p.tr.T("versions.close", nil), func() { dv.d.Cerrar() }).Con(ui.Primario, ui.TallaMD)
	dv.d = ui.NuevoDialogo(p.tr.T("versions.title", i18n.Vars{"name": el.Nombre}), dv.cuerpo).Con(ui.DialogoMD, cerrar)
	dv.d.RotuloCerrar = p.tr.T("ui.close", nil)
	dv.pintar()
	dv.d.Mostrar(p.win.Canvas())
	dv.p.enSegundoPlano(func() { dv.cargar("") })
	return dv
}

func (dv *dialogoVersiones) t(k string, v ...i18n.Vars) string {
	var vars i18n.Vars
	if len(v) > 0 {
		vars = v[0]
	}
	return dv.p.tr.T(k, vars)
}

// cargar pide una página. Un fallo en la primera deja el diálogo en error; en las
// siguientes, se avisa y se conserva lo que había.
func (dv *dialogoVersiones) cargar(cursor string) {
	ctx, cancelar := dv.p.ctx()
	defer cancelar()
	lista, sig, err := dv.p.svc.Versions(ctx, dv.el.ID, cursor)
	enHilo(func() {
		dv.cargando = false
		switch {
		case err != nil && cursor == "":
			dv.fallo = true
		case err != nil:
			dv.p.avisar(ui.AvisoPeligro, dv.t("versions.loadMoreError"), "")
		case cursor == "":
			dv.versiones, dv.cursor = lista, sig
		default:
			dv.versiones, dv.cursor = append(dv.versiones, lista...), sig
		}
		if dv.versiones == nil && err == nil {
			dv.versiones = []api.DriveVersion{}
		}
		dv.pintar()
	})
}

// actual es el número de la versión viva, o 0 si no se sabe.
func (dv *dialogoVersiones) actual() int {
	for _, v := range dv.versiones {
		if v.IsCurrent {
			return v.Version
		}
	}
	return 0
}

// restaurar hace actual una versión archivada. 403 es que no se tiene permiso.
func (dv *dialogoVersiones) restaurar(v api.DriveVersion) {
	if dv.restaurada != 0 {
		return
	}
	dv.restaurada = v.Version
	dv.pintar()
	dv.p.enSegundoPlano(func() {
		ctx, cancelar := dv.p.ctx()
		defer cancelar()
		err := dv.p.svc.RestoreVersion(ctx, dv.el.ID, v.Version)
		enHilo(func() {
			dv.restaurada = 0
			if err != nil {
				clave := "versions.restoreFailed"
				if codigo(err) == http.StatusForbidden {
					clave = "versions.restoreDenied"
				}
				dv.p.avisar(ui.AvisoPeligro, dv.t(clave), "")
				dv.pintar()
				return
			}
			dv.p.avisar(ui.AvisoExito, dv.t("versions.restored", i18n.Vars{"name": dv.el.Nombre, "version": v.Version}), "")
			dv.d.Cerrar()
		})
		if err == nil {
			dv.p.recargarVista()
		}
	})
}

// descargar baja una versión; la actual es el contenido de siempre.
func (dv *dialogoVersiones) descargar(v api.DriveVersion) {
	dv.p.enSegundoPlano(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancelar()
		ruta, err := DescargarVersion(ctx, dv.p.svc, dv.el, v)
		if err != nil {
			dv.p.avisar(ui.AvisoPeligro, dv.t("files.error.network"), motivo(err))
			return
		}
		dv.p.avisar(ui.AvisoExito, dv.el.Nombre, ruta)
	})
}

// DescargarVersion guarda una versión en la carpeta de descargas, con el nombre ACTUAL
// del fichero, sin pisar nada.
func DescargarVersion(ctx context.Context, svc *api.Drive, el Elemento, v api.DriveVersion) (string, error) {
	if v.IsCurrent {
		return Descargar(ctx, svc, el, nil)
	}
	cuerpo, err := svc.DownloadVersion(ctx, el.ID, v.Version)
	if err != nil {
		return "", err
	}
	defer func() { _ = cuerpo.Close() }()
	carpeta, err := ficheros.Carpeta()
	if err != nil {
		return "", err
	}
	return ficheros.Guardar(carpeta, el.Nombre, cuerpo)
}

// AltoCargaVersiones es el `h-40` de la espera y del error.
const AltoCargaVersiones float32 = 40 * 0.25 * tema.Rem

// pintar rehace el cuerpo con el estado de ahora.
func (dv *dialogoVersiones) pintar() {
	v := dv.p.variante()
	var dentro fyne.CanvasObject
	switch {
	case dv.versiones == nil && !dv.fallo:
		r := ui.NuevaRueda(22, "text", v)
		r.Rotulo = dv.t("versions.loading")
		dentro = container.New(&altoFijo{alto: AltoCargaVersiones}, container.NewCenter(r))
	case dv.fallo:
		texto := lineaTexto(ui.Texto(dv.t("versions.loadError"), tema.TextoBase, ui.Normal, tema.Color("text-muted", v)), tema.TextoBase)
		reintentar := ui.NuevoBoton(dv.t("versions.retry"), func() {
			dv.fallo, dv.versiones = false, nil
			dv.pintar()
			dv.p.enSegundoPlano(func() { dv.cargar("") })
		}).Con(ui.Secundario, ui.TallaSM)
		dentro = container.New(&altoFijo{alto: AltoCargaVersiones},
			container.NewCenter(container.New(ui.Columna{Hueco: tema.Espacio(2), Encaja: true}, texto, reintentar)))
	default:
		dentro = dv.lista(v)
	}
	dv.cuerpo.Objects = []fyne.CanvasObject{dentro}
	dv.cuerpo.Refresh()
	dv.d.Recolocar()
}

func (dv *dialogoVersiones) lista(v tema.Variante) fyne.CanvasObject {
	bloques := []fyne.CanvasObject{ui.NuevaAyudaEnLinea(dv.t("versions.helpBody")).ConTitulo(dv.t("versions.helpTitle"))}
	separas := []float32{0}
	if len(dv.versiones) <= 1 {
		x := ui.NuevoVacio(dv.t("versions.emptyTitle"), dv.t("versions.emptyBody"))
		x.Ilustracion = glifoCon(glifoReloj, 36, tema.Color("text-subtle", v)).Objeto()
		bloques, separas = append(bloques, x), append(separas, tema.Espacio(3))
	}
	filas := make([]fyne.CanvasObject, 0, len(dv.versiones))
	for i, ver := range dv.versiones {
		filas = append(filas, dv.fila(ver, i > 0, v))
	}
	borde := ui.NuevaCaja(color.NRGBA{}, tema.Color("border", v), tema.RadioMD)
	marco := container.New(&conMargen{m: tema.Borde}, append(borde.Objetos(), container.New(ui.Columna{}, filas...))...)
	bloques, separas = append(bloques, marco), append(separas, tema.Espacio(3))
	if dv.cursor != "" {
		mas := ui.NuevoBoton(dv.t("versions.loadMore"), func() {
			if dv.cargando {
				return
			}
			dv.cargando = true
			dv.pintar()
			dv.p.enSegundoPlano(func() { dv.cargar(dv.cursor) })
		}).Con(ui.Fantasma, ui.TallaSM)
		mas.Cargando = dv.cargando
		bloques, separas = append(bloques, container.NewCenter(mas)), append(separas, tema.Espacio(3))
	}
	return container.New(&pilaSeparada{separas: separas, ancho: &dv.anchoCuerpo}, bloques...)
}

// fila es una versión: la tesela del reloj, «Versión n» con su insignia si es la
// actual, la meta y, a la derecha, descargar y «Restaurar»; debajo, la confirmación si
// se ha pedido.
func (dv *dialogoVersiones) fila(ver api.DriveVersion, filete bool, v tema.Variante) fyne.CanvasObject {
	fecha := "—"
	if !ver.CreatedAt.IsZero() {
		fecha = fechaHora(dv.p.tr.Locale(), ver.CreatedAt)
	}
	resto := i18n.FormatBytes(dv.p.tr.Locale(), float64(ver.SizeBytes))
	if ver.AuthorSub != "" {
		resto = ver.AuthorSub + " · " + resto
	}
	// En el teléfono, la fecha y debajo autor y tamaño (como la web): en una línea
	// truncada a 137 se perdían los dos.
	telefono := dv.p.explor.Ventana > 0 && dv.p.explor.Ventana < ui.Estrecho
	meta := fecha + " · " + resto
	if telefono {
		meta = fecha
	}

	bajar := nuevoBotonTexto("", tema.TextoSM, ui.Normal, func() { dv.descargar(ver) })
	bajar.cuerpo, bajar.ladoIcono, bajar.margenX, bajar.alto = glifoDescargar, 16, (LadoBotonFila-16)/2, LadoBotonFila
	if ui.EsTactil() { // `pointer-coarse:h-11 w-11`
		bajar.margenX, bajar.alto = (ui.SueloTactilBarra-16)/2, ui.SueloTactilBarra
	}
	bajar.Nombre = dv.t("versions.downloadAria", i18n.Vars{"version": ver.Version})
	acciones := []fyne.CanvasObject{bajar}
	if !ver.IsCurrent && dv.confirmar != ver.Version {
		r := ui.NuevoBoton(dv.t("versions.restore"), func() { dv.confirmar = ver.Version; dv.pintar() }).Con(ui.Secundario, ui.TallaSM)
		r.Inerte = dv.restaurada != 0
		acciones = append(acciones, r)
	}

	f := &filaVersion{
		tesela:   ui.NuevaCaja(tema.Color("bg-subtle", v), color.NRGBA{}, tema.RadioMD),
		reloj:    glifoCon(glifoReloj, 16, tema.Color("text-muted", v)),
		nombre:   ui.Texto(dv.t("versions.version", i18n.Vars{"version": ver.Version}), tema.TextoBase, ui.Medio, tema.Color("text", v)),
		meta:     ui.Texto(meta, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		acciones: container.New(ui.Fila{Hueco: tema.Espacio(1)}, acciones...),
		linea:    canvas.NewRectangle(conAlfaBorde(v)),
	}
	f.linea.Hidden = !filete
	hijos := append(f.tesela.Objetos(), f.linea, f.reloj.Objeto(), f.nombre, f.meta, f.acciones)
	if telefono {
		f.meta2 = ui.Texto(resto, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
		hijos = append(hijos, f.meta2)
	}
	if ver.IsCurrent {
		f.actual = ui.Texto(dv.t("versions.current"), tema.TextoXS, ui.SemiNegrita, tema.Color("primary-text", v))
		f.fondoActual = ui.NuevaCaja(tema.Color("selected", v), color.NRGBA{}, tema.RadioPill)
		hijos = append(hijos, f.fondoActual.Objetos()...)
		hijos = append(hijos, f.actual)
	}
	if dv.confirmar == ver.Version {
		f.confirmacion = dv.confirmacion(ver, v)
		hijos = append(hijos, f.confirmacion)
	}
	return container.New(f, hijos...)
}

// confirmacion es el panel de la fila: `border bg-bg-subtle rounded-md p-3 mt-2.5`, las
// dos frases enteras del catálogo y los dos botones.
func (dv *dialogoVersiones) confirmacion(ver api.DriveVersion, v tema.Variante) fyne.CanvasObject {
	que := dv.t("versions.confirm", i18n.Vars{"name": dv.el.Nombre, "version": ver.Version})
	if !ver.CreatedAt.IsZero() {
		que = dv.t("versions.confirmDated", i18n.Vars{
			"name": dv.el.Nombre, "version": ver.Version, "date": fechaHora(dv.p.tr.Locale(), ver.CreatedAt),
		})
	}
	queda := dv.t("versions.confirmKeep")
	if a := dv.actual(); a != 0 {
		queda = dv.t("versions.confirmKeepCurrent", i18n.Vars{"version": a})
	}
	relajado := tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}
	uno := ui.NuevoParrafo(que, relajado, ui.Normal, "text", v)
	dos := ui.NuevoParrafo(queda, relajado, ui.Normal, "text-muted", v)
	si := ui.NuevoBoton(dv.t("versions.confirmCta"), func() { dv.restaurar(ver) }).Con(ui.Primario, ui.TallaSM)
	si.Cargando = dv.restaurada == ver.Version
	no := ui.NuevoBoton(dv.t("versions.cancel"), func() { dv.confirmar = 0; dv.pintar() }).Con(ui.Fantasma, ui.TallaSM)
	no.Inerte = dv.restaurada != 0
	botones := container.New(ui.Fila{Hueco: tema.Espacio(2)}, si, no)
	dentro := container.New(&pilaSeparada{separas: []float32{0, tema.Espacio(1), tema.Espacio(3)}, ancho: &dv.anchoPanel}, uno, dos, botones)
	caja := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD)
	return container.New(&conMargen{m: tema.Espacio(3) + tema.Borde}, append(caja.Objetos(), dentro)...)
}

// filaVersion coloca una versión: `px-3 py-2.5 gap-3`, y la confirmación debajo.
type filaVersion struct {
	tesela, fondoActual  *ui.Caja
	reloj                *iconos.Dibujo
	nombre, meta, actual *canvas.Text
	// meta2 es la segunda línea de la meta en el teléfono (autor y tamaño); nil en el
	// escritorio, donde va todo en una.
	meta2        *canvas.Text
	acciones     fyne.CanvasObject
	confirmacion fyne.CanvasObject
	linea        *canvas.Rectangle
}

var _ fyne.Layout = (*filaVersion)(nil)

// Medidas de la fila de una versión.
const (
	rellenoXVersion  float32 = 3 * 0.25 * tema.Rem
	rellenoYVersion  float32 = 2.5 * 0.25 * tema.Rem
	ladoTeselaVers   float32 = 9 * 0.25 * tema.Rem
	separaConfirmVer float32 = 2.5 * 0.25 * tema.Rem
)

// altoTexto es el nombre y la meta, en una línea o en dos.
func (f *filaVersion) altoTexto() float32 {
	alto := tema.TextoBase.Linea + SeparaMeta + tema.TextoXS.Linea
	if f.meta2 != nil {
		alto += tema.TextoXS.Linea
	}
	return alto
}

func (f *filaVersion) altoLinea() float32 {
	return max(ladoTeselaVers, f.altoTexto(), f.acciones.MinSize().Height)
}

func (f *filaVersion) MinSize([]fyne.CanvasObject) fyne.Size {
	alto := 2*rellenoYVersion + f.altoLinea()
	if f.confirmacion != nil {
		alto += separaConfirmVer + f.confirmacion.MinSize().Height
	}
	return fyne.NewSize(0, alto)
}

func (f *filaVersion) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	f.linea.Move(fyne.NewPos(0, 0))
	f.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
	linea := f.altoLinea()
	centro := func(h float32) float32 { return rellenoYVersion + (linea-h)/2 }
	x := rellenoXVersion
	ui.MoverCaja(f.tesela, fyne.NewPos(x, centro(ladoTeselaVers)), fyne.NewSize(ladoTeselaVers, ladoTeselaVers))
	f.reloj.Colocar(x+(ladoTeselaVers-16)/2, centro(16), 16)
	x += ladoTeselaVers + HuecoFilaPlana
	m := f.acciones.MinSize()
	derecha := t.Width - rellenoXVersion - m.Width
	f.acciones.Move(fyne.NewPos(derecha, centro(m.Height)))
	f.acciones.Resize(m)
	cabe := max(derecha-HuecoFilaPlana-x, 0)
	y := centro(f.altoTexto())
	ancho := ui.Truncar(f.nombre, f.nombre.Text, tema.TextoBase, ui.Medio, cabe)
	ui.ColocarTexto(f.nombre, tema.TextoBase, x, y)
	if f.actual != nil {
		// `rounded-full px-2 py-0.5 text-xs font-semibold` con `gap-2` tras el nombre.
		w := ui.Medir(f.actual.Text, tema.TextoXS, ui.SemiNegrita).Width + 2*tema.Espacio(2)
		h := tema.TextoXS.Linea + 2*tema.Espacio(0.5)
		ox := x + ancho + tema.Espacio(2)
		oy := y + (tema.TextoBase.Linea-h)/2
		ui.MoverCaja(f.fondoActual, fyne.NewPos(ox, oy), fyne.NewSize(w, h))
		ui.ColocarTexto(f.actual, tema.TextoXS, ox+tema.Espacio(2), oy+tema.Espacio(0.5))
	}
	ui.Truncar(f.meta, f.meta.Text, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(f.meta, tema.TextoXS, x, y+tema.TextoBase.Linea+SeparaMeta)
	if f.meta2 != nil {
		ui.Truncar(f.meta2, f.meta2.Text, tema.TextoXS, ui.Normal, cabe)
		ui.ColocarTexto(f.meta2, tema.TextoXS, x, y+tema.TextoBase.Linea+SeparaMeta+tema.TextoXS.Linea)
	}
	if f.confirmacion != nil {
		yc := 2*rellenoYVersion + linea - rellenoYVersion + separaConfirmVer
		f.confirmacion.Move(fyne.NewPos(rellenoXVersion, yc))
		f.confirmacion.Resize(fyne.NewSize(max(t.Width-2*rellenoXVersion, 0), f.confirmacion.MinSize().Height))
	}
}

// fechaHora es la fecha de la lista y la hora corta del idioma.
func fechaHora(l i18n.Locale, t time.Time) string { return Fecha(l, t) + ", " + i18n.FormatTime(l, t) }
