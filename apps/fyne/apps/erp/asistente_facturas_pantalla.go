// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"maps"
	"os"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La costura del asistente de facturas con el servicio. Como el resto de `Pantalla`,
// lo que habla con la red va en `p.fondo` y vuelve a la interfaz por `enHilo`.

// PuedeAsistirFacturas es la condición del botón de la lista: el módulo de facturación,
// fuera de la papelera, con permiso para crear alguna de las dos facturas.
func (p *Pantalla) PuedeAsistirFacturas() bool {
	l := p.lista
	return p.svc != nil && p.lienzo != nil && l.Modelo.Modulo == "facturacion" && !l.Papelera && p.puedeCrearFacturas()
}

// puedeCrearFacturas es poder crear alguna de las dos: el asistente elige entre ellas.
func (p *Pantalla) puedeCrearFacturas() bool {
	return p.PuedeCrear(api.ERPFacturaCompra) || p.PuedeCrear(api.ERPFacturaVenta)
}

// engancharAsistenteFacturas pone (o quita) el botón del asistente en la lista que se va a pintar.
func (p *Pantalla) engancharAsistenteFacturas() {
	p.lista.AlAsistenteFacturas = nil
	if p.PuedeAsistirFacturas() {
		p.lista.AlAsistenteFacturas = func() { p.AbrirAsistenteFacturas() }
	}
}

