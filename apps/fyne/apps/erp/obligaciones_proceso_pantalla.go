// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// AbrirProcesoObligacion abre el mismo estado compartido que ProcesoObligacion.tsx.
// Reutiliza la costura de diálogos vivos, documentos y banco de la pantalla.
func (p *Pantalla) AbrirProcesoObligacion(ob api.ERPObligacion) {
	if p.lienzo == nil {
		return
	}
	x := NuevoProcesoObligacion(p.tr, ob)
	x.SoloLectura = !p.PuedeEscribir("obligaciones")
	x.PuedeLeer, x.PuedeEscribir = p.PuedeLeer, p.PuedeEscribir
	x.PuedeAccion = p.PuedeAccion
	x.Banco = p.banco
	if p.oblig.Datos != nil && p.oblig.Datos.Moneda != "" {
		x.Moneda = p.oblig.Datos.Moneda
	}
	p.oblig.Proceso = x
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	x.Ventana = p.Ventana.Width
	cuerpo := nuevoVivo(x.Cuerpo(v, anchoCuerpoDialogo(ui.DialogoLG, p.Ventana.Width)))
	d := ui.NuevoDialogo(ob.Nombre+" · "+ob.Periodo, cuerpo.c).Con(ui.DialogoLG, nil)
	pintar := func() {
		if p.oblig.Proceso != x {
			return
		}
		x.Ventana = p.Ventana.Width
		x.SoloLectura = !p.PuedeEscribir("obligaciones")
		d.NoDescartable = x.Ocupado
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		cuerpo.poner(x.Cuerpo(v, anchoCuerpoDialogo(ui.DialogoLG, p.Ventana.Width)))
		d.Recolocar()
	}
	x.AlCambio = pintar
	p.abrirDialogoIA(d, pintar)
	cerrar := func() {
		if x.Ocupado {
			return
		}
		p.oblig.Proceso = nil
		p.cerrarDialogoIA(d)
	}
	x.AlCerrar, d.AlCerrar = cerrar, cerrar
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	x.AlComprobar = func() { p.comprobarProcesoObligacion(x) }
	x.AlAvanzar = func() { p.avanzarProcesoObligacion(x) }
	x.AlCalcular = func() { p.calcularProcesoObligacion(x) }
	x.AlFacturas = func() { p.facturasProcesoObligacion(x, false) }
	x.AlMas = func() { p.facturasProcesoObligacion(x, true) }
	x.AlAsentar = func(r Registro) { p.asentarProcesoObligacion(x, r) }
	x.AlElegir = func(paso int) {
		if paso == 1 && x.Obligacion.Calculable {
			p.calcularProcesoObligacion(x)
		}
	}
	x.AlAbrirRegistro = func(r Registro) {
		if p.AbrirApp != nil {
			p.AbrirApp("erp", "/?m="+url.QueryEscape(r.Modelo)+"&r="+url.QueryEscape(r.ID))
			return
		}
		cerrar()
		p.abrirRegistro(r.Modelo, r.ID)
	}
	x.AlNominas = func() {
		if p.AbrirApp != nil {
			p.AbrirApp("erp", "/?m=nomina")
			return
		}
		if m, ok := p.modeloPorClave("nomina"); ok {
			cerrar()
			p.Abrir(m)
		}
	}
	x.AlBanco = func() {
		x.VerBanco = !x.VerBanco
		if x.VerBanco {
			p.engancharBanco()
			p.banco.SoloLectura = !p.PuedeAccion("contabilidad", "conciliar")
			p.cargarBanco()
		}
		x.cambio()
	}
	p.conectarDocumentosProcesoObligacion(x)
	d.Mostrar(p.lienzo)
	p.cargarProcesoObligacion(x)
}

func (p *Pantalla) conectarDocumentosProcesoObligacion(x *ProcesoObligacion) {
	x.AlAbrirAdjunto = p.abrirAdjunto
	x.AlBorrarAdjunto = func(reg string, a api.ERPAdjunto) {
		if x.SoloLectura || p.svc == nil {
			return
		}
		p.fondo(func() {
			ctx, cancel := p.ctx()
			defer cancel()
			err := p.svc.BorrarAdjunto(ctx, a.ID)
			enHilo(func() {
				if p.oblig.Proceso != x {
					return
				}
				if err != nil {
					x.Fallo = MensajeDeError(err, p.tr)
					x.cambio()
					return
				}
				// El último justificante borrado devuelve el proceso a fase 3.
				// Releer ambos estados evita mostrar un archivo ya incompleto.
				p.cargarProcesoObligacion(x)
				p.cargarObligaciones()
			})
		})
	}
	if p.ElegirFicheros != nil {
		x.AlSubir = func(reg string) {
			if x.SoloLectura {
				return
			}
			p.subirDocumento("obligacion", reg, func(subiendo bool) {
				if !subiendo && p.oblig.Proceso == x {
					p.documentosProcesoObligacion(x)
				}
			})
		}
	}
}

