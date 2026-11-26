// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/asistente"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// Fallo enseña un error en la ventana.
func (l *App) Fallo(w fyne.Window, err error) {
	if err != nil {
		Info(w, l.T("error"), err.Error())
	}
}

// MostrarAjustes abre Ajustes: perfil y apariencia, IA, nube, datos y acerca de (§5.5).
//
// Las apps añaden sus pestañas (AjustesDeLaApp), detrás de las de la cáscara y antes de
// «Acerca de». Volver a abrir Ajustes tras un cambio deja la misma pestaña delante.
func (l *App) MostrarAjustes(w fyne.Window) { l.mostrarAjustes(w, "") }

func (l *App) mostrarAjustes(w fyne.Window, delante string) {
	var d *ui.Dialogo
	var pestañas *container.AppTabs
	reabrir := func() {
		actual := ""
		if pestañas != nil && pestañas.Selected() != nil {
			actual = pestañas.Selected().Text
		}
		d.Cerrar()
		l.mostrarAjustes(w, actual)
	}
	secciones := l.seccionesApp()
	items := make([]*container.TabItem, 0, 8+len(secciones))
	items = append(items,
		container.NewTabItem(l.T("perfil"), l.ajustesPerfil(w)),
		container.NewTabItem(l.T("ia"), l.ajustesIA(w)),
		container.NewTabItem(l.T("nube"), l.ajustesNube(w, reabrir)),
		container.NewTabItem(l.T("cuentas"), container.NewVScroll(l.ajustesCuentas(w, reabrir))),
		container.NewTabItem(l.T("datos"), l.ajustesDatos(w)),
		container.NewTabItem(l.T("copia"), container.NewVScroll(l.ajustesCopia(w, reabrir))),
		container.NewTabItem(l.T("red"), container.NewVScroll(l.ajustesRed(w))),
	)
	for _, s := range secciones {
		items = append(items, container.NewTabItem(s.titulo, container.NewVScroll(s.crear(w))))
	}
	items = append(items, container.NewTabItem(l.T("acerca"), l.acercaDe()))
	pestañas = container.NewAppTabs(items...)
	for _, it := range items {
		if delante != "" && it.Text == delante {
			pestañas.Select(it)
		}
	}
	d = Ventana(w, l.T("ajustes")+" — "+espacio.Nombre, pestañas, nil)
}

func ayuda(texto string) *widget.Label {
	l := widget.NewLabel(texto)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.LowImportance
	return l
}

func (l *App) ajustesPerfil(w fyne.Window) fyne.CanvasObject {
	p := l.Espacio.Perfil
	nombre := widget.NewEntry()
	nombre.SetText(p.Nombre)
	correo := widget.NewEntry()
	correo.SetText(p.Correo)
	idiomas := make([]string, 0, 1+len(i18n.Locales))
	idiomas = append(idiomas, "—")
	for _, loc := range i18n.Locales {
		idiomas = append(idiomas, i18n.Names[loc])
	}
	idioma := widget.NewSelect(idiomas, nil)
	idioma.SetSelectedIndex(0)
	for i, loc := range i18n.Locales {
		if string(loc) == p.Idioma {
			idioma.SetSelectedIndex(i + 1)
		}
	}
	temas := []string{"", "claro", "oscuro"}
	tm := widget.NewSelect([]string{l.T("tema.sistema"), l.T("tema.claro"), l.T("tema.oscuro")}, nil)
	tm.SetSelectedIndex(0)
	for i, t := range temas {
		if t == p.Tema {
			tm.SetSelectedIndex(i)
		}
	}
	guardar := widget.NewButton(l.T("guardar"), func() {
		p.Nombre, p.Correo = strings.TrimSpace(nombre.Text), strings.TrimSpace(correo.Text)
		p.Idioma = ""
		if i := idioma.SelectedIndex(); i > 0 {
			p.Idioma = string(i18n.Locales[i-1])
		}
		p.Tema = temas[max(tm.SelectedIndex(), 0)]
		if err := l.Espacio.GuardarPerfil(p); err != nil {
			l.Fallo(w, err)
			return
		}
		tema.Elegir(p.Tema)
		l.A.Settings().SetTheme(l.A.Settings().Theme()) // repinta con el tema nuevo
		Info(w, l.T("ajustes"), l.T("reiniciar"))
	})
	guardar.Importance = widget.HighImportance
	return container.NewVBox(
		widget.NewForm(widget.NewFormItem(l.T("perfil.nombre"), nombre), widget.NewFormItem(l.T("perfil.correo"), correo)),
		ayuda(l.T("perfil.ayuda")),
		widget.NewSeparator(),
		widget.NewForm(widget.NewFormItem(l.T("idioma"), idioma), widget.NewFormItem(l.T("tema"), tm)),
		container.NewHBox(guardar),
	)
}

