// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/formula/analisis"
)

// Libro es el modelo que el lienzo enseña: las hojas, el HTML de cada celda con su
// formato, y el motor de fórmulas compartido que las evalúa.
//
// # No hay un segundo evaluador, y no lo va a haber
//
// `apps/nucleo/formula` es el motor de Sytue Sheets: parsea, resuelve rangos, memoriza
// lo sucio, detecta ciclos y trae 451 funciones atadas por trazas a las de la web.
// Escribir aquí una segunda versión «más sencilla» es exactamente lo que
// `docs/CONVENTIONS.md` §13.2 prohíbe, y en una hoja de cálculo el precio se paga en
// silencio: dos evaluadores que difieren en un caso raro dan dos números distintos
// para el mismo libro.
//
// # Lo que se guarda es el HTML de la celda, y el motor ve su texto
//
// Es el `cells` de `SheetData` tal cual: el `innerHTML` de la celda de la web, con su
// envoltorio de alineación, negrita y cursiva (ver `celda.go`). El motor recibe su
// texto crudo —el literal o el «=fórmula»—, que es lo que enseña la barra de fórmulas.
// Una celda que no se ha tocado sale al documento con los MISMOS bytes con los que
// entró. El resto del formato —número, fuente, color, relleno, bordes— va en mapas
// aparte (`formato.go`), como en la web.
//
// # Deshacer son fotos, no operaciones inversas
//
// Por lo mismo que `sheets/historial.ts`: una inversa por comando es una ocasión por
// comando de que deshacer corrompa en vez de restaurar, y una foto no puede estar mal
// porque es lo que había. Todo cambio pasa por `anotar`, así que ninguno se queda
// fuera. Cincuenta pasos, como la web.
//
// ponytail: cada foto copia todas las hojas. Con 26×100 celdas por hoja son unos
// kilobytes; si un libro creciera lo bastante como para notarlo, el camino es guardar
// la diferencia por hoja, igual que dice la web.
type Libro struct {
	motor          *formula.Engine
	f              foto
	pasado, futuro []foto

	// version cuenta los cambios, para no recalcular el formato condicional en cada
	// pintado si nada ha cambiado.
	version uint64
	// ocultas es lo último que se le dijo al motor de las filas que no se ven, por hoja,
	// y en qué versión: SUBTOTAL lo necesita. Ver sincronizarOcultas.
	ocultas struct {
		version uint64
		por     map[string]string
		// listas son las filtradas y las de a mano de cada hoja, para el motor desechable.
		listas map[string][2][]int
	}
	decorado struct {
		version uint64
		activa  int
		mapa    map[Celda]*decoracion
	}
	// rotulo traduce lo poco que el libro PINTA con palabras (el «Total» de la fila de
	// totales). Lo pone el lienzo; sin él sale la clave.
	rotulo func(clave string) string
	// resolubles son las tablas del libro con los nombres de sus columnas, para las
	// referencias estructuradas; se rehacen cuando cambia la versión.
	resolubles struct {
		version uint64
		lista   []tablaResoluble
	}
}

// foto es el estado entero del libro: lo que se guarda, lo que se deshace.
type foto struct {
	hojas  []hojaLibro
	activa int
	// proteccion es la del libro; inmutable, así que la foto la comparte.
	proteccion *ProteccionLibro
	// nombres son los nombres definidos del libro (`names`); la lista es inmutable —
	// cambiarla es poner otra—, así que la foto la comparte.
	nombres []Nombre
}

