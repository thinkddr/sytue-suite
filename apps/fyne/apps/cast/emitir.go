// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/cast/src/pages/studio.astro

package cast

import (
	"image"
	"image/color"
	"image/draw"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// FaseEmision es el momento de «Emitir desde aquí» (el `estado` de EmitirAqui.tsx).
type FaseEmision uint8

// Las cuatro.
const (
	// EmisionPreparando: se eligen las fuentes y se mira la vista previa.
	EmisionPreparando FaseEmision = iota
	// EmisionConectando: se programa (si hace falta) y se entra en la sala.
	EmisionConectando
	// EmisionEnDirecto: se está emitiendo.
	EmisionEnDirecto
	// EmisionFallida: no se pudo empezar, o la sala se cortó.
	EmisionFallida
)

// Emision es el panel «Emitir desde aquí»: el directo que sale de esta app con la
// cámara, la pantalla y el micrófono.
type Emision struct {
	// Elegido es un directo ya programado para emitir desde aquí; nil, se programa uno
	// al empezar con Titulo y Visibilidad.
	Elegido     *api.CastVideo
	Titulo      *ui.Entrada
	Visibilidad string
	// Previa es el cuadro de lo que sale; vive en App (una Previa).
	Previa fyne.CanvasObject
	Fase   FaseEmision
	// Camara, Pantalla y Micro son las fuentes elegidas (antes de empezar) o
	// publicadas (en directo). Denegado: el sistema no dio la cámara o la pantalla.
	Camara, Pantalla, Micro bool
	Denegado                bool

	AlVisibilidad                 func(string)
	AlCamara, AlPantalla, AlMicro func()
	AlEmpezar, AlTerminar         func()
	// AlCambiarCamara pasa de la delantera a la trasera en el teléfono; nil, no hay
	// otra cámara a la que pasar.
	AlCambiarCamara func()
}

// Listo dice si se puede empezar: hay fuente de vídeo y título (o directo elegido).
func (e Emision) Listo() bool {
	hayTitulo := e.Elegido != nil || (e.Titulo != nil && strings.TrimSpace(e.Titulo.Texto()) != "")
	return (e.Camara || e.Pantalla) && hayTitulo
}

// Emitir son los datos del estudio.
type Emitir struct {
	// Clave es la de OBS; nil mientras llega. ClaveVisible enseña la clave en claro.
	Clave        *api.CastStreamKey
	ClaveVisible bool
	ErrorClave   string
	Regenerando  bool

	Emision Emision

	// Titulo y Descripcion son los campos del directo que se programa; viven en App.
	Titulo      *ui.Entrada
	Descripcion *ui.AreaTexto
	Visibilidad string
	// Fuente es "rtmp" (OBS) o "webrtc" (desde aquí).
	Fuente      string
	Programando bool

	// Directos son los míos; nil mientras llegan (SinConexion: no llegarán).
	Directos    []api.CastVideo
	SinConexion bool

	AlVerClave, AlCopiarServidor, AlCopiarClave, AlRegenerar func()
	AlVisibilidad, AlFuente                                  func(string)
	AlProgramar                                              func()
	AlEmitirAqui, AlAbrir, AlTerminar                        func(api.CastVideo)
}

// PantallaEmitir monta «Emitir» como StudioApp.tsx: a la izquierda, emitir desde aquí y
// la clave de OBS; a la derecha, programar un directo y la lista de mis directos. Por
// debajo de `lg`, todo en una columna en ese orden.
func PantallaEmitir(v Vista, d Emitir) *fyne.Container {
	bloques := []ui.Bloque{{Obj: Cabecera(v.Traduce("studio.title"), ""), Margen: HuecoSeccion}}
	if cuerpo := CuerpoDeEstado(v, nil); cuerpo != nil {
		return Pagina(v, append(bloques, ui.Bloque{Obj: cuerpo})...)
	}
	aqui := Recuadro(v, v.Traduce("studio.browser.title"), panelEmision(v, d.Emision))
	obs := Recuadro(v, v.Traduce("studio.obs.title"), panelOBS(v, d))
	programar := Recuadro(v, v.Traduce("studio.schedule.title"), panelProgramar(v, d))
	directos := tarjetaDirectos(v, d)
	hueco := tema.Espacio(6)
	if v.Ventana < ui.CorteLG {
		return Pagina(v, append(bloques,
			ui.Bloque{Obj: aqui, Margen: hueco}, ui.Bloque{Obj: obs, Margen: hueco},
			ui.Bloque{Obj: programar, Margen: hueco}, ui.Bloque{Obj: directos})...)
	}
	izquierda := ui.PilaBloques(ui.Bloque{Obj: aqui, Margen: hueco}, ui.Bloque{Obj: obs})
	derecha := ui.PilaBloques(ui.Bloque{Obj: programar, Margen: hueco}, ui.Bloque{Obj: directos})
	return Pagina(v, append(bloques, ui.Bloque{Obj: container.New(&mitades{}, izquierda, derecha)})...)
}

// ---- Emitir desde aquí ----

// panelEmision es EmitirAqui.tsx con su cabecera: el título y la visibilidad del
// directo (o el título del elegido), la vista previa, los avisos, las tres fuentes y
// «Emitir ahora» o «Terminar el directo».
func panelEmision(v Vista, e Emision) fyne.CanvasObject {
	var bloques []ui.Bloque
	quieto := e.Fase == EmisionPreparando || e.Fase == EmisionFallida
	switch {
	case e.Elegido != nil:
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, e.Elegido.Title, tema.TextoSM, ui.SemiNegrita, "text"), Margen: tema.Espacio(4)})
	case e.Titulo != nil:
		opciones := make([]string, len(api.CastVisibilidades))
		elegida := 0
		for i, vis := range api.CastVisibilidades {
			opciones[i] = v.Traduce(claveVisibilidad(vis))
			if vis == e.Visibilidad {
				elegida = i
			}
		}
		sel := ui.NuevoSelector(opciones, func(i int) {
			if e.AlVisibilidad != nil {
				e.AlVisibilidad(api.CastVisibilidades[i])
			}
		}).Con(ui.CampoSM)
		sel.Elegida, sel.Rotulo = elegida, v.Traduce("visibility.label")
		if !quieto {
			sel.Inerte(true)
		}
		campos := container.New(&campoYEstrecho{dos: v.Ventana >= ui.CorteSM},
			ui.GrupoCampo(v.Traduce("studio.schedule.titleField"), true, e.Titulo, "", ""),
			ui.GrupoCampo(v.Traduce("visibility.label"), false, sel, "", ""))
		bloques = append(bloques, ui.Bloque{Obj: campos, Margen: tema.Espacio(4)})
	}
	if e.Previa != nil {
		bloques = append(bloques, ui.Bloque{Obj: e.Previa, Margen: tema.Espacio(3)})
	}
	if e.Denegado {
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, v.Traduce("studio.app.denied"), tema.TextoXS, ui.Normal, "danger-text"), Margen: tema.Espacio(3)})
	}
	if e.Fase == EmisionFallida {
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, v.Traduce("studio.browser.error"), tema.TextoXS, ui.Normal, "danger-text"), Margen: tema.Espacio(3)})
	}
	microIcono := iconos.MicrofonoApagado
	if e.Micro {
		microIcono = iconos.Microfono
	}
	var accion *ui.Boton
	if e.Fase == EmisionEnDirecto {
		accion = ui.NuevoBoton(v.Traduce("studio.browser.stop"), e.AlTerminar).Con(ui.Peligro, ui.TallaMD)
	} else {
		rotulo := "studio.browser.start"
		if e.Fase == EmisionConectando {
			rotulo = "studio.browser.connecting"
		}
		accion = ui.NuevoBoton(v.Traduce(rotulo), e.AlEmpezar).Con(ui.Primario, ui.TallaMD)
		accion.Glifo = iconos.TrazoDeRuta(glifoDirecto, iconos.TallaSM, tema.Color("text", v.Variante))
		accion.Cargando = e.Fase == EmisionConectando
		accion.Inerte = !e.Listo() || e.Fase == EmisionConectando
	}
	fuentes := []fyne.CanvasObject{
		botonFuente(v, "studio.browser.camera", iconos.Camara, e.Camara, e.AlCamara),
		botonFuente(v, "studio.browser.mic", microIcono, e.Micro, e.AlMicro),
		botonFuente(v, "studio.browser.screen", iconos.Pantalla, e.Pantalla, e.AlPantalla),
	}
	if e.AlCambiarCamara != nil {
		girar := botonFuente(v, "studio.app.flipCamera", iconos.Rotar, false, e.AlCambiarCamara)
		girar.Inerte = !e.Camara // se cambia la cámara que se está viendo
		fuentes = append(fuentes[:1], append([]fyne.CanvasObject{girar}, fuentes[1:]...)...)
	}
	mandos := container.New(ui.FilaAuto{Hueco: tema.Espacio(1), Derecha: len(fuentes), Partir: true, SaltoFila: tema.Espacio(3)},
		append(fuentes, accion)...)
	bloques = append(bloques, ui.Bloque{Obj: mandos})
	return ui.PilaBloques(bloques...)
}

