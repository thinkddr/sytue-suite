// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// «Compartir calendario»: ShareCalendarModal.tsx, solo para el DUEÑO y desde el «⋮» de
// un calendario propio que no sea una suscripción. Se comparte por correo y el nivel se
// elige por lo que PERMITE HACER —ver, ver y editar, llevar tu agenda—, con la
// consecuencia delante; debajo, con quién está compartido y «revocar».
//
// La web escribe a mano en español el título, la leyenda, «Cargando…» y el nombre del
// botón de revocar; aquí salen de sus claves del catálogo (`share.title`,
// `share.legend`, `common.loading`, `share.revoke`), que dicen lo mismo en español y
// además existen en los otros 25 idiomas.

// permisosCompartir es PERMISSIONS: el valor, sus dos textos y su icono.
var permisosCompartir = []struct {
	valor, rotulo, pista string
	icono                iconos.Icono
}{
	{api.PermisoLeer, "share.readerLabel", "share.readerHint", iconos.Usuario},
	{api.PermisoEditar, "share.writerLabel", "share.writerHint", iconos.Usuarios},
	{api.PermisoLlevar, "share.manageLabel", "share.manageHint", iconos.Candado},
}

// insigniaPermiso es PERMISSION_BADGE.
var insigniaCompartido = map[string]string{
	api.PermisoLeer: "share.badgeReader", api.PermisoEditar: "share.badgeWriter", api.PermisoLlevar: "share.badgeManage",
}

// compartir abre el diálogo de un calendario propio.
func (a *App) compartir(c api.Calendar) {
	lienzo := a.lienzo()
	if lienzo == nil || c.ID == "" || a.P.SinConexion || a.Cal == nil {
		return
	}
	p := a.P
	cuerpo := container.New(ui.Columna{})
	correo := ui.NuevaEntrada(p.t("share.emailPlaceholder", nil))
	var (
		lista     []api.CalendarShare
		cargando  = true
		permiso   = api.PermisoLeer
		errCorreo string
		ocupado   bool
		pintar    func()
	)
	recolocar := func() {
		if arriba := lienzo.Overlays().Top(); arriba != nil {
			arriba.Refresh()
		}
	}
	revocar := func(s api.CalendarShare) {
		a.escribir(func(ctx context.Context) error { return a.Cal.RevokeShare(ctx, c.ID, s.ID) }, func(err error) {
			if err != nil {
				a.avisar(ui.AvisoPeligro, "toast.revokeFailed")
				return
			}
			resto := lista[:0:0]
			for _, x := range lista {
				if x.ID != s.ID {
					resto = append(resto, x)
				}
			}
			lista = resto
			a.avisar(ui.AvisoExito, "toast.shareRevoked")
			pintar()
		})
	}
	enviar := func() {
		dir := strings.TrimSpace(correo.Texto())
		if !correoValido.MatchString(dir) {
			errCorreo, correo.Invalida = p.t("share.emailInvalid", nil), true
			pintar()
			lienzo.Focus(correo)
			return
		}
		ocupado = true
		pintar()
		var nueva []api.CalendarShare
		a.escribir(func(ctx context.Context) error {
			if err := a.Cal.Share(ctx, c.ID, dir, permiso); err != nil {
				return err
			}
			var err error
			nueva, err = a.Cal.Shares(ctx, c.ID)
			return err
		}, func(err error) {
			ocupado = false
			if err != nil {
				a.avisar(ui.AvisoPeligro, "toast.shareFailed")
				pintar()
				return
			}
			lista, permiso = nueva, api.PermisoLeer
			correo.PonerTexto("")
			a.avisar(ui.AvisoExito, "toast.calendarShared")
			pintar()
		})
	}
	correo.AlCambiar(func(string) {
		if errCorreo != "" {
			errCorreo, correo.Invalida = "", false
			pintar()
		}
	})
	correo.AlTeclear = intro(enviar)
	pintar = func() {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		formulario := p.formularioCompartir(anchoUtilFormulario(lienzo.Size()), correo, errCorreo, permiso, ocupado, func(valor string) { permiso = valor; pintar() }, enviar, v)
		abajo := p.quienLoVe(cargando, lista, revocar, v)
		filete := canvas.NewRectangle(tema.Color("border", v))
		cuerpo.Objects = []fyne.CanvasObject{container.New(ui.Columna{Hueco: tema.Espacio(5)},
			ui.NuevaAyudaEnLinea(p.t("share.helpBody", nil)).ConTono(ui.AyudaInfo).ConTitulo(p.t("share.whatHappens", nil)),
			container.New(ui.Columna{Hueco: tema.Espacio(4)}, formulario...),
			container.New(&conFileteArriba{Pad: tema.Espacio(4)}, filete, container.New(ui.Columna{Hueco: espacio2}, abajo...)),
		)}
		cuerpo.Refresh()
		recolocar()
	}
	pintar()
	var d *ui.Dialogo
	hecho := ui.NuevoBoton(p.t("common.done", nil), func() { d.Cerrar() }).Con(ui.Secundario, ui.TallaMD)
	d = ui.NuevoDialogo(p.t("share.title", i18n.Vars{"name": c.Title()}), cuerpo).Con(ui.DialogoMD, hecho)
	d.RotuloCerrar = p.t("common.close", nil)
	d.Foco = correo
	d.Mostrar(lienzo)
	a.lanzar(func() {
		ctx, parar := context.WithTimeout(context.Background(), plazo)
		defer parar()
		l, err := a.Cal.Shares(ctx, c.ID)
		a.enUI(func() {
			cargando = false
			if err != nil {
				a.avisar(ui.AvisoPeligro, "toast.sharesLoadFailed")
			}
			lista = l
			pintar()
		})
	})
}