func (l *App) ajustesIA(w fyne.Window) fyne.CanvasObject {
	cfg := l.IA.Config()
	pre := asistente.Preajustes()
	nombres := make([]string, len(pre))
	for i, p := range pre {
		nombres[i] = l.NombreIA(p)
	}
	modelo, direccion, clave := widget.NewEntry(), widget.NewEntry(), widget.NewPasswordEntry()
	clave.SetText(l.IA.Clave())
	destino := ayuda("")
	notaSistema := ayuda("")
	form := widget.NewForm()
	elegido := 0
	refrescar := func(i int) {
		elegido = i
		p := pre[i]
		if p.ID != cfg.Proveedor || modelo.Text == "" {
			modelo.SetText(p.Modelo)
			direccion.SetText(p.URL)
		}
		form.Items = nil
		if p.ID != asistente.Ninguno {
			form.Append(l.T("ia.modelo"), modelo)
		}
		if p.EditaURL {
			form.Append(l.T("ia.url"), direccion)
		}
		if p.Clave {
			form.Append(l.T("ia.clave"), clave)
		}
		form.Refresh()
		notaSistema.SetText(l.notaIASistema(p))
	}
	sel := widget.NewSelect(nombres, nil)
	sel.OnChanged = func(string) { refrescar(sel.SelectedIndex()) }
	for i, p := range pre {
		if p.ID == cfg.Proveedor {
			modelo.SetText(cfg.Modelo)
			direccion.SetText(cfg.URL)
			elegido = i
		}
	}
	sel.SetSelectedIndex(elegido)
	guardarCfg := func() bool {
		p := pre[elegido]
		c := asistente.Config{Proveedor: p.ID, Modelo: strings.TrimSpace(modelo.Text)}
		if p.EditaURL || (direccion.Text != "" && direccion.Text != p.URL) {
			c.URL = strings.TrimSpace(direccion.Text)
		}
		if err := l.IA.Guardar(c, strings.TrimSpace(clave.Text)); err != nil {
			l.Fallo(w, err)
			return false
		}
		destino.SetText(l.T("ia.destino") + l.DestinoIA())
		l.iaCambiada()
		return true
	}
	if l.IA.Activa() {
		destino.SetText(l.T("ia.destino") + l.DestinoIA())
	}
	guardar := widget.NewButton(l.T("guardar"), func() { guardarCfg() })
	guardar.Importance = widget.HighImportance
	probar := widget.NewButton(l.T("ia.probar"), nil)
	probar.OnTapped = func() {
		if !guardarCfg() || !l.IA.Activa() {
			return
		}
		probar.Disable()
		probar.SetText(l.T("ia.probando"))
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), 90*time.Second)
			defer parar()
			_, err := l.IA.Probar(ctx)
			fyne.Do(func() {
				probar.Enable()
				probar.SetText(l.T("ia.probar"))
				l.avisarPruebaIA(w, err)
			})
		}()
	}
	return container.NewVBox(ayuda(l.T("ia.ayuda")), widget.NewForm(widget.NewFormItem(l.T("ia.proveedor"), sel)), form, notaSistema, destino, container.NewHBox(guardar, probar))
}