// hojaLibro es una `SheetData`, con las celdas por coordenadas y no por referencia A1.
// Los tres mapas guardan solo las celdas que tienen algo.
type hojaLibro struct {
	nombre   string
	celdas   map[Celda]string // HTML
	formatos map[Celda]FormatoNum
	estilos  map[Celda]Estilo
	fijas    Fijas
	// condiciones son las reglas de formato condicional, en orden de prioridad.
	condiciones []ReglaCond
	// validaciones son las de datos; si dos cubren una celda, gana la última.
	validaciones []Validacion
	// filtro es el autofiltro, y proteccion, la de la hoja; inmutables, así que la foto
	// los comparte.
	filtro     *Autofiltro
	proteccion *Proteccion
	// grupFilas y grupCols son el esquema (`rowGroups`, `colGroups`); inmutables, así
	// que la foto los comparte: cambiarlos es poner otra lista.
	grupFilas, grupCols []analisis.Grupo
	// dinamicas son las tablas dinámicas (`pivots`).
	dinamicas []analisis.Dinamica
	// escenarios son los del análisis de hipótesis (`scenarios`).
	escenarios []analisis.Escenario
	// Los objetos de la hoja (`objetos.go`): tablas con formato, minigráficos, gráficos
	// y segmentaciones. Cada elemento es inmutable: cambiarlo es poner otro.
	tablas   []TablaFmt
	minis    []Minigrafico
	graficos []Grafico
	segs     []Segmentacion
	// anchos y altos son las medidas personalizadas de la web (`colWidths`, `rowHeights`):
	// no se pintan, pero se conservan al guardar. Inmutables, así que la foto los comparte.
	anchos, altos map[string]float64
	// fusiones son las celdas combinadas (`merges`), tampoco pintadas y conservadas.
	fusiones []string
	// pestana es el color de la pestaña (`tabColor`, #RRGGBB); "" = sin color.
	pestana string
}

// Fijas es `freeze`: cuántas columnas y filas se quedan quietas desde el origen.
type Fijas struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

func nuevaHoja(nombre string) hojaLibro {
	return hojaLibro{nombre: nombre, celdas: map[Celda]string{}, formatos: map[Celda]FormatoNum{}, estilos: map[Celda]Estilo{}}
}

// clonar es la copia PROFUNDA: los mapas se copian, o deshacer devolvería el mismo mapa
// que se sigue escribiendo. Los valores sí se comparten: son inmutables.
func (h hojaLibro) clonar() hojaLibro {
	h.celdas, h.formatos, h.estilos = maps.Clone(h.celdas), maps.Clone(h.formatos), maps.Clone(h.estilos)
	h.condiciones, h.validaciones = slices.Clone(h.condiciones), slices.Clone(h.validaciones)
	h.tablas, h.minis, h.graficos, h.segs = slices.Clone(h.tablas), slices.Clone(h.minis), slices.Clone(h.graficos), slices.Clone(h.segs)
	h.dinamicas, h.escenarios = slices.Clone(h.dinamicas), slices.Clone(h.escenarios)
	return h
}

// TopeHistorial es el `TOPE` de `sheets/historial.ts`.
const TopeHistorial = 50

// NuevoLibro crea un libro vacío con una hoja.
func NuevoLibro() *Libro { return NuevoLibroCon(nil, 0) }

// NuevoLibroCon crea el libro de un documento ya leído. Todas las hojas entran en el
// motor, no solo la activa: una fórmula puede referirse a otra y sin ella daría #REF!.
func NuevoLibroCon(nombres []string, activa int) *Libro {
	if len(nombres) == 0 {
		nombres = []string{"Hoja1"}
	}
	f := foto{hojas: make([]hojaLibro, len(nombres)), activa: clamp(activa, len(nombres)-1)}
	for i, n := range nombres {
		f.hojas[i] = nuevaHoja(n)
	}
	l := &Libro{}
	l.restaurar(f)
	return l
}

func (l *Libro) h() *hojaLibro { return &l.f.hojas[l.f.activa] }

// Hoja es el nombre de la hoja activa.
func (l *Libro) Hoja() string { return l.h().nombre }

// Hojas son los nombres de las hojas del libro, en orden.
// ColorPestana es el color de la pestaña de la hoja i (#RRGGBB); "" si no tiene.
func (l *Libro) ColorPestana(i int) string {
	if i < 0 || i >= len(l.f.hojas) {
		return ""
	}
	return l.f.hojas[i].pestana
}

func (l *Libro) Hojas() []string {
	out := make([]string, len(l.f.hojas))
	for i, h := range l.f.hojas {
		out[i] = h.nombre
	}
	return out
}

// Activa es el índice de la hoja que se está viendo.
func (l *Libro) Activa() int { return l.f.activa }

// HTML es lo que se guarda en la celda: el `innerHTML` de la web.
func (l *Libro) HTML(c, f int) string { return l.h().celdas[Celda{C: c, F: f}] }

// Crudo es el texto tal cual se escribió: el literal o la fórmula con su «=». Es lo
// que enseña la celda en edición y lo que refleja la barra de fórmulas.
func (l *Libro) Crudo(c, f int) string { return textoPlano(l.HTML(c, f)) }

// Texto es lo que se ve en la celda: `clipDisplay` de la web. Ver Pintura.
func (l *Libro) Texto(c, f int) string {
	t, _ := l.Pintura(c, f)
	return t
}

