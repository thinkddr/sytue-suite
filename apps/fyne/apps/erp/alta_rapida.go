// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// altaRapida conserva la ficha y los controles mientras el panel está abierto.
// Las altas encadenadas usan el mismo formulario y el mismo cliente del ERP.
type altaRapida struct {
	ficha    *Ficha
	dialogo  *ui.Dialogo
	repintar func()
}

// PuedeCrearRelacion aplica el catálogo activo y los permisos del modelo destino.
func (p *Pantalla) PuedeCrearRelacion(clave string) bool {
	for _, m := range p.Modelos() {
		if m.Clave == clave {
			return slices.Contains(p.Catalogo.Activos, m.Modulo) && p.PuedeEscribir(m.Modulo)
		}
	}
	return false
}

func (p *Pantalla) conectarAltas(f *Ficha) {
	f.BuscarRelacion = p.buscarRelacion
	f.PuedeCrearRelacion = p.PuedeCrearRelacion
	f.CrearRelacion = func(clave string, creado func(Registro)) {
		for _, m := range p.Modelos() {
			if m.Clave == clave {
				p.AltaRapida(m, nil, nil, creado)
				return
			}
		}
	}
}

func (p *Pantalla) repintarFicha(f *Ficha) {
	if f.alRepintar != nil {
		f.alRepintar()
		return
	}
	p.Repintar()
}

// cuerpoAlta reutiliza los campos de Ficha; sólo omite los identificadores internos
// que tampoco presenta AltaRapida en la web. No incluye el seguimiento de un registro.
func (f *Ficha) cuerpoAlta(v tema.Variante, ancho float32) fyne.CanvasObject {
	f.ancho = ancho + tema.Borde + 2*FichaPX
	campos := make([]fyne.CanvasObject, 0, len(f.controles))
	for _, k := range f.controles {
		if slices.Contains([]string{"externo_id", "tpv_huella", "tpv_operacion"}, k.Campo.Clave) {
			continue
		}
		if inhabilitable, ok := k.obj.(fyne.Disableable); ok {
			if f.Guardando {
				inhabilitable.Disable()
			} else {
				inhabilitable.Enable()
			}
		}
		if k.entrada != nil {
			k.entrada.Inerte(f.Guardando)
		}
		if k.selector != nil {
			k.selector.Inerte(f.Guardando)
		}
		if k.area != nil {
			k.area.Inerte(f.Guardando)
		}
		if k.relacion != nil {
			k.relacion.Inerte = f.Guardando
			k.relacion.Refresh()
		}
		o, _ := f.bloqueCampo(v, k)
		campos = append(campos, o)
	}
	formulario := container.New(ui.Columna{Hueco: SeparaCampos}, campos...)
	if f.Fallo == "" {
		return formulario
	}
	fallo := ui.NuevoParrafo(f.Fallo, tema.TextoRaiz, ui.Normal, "danger-text", v)
	fallo.Resize(fyne.NewSize(ancho, ui.AltoParrafo(fallo, ancho)))
	return container.New(ui.Columna{Hueco: tema.Espacio(4)}, fallo, formulario)
}

// AltaRapida abre el panel derecho sin abandonar la lista ni la ficha de origen.
// El registro devuelto se entrega al selector o a la acción que pidió el alta.
func (p *Pantalla) AltaRapida(m Modelo, semilla *Registro, etiquetas map[string]string, creado func(Registro)) *ui.Dialogo {
	if p.lienzo == nil || !p.PuedeCrearRelacion(m.Clave) {
		return nil
	}
	f := NuevaFicha(p.tr, m, nil, semilla, etiquetas)
	p.conectarAltas(f)
	d := ui.NuevoDialogo(f.Titulo(), nil).ALaDerecha()
	d.RotuloCerrar = p.t("action.close")
	cerrar := func() {
		if f.Guardando {
			return
		}
		d.Cerrar()
		for i, alta := range p.altas {
			if alta.dialogo == d {
				p.altas = slices.Delete(p.altas, i, i+1)
				break
			}
		}
	}
	d.AlCerrar = cerrar
	f.AlCerrar = cerrar
	f.AlGuardar = func() { p.guardar(f) }
	f.alCreado = func(r Registro) {
		cerrar()
		p.revision++
		p.olvidar(m.Clave)
		if creado != nil {
			creado(r)
		}
		p.Repintar()
	}
	f.alRepintar = func() {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		ancho := ui.AnchoCuerpoDialogo(d, p.Ventana)
		posicion := d.Desplazador().Offset
		d.CambiarCuerpo(f.cuerpoAlta(v, ancho))
		d.Desplazador().Offset = posicion
		cerrarBoton := ui.NuevoBoton(p.t("action.close"), cerrar).Con(ui.Fantasma, ui.TallaMD)
		save := ui.NuevoBoton(p.t("action.save"), f.AlGuardar)
		cerrarBoton.Inerte, save.Cargando = f.Guardando, f.Guardando
		d.CambiarPie(container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cerrarBoton, save))
		d.Recolocar()
	}
	p.altas = append(p.altas, &altaRapida{ficha: f, dialogo: d, repintar: f.alRepintar})
	f.alRepintar()
	d.Mostrar(p.lienzo)
	return d
}