// botonFuente es un conmutador de fuente (`h-10 w-10`, `aria-pressed`): apagado, el
// glifo en `text-muted`.
func botonFuente(v Vista, clave string, icono iconos.Icono, encendido bool, al func()) *ui.BotonIcono {
	b := ui.NuevoBotonIcono(icono, v.Traduce(clave), al)
	b.Lado, b.TamIcono, b.Pulsado = tema.Espacio(10), 18, encendido
	if encendido {
		b.Trazo = "text"
	}
	b.Inerte = al == nil
	return b
}

// campoYEstrecho es la rejilla `sm:grid-cols-[minmax(0,1fr)_11rem] gap-3`: el título
// estirado y la visibilidad a 11rem; en un teléfono, uno encima del otro.
type campoYEstrecho struct{ dos bool }

func (c *campoYEstrecho) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	if c.dos {
		return fyne.NewSize(a.Width+tema.Espacio(3)+11*tema.Rem, max(a.Height, b.Height))
	}
	return fyne.NewSize(max(a.Width, b.Width), a.Height+tema.Espacio(3)+b.Height)
}

func (c *campoYEstrecho) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	if c.dos {
		estrecho := float32(11 * tema.Rem)
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(fyne.NewSize(max(t.Width-estrecho-tema.Espacio(3), 0), a.Height))
		objs[1].Move(fyne.NewPos(t.Width-estrecho, 0))
		objs[1].Resize(fyne.NewSize(estrecho, b.Height))
		return
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(t.Width, a.Height))
	objs[1].Move(fyne.NewPos(0, a.Height+tema.Espacio(3)))
	objs[1].Resize(fyne.NewSize(t.Width, b.Height))
}

