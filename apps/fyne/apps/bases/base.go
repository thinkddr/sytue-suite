// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

// paridad-web: apps/frontend/apps/bases/src/pages/base/[id].astro

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Base es una base abierta (BaseApp.tsx): sus tablas y la que se está mirando.
type Base struct {
	ID string
	// Datos es la base; con Role vacío, quien mira solo accede a alguna de sus tablas.
	Datos    api.BasesBase
	Tablas   []api.BasesTable
	Activa   string
	Cargando bool
	// irA es la tabla y el registro que pide abrir un enlace (el aviso de un
	// comentario): la ficha y sus comentarios se abren cuando llega la tabla.
	irA   struct{ tabla, registro string }
	Fallo string
	// Estado es lo último que ha pasado (el `role="status"` de la web): un registro
	// añadido, qué registros se ven tras cargar otra ventana, un valor pisado, dónde ha
	// quedado lo exportado. En la web solo lo oye un lector de pantalla; aquí se lee.
	Estado string

	// La tabla activa (tabla.go): sus campos, las ventanas de registros en memoria, el
	// rol que el servicio dice para ella y si le quedan fórmulas por recalcular.
	Campos       []api.BasesField
	vent         ventanas
	RolTabla     string
	Recalculando bool
	// Vistas son las guardadas de la tabla; VistaID, la que se mira ("" = todos los
	// registros, en rejilla).
	Vistas  []api.BasesView
	VistaID string
	// lectura es la búsqueda y el orden de un momento encima de la vista (lectura.go);
	// busqueda, su campo, el mismo entre repintados, y espera, el reloj de lo tecleado.
	lectura  lectura
	busqueda *ui.Entrada
	espera   *time.Timer
	// opciones y etiquetas son los registros de cada tabla enlazada, por id de tabla.
	opciones  map[string][]opcion
	etiquetas map[string]map[string]string
	// La rejilla y su desplazamiento de lado son los MISMOS entre repintados: el foco,
	// la celda que se edita y lo desplazado no se pierden al guardar.
	rejilla *rejilla
	lado    *ui.DesplazaLado

	// Los enlaces públicos de la vista de formulario que se mira (compartir.go): los que
	// hay, el cupo del que se va a crear y la dirección del recién creado, que solo se
	// enseña esa vez.
	enlaces       []api.BasesFormLink
	cupo          *ui.Entrada
	enlaceNuevo   string
	enlaceCopiado bool
	falloEnlaces  string

	// portadas son las miniaturas de la vista de galería (galeria.go).
	portadas portadas
	// resumen son los totales de lo que se mira (resumen.go); nil sin pedir o si fallan.
	resumen *api.BasesSummary
	// rt es el tiempo real de la base (tiemporeal.go); nil sin servicio. propias son las
	// operaciones de celda lanzadas desde aquí, para no volver a pedir su eco.
	rt      *enVivo
	propias opsPropias
}

// tabla es la tabla activa, o nil.
func (b *Base) tabla() *api.BasesTable {
	for i := range b.Tablas {
		if b.Tablas[i].ID == b.Activa {
			return &b.Tablas[i]
		}
	}
	return nil
}

// Abrir abre una base por su id y pide su estructura.
func (p *Pantalla) Abrir(id string) { p.AbrirEnlace(id, "", "") }

// AbrirEnlace abre una base y, si llegan, la tabla y el registro de un aviso de
// comentario (`?table=&record=`): la ficha de ese registro con su hilo abierto. Si la
// tabla no se ve, se queda en la primera, como en la web.
func (p *Pantalla) AbrirEnlace(id, tabla, registro string) {
	p.dejarDeSeguir(p.abierta)
	p.abierta = &Base{
		ID: id, Cargando: true, vent: sinVentanas(),
		opciones: map[string][]opcion{}, etiquetas: map[string]map[string]string{}, cupo: ui.NuevaEntrada(""),
	}
	p.abierta.cupo.Rotulo, p.abierta.cupo.Ancho = p.t("formlink.cupo"), 8*tema.Rem
	p.abierta.cupo.PonerTexto("500")
	if tabla != "" && registro != "" {
		p.abierta.irA.tabla, p.abierta.irA.registro = tabla, registro
	}
	p.seguir(p.abierta)
	p.Repintar()
	p.cargarEstructura()
}

// cargarEstructura pide la base y sus tablas. Quien solo accede a alguna tabla no puede
// leer la base (404): se queda con las tablas, como en la web.
func (p *Pantalla) cargarEstructura() {
	b := p.abierta
	if b == nil || p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		datos, err := p.svc.GetBase(ctx, b.ID)
		var e *api.Error
		parcial := errors.As(err, &e) && e.Status == http.StatusNotFound
		var tablas []api.BasesTable
		if err == nil || parcial {
			var errTablas error
			if tablas, errTablas = p.svc.ListTables(ctx, b.ID); errTablas == nil && parcial {
				datos, err = api.BasesBase{ID: b.ID}, nil
			} else if errTablas != nil && !parcial {
				err = errTablas
			}
		}
		enHilo(func() {
			if p.abierta != b {
				return // se ha vuelto a la lista o se ha abierto otra mientras tanto
			}
			b.Cargando = false
			if err != nil {
				b.Fallo = p.mensajeDeError(err)
				p.Repintar()
				return
			}
			b.Datos, b.Tablas, b.Fallo = datos, tablas, ""
			activa := b.tablaInicial()
			if activa != b.Activa || b.rejilla == nil {
				p.elegirTabla(activa)
				return
			}
			p.ponerRejilla() // un renombrado cambia el nombre accesible de la rejilla
			p.Repintar()
		})
	})
}

