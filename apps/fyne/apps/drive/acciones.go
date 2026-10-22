// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El «⋯» de un elemento y lo que hace cada acción
//
// `ActionsMenu` de FileBrowser.tsx, en el MISMO orden y con las mismas condiciones: sin
// red solo queda descargar, una carpeta se descarga como ZIP y un fichero sin id no se
// marca como favorito.
//
// Lo que NO está, y por qué:
//
//   - «Abrir en la aplicación»: la web lo esconde dentro de la app nativa (`inNativeShell`),
//     que es donde estamos.
//   - «Abrir en el editor» (WOPI): es una página web del despliegue, no una app de la
//     suite; en nativo no hay esquema al que llamar (ver `api.abre`).
//   - «Abrir en Docus/Calcus/Diapos»: en la web el rótulo va escrito a mano en castellano,
//     sin clave de catálogo; aquí pulsar la fila ya lo abre en su app.
//
// ponytail: falta «Organizar páginas», que es un diálogo propio y entra con él: un
// elemento de menú que no abre nada es peor que no tenerlo. (Las versiones van en la web
// detrás de «Compartir enlace» y delante de lo de PDF; aquí también.) Detrás de lo de PDF
// van «Clasificar» y «Resumir», y luego «Adjuntar a un correo» y «Compartir en Rooms»,
// que llaman a la otra app por su esquema (entreapps.go), juntas, como en la web.

// DuracionDeshacer es cuánto dura el aviso de «enviado a la papelera» con su «Deshacer»:
// los 10 s de la web y no los 5 de serie, porque deshacer hay que leerlo, decidirlo y
// alcanzarlo.
const DuracionDeshacer = 10 * time.Second

// accionMenu es una fila del menú: su rótulo, su glifo y qué hace.
type accionMenu struct {
	texto   string
	cuerpo  string       // glifo propio de Drive (`glyphs.tsx`)
	icono   iconos.Icono // o uno del catálogo
	peligro bool
	hacer   func()
}

// AccionesDe son las acciones del «⋯» de un elemento de «Mi unidad», ya filtradas. Es
// una función pura del elemento, de la conexión y de si es favorito, para que el
// guardián compare la lista sin abrir ningún menú.
func (p *Pantalla) AccionesDe(el Elemento, sinRed, favorito bool) []accionMenu {
	t := func(clave string) string { return p.tr.T(clave, nil) }
	var out []accionMenu
	add := func(si bool, a accionMenu) {
		if si {
			out = append(out, a)
		}
	}
	fichero := !el.Carpeta
	add(fichero, accionMenu{texto: t("action.download"), cuerpo: glifoDescargar, hacer: func() { go p.descargar(el) }})
	add(el.Carpeta && el.ID != "" && !sinRed, accionMenu{texto: t("action.downloadZip"), cuerpo: glifoDescargar, hacer: func() {
		go p.bajarZip(nil, []string{el.ID}, el.Nombre)
	}})
	add(!sinRed, accionMenu{texto: t("action.rename"), cuerpo: glifoLapiz, hacer: func() { p.pedirRenombrar(el) }})
	add(!sinRed, accionMenu{texto: t("action.move"), cuerpo: glifoMover, hacer: func() { p.pedirMover(el) }})
	add(!sinRed, accionMenu{texto: t("action.copy"), cuerpo: glifoCopia, hacer: func() { go p.copiar(el) }})
	add(!sinRed, accionMenu{texto: t("action.shareLink"), cuerpo: glifoEnlace, hacer: func() { p.pedirCompartir(el) }})
	// El historial es LECTURA: la web lo ofrece también sin red.
	add(fichero && el.ID != "", accionMenu{texto: t("action.versions"), cuerpo: glifoReloj, hacer: func() { p.pedirVersiones(el) }})
	// «Abrir en Sytue PDF» va con el verbo del producto: ahí se ve, se anota, se rellena
	// y se firma. El enlace lo arma `api.RutaDeApertura`, que es el de la web.
	add(!sinRed && fichero && Extension(el.Nombre) == "pdf",
		accionMenu{texto: t("pdf.openInPdf"), cuerpo: glifoLapiz, hacer: func() { p.abrirEnSuApp(el) }})
	// Clasificar dice la etiqueta que ya lleva: es lo primero que se quiere saber antes
	// de cambiarla.
	clasificar := t("action.classify")
	if el.Etiqueta != nil {
		clasificar = p.tr.T("action.classified", i18n.Vars{"name": el.Etiqueta.Name})
	}
	add(!sinRed && fichero && el.ID != "", accionMenu{texto: clasificar, cuerpo: glifoCandado, hacer: func() { p.pedirClasificar(el) }})
	// Resumir SOLO donde hay texto que leer: sobre un .xlsx sería un menú que promete y
	// luego falla.
	add(!sinRed && p.IA != nil && EsTexto(el), accionMenu{texto: t("ai.summarize"), icono: iconos.IA, hacer: func() { p.pedirResumen(el) }})
	// Llevarse el fichero a otro producto: van juntas porque son la misma idea.
	add(!sinRed && fichero && el.ID != "", accionMenu{texto: t("action.attachToMail"), cuerpo: glifoCorreo, hacer: func() { p.adjuntarACorreo(el) }})
	add(!sinRed && el.ID != "", accionMenu{texto: t("action.shareToRooms"), cuerpo: glifoUsuarios, hacer: func() { p.compartirEnRooms(el) }})
	// «Subir a Cast»: una COPIA en Sytue Cast (el fichero se queda aquí), solo para
	// vídeos. La pantalla de Cast enseña el fichero y pide confirmar.
	add(!sinRed && SubibleACast(el), accionMenu{texto: t("action.uploadToCast"), icono: iconos.Video, hacer: func() { p.subirACast(el) }})
	clave := "action.addFavorite"
	if favorito {
		clave = "action.removeFavorite"
	}
	add(!sinRed && el.ID != "", accionMenu{texto: t(clave), icono: iconos.DriveEstrella, hacer: func() { p.alternarFavorito(el, !favorito) }})
	add(!sinRed, accionMenu{texto: t("action.delete"), cuerpo: glifoPapelera, peligro: true, hacer: func() { go p.aLaPapelera(el) }})
	return out
}