// Previa es el cuadro de lo que sale (el `<video>` de EmitirAqui.tsx): negro 16:9, la
// imagen propia —la cámara en espejo, como la web; la pantalla, no—, el aviso de
// elegir una fuente si no hay ninguna y la marca de directo mientras se emite.
//
// Vive en App y no se rehace al repintar: los fotogramas llegan quince veces por
// segundo.
type Previa struct {
	imagen *canvas.Image
	vacio  *fyne.Container
	marca  fyne.CanvasObject
	raiz   *fyne.Container
}

// NuevaPrevia crea la previa vacía.
func NuevaPrevia(v Vista) *Previa {
	p := &Previa{}
	fondo := canvas.NewRectangle(color.NRGBA{A: 255})
	p.imagen = canvas.NewImageFromImage(nil)
	p.imagen.FillMode = canvas.ImageFillContain
	p.imagen.ScaleMode = canvas.ImageScaleSmooth
	blanco := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	p.vacio = container.New(&vacioPrevia{},
		iconos.TrazoConTinta(iconos.Camara, 28, blanco).Objeto(),
		ui.NuevoParrafoConTinta(v.Traduce("studio.browser.noSource"), tema.TextoSM, ui.Normal, blanco))
	p.marca = ui.NuevaInsignia(v.Traduce("studio.browser.live"), ui.InsigniaPeligro)
	p.marca.Hide()
	p.raiz = container.New(&previaLayout{}, fondo, p.imagen, p.vacio, p.marca)
	return p
}

// Objeto es lo que se cuelga del panel.
func (p *Previa) Objeto() fyne.CanvasObject { return p.raiz }

// Imagen pone lo que sale; nil vuelve al aviso de elegir fuente. `espejo` le da la
// vuelta (la cámara propia se ve como en un espejo). En el hilo de la interfaz.
func (p *Previa) Imagen(img image.Image, espejo bool) {
	if img != nil && espejo {
		img = espejar(img)
	}
	p.imagen.Image = img
	p.imagen.Refresh()
	if img == nil {
		p.vacio.Show()
	} else {
		p.vacio.Hide()
	}
}

