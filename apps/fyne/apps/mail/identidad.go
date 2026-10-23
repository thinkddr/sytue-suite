// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los borradores sin confirmar sobreviven a un cambio de cuenta en esta ventana,
// sólo en memoria y ligados a emisor+sujeto+organización. Nunca se suben con la otra sesión.
type redaccionSuspendida struct {
	borrador      *Borrador
	externo       *correoExterno
	envioIncierto bool
}

// CambiarSesion sustituye la vida del buzón al cambiar de identidad. Un cambio de
// apariencia o un refresh del mismo token no vuelve a abrir el SSE ni la bandeja.
func (b *Buzon) CambiarSesion(ctx context.Context, identidad string) {
	if identidad == b.identidad {
		return
	}
	if b.redacciones == nil {
		b.redacciones = make(map[string]redaccionSuspendida)
	}
	if b.identidad != "" {
		var pendiente redaccionSuspendida
		if rd := b.redaccion; rd != nil {
			br := rd.r.B
			pendiente.borrador, pendiente.envioIncierto = &br, rd.enviando
			b.cerrar(rd) // No intenta guardar con la nueva sesión ya adoptada.
		}
		if c := b.correoAbierto; c != nil {
			c.pausarLista()
			c.cancelar()
			c.cerrado = true
			c.limpiarClaves()
			c.d.Cerrar()
			if c.ocupado && c.redactando {
				c.errorClave = "externalMail.sendUnknown"
			}
			pendiente.externo = c
			b.correoAbierto, b.cerrarCorreoExterno = nil, nil
		}
		if pendiente.borrador != nil || pendiente.externo != nil {
			b.redacciones[b.identidad] = pendiente
		}
	}
	b.Parar()
	b.identidad = identidad
	//nolint:contextcheck // Sólo retira UI de la cuenta anterior; los callbacks que construye no se ejecutan aquí.
	b.olvidarBuzon()
	if identidad == "" {
		return
	}
	b.Arrancar(ctx)
	if pendiente, ok := b.redacciones[identidad]; ok {
		delete(b.redacciones, identidad)
		if pendiente.borrador != nil {
			//nolint:contextcheck // Los eventos del redactor usan la vida de sesión vigente guardada en b.ctx.
			b.abrirRedactor(*pendiente.borrador)
			if pendiente.envioIncierto {
				b.avisar(ui.AvisoPeligro, "externalMail.sendUnknown", nil)
			}
		}
		if c := pendiente.externo; c != nil {
			c.ctx, c.cancelar = context.WithCancel(b.ctx)
			c.cerrado, c.ocupado = false, false
			c.version++
			b.correoAbierto, b.cerrarCorreoExterno = c, c.cerrar
			c.d.Mostrar(b.P.Lienzo())
			c.pintar()
			//nolint:contextcheck // El temporizador consulta c.ctx, recién derivado de la vida del buzón.
			c.programar()
		}
	}
}

func (b *Buzon) olvidarBuzon() {
	b.vista++
	b.carpetas, b.etiquetas, b.compartidos = nil, nil, nil
	b.carpetaID, b.etiquetaID, b.carpetaComp = "", "", ""
	b.mensajes, b.resultados, b.hilo = nil, nil, nil
	b.cursor, b.consulta, b.estadoDelta = "", "", ""
	b.firma, b.principal, b.alias = "", "", nil
	b.hiloPedido, b.buzonDelAviso, b.reabrir = "", "", ""
	b.deltaEnCurso, b.otraVuelta, b.pidiendo, b.buscaMas = false, false, false, false
	b.esperaSondeo = SondeoBase
	b.cuerpos = make(map[string]Cuerpo)
	b.dejarCompartido()
	b.cerrarHilo()
	b.P.CerrarCajon()
	m := b.P.Menu
	m.Carpetas, m.Etiquetas, m.Compartidos = nil, nil, nil
	m.CarpetaID, m.EtiquetaID, m.CompartidoID = "", "", ""
	m.EstadoCarpetas, m.EstadoCuota = SinPermiso, Vacio
	m.CuotaFraccion, m.CuotaPorcentaje, m.CuotaUso, m.CuotaNota = 0, "", "", ""
	b.P.Bandeja.Conversaciones = nil
	b.P.Bandeja.Marcadas = make(map[string]bool)
	b.P.Bandeja.Estado, b.P.Bandeja.Titulo = SinPermiso, ""
	b.P.Bandeja.Buscando, b.P.Bandeja.Consulta = false, ""
	for _, aviso := range b.P.Avisos.Avisos() {
		b.P.Avisos.Descartar(aviso)
	}
	// Los otros diálogos y menús pertenecían a la cuenta que se acaba de cerrar.
	if lienzo := b.P.Lienzo(); lienzo != nil {
		for _, capa := range lienzo.Overlays().List() {
			lienzo.Overlays().Remove(capa)
		}
		lienzo.Unfocus()
	}
	b.P.Refrescar()
}
