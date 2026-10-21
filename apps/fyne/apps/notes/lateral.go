// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Paridad web: apps/frontend/apps/notes/src/components/NotebookTree.tsx.

// LAS MEDIDAS DEL ÁRBOL DE CUADERNOS, y de dónde sale cada una.
//
// Barrido de producción del 2026-09-21 (corporate.sytue.eu, 1280×800, rem 14). El
// carril, sus tres destinos de arriba y el encabezado de sección son del catálogo
// (`ui.Lateral`, `ui.FilaLateral`, `ui.SeccionLateral`) y no se vuelven a declarar.
//
//	fila de cuaderno   30,5 × 231   MEDIDO
//	fila de nota       21 × 231     MEDIDO (dentro de un `ul` con `py-0.5`)
//	botón de acción    20 × 20      MEDIDO — `p-1` sobre un icono de 13
//	entre filas        1,75         hoja, `gap-0.5`
//
// # LOS 0,75 QUE FALTABAN, cerrados
//
// El 30,5 no sale del texto: `py-1.5` (10,5) más la caja de línea de `text-base`
// (19,25) dan **29,75**. Los 0,75 que faltan son **los dos botones de acción de la
// derecha** —«nuevo cuaderno dentro» y «ajustes»—, que son `p-1` alrededor de un icono
// de 13 y por tanto miden **20**, más que la línea de texto. Están en el layout
// siempre, aunque a `opacity-0`, que es lo que hace que la fila no cambie de alto al
// pasar el ratón por encima.
//
// Medido en el volcado: `button[aria-label="Nuevo cuaderno dentro"]` sale 20×20 con
// `padding: 3.5px`, y el `li` del árbol da `alto.moda = 30,5`. 10,5 + 20 = 30,5.
//
// # Y los «5 px» de la fila de nota NO EXISTEN
//
// El encargo decía que la fila de nota medía 26 y que faltaban 5 px sobre los 21 de su
// marcado. En el volcado de producción esa fila mide **21**: el `li` del cuaderno con
// notas da 55, de los que 30,5 son la fila del cuaderno, 24,5 el `ul` de sus notas, y
// de esos 24,5 son `py-0.5` (3,5) más la fila, 21. Que es exactamente `py-1` (7) más la
// caja de línea de `text-xs` (14). No hay nada que explicar porque no falta nada.
//
// Lo que sí hacía falta era que `ui.RamaLateral` supiera dos cosas: que la hoja va en
// `text-xs` y que su glifo es un `size={12}` SUELTO y no una caja de 17,5 —con la caja
// la fila salía 24,5—. Son los dos campos que se le han añadido.
const (
	// RellenoCuaderno es `py-1.5`.
	RellenoCuaderno = 1.5 * 0.25 * tema.Rem
	// RellenoNota es `py-1`.
	RellenoNota = 1 * 0.25 * tema.Rem
	// SangriaBase y SangriaNivel son `paddingLeft: 0,5 + (nivel−1) × 0,85 rem`.
	SangriaBase  = 0.5 * tema.Rem
	SangriaNivel = 0.85 * tema.Rem
	// SangriaNotaBase es el `0,75 + nivel × 0,85 rem` de la nota que cuelga.
	SangriaNotaBase = 0.75 * tema.Rem
	// GlifoArbol es el `size={13}` de los iconos de una fila de cuaderno, y
	// GlifoNota el `size={12}` de la nota que cuelga.
	GlifoArbol = 13
	GlifoNota  = 12
	// LadoAccionArbol es `p-1` sobre un icono de 13: 20. Es lo que manda en el alto de
	// la fila y el número que cierra los 30,5. Lo miden el candado y, en el LAYOUT, los
	// dos botones de acción (ver DesbordeAccion).
	LadoAccionArbol = GlifoArbol + 2*0.25*tema.Rem
	// DesbordeAccion es el `-my-[2px]` de los dos botones de acción de un cuaderno: son
	// la acción del carril, de 24 (`ui.LadoAccionLateral`), y devuelven los 4 px que
	// crecen para que la fila siga midiendo 30,5 (web, PR #834).
	DesbordeAccion = 2
	// HuecoAcciones es el `gap-0.5` entre los dos botones de acción.
	HuecoAcciones = 0.5 * 0.25 * tema.Rem
	// RellenoNotas es el `py-0.5` del `ul` que envuelve las notas de un cuaderno.
	RellenoNotas = 0.5 * 0.25 * tema.Rem
)

