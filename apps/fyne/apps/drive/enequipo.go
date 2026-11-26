// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"io"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Drive en el equipo (Sytue Office)
//
// La edición local (PLAN-EDICION-LOCAL §6.4) monta esta misma pantalla sobre las carpetas
// del disco (`local/archivos`). Lo que no tiene sentido sin organización ni servidor
// —compartir y enlaces, «Compartido conmigo», unidades de equipo, «Documentos» de Docs,
// las nubes y el almacén de la organización, clasificar, «Traer mis datos», llevarse el
// fichero a Mail, Rooms o Cast— no se pinta (DR-05), y lo que en Drive es descargar pasa a
// ser «Copiar a…» y «Mostrar en la carpeta» (DR-08).
//
// Todo cuelga de un solo campo, `Pantalla.EnEquipo`: nil (el valor cero, el de la
// edición conectada) es exactamente el comportamiento de siempre.

// EnEquipo es lo que la edición local enchufa a la pantalla. Lo pone `cmd-local/drive`.
type EnEquipo struct {
	// Abrir abre un fichero en SU app de Sytue Office o, si no hay, con la del sistema,
	// directamente sobre el fichero del disco (DR-07). Se llama fuera de la interfaz.
	Abrir func(el Elemento) error
	// CrearDocumento deja en la carpeta (id; vacío, la raíz) un documento nuevo de ese
	// tipo y lo devuelve (DR-06). Se llama fuera de la interfaz.
	CrearDocumento func(ctx context.Context, carpeta string, k api.DocKind) (api.File, error)
	// CopiarA pide una carpeta del equipo y deja allí una copia del elemento; llama a
	// listo con la ruta de la copia, o no lo llama si se cancela (DR-08).
	CopiarA func(el Elemento, listo func(destino string, err error))
	// MostrarEnCarpeta abre la carpeta del sistema con el elemento señalado (DR-08).
	MostrarEnCarpeta func(el Elemento) error
	// Texto da los rótulos propios de la edición, que no están en el catálogo de Drive:
	// «copiarA», «mostrarCarpeta», «abrirCon», «noSeCopia», «noSeAbre», «nuevoFallo»;
	// «etiquetas», «etiquetaNueva», «etiquetaAnadir», «etiquetasVacio» y «etiquetasFallo»
	// (DR-09); los de la papelera que en el catálogo hablan del servidor
	// («papeleraAyuda», «papeleraCuenta», PapeleraPropia); los de mover entre unidades
	// («moviendo», «moviendoPaso», «moverParcial», «moverCancelado», «moverNoMovidos»),
	// los de subir con un nombre repetido («homonimoTitulo», «homonimosTitulo»,
	// «homonimoTexto», «homonimosTexto», «sustituir», «guardarAmbos», «noSubir») y la
	// estrella de lo que está en la nube («favoritoNube», «favoritoNubeQuitar»).
	Texto func(clave string) string
	// Etiquetas son las etiquetas libres, en el hueco de «Clasificar» (DR-09). nil: no se
	// ofrecen.
	Etiquetas *EtiquetasEnEquipo
	// Unidades son las cuentas de nube conectadas, que salen en el lateral (DR-13); nil o
	// vacío, ninguna. EsRemoto dice si un id es de una de ellas.
	Unidades func() []UnidadEquipo
	EsRemoto func(id string) bool
	// EditarFoto abre una imagen directamente en el editor de Photos de Sytue Office
	// (FO-06); el rótulo es «editarFoto». nil: no se ofrece. Se llama fuera de la interfaz.
	EditarFoto func(el Elemento) error
	// UnidadDe da la unidad de un id: "" el disco, o el id de la raíz de su unidad de la
	// nube. nil: todo es del disco.
	UnidadDe func(id string) string
	// MoverEntreUnidades lleva els a la carpeta destino de OTRA unidad (el disco y una
	// nube, o dos nubes): copia, comprueba y solo entonces quita el origen. avance dice
	// por dónde va; cancelar ctx lo para entre fichero y fichero. Se llama fuera de la
	// interfaz. Con UnidadDe, «Mover a…» ofrece las demás unidades; nil: solo la suya.
	MoverEntreUnidades func(ctx context.Context, els []Elemento, destino string, avance func(hechos, total int, nombre string)) InformeMover
	// Homonimos son los ficheros de una carpeta de una unidad de la nube a los que
	// alcanzaría subir uno con ese nombre (Google Drive admite varios que se llaman igual),
	// y libre el primer nombre que no coge nadie. Con ellos, subir o soltar pregunta si
	// sustituir (cuál, si hay varios) o guardar los dos. nil: no se pregunta.
	Homonimos func(ctx context.Context, carpeta, nombre string) (iguales []Elemento, libre string, err error)
}

