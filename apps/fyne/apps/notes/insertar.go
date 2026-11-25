// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// EL MENÚ «+» DE LA BARRA Y SUS SELECTORES: insertar con el ratón lo mismo que «/» con el
// teclado. Es el MISMO catálogo (`comandos`, en el mismo orden): un panel con la caja de
// buscar arriba y las filas debajo —icono, nombre y, a la derecha, el atajo que lo
// escribe—, que se recorren con las flechas, Intro elige y Escape cierra devolviendo el
// foco al cuerpo.
//
// Y los tres selectores pequeños de lo que se inserta con datos: el de una etiqueta de
// estado (su texto y uno de los seis colores), el de una fecha (dd/mm/aaaa) y el título
// de un desplegable. Los mismos abren al pulsar una etiqueta, una fecha o la cabecera de
// un desplegable ya escritos, para cambiarlos.

// menuInsertar abre «+» colgando de `disparador`.
func (e *EditorNota) menuInsertar(disparador fyne.CanvasObject) {
	var em *ui.Emergente
	campo := ui.NuevaEntrada(e.tr.T("picker.search", nil)).Con(ui.CampoSM)
	campo.Rotulo = e.tr.T("editor.insert", nil)
	campo.Ancho = anchoMenuSug
	lista := container.NewStack()
	activa := 0
	var visibles []comando
	elegir := func(c comando) {
		if em != nil {
			em.Cerrar()
		}
		e.reenfocar()
		e.aplicarComando(c)
	}
	var pintar func()
	pintar = func() {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		visibles = visibles[:0]
		for _, c := range filtrarComandos(campo.Texto(), e.rotuloComando) {
			if e.comandoDisponible(c) {
				visibles = append(visibles, c)
			}
		}
		activa = min(activa, max(len(visibles)-1, 0))
		filas := make([]fyne.CanvasObject, 0, len(visibles)+1)
		for i, c := range visibles {
			s := sugerencia{glifo: c.glifo, icono: c.icono, rotulo: e.rotuloComando(c), detalle: pistaComando(c), elegir: func() { elegir(c) }}
			f := nuevaFilaSugerencia(e, s, i, i == activa, anchoMenuSug, v)
			f.alEncima = func() {
				if activa != i {
					activa = i
					pintar()
				}
			}
			filas = append(filas, f)
		}
		if len(visibles) == 0 {
			filas = append(filas, container.New(ui.Margen{Izq: pxFilaCmd, Der: pxFilaCmd, Arr: pyFilaCmd, Abj: pyFilaCmd},
				lineaTok(e.tr.T("editor.slashNoResults", nil), tema.TextoSM, ui.Normal, "text-muted", v)))
		}
		col := container.New(ui.Columna{}, filas...)
		d := container.NewVScroll(col)
		alto := min(col.MinSize().Height, float32(filasMenuCmd)*ui.SueloTactil(max(tema.TextoSM.Linea, cajaDibCmd)+2*pyFilaCmd))
		d.SetMinSize(fyne.NewSize(anchoMenuSug, alto))
		if len(visibles) > 0 {
			d.ScrollToOffset(fyne.NewPos(0, max(0, float32(activa+1)*filas[0].MinSize().Height-alto)))
		}
		lista.Objects = []fyne.CanvasObject{d}
		lista.Refresh()
		if em != nil {
			em.Recolocar()
		}
	}
	campo.AlCambiar(func(string) { activa = 0; pintar() })
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		switch k.Name {
		case fyne.KeyDown, fyne.KeyUp:
			if len(visibles) > 0 {
				activa = ui.MoverActiva(activa, len(visibles), k.Name)
				pintar()
			}
			return true
		case fyne.KeyReturn, fyne.KeyEnter:
			if activa < len(visibles) {
				elegir(visibles[activa])
			}
			return true
		case fyne.KeyEscape:
			if em != nil {
				em.Cerrar()
			}
			e.reenfocar()
			return true
		default:
		}
		return false
	}
	pintar()
	em = ui.MostrarPopover(disparador, container.New(ui.Columna{Hueco: tema.Espacio(2)}, campo, lista))
	em.Rotulo = e.tr.T("editor.insert", nil)
	e.insertando = em
}