// menuElemento abre el «⋯» colgado de su disparador.
func (p *Pantalla) menuElemento(el Elemento, disparador fyne.CanvasObject) {
	acciones := p.AccionesDe(el, p.explor.SinRed(), p.favoritos[el.ID])
	m := ui.NuevoMenu(elementosMenu(acciones)...)
	m.Rotulo = p.tr.T("action.menu", i18n.Vars{"name": el.Nombre})
	m.Mostrar(disparador)
}

// elementosMenu convierte las acciones en filas del menú compartido. `ElementoMenu` no
// tiñe un glifo propio, así que va ya con la tinta del icono de su estado: `text-muted`,
// o `danger-text` en la de peligro (el hover no la cambia).
func elementosMenu(acciones []accionMenu) []fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	out := make([]fyne.CanvasObject, 0, len(acciones))
	for _, a := range acciones {
		e := ui.NuevoElementoMenu(a.texto, a.icono, a.hacer)
		if a.cuerpo != "" {
			tinta := "text-muted"
			if a.peligro {
				tinta = "danger-text"
			}
			e.Glifo = glifoCon(a.cuerpo, iconos.TallaSM, tema.Color(tinta, v))
		}
		e.Peligro = a.peligro
		out = append(out, e)
	}
	return out
}

// menuNuevo es el desplegable de «Nuevo»: la carpeta y, tras el separador, los tres
// tipos de Sytue Docs. Antes solo había «Nueva carpeta» y para empezar un documento
// había que salir a Docs.
func (p *Pantalla) menuNuevo(disparador fyne.CanvasObject) {
	t := func(clave string) string { return p.tr.T(clave, nil) }
	m := ui.NuevoMenu(
		ui.NuevoElementoMenu(t("files.new.folder"), iconos.DriveCarpeta, p.pedirCarpeta),
		ui.NuevoSeparadorMenu(),
		ui.NuevoElementoMenu(t("files.new.document"), iconos.DriveDocumento, func() { p.nuevoDoc(api.DocText) }),
		ui.NuevoElementoMenu(t("files.new.spreadsheet"), iconos.DriveHoja, func() { p.nuevoDoc(api.DocSheet) }),
		ui.NuevoElementoMenu(t("files.new.presentation"), iconos.DrivePresentacion, func() { p.nuevoDoc(api.DocSlides) }),
	)
	m.Rotulo = t("files.new")
	m.Mostrar(disparador)
}