// Lateral es el menú de Notepads: los tres destinos de arriba y el árbol de cuadernos.
//
// Es estado puro: quien lo monta rellena los campos y llama a `Objeto`. Nada de aquí
// habla con el servicio, que es lo que hace que los guardianes de píxel no necesiten
// red.
type Lateral struct {
	tr *i18n.Translator

	// Cuadernos son todos, con su `Padre`; el árbol se arma aquí.
	Cuadernos []Cuaderno
	// Activo es el ID del cuaderno abierto.
	Activo string
	// Notas son las del cuaderno activo, que cuelgan de su fila.
	Notas []Nota
	// NotaActiva es la seleccionada dentro del árbol.
	NotaActiva string
	// Vista es cuál de los tres destinos de arriba está marcado.
	Vista Vista
	// Bloqueado dice que el cuaderno activo está cifrado y todavía cerrado.
	Bloqueado bool
	// Cerrados son las ramas plegadas, por ID.
	Cerrados map[string]bool
	// EsMio dice si un dueño es quien mira: solo sus diarios salen del árbol.
	EsMio func(string) bool
	// Fase es la del árbol: mientras carga no se pinta nada y con error se escribe el
	// aviso, igual que hace la web con su `cargandoArbol` / `errorArbol`.
	Fase Fase

	AlHoy                 func()
	AlDia                 func(string)
	AlIrA                 func(Vista)
	AlEtiquetas           func()
	AlAbrir               func(Cuaderno)
	AlAjustes             func(Cuaderno)
	AlNuevoDentro         func(Cuaderno)
	AlNuevoCuaderno       func()
	AlImportar            func()
	AlServiciosVinculados func()
	AlDesbloquear         func(Cuaderno)
	AlPlegar              func(string)
	AlAbrirNota           func(string)
}

// NuevoLateral crea el menú vacío.
func NuevoLateral(tr *i18n.Translator) *Lateral {
	return &Lateral{tr: tr, Cerrados: map[string]bool{}}
}

// Objeto arma el carril entero.
func (l *Lateral) Objeto(v tema.Variante) fyne.CanvasObject {
	destino := func(vista Vista, d, clave string) *ui.FilaLateral {
		f := ui.NuevaFilaLateral(l.tr.T(clave, nil), Glifo(d, ui.GlifoMenu, "sidebar-fg", v),
			func() { llamar1(l.AlIrA, vista) })
		f.Activa = l.Vista == vista
		return f
	}
	hoy := ui.NuevaFilaLateral(l.tr.T("daily.today", nil), Icono(iconos.Calendario, ui.GlifoMenu, "sidebar-fg", v), func() { llamar0(l.AlHoy) })
	for _, c := range l.Cuadernos {
		if c.ID == l.Activo && c.Crudo.DailyDate == time.Now().Format(time.DateOnly) {
			hoy.Activa = l.Vista == VistaCuaderno
		}
	}
	// Lo de cada día arriba (hoy, tareas, fijadas, buscar), los cuadernos en medio y lo
	// que se visita de tarde en tarde al final y sin rótulo, como en la web.
	arriba := ui.NuevaSeccionLateral("", hoy,
		destinoIcono(l, v, iconos.Tic, VistaTareas, "tasks.title"),
		destino(VistaFijadas, FijarD, "focus.pinned"),
		destinoIcono(l, v, iconos.Buscar, VistaBusqueda, "sidebar.search"),
	)
	abajo := ui.NuevaSeccionLateral("",
		// «Compartido conmigo», del catálogo COMÚN (el mismo rótulo que en Photos).
		destinoIcono(l, v, iconos.Usuarios, VistaCompartidas, "sharedWithMe.title"),
		// «Etiquetas» abre un DIÁLOGO y no cambia de vista: no se marca nunca como
		// activa, que es lo que hace la web (no le pasa `active`).
		ui.NuevaFilaLateral(l.tr.T("sidebar.tags", nil),
			Icono(iconos.Etiqueta, ui.GlifoMenu, "sidebar-fg", v),
			func() { llamar0(l.AlEtiquetas) }),
		destino(VistaArchivo, ArchivoD, "archive.title"),
		destino(VistaPapelera, PapeleraD, "sidebar.trash"),
	)

	cuadernos := ui.NuevaSeccionLateral(l.tr.T("sidebar.notebooks", nil), l.ramas(v)...)
	cuadernos.Acciones = []fyne.CanvasObject{
		ui.NuevaAccionLateralCon(l.tr.T("sidebar.newNotebook", nil),
			iconos.Trazo(iconos.Mas, ui.CajaIcono, "sidebar-fg-muted", v),
			func() { llamar0(l.AlNuevoCuaderno) }),
		// Importar cuelga de «nuevo cuaderno» porque es lo mismo con otro origen: en la
		// web es un `<label>` con un `<input type=file>` dentro; aquí, el diálogo de
		// fichero del sistema, que lo abre la pantalla.
		ui.NuevaAccionLateralCon(l.tr.T("import.label", nil),
			Dibujo(SubirD, ui.CajaIcono, "sidebar-fg-muted", v),
			func() { llamar0(l.AlImportar) }),
	}

	cuerpo := container.New(ui.Columna{}, arriba, cuadernos, abajo)
	if l.AlServiciosVinculados != nil {
		cuerpo.Add(ui.NuevaFilaLateral(l.tr.T("linkedServices.connected", nil), Icono(iconos.Globo, ui.GlifoMenu, "sidebar-fg", v), l.AlServiciosVinculados))
	}
	return ui.NuevoLateral(l.tr.T("ui.mainNav", nil), cuerpo)
}