// Pintura es lo que se PINTA en la celda, que es lo que hace `cellHtml` en la web: el
// valor calculado con su formato si es una fórmula; el literal con su formato si es un
// número y tiene uno; y si no, el literal TAL CUAL —un «007» tecleado se ve «007», no
// el 7 que el motor lee de él—. El color es el del `[Rojo]` de una máscara, o "".
func (l *Libro) Pintura(c, f int) (texto, color string) {
	html := l.HTML(c, f)
	fmtNum, conFmt := l.h().formatos[Celda{C: c, F: f}]
	var pf *FormatoNum
	if conFmt {
		pf = &fmtNum
	}
	if info, ok := infoTablaEn(l.tablasActivas(), c, f); ok && info.papel == papelTotal {
		return l.totalDeTabla(info, l.FilasOcultas(), l.traducir("sheet.total")), ""
	}
	if esFormula(html) {
		return textoConFormato(l.Valor(c, f), pf)
	}
	if html == "" || !conFmt || fmtNum.ID == "general" {
		return textoPlano(html), ""
	}
	n, ok := l.Valor(c, f).(float64)
	if !ok {
		return textoPlano(html), "" // el formato solo re-pinta números
	}
	return aplicarFormato(fmtNum, n)
}

// traducir es el rótulo de esa clave, o la clave.
func (l *Libro) traducir(clave string) string {
	if l.rotulo == nil {
		return clave
	}
	return l.rotulo(clave)
}

// Valor es el valor calculado de una celda de la hoja activa.
func (l *Libro) Valor(c, f int) formula.Value {
	l.sincronizarOcultas()
	return l.motor.ValueAt(l.Hoja(), c, f)
}

// Formato es el envoltorio de la celda: negrita, cursiva y alineación ("", "center" o
// "right"). El atajo de la primera línea es por coste: se pregunta por cada celda
// visible en cada pintado, y una celda sin formato nunca empieza por «<».
func (l *Libro) Formato(c, f int) (negrita, cursiva bool, alinear string) {
	html := l.HTML(c, f)
	if !strings.HasPrefix(strings.TrimSpace(html), "<") {
		return false, false, ""
	}
	p := leerCelda(html)
	return p.b, p.i, p.alinear
}

// Estilo es el formato visual de la celda (el cero si no tiene).
func (l *Libro) Estilo(c, f int) Estilo { return l.h().estilos[Celda{C: c, F: f}] }

// FormatoNumero es el formato de número de la celda: `{id: 'general'}` si no tiene.
func (l *Libro) FormatoNumero(c, f int) FormatoNum {
	if fm, ok := l.h().formatos[Celda{C: c, F: f}]; ok {
		return fm
	}
	return FormatoNum{ID: "general"}
}

// Fijas son los paneles inmovilizados de la hoja activa.
func (l *Libro) Fijas() Fijas { return l.h().fijas }

// ---- escribir ----

// Escribir pone el texto crudo de una celda de la hoja activa conservando su formato.
// Si no cambia nada, no se anota: deshacer no debe gastar un paso en no hacer nada.
//
// Escribir justo a la derecha o debajo de una tabla la hace crecer (`grownTables`), en
// el mismo paso de deshacer.
func (l *Libro) Escribir(c, f int, texto string) {
	html := conTexto(l.HTML(c, f), texto)
	if html == l.HTML(c, f) {
		return
	}
	l.anotar()
	l.poner(l.f.activa, c, f, html)
	if html == "" {
		return
	}
	if crecidas := crecerTablas(l.h().tablas, c, f); crecidas != nil {
		l.cambiarTablas(crecidas)
	}
}

// Vaciar borra el CONTENIDO de un rango en UN paso de deshacer (Supr sobre un rango).
// El formato se queda, como en la web (`clearRange` solo toca `cells`).
func (l *Libro) Vaciar(c1, f1, c2, f2 int) {
	hay := false
	for k := range l.h().celdas {
		if k.C >= c1 && k.C <= c2 && k.F >= f1 && k.F <= f2 {
			hay = true
			break
		}
	}
	if !hay {
		return
	}
	l.anotar()
	for f := f1; f <= f2; f++ {
		for c := c1; c <= c2; c++ {
			l.poner(l.f.activa, c, f, "")
		}
	}
}

