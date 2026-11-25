// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Los estados vacíos de una base abierta (lib/vacios.ts): cada uno dice qué falta y
// ofrece el paso siguiente, pero solo el que la persona puede dar. Quien no es
// propietario no ve «Añadir tabla» para que el servicio se lo niegue después.

// estadoVacio es lo que falta: tablas, campos, registros, o registros que pasen el recorte.
type estadoVacio int

const (
	sinTablas estadoVacio = iota
	sinCampos
	sinRegistros
	sinResultados
)

// accionVacio es el paso que ofrece un estado vacío.
type accionVacio string

const (
	crearTabla     accionVacio = "crearTabla"
	importarCsv    accionVacio = "importarCsv"
	anadirCampo    accionVacio = "anadirCampo"
	anadirRegistro accionVacio = "anadirRegistro"
	verTodos       accionVacio = "verTodos"
)

// permisos: esquema (tablas y campos) es del propietario; escribir, de propietario y editor.
type permisos struct{ esquema, escribir bool }

// vacio es el estado completo: glifo, claves de título y descripción y las acciones en
// orden (la primera es la principal).
type vacio struct {
	icono       iconos.Icono
	titulo      string
	descripcion string
	acciones    []accionVacio
}

// vacioDe es `vacioDe` de la web. busqueda distingue, en un recorte, la búsqueda escrita
// de una vista con filtros.
func vacioDe(e estadoVacio, p permisos, busqueda bool) vacio {
	switch e {
	case sinTablas:
		if p.esquema {
			return vacio{iconos.Tabla, "base.noTables", "base.noTables.owner", []accionVacio{crearTabla, importarCsv}}
		}
		return vacio{iconos.Tabla, "base.noTables", "base.noTables.reader", nil}
	case sinCampos:
		if p.esquema {
			return vacio{iconos.CampoDeFormulario, "grid.noFields", "grid.noFields.owner", []accionVacio{anadirCampo}}
		}
		return vacio{iconos.CampoDeFormulario, "grid.noFields", "grid.noFields.reader", nil}
	case sinRegistros:
		if p.escribir {
			return vacio{iconos.Lista, "grid.empty", "grid.empty.editor", []accionVacio{anadirRegistro}}
		}
		return vacio{iconos.Lista, "grid.empty", "grid.empty.reader", nil}
	default:
		// Volver a verlo todo no cambia nada guardado: lo puede hacer cualquiera.
		if busqueda {
			return vacio{iconos.Buscar, "grid.noMatches", "grid.noMatches.body", []accionVacio{verTodos}}
		}
		return vacio{iconos.Buscar, "grid.viewEmpty", "grid.viewEmpty.body", []accionVacio{verTodos}}
	}
}

// esRecorte dice si lo que se mira es un recorte de la tabla: una búsqueda escrita o una
// vista con filtros. Ordenar no recorta.
func esRecorte(l lectura, v *api.BasesView) bool {
	return strings.TrimSpace(l.Q) != "" || (v != nil && len(v.Config.Filters) > 0)
}

// rotuloAccion es la clave del botón de cada acción: las mismas que en el resto de la app.
var rotuloAccion = map[accionVacio]string{
	crearTabla:     "table.add",
	importarCsv:    "import.open",
	anadirCampo:    "field.add",
	anadirRegistro: "grid.addRecord",
	verTodos:       "grid.showAll",
}

var iconoAccion = map[accionVacio]iconos.Icono{
	crearTabla:     iconos.Mas,
	importarCsv:    iconos.Subir,
	anadirCampo:    iconos.Mas,
	anadirRegistro: iconos.Mas,
}

// vacioGuiado es el `VacioGuiado` de la web: el `ui.Vacio` con el glifo y las acciones
// que el estado concede y que tienen qué hacer. La primera va en primario y la segunda
// al lado, en secundario.
func (p *Pantalla) vacioGuiado(vc vacio, hacer map[accionVacio]func(), compacto bool) *ui.Vacio {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	x := ui.NuevoVacio(p.t(vc.titulo), p.t(vc.descripcion))
	x.Compacto = compacto
	lado := ui.LadoGlifoVacio
	if compacto {
		lado = ui.LadoGlifoVacioCompacto
	}
	x.Glifo = iconos.Trazo(vc.icono, lado, "text-subtle", v)
	var botones []fyne.CanvasObject
	for _, a := range vc.acciones {
		f := hacer[a]
		if f == nil {
			continue
		}
		variante := ui.Primario
		if len(botones) > 0 {
			variante = ui.Secundario
		}
		b := botonSM(p.t(rotuloAccion[a]), variante, f)
		if i, ok := iconoAccion[a]; ok {
			b = b.ConIcono(i, 14)
		}
		botones = append(botones, b)
	}
	if len(botones) > 0 {
		x.Accion = botones[0]
	}
	if len(botones) > 1 {
		x.AccionSecundaria = botones[1]
	}
	return x
}

// verTodosLosRegistros sale de un recorte vacío: fuera la búsqueda y, si la vista
// filtra, a «todos». El orden elegido se queda: no esconde nada.
func (p *Pantalla) verTodosLosRegistros() {
	b := p.abierta
	if b == nil {
		return
	}
	if b.busqueda != nil {
		b.busqueda.PonerTexto("")
	}
	if b.espera != nil {
		b.espera.Stop()
		b.espera = nil
	}
	if v := b.vista(); v != nil && len(v.Config.Filters) > 0 {
		b.lectura.Q = ""
		p.elegirVista("")
		return
	}
	l := b.lectura
	l.Q = ""
	p.cambiarLectura(l)
}

// vacioLista es «Todavía no tienes ninguna base» con su dibujo, el `icon="database"` del
// `EmptyState` de BasesApp.tsx (BASE-05): era solo texto. El paso siguiente es el campo
// «Nombre de la nueva base» que tiene justo encima, así que no lleva botón.
func (p *Pantalla) vacioLista(v tema.Variante) *ui.Vacio {
	x := ui.NuevoVacio(p.t("bases.empty.title"), p.t("bases.empty.body"))
	x.Glifo = iconos.Trazo(iconos.BaseDeDatos, ui.LadoGlifoVacio, "text-subtle", v)
	return x
}