// destinoIcono es un destino cuyo glifo SÍ está en el catálogo compartido.
func destinoIcono(l *Lateral, v tema.Variante, i iconos.Icono, vista Vista, clave string) *ui.FilaLateral {
	f := ui.NuevaFilaLateral(l.tr.T(clave, nil), Icono(i, ui.GlifoMenu, "sidebar-fg", v),
		func() { llamar1(l.AlIrA, vista) })
	f.Activa = l.Vista == vista
	return f
}

// ramas arma el árbol de cuadernos, y debajo de cada uno abierto sus notas.
func (l *Lateral) ramas(v tema.Variante) []fyne.CanvasObject {
	switch l.Fase {
	case Cargando:
		// La web no pinta nada mientras carga el árbol (`cargandoArbol ? null : …`).
		return nil
	case Fallo, SinSesion, Prohibida, SinConexion:
		return []fyne.CanvasObject{ui.Texto(l.tr.T("error.loadFailed", nil),
			tema.TextoBase, ui.Normal, tema.Color("sidebar-fg-muted", v))}
	case Exito, Vacia:
	}
	var filas []fyne.CanvasObject
	var bajar func(padre string, nivel int)
	arbol := cuadernosDelArbol(l.Cuadernos, l.EsMio)
	bajar = func(padre string, nivel int) {
		for _, c := range arbol {
			if c.Padre != padre {
				continue
			}
			filas = append(filas, l.fila(c, nivel, v))
			if l.Cerrados[c.ID] {
				continue
			}
			if c.ID == l.Activo {
				for _, n := range l.Notas {
					filas = append(filas, l.filaNota(n, nivel, v))
				}
			}
			bajar(c.ID, nivel+1)
		}
	}
	bajar("", 1)
	return filas
}

// fila es la de un cuaderno: sangría por nivel, marca de color o emoji, contador y los
// dos botones de acción que cierran los 30,5.
func (l *Lateral) fila(c Cuaderno, nivel int, v tema.Variante) *ui.FilaLateral {
	f := ui.NuevaFilaLateral(c.Titulo, nil, func() { llamar1(l.AlAbrir, c) })
	var caja float32
	if c.Emoji != "" {
		// `w-5 text-center`: el emoji tiene su caja de 17,5.
		f.Icono, caja = l.marca(c, v), AnchoEmojiMover
	} else {
		// El punto de color: `h-2.5 w-2.5 rounded-full` con el color del CUADERNO,
		// que es un dato y no un token. Suelto, sin caja: ocupa sus 8,75.
		f.Punto, caja = hexO(c.Color, tema.Color("sidebar-fg-muted", v)), ui.PuntoEtiqueta
	}
	f.Activa = c.ID == l.Activo
	cola, acciones := l.cola(c, v)
	f.Rama = &ui.RamaLateral{
		Sangria:   SangriaBase + float32(nivel-1)*SangriaNivel,
		RellenoY:  RellenoCuaderno,
		CajaIcono: caja,
		Cola:      cola,
		// Solo los BOTONES se esconden. El contador, el distintivo de visibilidad y el
		// candado son información y la web los pinta siempre.
		ColaAlPasar: acciones,
	}
	// El chevron, o el hueco de 13 donde iría (con su `gap-1.5`): así los nombres de un
	// mismo nivel caen en la misma vertical tengan hijos o no.
	f.Rama.LadoPliegue = GlifoArbol
	if b := l.pliegue(c, f.Activa); b != nil {
		f.Rama.Pliegue = b
	}
	return f
}