// celdaCopia es una celda del portapapeles de alta fidelidad (`ClipCell`): su HTML y
// su formato. nil es «sin formato», que al pegar BORRA el que hubiera.
type celdaCopia struct {
	html string
	num  *FormatoNum
	est  *Estilo
}

// Pegar escribe un bloque a partir de (c0, f0) en UN paso de deshacer. Cada celda
// pegada sustituye el contenido Y el formato de la de destino, como `applyPaste`. Lo que
// se sale de la rejilla (`cols`×`fils`) se descarta, como en la web.
//
// Con `corte`, antes vacía ese rango de la hoja activa —contenido y formato—: cortar
// es MOVER, y se vacía el origen antes de escribir el destino por si se solapan.
func (l *Libro) Pegar(c0, f0, cols, fils int, bloque [][]celdaCopia, corte *[4]int) {
	l.anotar()
	h := l.h()
	if corte != nil {
		for f := corte[1]; f <= corte[3]; f++ {
			for c := corte[0]; c <= corte[2]; c++ {
				l.poner(l.f.activa, c, f, "")
				delete(h.formatos, Celda{C: c, F: f})
				delete(h.estilos, Celda{C: c, F: f})
			}
		}
	}
	for i, fila := range bloque {
		for j, cc := range fila {
			c, f := c0+j, f0+i
			if c >= cols || f >= fils {
				continue
			}
			k := Celda{C: c, F: f}
			l.poner(l.f.activa, c, f, cc.html)
			delete(h.formatos, k)
			delete(h.estilos, k)
			if cc.num != nil {
				h.formatos[k] = *cc.num
			}
			if cc.est != nil {
				h.estilos[k] = *cc.est
			}
		}
	}
}

// CargarEn pone el HTML de una celda de cualquier hoja SIN anotarlo. Es para montar un
// documento recién leído: abrirlo no es un cambio que se pueda deshacer.
func (l *Libro) CargarEn(hoja, c, f int, html string) { l.poner(hoja, c, f, html) }

func (l *Libro) poner(hoja, c, f int, html string) {
	l.version++
	k := Celda{C: c, F: f}
	h := &l.f.hojas[hoja]
	antes := h.celdas[k]
	if html == "" {
		delete(h.celdas, k)
	} else {
		h.celdas[k] = html
	}
	// Una cabecera de tabla da nombre a una columna: si cambia, lo que la web haría al
	// reconstruir el motor en cada cambio (`buildEngine`) aquí se hace a mano.
	if textoPlano(antes) != textoPlano(html) && enCabeceraDeTabla(h.tablas, c, f) {
		l.rehacerMotor()
		return
	}
	l.motor.SetCellAt(h.nombre, c, f, textoPlano(html))
}

// ---- formato (los comandos de la cinta) ----

// rango recorre las celdas de un rectángulo.
func rango(c1, f1, c2, f2 int, fn func(k Celda)) {
	for f := f1; f <= f2; f++ {
		for c := c1; c <= c2; c++ {
			fn(Celda{C: c, F: f})
		}
	}
}

// NegritaOCursiva es el comando `bold`/`italic` sobre un rango: el objetivo es lo
// CONTRARIO de lo que tiene la celda ancla (ac, af), y se aplica a todas las celdas con
// algo escrito —una celda vacía no tiene envoltorio que llevar—.
func (l *Libro) NegritaOCursiva(negrita bool, ac, af, c1, f1, c2, f2 int) {
	ancla := leerCelda(l.HTML(ac, af))
	objetivo := !ancla.i
	if negrita {
		objetivo = !ancla.b
	}
	l.cambiarHTML(c1, f1, c2, f2, func(p *partes) {
		if negrita {
			p.b = objetivo
		} else {
			p.i = objetivo
		}
	})
}

// Alinear es el comando `align`: "", "center" o "right" (lo demás es izquierda).
func (l *Libro) Alinear(a string, c1, f1, c2, f2 int) {
	if a != "center" && a != "right" {
		a = ""
	}
	l.cambiarHTML(c1, f1, c2, f2, func(p *partes) { p.alinear = a })
}

// cambiarHTML reescribe el envoltorio de las celdas con algo escrito. Se anota solo si
// alguna cambia.
func (l *Libro) cambiarHTML(c1, f1, c2, f2 int, fn func(*partes)) {
	nuevo := map[Celda]string{}
	rango(c1, f1, c2, f2, func(k Celda) {
		viejo := l.h().celdas[k]
		p := leerCelda(viejo)
		if p.dentro == "" {
			return
		}
		fn(&p)
		if h := p.html(); h != viejo {
			nuevo[k] = h
		}
	})
	if len(nuevo) == 0 {
		return
	}
	l.anotar()
	for k, h := range nuevo {
		l.poner(l.f.activa, k.C, k.F, h)
	}
}

