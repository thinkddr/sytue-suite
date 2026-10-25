// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Lo que escribe: la ficha, el lote y lo que cuelga del registro
//
// Cada acción es la de la web, con su aviso: guardar cerraba el panel sin decir nada y
// quedabas mirando la lista sin saber si había entrado. Las llamadas van en `fondo`
// (otra rutina) y lo que contestan se pinta en la de la interfaz (`enHilo`).

// MensajeDeError es `mensajeDeError` de lib/api.ts: `sin_permiso` se traduce, y el
// resto es el mensaje del servicio o, sin él, «algo ha ido mal».
func MensajeDeError(err error, tr *i18n.Translator) string {
	var e *api.Error
	switch {
	case errors.As(err, &e) && e.Code() == "sin_permiso":
		return tr.T("error.sinPermiso", nil)
	case errors.As(err, &e) && e.Code() == "certificado_invalido":
		return tr.T("vf.error.invalidCertificate", nil)
	case errors.As(err, &e) && e.Code() == "certificado_nif_no_coincide":
		return tr.T("vf.error.certificateTaxIdMismatch", nil)
	case errors.As(err, &e) && e.Message() != "":
		return e.Message()
	}
	return tr.T("error.generic", nil)
}

// avisar pone un aviso en la pila de la ventana.
func (p *Pantalla) avisar(tono ui.TonoAviso, titulo string) {
	p.avisos.Notificar(ui.NuevoAviso(tono, titulo, ""))
}

// PuedeEscribir es `puedeEscribir`: mientras no se sepan los permisos se ofrece todo y
// manda el servicio; sabidos, escribir pide «usuario» o «administrador».
func (p *Pantalla) PuedeEscribir(modulo string) bool {
	return !p.SabePermisos || p.Permisos.Alcanza(modulo, api.ERPUsuario)
}

// PuedeAccion ofrece un gesto solo cuando el servicio ya ha concedido su
// verbo efectivo. Un nivel de usuario por sí solo no concede asentar ni conciliar.
func (p *Pantalla) PuedeAccion(modulo, verbo string) bool {
	return p.SabePermisos && p.Permisos.PuedeAccion(modulo, verbo)
}

// facturae es si la organización es española: sale Facturae y VERI*FACTU.
func (p *Pantalla) facturae() bool { return p.Organizacion != nil && p.Organizacion.Pais == "ES" }

// AbrirFicha abre el registro en el panel de la derecha y pide lo que cuelga de él.
func (p *Pantalla) AbrirFicha(r Registro) {
	reg := r
	p.abrirFicha(&reg, nil)
}

// NuevaFicha abre la ficha en blanco, o con los valores de `semilla` (duplicar).
func (p *Pantalla) NuevaFicha(semilla *Registro) { p.abrirFicha(nil, semilla) }

func (p *Pantalla) abrirFicha(r, semilla *Registro) {
	m := p.lista.Modelo
	if m.Clave == "" {
		return
	}
	f := NuevaFicha(p.tr, m, r, semilla, p.lista.Pagina.Etiquetas)
	f.EnPapelera, f.Facturae = p.lista.Papelera, p.facturae()
	f.SoloLectura = !p.PuedeEscribir(m.Modulo)
	f.Requisitos = p.Requisitos
	f.BuscarRelacion = p.buscarRelacion
	p.conectarAltas(f)
	p.ficha = f
	p.engancharFicha(f)
	p.Repintar()
	if p.lienzo != nil {
		if e := f.Enfocable(); e != nil {
			p.lienzo.Focus(e)
		}
	}
	if r != nil {
		p.cargarSeguimiento(f)
	}
}

// buscarRelacion es la consulta del selector: ocho, por texto, del modelo apuntado.
func (p *Pantalla) buscarRelacion(ctx context.Context, modelo, q string) ([]Registro, error) {
	if p.svc == nil {
		return nil, nil
	}
	pag, err := p.svc.ListarCon(ctx, modelo, api.ERPFiltro{Q: q, Limite: MaxOpcionesRelacion})
	return pag.Registros, err
}

// cerrarFicha la cierra sin guardar.
func (p *Pantalla) cerrarFicha() {
	p.ficha = nil
	p.Repintar()
}

// esta dice si `f` sigue siendo la ficha abierta: lo que llega tarde de otra no pinta.
func (p *Pantalla) esta(f *Ficha) bool { return p.ficha == f }

// cargarSeguimiento pide los asientos, las tareas, los documentos, el historial y lo
// que dijo la Agencia Tributaria del registro. Un fallo de cualquiera deja su bloque
// vacío y no rompe la ficha, como en la web.
func (p *Pantalla) cargarSeguimiento(f *Ficha) {
	if p.svc == nil || f.Registro == nil {
		return
	}
	m, id := f.Modelo, f.Registro.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		var asientos []api.ERPAsiento
		if m.Contable {
			asientos, _ = p.svc.AsientosDe(ctx, m.Clave, id)
		}
		tareas, errT := p.svc.Actividades(ctx, api.ERPFiltroActividades{Modelo: m.Clave, Registro: id, Estado: "pendiente"})
		adjuntos, errA := p.svc.Adjuntos(ctx, m.Clave, id)
		historial, errH := p.svc.Historial(ctx, m.Clave, id)
		var vf *api.ERPFacturaVerifactu
		if m.Clave == api.ERPFacturaVenta && f.Facturae {
			if r, err := p.svc.RegistrosVerifactu(ctx, m.Clave, id); err == nil {
				vf = &r
			}
		}
		enHilo(func() {
			if !p.esta(f) {
				return
			}
			f.Asientos, f.Verifactu = asientos, vf
			f.Tareas, f.Adjuntos, f.Historial = vacioSiFalla(tareas, errT), vacioSiFalla(adjuntos, errA),
				vacioSiFalla(historial, errH)
			p.Repintar()
		})
	})
}

// vacioSiFalla es el `.catch(() => setLista([]))` de la web: con error, lista vacía
// (y no nil, que es «cargando»).
func vacioSiFalla[T any](xs []T, err error) []T {
	if err != nil || xs == nil {
		return []T{}
	}
	return xs
}

// engancharFicha conecta los botones de la ficha con el servicio.
func (p *Pantalla) engancharFicha(f *Ficha) {
	f.AlCerrar = p.cerrarFicha
	f.AlGuardar = func() { p.guardar(f) }
	f.AlEstado = func(e string) { p.cambiarEstado(f, e) }
	f.AlAsentar = func() { p.asentar(f) }
	f.AlAnular = func(a api.ERPAsiento) { p.pedirAnular(f, a) }
	f.AlBorrar = func() { p.pedirBorrar(f) }
	f.AlRestaurar = func() { p.restaurar(f) }
	f.AlDuplicar = func() {
		if f.Registro != nil {
			semilla := *f.Registro
			p.NuevaFicha(&semilla)
		}
	}
	f.AlCopiarID = func() {
		if f.Registro == nil {
			return
		}
		if a := fyne.CurrentApp(); a != nil {
			a.Clipboard().SetContent(f.Registro.ID)
		}
		p.avisar(ui.AvisoInfo, p.t("form.copiedId"))
	}
	f.AlDescargar = func(que string) { p.descargar(f, que) }
	f.AlPlanear = func() {
		f.Planificando = !f.Planificando
		p.Repintar()
		if f.Planificando && f.Equipo == nil {
			p.cargarEquipo(f)
		}
	}
	f.AlPlanificar = func(tipo, resumen, vence string, para api.ERPPersona) { p.planificar(f, tipo, resumen, vence, para) }
	f.AlHecha = func(a api.ERPActividad) { p.cerrarTarea(f, a) }
	f.AlAbrirAdjunto = func(a api.ERPAdjunto) { p.abrirAdjunto(a) }
	f.AlBorrarAdjunto = func(a api.ERPAdjunto) { p.borrarAdjunto(f, a) }
	f.AlAnotar = func(texto string) { p.anotar(f, texto) }
	f.AlReenviarVF = func() { p.reenviarVF(f) }
	f.AlIA = func() { p.rellenarIA(f) }
	f.AlRequisitos = func() { f.VerRequisitos = !f.VerRequisitos; p.Repintar() }
	if p.ElegirFicheros != nil {
		f.AlSubir = func() {
			if f.Registro == nil || f.SubiendoDoc {
				return
			}
			p.subirDocumento(f.Modelo.Clave, f.Registro.ID, func(subiendo bool) {
				f.SubiendoDoc = subiendo
				if !subiendo && p.esta(f) {
					p.cargarSeguimiento(f)
				}
			})
		}
	}
}