// pliegue es el chevron de un cuaderno con hijos: `-m-1 rounded p-1` alrededor del de 13,
// que se pinta 21 y ocupa los 13 del hueco. Abierto apunta abajo (`rotate-90`). Sin
// hijos, nil: el hueco se queda vacío.
func (l *Lateral) pliegue(c Cuaderno, activa bool) fyne.CanvasObject {
	tiene := false
	for _, x := range l.Cuadernos {
		if x.Padre == c.ID {
			tiene = true
			break
		}
	}
	if !tiene {
		return nil
	}
	cerrado := l.Cerrados[c.ID]
	icono, rotulo := iconos.ChevronAbajo, l.tr.T("note.collapse", nil)
	if cerrado {
		icono, rotulo = iconos.ChevronDerecha, l.tr.T("note.expand", nil)
	}
	b := accionArbol(icono, rotulo, func() { llamar1(l.AlPlegar, c.ID) })
	b.Lado, b.Radio = GlifoArbol+2*0.25*tema.Rem, 0.25*tema.Rem
	// El chevron hereda el color del texto de la fila y no lleva fondo al pasar.
	b.Trazo, b.FondoEncima, b.FondoActivo = "sidebar-fg", "", ""
	if activa {
		b.Trazo = "sidebar-active-fg"
	}
	return b
}

// marca es el emoji del cuaderno. Sin emoji va el punto de su color (`FilaLateral.Punto`):
// la web pinta uno de los dos y nunca los dos.
func (l *Lateral) marca(c Cuaderno, v tema.Variante) fyne.CanvasObject {
	// ponytail: el emoji va como texto con la fuente de la suite, que es Figtree y no
	// trae emoji en color — sale el glifo de reserva del sistema, o una caja. Se arregla
	// embebiendo una fuente de emoji, que son 10 MB por binario y toca a las seis apps
	// que pintan uno; hoy ninguna lo hace.
	t := ui.Texto(c.Emoji, tema.TextoBase, ui.Normal, tema.Color("sidebar-fg", v))
	t.Alignment = fyne.TextAlignCenter // `text-center` en su caja de 17,5
	return t
}