// EnDirecto enciende o apaga la marca «Estás en directo».
func (p *Previa) EnDirecto(si bool) {
	if si {
		p.marca.Show()
	} else {
		p.marca.Hide()
	}
}

// TieneImagen dice si se ve algo.
func (p *Previa) TieneImagen() bool { return p.imagen.Image != nil }

// espejar devuelve una copia volteada en horizontal. Se pasa a RGBA de una vez (el
// camino rápido de `draw`) y se cambian los píxeles de cada fila de sitio: pedir
// `At` píxel a píxel a un YCbCr costaría una asignación por píxel, quince veces por
// segundo.
func espejar(img image.Image) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	w := b.Dx()
	for y := range b.Dy() {
		fila := out.Pix[y*out.Stride : y*out.Stride+4*w]
		for i, j := 0, w-1; i < j; i, j = i+1, j-1 {
			a, z := fila[4*i:4*i+4], fila[4*j:4*j+4]
			a[0], a[1], a[2], a[3], z[0], z[1], z[2], z[3] = z[0], z[1], z[2], z[3], a[0], a[1], a[2], a[3]
		}
	}
	return out
}

// previaLayout: el cuadro 16:9 a todo el ancho, la imagen dentro, el aviso centrado y
// la marca arriba a la izquierda (`top-3 left-3`).
type previaLayout struct{}

func (previaLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(16*tema.Rem, 9*tema.Rem)
}

// AltoCon es el 16:9 de ese ancho.
func (previaLayout) AltoCon(_ []fyne.CanvasObject, ancho float32) float32 { return ancho * 9 / 16 }

func (previaLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs[:3] {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
	m := objs[3].MinSize()
	objs[3].Move(fyne.NewPos(tema.Espacio(3), tema.Espacio(3)))
	objs[3].Resize(m)
}

// vacioPrevia centra el glifo y, debajo con `gap-2`, el aviso (`px-6 text-center`).
type vacioPrevia struct{}

func (vacioPrevia) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (vacioPrevia) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	texto, ok := objs[1].(*ui.Parrafo)
	if !ok {
		return
	}
	ancho := max(min(t.Width-2*tema.Espacio(6), ui.Medir(texto.Texto, tema.TextoSM, ui.Normal).Width), 0)
	alto := ui.AltoParrafo(texto, ancho)
	lado := float32(28)
	y := (t.Height - lado - tema.Espacio(2) - alto) / 2
	objs[0].Move(fyne.NewPos((t.Width-lado)/2, y))
	objs[0].Resize(fyne.NewSize(lado, lado))
	texto.Move(fyne.NewPos((t.Width-ancho)/2, y+lado+tema.Espacio(2)))
	texto.Resize(fyne.NewSize(ancho, alto))
}

// ---- OBS ----

// panelOBS es la clave de emisión: el servidor y la clave, con mostrar, copiar y
// regenerar, y los tres pasos de OBS.
func panelOBS(v Vista, d Emitir) fyne.CanvasObject {
	if d.Clave == nil {
		if d.ErrorClave != "" {
			return Parrafo(v, d.ErrorClave, tema.TextoSM, ui.Normal, "danger-text")
		}
		r := ui.NuevaRueda(ui.LadoRueda, "text-muted", v.Variante)
		r.Rotulo = v.Traduce("ui.loading")
		return container.New(ui.FilaAuto{Derecha: 1}, r)
	}
	servidor := campoCopiable(v, v.Traduce("studio.obs.server"), d.Clave.RTMPURL, nil, d.AlCopiarServidor)
	clave := d.Clave.StreamKey
	if !d.ClaveVisible {
		clave = strings.Repeat("•", min(len(clave), 24))
	}
	glifo, rotulo := glifoOjo, "studio.obs.show"
	if d.ClaveVisible {
		glifo, rotulo = glifoOjoNo, "studio.obs.hide"
	}
	ver := ui.NuevoBotonIcono(iconos.Ninguno, v.Traduce(rotulo), d.AlVerClave)
	ver.Glifo = iconos.TrazoDeRuta(glifo, iconos.TallaSM, tema.Color("text-muted", v.Variante))
	ver.Lado = ui.LadoIconoLG
	claveFila := campoCopiable(v, v.Traduce("studio.obs.key"), clave, ver, d.AlCopiarClave)
	regenerar := ui.NuevoBoton(v.Traduce("studio.obs.rotate"), d.AlRegenerar).Con(ui.Fantasma, ui.TallaSM)
	regenerar.Inerte = d.Regenerando
	pasos := Parrafo(v, v.Traduce("studio.obs.step1")+" "+v.Traduce("studio.obs.step2")+" "+v.Traduce("studio.obs.step3")+" "+v.Traduce("studio.obs.secret"), tema.TextoXS, ui.Normal, "text-muted")
	return ui.PilaBloques(
		ui.Bloque{Obj: servidor, Margen: tema.Espacio(3)},
		ui.Bloque{Obj: claveFila, Margen: tema.Espacio(3)},
		ui.Bloque{Obj: pasos, Margen: tema.Espacio(3)},
		ui.Bloque{Obj: container.New(ui.FilaAuto{Derecha: 1}, regenerar)},
	)
}

