// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Mover a otra unidad (Sytue Office)
//
// Entre el disco y una nube, o entre dos nubes, no hay quien mueva: se copia cada fichero,
// se comprueba que llegó entero y solo entonces se quita el origen
// (`EnEquipo.MoverEntreUnidades`, `local/archivos.Remotas.Trasladar`). Puede tardar, así
// que va en una hoja con su avance —«3 de 12: acta.pdf»— y «Cancelar», que para entre
// fichero y fichero: lo que ya llegó se queda movido y lo demás, donde estaba. Al final, un
// aviso dice cuántos se movieron y, si algo no, una hoja lo lista con su porqué.

// moverEntreUnidades lleva els a la carpeta destino de otra unidad; donde es su nombre,
// para el título. Se llama en la rutina de la interfaz y devuelve la hoja (para el
// guardián).
func (p *Pantalla) moverEntreUnidades(els []Elemento, destino, donde string) *ui.Dialogo {
	if p.win == nil || p.EnEquipo == nil || p.EnEquipo.MoverEntreUnidades == nil || len(els) == 0 {
		return nil
	}
	ctx, cancelar := context.WithCancel(context.Background())
	v := p.variante()
	titulo := strings.ReplaceAll(p.textoEquipo("moviendo"), "{name}", donde)
	paso := container.NewStack(parrafo(v, " "))
	barra := ui.NuevaBarraProgreso(titulo, 0)
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(3)}, paso, barra)
	var boton *ui.Boton
	parado := false
	parar := func() {
		if parado {
			return
		}
		parado = true
		cancelar()
		boton.Cargando = true
		boton.Refresh()
	}
	boton = ui.NuevoBoton(p.tr.T("files.cancel", nil), parar).Con(ui.Secundario, ui.TallaMD)
	d := ui.NuevoDialogo(titulo, cuerpo).Con(ui.DialogoSM, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, boton))
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	// El aspa y Escape cancelan; la hoja se queda hasta que termina lo que va.
	d.AlCerrar = parar
	d.Mostrar(p.win.Canvas())
	enMover := p.EnEquipo.MoverEntreUnidades
	go func() {
		defer cancelar()
		inf := enMover(ctx, els, destino, func(hechos, total int, nombre string) {
			texto := ""
			if nombre != "" {
				texto = strings.NewReplacer("{current}", strconv.Itoa(min(hechos+1, total)), "{total}", strconv.Itoa(total),
					"{name}", nombre).Replace(p.textoEquipo("moviendoPaso"))
			}
			fraccion := 0.0
			if total > 0 {
				fraccion = float64(hechos) / float64(total)
			}
			enHilo(func() {
				paso.Objects = []fyne.CanvasObject{parrafo(v, texto)}
				paso.Refresh()
				barra.Fraccion = fraccion
				barra.Refresh()
			})
		})
		enHilo(func() {
			d.Cerrar()
			clear(p.explor.Seleccion)
			p.ancla = -1
			p.avisarTraslado(els, inf)
		})
		// Lo de después va aunque se haya cancelado el traslado: lo que llegó se movió.
		go p.cargarCuota() //nolint:contextcheck // la cuota tiene su propio plazo y no depende del traslado
		p.recargarVista()  //nolint:contextcheck // recargar tiene su propio plazo y no depende del traslado
	}()
	return d
}

// avisarTraslado dice cómo fue: todo movido es un éxito; si algo se quedó en el origen, un
// peligro con cuántos se movieron y una hoja con lo que no y por qué. Se llama en la rutina
// de la interfaz.
func (p *Pantalla) avisarTraslado(els []Elemento, inf InformeMover) {
	switch {
	case inf.Err != nil:
		p.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, p.tr.T("move.failed", nil), ""))
	case !inf.Cancelado && len(inf.NoMovidos) == 0 && inf.Movidos == inf.Total:
		titulo := p.tr.T("files.move.done", i18n.Vars{"count": inf.Movidos})
		if len(els) == 1 {
			titulo = p.tr.T("move.done", i18n.Vars{"name": els[0].Nombre})
		}
		p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, titulo, ""))
	default:
		clave := "moverParcial"
		if inf.Cancelado {
			clave = "moverCancelado"
		}
		texto := strings.NewReplacer("{moved}", strconv.Itoa(inf.Movidos), "{total}", strconv.Itoa(inf.Total)).Replace(p.textoEquipo(clave))
		p.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, texto, ""))
		if len(inf.NoMovidos) > 0 {
			p.dialogoNoMovidos(inf.NoMovidos)
		}
	}
}

// maxNoMovidos es cuántas líneas lista la hoja; las demás se cuentan.
const maxNoMovidos = 30

// dialogoNoMovidos lista lo que se quedó en el origen, una línea por elemento con su
// porqué. Se llama en la rutina de la interfaz y la devuelve (para el guardián).
func (p *Pantalla) dialogoNoMovidos(lineas []string) *ui.Dialogo {
	if p.win == nil {
		return nil
	}
	v := p.variante()
	ancho := AnchoCuerpoDialogo(ui.DialogoMD, p.explor.Ventana)
	filas := make([]fyne.CanvasObject, 0, min(len(lineas), maxNoMovidos)+1)
	for i, l := range lineas {
		if i == maxNoMovidos {
			filas = append(filas, parrafoEn(v, "… +"+strconv.Itoa(len(lineas)-maxNoMovidos), ancho))
			break
		}
		filas = append(filas, parrafoEn(v, l, ancho))
	}
	var cuerpo fyne.CanvasObject = container.New(ui.Columna{Hueco: tema.Espacio(2)}, filas...)
	if len(filas) > 6 {
		// Muchas: en una caja que se desplaza, del alto de la del selector de mover.
		cuerpo = container.New(&altoFijo{alto: AltoCajaMover}, container.NewVScroll(cuerpo))
	}
	var d *ui.Dialogo
	cerrar := ui.NuevoBoton(p.tr.T("ui.close", nil), func() { d.Cerrar() }).Con(ui.Primario, ui.TallaMD)
	d = ui.NuevoDialogo(p.textoEquipo("moverNoMovidos"), cuerpo).Con(ui.DialogoMD,
		container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cerrar))
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	d.Mostrar(p.win.Canvas())
	return d
}

// parrafoEn es parrafo repartido en otro ancho: el del cuerpo de un diálogo `md`.
func parrafoEn(v tema.Variante, texto string, ancho float32) fyne.CanvasObject {
	lineas := ui.Repartir(texto, tema.TextoBase, ui.Normal, ancho)
	b := &bloqueTexto{}
	hijos := make([]fyne.CanvasObject, 0, len(lineas))
	for _, l := range lineas {
		t := ui.Texto(l, tema.TextoBase, ui.Normal, tema.Color("text-muted", v))
		b.lineas = append(b.lineas, t)
		hijos = append(hijos, t)
	}
	return container.New(b, hijos...)
}