// AbrirAsistenteFacturas abre el asistente en un diálogo `lg`, el `Modal size="lg"`.
func (p *Pantalla) AbrirAsistenteFacturas() *ui.Dialogo {
	if p.lienzo == nil || p.svc == nil || p.facturas != nil || !p.puedeCrearFacturas() {
		return nil
	}
	x := NuevoAsistenteFacturas(p.tr, p.Modelos(), p.PuedeCrear,
		func(modulo string) bool { return p.PuedeAccion(modulo, "asentar") })
	x.Ventana = p.Ventana.Width
	var d *ui.Dialogo
	// Mientras lee, sube o asienta no se cierra: la respuesta llegaría a un asistente
	// que ya no está, y un original a medio subir es justo el resto que no se quiere.
	cerrar := func() {
		if x.Ocupado {
			return
		}
		p.facturas = nil
		p.cerrarDialogoIA(d)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := nuevoVivo(x.Cuerpo(v, anchoCuerpoDialogo(ui.DialogoLG, p.Ventana.Width)))
	// El pie es SIEMPRE el mismo contenedor: el diálogo lo marca como hoja en el
	// teléfono por su layout, y un envoltorio lo escondería. Cambian sus acciones.
	pie := x.Pie(cerrar)
	d = ui.NuevoDialogo(p.t("invoiceAssist.title"), cuerpo.c).Con(ui.DialogoLG, pie)
	pintar := func() {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		x.Ventana = p.Ventana.Width
		cuerpo.poner(x.Cuerpo(v, anchoCuerpoDialogo(ui.DialogoLG, p.Ventana.Width)))
		pie.Objects = x.Acciones(cerrar)
		pie.Refresh()
		d.Recolocar()
	}
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	x.AlCambio = pintar
	p.abrirDialogoIA(d, pintar)
	d.AlCerrar = cerrar
	p.facturas = x

	if p.ElegirFicheros != nil {
		// Se apunta lo elegido tal cual: el tipo y el tope se comprueban al preparar,
		// con el mismo aviso que la web.
		x.AlElegirFichero = func() {
			p.ElegirFicheros(func(rutas []string) {
				if len(rutas) == 0 || p.facturas != x {
					return
				}
				var tamano int64
				if st, err := os.Stat(rutas[0]); err == nil {
					tamano = st.Size()
				}
				x.PonerFichero(rutas[0], tamano)
			})
		}
	}
	x.AlPreparar = func() { p.leerFactura(x) }
	x.AlRevisar = func() { p.revisarFactura(x) }
	x.AlAdjuntar = func() { p.adjuntarFactura(x) }
	x.AlAsentar = func() { p.asentarFactura(x) }
	x.AlAbrir = func() {
		if x.Ocupado || x.Registro == nil {
			return
		}
		clave, id := x.Clave, x.Registro.ID
		cerrar()
		p.abrirRegistro(clave, id)
	}
	d.Mostrar(p.lienzo)
	return d
}

// leerFactura manda el original: el PDF a Sytue AI y el XML a la lectura de facturas
// de proveedor. No crea nada.
func (p *Pantalla) leerFactura(x *AsistenteFacturas) {
	if p.svc == nil || x.Ocupado || x.Fichero == "" {
		return
	}
	x.Ocupado, x.Fallo = true, ""
	x.cambio()
	clave, ruta, pdf := x.Clave, x.Fichero, esPDF(x.Fichero)
	p.fondo(func() {
		var r api.ERPRelleno
		datos, err := os.ReadFile(ruta) //nolint:gosec // G304: la ruta la eligió quien usa la app en el selector del sistema
		if err == nil {
			ctx, cancelar := p.ctxIA()
			defer cancelar()
			if pdf {
				r, err = p.svc.RellenarFichaPDF(ctx, clave, datos)
			} else {
				r, err = p.svc.LeerFacturaXML(ctx, datos)
			}
		}
		enHilo(func() {
			if p.facturas != x {
				return
			}
			if err != nil {
				x.Ocupado = false
				x.Fallo = MensajeDeError(err, p.tr)
				if pdf && estadoDe(err) == 422 {
					x.Fallo = p.t("ai.pdfUnreadable")
				}
				x.cambio()
				return
			}
			x.Leido(r)
		})
	})
}

// revisarFactura abre el alta rápida con lo leído. Es ella quien crea la factura;
// cancelarla deja el asistente en la revisión. Si se leyó un proveedor que aún no está
// en contactos, crear la relación desde el formulario lo propone con sus datos.
func (p *Pantalla) revisarFactura(x *AsistenteFacturas) {
	m, ok := x.Modelo()
	if !ok || x.Ocupado || x.Registro != nil {
		return
	}
	semilla := &Registro{Data: maps.Clone(x.Semilla)}
	if p.AltaRapida(m, semilla, x.Etiquetas, func(r Registro) {
		// Como `onCreado` en la web: la factura nueva sale en la lista que se está
		// mirando y cuentan sus tareas (el alta rápida solo olvida la caché).
		p.contarTareas()
		if p.lista.Modelo.Clave == m.Clave {
			p.pedirLista()
		}
		if p.facturas == x {
			x.Creada(r)
		}
	}) == nil {
		return
	}
	alta := p.altas[len(p.altas)-1].ficha
	previa := alta.CrearRelacion
	alta.CrearRelacion = func(clave string, creado func(Registro)) {
		cm, hay := p.modeloPorClave(clave)
		if clave != "contacto" || x.Contacto == nil || !hay {
			previa(clave, creado)
			return
		}
		p.AltaRapida(cm, &Registro{Data: maps.Clone(x.Contacto)}, nil, func(r Registro) {
			x.Contacto = nil
			x.Etiquetas[r.ID] = NombreDe(r)
			creado(r)
		})
	}
}

// adjuntarFactura sube el MISMO original que se leyó: elegirlo otra vez sería pedir
// dos veces lo mismo.
func (p *Pantalla) adjuntarFactura(x *AsistenteFacturas) {
	if p.svc == nil || x.Ocupado || x.Registro == nil || x.Fichero == "" {
		return
	}
	x.Ocupado, x.Fallo = true, ""
	x.cambio()
	clave, id, ruta := x.Clave, x.Registro.ID, x.Fichero
	p.fondo(func() {
		_, err := p.subirRuta(clave, id, ruta)
		enHilo(func() {
			if p.facturas != x {
				return
			}
			if err != nil {
				x.Ocupado, x.Fallo = false, MensajeDeError(err, p.tr)
				x.cambio()
				return
			}
			p.avisar(ui.AvisoExito, p.t("docs.uploaded"))
			p.contarTareas()
			x.Adjuntado()
		})
	})
}

// asentarFactura la lleva al libro y refresca la lista: ya no es un borrador.
func (p *Pantalla) asentarFactura(x *AsistenteFacturas) {
	if p.svc == nil || x.Ocupado || !x.PuedeAsentarla() {
		return
	}
	x.Ocupado, x.Fallo = true, ""
	x.cambio()
	clave, id := x.Clave, x.Registro.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		ref, err := p.svc.Asentar(ctx, clave, id)
		enHilo(func() {
			if p.facturas != x {
				return
			}
			if err != nil {
				x.Ocupado, x.Fallo = false, MensajeDeError(err, p.tr)
				x.cambio()
				return
			}
			x.Asentado(ref)
			p.revision++
			p.olvidar(clave)
			if p.lista.Modelo.Clave == clave {
				p.pedirLista()
			}
		})
	})
}