// formularioCompartir es la parte de arriba del diálogo: el correo, qué podrá hacer, el
// aviso si podrá editar y «Compartir».
func (p *Pantalla) formularioCompartir(util float32, correo *ui.Entrada, errCorreo, permiso string, ocupado bool,
	elegir func(string), enviar func(), v tema.Variante,
) []fyne.CanvasObject {
	tarjetas := make([]fyne.CanvasObject, 0, 1+len(permisosCompartir))
	tarjetas = append(tarjetas, alto(rotuloFuerte(p.t("share.legend", nil)), tema.TextoSM.Linea+tema.Espacio(1)))
	for _, pc := range permisosCompartir {
		valor := pc.valor
		tj := ui.NuevaTarjetaEleccion(p.t(pc.rotulo, nil), p.t(pc.pista, nil), func() { elegir(valor) }).A(util)
		tj.Densa, tj.Icono, tj.Elegida = true, pc.icono, permiso == valor
		tarjetas = append(tarjetas, tj)
	}
	// La pista de la web (`FieldHint`) envuelve: son dos renglones a 560, y
	// `ui.PistaCampo` es de uno por diseño. El error sí es de una línea.
	campo := ui.GrupoCampo(p.t("share.recipientEmail", nil), false, correo, "", errCorreo)
	if errCorreo == "" {
		snug := tema.Escalon{Tam: tema.TextoXS.Tam, Linea: 1.375 * tema.TextoXS.Tam}
		campo.Add(ui.NuevoParrafo(p.t("share.recipient.hint", nil), snug, ui.Normal, "text-muted", v).A(util))
	}
	formulario := []fyne.CanvasObject{
		campo,
		container.New(ui.Columna{Hueco: espacio2}, tarjetas...),
	}
	if permiso != api.PermisoLeer {
		clave := "share.role.write.hint"
		if permiso == api.PermisoLlevar {
			clave = "share.role.manage.hint"
		}
		formulario = append(formulario, ui.NuevaAyudaEnLinea(p.t(clave, nil)).ConTono(ui.AyudaAviso).ConTitulo(p.t("share.canEdit", nil)))
	}
	boton := ui.NuevoBoton(p.t("share.submit", nil), enviar).Con(ui.Primario, ui.TallaMD)
	boton.Cargando = ocupado
	return append(formulario, container.New(&pieRepartido{}, boton))
}

// quienLoVe es «Quién lo ve ahora»: cargando, «Solo tú» o la lista con revocar.
func (p *Pantalla) quienLoVe(cargando bool, lista []api.CalendarShare, revocar func(api.CalendarShare), v tema.Variante) []fyne.CanvasObject {
	abajo := []fyne.CanvasObject{alto(rotuloFuerte(p.t("share.whoSeesNow", nil)), tema.TextoSM.Linea)}
	switch {
	case cargando:
		abajo = append(abajo, ui.NuevoParrafo(p.t("common.loading", nil), tema.TextoSM, ui.Normal, "text-muted", v))
	case len(lista) == 0:
		vacio := ui.NuevoVacio(p.t("share.onlyYou", nil), p.t("share.empty.hint", nil))
		vacio.Ilustracion = iconos.Trazo(iconos.Candado, 32, "text-subtle", v).Objeto()
		abajo = append(abajo, vacio)
	default:
		filas := make([]fyne.CanvasObject, 0, len(lista))
		for _, s := range lista {
			filas = append(filas, p.filaCompartido(s, func() { revocar(s) }))
		}
		abajo = append(abajo, container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...))
	}
	return abajo
}

// filaCompartido es un `<li>` de «Quién lo ve ahora»:
//
//	border-border flex items-center gap-2 rounded-md border px-2.5 py-2
//	correo `text-sm truncate` · insignia del permiso · revocar (ghost sm iconOnly)
func (p *Pantalla) filaCompartido(s api.CalendarShare, revocar func()) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tono := ui.InsigniaPrimaria
	if s.Permission == api.PermisoLeer {
		tono = ui.InsigniaNeutra
	}
	insignia := ui.NuevaInsignia(p.t(insigniaCompartido[s.Permission], nil), tono)
	quitar := botonAspa(p.t("share.revoke", i18n.Vars{"email": s.Correo}), revocar)
	caja := ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD)
	texto := ui.Texto(s.Correo, tema.TextoSM, ui.Normal, tema.Color("text", v))
	return container.New(&filaCompartidoLayout{caja: caja, texto: texto, correo: s.Correo, insignia: insignia, boton: quitar},
		append(caja.Objetos(), texto, insignia, quitar)...)
}

type filaCompartidoLayout struct {
	caja     *ui.Caja
	texto    *canvas.Text
	correo   string
	insignia fyne.CanvasObject
	boton    fyne.CanvasObject
}

func (l *filaCompartidoLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, 2*(tema.Borde+espacio2)+max(l.boton.MinSize().Height, tema.TextoSM.Linea))
}

func (l *filaCompartidoLayout) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	l.caja.Redimensionar(t)
	centro := t.Height / 2
	x := t.Width - tema.Borde - espacio2p5
	b := l.boton.MinSize()
	x -= b.Width
	l.boton.Move(fyne.NewPos(x, centro-b.Height/2))
	l.boton.Resize(b)
	i := l.insignia.MinSize()
	x -= espacio2 + i.Width
	l.insignia.Move(fyne.NewPos(x, centro-i.Height/2))
	l.insignia.Resize(i)
	izq := tema.Borde + espacio2p5
	ui.Truncar(l.texto, l.correo, tema.TextoSM, ui.Normal, max(x-espacio2-izq, 0))
	ui.ColocarTexto(l.texto, tema.TextoSM, izq, centro-tema.TextoSM.Linea/2)
}