// TituloNuevoDoc es el título con el que nace un documento creado desde Drive: el
// `DEFAULT_TITLE` del BFF de la web (`pages/api/new-doc.ts`), literal. Es un DATO que se
// guarda en el servicio y no un rótulo de pantalla, y la web lo manda así en cualquier
// idioma; si aquí se tradujera, el mismo gesto dejaría documentos con nombres distintos
// según desde dónde se hizo.
var TituloNuevoDoc = map[api.DocKind]string{
	api.DocText:   "Documento sin título",
	api.DocSheet:  "Hoja sin título",
	api.DocSlides: "Presentación sin título",
}

// nuevoDoc es el elemento del menú: se comprueba la conexión aquí, en la rutina de la
// interfaz, y la petición va aparte.
func (p *Pantalla) nuevoDoc(k api.DocKind) {
	if p.Docs == nil || p.explor.SinRed() {
		return
	}
	go p.crearDoc(k)
}

// crearDoc da de alta el documento y lo abre en su editor. Corre en su propia rutina.
//
// El editor es la PÁGINA de la web de su producto (`/calcus/edit?id=`…), que es lo que abre la
// web en una pestaña nueva: las apps nativas de Docs abren ficheros de Drive en solo
// lectura (`api.ImportarDeDrive`) y no editan un documento del servicio.
func (p *Pantalla) crearDoc(k api.DocKind) {
	if !p.ocupar() {
		return
	}
	defer p.soltar()
	ctx, cancelar := p.ctx()
	defer cancelar()
	d, err := p.Docs.Create(ctx, k, TituloNuevoDoc[k])
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("files.doc.failed", nil), "")
		return
	}
	_ = api.AbrirFuera(p.abridor(), api.EnlaceDocumento(p.apex(), p.org(), d.ID, d.Kind))
	// Dónde quedó, porque no es donde se estaba: un documento de Docs no tiene carpeta
	// y, sin el aviso, «Nuevo → Documento» parece no haber hecho nada al volver.
	p.avisar(ui.AvisoExito, p.tr.T("files.doc.created", nil), p.tr.T("files.doc.createdWhere", nil))
}

// ocupar es el `busy` de la web: una operación a la vez, para que dos pulsaciones seguidas
// no manden dos veces lo mismo. Devuelve false si ya había una. Se llama desde cualquier
// rutina.
func (p *Pantalla) ocupar() bool {
	if !p.ocupado.CompareAndSwap(false, true) {
		return false
	}
	enHilo(func() { p.explor.Ocupado = true; p.Repintar() })
	return true
}

func (p *Pantalla) soltar() {
	p.ocupado.Store(false)
	enHilo(func() { p.explor.Ocupado = false; p.Repintar() })
}

// formCampo es un Modal de un solo campo: su título, la etiqueta, el primario y el de
// cancelar, el campo, la pista de debajo (puede no haber) y una nota tras el grupo.
type formCampo struct {
	titulo, etiqueta, cta, cancelar string
	campo                           *ui.Entrada
	pista                           *ui.PistaCampo
	nota                            *ui.Parrafo
}

// dialogoCampo es el Modal de un solo campo de la web —«Nueva carpeta», «Renombrar»—:
// talla md, la etiqueta encima del campo, Intro envía, el primario apagado mientras el
// campo esté vacío y girando mientras se envía. Se cierra SOLO si el envío sale bien:
// un nombre repetido deja el diálogo abierto para corregirlo, como en la web.
//
// `enviar` corre fuera de la rutina de la interfaz y dice si ha ido bien.
func (p *Pantalla) dialogoCampo(titulo, etiqueta, cta string, campo *ui.Entrada, enviar func(string) bool) *ui.Dialogo {
	return p.dialogoCampoCon(formCampo{
		titulo: titulo, etiqueta: etiqueta, cta: cta,
		cancelar: p.tr.T("files.cancel", nil), campo: campo,
	}, enviar)
}