// tablaInicial es la tabla que se abre al llegar la estructura: la del enlace si se
// ve, la activa si sigue existiendo, o la primera. Un enlace a una tabla que no se ve
// se olvida, para no abrir la ficha de otro registro.
func (b *Base) tablaInicial() string {
	if slices.ContainsFunc(b.Tablas, func(t api.BasesTable) bool { return t.ID == b.irA.tabla }) {
		return b.irA.tabla
	}
	b.irA.tabla, b.irA.registro = "", ""
	if b.tabla() != nil {
		return b.Activa
	}
	if len(b.Tablas) > 0 {
		return b.Tablas[0].ID
	}
	return ""
}

// hacer lanza una escritura y, si falla, enseña su mensaje en la base abierta.
func (p *Pantalla) hacer(f func() error, despues func()) {
	b := p.abierta
	p.fondo(func() {
		err := f()
		enHilo(func() {
			if err != nil {
				if p.abierta == b && b != nil {
					b.Fallo = p.mensajeDeError(err)
					p.Repintar()
				}
				return
			}
			despues()
		})
	})
}

// confirmar es `useConfirm`: el título es el rótulo del botón, que va en rojo.
func (p *Pantalla) confirmar(mensaje, rotulo string, si func()) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := ui.NuevoParrafo(mensaje, tema.TextoSM, ui.Normal, "text", v).A(p.anchoDialogo(400))
	var d *ui.Dialogo
	d = ui.Confirmacion(rotulo, p.t("ui.cancel"), rotulo, true, cuerpo, func() { d.Cerrar() }, func() { d.Cerrar(); si() })
	d.RotuloCerrar = p.t("ui.close")
	d.Mostrar(p.ventana.Canvas())
}

// preguntar es `usePrompt`: devuelve lo escrito ya recortado, y nada si queda vacío.
func (p *Pantalla) preguntar(mensaje, valor, rotulo string, alAceptar func(string)) {
	d := ui.Pregunta(mensaje, valor, p.t("ui.cancel"), rotulo, p.t("ui.required"), func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			alAceptar(s)
		}
	})
	d.RotuloCerrar = p.t("ui.close")
	d.Mostrar(p.ventana.Canvas())
}

func (p *Pantalla) renombrarBase() {
	b := p.abierta
	p.preguntar(p.t("base.namePrompt"), b.Datos.Name, p.t("ui.save"), func(nombre string) {
		if nombre == b.Datos.Name {
			return
		}
		p.hacer(func() error {
			ctx, cancelar := p.ctx()
			defer cancelar()
			return p.svc.UpdateBase(ctx, b.ID, nombre, b.Datos.Description)
		}, p.cargarEstructura)
	})
}

func (p *Pantalla) borrarBase() {
	b := p.abierta
	p.confirmar(p.t("base.deleteConfirm", i18n.Vars{"name": b.Datos.Name}), p.t("common.delete"), func() {
		p.hacer(func() error {
			ctx, cancelar := p.ctx()
			defer cancelar()
			return p.svc.DeleteBase(ctx, b.ID)
		}, p.volverALista)
	})
}