// escrito es el `onGuardado` de la web: la ficha se cierra, lo cacheado de ese modelo
// deja de valer y la lista y su recuento se vuelven a pedir.
func (p *Pantalla) escrito(m Modelo) {
	p.revision++
	if m.Clave == api.ERPFacturaVenta && p.facturae() {
		p.cargarVerifactu()
	}
	p.ficha = nil
	p.olvidar(m.Clave)
	p.pedirLista()
	if p.svc != nil {
		p.fondo(func() { p.cargarEstadisticas(m.Clave) })
	}
}

// guardar crea o actualiza el registro con el cuerpo entero del formulario.
func (p *Pantalla) guardar(f *Ficha) {
	if f.Guardando {
		return
	}
	if !f.Comprobar() {
		p.repintarFicha(f)
		return
	}
	if p.svc == nil {
		return
	}
	f.Guardando, f.Fallo, f.Errores = true, "", map[string]string{}
	p.repintarFicha(f)
	data := ACuerpo(f.Modelo, f.Valores, time.Local)
	m, r := f.Modelo, f.Registro
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		var err error
		var guardado Registro
		if r != nil {
			guardado, err = p.svc.Actualizar(ctx, m.Clave, r.ID, data)
		} else {
			guardado, err = p.svc.Crear(ctx, m.Clave, data)
		}
		enHilo(func() {
			f.Guardando = false
			var e *api.Error
			switch {
			case err == nil:
				if f.alCreado != nil {
					f.alCreado(guardado)
					return
				}
				titulo := p.t("toast.created")
				if r != nil {
					titulo = p.t("toast.saved")
				}
				p.avisar(ui.AvisoExito, titulo)
				if p.esta(f) {
					p.escrito(m)
					return
				}
			case errors.As(err, &e) && len(e.Campos()) > 0:
				// Con los campos señalados la frase de arriba sobra.
				f.Errores = e.Campos()
			default:
				f.Fallo = MensajeDeError(err, p.tr)
			}
			p.repintarFicha(f)
		})
	})
}

// cambiarEstado mueve el registro de etapa. Como en la web, cierra la ficha.
func (p *Pantalla) cambiarEstado(f *Ficha, estado string) {
	if p.svc == nil || f.Registro == nil {
		return
	}
	f.Guardando, f.Fallo = true, ""
	p.Repintar()
	m, id := f.Modelo, f.Registro.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		act, err := p.svc.CambiarEstado(ctx, m.Clave, id, estado)
		enHilo(func() {
			f.Guardando = false
			if err != nil {
				f.Fallo = MensajeDeError(err, p.tr)
				p.Repintar()
				return
			}
			if act.Estado != "" {
				estado = act.Estado
			}
			f.EstadoActual = estado
			p.avisar(ui.AvisoExito, p.t("form.stateUpdated", i18n.Vars{"status": RotuloEstado(m, estado)}))
			if p.esta(f) {
				p.escrito(m)
			} else {
				p.Repintar()
			}
		})
	})
}

// asentar lleva el documento al libro y enseña su asiento debajo en el acto.
func (p *Pantalla) asentar(f *Ficha) {
	if p.svc == nil || f.Registro == nil {
		return
	}
	f.Guardando, f.Fallo = true, ""
	p.Repintar()
	m, id := f.Modelo, f.Registro.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		ref, err := p.svc.Asentar(ctx, m.Clave, id)
		var asientos []api.ERPAsiento
		if err == nil {
			asientos, _ = p.svc.AsientosDe(ctx, m.Clave, id)
		}
		enHilo(func() {
			f.Guardando = false
			if err != nil {
				f.Fallo = MensajeDeError(err, p.tr)
			} else {
				f.Aviso, f.Asientos = p.t("form.posted", i18n.Vars{"ref": ref}), asientos
			}
			p.Repintar()
		})
	})
}

// pedirAnular pregunta el motivo —el `usePrompt` de la web— y anula el asiento.
func (p *Pantalla) pedirAnular(f *Ficha, a api.ERPAsiento) {
	if p.lienzo == nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	campo := ui.NuevaEntrada("")
	mensaje := ui.NuevoParrafo(p.t("ledger.voidReason"), tema.TextoBase, ui.Normal, "text-muted", v)
	cuerpo := container.New(&preguntaLay{p: mensaje, e: campo}, mensaje, campo)
	var d *ui.Dialogo
	d = ui.Confirmacion(p.t("ledger.voidTitle", i18n.Vars{"ref": a.Ref}), p.tr.T("ui.cancel", nil), p.t("ledger.void"), false, cuerpo,
		func() { d.Cerrar() },
		func() {
			motivo := strings.TrimSpace(campo.Texto())
			d.Cerrar()
			if motivo != "" {
				p.anular(f, a, motivo)
			}
		})
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	d.Foco = campo
	d.Mostrar(p.lienzo)
}

func (p *Pantalla) anular(f *Ficha, a api.ERPAsiento, motivo string) {
	if p.svc == nil || f.Registro == nil {
		return
	}
	f.Guardando, f.Fallo = true, ""
	p.Repintar()
	m, id := f.Modelo, f.Registro.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.svc.AnularAsiento(ctx, a.ID, motivo)
		asientos, _ := p.svc.AsientosDe(ctx, m.Clave, id)
		enHilo(func() {
			f.Guardando = false
			if err != nil {
				f.Fallo = MensajeDeError(err, p.tr)
			} else {
				f.Aviso, f.Asientos = p.t("ledger.voided", i18n.Vars{"ref": a.Ref}), asientos
			}
			p.Repintar()
		})
	})
}

// preguntaLay es el cuerpo del diálogo de pregunta: el mensaje y, debajo, el campo.
type preguntaLay struct {
	p *ui.Parrafo
	e *ui.Entrada
}

func (l *preguntaLay) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, tema.TextoBase.Linea+tema.Espacio(3)+ui.AltoCampo(ui.CampoMD))
}

func (l *preguntaLay) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	alto := ui.AltoParrafo(l.p, t.Width)
	l.p.Resize(fyne.NewSize(t.Width, alto))
	l.e.Move(fyne.NewPos(0, alto+tema.Espacio(3)))
	l.e.Resize(fyne.NewSize(t.Width, ui.AltoCampo(ui.CampoMD)))
}

// pedirBorrar PREGUNTA antes: va a la papelera, pero desde la interfaz parece
// definitivo, y se pregunta como tal.
func (p *Pantalla) pedirBorrar(f *Ficha) {
	if p.lienzo == nil || f.Registro == nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := ui.NuevoParrafo(p.t("confirm.delete", i18n.Vars{"nombre": NombreDe(*f.Registro)}),
		tema.TextoBase, ui.Normal, "text-muted", v)
	var d *ui.Dialogo
	d = ui.Confirmacion(p.t("action.delete"), p.tr.T("ui.cancel", nil), p.t("action.delete"), true, cuerpo,
		func() { d.Cerrar() },
		func() { d.Cerrar(); p.borrar(f) })
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	d.Mostrar(p.lienzo)
}

