// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/cast/src/pages/upload.astro

package cast

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// FaseSubida es en qué punto está un fichero de la cola (cola.ts de la web).
type FaseSubida uint8

// Las cuatro fases.
const (
	// Esperando: en la cola, detrás del que sube (se sube de uno en uno).
	Esperando FaseSubida = iota
	Subiendo
	// Procesando: el original ya está arriba y el transcodificador lo tiene.
	Procesando
	Fallida
)

// Envio es un fichero de la cola de subida.
type Envio struct {
	Clave   int
	Ruta    string
	Nombre  string
	Tamano  int64
	Enviado int64
	Fase    FaseSubida
	// Motivo es el texto del fallo, ya traducido.
	Motivo  string
	VideoID string
	// Titulo es su campo: se edita mientras sube y se guarda al pulsar Intro o al
	// terminar. Vive en App.
	Titulo *ui.Entrada

	visibilidad string // con la que nace
	guardado    string // el último título guardado en el servicio
	calidades   []int  // las elegidas; vacía, la recomendada
}

// Preparado es un fichero elegido que espera en «Antes de subir».
type Preparado struct {
	Clave  int
	Ruta   string
	Nombre string
	Tamano int64
	// Estimacion es la del servicio para este fichero; nil mientras llega.
	Estimacion *api.CastEstimate
	Error      bool
}

// Subir son los datos de la pantalla «Subir».
type Subir struct {
	// Cuota es la del plan; nil mientras llega. ErrorCuota: no se pudo leer.
	Cuota       *api.CastQuota
	ErrorCuota  bool
	SinConexion bool
	// Visibilidad es con la que nacen los vídeos nuevos.
	Visibilidad string
	Arrastrando bool
	Cola        []Envio
	// Preparados son los ficheros elegidos que aún no suben: esperan a que se
	// confirmen las calidades (Calidades), con lo que ocupará cada una.
	Preparados []Preparado
	Calidades  SelCalidades
	AlEmpezar  func()
	AlDescarte func()

	AlElegir      func()
	AlVisibilidad func(string)
	AlCancelar    func(clave int)
	AlQuitar      func(clave int)
	AlEditar      func(videoID string)
}

// TopeVideo es el tope por vídeo (decisión 6 del dueño).
const TopeVideo int64 = 8 << 30

// AnchoLateralSubir es la columna de la derecha (`lg:grid-cols-[1fr_20rem]`).
const AnchoLateralSubir = 20 * tema.Rem

// PantallaSubir monta «Subir»: la zona de soltar y la cola a la izquierda; la cuota y
// la visibilidad de lo nuevo a la derecha.
func PantallaSubir(v Vista, d Subir) *fyne.Container {
	bloques := []ui.Bloque{{Obj: Cabecera(v.Traduce("upload.title"), ""), Margen: tema.Espacio(6)}}
	if cuerpo := CuerpoDeEstado(v, nil); cuerpo != nil {
		return Pagina(v, append(bloques, ui.Bloque{Obj: cuerpo})...)
	}
	izq := []ui.Bloque{}
	if d.SinConexion {
		izq = append(izq, ui.Bloque{Obj: avisoTono(v, v.Traduce("errors.offline.title")+". "+v.Traduce("errors.offline.description"), "warning-bg", "warning-text"), Margen: tema.Espacio(6)})
	}
	izq = append(izq, ui.Bloque{Obj: zonaSoltar(v, d), Margen: tema.Espacio(6)})
	if len(d.Preparados) > 0 {
		izq = append(izq, ui.Bloque{Obj: antesDeSubir(v, d), Margen: tema.Espacio(6)})
	}
	if len(d.Cola) > 0 {
		izq = append(izq, ui.Bloque{Obj: TituloSeccion(v, v.Traduce("upload.queue")), Margen: tema.Espacio(2)})
		for _, e := range d.Cola {
			izq = append(izq, ui.Bloque{Obj: filaEnvio(v, e, d), Margen: HuecoFila})
		}
	}
	var cuota fyne.CanvasObject
	switch {
	case d.Cuota != nil:
		cuota = medidorCuota(v, *d.Cuota)
	case d.ErrorCuota:
		cuota = Parrafo(v, v.Traduce("upload.quotaError"), tema.TextoSM, ui.Normal, "danger-text")
	default:
		cuota = ui.NuevoEsqueleto(0, tema.Espacio(8))
	}
	der := ui.PilaBloques(
		ui.Bloque{Obj: Recuadro(v, v.Traduce("upload.quota"), cuota), Margen: tema.Espacio(6)},
		ui.Bloque{Obj: Recuadro(v, v.Traduce("upload.defaultVisibility"), SelectorVisibilidad(v, d.Visibilidad, d.AlVisibilidad))},
	)
	if v.Ventana < ui.CorteLG {
		bloques = append(bloques, ui.Bloque{Obj: ui.PilaBloques(izq...), Margen: tema.Espacio(6)}, ui.Bloque{Obj: der})
	} else {
		bloques = append(bloques, ui.Bloque{Obj: container.New(&dosColumnas{derecha: AnchoLateralSubir}, ui.PilaBloques(izq...), der)})
	}
	return Pagina(v, bloques...)
}