// Combinar es el comando `merge`: junta el texto del rango, separado por espacios, en
// la celda de arriba a la izquierda y vacía las demás. Es combinación de CONTENIDO, sin
// celda extendida, igual que en la web. Devuelve false si no había rango.
func (l *Libro) Combinar(c1, f1, c2, f2 int) bool {
	if c1 == c2 && f1 == f2 {
		return false
	}
	var textos []string
	rango(c1, f1, c2, f2, func(k Celda) {
		if t := strings.TrimSpace(textoPlano(l.h().celdas[k])); t != "" {
			textos = append(textos, t)
		}
	})
	arriba := leerCelda(l.HTML(c1, f1))
	l.anotar()
	rango(c1, f1, c2, f2, func(k Celda) { l.poner(l.f.activa, k.C, k.F, "") })
	arriba.dentro = escaparHTML(strings.Join(textos, " "))
	l.poner(l.f.activa, c1, f1, arriba.html())
	return true
}

// PonerFormatoNumero es el comando `numfmt`: un preset ("general" lo quita) o, con
// `mascara`, una máscara propia (vacía también lo quita).
func (l *Libro) PonerFormatoNumero(id, mascara string, c1, f1, c2, f2 int) {
	if id != "custom" && (!contiene(idsFormato, id) || id == "general") {
		id = "general"
	}
	l.anotar()
	h := l.h()
	rango(c1, f1, c2, f2, func(k Celda) {
		switch {
		case id == "general", id == "custom" && strings.TrimSpace(mascara) == "":
			delete(h.formatos, k)
		case id == "custom":
			h.formatos[k] = FormatoNum{ID: "custom", Mascara: mascara}
		default:
			h.formatos[k] = FormatoNum{ID: id}
		}
	})
}

// Decimales es el comando `decimals` (±1): una celda en General pasa a Número, y las
// fechas y las máscaras no se tocan —sus decimales no significan nada—.
func (l *Libro) Decimales(delta, c1, f1, c2, f2 int) {
	l.anotar()
	h := l.h()
	rango(c1, f1, c2, f2, func(k Celda) {
		base, ok := h.formatos[k]
		if !ok || base.ID == "general" {
			base = FormatoNum{ID: "number"}
		}
		if base.ID == "date" || base.ID == "custom" {
			return
		}
		d := min(8, max(0, base.decimales()+delta))
		h.formatos[k] = FormatoNum{ID: base.ID, Decimales: &d}
	})
}

// CambiarEstilo es el comando `style`: el parche sobre el estilo de cada celda del
// rango; lo que se queda vacío se quita del mapa.
func (l *Libro) CambiarEstilo(p Parche, c1, f1, c2, f2 int) {
	l.anotar()
	h := l.h()
	rango(c1, f1, c2, f2, func(k Celda) { l.ponerEstilo(h, k, p) })
}

func (l *Libro) ponerEstilo(h *hojaLibro, k Celda, p Parche) {
	if e, ok := fusionar(h.estilos[k], p); ok {
		h.estilos[k] = e
	} else {
		delete(h.estilos, k)
	}
}

// QuitarEstilo es `clearstyle`: el formato visual fuera; el de número y el texto se
// quedan.
func (l *Libro) QuitarEstilo(c1, f1, c2, f2 int) {
	l.anotar()
	h := l.h()
	rango(c1, f1, c2, f2, func(k Celda) { delete(h.estilos, k) })
}

// PonerBordes es el comando `border`: "all" (caja a cada celda), "outer" (solo el
// contorno del rango), "bottom" (el de abajo de cada celda) o "none" (quitar).
func (l *Libro) PonerBordes(modo string, c1, f1, c2, f2 int) {
	l.anotar()
	h := l.h()
	rango(c1, f1, c2, f2, func(k Celda) {
		switch modo {
		case "all":
			l.ponerEstilo(h, k, conBordes(&Bordes{T: true, R: true, B: true, L: true}))
		case "bottom":
			l.ponerEstilo(h, k, conBordes(&Bordes{B: true}))
		case "outer":
			b := Bordes{T: k.F == f1, B: k.F == f2, L: k.C == c1, R: k.C == c2}
			if b != (Bordes{}) {
				l.ponerEstilo(h, k, conBordes(&b))
			}
		default:
			l.ponerEstilo(h, k, conBordes(nil))
		}
	})
}

