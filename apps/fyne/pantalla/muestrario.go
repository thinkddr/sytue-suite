// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pantalla

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # El muestrario de `apps/fyne/ui`
//
// Una sección por familia de widgets, con sus variantes y estados uno al lado del otro,
// para revisar la paridad con la web SIN abrir una ventana: `muestrario_test.go` pinta
// cada sección en las 16 combinaciones del tema —2 colores × 4 modos × 2 formas, las
// del muestrario de la web— con el pintor de software (y con `SYTUE_CAPTURAS` deja los
// PNG con el mismo nombre que los de la web), y `cmd/demo` con `SYTUE_MUESTRARIO=1` o
// `SYTUE_SECCION` abre la misma en una ventana de verdad.
//
// El guardián de `muestrario_test.go` lee los `func Nuevo*` exportados de `ui` y exige
// que cada uno salga AQUÍ o en su lista de exclusiones con el motivo: un widget nuevo
// que nadie enseña es uno que nadie compara con la web.
//
// Los textos son de muestra y van tal cual, como los de `Demo`: esto no es una app de
// la suite, no se traduce ni lo ve nadie fuera de quien revisa.

// idiomaMuestrario es el de los textos de muestra, que van en español tal cual.
const idiomaMuestrario i18n.Locale = "es"

// AnchoMuestrario y AltoMuestrario son los del lienzo de cada sección. Fijos, como los de
// `Demo`, porque las capturas se comparan píxel a píxel. El ancho pasa de `ui.Estrecho`:
// por debajo, el menú y el diálogo son la hoja del teléfono y no el panel de escritorio.
const (
	AnchoMuestrario = 800
	AltoMuestrario  = 560
)

// seccion es una familia del muestrario. `construir` devuelve el contenido y, si la
// familia es de capa —el diálogo y el menú—, lo que la abre sobre el lienzo ya montado.
type seccion struct {
	clave, titulo string
	construir     func(v tema.Variante) (fyne.CanvasObject, func(fyne.Canvas))
}

// secciones va en el orden en que se recorren, el de la lista del plan.
var secciones = []seccion{
	{"botones", "Botones", seccionBotones},
	{"campos", "Campos y grupo de campo", seccionCampos},
	{"conmutador", "Conmutador, casilla y radio", seccionConmutador},
	{"selector", "Selector y segmentado", seccionSelector},
	{"eleccion", "Tarjeta de elección", seccionEleccion},
	{"pasos", "Pasos y progreso", seccionPasos},
	{"ajustes", "Marco, sección y resumen de ajustes", seccionAjustes},
	{"asistente", "Asistente", seccionAsistente},
	{"avisos", "Avisos", seccionAvisos},
	{"esqueleto", "Esqueleto y carga", seccionEsqueleto},
	{"pestanas", "Pestañas y migas", seccionPestanas},
	{"insignia", "Insignias y contador", seccionInsignia},
	{"avatar", "Avatar", seccionAvatar},
	{"vacio", "Estado vacío", seccionVacio},
	{"filas", "Tarjeta y filas", seccionFilas},
	{"dialogo", "Diálogo", seccionDialogo},
	{"menu", "Menú", seccionMenu},
	{"paleta", "Paleta de la suite", seccionPaleta},
}

// Secciones son las claves del muestrario, en orden: lo que acepta `SYTUE_SECCION`.
func Secciones() []string {
	claves := make([]string, len(secciones))
	for i, s := range secciones {
		claves[i] = s.clave
	}
	return claves
}

// Combinacion es una de las 2 × 4 × 2 del tema que recorre el muestrario: los mismos
// ejes que el de la web (`packages/ui/test/muestrario.tsx`), para ponerlos lado a lado.
// Modo vacío es «el del sistema», como el `system` de la web.
type Combinacion struct{ Color, Modo, Forma string }