// avisoTono es el `<p class="bg-*-bg text-*-text rounded-md px-4 py-3">` de un aviso.
func avisoTono(v Vista, texto, fondo, tinta string) fyne.CanvasObject {
	caja := ui.NuevaCaja(tema.Color(fondo, v.Variante), transparente(), tema.RadioMD)
	return container.New(&recuadroFino{caja: caja}, append(caja.Objetos(), Parrafo(v, texto, tema.TextoSM, ui.Normal, tinta))...)
}

// medidorCuota es el Meter del espacio del plan, compartido con Drive.
func medidorCuota(v Vista, q api.CastQuota) fyne.CanvasObject {
	fraccion := 0.0
	if q.LimitBytes > 0 {
		fraccion = float64(q.UsedBytes) / float64(q.LimitBytes)
	}
	m := ui.NuevoMedidor(v.Traduce("upload.quota"), fraccion, i18n.FormatPercent(v.T.Locale(), fraccion))
	m.Cuanto = v.Traduce("upload.quotaText", i18n.Vars{"used": Tamano(v, q.UsedBytes), "limit": Tamano(v, q.LimitBytes), "cast": Tamano(v, q.CastBytes)})
	switch {
	case fraccion >= 1:
		m.Nota = v.Traduce("ui.meter.full")
	case fraccion >= 0.9:
		m.Nota = v.Traduce("ui.meter.almostFull")
	}
	return m
}

// zonaSoltar es el recuadro discontinuo de «arrastra vídeos aquí», que es también un
// botón para quien no arrastra.
func zonaSoltar(v Vista, d Subir) fyne.CanvasObject {
	token := "border-strong"
	if d.Arrastrando {
		token = "primary"
	}
	borde := ui.NuevoBordeDiscontinuo(2, tema.RadioMD, token)
	glifo := iconos.Trazo(iconos.Subir, 28, "text-subtle", v.Variante).Objeto()
	// En el teléfono no se arrastra nada: el recuadro dice de dónde sale el vídeo, y el
	// botón abre la galería (sesion.tiposMovil).
	clave := "upload.drop"
	if v.Movil {
		clave = "upload.drive.other"
	}
	titulo := ui.Texto(v.Traduce(clave), tema.TextoMD, ui.SemiNegrita, tema.Color("text", v.Variante))
	ayuda := ui.Texto(v.Traduce("upload.hint"), tema.TextoXS, ui.Normal, tema.Color("text-muted", v.Variante))
	elegir := ui.NuevoBoton(v.Traduce("upload.choose"), d.AlElegir).Con(ui.Primario, ui.TallaSM)
	return container.New(&zonaLayout{}, borde, glifo, titulo, ayuda, elegir)
}

// zonaLayout centra en columna el glifo, el título, la ayuda y el botón dentro del
// borde, con `py-12`.
type zonaLayout struct{}