// ClavesColorEstado son los rótulos de los seis colores de la lista cerrada.
var ClavesColorEstado = map[string]string{
	"gris": "editor.statusGray", "azul": "editor.statusBlue", "verde": "editor.statusGreen",
	"amarillo": "editor.statusYellow", "rojo": "editor.statusRed", "morado": "editor.statusPurple",
}

// abrirEstado es el selector de una etiqueta de estado: su texto y su color. Sobre una que
// ya está (el cursor en ella), la cambia; si no, pone una nueva.
func (e *EditorNota) abrirEstado(disparador fyne.CanvasObject) {
	var em *ui.Emergente
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	actual, sobre := e.doc.especialEn(e.cur)
	sobre = sobre && actual.Estado != ""
	valor, texto := "gris", ""
	if sobre {
		valor = actual.Estado
		if a, z, ok := e.doc.tramoEspecial(e.cur); ok {
			texto = e.doc.TextoEntre(a, z)
		}
	}
	campo := ui.NuevaEntrada(strings.ToUpper(e.tr.T("editor.status", nil))).Con(ui.CampoSM)
	campo.Rotulo = e.tr.T("editor.status", nil)
	campo.PonerTexto(texto)
	campo.Ancho = tema.Espacio(56)
	aplicar := func() {
		t := strings.TrimSpace(campo.Texto())
		if t == "" {
			t = strings.ToUpper(e.tr.T(ClavesColorEstado[valor], nil))
		}
		if em != nil {
			em.Cerrar()
		}
		e.reenfocar()
		if sobre {
			p, val := e.cur, valor
			e.hacer(func() { e.doc.CambiarEtiqueta(p, t, func(x *Tramo) { x.Estado = val }) })
			return
		}
		e.insertarEspecial(t, Tramo{Estado: valor})
	}
	muestras := make([]*Muestra, 0, len(EstadosPermitidos))
	for _, c := range EstadosPermitidos {
		fondo, _ := coloresEstado(c)
		m := NuevaMuestra(tema.Color(fondo, v), LadoBotonEd, 0, e.tr.T(ClavesColorEstado[c], nil), nil)
		m.Activa = c == valor
		m.AlPulsar = func() {
			valor = c
			for _, o := range muestras {
				o.Activa = o == m
				o.Refresh()
			}
		}
		if ui.EsTactil() {
			m.Celda = ui.SueloTactilBarra
		}
		muestras = append(muestras, m)
	}
	objs := make([]fyne.CanvasObject, len(muestras))
	for i, m := range muestras {
		objs[i] = m
	}
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		switch k.Name {
		case fyne.KeyReturn, fyne.KeyEnter:
			aplicar()
			return true
		case fyne.KeyEscape:
			em.Cerrar()
			e.reenfocar()
			return true
		default:
		}
		return false
	}
	listo := ui.NuevoBoton(e.tr.T("common.done", nil), aplicar)
	listo.Talla = ui.TallaSM
	em = ui.MostrarPopover(disparador, container.New(ui.Columna{Hueco: tema.Espacio(2)},
		campo, container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, objs...), listo))
	em.Rotulo = e.tr.T("editor.status", nil)
}