// colores, modos y formas del recorrido. «El de cada app» no es UN color: se revisa en
// las capturas de cada app, no en el catálogo.
var (
	coloresMuestrario = []string{"petroleo", "cobalto"}
	modosMuestrario   = []string{"claro", "oscuro", "traslucido", ""}
	formasMuestrario  = []string{"marco", "carril"}
)

// modoWeb es el nombre del modo en la web (`ThemeMode`), el que lleva el de la captura.
var modoWeb = map[string]string{"claro": "light", "oscuro": "dark", "traslucido": "translucent", "": "system"}

// Combinaciones son las 16, en el orden del muestrario de la web.
func Combinaciones() []Combinacion {
	out := make([]Combinacion, 0, len(coloresMuestrario)*len(modosMuestrario)*len(formasMuestrario))
	for _, c := range coloresMuestrario {
		for _, m := range modosMuestrario {
			for _, f := range formasMuestrario {
				out = append(out, Combinacion{c, m, f})
			}
		}
	}
	return out
}

// Nombre es el de la web, `petroleo-dark-carril`: la captura de Fyne y la de la web de la
// misma combinación se llaman igual.
func (c Combinacion) Nombre() string { return c.Color + "-" + modoWeb[c.Modo] + "-" + c.Forma }

// Aplicar pone la combinación en el tema de la suite y devuelve la variante con la que
// pintar. Es global, como lo es en la suite: quien la aplica en un test la deshace.
func (c Combinacion) Aplicar() tema.Variante {
	tema.Elegir(c.Modo)
	tema.ElegirColor(c.Color)
	tema.ElegirForma(c.Forma)
	return tema.De(fyne.CurrentApp().Settings().ThemeVariant())
}

// Muestrario construye la sección `clave` sobre el papel del modo `v`. `abrir` no es nil
// en las familias de capa: hay que llamarlo con el lienzo cuando el contenido ya esté
// puesto y medido, porque el diálogo y el menú se cuelgan de él. `ok` es falso si la
// clave no es de ninguna sección.
func Muestrario(clave string, v tema.Variante) (contenido fyne.CanvasObject, abrir func(fyne.Canvas), ok bool) {
	for _, s := range secciones {
		if s.clave != clave {
			continue
		}
		cuerpo, abrir := s.construir(v)
		fondo := canvas.NewRectangle(tema.Color("bg", v))
		cabeza := container.New(ui.Columna{Hueco: tema.Espacio(1)},
			ui.NuevoRotulo(s.titulo, tema.TextoXL, ui.SemiNegrita, "text"),
			ui.NuevoRotulo("apps/fyne/ui · "+s.clave, tema.TextoXS, ui.Normal, "text-muted"))
		margen := tema.Espacio(6)
		pagina := container.New(ui.Margen{Izq: margen, Arr: margen, Der: margen, Abj: margen},
			container.New(ui.Columna{Hueco: tema.Espacio(5)}, cabeza, cuerpo))
		// A ancho fijo y no en una pila: el mínimo de una pila es el de sus hijos a su
		// ancho más estrecho, y con el texto repartido en 180 px «filas» pedía 746 de
		// alto. La ventana de `cmd/demo` crece hasta el mínimo de su contenido, así que
		// se abría más alta que el lienzo de las capturas. Así el alto es el de la página
		// a `AnchoMuestrario`, el que se pinta, y el test comprueba que cabe.
		return ancho(AnchoMuestrario, fondo, pagina), abrir, true
	}
	return nil, nil, false
}

// columna, fila y ancho son los tres layouts del muestrario, con el `gap` de la web.
func columna(hueco float32, hijos ...fyne.CanvasObject) *fyne.Container {
	return container.New(ui.Columna{Hueco: hueco}, hijos...)
}

func fila(hijos ...fyne.CanvasObject) *fyne.Container {
	return container.New(ui.Fila{Hueco: tema.Espacio(2)}, hijos...)
}