func (zonaLayout) alto(objs []fyne.CanvasObject) float32 {
	return 2*tema.Espacio(12) + 28 + tema.Espacio(3) + tema.TextoMD.Linea + tema.Espacio(1) +
		tema.TextoXS.Linea + tema.Espacio(4) + objs[4].MinSize().Height
}

func (z zonaLayout) MinSize(objs []fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, z.alto(objs)) }

// AltoCon es el fijo.
func (z zonaLayout) AltoCon(objs []fyne.CanvasObject, _ float32) float32 { return z.alto(objs) }

func (zonaLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	y := tema.Espacio(12)
	objs[1].Move(fyne.NewPos((t.Width-28)/2, y))
	objs[1].Resize(fyne.NewSize(28, 28))
	y += 28 + tema.Espacio(3)
	for i, e := range []tema.Escalon{tema.TextoMD, tema.TextoXS} {
		texto, ok := objs[2+i].(*canvas.Text)
		if !ok {
			continue
		}
		peso := ui.SemiNegrita
		if i == 1 {
			peso = ui.Normal
		}
		w := ui.Medir(texto.Text, e, peso).Width
		ui.ColocarTexto(texto, e, (t.Width-w)/2, y)
		y += e.Linea + tema.Espacio(1)
	}
	y += tema.Espacio(3)
	b := objs[4].MinSize()
	objs[4].Move(fyne.NewPos((t.Width-b.Width)/2, y))
	objs[4].Resize(b)
}

// filaEnvio es un fichero de la cola (UploadQueue.tsx): el icono de su estado, el
// título editable, el nombre y el tamaño, la barra o el estado, y sus mandos.
func filaEnvio(v Vista, e Envio, d Subir) fyne.CanvasObject {
	glifo, tinta := glifoVideo, "text-subtle"
	switch e.Fase {
	case Fallida:
		glifo, tinta = "", "danger-text"
	case Procesando:
		glifo, tinta = "", "success-text"
	case Esperando, Subiendo:
	}
	var icono fyne.CanvasObject
	switch {
	case glifo != "":
		icono = iconos.TrazoDeRuta(glifo, iconos.TallaSM, tema.Color(tinta, v.Variante)).Objeto()
	case e.Fase == Fallida:
		icono = iconos.Trazo(iconos.Peligro, iconos.TallaSM, tinta, v.Variante).Objeto()
	default:
		icono = iconos.Trazo(iconos.Tic, iconos.TallaSM, tinta, v.Variante).Objeto()
	}
	var estado fyne.CanvasObject
	switch e.Fase {
	case Subiendo:
		fraccion := 0.0
		if e.Tamano > 0 {
			fraccion = float64(e.Enviado) / float64(e.Tamano)
		}
		estado = ui.NuevaBarraProgreso(v.Traduce("upload.uploading", i18n.Vars{"name": e.Titulo.Texto()}), fraccion)
	case Esperando:
		estado = Parrafo(v, v.Traduce("upload.waiting"), tema.TextoXS, ui.Normal, "text-muted")
	case Procesando:
		estado = Parrafo(v, v.Traduce("upload.processing"), tema.TextoXS, ui.Normal, "success-text")
	case Fallida:
		estado = Parrafo(v, e.Motivo, tema.TextoXS, ui.Normal, "danger-text")
	}
	if e.Fase == Fallida {
		e.Titulo.Inerte(true)
	}
	centro := ui.PilaBloques(
		ui.Bloque{Obj: e.Titulo, Margen: tema.Espacio(1.5)},
		ui.Bloque{Obj: Parrafo(v, e.Nombre+" · "+Tamano(v, e.Tamano), tema.TextoXS, ui.Normal, "text-subtle"), Margen: tema.Espacio(1.5)},
		ui.Bloque{Obj: estado},
	)
	var mandos []fyne.CanvasObject
	if e.Fase == Procesando && e.VideoID != "" {
		id := e.VideoID
		mandos = append(mandos, ui.NuevoBoton(v.Traduce("upload.edit"), func() {
			if d.AlEditar != nil {
				d.AlEditar(id)
			}
		}).Con(ui.Secundario, ui.TallaSM))
	}
	clave := e.Clave
	if e.Fase == Subiendo || e.Fase == Esperando {
		mandos = append(mandos, ui.NuevoBotonIcono(iconos.Cerrar, v.Traduce("upload.cancel"), func() {
			if d.AlCancelar != nil {
				d.AlCancelar(clave)
			}
		}))
	} else {
		mandos = append(mandos, ui.NuevoBotonIcono(iconos.Cerrar, v.Traduce("upload.remove"), func() {
			if d.AlQuitar != nil {
				d.AlQuitar(clave)
			}
		}))
	}
	fila := container.New(&filaCola{}, append([]fyne.CanvasObject{icono, centro}, mandos...)...)
	caja := ui.NuevaCaja(tema.Color("surface", v.Variante), tema.Color("border", v.Variante), tema.RadioMD)
	return container.New(&recuadroFino{caja: caja}, append(caja.Objetos(), fila)...)
}

