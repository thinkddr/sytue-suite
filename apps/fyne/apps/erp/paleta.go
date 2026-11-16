// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"cmp"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Lo que el ERP pone en la paleta de la suite (⌘K)
//
// La tercera decisión de interfaz del ERP: sustituye a los menús anidados. No hay que
// saber en qué submenú vive «albaranes», basta con escribirlo.
//
// Hasta ahora era una paleta PROPIA, con su ⌘K que la barra le cedía. Ahora es la de la
// suite (`ui.PaletaDeLaSuite`, la de la barra), y el ERP le pone tres CAPAS, las mismas
// que `ComandosDelErp` en la web: las pantallas («Ir a»: Libro diario, Banco, Tareas…,
// las mismas que el menú enseña a esta persona), las ÁREAS del catálogo (doce como
// mucho) y los REGISTROS («FV-2026-0012», «Acme S.L.»), que busca el servicio con sus
// permisos y que, mientras contesta, dejan la paleta en «buscando».

const (
	// CapaVistas, CapaAreas y CapaRegistros son los ids de las tres capas, los de la web.
	CapaVistas    = "erp:vistas"
	CapaAreas     = "erp"
	CapaRegistros = "erp:registros"
	// MaxCoincidencias es el `limite: 12` de las áreas en la web.
	MaxCoincidencias = 12
	// MaxRegistrosPaleta es el `MAX_REGISTROS_PALETA` de la web: los registros que se
	// piden al servicio por consulta.
	MaxRegistrosPaleta = 8
	// MinimoBusquedaRegistros son las letras desde las que se pregunta al servicio: con
	// una casaría media base de datos y no le diría nada a nadie.
	MinimoBusquedaRegistros = 2
)

// usarPaleta engancha el ERP a una paleta: le pone sus capas y oye lo escrito para
// buscar los registros. La de la barra sustituye a la propia de la pantalla suelta.
func (p *Pantalla) usarPaleta(pa *ui.PaletaDeLaSuite) {
	if p.paleta == pa {
		return
	}
	if p.paleta != nil {
		for _, id := range []string{CapaVistas, CapaAreas, CapaRegistros} {
			p.paleta.QuitarCapa(id)
		}
		if p.dejarPaleta != nil {
			p.dejarPaleta()
		}
	}
	p.paleta = pa
	p.dejarPaleta = pa.Oir(p.alConsultarPaleta)
	p.ponerCapas()
}

// ponerCapas registra las tres capas con lo que la pantalla sabe ahora.
func (p *Pantalla) ponerCapas() {
	p.ponerCapaVistas()
	p.ponerCapaAreas()
	p.ponerCapaRegistros()
}

// ponerCapaVistas son las pantallas que no son modelos, con las condiciones del menú.
func (p *Pantalla) ponerCapaVistas() {
	grupo := p.t("palette.groupViews")
	capa := ui.CapaDePaleta{}
	for _, d := range p.pantallasPaleta {
		capa.Comandos = append(capa.Comandos, ui.ComandoDePaleta{
			ID: "vista:" + d.Rotulo, Rotulo: d.Rotulo, Grupo: grupo, Ejecutar: d.AlPulsar,
		})
	}
	p.paleta.PonerCapa(CapaVistas, capa)
}

// ponerCapaAreas son TODOS los modelos del catálogo, encendidos o no: la web no los
// filtra por módulo activo, y por eso la paleta llega donde el menú no. La pista dice
// el nombre del área («Contabilidad»), no su clave interna («consolidacion»).
func (p *Pantalla) ponerCapaAreas() {
	grupo := p.t("palette.groupAreas")
	areas := map[string]string{}
	for _, m := range p.Catalogo.Modulos {
		areas[m.Clave] = m.Etiqueta
	}
	capa := ui.CapaDePaleta{Limite: MaxCoincidencias}
	for _, m := range p.Modelos() {
		modelo := m
		capa.Comandos = append(capa.Comandos, ui.ComandoDePaleta{
			ID: "modelo:" + m.Clave, Rotulo: m.Etiqueta, Grupo: grupo,
			Pista:    cmp.Or(areas[m.Modulo], m.Modulo),
			Ejecutar: func() { p.Abrir(modelo) },
		})
	}
	p.paleta.PonerCapa(CapaAreas, capa)
}

// ponerCapaRegistros son lo que contestó el servicio a la consulta de ahora, ya
// filtrado por él. Solo lo que se puede abrir aquí: un registro de un modelo que el
// catálogo no trae no tendría ficha que enseñar.
func (p *Pantalla) ponerCapaRegistros() {
	grupo := p.t("palette.groupRecords")
	porClave := map[string]Modelo{}
	for _, m := range p.Modelos() {
		porClave[m.Clave] = m
	}
	capa := ui.CapaDePaleta{Buscando: p.buscandoRegistros}
	for _, r := range p.registrosPaleta {
		m, hay := porClave[r.Modelo]
		if !hay {
			continue
		}
		rotulo := cmp.Or(r.Etiqueta, r.Ref, r.ID)
		pista := m.Etiqueta
		if r.Ref != "" && r.Ref != rotulo {
			pista = r.Ref + " · " + m.Etiqueta
		}
		reg := r
		capa.Comandos = append(capa.Comandos, ui.ComandoDePaleta{
			ID: "registro:" + r.Modelo + ":" + r.ID, Rotulo: rotulo, Grupo: grupo, Pista: pista,
			YaFiltrado: true,
			Ejecutar:   func() { p.abrirRegistro(reg.Modelo, reg.ID) },
		})
	}
	p.paleta.PonerCapa(CapaRegistros, capa)
}