// cola es lo que va pegado al borde derecho de la fila: el distintivo de visibilidad o
// el candado, el contador, y los dos botones de acción.
//
// Va junto en un solo objeto porque es lo que la web tiene (`flex shrink-0`) y porque
// su ALTO es el que manda en la fila: los botones son 20 y la línea de texto 19,25.
func (l *Lateral) cola(c Cuaderno, v tema.Variante) (cola, acciones fyne.CanvasObject) {
	var piezas []fyne.CanvasObject

	switch {
	case c.Cifrado && l.Bloqueado && c.ID == l.Activo:
		// Cerrado: el candado del catálogo compartido, en el ámbar del CROMO. El de papel
		// (`warning-text`) daba 2,07:1 sobre la tinta —1,62 en la fila activa—; este,
		// 7,24:1 (web, PR #834).
		b := accionArbol(iconos.Candado, l.tr.T("crypto.unlock", nil),
			func() { llamar1(l.AlDesbloquear, c) })
		b.Trazo = "on-chrome-warning"
		piezas = append(piezas, b)
	case c.Cifrado:
		// Abierto: el candado ABIERTO, que es un glifo de esta app.
		piezas = append(piezas, accionArbolPropia(AbiertoD, l.tr.T("crypto.lockNow", nil),
			"sidebar-fg-muted", func() { llamar1(l.AlDesbloquear, c) }))
	default:
		if d := distintivo(c.Publico); d != "" {
			piezas = append(piezas, Glifo(d, GlifoArbol, "sidebar-fg-muted", v))
		}
	}

	// El contador es un `text-sm` y NO el `<Badge count>` de 18 del menú de Mail:
	// medido, 11,38/15,75 en peso 500.
	if n := l.cuantas(c); n > 0 {
		piezas = append(piezas, ui.Texto(strconv.Itoa(n), tema.TextoSM, ui.Medio,
			tema.Color("sidebar-fg-muted", v)))
	}

	// LOS DOS BOTONES QUE CIERRAN LOS 30,5. Cuentan SIEMPRE para la medida aunque solo
	// se vean al pasar por encima (`ColaAlPasar`): si entraran y salieran del layout,
	// la fila pegaría un salto de 0,75 cada vez que el ratón la cruza.
	botones := container.New(desborde{DesbordeAccion}, container.New(ui.Fila{Hueco: HuecoAcciones},
		accionCuaderno(iconos.Mas, l.tr.T("notebook.newChild", nil),
			func() { llamar1(l.AlNuevoDentro, c) }),
		accionCuaderno(iconos.Ajustes, l.tr.T("notebook.settings", nil),
			func() { llamar1(l.AlAjustes, c) }),
	))
	botones.Hide() // en reposo no se ven; la fila los enseña al pasar por encima
	piezas = append(piezas, botones)
	return container.New(colaArbol{hueco: HuecoAcciones}, piezas...), botones
}

// colaArbol es la fila de la derecha de una rama: mide contando TAMBIÉN a los hijos
// ocultos y coloca solo a los visibles.
//
// Es propia y no `ui.Fila` por una razón medida: `ui.Fila` se salta lo invisible en las
// dos cosas, así que al ocultar los dos botones la fila bajaba de 30,5 a 29,75 y
// pegaba un salto cada vez que el ratón la cruzaba. En la web no pasa porque
// `opacity-0` sigue ocupando sitio.
type colaArbol struct{ hueco float32 }

var _ fyne.Layout = colaArbol{}

// MinSize cuenta a TODOS, visibles o no: es lo que hace `opacity-0`.
func (c colaArbol) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		m := o.MinSize()
		if i > 0 {
			ancho += c.hueco
		}
		ancho += m.Width
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(ancho, alto)
}

// Layout coloca a los visibles y reserva el hueco de los que no lo están, para que el
// contador no se mueva al aparecer los botones.
func (c colaArbol) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	var x float32
	for _, o := range objs {
		m := o.MinSize()
		if o.Visible() {
			o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
			o.Resize(m)
		}
		x += m.Width + c.hueco
	}
}

// cuantas son las notas de un cuaderno.
//
// EN EL ABIERTO SE CUENTAN LAS QUE HAY, no las que dijo el servidor al cargar. Es la
// misma decisión que la web tomó el 2026-09-11 tras medirlo con siete notas: el árbol
// decía 1 y la lista de al lado, 7. `note_count` era un número aparte que había que
// acordarse de subir al crear, bajar al borrar y otra vez al deshacer cada uno.
func (l *Lateral) cuantas(c Cuaderno) int {
	if c.ID == l.Activo && l.Fase == Exito {
		return len(l.Notas)
	}
	return c.Notas
}

// distintivo es el glifo de visibilidad. `personal` no lleva: es el caso normal, y es
// el mapa `MARCA` de NotebookTree.tsx.
func distintivo(visibilidad string) string {
	switch visibilidad {
	case "private":
		return CuadernoD
	case "organization", "public":
		return EnlaceD
	}
	return ""
}