// InformeMover es lo que pasó al mover entre unidades.
type InformeMover struct {
	// Total es cuántos había que mover y Movidos cuántos están ya solo en el destino.
	Total, Movidos int
	// NoMovidos dice, uno por línea y ya en el idioma de la app, lo que se quedó en el
	// origen y por qué. Lo que no llegó por cancelar no se lista.
	NoMovidos []string
	Cancelado bool
	// Err: no se pudo ni empezar.
	Err error
}

// PapeleraPropia son las claves del catálogo de la papelera que en el equipo dicen otra
// cosa (que se borra solo «de este equipo», que ocupa el disco y no «tu almacenamiento»),
// con la clave del texto propio que las sustituye. Sin ese texto, el del catálogo.
var PapeleraPropia = map[string]string{
	"trash.help":          "papeleraAyuda",
	"trash.helpCountdown": "papeleraCuenta",
}

// enEquipo dice si la pantalla es la de Sytue Office.
func (p *Pantalla) enEquipo() bool { return p.EnEquipo != nil }

// textoEquipo es un rótulo propio de la edición; sin tabla, la clave.
func (p *Pantalla) textoEquipo(clave string) string {
	if p.EnEquipo != nil && p.EnEquipo.Texto != nil {
		return p.EnEquipo.Texto(clave)
	}
	return clave
}

// accionesEnEquipo es el «⋯» de un elemento en el equipo: el de `AccionesDe` sin lo que
// necesita organización o servidor, y con «Copiar a…» y «Mostrar en la carpeta» donde
// Drive tiene «Descargar».
func (p *Pantalla) accionesEnEquipo(el Elemento, favorito bool) []accionMenu {
	if p.esRemoto(el) {
		return p.accionesRemotas(el, favorito)
	}
	t := func(clave string) string { return p.tr.T(clave, nil) }
	fichero := !el.Carpeta
	out := []accionMenu{
		{texto: p.textoEquipo("copiarA"), cuerpo: glifoCopia, hacer: func() { p.copiarA(el) }},
		{texto: p.textoEquipo("mostrarCarpeta"), cuerpo: glifoCarpeta, hacer: func() { go p.mostrarEnSistema(el) }},
	}
	add := func(si bool, a accionMenu) {
		if si {
			out = append(out, a)
		}
	}
	add(p.explor.EnBusqueda && el.ID != "", accionMenu{texto: t("action.showInFolder"), cuerpo: glifoCarpeta, hacer: func() { p.mostrarEnSuCarpeta(el) }})
	add(true, accionMenu{texto: t("action.rename"), cuerpo: glifoLapiz, hacer: func() { p.pedirRenombrar(el) }})
	add(true, accionMenu{texto: t("action.move"), cuerpo: glifoMover, hacer: func() { p.pedirMover(el) }})
	add(true, accionMenu{texto: t("action.copy"), cuerpo: glifoCopia, hacer: func() { go p.copiar(el) }})
	add(fichero && el.ID != "", accionMenu{texto: t("action.versions"), cuerpo: glifoReloj, hacer: func() { p.pedirVersiones(el) }})
	add(el.ID != "", accionMenu{texto: t("details.action"), icono: iconos.Informacion, hacer: func() { p.pedirDetalles(el) }})
	add(fichero && Extension(el.Nombre) == "pdf", accionMenu{texto: t("pdf.openInPdf"), cuerpo: glifoLapiz, hacer: func() { go p.abrirConEquipo(el) }})
	add(p.EnEquipo.EditarFoto != nil && TipoVista(el, true) == "imagen", accionMenu{texto: p.textoEquipo("editarFoto"), cuerpo: glifoLapiz, hacer: func() { go p.editarFoto(el) }})
	// «Etiquetas» va donde la conectada pone «Clasificar» (DR-09).
	add(p.EnEquipo.Etiquetas != nil && el.ID != "", accionMenu{texto: p.textoEquipo("etiquetas"), icono: iconos.Etiqueta, hacer: func() { p.pedirEtiquetas(el) }})
	add(p.IA != nil && EsTexto(el), accionMenu{texto: t("ai.summarize"), icono: iconos.IA, hacer: func() { p.pedirResumen(el) }})
	clave := "action.addFavorite"
	if favorito {
		clave = "action.removeFavorite"
	}
	add(el.ID != "", accionMenu{texto: t(clave), icono: iconos.DriveEstrella, hacer: func() { p.alternarFavorito(el, !favorito) }})
	add(true, accionMenu{texto: t("action.delete"), cuerpo: glifoPapelera, peligro: true, hacer: func() { go p.aLaPapelera(el) }})
	return out
}