// filaCola: el icono arriba a la izquierda, el centro estirado y los mandos a la
// derecha, centrados con el campo del título (`flex items-start gap-3` y `gap-1`).
type filaCola struct{}

func (filaCola) mandos(objs []fyne.CanvasObject) float32 {
	w := float32(0)
	for _, o := range objs[2:] {
		w += o.MinSize().Width + tema.Espacio(1)
	}
	return w
}

func (f filaCola) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, objs[1].MinSize().Height)
}

// AltoCon es el del centro al ancho que le queda.
func (f filaCola) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return ui.AltoA(objs[1], ancho-iconos.TallaSM-tema.Espacio(3)-f.mandos(objs)-tema.Espacio(3))
}

func (f filaCola) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, tema.Espacio(2)))
	objs[0].Resize(fyne.NewSize(iconos.TallaSM, iconos.TallaSM))
	x := iconos.TallaSM + tema.Espacio(3)
	w := t.Width - x - f.mandos(objs) - tema.Espacio(3)
	objs[1].Move(fyne.NewPos(x, 0))
	objs[1].Resize(fyne.NewSize(w, ui.AltoA(objs[1], w)))
	x += w + tema.Espacio(3)
	for _, o := range objs[2:] {
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (tema.Espacio(8)-m.Height)/2))
		o.Resize(m)
		x += m.Width + tema.Espacio(1)
	}
}

// recuadroFino es el recuadro con `px-4 py-3`, el de las filas.
type recuadroFino struct{ caja *ui.Caja }

func (r *recuadroFino) m() (x, y float32) { return tema.Espacio(4), tema.Espacio(3) }

func (r *recuadroFino) MinSize(objs []fyne.CanvasObject) fyne.Size {
	x, y := r.m()
	m := objs[len(objs)-1].MinSize()
	return fyne.NewSize(m.Width+2*x, m.Height+2*y)
}

// AltoCon es el del cuerpo con su relleno.
func (r *recuadroFino) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	x, y := r.m()
	return ui.AltoA(objs[len(objs)-1], ancho-2*x) + 2*y
}

func (r *recuadroFino) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	x, y := r.m()
	r.caja.Redimensionar(t)
	c := objs[len(objs)-1]
	c.Move(fyne.NewPos(x, y))
	c.Resize(fyne.NewSize(t.Width-2*x, t.Height-2*y))
}

// SelectorVisibilidad es el VisibilityPicker: los cuatro niveles como tarjetas de
// elección (ChoiceCard), cada una con lo que significa, en dos columnas.
func SelectorVisibilidad(v Vista, elegida string, alElegir func(string)) fyne.CanvasObject {
	iconosVis := map[string]iconos.Icono{
		api.CastPublic: iconos.Globo, api.CastUnlisted: iconos.Enlace,
		api.CastOrg: iconos.Edificio, api.CastPeople: iconos.Usuarios,
	}
	tarjetas := make([]fyne.CanvasObject, len(api.CastVisibilidades))
	for i, vis := range api.CastVisibilidades {
		clave := claveVisibilidad(vis)
		c := ui.NuevaTarjetaEleccion(v.Traduce(clave), v.Traduce(clave+".description"), func() {
			if alElegir != nil {
				alElegir(vis)
			}
		})
		c.Icono, c.Densa, c.Elegida = iconosVis[vis], true, vis == elegida
		tarjetas[i] = c
	}
	rotulo := ui.Texto(v.Traduce("visibility.label"), tema.TextoSM, ui.SemiNegrita, tema.Color("text", v.Variante))
	return ui.PilaBloques(
		ui.Bloque{Obj: container.New(ui.FilaAuto{Derecha: 1, AlturaFila: tema.TextoSM.Linea}, rotulo), Margen: ui.HuecoGrupoCampo},
		ui.Bloque{Obj: container.New(&eleccionEnDos{}, tarjetas...)},
	)
}