func (p *Pantalla) cargarProcesoObligacion(x *ProcesoObligacion) {
	if p.svc == nil {
		return
	}
	x.Fallo = ""
	x.cambio()
	p.fondo(func() {
		ctx, cancel := p.ctx()
		defer cancel()
		progreso, err := p.svc.ProcesoObligacion(ctx, x.Obligacion.Clave, x.Obligacion.Periodo)
		enHilo(func() {
			if p.oblig.Proceso != x {
				return
			}
			if err != nil {
				x.Fallo = MensajeDeError(err, p.tr)
				x.cambio()
				return
			}
			x.Cargado(progreso)
			x.cambio()
			p.documentosProcesoObligacion(x)
			if x.Paso == 1 && x.Obligacion.Calculable {
				p.calcularProcesoObligacion(x)
			}
			if x.Obligacion.Pendientes["facturas"] > 0 && x.puedeLeer("facturacion") {
				p.facturasProcesoObligacion(x, false)
			}
		})
	})
}

func (p *Pantalla) calendarioProceso(ctx context.Context, x *ProcesoObligacion) (api.ERPObligacion, api.ERPObligaciones, error) {
	anio, _ := strconv.Atoi(x.Obligacion.Periodo[:min(4, len(x.Obligacion.Periodo))])
	d, err := p.svc.Obligaciones(ctx, anio)
	if err != nil {
		return api.ERPObligacion{}, d, err
	}
	for _, ob := range d.Obligaciones {
		if IDObligacion(ob) == IDObligacion(x.Obligacion) {
			return ob, d, nil
		}
	}
	return api.ERPObligacion{}, d, errors.New(p.t("error.generic"))
}

func (p *Pantalla) comprobarProcesoObligacion(x *ProcesoObligacion) {
	if p.svc == nil || x.Ocupado {
		return
	}
	if x.Progreso == nil {
		p.cargarProcesoObligacion(x)
		return
	}
	p.fondo(func() {
		ctx, cancel := p.ctx()
		defer cancel()
		ob, d, err := p.calendarioProceso(ctx, x)
		enHilo(func() {
			if p.oblig.Proceso != x {
				return
			}
			if err != nil {
				x.Fallo = MensajeDeError(err, p.tr)
			} else {
				x.Obligacion = ob
				x.Fallo = ""
				if p.oblig.Anio == d.Anio {
					p.oblig.Datos = &d
				}
			}
			x.cambio()
			p.Repintar()
			if err == nil && ob.Pendientes["facturas"] > 0 && x.puedeLeer("facturacion") {
				p.facturasProcesoObligacion(x, false)
			}
		})
	})
}

// avanzarProcesoObligacion comprueba el calendario antes de avanzar. El cliente
// nunca convierte un botón en aceptación fiscal: adopta la respuesta persistida.
func (p *Pantalla) avanzarProcesoObligacion(x *ProcesoObligacion) {
	if p.svc == nil || !x.PuedeContinuar() {
		return
	}
	paso, referencia := x.Paso, x.Referencia()
	x.Ocupado = true
	x.Fallo = ""
	x.cambio()
	p.fondo(func() {
		ctx, cancel := p.ctx()
		defer cancel()
		ob, d, pendientes, err := p.calendarioAntesDeAvanzarProceso(ctx, x, paso)
		var progreso api.ERPProcesoObligacion
		if err == nil {
			progreso, err = p.svc.AvanzarProcesoObligacion(ctx, x.Obligacion.Clave, x.Obligacion.Periodo, paso+1, referencia)
		}
		enHilo(func() {
			p.adoptarAvanceProcesoObligacion(x, ob, d, progreso, pendientes, err)
		})
	})
}

func (p *Pantalla) calendarioAntesDeAvanzarProceso(ctx context.Context, x *ProcesoObligacion, paso int) (api.ERPObligacion, api.ERPObligaciones, bool, error) {
	if paso >= 3 {
		return api.ERPObligacion{}, api.ERPObligaciones{}, false, nil
	}
	ob, d, err := p.calendarioProceso(ctx, x)
	if err != nil {
		return ob, d, false, err
	}
	for _, n := range ob.Pendientes {
		if n > 0 {
			return ob, d, true, errors.New(p.t("process.resolve"))
		}
	}
	return ob, d, false, nil
}