// campoCopiable es un rótulo y, debajo, un valor en monoespaciada dentro de una caja
// con sus botones (mostrar y copiar).
func campoCopiable(v Vista, rotulo, valor string, extra *ui.BotonIcono, alCopiar func()) fyne.CanvasObject {
	copiar := ui.NuevoBoton(v.Traduce("studio.obs.copy"), alCopiar).Con(ui.Secundario, ui.TallaSM).ConIcono(iconos.Copias, iconos.TallaSM)
	texto := ui.Texto(valor, tema.TextoSM, ui.Mono, tema.Color("text", v.Variante))
	caja := ui.NuevaCaja(tema.Color("bg-subtle", v.Variante), tema.Color("border", v.Variante), tema.RadioMD)
	valorCaja := container.New(&cajaValor{caja: caja, texto: texto, completo: valor}, append(caja.Objetos(), texto)...)
	hijos := []fyne.CanvasObject{valorCaja}
	if extra != nil {
		hijos = append(hijos, extra)
	}
	hijos = append(hijos, copiar)
	fila := container.New(ui.FilaAuto{Hueco: HuecoFila, Derecha: len(hijos), Elastico: 1, Partir: true}, hijos...)
	return ui.GrupoCampo(rotulo, false, fila, "", "")
}

// cajaValor es el `<code>` del valor: `h-8 px-3`, truncado si no cabe.
type cajaValor struct {
	caja     *ui.Caja
	texto    *canvas.Text
	completo string
}

func (c *cajaValor) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(6*tema.Rem, tema.Espacio(8))
}

func (c *cajaValor) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.caja.Redimensionar(t)
	ui.Truncar(c.texto, c.completo, tema.TextoSM, ui.Mono, t.Width-2*tema.Espacio(3))
	ui.ColocarTexto(c.texto, tema.TextoSM, tema.Espacio(3), (t.Height-tema.TextoSM.Linea)/2)
}

// ---- Programar ----

// panelProgramar es el formulario de Programar (StudioApp.tsx): título, descripción,
// desde dónde se emitirá, visibilidad y «Programar».
func panelProgramar(v Vista, d Emitir) fyne.CanvasObject {
	fuente := func(clave, valor string) *ui.Eleccion {
		r := ui.NuevoRadio(v.Traduce(clave), func(bool) {
			if d.AlFuente != nil {
				d.AlFuente(valor)
			}
		})
		r.Marcada, r.Escalon = d.Fuente == valor, tema.TextoSM
		return r
	}
	// La web dice «Este navegador» (studio.source.webrtc); aquí se emite desde la app,
	// y el dueño decidió (2026-09-28) que en nativo el origen se llame «Este Cast»:
	// clave propia de la app en el catálogo web, `studio.source.app`.
	fuentes := container.New(ui.FilaAuto{Hueco: tema.Espacio(4), Derecha: 2, Partir: true, SaltoFila: tema.Espacio(2)},
		fuente("studio.source.rtmp", "rtmp"), fuente("studio.source.app", "webrtc"))
	leyenda := ui.Texto(v.Traduce("studio.schedule.source"), tema.TextoSM, ui.SemiNegrita, tema.Color("text", v.Variante))
	enviar := ui.NuevoBoton(v.Traduce("studio.schedule.submit"), d.AlProgramar).Con(ui.Primario, ui.TallaMD)
	enviar.Cargando = d.Programando
	enviar.Inerte = d.Programando || d.Titulo == nil || strings.TrimSpace(d.Titulo.Texto()) == ""
	var bloques []ui.Bloque
	if d.Titulo != nil {
		bloques = append(bloques, ui.Bloque{Obj: ui.GrupoCampo(v.Traduce("studio.schedule.titleField"), true, d.Titulo, "", ""), Margen: tema.Espacio(4)})
	}
	if d.Descripcion != nil {
		bloques = append(bloques, ui.Bloque{Obj: ui.GrupoCampo(v.Traduce("studio.schedule.descriptionField"), false, d.Descripcion, "", ""), Margen: tema.Espacio(4)})
	}
	return ui.PilaBloques(append(bloques,
		ui.Bloque{Obj: container.New(ui.FilaAuto{Derecha: 1, AlturaFila: tema.TextoSM.Linea}, leyenda), Margen: tema.Espacio(2) + tema.Espacio(1.5)},
		ui.Bloque{Obj: fuentes, Margen: tema.Espacio(4)},
		ui.Bloque{Obj: SelectorVisibilidad(v, d.Visibilidad, d.AlVisibilidad), Margen: tema.Espacio(4)},
		ui.Bloque{Obj: container.New(ui.FilaAuto{}, enviar)},
	)...)
}