// objetoBase es BaseApp.tsx: la cabecera con el nombre y lo que el propietario puede
// hacer con la base, y debajo sus tablas en pestañas.
//
// La cabecera son dos líneas: «Todas las bases» encima, y el nombre con «Compartir» y el
// menú «⋯» a su derecha. Renombrar, añadir una tabla y borrar la base se hacen poco y
// van en el menú: sueltos, en un teléfono, partían la cabecera en tres filas de botones.
func (p *Pantalla) objetoBase(v tema.Variante) fyne.CanvasObject {
	b := p.abierta
	nombre := b.Datos.Name
	if nombre == "" {
		nombre = p.t("nav.title")
	}
	volver := ui.NuevoEnlace(p.t("base.back"), tema.TextoSM, ui.Normal, "primary-text", p.volverALista)
	titulo := ui.NuevoRotulo(nombre, tema.Texto2XL, ui.SemiNegrita, "text")
	titulo.Truncar = true
	linea := []fyne.CanvasObject{titulo}
	if puedeEsquema(b.Datos.Role) {
		compartir := ui.NuevoBoton(p.t("share.open"), p.dialogoCompartir).Con(ui.Secundario, ui.TallaSM)
		linea = append(linea, compartir, p.menuMas(p.t("actions.more")+" · "+nombre, func() []fyne.CanvasObject {
			borrar := ui.NuevoElementoMenu(p.t("base.delete"), iconos.Papelera, p.borrarBase)
			borrar.Peligro = true
			menu := []fyne.CanvasObject{
				ui.NuevoElementoMenu(p.t("base.rename"), iconos.Lapiz, p.renombrarBase),
				ui.NuevoElementoMenu(p.t("base.editDescription"), iconos.Ninguno, p.editarDescripcion),
				ui.NuevoElementoMenu(p.t("activity.open"), iconos.Reloj, func() { p.abrirActividad() }),
				ui.NuevoElementoMenu(p.t("table.add"), iconos.Mas, p.nuevaTabla),
			}
			if p.Elegir != nil {
				menu = append(menu, ui.NuevoElementoMenu(p.t("import.open"), iconos.Subir, p.importarCSV))
			}
			return append(menu, ui.NuevoSeparadorMenu(), borrar)
		}))
	}
	cabecera := container.New(ui.Columna{Hueco: tema.Espacio(1)},
		container.New(ui.Fila{}, volver),
		container.New(extremos{Hueco: tema.Espacio(2), Centro: true}, linea...))
	hijos := []fyne.CanvasObject{cabecera}
	if d := strings.TrimSpace(b.Datos.Description); d != "" {
		// `line-clamp-3 max-w-3xl`: la descripción, bajo el nombre.
		hijos = append(hijos, ui.NuevoParrafo(d, tema.TextoSM, ui.Normal, "text-muted", v).
			A(min(p.anchoContenido(), 48*tema.Rem)).Recortado(3))
	}
	if b.Fallo != "" {
		hijos = append(hijos, alerta(b.Fallo, v))
	}
	hijos = append(hijos, p.objetoEnVivo(b, v)...)
	// Arriba, con el fallo: al pie de una rejilla de cien filas no lo vería nadie.
	if b.Estado != "" {
		hijos = append(hijos, ui.NuevoParrafo(b.Estado, tema.TextoSM, ui.Normal, "text-muted", v).A(p.anchoContenido()))
	}
	switch {
	case b.Cargando:
		hijos = append(hijos, ui.NuevoEsqueleto(0, 12*tema.Rem))
	case len(b.Tablas) == 0:
		// Sin tablas, añadir la primera (o importarla) es lo único que hay que hacer: va
		// a la vista, y no solo en el menú.
		vc := vacioDe(sinTablas, permisos{esquema: puedeEsquema(b.Datos.Role), escribir: puedeEscribir(b.Datos.Role)}, false)
		hijos = append(hijos, p.vacioGuiado(vc, map[accionVacio]func(){
			crearTabla:  p.nuevaTabla,
			importarCsv: p.importarCSV,
		}, false))
	default:
		hijos = append(hijos, container.New(ui.Columna{Hueco: tema.Espacio(4)}, p.pestanasDeTablas(), p.objetoTabla(v)))
	}
	if b.Recalculando {
		hijos = append(hijos, ui.NuevoRotulo(p.t("grid.recalculating"), tema.TextoSM, ui.Normal, "text-muted"))
	}

	return container.New(acotado{Margen: tema.Espacio(6)},
		container.New(ui.Columna{Hueco: tema.Espacio(4)}, hijos...))
}

// pestanasDeTablas es el `Tabs` con una pestaña por tabla; el contenido es de la activa.
func (p *Pantalla) pestanasDeTablas() fyne.CanvasObject {
	b := p.abierta
	rotulos := make([]string, len(b.Tablas))
	activa := 0
	for i, x := range b.Tablas {
		rotulos[i] = x.Name
		if x.ID == b.Activa {
			activa = i
		}
	}
	pest := ui.NuevasPestanas(rotulos, nil)
	pest.Nombre = p.t("base.tables")
	pest.Activa = activa
	pest.AlCambiar = func(i int) { p.elegirTabla(b.Tablas[i].ID) }
	return pest
}

// menuMas es el botón «⋯» que despliega un menú de acciones. Los elementos se arman al
// abrirlo: cada uno lleva su menú, y un menú no se vuelve a mostrar.
func (p *Pantalla) menuMas(rotulo string, elementos func() []fyne.CanvasObject) *ui.BotonIcono {
	var b *ui.BotonIcono
	b = ui.NuevoBotonIcono(iconos.Puntos, rotulo, func() {
		m := ui.NuevoMenu(elementos()...)
		m.Rotulo = rotulo
		m.Mostrar(b)
	})
	// A la altura de los botones de la barra y con el glifo entero de la rejilla (24): a
	// 16, los tres puntos de «⋯» eran motas de 1,7 px que apenas se veían junto a
	// «Compartir»; así son de 2,5 y en la tinta del texto, no en la del enlace.
	b.Lado, b.TamIcono = ui.LadoIconoLG, iconos.TallaXL
	b.Trazo, b.TrazoEncima = "text-muted", "text"
	return b
}