// notaIASistema es lo que se dice debajo al elegir la IA del sistema en el Mac y en
// Windows (qué hace falta para que responda); "" con las demás.
func (l *App) notaIASistema(p asistente.Preajuste) string {
	if p.ID != asistente.Sistema {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return l.T("ia.sistema.mac")
	case "windows":
		return l.T("ia.sistema.win")
	}
	return ""
}

// avisarPruebaIA dice cómo fue «Probar»: lo que le falta a la IA del sistema en el
// teléfono o en el escritorio, el fallo tal cual o que responde. En el hilo de la interfaz.
func (l *App) avisarPruebaIA(w fyne.Window, err error) {
	var sinModelo *asistente.ErrorSistema
	if errors.As(err, &sinModelo) && (runtime.GOOS == "ios" || runtime.GOOS == "android") {
		Info(w, l.T("error"), l.nombreIAMovil(runtime.GOOS, sinModelo.Estado))
		return
	}
	var escritorio *asistente.ErrorEscritorio
	if errors.As(err, &escritorio) {
		Info(w, l.T("error"), l.textoIAEscritorio(escritorio))
		return
	}
	if err != nil {
		l.Fallo(w, err)
		return
	}
	Info(w, l.T("ia"), l.T("ia.ok"))
}

// NombreIA es el nombre de un proveedor de IA en el idioma de la app: los de marca
// (Anthropic, OpenAI, Gemini) tal cual, y los genéricos y la IA del sistema, de la tabla.
// En el móvil, la IA del sistema lleva además lo que le falta: el núcleo da el estado
// (`asistente.EstadoDelSistema`) y aquí se dice (nombreIAMovil).
func (l *App) NombreIA(p asistente.Preajuste) string {
	switch p.ID {
	case asistente.Ninguno:
		return l.T("ia.ninguna")
	case asistente.Local:
		return l.T("ia.local")
	case asistente.Otro:
		return l.T("ia.otro")
	case asistente.Sistema:
		switch runtime.GOOS {
		case "darwin":
			return l.T("ia.nombre.mac")
		case "windows":
			return l.T("ia.nombre.win")
		case "ios", "android":
			return l.nombreIAMovil(runtime.GOOS, asistente.EstadoDelSistema())
		}
		return l.T("ia.nombre.sistema")
	}
	return p.Nombre
}

// textoEstadoIA es la clave de lo que le falta a la IA del sistema del móvil en cada
// estado; "" es que no falta nada (lista, o aún preguntando).
var textoEstadoIA = map[asistente.EstadoSistema]string{
	asistente.SistemaComprobando: "",
	asistente.SistemaLista:       "",
	asistente.SistemaDescargable: "ia.estado.descargable",
	asistente.SistemaDescargando: "ia.estado.descargando",
	asistente.SistemaNoSoportada: "ia.estado.noSoportada",
	asistente.SistemaApagada:     "ia.estado.apagada",
	asistente.SistemaAntiguo:     "ia.estado.antiguo",
	asistente.SistemaSinPuente:   "ia.estado.sinPuente",
	asistente.SistemaFallo:       "ia.estado.fallo",
}

// nombreIAMovil es el nombre de la IA del sistema de iOS o Android (goos) con lo que le
// falta en el estado e, en el idioma de la app: «Gemini Nano (en el teléfono) — no
// disponible: …». Lista, o mientras se pregunta, el nombre solo.
func (l *App) nombreIAMovil(goos string, e asistente.EstadoSistema) string {
	nombre := l.T("ia.nombre.android")
	if goos == "ios" {
		nombre = l.T("ia.nombre.ios")
	}
	clave, ok := textoEstadoIA[e]
	if !ok {
		clave = textoEstadoIA[asistente.SistemaFallo]
	}
	if clave == "" {
		return nombre
	}
	return fmt.Sprintf(l.T("ia.noDisponible"), nombre, l.T(clave))
}