func (p *Pantalla) borrar(f *Ficha) {
	p.escribirRegistro(f, "toast.deleted", func(ctx context.Context) error {
		return p.svc.Borrar(ctx, f.Modelo.Clave, f.Registro.ID)
	})
}

func (p *Pantalla) restaurar(f *Ficha) {
	p.escribirRegistro(f, "toast.restored", func(ctx context.Context) error {
		return p.svc.Restaurar(ctx, f.Modelo.Clave, f.Registro.ID)
	})
}

// escribirRegistro es el esqueleto de borrar y restaurar: guardando, la llamada, el
// aviso y, si fue bien, `onGuardado`.
func (p *Pantalla) escribirRegistro(f *Ficha, aviso string, llamar func(context.Context) error) {
	if p.svc == nil || f.Registro == nil {
		return
	}
	f.Guardando, f.Fallo = true, ""
	p.Repintar()
	m := f.Modelo
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := llamar(ctx)
		enHilo(func() {
			f.Guardando = false
			if err != nil {
				f.Fallo = MensajeDeError(err, p.tr)
				p.Repintar()
				return
			}
			p.avisar(ui.AvisoExito, p.t(aviso))
			if p.esta(f) {
				p.escrito(m)
			} else {
				p.Repintar()
			}
		})
	})
}

// duplicarDeLista es «Duplicar» en una fila: la copia la hace el SERVIDOR (borrador con
// referencia nueva) y se abre; si falla, se abre una ficha nueva con los valores.
func (p *Pantalla) duplicarDeLista(r Registro) {
	m := p.lista.Modelo
	if p.svc == nil {
		p.NuevaFicha(&r)
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		copia, err := p.svc.Duplicar(ctx, m.Clave, r.ID)
		enHilo(func() {
			if p.lista.Modelo.Clave != m.Clave {
				return
			}
			if err != nil {
				p.NuevaFicha(&r)
				return
			}
			p.avisar(ui.AvisoExito, p.t("toast.created"))
			p.AbrirFicha(copia)
		})
	})
}

// lote aplica una acción a las filas marcadas. Mandar a la papelera se pregunta antes.
func (p *Pantalla) lote(accion, estado string) {
	l := p.lista
	var ids []string
	for _, r := range l.registros() {
		if l.Marcadas[r.ID] {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	if accion == "borrar" && p.lienzo != nil {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		cuerpo := ui.NuevoParrafo(p.t("list.batchDeleteConfirm", i18n.Vars{"n": len(ids)}), tema.TextoBase, ui.Normal,
			"text-muted", v)
		var d *ui.Dialogo
		d = ui.Confirmacion(p.t("list.batchDelete"), p.tr.T("ui.cancel", nil), p.t("list.batchDelete"), true, cuerpo,
			func() { d.Cerrar() },
			func() { d.Cerrar(); p.aplicarLote(accion, estado, ids) })
		d.RotuloCerrar = p.tr.T("ui.close", nil)
		d.Mostrar(p.lienzo)
		return
	}
	p.aplicarLote(accion, estado, ids)
}

func (p *Pantalla) aplicarLote(accion, estado string, ids []string) {
	l := p.lista
	if p.svc == nil {
		return
	}
	l.EnLote = true
	p.Repintar()
	m := l.Modelo
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		_, err := p.svc.Lote(ctx, m.Clave, accion, ids, estado)
		enHilo(func() {
			l.EnLote = false
			if err != nil {
				p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
				p.Repintar()
				return
			}
			titulo := p.t("toast.saved")
			if accion == "borrar" {
				titulo = p.t("toast.deleted")
			}
			p.avisar(ui.AvisoExito, titulo)
			l.Marcadas = map[string]bool{}
			p.revision++
			p.olvidar(m.Clave)
			p.pedirLista()
		})
	})
}

// descargar baja la factura como UBL, Facturae o para imprimir, la guarda y la abre.
func (p *Pantalla) descargar(f *Ficha, que string) {
	if p.svc == nil || f.Registro == nil {
		return
	}
	m, id, ref := f.Modelo.Clave, f.Registro.ID, f.Registro.Ref
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		var (
			cuerpo io.ReadCloser
			nombre string
			err    error
		)
		switch que {
		case "ubl":
			cuerpo, nombre, err = p.svc.UBL(ctx, m, id)
		case "facturae":
			cuerpo, nombre, err = p.svc.Facturae(ctx, m, id)
		default:
			cuerpo, err = p.svc.Imprimir(ctx, m, id)
			// La factura para imprimir es una página HTML: se nombra por su referencia.
			nombre = "factura" + extHTML
			if ref != "" {
				nombre = ref + extHTML
			}
		}
		p.guardarYAbrir(cuerpo, nombre, err)
	})
}

// extHTML va aparte porque «factura.html» tiene forma de clave de catálogo y el
// guardián de i18n lo leería como tal.
const extHTML = ".html"

// guardarYAbrir es la descarga del navegador: a la carpeta de descargas y abierta.
func (p *Pantalla) guardarYAbrir(cuerpo io.ReadCloser, nombre string, err error) {
	if err == nil {
		defer cuerpo.Close()
		var ruta string
		ruta, err = guardarDescarga(carpetaDescargas(), nombre, cuerpo)
		if err == nil {
			enHilo(func() { abrirFichero(ruta) })
			return
		}
	}
	enHilo(func() { p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr)) })
}

// abrirAdjunto baja el documento y lo abre, que es el `target="_blank"` de la web.
func (p *Pantalla) abrirAdjunto(a api.ERPAdjunto) {
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		cuerpo, err := p.svc.Adjunto(ctx, a.ID)
		p.guardarYAbrir(cuerpo, a.Nombre, err)
	})
}

// seguirFicha vuelve a pedir lo que cuelga del registro tras tocarlo.
func (p *Pantalla) seguirFicha(f *Ficha, err error) {
	enHilo(func() {
		if err != nil {
			p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
		}
		if p.esta(f) {
			p.cargarSeguimiento(f)
		}
	})
}

func (p *Pantalla) borrarAdjunto(f *Ficha, a api.ERPAdjunto) {
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		p.seguirFicha(f, p.svc.BorrarAdjunto(ctx, a.ID))
	})
}

func (p *Pantalla) cerrarTarea(f *Ficha, a api.ERPActividad) {
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		p.seguirFicha(f, p.svc.CerrarActividad(ctx, a.ID, "hecha"))
	})
}

// cargarEquipo pide la gente de la organización para el selector de responsable. Si
// falla, la tarea es de quien la crea, como antes.
func (p *Pantalla) cargarEquipo(f *Ficha) {
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		equipo, err := p.svc.Equipo(ctx)
		if err != nil || len(equipo) == 0 {
			return
		}
		enHilo(func() {
			f.Equipo, f.plan = equipo, nil // el editor se rehace con el selector
			p.Repintar()
		})
	})
}

func (p *Pantalla) planificar(f *Ficha, tipo, resumen, vence string, para api.ERPPersona) {
	if p.svc == nil || f.Registro == nil || resumen == "" {
		return
	}
	m, id := f.Modelo.Clave, f.Registro.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.svc.CrearActividad(ctx, m, id, tipo, resumen, vence, para)
		enHilo(func() {
			if err == nil && p.esta(f) {
				f.Planificando, f.plan = false, nil
			}
		})
		p.seguirFicha(f, err)
	})
}

func (p *Pantalla) anotar(f *Ficha, texto string) {
	if p.svc == nil || f.Registro == nil {
		return
	}
	f.Anotando = true
	p.Repintar()
	m, id := f.Modelo.Clave, f.Registro.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.svc.Anotar(ctx, m, id, texto)
		enHilo(func() {
			f.Anotando = false
			if err == nil {
				f.nota.PonerTexto("")
			}
		})
		p.seguirFicha(f, err)
	})
}

