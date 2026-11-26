// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Unidades de la nube (Sytue Office, DR-13)
//
// Las cuentas de nube que la persona conectó en Ajustes (Sytue Drive o cualquier WebDAV,
// Google Drive, OneDrive) salen en el lateral, bajo «Tus nubes», como unidades más. Una
// unidad se mira con el MISMO explorador que la carpeta del disco: su nombre es la
// primera miga, se entra en sus carpetas, se abre un fichero (se baja a una caché y se
// abre en su app; lo guardado sube solo, `local/archivos.Remotas`) y se sube o se suelta.
// Crear carpetas, renombrar, mover y tirar los hace el proveedor; mover a OTRA unidad
// (o al disco) copia, comprueba y quita el origen (mover_unidades.go); la estrella y las
// etiquetas se guardan en este equipo. Las versiones no se ofrecen sobre ella.

// UnidadEquipo es una unidad de la nube: el id de su carpeta raíz y su nombre.
type UnidadEquipo struct {
	ID     string
	Nombre string
}

// montarUnidades pone las unidades en el lateral y hace que elegir una sección del
// lateral salga de la unidad. Se llama al montar la pantalla del equipo.
func (p *Pantalla) montarUnidades() {
	p.lateral.AlElegir = func(s Seccion) {
		if p.unidad.ID != "" {
			p.salirDeUnidad()
		}
		p.irA(s)
	}
	p.lateral.AlElegirUnidad = p.AbrirUnidad
	p.RefrescarUnidades()
}

// salirDeUnidad vuelve a la carpeta del disco.
func (p *Pantalla) salirDeUnidad() {
	p.unidad = UnidadEquipo{}
	p.explor.Ruta, p.explor.RaizNombre = nil, ""
	p.lateral.UnidadActiva = ""
}

// RefrescarUnidades vuelve a leer las cuentas (pueden haber cambiado en Ajustes). Si la
// unidad que se mira ya no está, se vuelve al disco. Se llama en la rutina de la
// interfaz; no repinta.
func (p *Pantalla) RefrescarUnidades() {
	if p.EnEquipo == nil || p.EnEquipo.Unidades == nil {
		return
	}
	p.lateral.Unidades = p.EnEquipo.Unidades()
	if p.unidad.ID == "" {
		return
	}
	for _, u := range p.lateral.Unidades {
		if u.ID == p.unidad.ID {
			return
		}
	}
	p.salirDeUnidad()
}

// AbrirUnidad entra en la raíz de una unidad. Se llama en la rutina de la interfaz.
func (p *Pantalla) AbrirUnidad(u UnidadEquipo) {
	if p.explor.EnBusqueda {
		p.salirDeLaBusqueda()
	}
	p.unidad = u
	p.explor.Ruta, p.explor.RaizNombre = nil, u.Nombre
	p.lateral.UnidadActiva = u.ID
	p.irA(MiUnidad)
}

// EnUnidad dice si se está mirando una unidad de la nube.
func (p *Pantalla) EnUnidad() bool { return p.unidad.ID != "" }

// esRemoto dice si un elemento es de una unidad de la nube.
func (p *Pantalla) esRemoto(el Elemento) bool {
	return p.EnEquipo != nil && p.EnEquipo.EsRemoto != nil && p.EnEquipo.EsRemoto(el.ID)
}