// alConsultarPaleta es el efecto de `ComandosDelErp`: con dos letras o más pide los
// registros al servicio. Los de una consulta anterior se sueltan al cambiarla: mientras
// llega la respuesta nueva, la paleta dice «buscando» y no enseña los de otra búsqueda.
func (p *Pantalla) alConsultarPaleta(abierta bool, consulta string) {
	q := strings.TrimSpace(consulta)
	if !abierta || len([]rune(q)) < MinimoBusquedaRegistros {
		p.registrosPaleta, p.consultaRegistros, p.buscandoRegistros = nil, "", false
		p.buscarEnPaleta("")
		p.ponerCapaRegistros()
		return
	}
	if q != p.consultaRegistros {
		p.registrosPaleta = nil
	}
	p.consultaRegistros, p.buscandoRegistros = q, p.svc != nil
	p.ponerCapaRegistros()
	p.buscarEnPaleta(q)
}

// encontrados recoge lo que contestó el servicio a `q`. Si entretanto se ha escrito otra
// cosa, o se ha cerrado la paleta, no se toca nada: una respuesta lenta de «ac» no pisa
// la de «acme».
func (p *Pantalla) encontrados(q string, regs []Registro) {
	if !p.paleta.Abierta() || strings.TrimSpace(p.paleta.Consulta()) != q {
		return
	}
	p.registrosPaleta, p.consultaRegistros, p.buscandoRegistros = regs, q, false
	p.ponerCapaRegistros()
}

// Medidas de FilaPaleta, la fila del selector de relaciones.
var (
	paletaPXRelacion = tema.Espacio(4)
	altoFilaRelacion = tema.TextoSM.Linea + 2*tema.Espacio(2)
)

// FilaPaleta es la opción del selector de relaciones (era la de la paleta antes de que
// pasara a `PaletaDeComandos`): el nombre y, apagado a la derecha, la clave del módulo. Es un BOTÓN en la web —se pulsa, se enfoca y se anuncia—, así
// que aquí es un widget y no un contenedor: la regla de `ui.go` es elegir por
// comportamiento.
type FilaPaleta struct {
	ui.Pulsable

	Modelo Modelo
	// Resaltada es la fila del cursor: `bg-menu-selected text-text`.
	Resaltada bool
	// AlEntrar avisa de que el ratón ha entrado, para mover el cursor.
	AlEntrar func()
	// Margen es el relleno horizontal; cero es el `px-4` de la paleta. Las opciones
	// del selector de relaciones, que son la misma fila, llevan `px-3`.
	Margen float32
}

func (f *FilaPaleta) margen() float32 {
	if f.Margen > 0 {
		return f.Margen
	}
	return paletaPXRelacion
}

var _ fyne.Widget = (*FilaPaleta)(nil)

// NuevaFilaPaleta crea una fila de la paleta.
func NuevaFilaPaleta(m Modelo, resaltada bool, alPulsar func()) *FilaPaleta {
	f := &FilaPaleta{Modelo: m, Resaltada: resaltada}
	f.Nombre, f.AlPulsar = m.Etiqueta, alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// MouseIn mueve el cursor a esta fila además del hover de `Pulsable`.
func (f *FilaPaleta) MouseIn(e *desktop.MouseEvent) {
	f.Pulsable.MouseIn(e)
	if f.AlEntrar != nil {
		f.AlEntrar()
	}
}

// MedidaFilaPaleta es el `py-2` sobre la caja de `text-sm`; el ancho lo pone el panel.
func MedidaFilaPaleta(*FilaPaleta) fyne.Size { return fyne.NewSize(0, altoFilaRelacion) }

// CreateRenderer construye el fondo y los dos textos.
func (f *FilaPaleta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaPaletaRenderer{
		f:      f,
		aro:    ui.NuevoAro(0, ui.SinHueco, v),
		fondo:  ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, 0),
		nombre: ui.Texto(f.Modelo.Etiqueta, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
		modulo: ui.Texto(f.Modelo.Modulo, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.fondo.Objetos()...)
	r.objetos = append(r.objetos, r.nombre, r.modulo)
	r.Refresh()
	return r
}

type filaPaletaRenderer struct {
	f              *FilaPaleta
	aro            *ui.Aro
	fondo          *ui.Caja
	nombre, modulo *canvas.Text
	objetos        []fyne.CanvasObject
}

func (r *filaPaletaRenderer) MinSize() fyne.Size           { return MedidaFilaPaleta(r.f) }
func (r *filaPaletaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaPaletaRenderer) Destroy()                     {}

func (r *filaPaletaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Redimensionar(t)
	anchoMod := ui.Medir(r.f.Modelo.Modulo, tema.TextoXS, ui.Normal).Width
	ui.ColocarTexto(r.modulo, tema.TextoXS, t.Width-r.f.margen()-anchoMod,
		(t.Height-tema.TextoXS.Linea)/2)
	cabe := max(t.Width-2*r.f.margen()-anchoMod-tema.Espacio(2), 0)
	// El nombre SIN recortar, siempre: truncar sobre lo truncado deja «…» para siempre
	// en cuanto el primer layout llega con ancho cero.
	ui.Truncar(r.nombre, r.f.Modelo.Etiqueta, tema.TextoSM, ui.Normal, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoSM, r.f.margen(), (t.Height-tema.TextoSM.Linea)/2)
}

func (r *filaPaletaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta, fondo := tema.Color("text-muted", v), color.NRGBA{}
	if r.f.Resaltada {
		tinta, fondo = tema.Color("text", v), tema.Color("menu-selected", v)
	}
	r.aro.Ver(r.f.Enfocado())
	r.aro.Pintar(v)
	r.fondo.Pintar(fondo, color.NRGBA{})
	r.nombre.Color = tinta
	r.nombre.Refresh()
	r.modulo.Color = tema.Color("text-subtle", v)
	r.modulo.Refresh()
}