func (p *Pantalla) reenviarVF(f *Ficha) {
	if p.svc == nil || f.Registro == nil {
		return
	}
	f.Reenviando = true
	p.Repintar()
	m, id := f.Modelo.Clave, f.Registro.ID
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		r, err := p.svc.SubsanarVerifactu(ctx, m, id)
		enHilo(func() {
			f.Reenviando = false
			if err != nil {
				p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
			} else {
				f.Verifactu = &r
				p.avisar(ui.AvisoExito, p.t("vf.resent"))
			}
			p.Repintar()
		})
	})
}

// exportar baja la lista en CSV con la búsqueda y el estado de ahora, o las marcadas.
func (p *Pantalla) exportar(ids []string) {
	if p.svc == nil {
		return
	}
	l := p.lista
	m, q, estado := l.Modelo.Clave, l.Busqueda, l.Estado
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		cuerpo, nombre, err := p.svc.Exportar(ctx, m, q, estado, ids)
		p.guardarYAbrir(cuerpo, nombre, err)
	})
}

// --- El marco del contenido -----------------------------------------------------

// vigia es la raíz: coloca su hijo a pantalla completa y, si cambia el tamaño, lo
// apunta y repinta, porque el reparto de la web (`md:`, `sm:`, las columnas de la
// rejilla) mira la VENTANA.
type vigia struct{ p *Pantalla }

func (v *vigia) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.NewSize(0, 0)
	}
	return objs[0].MinSize()
}

func (v *vigia) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
	if t.Width > 0 && t != v.p.Ventana {
		v.p.Ventana = t
		fyne.Do(v.p.Repintar)
	}
}

// conFicha pone `main` y, a su derecha, la ficha. En el móvil la ficha tapa `main`.
type conFicha struct {
	main  fyne.CanvasObject
	ficha fyne.CanvasObject
	ancho float32
	movil bool
}

func (c *conFicha) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (c *conFicha) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.main.Move(fyne.NewPos(0, 0))
	if c.ficha == nil {
		c.main.Resize(t)
		return
	}
	if c.movil {
		c.main.Resize(t)
		c.ficha.Move(fyne.NewPos(0, 0))
		c.ficha.Resize(t)
		return
	}
	c.main.Resize(fyne.NewSize(max(t.Width-c.ancho, 0), t.Height))
	c.ficha.Move(fyne.NewPos(t.Width-c.ancho, 0))
	c.ficha.Resize(fyne.NewSize(c.ancho, t.Height))
}

func quitarNil(objs ...fyne.CanvasObject) []fyne.CanvasObject {
	var out []fyne.CanvasObject
	for _, o := range objs {
		if o != nil {
			out = append(out, o)
		}
	}
	return out
}

// Medidas del botón del cajón: `mt-4 ml-6`, 44×44 (`min-h-[44px] min-w-[44px]`),
// `rounded-md border text-lg`, con el «☰» de texto.
const (
	CajonMT   float32 = 4 * 0.25 * tema.Rem
	CajonML   float32 = 6 * 0.25 * tema.Rem
	LadoCajon float32 = 44
)

// mainERP es el contenido de `main`: en el móvil, el botón del cajón arriba y la vista
// debajo; en escritorio, la vista sola.
type mainERP struct {
	cajon     *BotonCajon
	contenido fyne.CanvasObject
}

func (m *mainERP) arriba() float32 {
	if m.cajon == nil {
		return 0
	}
	return CajonMT + LadoCajon
}

func (m *mainERP) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, m.arriba()+m.contenido.MinSize().Height)
}

func (m *mainERP) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	if m.cajon != nil {
		m.cajon.Move(fyne.NewPos(CajonML, CajonMT))
		m.cajon.Resize(fyne.NewSize(LadoCajon, LadoCajon))
	}
	y := m.arriba()
	m.contenido.Move(fyne.NewPos(0, y))
	m.contenido.Resize(fyne.NewSize(t.Width, max(t.Height-y, m.contenido.MinSize().Height)))
}

// BotonCajon es el «☰» que abre el menú en el móvil: el único mando de navegación que
// hay por debajo de 768 px.
type BotonCajon struct {
	ui.Pulsable
}

// NuevoBotonCajon crea el botón. `nombre` es su `aria-label` («Menú»).
func NuevoBotonCajon(nombre string, alPulsar func()) *BotonCajon {
	b := &BotonCajon{}
	b.Nombre, b.AlPulsar = nombre, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// MinSize son los 44 de la diana táctil.
func (b *BotonCajon) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return fyne.NewSize(LadoCajon, LadoCajon)
}

// CreateRenderer construye la caja perfilada y el «☰».
func (b *BotonCajon) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &cajonRenderer{
		b:     b, // `rounded-control`: el radio del botón es de la forma
		aro:   ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v),
		caja:  ui.NuevaCaja(tema.Color("bg", v), tema.Color("border", v), tema.Forma("radius-control")),
		texto: ui.Texto("☰", tema.TextoLG, ui.Normal, tema.Color("text-muted", v)),
	}
	r.Refresh()
	return r
}

type cajonRenderer struct {
	b     *BotonCajon
	aro   *ui.Aro
	caja  *ui.Caja
	texto *canvas.Text
}

func (r *cajonRenderer) MinSize() fyne.Size { return r.b.MinSize() }
func (r *cajonRenderer) Objects() []fyne.CanvasObject {
	return append(append(r.aro.Objetos(), r.caja.Objetos()...), r.texto)
}
func (r *cajonRenderer) Destroy() {}

func (r *cajonRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	ancho := ui.Medir("☰", tema.TextoLG, ui.Normal).Width
	ui.ColocarTexto(r.texto, tema.TextoLG, (t.Width-ancho)/2, (t.Height-tema.TextoLG.Linea)/2)
}

func (r *cajonRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.texto.Color = tema.Color("text-muted", v)
	if r.b.Encima() {
		r.texto.Color = tema.Color("text", v)
	}
	r.texto.Refresh()
	r.caja.CambiarRadio(tema.Forma("radius-control")) // la forma se cambia en caliente
	r.aro.CambiarRadio(tema.Forma("radius-control"))
	r.aro.Ver(r.b.Enfocado())
	r.aro.Pintar(v)
}

// --- Tareas, libro y balance ------------------------------------------------------

// contarTareas pide las tareas propias que vencen hoy o ya vencieron: el número del
// menú. Su fallo es silencioso, como en la web.
func (p *Pantalla) contarTareas() {
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		as, err := p.svc.Actividades(ctx, api.ERPFiltroActividades{Estado: "pendiente", Mias: true, Hasta: HoyISO(time.Now())})
		if err != nil {
			return
		}
		enHilo(func() { p.TareasHoy = len(as); p.Repintar() })
	})
}

// cargarTareas pide las pendientes, las propias o las de todos.
func (p *Pantalla) cargarTareas() {
	if p.svc == nil {
		return
	}
	mias := p.tareas.Mias
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		as, err := p.svc.Actividades(ctx, api.ERPFiltroActividades{Estado: "pendiente", Mias: mias})
		enHilo(func() {
			if p.Vista != VistaTareas || p.tareas.Mias != mias {
				return
			}
			p.tareas.Lista = vacioSiFalla(as, err)
			p.Repintar()
		})
	})
}

// cerrarTareaDeLista da por hecha o descarta una tarea desde la pantalla de Tareas.
func (p *Pantalla) cerrarTareaDeLista(a api.ERPActividad, como string) {
	if p.svc == nil {
		return
	}
	p.tareas.Ocupada = a.ID
	p.Repintar()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.svc.CerrarActividad(ctx, a.ID, como)
		enHilo(func() {
			p.tareas.Ocupada = ""
			if err != nil {
				p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
			}
			p.contarTareas()
			p.cargarTareas()
		})
	})
}