// dialogoCampoCon es dialogoCampo con pista y nota (el «Nueva unidad» de equipo).
func (p *Pantalla) dialogoCampoCon(f formCampo, enviar func(string) bool) *ui.Dialogo {
	var d *ui.Dialogo
	var confirmar *ui.Boton
	campo := f.campo
	vacio := func(s string) bool { return strings.TrimSpace(s) == "" }
	mandar := func() {
		texto := strings.TrimSpace(campo.Texto())
		if texto == "" || confirmar.Cargando {
			return
		}
		confirmar.Cargando = true
		confirmar.Refresh()
		p.enSegundoPlano(func() {
			ok := enviar(texto)
			enHilo(func() {
				confirmar.Cargando = false
				confirmar.Refresh()
				if ok {
					d.Cerrar()
				}
			})
		})
	}
	cancelar := ui.NuevoBoton(f.cancelar, func() { d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	confirmar = ui.NuevoBoton(f.cta, mandar).Con(ui.Primario, ui.TallaMD)
	confirmar.Inerte = vacio(campo.Texto())
	campo.AlCambiar(func(s string) {
		if confirmar.Inerte != vacio(s) {
			confirmar.Inerte = vacio(s)
			confirmar.Refresh()
		}
	})
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			mandar()
			return true
		}
		return false
	}
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, confirmar)
	var debajo []fyne.CanvasObject
	if f.pista != nil {
		debajo = append(debajo, f.pista)
	}
	cuerpo := grupoCampo(f.etiqueta, campo, debajo...)
	if f.nota != nil {
		// `mt-3` de la nota tras el grupo.
		ancho := AnchoCuerpoDialogo(ui.DialogoMD, p.explor.Ventana)
		cuerpo = container.New(&pilaSeparada{separas: []float32{0, tema.Espacio(3)}, ancho: &ancho}, cuerpo, f.nota)
	}
	d = ui.NuevoDialogo(f.titulo, cuerpo).Con(ui.DialogoMD, pie)
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	return d
}

// pedirRenombrar abre «Renombrar» con el nombre de ahora ya escrito.
func (p *Pantalla) pedirRenombrar(el Elemento) {
	if p.win == nil {
		return
	}
	titulo := "files.rename.fileTitle"
	if el.Carpeta {
		titulo = "files.rename.folderTitle"
	}
	campo := ui.NuevaEntrada(el.Nombre)
	campo.PonerTexto(el.Nombre)
	d := p.dialogoCampo(p.tr.T(titulo, nil), p.tr.T("files.rename.label", nil), p.tr.T("files.rename.cta", nil),
		campo, func(nombre string) bool { return p.renombrar(el, nombre) })
	d.Mostrar(p.win.Canvas())
}

// renombrar pide el cambio. El mismo nombre no es un cambio y cierra sin preguntar.
func (p *Pantalla) renombrar(el Elemento, nombre string) bool {
	if nombre == el.Nombre {
		return true
	}
	if !p.ocupar() {
		return false
	}
	defer p.soltar()
	ctx, cancelar := p.ctx()
	defer cancelar()
	if err := p.svc.Rename(ctx, claseDe(el), el.ID, nombre); err != nil {
		clave := "files.rename.failed"
		if codigo(err) == http.StatusConflict {
			clave = "files.rename.conflict"
		}
		p.avisar(ui.AvisoPeligro, p.tr.T(clave, i18n.Vars{"name": nombre}), "")
		return false
	}
	p.avisar(ui.AvisoExito, p.tr.T("files.rename.done", i18n.Vars{"name": nombre}), "")
	go p.recargarVista()
	return true
}

// copiar hace «una copia» en el servidor: no mueve bytes, apunta al mismo contenido.
// Una carpeta se copia con todo lo que lleva dentro.
func (p *Pantalla) copiar(el Elemento) {
	if el.ID == "" || !p.ocupar() {
		return
	}
	defer p.soltar()
	ctx, cancelar := p.ctx()
	defer cancelar()
	var err error
	if el.Carpeta {
		_, err = p.svc.CopyFolder(ctx, el.ID)
	} else {
		_, err = p.svc.CopyFile(ctx, el.ID)
	}
	if err != nil {
		clave := "files.copy.failed"
		if codigo(err) == http.StatusInsufficientStorage {
			clave = "files.copy.noSpace"
		}
		p.avisar(ui.AvisoPeligro, p.tr.T(clave, nil), "")
		return
	}
	p.avisar(ui.AvisoExito, p.tr.T("files.copy.done", i18n.Vars{"name": el.Nombre}), "")
	go p.cargarCuota()
	p.recargarVista()
}