// accionesRemotas es el «⋯» de un elemento de una unidad: traérselo («Copiar a…»), lo
// que hace el proveedor (renombrar, mover dentro de la unidad, tirar a SU papelera) y, con
// MoverEntreUnidades, mover a otra unidad; la estrella y las etiquetas, que se guardan en
// este equipo (`local/archivos.Remotas`), y leerlo.
func (p *Pantalla) accionesRemotas(el Elemento, favorito bool) []accionMenu {
	t := func(clave string) string { return p.tr.T(clave, nil) }
	out := []accionMenu{}
	if !el.Carpeta {
		out = append(out, accionMenu{texto: p.textoEquipo("copiarA"), cuerpo: glifoCopia, hacer: func() { p.copiarA(el) }})
	}
	if el.ID == "" {
		return out
	}
	out = append(out,
		accionMenu{texto: t("action.rename"), cuerpo: glifoLapiz, hacer: func() { p.pedirRenombrar(el) }},
		accionMenu{texto: t("action.move"), cuerpo: glifoMover, hacer: func() { p.pedirMover(el) }},
	)
	if !el.Carpeta {
		// «nombre (2).ext» al lado, bajado y vuelto a subir (una carpeta no se copia en la nube).
		out = append(out, accionMenu{texto: t("action.copy"), cuerpo: glifoCopia, hacer: func() { go p.copiar(el) }})
	}
	if p.EnEquipo.Etiquetas != nil {
		out = append(out, accionMenu{texto: p.textoEquipo("etiquetas"), icono: iconos.Etiqueta, hacer: func() { p.pedirEtiquetas(el) }})
	}
	if p.IA != nil && !el.Carpeta && EsTexto(el) {
		out = append(out, accionMenu{texto: t("ai.summarize"), icono: iconos.IA, hacer: func() { p.pedirResumen(el) }})
	}
	// La estrella es de este equipo, no de la cuenta: el rótulo lo dice.
	clave := "favoritoNube"
	if favorito {
		clave = "favoritoNubeQuitar"
	}
	out = append(out,
		accionMenu{texto: p.textoEquipo(clave), icono: iconos.DriveEstrella, hacer: func() { p.alternarFavorito(el, !favorito) }},
		accionMenu{texto: t("action.delete"), cuerpo: glifoPapelera, peligro: true, hacer: func() { go p.aLaPapelera(el) }},
	)
	return out
}

// unidadDe es la unidad de un id ("" el disco).
func (p *Pantalla) unidadDe(id string) string {
	if p.EnEquipo == nil || p.EnEquipo.UnidadDe == nil {
		return ""
	}
	return p.EnEquipo.UnidadDe(id)
}

// nombreUnidad es el nombre de una unidad: «Mi unidad» para el disco.
func (p *Pantalla) nombreUnidad(id string) string {
	for _, u := range p.lateral.Unidades {
		if u.ID == id {
			return u.Nombre
		}
	}
	return p.tr.T("nav.myDrive", nil)
}

// unidadPara entra en la unidad de un elemento que se abre desde «Favoritos» o una
// búsqueda (o vuelve al disco), sin cargar nada: lo carga quien llama. Se llama en la
// rutina de la interfaz.
func (p *Pantalla) unidadPara(el Elemento) {
	u := p.unidadDe(el.ID)
	if u == p.unidad.ID {
		return
	}
	if u == "" {
		p.salirDeUnidad()
		return
	}
	for _, x := range p.lateral.Unidades {
		if x.ID == u {
			p.unidad = x
			p.explor.Ruta, p.explor.RaizNombre = nil, x.Nombre
			p.lateral.UnidadActiva = x.ID
			return
		}
	}
}

// Avisar enseña un aviso de la casa (lo que pasó con una subida a la nube hecha sin que
// la persona lo pidiera en ese momento, por ejemplo). Se puede llamar desde cualquier
// rutina.
func (p *Pantalla) Avisar(peligro bool, titulo, detalle string) {
	tipo := ui.AvisoExito
	if peligro {
		tipo = ui.AvisoPeligro
	}
	p.avisar(tipo, titulo, detalle)
}

// elementosNuevoEnUnidad es «Nuevo» dentro de una unidad: una carpeta (la crea el
// proveedor) y subir.
func (p *Pantalla) elementosNuevoEnUnidad() []fyne.CanvasObject {
	return []fyne.CanvasObject{
		ui.NuevoElementoMenu(p.tr.T("files.new.folder", nil), iconos.DriveCarpeta, p.pedirCarpeta),
		ui.NuevoElementoMenu(p.tr.T("files.upload", nil), iconos.Subir, p.elegirYSubir),
	}
}