// ancho fija el ancho de lo que lleva, como el `w-80` de una muestra en la web: la
// columna del muestrario estira y un aviso de 672 no se parece al de la suite.
func ancho(w float32, hijos ...fyne.CanvasObject) *fyne.Container {
	return container.New(anchoFijo{w}, hijos...)
}

type anchoFijo struct{ w float32 }

func (a anchoFijo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(a.w, a.AltoCon(objs, a.w))
}

func (a anchoFijo) AltoCon(objs []fyne.CanvasObject, _ float32) float32 {
	var alto float32
	for _, o := range objs {
		if o.Visible() {
			alto = max(alto, ui.AltoA(o, a.w))
		}
	}
	return alto
}

func (a anchoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(fyne.NewSize(min(a.w, t.Width), t.Height))
	}
}

// rotulo es el nombre de una fila de muestras, en `text-xs` apagado.
func rotulo(s string) fyne.CanvasObject {
	return ui.NuevoRotulo(s, tema.TextoXS, ui.Medio, "text-muted")
}

func seccionBotones(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	variantes := fila(
		ui.NuevoBoton("Guardar cambios", nil),
		ui.NuevoBoton("Cancelar", nil).Con(ui.Secundario, ui.TallaMD),
		ui.NuevoBoton("Más opciones", nil).Con(ui.Fantasma, ui.TallaMD),
		ui.NuevoBoton("Eliminar", nil).Con(ui.Peligro, ui.TallaMD))
	tallas := fila(
		ui.NuevoBoton("XS", nil).Con(ui.Primario, ui.TallaXS),
		ui.NuevoBoton("SM", nil).Con(ui.Primario, ui.TallaSM),
		ui.NuevoBoton("MD", nil),
		ui.NuevoBoton("LG", nil).Con(ui.Primario, ui.TallaLG))
	inerte := ui.NuevoBoton("Inerte", nil)
	inerte.Inerte = true
	cargando := ui.NuevoBoton("Enviando…", nil)
	cargando.Cargando = true
	estados := fila(inerte, cargando)
	// La de la hoja de atajos: la tecla no trae juego por defecto (ver `ui.Tecla`).
	tecla := ui.NuevaTecla(ui.AtajoDelSistema("Ctrl K"), tema.TextoXS, ui.Normal)
	tecla.Fondo, tecla.Tinta, tecla.Borde = "bg-subtle", "text-muted", "border"
	tecla.RellenoX, tecla.Minimo = tema.Espacio(1.5), fyne.NewSize(24, 24)
	otros := fila(
		ui.NuevoBotonIcono(iconos.Ajustes, "Ajustes", nil),
		ui.NuevoBotonIcono(iconos.Papelera, "Mover a la papelera", nil),
		ui.NuevoEnlace("Ver el calendario", tema.TextoBase, ui.Medio, "primary-text", nil),
		tecla,
		ui.NuevoBotonFlotante("Redactar", iconos.Lapiz, nil))
	return columna(tema.Espacio(3),
		rotulo("Variantes"), variantes,
		rotulo("Tallas"), tallas,
		rotulo("Estados"), estados,
		rotulo("Icono, enlace, tecla y flotante"), otros), nil
}