// aLaPapelera manda un elemento a la papelera SIN preguntar: es reversible, y un diálogo
// por cada fichero que se tira es una interrupción que no compra nada. Lo que sí compra
// es poder deshacerlo desde el mismo aviso. La confirmación se queda donde no hay vuelta
// atrás, que es la purga.
func (p *Pantalla) aLaPapelera(el Elemento) {
	if !p.ocupar() {
		return
	}
	defer p.soltar()
	ctx, cancelar := p.ctx()
	defer cancelar()
	if err := p.svc.Delete(ctx, claseDe(el), el.ID); err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("files.delete.failed", nil), "")
		return
	}
	enHilo(func() {
		a := ui.NuevoAviso(ui.AvisoExito, p.tr.T("files.delete.done", i18n.Vars{"name": el.Nombre}), "")
		a.Duracion = DuracionDeshacer
		// Sin id no hay a qué llamar para restaurar: la papelera trabaja por id.
		if el.ID != "" {
			a.RotuloAccion = p.tr.T("files.delete.undo", nil)
			a.AlPulsarAccion = func() { go p.deshacerPapelera(el) }
		}
		a.RotuloCerrar = p.tr.T("ui.close", nil)
		p.avisos.Notificar(a)
	})
	p.recargarVista()
}

// deshacerPapelera devuelve a su carpeta lo que se acaba de tirar.
func (p *Pantalla) deshacerPapelera(el Elemento) {
	ctx, cancelar := p.ctx()
	defer cancelar()
	if err := p.svc.Restore(ctx, claseDe(el), el.ID); err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("trash.restoreFailed", nil), "")
		return
	}
	p.avisar(ui.AvisoExito, p.tr.T("trash.restored", i18n.Vars{"name": el.Nombre}), "")
	p.recargarVista()
}

// alternarFavorito pone o quita la estrella al momento y la devuelve si el servicio no
// la acepta: el menú no tiene que esperar a la red para decir lo que la persona pidió.
// Se llama en la rutina de la interfaz, que es la dueña del mapa.
func (p *Pantalla) alternarFavorito(el Elemento, quiere bool) {
	if el.ID == "" {
		return
	}
	p.favoritos[el.ID] = quiere
	go func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		if err := p.svc.SetFavorite(ctx, claseDe(el), el.ID, quiere); err != nil {
			enHilo(func() { p.favoritos[el.ID] = !quiere })
			p.avisar(ui.AvisoPeligro, p.tr.T("files.favorite.failed", nil), "")
		}
	}()
}

// cargarFavoritos trae los ids con estrella una vez: el menú dice «Añadir» o «Quitar»
// sin pedir nada por fila. El explorador funciona igual sin ellos.
func (p *Pantalla) cargarFavoritos() {
	if p.svc == nil {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	favs, err := p.svc.Favorites(ctx)
	if err != nil {
		return
	}
	enHilo(func() {
		for _, x := range favs {
			p.favoritos[x.ID] = true
		}
	})
}

// codigo es el estado HTTP del error del servicio, o 0 si no lo es.
func codigo(err error) int {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Status
	}
	return 0
}

// abridor es quien delega en el sistema: el de la pantalla o, si no hay, la app.
func (p *Pantalla) abridor() api.Abridor {
	if p.App != nil {
		return p.App
	}
	return fyne.CurrentApp()
}

// apex es el dominio de la suite; vacío es producción.
func (p *Pantalla) apex() string {
	if p.Apex == "" {
		return api.ApexProd
	}
	return p.Apex
}

// org es la organización activa de la sesión, o "" sin sesión y en el guardián.
func (p *Pantalla) org() string {
	if p.Org == nil {
		return ""
	}
	return p.Org()
}

// recargarVista vuelve a pedir lo que se está viendo tras una mutación: la búsqueda si
// es lo que hay delante, la carpeta si no. Corre en su propia rutina.
func (p *Pantalla) recargarVista() {
	var busqueda bool
	var consulta, carpeta string
	fyne.DoAndWait(func() {
		busqueda, consulta, carpeta = p.explor.EnBusqueda, strings.TrimSpace(p.explor.Consulta), p.carpetaActual()
	})
	if busqueda {
		p.ejecutarBusqueda(consulta)
		return
	}
	p.cargarCarpeta(carpeta)
}

// grupoCampo es el FieldGroup de la web con su ayuda debajo: ui.GrupoCampo pone la
// etiqueta y el control, y la PistaCampo va a continuación en la misma columna
// `gap-1.5`. No se le pasa como `ayuda` porque su rótulo no parte líneas y la ayuda de
// la web sí.
func grupoCampo(etiqueta string, control fyne.CanvasObject, debajo ...fyne.CanvasObject) fyne.CanvasObject {
	g := ui.GrupoCampo(etiqueta, false, control, "", "")
	if len(debajo) == 0 {
		return g
	}
	return container.New(ui.Columna{Hueco: ui.HuecoGrupoCampo}, append([]fyne.CanvasObject{g}, debajo...)...)
}