func (p *Pantalla) adoptarAvanceProcesoObligacion(x *ProcesoObligacion, ob api.ERPObligacion, d api.ERPObligaciones, progreso api.ERPProcesoObligacion, pendientes bool, err error) {
	if p.oblig.Proceso != x {
		return
	}
	x.Ocupado = false
	if d.Anio != 0 {
		x.Obligacion = ob
		if p.oblig.Anio == d.Anio {
			p.oblig.Datos = &d
		}
	}
	if err != nil {
		if pendientes {
			x.Paso = 0
			x.Fallo = p.t("process.resolve")
		} else {
			x.Fallo = MensajeDeError(err, p.tr)
		}
		x.cambio()
		return
	}
	x.Cargado(progreso)
	x.Obligacion.Proceso = &progreso
	x.cambio()
	p.cargarObligaciones()
	if x.Paso == 1 && x.Obligacion.Calculable {
		p.calcularProcesoObligacion(x)
	}
	p.documentosProcesoObligacion(x)
}

func (p *Pantalla) calcularProcesoObligacion(x *ProcesoObligacion) {
	if p.svc == nil {
		return
	}
	x.Calculo = nil
	x.FalloCalculo = ""
	x.Revisado = false
	x.cambio()
	p.fondo(func() {
		ctx, cancel := p.ctx()
		defer cancel()
		c, err := p.svc.CalculoObligacion(ctx, x.Obligacion.Clave, x.Obligacion.Periodo)
		enHilo(func() {
			if p.oblig.Proceso != x || x.Paso != 1 {
				return
			}
			if err != nil {
				x.FalloCalculo = MensajeDeError(err, p.tr)
			} else {
				x.Calculo = &c
			}
			x.cambio()
		})
	})
}

func (p *Pantalla) documentosProcesoObligacion(x *ProcesoObligacion) {
	if p.svc == nil {
		return
	}
	for _, doc := range x.documentos() {
		reg := RegistroDocumento(x.Obligacion, doc.Clave)
		delete(x.Adjuntos, reg)
		p.fondo(func() {
			ctx, cancel := p.ctx()
			defer cancel()
			adj, err := p.svc.Adjuntos(ctx, "obligacion", reg)
			enHilo(func() {
				if p.oblig.Proceso != x {
					return
				}
				if err != nil {
					x.Fallo = MensajeDeError(err, p.tr)
				} else {
					x.Adjuntos[reg] = vacioSiFalla(adj, nil)
					p.oblig.Adjuntos[reg] = x.Adjuntos[reg]
				}
				x.cambio()
			})
		})
	}
	x.cambio()
}

func (p *Pantalla) facturasProcesoObligacion(x *ProcesoObligacion, mas bool) {
	if p.svc == nil || !x.puedeLeer("facturacion") || x.Ocupada != "" || x.CargandoMas {
		return
	}
	cursor := ""
	if mas {
		if x.Facturas == nil || x.Facturas.Siguiente == "" {
			return
		}
		cursor = x.Facturas.Siguiente
		x.CargandoMas = true
	} else {
		x.Facturas = nil
	}
	x.FalloFacturas = ""
	x.cambio()
	p.fondo(func() {
		ctx, cancel := p.ctx()
		defer cancel()
		pagina, err := p.svc.FacturasPendientesObligacion(ctx, x.Obligacion.Clave, x.Obligacion.Periodo, cursor, 0)
		enHilo(func() {
			if p.oblig.Proceso != x {
				return
			}
			x.CargandoMas = false
			if err != nil {
				x.FalloFacturas = MensajeDeError(err, p.tr)
			} else {
				if mas && x.Facturas != nil {
					pagina.Registros = append(x.Facturas.Registros, pagina.Registros...)
				}
				x.Facturas = &pagina
			}
			x.cambio()
		})
	})
}

func (p *Pantalla) asentarProcesoObligacion(x *ProcesoObligacion, r Registro) {
	if p.svc == nil || !x.puedeAccion("facturacion", "asentar") || x.Ocupada != "" || x.CargandoMas {
		return
	}
	x.Ocupada = r.ID
	x.FalloFacturas = ""
	x.cambio()
	p.fondo(func() {
		ctx, cancel := p.ctx()
		defer cancel()
		_, err := p.svc.Asentar(ctx, r.Modelo, r.ID)
		enHilo(func() {
			if p.oblig.Proceso != x {
				return
			}
			x.Ocupada = ""
			if err != nil {
				x.FalloFacturas = MensajeDeError(err, p.tr)
				x.cambio()
				return
			}
			p.facturasProcesoObligacion(x, false)
			p.comprobarProcesoObligacion(x)
		})
	})
}