// eleccionEnDos reparte las tarjetas en dos columnas con `gap-2` (una sola por debajo de
// 30rem), cada fila del alto de su tarjeta más alta a ese ancho.
type eleccionEnDos struct{}

func (eleccionEnDos) columnas(ancho float32) (int, float32) {
	if ancho < 30*tema.Rem {
		return 1, ancho
	}
	return 2, (ancho - HuecoFila) / 2
}

func (e eleccionEnDos) filas(objs []fyne.CanvasObject, ancho float32) []float32 {
	n, w := e.columnas(ancho)
	var altos []float32
	for i := 0; i < len(objs); i += n {
		h := float32(0)
		for j := i; j < min(i+n, len(objs)); j++ {
			if c, ok := objs[j].(*ui.TarjetaEleccion); ok {
				h = max(h, ui.AltoTarjetaEleccion(c, w))
			}
		}
		altos = append(altos, h)
	}
	return altos
}

func (e eleccionEnDos) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, e.AltoCon(objs, 40*tema.Rem))
}

// AltoCon es la suma de las filas y sus huecos.
func (e eleccionEnDos) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	total := float32(0)
	for i, h := range e.filas(objs, ancho) {
		if i > 0 {
			total += HuecoFila
		}
		total += h
	}
	return total
}

func (e eleccionEnDos) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	n, w := e.columnas(t.Width)
	y := float32(0)
	for f, h := range e.filas(objs, t.Width) {
		for c := range n {
			i := f*n + c
			if i >= len(objs) {
				break
			}
			if c, ok := objs[i].(*ui.TarjetaEleccion); ok {
				c.Ancho = w
			}
			objs[i].Move(fyne.NewPos(float32(c)*(w+HuecoFila), y))
			objs[i].Resize(fyne.NewSize(w, h))
		}
		y += h + HuecoFila
	}
}

// antesDeSubir es la tarjeta «Antes de subir» (UploadApp.tsx): los ficheros elegidos,
// las calidades con lo que ocupará cada una y los botones de empezar o descartar.
func antesDeSubir(v Vista, d Subir) fyne.CanvasObject {
	bloques := make([]ui.Bloque, 0, len(d.Preparados)+2)
	for _, p := range d.Preparados {
		bloques = append(bloques, ui.Bloque{Obj: Parrafo(v, p.Nombre+" · "+Tamano(v, p.Tamano), tema.TextoSM, ui.Normal, "text-muted"), Margen: tema.Espacio(1)})
	}
	bloques[len(bloques)-1].Margen = tema.Espacio(4)
	empezar := ui.NuevoBoton(v.Traduce("upload.prepare.start"), d.AlEmpezar).Con(ui.Primario, ui.TallaMD)
	empezar.Inerte = d.SinConexion || (d.Calidades.Estimacion == nil && !d.Calidades.Error)
	descartar := ui.NuevoBoton(v.Traduce("upload.prepare.discard"), d.AlDescarte).Con(ui.Fantasma, ui.TallaMD)
	bloques = append(bloques,
		ui.Bloque{Obj: SelectorCalidades(v, d.Calidades), Margen: tema.Espacio(4)},
		ui.Bloque{Obj: Fila(empezar, descartar)},
	)
	return Recuadro(v, v.Traduce("upload.prepare.title"), ui.PilaBloques(bloques...))
}
