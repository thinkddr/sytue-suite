// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La costura de «Importar desde otro programa» con el servicio. Como el resto de
// `Pantalla`, lo que habla con la red va en `p.fondo` y vuelve a la interfaz por
// `enHilo`; una respuesta que llega con el asistente ya cerrado no toca nada.

// PuedeImportarDesde es la condición de la web (`permisos.administrador`): quien
// administra la organización, que es a quien el servicio deja usar esas rutas.
func (p *Pantalla) PuedeImportarDesde() bool {
	return p.svc != nil && p.lienzo != nil && p.SabePermisos && p.Permisos.Administrador
}

// AbrirImportarDesde abre el asistente en un diálogo `md`, el `Modal size="md"` de la
// web. Mientras se prueba, se guarda o se importa no se cierra (`dismissable`).
func (p *Pantalla) AbrirImportarDesde() *ui.Dialogo {
	if !p.PuedeImportarDesde() || p.importando != nil {
		return nil
	}
	x := NuevoImportarDesde(p.tr)
	x.Ventana = p.Ventana.Width
	if u := p.PoliticaURL; u != "" {
		x.AlPolitica = func() { abrirURL(u) }
	}
	var d *ui.Dialogo
	cerrar := func() {
		if x.Ocupado || x.Probando {
			return
		}
		p.importando = nil
		p.cerrarDialogoIA(d)
	}
	x.AlCerrar = cerrar
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := nuevoVivo(x.Objeto(v, anchoCuerpoDialogo(ui.DialogoMD, p.Ventana.Width)))
	d = ui.NuevoDialogo(p.t("migrate.title"), cuerpo.c).Con(ui.DialogoMD, nil)
	pintar := func() {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		x.Ventana = p.Ventana.Width
		cuerpo.poner(x.Objeto(v, anchoCuerpoDialogo(ui.DialogoMD, p.Ventana.Width)))
		d.Recolocar()
	}
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	x.AlCambio = pintar
	p.abrirDialogoIA(d, pintar)
	d.AlCerrar = cerrar
	p.importando = x
	p.engancharImportarDesde(x, pintar)
	d.Mostrar(p.lienzo)
	// Las conexiones ya guardadas rellenan el identificador; si no llegan, se escribe.
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		cx, err := p.svc.Conexiones(ctx)
		enHilo(func() {
			if err == nil && p.importando == x {
				x.PonerGuardadas(cx)
				pintar()
			}
		})
	})
	return d
}

// engancharImportarDesde conecta las tres acciones del asistente con el servicio.
func (p *Pantalla) engancharImportarDesde(x *ImportarDesde, pintar func()) {
	vivo := func() bool { return p.importando == x }
	x.AlProbar = func(proveedor, instancia string) {
		p.fondo(func() {
			ctx, cancelar := p.ctx()
			defer cancelar()
			err := p.svc.ProbarConexion(ctx, proveedor, instancia)
			enHilo(func() {
				if vivo() {
					x.Probado(err, MensajeDeError(err, p.tr))
					pintar()
				}
			})
		})
	}
	x.AlGuardar = func(proveedor, instancia string) {
		p.fondo(func() {
			ctx, cancelar := p.ctx()
			defer cancelar()
			err := p.svc.GuardarConexion(ctx, proveedor, instancia)
			enHilo(func() {
				if vivo() {
					x.Guardado(err, MensajeDeError(err, p.tr))
					pintar()
				}
			})
		})
	}
	x.AlImportar = func(proveedor string, objetos []string) {
		p.fondo(func() {
			// Traer miles de filas tarda: el plazo es el de Sytue AI, no el de una lista.
			ctx, cancelar := p.ctxIA()
			defer cancelar()
			r, err := p.svc.ImportarDesde(ctx, proveedor, objetos)
			enHilo(func() {
				if err == nil {
					// `onImportado`: la lista abierta y los contadores se vuelven a pedir.
					p.revision++
					for _, o := range r {
						p.olvidar(o.Modelo)
					}
				}
				if vivo() {
					x.Importado(r, err, MensajeDeError(err, p.tr))
					pintar()
				}
				p.Repintar()
			})
		})
	}
}