// Fijar es el comando `freeze`: inmoviliza `cols` columnas y `filas` filas (0, 0
// descongela). Se acotan a la rejilla menos una, como en la web.
func (l *Libro) Fijar(cols, filas int) {
	nuevo := Fijas{Cols: clamp(cols, Columnas-1), Rows: clamp(filas, Filas-1)}
	if nuevo == l.h().fijas {
		return
	}
	l.anotar()
	l.h().fijas = nuevo
}

// ---- hojas ----

// Activar cambia de hoja. Se anota, como en la web: el cambio de hoja también es parte
// del documento (`apply({...model, active: i})`).
func (l *Libro) Activar(i int) {
	if i == l.f.activa || i < 0 || i >= len(l.f.hojas) {
		return
	}
	l.anotar()
	l.f.activa = i
}

// Anadir crea una hoja vacía al final y la activa.
func (l *Libro) Anadir(nombre string) {
	l.anotar()
	f := l.copia()
	f.hojas = append(f.hojas, nuevaHoja(nombre))
	f.activa = len(f.hojas) - 1
	l.restaurar(f)
}

// Duplicar copia la hoja activa justo detrás, con un nombre que no choca —«Datos (2)»,
// «Datos (3)»…—: el motor indexa las hojas por nombre.
func (l *Libro) Duplicar() {
	l.anotar()
	f := l.copia()
	nombres := l.Hojas()
	src := nombres[f.activa]
	n := 2
	for slices.Contains(nombres, src+" ("+strconv.Itoa(n)+")") {
		n++
	}
	i := f.activa + 1
	nueva := f.hojas[f.activa].clonar()
	nueva.nombre = src + " (" + strconv.Itoa(n) + ")"
	f.hojas = slices.Insert(f.hojas, i, nueva)
	f.activa = i
	l.restaurar(f)
}

// Borrar quita la hoja activa. Siempre queda al menos una.
func (l *Libro) Borrar() {
	if len(l.f.hojas) <= 1 {
		return
	}
	l.anotar()
	f := l.copia()
	f.nombres = quitarHojaDeNombres(f.nombres, f.hojas[f.activa].nombre)
	f.hojas = slices.Delete(f.hojas, f.activa, f.activa+1)
	f.activa = min(f.activa, len(f.hojas)-1)
	l.restaurar(f)
}

// Renombrar cambia el nombre de una hoja. Un nombre vacío o el mismo no hacen nada,
// igual que en la web (`if (!name || name === current.name) return`).
func (l *Libro) Renombrar(i int, nombre string) {
	nombre = strings.TrimSpace(nombre)
	if i < 0 || i >= len(l.f.hojas) || nombre == "" || nombre == l.f.hojas[i].nombre {
		return
	}
	l.anotar()
	f := l.copia()
	f.nombres = renombrarHojaEnNombres(f.nombres, f.hojas[i].nombre, nombre)
	f.hojas[i].nombre = nombre
	l.restaurar(f)
}

// Mover lleva la hoja `de` a la posición `a` (arrastrar la pestaña). La hoja activa
// sigue siendo la misma, esté donde esté.
func (l *Libro) Mover(de, a int) {
	if de == a || de < 0 || a < 0 || de >= len(l.f.hojas) || a >= len(l.f.hojas) {
		return
	}
	l.anotar()
	f := l.copia()
	activa := f.hojas[f.activa].nombre
	h := f.hojas[de]
	f.hojas = slices.Insert(slices.Delete(f.hojas, de, de+1), a, h)
	for i, x := range f.hojas {
		if x.nombre == activa {
			f.activa = i
		}
	}
	l.restaurar(f)
}

// ---- deshacer ----

// PuedeDeshacer y PuedeRehacer encienden los botones de la cinta.
func (l *Libro) PuedeDeshacer() bool { return len(l.pasado) > 0 }

// PuedeRehacer dice si hay algo deshecho que se pueda volver a hacer.
func (l *Libro) PuedeRehacer() bool { return len(l.futuro) > 0 }