func seccionCampos(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	nombre := ui.NuevaEntrada("Nombre y apellidos")
	nombre.PonerTexto("Marta López")
	correo := ui.NuevaEntrada("nombre@empresa.es")
	correo.PonerTexto("marta@")
	inerte := ui.NuevaEntrada("Sin permiso para editar")
	inerte.Inerte(true)
	notas := ui.NuevaAreaTexto("Escribe una nota para el equipo")
	izquierda := columna(tema.Espacio(4),
		ui.GrupoCampo("Nombre", true, nombre, "", ""),
		ui.GrupoCampo("Correo", true, correo, "", "Falta el dominio del correo."),
		ui.GrupoCampo("Cargo", false, inerte, "Lo cambia la administración.", ""))
	cuando := ui.NuevoCampoFechaHora(ui.CampoMD, time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC))
	derecha := columna(tema.Espacio(4),
		ui.GrupoCampo("Notas", false, notas, "Se envía a las personas invitadas.", ""),
		ui.GrupoCampo("Empieza", false, cuando.Objeto(), "", ""),
		ui.GrupoCampo("Volumen", false, ui.NuevoDeslizador(0, 100, 60, nil), "", ""),
		ui.NuevaPistaCampo("La contraseña tiene doce caracteres como poco.").ConTono(ui.AyudaAviso),
		ui.NuevaAyudaEnLinea("Los cambios se guardan solos y se ven al instante en los demás dispositivos.").ConTono(ui.AyudaInfo))
	return container.NewGridWithColumns(2, izquierda, derecha), nil
}

func seccionConmutador(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	encendido := ui.NuevoConmutador("Respuesta automática", nil)
	encendido.Encendido = true
	marcada := ui.NuevaCasilla("Todo el día", nil)
	marcada.Marcada = true
	parcial := ui.NuevaCasilla("Seleccionar todo", nil)
	parcial.Parcial = true
	radio := ui.NuevoRadio("Solo yo", nil)
	radio.Marcada = true
	return columna(tema.Espacio(3),
		rotulo("Conmutador"),
		encendido,
		ui.NuevoConmutador("Avisar por correo", nil),
		rotulo("Casilla"),
		marcada, parcial, ui.NuevaCasilla("Repetir cada semana", nil),
		rotulo("Radio"),
		radio, ui.NuevoRadio("Cualquiera con el enlace", nil)), nil
}

func seccionSelector(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	calendario := ui.NuevoSelector([]string{"Trabajo", "Personal", "Festivos"}, nil)
	pequeno := ui.NuevoSelector([]string{"Por fecha", "Por nombre"}, nil).Con(ui.CampoSM)
	inerte := ui.NuevoSelector([]string{"Europa/Madrid"}, nil)
	inerte.Inerte(true)
	vista := ui.NuevoSegmentado("Vista", []string{"Día", "Semana", "Mes"}, nil)
	vista.Elegida = 1
	return columna(tema.Espacio(4),
		ancho(280, ui.GrupoCampo("Calendario", false, calendario, "", "")),
		ancho(280, ui.GrupoCampo("Ordenar", false, pequeno, "Talla pequeña, la de las barras de herramientas.", "")),
		ancho(280, ui.GrupoCampo("Zona horaria", false, inerte, "", "")),
		rotulo("Segmentado"),
		fila(vista)), nil
}

func seccionEleccion(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	const w = 420
	elegida := ui.NuevaTarjetaEleccion("Solo ver", "Pueden abrir los archivos y descargarlos.", nil)
	elegida.Elegida, elegida.Icono, elegida.Ancho = true, iconos.Globo, w
	elegida.Insignia, elegida.TonoInsignia = "Lo más habitual", ui.InsigniaPrimaria
	editar := ui.NuevaTarjetaEleccion("Ver y añadir", "Además pueden subir archivos a la carpeta.", nil)
	editar.Icono, editar.Ancho, editar.Nota = iconos.Mas, w, "Lo que suban cuenta en tu espacio."
	inerte := ui.NuevaTarjetaEleccion("Editar", "Lo decide la administración de la organización.", nil)
	inerte.Inerte, inerte.Ancho = true, w
	densa := ui.NuevaTarjetaEleccion("Densa", "Con menos relleno, la de las listas largas.", nil)
	densa.Densa, densa.Ancho = true, w
	return columna(tema.Espacio(2),
		ancho(w, elegida), ancho(w, editar), ancho(w, inerte), ancho(w, densa)), nil
}