// abrirRegistro abre la lista del modelo de una tarea y la ficha del registro.
func (p *Pantalla) abrirRegistro(modelo, id string) {
	var m Modelo
	for _, x := range p.Modelos() {
		if x.Clave == modelo {
			m = x
		}
	}
	if m.Clave == "" {
		return
	}
	p.Abrir(m)
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		r, err := p.svc.Obtener(ctx, modelo, id)
		enHilo(func() {
			if err != nil {
				p.avisar(ui.AvisoPeligro, p.t("error.generic"))
				return
			}
			if p.lista.Modelo.Clave == modelo {
				p.AbrirFicha(r)
			}
		})
	})
}

// cargarLibro pide los últimos cincuenta asientos.
func (p *Pantalla) cargarLibro() {
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		as, err := p.svc.Libro(ctx, LimitePagina)
		enHilo(func() {
			if p.Vista != VistaLibro {
				return
			}
			if err != nil {
				p.libro.Fallo = MensajeDeError(err, p.tr)
			} else {
				p.libro.Asientos = vacioSiFalla(as, nil)
			}
			p.Repintar()
		})
	})
}

// cargarBalance pide el sumas y saldos.
func (p *Pantalla) cargarBalance() {
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		b, err := p.svc.Balance(ctx)
		enHilo(func() {
			if p.Vista != VistaBalance {
				return
			}
			if err != nil {
				p.libro.Fallo = MensajeDeError(err, p.tr)
			} else {
				p.libro.Balance = &b
			}
			p.Repintar()
		})
	})
}

// --- Obligaciones ---------------------------------------------------------------

// cargarObligaciones pide el calendario del año que se está mirando.
func (p *Pantalla) cargarObligaciones() {
	o := p.oblig
	o.Cargando, o.Fallo = true, ""
	if o.Datos != nil && o.Datos.Anio != o.Anio {
		o.Datos = nil
	}
	p.Repintar()
	if p.svc == nil {
		return
	}
	anio := o.Anio
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		d, err := p.svc.Obligaciones(ctx, anio)
		enHilo(func() {
			if o.Anio != anio {
				return
			}
			o.Cargando = false
			if err != nil {
				o.Fallo = MensajeDeError(err, p.tr)
			} else {
				o.Datos = &d
			}
			p.Repintar()
		})
	})
}

// prepararObligacion es «Preparar cifras»: la cifra del periodo sacada del libro.
func (p *Pantalla) prepararObligacion(ob api.ERPObligacion) {
	o, id := p.oblig, IDObligacion(ob)
	o.Calculos[id] = &CalculoObl{Cargando: true}
	p.Repintar()
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		c, err := p.svc.CalculoObligacion(ctx, ob.Clave, ob.Periodo)
		enHilo(func() {
			o.Calculos[id] = &CalculoObl{Fallo: err != nil, R: c}
			p.Repintar()
		})
	})
}

// icsObligaciones baja el calendario de avisos y lo abre, que es lo que lo importa en
// el calendario del sistema.
func (p *Pantalla) icsObligaciones() {
	if p.svc == nil {
		return
	}
	anio := p.oblig.Anio
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		cuerpo, nombre, err := p.svc.ObligacionesICS(ctx, anio)
		p.guardarYAbrir(cuerpo, nombre, err)
	})
}

// documentosObligacion pide los ficheros de cada documento de una presentación.
func (p *Pantalla) documentosObligacion(ob api.ERPObligacion) {
	if p.svc == nil {
		return
	}
	for _, d := range ob.Documentos {
		p.cargarAdjuntosObligacion(RegistroDocumento(ob, d.Clave))
	}
}

func (p *Pantalla) cargarAdjuntosObligacion(reg string) {
	o := p.oblig
	delete(o.Adjuntos, reg)
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		as, err := p.svc.Adjuntos(ctx, "obligacion", reg)
		enHilo(func() {
			o.Adjuntos[reg] = vacioSiFalla(as, err)
			p.Repintar()
		})
	})
}

func (p *Pantalla) borrarAdjuntoObligacion(reg string, a api.ERPAdjunto) {
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.svc.BorrarAdjunto(ctx, a.ID)
		enHilo(func() {
			if err != nil {
				p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
			}
			p.cargarAdjuntosObligacion(reg)
		})
	})
}

// --- La portada -------------------------------------------------------------------

// modeloPorClave busca un modelo del catálogo.
func (p *Pantalla) modeloPorClave(clave string) (Modelo, bool) {
	for _, m := range p.Modelos() {
		if m.Clave == clave {
			return m, true
		}
	}
	return Modelo{}, false
}

// PonerNombre es el de quien ha entrado, para el saludo de la portada.
func (p *Pantalla) PonerNombre(n string) {
	p.inicio.Nombre = n
	p.Repintar()
}

// cargarPortada pide lo que la portada enseña además del catálogo: el resumen, los
// movimientos del banco sin justificar y, con perfil, las próximas obligaciones. Lo que
// falle no se pinta, como en la web: la portada sigue sirviendo con lo que haya.
func (p *Pantalla) cargarPortada() {
	if p.svc == nil {
		return
	}
	b := p.inicio
	conPerfil := p.Organizacion != nil && p.PuedeLeer("obligaciones")
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		r, errR := p.svc.Resumen(ctx)
		e, errE := p.svc.Estadisticas(ctx, "linea_extracto")
		var proximas []api.ERPObligacion
		if conPerfil {
			proximas = p.proximasObligaciones(ctx)
		}
		enHilo(func() {
			if errR == nil {
				b.Resumen = &r
			}
			if errE == nil {
				b.SinJustificar = e.PorEstado["pendiente"]
			}
			if conPerfil {
				b.Proximas = proximas
			}
			p.Repintar()
		})
	})
}

// proximasObligaciones son las cuatro que vencen antes, contando desde hoy. El año que
// viene se pide cuando el actual ya no da más, que es lo que pasa desde noviembre.
func (p *Pantalla) proximasObligaciones(ctx context.Context) []api.ERPObligacion {
	hoy := time.Now()
	este, err := p.svc.Obligaciones(ctx, hoy.Year())
	if err != nil {
		return []api.ERPObligacion{}
	}
	lista := make([]api.ERPObligacion, 0, len(este.Obligaciones))
	for _, o := range este.Obligaciones {
		if DiasHasta(o.Limite, hoy) >= 0 {
			lista = append(lista, o)
		}
	}
	sort.SliceStable(lista, func(i, j int) bool { return lista[i].Recomendada.Before(lista[j].Recomendada) })
	if len(lista) < MaxProximas {
		if sig, err := p.svc.Obligaciones(ctx, hoy.Year()+1); err == nil {
			lista = append(lista, sig.Obligaciones...)
		}
	}
	return lista[:min(MaxProximas, len(lista))]
}

// cambiarModulo enciende o apaga un área y avisa de lo que arrastró con ella.
func (p *Pantalla) cambiarModulo(m Modulo, activo bool) {
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		res, err := p.svc.CambiarModulo(ctx, m.Clave, activo)
		enHilo(func() {
			if err != nil {
				p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
				return
			}
			p.Catalogo.Activos = res.Activos
			nombres := func(claves []string) string {
				out := make([]string, len(claves))
				for i, c := range claves {
					out[i] = c
					for _, x := range p.Catalogo.Modulos {
						if x.Clave == c {
							out[i] = x.Etiqueta
						}
					}
				}
				return strings.Join(out, ", ")
			}
			switch {
			case activo && len(res.Activados) > 0:
				p.avisar(ui.AvisoInfo, p.t("modules.dependencyActivated", i18n.Vars{"module": m.Etiqueta, "deps": nombres(res.Activados)}))
			case !activo && len(res.Desactivados) > 0:
				p.avisar(ui.AvisoInfo, p.t("modules.dependencyDeactivated", i18n.Vars{"module": m.Etiqueta, "deps": nombres(res.Desactivados)}))
			}
			p.Repintar()
		})
	})
}