// abrirFecha es el selector de una fecha: dd/mm/aaaa, hoy de partida. Sobre una que ya
// está, la cambia.
func (e *EditorNota) abrirFecha(disparador fyne.CanvasObject) {
	var em *ui.Emergente
	actual, sobre := e.doc.especialEn(e.cur)
	sobre = sobre && actual.Fecha != ""
	f := ahoraEditor()
	if sobre {
		if t, err := time.Parse(time.DateOnly, actual.Fecha); err == nil {
			f = t
		}
	}
	campo := ui.CampoFecha(ui.CampoSM)
	campo.Rotulo = e.tr.T("editor.date", nil)
	campo.PonerTexto(ui.EscribirFecha(f))
	aplicar := func() {
		t, vacia, ok := ui.LeerFecha(campo.Texto(), time.Local)
		if vacia || !ok {
			return
		}
		if em != nil {
			em.Cerrar()
		}
		e.reenfocar()
		if sobre {
			p := e.cur
			e.hacer(func() {
				e.doc.CambiarEtiqueta(p, fechaTexto(e, t), func(x *Tramo) { x.Fecha = t.Format(time.DateOnly) })
			})
			return
		}
		e.insertarFecha(t)
	}
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		switch k.Name {
		case fyne.KeyReturn, fyne.KeyEnter:
			aplicar()
			return true
		case fyne.KeyEscape:
			em.Cerrar()
			e.reenfocar()
			return true
		default:
		}
		return false
	}
	listo := ui.NuevoBoton(e.tr.T("common.done", nil), aplicar)
	listo.Talla = ui.TallaSM
	em = ui.MostrarPopover(disparador, container.New(ui.Columna{Hueco: tema.Espacio(2)}, campo, listo))
	em.Rotulo = e.tr.T("editor.date", nil)
}

// abrirTituloCaja cambia el título del desplegable del bloque `i`.
func (e *EditorNota) abrirTituloCaja(i int, disparador fyne.CanvasObject) {
	if i < 0 || i >= len(e.doc.Bloques) || e.doc.Bloques[i].Caja == nil {
		return
	}
	var em *ui.Emergente
	campo := ui.NuevaEntrada(e.tr.T("editor.expand", nil)).Con(ui.CampoSM)
	campo.Rotulo = e.tr.T("editor.expand", nil)
	campo.Ancho = tema.Espacio(64)
	campo.PonerTexto(e.doc.Bloques[i].Caja.Titulo)
	aplicar := func() {
		t := strings.TrimSpace(campo.Texto())
		em.Cerrar()
		e.reenfocar()
		e.hacer(func() { e.doc.TitularCaja(i, t) })
	}
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		switch k.Name {
		case fyne.KeyReturn, fyne.KeyEnter:
			aplicar()
			return true
		case fyne.KeyEscape:
			em.Cerrar()
			e.reenfocar()
			return true
		default:
		}
		return false
	}
	em = ui.MostrarPopover(disparador, campo)
	em.Rotulo = e.tr.T("editor.expand", nil)
}

// menuLenguaje ofrece los lenguajes de un bloque de código, el actual marcado.
func (e *EditorNota) menuLenguaje(disparador fyne.CanvasObject) {
	i := e.cur.B
	if i >= len(e.doc.Bloques) || e.doc.Bloques[i].Tipo != Codigo {
		return
	}
	actual := e.doc.Bloques[i].Lenguaje
	opciones := []*opcionMenu{{rotulo: e.tr.T("editor.codeLanguageNone", nil), marcada: actual == "", hacer: func() {
		e.hacer(func() { e.doc.PonerLenguaje(i, "") })
	}}, nil}
	for _, l := range LenguajesPermitidos {
		opciones = append(opciones, &opcionMenu{rotulo: nombreLenguaje(l), marcada: actual == l, hacer: func() {
			e.hacer(func() { e.doc.PonerLenguaje(i, l) })
		}})
	}
	e.mostrarMenu(disparador, e.tr.T("editor.codeLanguage", nil), opciones)
}

// fechaTexto es el texto de una fecha en el idioma de la app («24 nov 2026»).
func fechaTexto(e *EditorNota, f time.Time) string { return i18n.FormatDate(e.tr.Locale(), f) }