func seccionPasos(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	primero := ui.NuevosPasos("Cuenta", "Dominio", "Usuarios", "Listo")
	enMedio := ui.NuevosPasos("Cuenta", "Dominio", "Usuarios", "Listo")
	enMedio.Actual = 2
	return columna(tema.Espacio(4),
		rotulo("Primer paso"), primero,
		rotulo("Tercer paso"), enMedio,
		rotulo("Progreso"),
		ancho(420, ui.NuevaBarraProgreso("Subiendo 3 de 8 archivos", 0.4)),
		ancho(420, ui.NuevoMedidor("Espacio usado", 0.72, "72 %"))), nil
}

// seccionAjustes es la configuración guiada de una página de ajustes (§2.8): el marco
// con su título y su acción, una sección con tarjeta que guarda por su cuenta y, dentro,
// el resumen con su «Editar». El marco ocupa lo que le den, como la página que es.
func seccionAjustes(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	resumen := ui.NuevoResumen(nil,
		ui.FilaResumen{Clave: "Organización", Valor: "Acme, S. L."},
		ui.FilaResumen{Clave: "Dominio", Valor: "acme.example", Mono: true, AlEditar: func() {}})
	resumen.RotuloEditar = "Editar"
	seccion := ui.NuevaSeccionAjustes("Tu organización", "Lo que ven las personas a las que invitas.", resumen)
	seccion.Acciones = []fyne.CanvasObject{ui.NuevoBoton("Guardar", nil).Con(ui.Secundario, ui.TallaSM)}
	marco := ui.NuevoMarcoAjustes("Ajustes", "Cambia cómo se presenta tu organización en la suite.", seccion)
	return marco, nil
}

// seccionAsistente es el `Wizard` en su segundo paso: con progreso pulsable hacia atrás,
// «Atrás», «Cancelar» y la verja cerrada en «Continuar» hasta que se rellene el campo.
func seccionAsistente(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	dominio := ui.NuevaEntrada("acme.example")
	a := ui.NuevoAsistente(nil,
		ui.PasoAsistente{Titulo: "Cuenta"},
		ui.PasoAsistente{
			Titulo: "Dominio", Encabezado: "¿Con qué dominio trabajáis?",
			Descripcion: "Lo usamos para las direcciones de correo de tu equipo.",
			Cuerpo:      ui.GrupoCampo("Dominio", true, dominio, "", ""),
			PuedeSeguir: func() bool { return dominio.Texto() != "" },
		},
		ui.PasoAsistente{Titulo: "Usuarios"},
		ui.PasoAsistente{Titulo: "Listo", Terminal: true})
	a.Actual = 1
	a.AlCancelar = func() {}
	a.Rotulos = ui.RotulosAsistente{Atras: "Atrás", Siguiente: "Continuar", Cancelar: "Cancelar"}
	return ancho(560, a), nil
}

func seccionAvisos(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	const w = 360
	aviso := func(t ui.TonoAviso, titulo, descripcion string) fyne.CanvasObject {
		return ancho(w, ui.NuevoAviso(t, titulo, descripcion))
	}
	pila := ui.NuevaPilaAvisos()
	return columna(tema.Espacio(3),
		container.NewGridWithColumns(2,
			aviso(ui.AvisoInfo, "Sincronizando", "Quedan 12 archivos."),
			aviso(ui.AvisoExito, "Guardado", "Los cambios ya están en la nube."),
			aviso(ui.AvisoAdvertencia, "Sin conexión", "Se enviará al volver la red."),
			aviso(ui.AvisoPeligro, "No se ha podido enviar", "El servidor no responde.")),
		ui.NuevaFranjaSoloLectura("Este documento es de solo lectura: pide permiso para editarlo."),
		pila), nil
}