// --- El asistente de arranque -----------------------------------------------------

// abrirAsistente lo monta con el perfil que haya y pide los países.
func (p *Pantalla) abrirAsistente() {
	a := NuevoAsistente(p.tr, p.Organizacion, time.Now().Year())
	a.Modulos, a.Activos = p.Catalogo.Modulos, p.Catalogo.Activos
	a.AlCambio = p.Repintar
	a.AlGuardar = p.guardarPerfil
	a.AlVerCalendario = func() { p.IrA(VistaObligaciones) }
	a.AlICS = p.icsObligaciones
	a.AlActivar = p.activarRecomendadas
	a.AlCancelar = func() { p.Abrir(Modelo{}) }
	p.asistente = a
	if p.svc == nil {
		a.Cargados([]api.ERPPais{})
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		ps, err := p.svc.Paises(ctx)
		enHilo(func() {
			if p.asistente == a {
				a.Cargados(vacioSiFalla(ps, err))
				p.Repintar()
			}
		})
	})
}

func (p *Pantalla) objetoAsistente(v tema.Variante) fyne.CanvasObject {
	if p.asistente == nil {
		p.abrirAsistente()
	}
	a := p.asistente
	a.Ventana, a.Activos = p.Ventana.Width, p.Catalogo.Activos
	return a.Objeto(v)
}

// guardarPerfil guarda el perfil y pide el calendario del año para decir cuántas
// obligaciones salieron.
func (p *Pantalla) guardarPerfil(o api.ERPOrganizacion) {
	a := p.asistente
	if p.svc == nil || a == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		g, err := p.svc.GuardarOrganizacion(ctx, o)
		total := -1
		if err == nil {
			if cal, errC := p.svc.Obligaciones(ctx, a.Anio); errC == nil {
				total = len(cal.Obligaciones)
			} else {
				total = 0
			}
		}
		enHilo(func() {
			a.Ocupado = false
			if err != nil {
				p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
				p.Repintar()
				return
			}
			p.Organizacion, p.SabeOrg = &g, true
			p.oblig.Datos = nil
			a.Total, a.Paso = total, PasoListo
			p.Repintar()
		})
	})
}

// activarRecomendadas enciende, una tras otra, las áreas que propone el asistente.
func (p *Pantalla) activarRecomendadas(claves []string) {
	a := p.asistente
	if p.svc == nil || a == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		var activos []string
		var err error
		for _, c := range claves {
			var r api.ERPModulosActivos
			if r, err = p.svc.CambiarModulo(ctx, c, true); err != nil {
				break
			}
			activos = r.Activos
		}
		enHilo(func() {
			a.Activando = false
			if activos != nil {
				p.Catalogo.Activos = activos
			}
			if err != nil {
				p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
			} else {
				a.Activados = true
			}
			p.Repintar()
		})
	})
}

// --- VERI*FACTU -------------------------------------------------------------------

// cargarVerifactu pide el estado de la tarjeta. Sin estado no se enseña: las facturas
// se siguen pudiendo usar.
func (p *Pantalla) cargarVerifactu() {
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		e, err := p.svc.EstadoVerifactu(ctx)
		if err != nil {
			return
		}
		enHilo(func() { p.verifactu = &e; p.Repintar() })
	})
}

// conTarjetaVF pone la tarjeta de VERI*FACTU encima de la lista, en su columna.
func (p *Pantalla) conTarjetaVF(v tema.Variante, lista fyne.CanvasObject) fyne.CanvasObject {
	ancho := p.Ventana.Width - p.lista.Ocupado - ocupaCarcasa(p.Ventana.Width)
	mx := MargenTarjetaVF(p.Ventana.Width)
	t := &TarjetaVF{
		tr: p.tr, loc: p.tr.Locale(), Estado: p.verifactu, Activando: p.vfActivando,
		Administrador: p.Permisos.Administrador,
		AlActivar:     p.pedirActivarVF,
		AlDeclaracion: func() { p.bajarVF(p.svc.DeclaracionVerifactu) },
		AlExportar:    func() { p.bajarVF(p.svc.ExportarVerifactu) },
	}
	o, alto := t.Objeto(v, max(ancho-2*mx, 0))
	return container.New(&conTarjeta{tarjeta: o, alto: alto, mx: mx, lista: lista}, o, lista)
}

// conTarjeta es la tarjeta con su `mx mt-4` y, debajo, la lista.
type conTarjeta struct {
	tarjeta, lista fyne.CanvasObject
	alto, mx       float32
}

func (c *conTarjeta) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, TarjetaVFMT+c.alto+c.lista.MinSize().Height)
}

func (c *conTarjeta) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.tarjeta.Move(fyne.NewPos(c.mx, TarjetaVFMT))
	c.tarjeta.Resize(fyne.NewSize(max(t.Width-2*c.mx, 0), c.alto))
	y := TarjetaVFMT + c.alto
	c.lista.Move(fyne.NewPos(0, y))
	c.lista.Resize(fyne.NewSize(t.Width, max(t.Height-y, c.lista.MinSize().Height)))
}

// pedirActivarVF pregunta antes de activar: desde ese momento cada factura confirmada
// se envía a la Agencia Tributaria y ya no se puede modificar.
func (p *Pantalla) pedirActivarVF() {
	if p.lienzo == nil {
		p.activarVF()
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := ui.NuevoParrafo(p.t("vf.confirmBody"), tema.TextoBase, ui.Normal, "text-muted", v)
	var d *ui.Dialogo
	d = ui.Confirmacion(p.t("vf.confirmTitle"), p.tr.T("ui.cancel", nil), p.t("vf.activate"), false, cuerpo,
		func() { d.Cerrar() },
		func() { d.Cerrar(); p.activarVF() })
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	d.Mostrar(p.lienzo)
}

func (p *Pantalla) activarVF() {
	if p.svc == nil {
		return
	}
	p.vfActivando = true
	p.Repintar()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		e, err := p.svc.ActivarVerifactu(ctx)
		enHilo(func() {
			p.vfActivando = false
			if err != nil {
				p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
			} else {
				p.verifactu = &e
				p.avisar(ui.AvisoExito, p.t("vf.activated"))
			}
			p.Repintar()
		})
	})
}

// bajarVF baja la declaración o los registros y los abre.
func (p *Pantalla) bajarVF(bajar func(context.Context) (io.ReadCloser, string, error)) {
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		cuerpo, nombre, err := bajar(ctx)
		p.guardarYAbrir(cuerpo, nombre, err)
	})
}

// --- Subir e importar ficheros ----------------------------------------------------

// ExtensionesDocumento son las de `ACEPTA_DOCUMENTOS`: las que el servicio comprueba
// por su contenido.
var ExtensionesDocumento = []string{".pdf", ".jpg", ".jpeg", ".png", ".webp", ".xml"}

// primeroCon es la primera ruta con una de esas extensiones: el `accept` del
// `<input type="file">`.
func primeroCon(rutas, extensiones []string) (string, bool) {
	for _, r := range rutas {
		if slices.Contains(extensiones, strings.ToLower(filepath.Ext(r))) {
			return r, true
		}
	}
	return "", false
}

