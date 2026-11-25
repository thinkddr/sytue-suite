// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # «Documento nuevo» del widget nativo
//
// La acción rápida del widget de Docus, Calcus y Diapos (`sytue-docs:/?action=create`,
// `sytue-sheets:` y `sytue-slides:`) PREGUNTA antes de crear: un enlace del esquema lo
// puede abrir cualquiera y nunca escribe nada solo (ADR 0078). La pregunta es la de
// «Nuevo documento» (o «Nueva hoja de cálculo», «Nueva presentación») con su título, que
// puede quedarse vacío; con el sí se crea en blanco y se abre, como «En blanco» de la
// portada, que sigue creando en el acto porque la pulsa la persona.

// PreguntarCrear pide la pregunta de «Documento nuevo» sobre `lienzo` (nil: el de la
// portada). Si la portada aún no tiene su lista —sin la vista montada, cargando, sin
// sesión—, la pregunta espera y sale al cargarla; un segundo enlace con una pendiente o
// abierta no abre otra. Se llama en el hilo de la interfaz.
func (p *Portada) PreguntarCrear(lienzo fyne.Canvas) {
	if p.preguntandoNuevo {
		return // ya está a la vista
	}
	p.nuevoPendiente = true
	if lienzo != nil {
		p.lienzoNuevo = lienzo
	}
	p.atenderNuevo()
}

// NuevoPendiente dice si hay una pregunta de «Documento nuevo» esperando a la lista.
func (p *Portada) NuevoPendiente() bool { return p.nuevoPendiente }

// atenderNuevo enseña la pregunta pendiente en cuanto se puede crear: con servicio, red y
// la lista ya cargada (Listo o Vacio, que es cuando la cuenta está montada).
func (p *Portada) atenderNuevo() {
	if !p.nuevoPendiente || p.preguntandoNuevo || p.Svc == nil || p.Creando || !p.hayRed() ||
		(p.Estado != Listo && p.Estado != Vacio) {
		return
	}
	lienzo := p.lienzoNuevo
	if lienzo == nil {
		lienzo = p.lienzo()
	}
	if lienzo == nil {
		return
	}
	p.nuevoPendiente, p.preguntandoNuevo = false, true
	campo := ui.NuevaEntrada("")
	var dlg *ui.Dialogo
	cerrar := func() {
		p.preguntandoNuevo = false
		dlg.Cerrar()
	}
	crear := func() {
		if !p.preguntandoNuevo {
			return // ya se contestó: Intro y el botón a la vez crean uno
		}
		titulo := strings.TrimSpace(campo.Texto())
		cerrar()
		p.CrearCon(titulo)
	}
	campo.AlTeclear = func(e *fyne.KeyEvent) bool {
		if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
			crear()
			return true
		}
		return false
	}
	cancelar := ui.NuevoBoton(p.t("action.cancel"), cerrar).Con(ui.Fantasma, ui.TallaMD)
	aceptar := ui.NuevoBoton(p.t("action.ok"), crear)
	rotulo := ui.NuevoRotulo(p.t("rename.field"), tema.TextoSM, ui.SemiNegrita, "text")
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, rotulo, campo)
	pie := container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, aceptar)
	dlg = ui.NuevoDialogo(p.t(claveNuevo(p.Tipo)), cuerpo).Con(ui.DialogoMD, pie)
	dlg.RotuloCerrar = p.t("ui.close")
	dlg.AlCerrar = cerrar
	dlg.Mostrar(lienzo)
}

// Crear da de alta un documento en blanco del tipo del producto y lo abre: «En blanco».
func (p *Portada) Crear() { p.CrearCon("") }

// CrearCon es Crear con un título (vacío: el que ponga el servicio). Sin id en la
// respuesta NO se navega: se avisa.
func (p *Portada) CrearCon(titulo string) {
	if p.Svc == nil || p.Creando || !p.hayRed() {
		return
	}
	p.Creando = true
	p.Repintar()
	p.lanzar(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		d, err := p.Svc.Create(ctx, p.Tipo.Kind(), titulo)
		fyne.Do(func() { p.creado(d, err) })
	})
}

func (p *Portada) creado(d api.Document, err error) {
	p.Creando = false
	if err != nil || d.ID == "" {
		if p.Avisos != nil {
			p.Avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro,
				p.t("create.error.title"), p.t("create.error.description")))
		}
		p.Repintar()
		return
	}
	p.Repintar()
	if p.AlAbrir != nil {
		p.AlAbrir(d)
	}
}