func seccionEsqueleto(v tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	filaEsqueleto := func() fyne.CanvasObject {
		return fila(ui.NuevoEsqueletoRedondo(36),
			columna(tema.Espacio(2), ui.NuevoEsqueleto(240, 12), ui.NuevoEsqueleto(160, 10)))
	}
	// Lo mismo que el muestrario web: avatar, dos líneas y un metadato, y la tesela.
	filasConAvatar := ui.NuevasFilasEsqueleto(2, ui.DelanteroAvatar, 2)
	filasConAvatar.Meta = []ui.MetaEsqueleto{{Ancho: 42}}
	filasConAvatar.Filete = true
	// La tesela, con cabecera de columnas (el `header` de la lista de Drive): rótulo a la
	// izquierda, dos columnas a la derecha y su filete, latiendo con las filas.
	filasConCabecera := ui.NuevasFilasEsqueleto(1, ui.DelanteroTesela, 1).ConCabecera(ui.CabeceraEsqueleto{
		Alto:      2*tema.Espacio(2.5) + 0.6*tema.Rem + tema.Borde,
		Filete:    true,
		Rotulo:    ui.MetaEsqueleto{Ancho: 56, Alto: 0.6 * tema.Rem},
		Meta:      []ui.MetaEsqueleto{{Ancho: 48, Alto: 0.6 * tema.Rem}, {Ancho: 72, Alto: 0.6 * tema.Rem}},
		HuecoMeta: 2 * tema.Espacio(4),
	})
	filasConCabecera.Meta = []ui.MetaEsqueleto{{Ancho: 44}, {Ancho: 76}}
	// Lo que no son filas: una semana de la rejilla del mes de Calendar —el círculo del
	// número, que no se estira con la celda— y dos líneas, una de ancho fijo y otra a una
	// fracción del ancho. Cada grupo late a una con un solo «Cargando».
	rejilla := ui.NuevosBloquesEsqueleto()
	celdas := make([]fyne.CanvasObject, 0, 7)
	for range 7 {
		celdas = append(celdas, container.New(ui.Margen{Izq: 4, Arr: 4, Der: 4, Abj: 4}, rejilla.Circulo(20)))
	}
	rejilla.Con(container.New(ui.Rejilla{Fijas: 7}, celdas...))
	lineas := ui.NuevosBloquesEsqueleto()
	lineas.Con(columna(tema.Espacio(2), lineas.BloqueFijo(2*tema.Rem, 12), lineas.BloqueFraccion(0.6, 12)))
	return columna(tema.Espacio(4),
		rotulo("Filas cargando"),
		filaEsqueleto(), filaEsqueleto(),
		rotulo("Bloque a fracción del ancho y bloques de una rejilla (BloquesEsqueleto)"),
		fila(ancho(420, columna(tema.Espacio(2), ui.NuevoEsqueletoFraccion(1, 12), ui.NuevoEsqueletoFraccion(0.6, 12))),
			ancho(220, rejilla), ancho(140, lineas)),
		rotulo("Filas de carga de una lista (SkeletonRows)"),
		fila(ancho(340, filasConAvatar), ancho(420, filasConCabecera)),
		rotulo("Rueda"),
		fila(ui.NuevaRueda(20, "primary", v), ui.NuevaRueda(28, "text-muted", v))), nil
}

func seccionPestanas(v tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	panel := func(s string) fyne.CanvasObject {
		return ui.NuevoParrafo(s, tema.TextoSM, ui.Normal, "text", v)
	}
	pestanas := ui.NuevasPestanas([]string{"Actividad", "Detalles", "Versiones"},
		[]fyne.CanvasObject{panel("Ana ha comentado hace 5 minutos."), panel("Creado el 3 de noviembre."), panel("Tres versiones guardadas.")})
	return columna(tema.Espacio(4),
		rotulo("Pestañas"), pestanas,
		rotulo("Pestaña suelta"), fila(ui.NuevaPestana("Compartido conmigo", nil)),
		rotulo("Migas"), ui.NuevasMigas("Mi unidad", []string{"Proyectos 2026", "Presupuestos"}, nil),
		fila(ui.NuevaMiga("Documentos", nil))), nil
}