// ---- Mis directos ----

// tarjetaDirectos es MisDirectos: la tarjeta con su título y, dentro, una fila por
// directo separadas por una raya de borde a borde (`divide-y -mx-5`).
func tarjetaDirectos(v Vista, d Emitir) fyne.CanvasObject {
	titulo := ui.NuevoParrafo(v.Traduce("studio.lives.title"), tema.TextoMD, ui.SemiNegrita, "text", v.Variante)
	var cuerpo []fyne.CanvasObject
	switch {
	case d.Directos == nil && d.SinConexion:
		cuerpo = append(cuerpo, Parrafo(v, v.Traduce("errors.offline.description"), tema.TextoSM, ui.Normal, "text-muted"))
	case d.Directos == nil:
		cuerpo = append(cuerpo, container.New(&altoFijo{alto: 2.5 * tema.Rem}, ui.NuevoEsqueleto(0, 0)),
			container.New(&altoFijo{alto: 2.5 * tema.Rem}, ui.NuevoEsqueleto(0, 0)))
	case len(d.Directos) == 0:
		cuerpo = append(cuerpo, Parrafo(v, v.Traduce("studio.lives.empty"), tema.TextoSM, ui.Normal, "text-muted"))
	}
	caja := ui.NuevaCaja(tema.Color("surface", v.Variante), tema.Color("border", v.Variante), tema.RadioLG)
	objs := append(caja.Objetos(), titulo)
	l := &listaEnTarjeta{caja: caja, n: len(caja.Objetos())}
	if cuerpo != nil {
		l.sueltos = len(cuerpo)
		objs = append(objs, cuerpo...)
		return container.New(l, objs...)
	}
	for i, x := range d.Directos {
		raya := canvas.NewRectangle(tema.Color("border", v.Variante))
		if i == 0 {
			raya.Hide()
		}
		objs = append(objs, raya, filaDirecto(v, x, d))
	}
	return container.New(l, objs...)
}

// filaDirecto es un directo mío: título y cuándo, su estado y sus acciones.
func filaDirecto(v Vista, x api.CastVideo, d Emitir) fyne.CanvasObject {
	cuando := x.CreatedAt
	if x.Live != nil && x.Live.StartedAt != nil {
		cuando = *x.Live.StartedAt
	}
	titulo := Parrafo(v, x.Title, tema.TextoSM, ui.SemiNegrita, "text")
	titulo.Lineas = 1
	texto := ui.PilaBloques(
		ui.Bloque{Obj: titulo},
		ui.Bloque{Obj: Parrafo(v, i18n.FormatDateTime(v.T.Locale(), cuando.Local()), tema.TextoXS, ui.Normal, "text-subtle")},
	)
	con := func(f func(api.CastVideo)) func() {
		return func() {
			if f != nil {
				f(x)
			}
		}
	}
	hijos := []fyne.CanvasObject{texto, ui.NuevaInsignia(v.Traduce(claveEstado(x.Status)), tonoEstado(x.Status))}
	if x.Status == api.CastStatusScheduled && x.Live != nil && x.Live.Source == "webrtc" {
		hijos = append(hijos, ui.NuevoBoton(v.Traduce("studio.lives.emitHere"), con(d.AlEmitirAqui)).Con(ui.Fantasma, ui.TallaSM))
	}
	if x.Status == api.CastStatusLive || x.Status == api.CastStatusScheduled {
		hijos = append(hijos, ui.NuevoBoton(v.Traduce("studio.lives.end"), con(d.AlTerminar)).Con(ui.Fantasma, ui.TallaSM))
	}
	hijos = append(hijos, ui.NuevoBoton(v.Traduce("studio.lives.open"), con(d.AlAbrir)).Con(ui.Secundario, ui.TallaSM))
	return container.New(ui.FilaAuto{Hueco: tema.Espacio(3), Derecha: 1, Elastico: 1, Partir: true, SaltoFila: tema.Espacio(3)}, hijos...)
}