// tocarEspecial atiende el toque en una etiqueta de estado, una fecha o la cabecera de un
// desplegable: abre su selector. Dice si lo era.
func (z *zonaEdicion) tocarEspecial(p fyne.Position) bool {
	e := z.e
	x, y := p.X-PXZonaEd, p.Y-PYZonaEd
	for _, a := range z.maquetado().Adornos {
		if a.Tipo == adornoCabecera && x >= a.X && x <= a.X+a.An && y >= a.Y && y <= a.Y+a.Al {
			if x <= a.X+14+tema.Espacio(1.5) && a.caja != nil {
				// La flecha: si al leer la nota se ve abierto (el `open` de la web), en su
				// paso de deshacer. Escribiendo se sigue viendo lo de dentro.
				bloque, abierta := a.Bloque, !a.caja.Abierta
				e.hacer(func() { e.doc.AbrirCaja(bloque, abierta) })
				return true
			}
			e.abrirTituloCaja(a.Bloque, z.anclaEn(fyne.NewPos(PXZonaEd+a.X, PYZonaEd+a.Y+a.Al)))
			return true
		}
	}
	q, ok := z.posEn(p)
	if !ok {
		return false
	}
	t, hay := e.doc.especialEn(q)
	if !hay {
		return false
	}
	// Dentro de la etiqueta, no en su borde: el cursor detrás de una fecha sigue
	// escribiendo.
	a, b, _ := e.doc.tramoEspecial(q)
	if q.O <= a.O || q.O >= b.O {
		return false
	}
	e.cur, e.ancla = q, q
	ancla := e.anclaCursor()
	if t.Estado != "" {
		e.abrirEstado(ancla)
	} else {
		e.abrirFecha(ancla)
	}
	return true
}

// enCodigo dice si el cursor está en un bloque de código: es lo que enseña su barra.
func (e *EditorNota) enCodigo() bool {
	if len(e.doc.Bloques) == 0 {
		return false
	}
	b := e.bloqueActual()
	return b.Tipo == Codigo && (!b.Celda.EnTabla() || b.Celda.Tabla.EsColumnas())
}

// armarBarraCodigo es la barra de un bloque de código: su lenguaje, que se elige de la
// lista cerrada y se guarda como `<code data-lenguaje>`. El botón dice el lenguaje
// actual en su pista y en su nombre accesible.
func (e *EditorNota) armarBarraCodigo() *fyne.Container {
	orden := func(o string) func(fyne.CanvasObject) {
		return func(fyne.CanvasObject) { e.Orden(o, "") }
	}
	e.codigoEd = e.nuevaBarraEd([]accionBarra{{
		clave: "lenguaje", grupo: grupoBloque, prioridad: prioEsencial, icono: iconos.Codigo,
		rotulo: e.tr.T("editor.codeLanguage", nil), abre: true, pulsar: e.menuLenguaje,
	}, {
		// Los números de línea (`data-lineas`) y el ajuste de las líneas largas
		// (`data-ajuste`), como la barra del bloque de código de la web.
		clave: "lineas", grupo: grupoFormato, prioridad: prioEsencial, glifo: NumerosD,
		rotulo: e.tr.T("editor.lineNumbers", nil), pulsar: orden("lineas"),
	}, {
		clave: "ajuste", grupo: grupoFormato, prioridad: prioEsencial, glifo: AjusteD,
		rotulo: e.tr.T("editor.wrap", nil), pulsar: orden("ajuste"),
	}})
	return e.codigoEd.c
}

// AlternarCodigo pone o quita los números de línea («lineas») o el ajuste («ajuste») del
// bloque de código `i`.
func (d *Documento) AlternarCodigo(i int, que string) {
	if i < 0 || i >= len(d.Bloques) || d.Bloques[i].Tipo != Codigo {
		return
	}
	b := &d.Bloques[i]
	switch que {
	case "lineas":
		b.Lineas = !b.Lineas
	case "ajuste":
		b.Ajuste = !b.Ajuste
	}
}

// refrescarBarraCodigo pone en el botón el lenguaje del bloque del cursor.
func (e *EditorNota) refrescarBarraCodigo() {
	b := e.botones["lenguaje"]
	if b == nil || !e.enCodigo() {
		return
	}
	nombre := e.tr.T("editor.codeLanguageNone", nil)
	if l := e.bloqueActual().Lenguaje; l != "" {
		nombre = nombreLenguaje(l)
	}
	b.Rotulo = e.tr.T("editor.codeLanguage", nil) + ": " + nombre
	if b.pista != nil {
		b.pista.Rotulo = b.Rotulo
	}
}