func seccionInsignia(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	tonos := []struct {
		texto string
		tono  ui.Tono
	}{
		{"Neutra", ui.InsigniaNeutra},
		{"Primaria", ui.InsigniaPrimaria},
		{"Acento", ui.InsigniaAcento},
		{"Éxito", ui.InsigniaExito},
		{"Aviso", ui.InsigniaAviso},
		{"Peligro", ui.InsigniaPeligro},
		{"Info", ui.InsigniaInfo},
	}
	pildoras, cuadradas := fila(), fila()
	for _, t := range tonos {
		pildoras.Add(ui.NuevaInsignia(t.texto, t.tono))
		c := ui.NuevaInsignia(t.texto, t.tono)
		c.Cuadrada = true
		cuadradas.Add(c)
	}
	return columna(tema.Espacio(3),
		rotulo("Píldora"), pildoras,
		rotulo("Cuadrada"), cuadradas,
		rotulo("Contador"), fila(ui.NuevoContador("3"), ui.NuevoContador("27"), ui.NuevoContador("99+")),
		rotulo("Etiqueta"), fila(ui.NuevaEtiqueta("Trabajo"), ui.NuevaEtiqueta("Urgente"))), nil
}

func seccionAvatar(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	tallas := fila()
	for _, t := range []ui.TallaAvatar{ui.AvatarXS, ui.AvatarSM, ui.AvatarMD, ui.AvatarLG, ui.AvatarXL} {
		a := ui.NuevoAvatar("Marta López")
		a.Talla = t
		tallas.Add(a)
	}
	presencias := fila()
	for i, p := range []ui.Presencia{ui.EnLinea, ui.Ausente, ui.NoMolestar, ui.Desconectado} {
		a := ui.NuevoAvatar([]string{"Ana Ruiz", "Mario Gil", "Lucía Fernández", "Pablo Sánchez"}[i])
		a.Talla, a.Presencia = ui.AvatarLG, p
		presencias.Add(a)
	}
	propio := ui.NuevoAvatar("Irene Navarro")
	propio.Talla = ui.AvatarXL
	return columna(tema.Espacio(3),
		rotulo("Tallas"), tallas,
		rotulo("Presencia"), presencias,
		rotulo("Selector del avatar propio"), fila(ui.NuevoSelectorAvatar(propio))), nil
}

func seccionVacio(v tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	vacio := ui.NuevoVacio("No hay nada en esta carpeta", "Arrastra archivos aquí o súbelos con el botón.")
	vacio.Glifo = iconos.Trazo(iconos.Nube, ui.LadoGlifoVacio, "text-subtle", v)
	vacio.Accion = ui.NuevoBoton("Subir archivos", nil)
	fallo := ui.NuevoVacio("No se ha podido cargar", "Comprueba la conexión y vuelve a intentarlo.")
	fallo.Tono, fallo.Compacto = ui.VacioError, true
	fallo.Accion = ui.NuevoBoton("Reintentar", nil).Con(ui.Secundario, ui.TallaSM)
	return container.NewGridWithColumns(2, vacio, fallo), nil
}

func seccionFilas(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	presupuesto := ui.NuevaFilaDrive("Presupuesto 2027", "xlsx")
	presupuesto.Tamano, presupuesto.Fecha, presupuesto.Seleccionada = i18n.FormatBytes(idiomaMuestrario, 48*1024), "Ayer", true
	carpeta := ui.NuevaFilaDrive("Proyectos 2026", "")
	carpeta.Carpeta, carpeta.Tamano, carpeta.Fecha = true, "—", "3 nov."
	acta := ui.NuevaFilaDrive("Acta de la reunión", "docx")
	acta.Tamano, acta.Fecha = i18n.FormatBytes(idiomaMuestrario, 12*1024), "Hoy"
	explorador := columna(0, ui.NuevaCabeceraDrive("Nombre", "Tamaño", "Modificado"), carpeta, presupuesto, acta)
	registro := columna(tema.Espacio(2),
		ui.NuevaFilaCopia("Servidor", "imap.sytue.eu", nil),
		ui.NuevaFilaCopia("Puerto", "993", nil))
	return columna(tema.Espacio(4),
		ui.NuevaTarjeta("Archivos recientes", explorador),
		rotulo("Filas para copiar"), registro), nil
}