// listaEnTarjeta coloca la tarjeta de «Mis directos»: la caja, el título (`p-5`,
// `gap-3`) y, o bien lo suelto (esqueletos o un aviso, con `gap-2`), o bien pares de
// raya y fila, cada fila con `px-5 py-2.5` y la raya de lado a lado.
type listaEnTarjeta struct {
	caja *ui.Caja
	// n son los objetos de la caja; sueltos, los hijos que no son pares de raya y fila.
	n, sueltos int
}

func (l *listaEnTarjeta) partes(objs []fyne.CanvasObject) (fyne.CanvasObject, []fyne.CanvasObject) {
	return objs[l.n], objs[l.n+1:]
}

func (l *listaEnTarjeta) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(16*tema.Rem, l.AltoCon(objs, 24*tema.Rem))
}

// AltoCon suma el título y las filas a ese ancho.
func (l *listaEnTarjeta) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	p := float32(rellenoRecuadro)
	titulo, resto := l.partes(objs)
	interior := ancho - 2*p
	h := p + ui.AltoA(titulo, interior)
	if l.sueltos > 0 {
		for _, o := range resto {
			h += tema.Espacio(3) + ui.AltoA(o, interior)
		}
		return h + p
	}
	h += tema.Espacio(3) - tema.Espacio(2.5)
	for i := 1; i < len(resto); i += 2 {
		h += 2*tema.Espacio(2.5) + ui.AltoA(resto[i], interior)
		if i > 1 {
			h += tema.Borde
		}
	}
	return h + p
}

func (l *listaEnTarjeta) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	p := float32(rellenoRecuadro)
	l.caja.Redimensionar(t)
	titulo, resto := l.partes(objs)
	interior := t.Width - 2*p
	y := p
	colocar := func(o fyne.CanvasObject) {
		h := ui.AltoA(o, interior)
		o.Move(fyne.NewPos(p, y))
		o.Resize(fyne.NewSize(interior, h))
		y += h
	}
	colocar(titulo)
	if l.sueltos > 0 {
		for _, o := range resto {
			y += tema.Espacio(3)
			colocar(o)
		}
		return
	}
	y += tema.Espacio(3) - tema.Espacio(2.5)
	for i := 0; i+1 < len(resto); i += 2 {
		if i > 0 {
			resto[i].Move(fyne.NewPos(tema.Borde, y))
			resto[i].Resize(fyne.NewSize(t.Width-2*tema.Borde, tema.Borde))
			y += tema.Borde
		}
		y += tema.Espacio(2.5)
		colocar(resto[i+1])
		y += tema.Espacio(2.5)
	}
}

// mitades reparte dos hijos a partes iguales con `gap-6`, del alto del más alto.
type mitades struct{}

func (mitades) ancho(t float32) float32 { return (t - tema.Espacio(6)) / 2 }

func (m mitades) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, max(objs[0].MinSize().Height, objs[1].MinSize().Height))
}

// AltoCon es el del más alto a media anchura.
func (m mitades) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	w := m.ancho(ancho)
	return max(ui.AltoA(objs[0], w), ui.AltoA(objs[1], w))
}

func (m mitades) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	w := m.ancho(t.Width)
	h := m.AltoCon(objs, t.Width)
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(w, h))
	objs[1].Move(fyne.NewPos(w+tema.Espacio(6), 0))
	objs[1].Resize(fyne.NewSize(w, h))
}