// Deshacer vuelve un paso atrás. Devuelve false si no había nada que deshacer.
func (l *Libro) Deshacer() bool {
	if len(l.pasado) == 0 {
		return false
	}
	ant := l.pasado[len(l.pasado)-1]
	l.pasado = l.pasado[:len(l.pasado)-1]
	l.futuro = append(l.futuro, l.copia())
	l.restaurar(ant)
	return true
}

// Rehacer vuelve a hacer lo último deshecho.
func (l *Libro) Rehacer() bool {
	if len(l.futuro) == 0 {
		return false
	}
	sig := l.futuro[len(l.futuro)-1]
	l.futuro = l.futuro[:len(l.futuro)-1]
	l.pasado = append(l.pasado, l.copia())
	l.restaurar(sig)
	return true
}

// anotar guarda el estado ANTERIOR a un cambio y vacía el futuro: si tras deshacer se
// escribe algo nuevo, lo deshecho ya no lleva a ninguna parte.
func (l *Libro) anotar() {
	l.pasado = append(l.pasado, l.copia())
	if len(l.pasado) > TopeHistorial {
		l.pasado = l.pasado[len(l.pasado)-TopeHistorial:]
	}
	l.futuro = nil
}

// copia es una foto PROFUNDA del estado.
func (l *Libro) copia() foto {
	f := foto{hojas: make([]hojaLibro, len(l.f.hojas)), activa: l.f.activa, proteccion: l.f.proteccion, nombres: l.f.nombres}
	for i, h := range l.f.hojas {
		f.hojas[i] = h.clonar()
	}
	return f
}

// restaurar pone una foto y rehace el motor con ella. Es lo que hace la web en cada
// cambio de estructura (`buildEngine(model)`): las celdas con contenido son pocas y
// sale barato, y así ninguna fórmula se queda con el #REF! de una hoja que ya existe.
func (l *Libro) restaurar(f foto) {
	l.f = f
	l.rehacerMotor()
}

// rehacerMotor monta un motor nuevo con todas las celdas del libro, que es lo que la
// web hace en cada cambio del modelo (`buildEngine`). Aquí solo hace falta cuando cambia
// lo que el motor no ve celda a celda: las hojas o las tablas con sus cabeceras.
func (l *Libro) rehacerMotor() {
	l.version++
	l.motor = formula.New(formula.Options{Sheets: l.Hojas(), ResolveTable: l.resolverTabla})
	l.ocultas.por = nil // el motor es nuevo: no sabe nada de filas ocultas
	l.definirNombres()  // ANTES que las celdas, como la web: con el motor vacío no cuesta nada
	for _, h := range l.f.hojas {
		for k, html := range h.celdas {
			l.motor.SetCellAt(h.nombre, k.C, k.F, textoPlano(html))
		}
	}
}

// ---- resumen ----

// Resumen es lo que enseña la barra de hojas de un rango: sus valores pasados por
// resumenDe.
func (l *Libro) Resumen(c1, f1, c2, f2 int) (recuento int, suma, media float64, ok bool) {
	vals := make([]formula.Value, 0, (c2-c1+1)*(f2-f1+1))
	for f := f1; f <= f2; f++ {
		for c := c1; c <= c2; c++ {
			vals = append(vals, l.Valor(c, f))
		}
	}
	return resumenDe(vals)
}

// resumenDe es `selectionStats` de `sheets/format.ts`: cuántas celdas tienen algo
// —también el texto, que es el criterio de Excel—, cuánto suman las numéricas y su
// promedio. ok es false si no hay ningún número, que es cuando la web no enseña nada.
func resumenDe(vals []formula.Value) (recuento int, suma, media float64, ok bool) {
	nums := 0
	for _, v := range vals {
		if s, es := v.(string); es && s == "" {
			continue
		}
		recuento++
		if n, es := v.(float64); es && !math.IsNaN(n) && !math.IsInf(n, 0) {
			nums++
			suma += n
		}
	}
	if nums == 0 {
		return 0, 0, 0, false
	}
	// Seis decimales como en la web, y los números enormes tal cual: multiplicarlos por
	// 1e6 los sacaría del entero seguro. `Floor(x+0,5)` y no `Round`, que es como
	// redondea `Math.round` los negativos (−2,5 → −2).
	media = suma / float64(nums)
	if math.Abs(media) < 1e9 {
		media = math.Floor(media*1e6+0.5) / 1e6
	}
	return recuento, suma, media, true
}

// Ref es la referencia A1 de una celda por índices de base cero: (1, 2) es «B3».
func Ref(c, f int) string { return formula.NumToCol(c) + strconv.Itoa(f+1) }