func seccionDialogo(v tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	// Lo de detrás del velo: unas filas, para que se vea que el velo tapa.
	fondo := columna(tema.Espacio(1))
	for _, s := range []string{"Plan del trimestre", "Notas de la reunión de equipo", "Preparar la demo del jueves"} {
		fondo.Add(ui.NuevaFilaLista(s, "Trabajo · hoy"))
	}
	titulo := ui.NuevaEntrada("")
	titulo.PonerTexto("Revisión del presupuesto")
	cuerpo := columna(tema.Espacio(4),
		ui.NuevoParrafo("El evento se añade a tu calendario y se avisa a las personas invitadas.", tema.TextoSM, ui.Normal, "text-muted", v),
		ui.GrupoCampo("Título", true, titulo, "", ""),
		ui.NuevaCasilla("Todo el día", nil))
	pie := container.New(ui.PieAcciones{},
		ui.NuevoBoton("Cancelar", nil).Con(ui.Fantasma, ui.TallaMD),
		ui.NuevoBoton("Guardar", nil))
	d := ui.NuevoDialogo("Nuevo evento", cuerpo).Con(ui.DialogoSM, pie)
	return fondo, d.Mostrar
}

func seccionMenu(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	disparador := ui.NuevoBoton("Acciones", nil).Con(ui.Secundario, ui.TallaMD)
	m := ui.NuevoMenu(
		ui.NuevoElementoMenu("Abrir", iconos.Ninguno, nil),
		ui.NuevoElementoMenu("Cambiar el nombre", iconos.Lapiz, nil),
		ui.NuevoElementoMenu("Descargar", iconos.Descargar, nil),
		ui.NuevoSeparadorMenu(),
		ui.NuevoElementoMenu("Mover a la papelera", iconos.Papelera, nil))
	return fila(disparador), func(fyne.Canvas) { m.Mostrar(disparador) }
}

func seccionPaleta(tema.Variante) (fyne.CanvasObject, func(fyne.Canvas)) {
	// Lo de detrás del velo, como en el diálogo.
	fondo := columna(tema.Espacio(1))
	for _, s := range []string{"Facturas de cliente", "Pedidos de compra", "Libro diario"} {
		fondo.Add(ui.NuevaFilaLista(s, "Contabilidad · hoy"))
	}
	// La paleta lee el catálogo común: es la de la barra de la suite.
	tr, _ := i18n.New(idiomaMuestrario, i18n.Common())
	p := ui.NuevaPaletaDeLaSuite(tr)
	p.PonerCapa("acciones", ui.CapaDePaleta{Comandos: []ui.ComandoDePaleta{
		{ID: "nuevo", Rotulo: "Nueva factura", Grupo: "Acciones", Icono: iconos.Mas},
		{ID: "subir", Rotulo: "Subir un archivo", Grupo: "Acciones", Icono: iconos.Subir},
	}})
	p.PonerCapa("areas", ui.CapaDePaleta{Comandos: []ui.ComandoDePaleta{
		{ID: "fv", Rotulo: "Facturas de cliente", Grupo: "Áreas", Pista: "Ventas"},
		{ID: "pc", Rotulo: "Pedidos de compra", Grupo: "Áreas", Pista: "Compras"},
		{ID: "ld", Rotulo: "Libro diario", Grupo: "Áreas", Pista: "Contabilidad"},
	}})
	return fondo, func(c fyne.Canvas) {
		p.EnLienzo(c)
		p.Abrir()
	}
}