// DestinoIA es a quién se envía el texto que se le pide a la IA, en el idioma de la app.
func (l *App) DestinoIA() string { return l.IA.DestinoCon(l.NombreIA, l.T("ia.enEsteEquipo")) }

func (l *App) ajustesNube(w fyne.Window, reabrir func()) fyne.CanvasObject {
	caja := container.NewVBox(ayuda(l.T("nube.ayuda")), widget.NewLabelWithStyle(l.T("nube.cuentas"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	cuentas := l.Nube.Cuentas()
	if len(cuentas) == 0 {
		caja.Add(ayuda(l.T("nube.ninguna")))
	}
	for _, c := range cuentas {
		id := c.ID
		detalle := c.Rotulo
		if c.Usuario != "" {
			detalle += " — " + c.Usuario
		}
		caja.Add(container.NewBorder(nil, nil, nil, widget.NewButton(l.T("nube.quitar"), func() {
			l.Fallo(w, l.Nube.Quitar(id))
			reabrir()
		}), widget.NewLabel(detalle)))
	}
	caja.Add(widget.NewSeparator())
	caja.Add(widget.NewButton(l.T("nube.sytue"), func() { l.altaDAV(w, nube.SytueDrive, reabrir) }))
	caja.Add(widget.NewButton(l.T("nube.webdav"), func() { l.altaDAV(w, nube.WebDAV, reabrir) }))
	caja.Add(widget.NewButton(l.T("nube.google"), func() { l.altaOAuth(w, nube.Google, l.Nube.Clientes.Google, reabrir) }))
	caja.Add(widget.NewButton(l.T("nube.onedrive"), func() { l.altaOAuth(w, nube.OneDrive, l.Nube.Clientes.Microsoft, reabrir) }))
	caja.Add(widget.NewSeparator())
	caja.Add(widget.NewLabelWithStyle(l.T("nube.sincronizadas"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	sinc := nube.Sincronizadas()
	if len(sinc) == 0 {
		caja.Add(ayuda(l.T("nube.sincronizadas.no")))
	}
	for _, c := range sinc {
		ruta := c.Ruta
		// Los datos se llevan a una carpeta «Sytue Office» dentro de la sincronizada, con
		// todo lo que hay (ajustes_datos.go).
		caja.Add(container.NewBorder(nil, nil, nil, widget.NewButton(l.T("nube.usar"), func() {
			l.moverDatos(w, filepath.Join(ruta, espacio.CarpetaDatos))
		}), widget.NewLabel(c.Nombre+" — "+ruta)))
	}
	return caja
}

func (l *App) altaDAV(w fyne.Window, tipo string, listo func()) {
	dir, usuario, clave := widget.NewEntry(), widget.NewEntry(), widget.NewPasswordEntry()
	items := []*widget.FormItem{}
	if tipo == nube.WebDAV {
		dir.SetPlaceHolder("https://cloud.example.org/remote.php/dav/files/ana")
		items = append(items, widget.NewFormItem(l.T("ia.url"), dir))
	}
	items = append(items, widget.NewFormItem(l.T("nube.usuario"), usuario), widget.NewFormItem(l.T("nube.contrasena"), clave))
	titulo := l.T("nube.webdav")
	if tipo == nube.SytueDrive {
		titulo = l.T("nube.sytue")
		items = append(items, widget.NewFormItem("", ayuda(l.T("nube.sytue.ayuda"))))
	}
	Formulario(w, titulo, l.T("cancelar"), l.T("guardar"), items, func(d *ui.Dialogo) {
		d.Cerrar()
		c := nube.Cuenta{Tipo: tipo, URL: strings.TrimSpace(dir.Text), Usuario: strings.TrimSpace(usuario.Text)}
		cuenta, err := l.Nube.Añadir(c, clave.Text)
		if err != nil {
			l.Fallo(w, err)
			return
		}
		// Se comprueba en el acto: una contraseña mal puesta se dice ahora, no al abrir.
		go func() {
			p, err := l.Nube.Proveedor(cuenta.ID)
			if err == nil {
				ctx, parar := context.WithTimeout(context.Background(), 30*time.Second)
				_, err = p.Listar(ctx, "")
				parar()
			}
			fyne.Do(func() {
				if err != nil {
					_ = l.Nube.Quitar(cuenta.ID)
					l.Fallo(w, err)
				}
				listo()
			})
		}()
	})
}

func (l *App) altaOAuth(w fyne.Window, tipo, cliente string, listo func()) {
	conectar := func(cliente string) {
		aviso := Espera(w, l.T("nube"), l.T("nube.navegador"))
		go func() {
			ctx, parar := context.WithTimeout(context.Background(), 5*time.Minute)
			defer parar()
			// El portero deja ir al servidor de tokens solo mientras dura el alta (§5.8).
			hosts := nube.HostsDeAlta(tipo)
			ds := make([]red.Destino, 0, len(hosts))
			for _, h := range hosts {
				ds = append(ds, red.Destino{Proposito: red.Nube, Host: h, Motivo: l.T("red.motivo.alta")})
			}
			defer l.Red.Permitir(ds...)()
			_, err := l.Nube.ConectarOAuth(ctx, tipo, cliente, func(destino string) error { return api.AbrirFuera(l.A, destino) })
			fyne.Do(func() {
				aviso.Cerrar()
				l.Fallo(w, err)
				listo()
			})
		}()
	}
	if cliente != "" {
		conectar(cliente)
		return
	}
	id := widget.NewEntry()
	Formulario(w, l.T("nube.cliente"), l.T("cancelar"), l.T("guardar"), []*widget.FormItem{
		widget.NewFormItem(l.T("nube.cliente"), id), widget.NewFormItem("", ayuda(l.T("nube.cliente.ayuda"))),
	}, func(d *ui.Dialogo) {
		if strings.TrimSpace(id.Text) != "" {
			d.Cerrar()
			conectar(strings.TrimSpace(id.Text))
		}
	})
}

func (l *App) acercaDe() fyne.CanvasObject {
	m := l.A.Metadata()
	enlace, _ := url.Parse("https://github.com/thinkddr/sytue-suite")
	return container.NewVBox(
		widget.NewLabelWithStyle(m.Name+" "+m.Version, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		ayuda(l.T("acerca.texto")),
		ayuda(l.Espacio.Rutas.Ajustes),
		widget.NewHyperlink("Apache-2.0", enlace),
	)
}

// textoIAEscritorio es lo que falla de la IA del sistema en el escritorio (el atajo de
// macOS, Foundry Local en Windows), en el idioma de la app.
func (l *App) textoIAEscritorio(e *asistente.ErrorEscritorio) string {
	return textoIAEscritorio(l.Idioma(), e)
}

func textoIAEscritorio(idioma i18n.Locale, e *asistente.ErrorEscritorio) string {
	switch e.Codigo {
	case asistente.EscritorioFaltaAtajo:
		return fmt.Sprintf(Texto(idioma, "ia.escritorio.faltaAtajo"), asistente.AtajoIA)
	case asistente.EscritorioFalloAtajo:
		return fmt.Sprintf(Texto(idioma, "ia.escritorio.falloAtajo"), e.Detalle)
	case asistente.EscritorioSinTexto:
		return Texto(idioma, "ia.escritorio.sinTexto")
	case asistente.EscritorioFaltaFoundry:
		return Texto(idioma, "ia.escritorio.faltaFoundry")
	case asistente.EscritorioFoundryParado:
		return Texto(idioma, "ia.escritorio.foundryParado")
	}
	return e.Error()
}