// accionesPlanoEnEquipo es el «⋯» de «Recientes» y «Destacados» en el equipo.
func (p *Pantalla) accionesPlanoEnEquipo(el Elemento) []accionMenu {
	if p.esRemoto(el) {
		// Lo de una nube (sus favoritos, lo que lleva una etiqueta) no tiene carpeta del
		// sistema que enseñar: se trae con «Copiar a…» o se abre.
		if el.Carpeta {
			return nil
		}
		return []accionMenu{{texto: p.textoEquipo("copiarA"), cuerpo: glifoCopia, hacer: func() { p.copiarA(el) }}}
	}
	out := []accionMenu{
		{texto: p.textoEquipo("copiarA"), cuerpo: glifoCopia, hacer: func() { p.copiarA(el) }},
		{texto: p.textoEquipo("mostrarCarpeta"), cuerpo: glifoCarpeta, hacer: func() { go p.mostrarEnSistema(el) }},
	}
	if el.ID != "" {
		out = append(out, accionMenu{texto: p.tr.T("action.showInFolder", nil), cuerpo: glifoCarpeta, hacer: func() { p.mostrarEnSuCarpeta(el) }})
	}
	return out
}

// elementosNuevoEnEquipo es «Nuevo» en el equipo: la carpeta y los tres documentos de la
// ofimática, que nacen como ficheros en la carpeta abierta y se abren en su app.
func (p *Pantalla) elementosNuevoEnEquipo() []fyne.CanvasObject {
	if p.EnUnidad() {
		return p.elementosNuevoEnUnidad()
	}
	t := func(clave string) string { return p.tr.T(clave, nil) }
	out := []fyne.CanvasObject{
		ui.NuevoElementoMenu(t("files.new.folder"), iconos.DriveCarpeta, p.pedirCarpeta),
	}
	if p.ElegirCarpeta != nil {
		out = append(out, ui.NuevoElementoMenu(t("files.upload.folder"), iconos.Subir, p.elegirCarpetaYSubir))
	}
	if p.EnEquipo.CrearDocumento == nil {
		return out
	}
	return append(out,
		ui.NuevoSeparadorMenu(),
		ui.NuevoElementoMenu(t("files.new.document"), iconos.DriveDocumento, func() { p.nuevoEnEquipo(api.DocText) }),
		ui.NuevoElementoMenu(t("files.new.spreadsheet"), iconos.DriveHoja, func() { p.nuevoEnEquipo(api.DocSheet) }),
		ui.NuevoElementoMenu(t("files.new.presentation"), iconos.DrivePresentacion, func() { p.nuevoEnEquipo(api.DocSlides) }),
	)
}