// subirDocumento elige un fichero con el selector del sistema y lo sube al registro.
// `estado` avisa de cuándo empieza (true) y termina (false) la subida, para el botón y
// para volver a pedir lo que cuelga del registro. Subirlo puede cerrar su tarea: el
// contador del menú se vuelve a pedir.
func (p *Pantalla) subirDocumento(modelo, registro string, estado func(subiendo bool)) {
	if p.ElegirFicheros == nil || p.svc == nil {
		return
	}
	p.ElegirFicheros(func(rutas []string) {
		ruta, ok := primeroCon(rutas, ExtensionesDocumento)
		if !ok {
			return
		}
		estado(true)
		p.Repintar()
		p.fondo(func() {
			datos, err := os.ReadFile(ruta) //nolint:gosec // G304: la ruta la eligió quien usa la app en el selector del sistema
			cerradas := 0
			if err == nil {
				ctx, cancelar := p.ctx()
				defer cancelar()
				cerradas, err = p.svc.SubirAdjunto(ctx, modelo, registro, filepath.Base(ruta),
					mime.TypeByExtension(strings.ToLower(filepath.Ext(ruta))), datos)
			}
			enHilo(func() {
				switch {
				case err != nil:
					p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
				case cerradas > 0:
					p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, p.t("docs.uploaded"), p.t("docs.taskDone")))
				default:
					p.avisar(ui.AvisoExito, p.t("docs.uploaded"))
				}
				estado(false)
				p.contarTareas()
				p.Repintar()
			})
		})
	})
}

// importar elige uno o varios ficheros y los importa en el modelo abierto: CSV y, en
// los que reciben facturas, facturas electrónicas en XML.
func (p *Pantalla) importar() {
	l := p.lista
	m := l.Modelo
	if p.ElegirFicheros == nil || p.svc == nil || l.Importando {
		return
	}
	extensiones := []string{".csv"}
	if m.Recibe {
		extensiones = append(extensiones, ".xml")
	}
	p.ElegirFicheros(func(rutas []string) {
		elegidas := slices.DeleteFunc(slices.Clone(rutas), func(r string) bool {
			return !slices.Contains(extensiones, strings.ToLower(filepath.Ext(r)))
		})
		if !m.Recibe && len(elegidas) > 1 {
			elegidas = elegidas[:1] // sin `multiple`: uno
		}
		if len(elegidas) == 0 {
			return
		}
		l.Importando = true
		p.Repintar()
		p.fondo(func() {
			r := p.importarRutas(m.Clave, elegidas)
			enHilo(func() {
				l.Importando = false
				p.avisarImportacion(r)
				if r.creados+r.actualizados > 0 && l.Modelo.Clave == m.Clave {
					p.escrito(m)
				}
				p.Repintar()
			})
		})
	})
}

// importacion es lo que pasó con todos los ficheros de una importación.
type importacion struct {
	creados, actualizados int
	ignoradas, fallos     []string
}

// importarRutas sube los ficheros uno tras otro y suma lo que pasó. Con varios, cada
// fallo lleva delante el nombre de su fichero.
func (p *Pantalla) importarRutas(modelo string, rutas []string) importacion {
	var r importacion
	for _, ruta := range rutas {
		nombre := filepath.Base(ruta)
		datos, err := os.ReadFile(ruta) //nolint:gosec // G304: la ruta la eligió quien usa la app en el selector del sistema
		var res api.ERPResultadoImportacion
		if err == nil {
			ctx, cancelar := p.ctx()
			res, err = p.svc.Importar(ctx, modelo, datos, strings.EqualFold(filepath.Ext(ruta), ".xml"))
			cancelar()
		}
		if err != nil {
			motivo := MensajeDeError(err, p.tr)
			if len(rutas) > 1 {
				motivo = nombre + ": " + motivo
			}
			r.fallos = append(r.fallos, motivo)
			continue
		}
		r.creados += res.Creados
		r.actualizados += res.Actualizados
		for _, c := range res.Ignoradas {
			if !slices.Contains(r.ignoradas, c) {
				r.ignoradas = append(r.ignoradas, c)
			}
		}
		for _, f := range res.Fallos {
			fila := p.t("list.importFailedRow", i18n.Vars{"row": f.Fila, "error": f.Error})
			if len(rutas) > 1 {
				fila = nombre + " · " + fila
			}
			r.fallos = append(r.fallos, fila)
		}
	}
	return r
}

// avisarImportacion cuenta el resultado en un aviso y los fallos, fila a fila, en otros
// (como mucho cinco): quien importa mil filas quiere saber cuáles no entraron.
func (p *Pantalla) avisarImportacion(r importacion) {
	tono := ui.AvisoExito
	if len(r.fallos) > 0 {
		tono = ui.AvisoInfo
	}
	desc := ""
	if len(r.ignoradas) > 0 {
		desc = p.t("list.importIgnored", i18n.Vars{"columns": strings.Join(r.ignoradas, ", ")})
	}
	p.avisos.Notificar(ui.NuevoAviso(tono, p.t("list.importResult", i18n.Vars{
		"created": r.creados, "updated": r.actualizados, "failed": len(r.fallos),
	}), desc))
	for _, f := range r.fallos[:min(5, len(r.fallos))] {
		p.avisar(ui.AvisoPeligro, f)
	}
}

// conFicheros engancha subir e importar solo si hay selector de ficheros: sin él, los
// botones no se pintan o quedan inertes. Se llama al pintar porque el selector lo pone
// quien monta la app, después de crear la pantalla.
func (p *Pantalla) conFicheros() {
	p.engancharBanco()
	if p.ElegirFicheros == nil {
		p.lista.AlImportar, p.tareas.AlSubir, p.oblig.AlSubirDoc = nil, nil, nil
		return
	}
	p.lista.AlImportar = p.importar
	p.tareas.AlSubir = func(a api.ERPActividad) {
		p.subirDocumento(a.Modelo, a.Registro, func(subiendo bool) {
			p.tareas.Ocupada = ""
			if subiendo {
				p.tareas.Ocupada = a.ID
			} else if p.Vista == VistaTareas {
				p.cargarTareas()
			}
		})
	}
	p.oblig.AlSubirDoc = func(reg string) {
		p.subirDocumento("obligacion", reg, func(subiendo bool) {
			if !subiendo {
				p.cargarAdjuntosObligacion(reg)
			}
		})
	}
}

// --- Banco ------------------------------------------------------------------------

// ExtensionesExtracto son las del `accept` de «Subir movimientos»: el Norma 43 con los
// nombres que le da cada banco.
var ExtensionesExtracto = []string{".n43", ".q43", ".aeb", ".csb", ".txt"}

// engancharBanco conecta el banco con el servicio. Se llama al pintar: el selector de
// ficheros y el abridor de apps los pone quien monta la app.
func (p *Pantalla) engancharBanco() {
	b := p.banco
	b.AlCambio, b.Buscar, b.AlAbrirURL = p.Repintar, p.buscarRelacion, abrirURL
	b.AlCasar = func(id, factura string) {
		p.hacerBanco(id, func(ctx context.Context) error { return p.svc.ConciliarMovimiento(ctx, id, factura) })
	}
	b.AlExplicar = func(id, motivo string) {
		p.hacerBanco(id, func(ctx context.Context) error { return p.svc.ExplicarMovimiento(ctx, id, motivo) })
	}
	b.AlReabrir = func(id string) {
		p.hacerBanco(id, func(ctx context.Context) error { return p.svc.ReabrirMovimiento(ctx, id) })
	}
	b.AlCrear = p.crearFacturaMovimiento
	b.AlSubir, b.AlPedirFactura = nil, nil
	if p.ElegirFicheros != nil {
		b.AlSubir = p.subirExtracto
	}
	if p.AbrirApp != nil {
		b.AlPedirFactura = func(t api.ERPTareaBanco) {
			p.AbrirApp("mail", PedirFacturaRuta(p.tr, p.tr.Locale(), t, p.Organizacion))
		}
	}
}