// celdaCopia es la celda (c, f) de la hoja activa como la lleva el portapapeles.
func (l *Libro) celdaCopia(c, f int) celdaCopia {
	k := Celda{C: c, F: f}
	cc := celdaCopia{html: l.h().celdas[k]}
	if fm, ok := l.h().formatos[k]; ok {
		cc.num = &fm
	}
	if e, ok := l.h().estilos[k]; ok {
		cc.est = &e
	}
	return cc
}

// UltimaUsada es `lastUsedCell`: con `soloFila`, la última columna con algo de esa
// fila; sin él, la última columna y la última fila con algo de la hoja —por separado,
// como en Excel: puede no ser una celda con contenido—. false si no hay nada.
func (l *Libro) UltimaUsada(fila int, soloFila bool) (Celda, bool) {
	c, f := -1, -1
	for k := range l.h().celdas {
		if soloFila && k.F != fila {
			continue
		}
		c, f = max(c, k.C), max(f, k.F)
	}
	return Celda{C: c, F: f}, c >= 0
}

// ---- formato condicional ----

// Condiciones son las reglas de formato condicional de la hoja activa.
func (l *Libro) Condiciones() []ReglaCond { return l.h().condiciones }

// cambiarCondiciones sustituye las reglas de la hoja activa en un paso de deshacer.
func (l *Libro) cambiarCondiciones(reglas []ReglaCond) {
	l.anotar()
	l.version++
	l.h().condiciones = reglas
}

// AnadirCondicion es el comando `condadd`: la regla, saneada, al final (la de menor
// prioridad). false si no vale.
func (l *Libro) AnadirCondicion(crudo reglaCruda) bool {
	r, ok := crudo.sanear()
	if !ok {
		return false
	}
	l.cambiarCondiciones(append(slices.Clone(l.h().condiciones), r))
	return true
}

// QuitarCondicion es `conddel`.
func (l *Libro) QuitarCondicion(i int) {
	if i < 0 || i >= len(l.h().condiciones) {
		return
	}
	l.cambiarCondiciones(slices.Delete(slices.Clone(l.h().condiciones), i, i+1))
}

// MoverCondicion es `condmove`: cambia la prioridad de la regla i con la de al lado.
func (l *Libro) MoverCondicion(i, dir int) {
	j := i + 1
	if dir < 0 {
		j = i - 1
	}
	rs := l.h().condiciones
	if i < 0 || i >= len(rs) || j < 0 || j >= len(rs) {
		return
	}
	nuevas := slices.Clone(rs)
	nuevas[i], nuevas[j] = nuevas[j], nuevas[i]
	l.cambiarCondiciones(nuevas)
}

// celdaApoyo es `SCRATCH`: la celda de apoyo del motor donde se evalúan las reglas de
// fórmula, fuera de la rejilla.
const celdaApoyo = "ZZ1000"

// decoracionDe es lo que el formato condicional le pone a la celda (c, f) de la hoja
// activa, o nil. Se recalcula solo cuando el libro cambia.
func (l *Libro) decoracionDe(c, f int) *decoracion {
	d := &l.decorado
	if d.mapa == nil || d.version != l.version || d.activa != l.f.activa {
		d.version, d.activa, d.mapa = l.version, l.f.activa, l.decorar()
	}
	return d.mapa[Celda{C: c, F: f}]
}

func (l *Libro) decorar() map[Celda]*decoracion {
	reglas := l.h().condiciones
	if len(reglas) == 0 {
		return map[Celda]*decoracion{}
	}
	hoja := l.Hoja()
	usada := false
	evaluar := func(f string, dc, df int) bool {
		if !strings.HasPrefix(f, "=") {
			f = "=" + f
		}
		movida := formula.ShiftRefs(f, dc, df)
		if strings.HasPrefix(movida, "#") {
			return false // #REF!: se sale de la rejilla y no dispara
		}
		usada = true
		_ = l.motor.SetCell(hoja, celdaApoyo, movida)
		v, _ := l.motor.GetValue(hoja, celdaApoyo)
		switch x := v.(type) {
		case bool:
			return x
		case float64:
			return x != 0
		}
		return false
	}
	mapa := decorar(reglas, l.Valor, evaluar)
	if usada {
		_ = l.motor.SetCell(hoja, celdaApoyo, "")
	}
	return mapa
}