// nuevoEnEquipo crea el documento en la carpeta abierta, recarga y lo abre en su app.
// Se llama en la rutina de la interfaz.
func (p *Pantalla) nuevoEnEquipo(k api.DocKind) {
	carpeta := p.carpetaActual()
	go func() {
		if !p.ocupar() {
			return
		}
		ctx, cancelar := p.ctx()
		f, err := p.EnEquipo.CrearDocumento(ctx, carpeta, k)
		cancelar()
		p.soltar()
		if err != nil {
			p.avisar(ui.AvisoPeligro, p.textoEquipo("nuevoFallo"), "")
			return
		}
		p.recargarVista()
		p.abrirConEquipo(DeFichero(f))
	}()
}

// abrirEnEquipo es `abrir` para un fichero en el equipo: la vista previa si la tiene
// —texto, y también las fotos (DR-11), que aquí no tienen otra app que las enseñe sin
// importarlas—; si no, su app de Sytue Office o la del sistema. Nada se descarga: el
// fichero ya está en el disco. Se llama en la rutina de la interfaz.
func (p *Pantalla) abrirEnEquipo(el Elemento) {
	if TipoVista(el, true) != "" && p.svc != nil {
		p.abrirVista(el, true, func(ctx context.Context) (io.ReadCloser, error) { return p.svc.Download(ctx, el.ID) },
			func() { go p.abrirConEquipo(el) })
		return
	}
	go p.abrirConEquipo(el)
}

// abrirConEquipo le pasa el fichero a su app o al sistema y, si nadie lo abre, lo dice.
// Corre fuera de la interfaz.
func (p *Pantalla) abrirConEquipo(el Elemento) {
	if p.EnEquipo.Abrir == nil {
		return
	}
	if err := p.EnEquipo.Abrir(el); err != nil {
		p.avisar(ui.AvisoPeligro, p.textoEquipo("noSeAbre"), el.Nombre)
	}
}

// editarFoto lleva la imagen al editor de Photos y, si no se puede, lo dice. Corre fuera
// de la interfaz.
func (p *Pantalla) editarFoto(el Elemento) {
	if err := p.EnEquipo.EditarFoto(el); err != nil {
		p.avisar(ui.AvisoPeligro, p.textoEquipo("noSeAbre"), el.Nombre)
	}
}

// copiarA es «Copiar a…»: el «Descargar» del equipo.
func (p *Pantalla) copiarA(el Elemento) {
	if p.EnEquipo.CopiarA == nil {
		return
	}
	p.EnEquipo.CopiarA(el, func(destino string, err error) {
		if err != nil {
			p.avisar(ui.AvisoPeligro, p.textoEquipo("noSeCopia"), el.Nombre)
			return
		}
		p.avisar(ui.AvisoExito, p.tr.T("files.copy.done", i18n.Vars{"name": el.Nombre}), destino)
	})
}

// mostrarEnSistema abre la carpeta del sistema con el elemento. Corre fuera de la interfaz.
func (p *Pantalla) mostrarEnSistema(el Elemento) {
	if p.EnEquipo.MostrarEnCarpeta == nil {
		return
	}
	if err := p.EnEquipo.MostrarEnCarpeta(el); err != nil {
		p.avisar(ui.AvisoPeligro, p.textoEquipo("noSeAbre"), el.Nombre)
	}
}

// FuncionIA convierte la función de IA de la cáscara local (`local.App.Asistir`) en un
// Asistente para «Resumir» (DR-10). nil sigue siendo nil: sin IA, no hay acción.
func FuncionIA(f func(context.Context, api.PeticionIA) (string, error)) Asistente {
	if f == nil {
		return nil
	}
	return funcionIA(f)
}

type funcionIA func(context.Context, api.PeticionIA) (string, error)

func (f funcionIA) Asistir(ctx context.Context, q api.PeticionIA) (string, error) { return f(ctx, q) }