// filaNota es una nota colgada de su cuaderno: `py-1 text-xs`, glifo de 12 suelto.
func (l *Lateral) filaNota(n Nota, nivel int, v tema.Variante) *ui.FilaLateral {
	// `tinta` y no `token`: gosec lee una variable llamada `token` con una cadena
	// literal como una credencial incrustada.
	tinta := "sidebar-fg-muted"
	d := GlifoPorNombre(n.Icono)
	if d != "" {
		tinta = "primary-text"
	} else if d = GlifoDeTipo(n.Tipo); d == "" {
		d = NotaD
	}
	f := ui.NuevaFilaLateral(n.Rotulo(l.tr), Glifo(d, GlifoNota, tinta, v),
		func() { llamar1(l.AlAbrirNota, n.ID) })
	f.Activa = n.ID == l.NotaActiva
	f.Tachada = n.Terminada() // `terminada(n) && 'line-through'` de NotebookTree.tsx
	f.Rama = &ui.RamaLateral{
		Sangria:   SangriaNotaBase + float32(nivel)*SangriaNivel,
		RellenoY:  RellenoNota,
		CajaIcono: GlifoNota,
		Hoja:      true,
	}
	if n.Fijada {
		f.Rama.Cola = Glifo(FijarD, 10, "primary-text", v)
	}
	return f
}

// accionArbol es el botón de solo icono de una fila del árbol: `p-1` alrededor de un
// glifo de 13, o sea 20 de lado.
//
// Es `ui.BotonIcono` y NO `ui.Boton` con `SoloIcono`, y la diferencia se midió: un
// `Boton` toma su alto de la TALLA (31,5 la md, 24 la xs) y no del ancho que se le
// pida, así que la fila salía 42 donde producción mide 30,5. `BotonIcono` es el que
// tiene el lado libre, que es justo lo que hace falta aquí.
//
// Tampoco es `ui.AccionLateral`, que mide 21 (`h-6 w-6`): esa es la del ENCABEZADO de
// sección, y en el volcado se distinguen —«Nuevo cuaderno» sale 21×21 y «Nuevo cuaderno
// dentro», 20×20—.
//
// El anillo es el del CROMO: el azul de papel medía 2,67:1 contra el cuaderno activo, por
// debajo del 3:1 de WCAG 1.4.11 (web, PR #834).
func accionArbol(i iconos.Icono, rotulo string, alPulsar func()) *ui.BotonIcono {
	b := ui.NuevoBotonIcono(i, rotulo, alPulsar)
	b.Lado, b.TamIcono = LadoAccionArbol, GlifoArbol
	b.Trazo, b.TrazoEncima = "sidebar-fg", ""
	b.FondoEncima, b.FondoActivo = "sidebar-hover-bg", "sidebar-active-bg"
	b.TokenAro = "chrome-focus"
	return b
}

// accionCuaderno es «nuevo cuaderno dentro» y «ajustes»: la acción del carril
// (SIDEBAR_ACTION_CLASS) con el glifo de 13 del árbol. 24 de lado, `rounded-md`, glifo
// en `sidebar-fg-muted` que pasa a `sidebar-fg` con el velo de `sidebar-hover`.
func accionCuaderno(i iconos.Icono, rotulo string, alPulsar func()) *ui.BotonIcono {
	b := accionArbol(i, rotulo, alPulsar)
	b.Lado, b.Radio = ui.LadoAccionLateral, tema.Forma("radius-control")
	b.Trazo, b.TrazoEncima = "sidebar-fg-muted", "sidebar-fg"
	b.FondoActivo = ""
	return b
}

// desborde es un margen vertical NEGATIVO (`-my-[d]`): el hijo se pinta con su alto
// entero y al padre le dice que mide 2d menos.
type desborde struct{ d float32 }

func (b desborde) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width, m.Height-2*b.d)
}

func (b desborde) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, -b.d))
	objs[0].Resize(fyne.NewSize(t.Width, t.Height+2*b.d))
}

// accionArbolPropia es accionArbol con un glifo de esta app.
func accionArbolPropia(d, rotulo, trazo string, alPulsar func()) *ui.BotonIcono {
	b := accionArbol(iconos.Ninguno, rotulo, alPulsar)
	// El color de aquí es solo el de partida: `BotonIcono.dibujo` lo tiñe con el del
	// estado en cada refresco.
	b.Glifo, b.Trazo = iconos.TrazoDeRuta(d, ui.LadoIconoSM, tema.Color(trazo, tema.Claro)), trazo
	return b
}

// llamar0 y llamar1 dejan que una acción sin conectar no sea un pánico: la pantalla se
// monta antes de engancharla, y los guardianes la montan sin enganchar nada.
func llamar0(f func()) {
	if f != nil {
		f()
	}
}

func llamar1[T any](f func(T), v T) {
	if f != nil {
		f(v)
	}
}