// cargarBanco pide las tareas y, una vez por visita, le pasa a Sytue AI los cargos que
// aún no tienen pista; si encontró algo, se vuelve a pedir.
func (p *Pantalla) cargarBanco() {
	if p.svc == nil {
		return
	}
	b := p.banco
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		d, err := p.svc.Banco(ctx)
		enHilo(func() {
			if err != nil {
				b.Fallo = MensajeDeError(err, p.tr)
				p.Repintar()
				return
			}
			b.Datos, b.Fallo = &d, ""
			p.Repintar()
			if b.SoloLectura || b.pistas || !slices.ContainsFunc(d.Pendientes, func(t api.ERPTareaBanco) bool {
				return t.Importe < 0 && t.Empresa == ""
			}) {
				return
			}
			b.pistas = true
			p.fondo(func() {
				ctx, cancelar := p.ctx()
				defer cancelar()
				if n, err := p.svc.PistasBanco(ctx); err == nil && n > 0 {
					enHilo(func() { b.pistas = false; p.cargarBanco() })
				}
			})
		})
	})
}

// hacerBanco es una acción sobre un movimiento: la ejecuta, avisa si falla y recarga.
func (p *Pantalla) hacerBanco(id string, hacer func(context.Context) error) {
	if p.svc == nil {
		return
	}
	b := p.banco
	b.Ocupada = id
	p.Repintar()
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := hacer(ctx)
		enHilo(func() {
			b.Ocupada = ""
			delete(b.Modo, id)
			if err != nil {
				p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
			}
			p.cargarBanco()
		})
	})
}

// subirExtracto sube uno o varios Norma 43, en bruto, y cuenta lo que hizo el servicio.
func (p *Pantalla) subirExtracto() {
	b := p.banco
	if p.ElegirFicheros == nil || p.svc == nil || b.Subiendo {
		return
	}
	p.ElegirFicheros(func(rutas []string) {
		elegidas := slices.DeleteFunc(slices.Clone(rutas), func(r string) bool {
			return !slices.Contains(ExtensionesExtracto, strings.ToLower(filepath.Ext(r)))
		})
		if len(elegidas) == 0 {
			return
		}
		b.Subiendo = true
		p.Repintar()
		p.fondo(func() {
			for _, r := range elegidas {
				datos, err := os.ReadFile(r) //nolint:gosec // G304: la ruta la eligió quien usa la app en el selector del sistema
				var res api.ERPResultadoExtracto
				if err == nil {
					ctx, cancelar := p.ctx()
					res, err = p.svc.ImportarExtracto(ctx, datos)
					cancelar()
				}
				enHilo(func() {
					if err != nil {
						p.avisar(ui.AvisoPeligro, MensajeDeError(err, p.tr))
						return
					}
					p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, p.t("bank.imported", i18n.Vars{"count": res.Nuevos}),
						p.t("bank.importedDetail", i18n.Vars{"matched": res.Casados, "explained": res.Explicados, "pending": res.Pendientes})))
				})
				if err != nil {
					break
				}
			}
			enHilo(func() { b.Subiendo = false; p.cargarBanco() })
		})
	})
}

// crearFacturaMovimiento abre el diálogo de la factura de un movimiento.
func (p *Pantalla) crearFacturaMovimiento(t api.ERPTareaBanco) {
	if p.lienzo == nil {
		return
	}
	f := NuevaFacturaMovimiento(p.tr, t)
	f.Requisitos = p.Requisitos
	f.Estrecha = p.Ventana.Width > 0 && p.Ventana.Width < ui.CorteSM
	var d *ui.Dialogo
	cerrar := func() { d.Cerrar() }
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// Al ancho de verdad del cuerpo: a 512 fijos, en la hoja de 390 quedaban 37 piezas
	// fuera del panel (el concepto, el proveedor y el IVA cortados a la derecha).
	ancho := anchoCuerpoDialogo(ui.DialogoMD, p.Ventana.Width)
	cuerpo, pie := nuevoVivo(f.Cuerpo(v, ancho)), nuevoVivo(f.Pie(cerrar))
	d = ui.NuevoDialogo(f.Titulo(), cuerpo.c).Con(ui.DialogoMD, pie.c)
	// El diálogo monta su cuerpo y su pie una vez: lo que se rehace es lo de dentro.
	// Antes se asignaba otro `Cuerpo` y no se veía: el IVA elegido, los requisitos
	// desplegados y la rueda de «Guardar y casar» no llegaban a la pantalla.
	pintar := func() {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		cuerpo.poner(f.Cuerpo(v, ancho))
		pie.poner(f.Pie(cerrar))
		d.Recolocar()
	}
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	f.AlCambio = pintar
	if p.ElegirFicheros != nil {
		f.AlElegirPDF = func() {
			p.ElegirFicheros(func(rutas []string) {
				if r, ok := primeroCon(rutas, ExtensionesDocumento); ok {
					f.PDF = r
					pintar()
				}
			})
		}
	}
	f.AlGuardar = func() { p.guardarFacturaMovimiento(f, d, pintar) }
	d.Mostrar(p.lienzo)
}

// guardarFacturaMovimiento busca el contacto por su nombre (o lo crea), crea la factura
// con la fecha y el total del movimiento, sube el PDF o deja la tarea de subirlo hoy, y
// casa el movimiento con ella.
func (p *Pantalla) guardarFacturaMovimiento(f *FacturaMovimiento, d *ui.Dialogo, pintar func()) {
	nombre := f.Nombre()
	if p.svc == nil || nombre == "" || f.Guardando {
		return
	}
	f.Guardando, f.Fallo = true, ""
	pintar()
	t := f.Tarea
	modelo, rol := "factura_compra", "proveedor"
	if f.Entra() {
		modelo, rol = api.ERPFacturaVenta, "cliente"
	}
	base, iva := Desglosar(f.Total(), f.Tipo)
	fecha, pdf := f.Fecha(), f.PDF
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		err := p.escribirFacturaMovimiento(ctx, t, modelo, rol, nombre, pdf, map[string]any{
			"fecha": fecha, "base": base, "iva": iva, "total": f.Total(), "notas": t.Descripcion,
		})
		enHilo(func() {
			if err != nil {
				f.Guardando, f.Fallo = false, MensajeDeError(err, p.tr)
				pintar()
				return
			}
			d.Cerrar()
			p.contarTareas()
			p.cargarBanco()
		})
	})
}

// escribirFacturaMovimiento hace la escritura de «Guardar y casar»: el contacto (el que
// se llame así, o uno nuevo), la factura, su PDF o la tarea de subirlo, y el casado.
func (p *Pantalla) escribirFacturaMovimiento(ctx context.Context, t api.ERPTareaBanco, modelo, rol, nombre, pdf string, datos map[string]any) error {
	contacto := ""
	if pag, err := p.svc.Listar(ctx, "contacto", nombre, "", 8); err == nil {
		for _, c := range pag.Registros {
			if strings.EqualFold(c.Etiqueta, nombre) {
				contacto = c.ID
			}
		}
	}
	if contacto == "" {
		nuevo, err := p.svc.Crear(ctx, "contacto", map[string]any{"nombre": nombre, "tipo": "organizacion"})
		if err != nil {
			return err
		}
		contacto = nuevo.ID
	}
	datos[rol] = contacto
	factura, err := p.svc.Crear(ctx, modelo, datos)
	if err != nil {
		return err
	}
	// Con el PDF, la factura queda completa. Sin él, queda la tarea de subirlo hoy: una
	// factura sin su papel no sirve ante Hacienda.
	if pdf == "" {
		_ = p.svc.CrearActividad(ctx, modelo, factura.ID, "subir_documento",
			p.t("bank.uploadTask", i18n.Vars{"company": nombre}), HoyISO(time.Now()))
	} else {
		contenido, err := os.ReadFile(pdf) //nolint:gosec // G304: la ruta la eligió quien usa la app en el selector del sistema
		if err != nil {
			return err
		}
		if _, err := p.svc.SubirAdjunto(ctx, modelo, factura.ID, filepath.Base(pdf),
			mime.TypeByExtension(strings.ToLower(filepath.Ext(pdf))), contenido); err != nil {
			return err
		}
	}
	return p.svc.ConciliarMovimiento(ctx, t.ID, factura.ID)
}
