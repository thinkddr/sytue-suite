// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"bytes"
	"context"
	"image"
	"image/draw"
	_ "image/jpeg" // las variantes salen en JPEG, salvo que la original fuera PNG
	_ "image/png"
	"slices"
	"sync"

	"fyne.io/fyne/v2"
	xdraw "golang.org/x/image/draw"
)

// # Por qué una galería no puede usar `canvas.Image`, medido
//
// `canvas.Image` decodifica el fichero a su tamaño NATURAL y luego estira el mapa de
// bits hasta la caja con `ApproxBiLinear`. Con los SVG de los iconos eso ya costó dos
// filas negras y cuatro medias donde el navegador pinta cuatro limpias, y por eso los
// iconos van con `canvas.Raster`.
//
// Con fotos es peor por dos razones distintas:
//
//  1. La variante «miniatura» del servicio son 512 px de lado mayor y la tesela mide
//     201 px de CSS (402 de dispositivo a ×2). Estirar 512→402 con un bilineal de dos
//     tomas es visiblemente más blando que remuestrear una vez con Catmull-Rom, que es
//     lo que hace el navegador con `object-fit: cover`.
//  2. `object-cover` RECORTA al centro. `canvas.Image` no tiene modo de recorte: con
//     `ImageFillStretch` deforma y con `ImageFillContain` deja bandas. Una foto
//     apaisada en una tesela cuadrada saldría aplastada.
//
// `canvas.Raster` no tiene ninguno de los dos problemas: Fyne le pide el mapa de bits
// en píxeles de DISPOSITIVO y lo pega 1:1. El recorte, el remuestreo y las esquinas
// redondeadas los hacemos nosotros, que es exactamente lo que hace el navegador.
//
// # Las dos cachés, y por qué son dos
//
// Fyne llama al generador del raster en CADA repintado. Con cien teselas en pantalla,
// decodificar un JPEG de 512 px por tesela y repintado es inviable, así que lo pintado
// se guarda. Y al cambiar el tamaño de la ventana el mapa guardado ya no vale, pero los
// bytes sí: sin la segunda caché, arrastrar el borde de la ventana volvería a pedir la
// galería entera por red.
//
// Las dos van acotadas: una galería son miles de fotos y un mapa sin tope se come la
// memoria de la máquina. La expulsión es FIFO y no LRU de verdad.
//
// ponytail: FIFO por orden de llegada. En una galería se recorre hacia abajo, así que
// lo primero que entró es lo que ya no se ve y FIFO acierta casi siempre; si alguna vez
// se mide un vaivén arriba y abajo que la haga fallar, `container/list` lo convierte en
// LRU sin tocar a quien la llama.

// Los topes de las dos cachés. Medidos contra el tamaño real de las variantes del
// servicio: una «miniatura» de 512 px pesa entre 28 y 60 KB en JPEG, así que 32 MB son
// unas seiscientas fotos de bytes. Y 8 Mpx de mapas pintados son 32 MB a 4 bytes por
// píxel, que a ×2 con teselas de 402 son unas cincuenta teselas: dos pantallas y media.
const (
	TopeBytes    = 32 << 20
	TopePixeles  = 8 << 20
	bytesPorPixe = 4
	// Una página de200 fotos llegó a abrir200 descargas y201 goroutines.
	// Dos preparaciones bajan el pico medido de670MiB a115MiB; el visor usa
	// su propia fototeca y TeselaYa conserva el acceso inmediato a una foto.
	descargasSimultaneas     = 6
	preparacionesSimultaneas = 2
)

type trabajoFoto struct {
	epoca   uint64
	version uint64
	ctx     context.Context
}

type descargaFoto struct {
	id string
	trabajoFoto
}

type preparacionFoto struct {
	clave clavePintada
	radio float32
	trabajoFoto
}

type clavePintada struct {
	id   string
	w, h int
}

// Fototeca es la caché de miniaturas de la galería. El cero NO sirve: usa NuevaFototeca.
type Fototeca struct {
	// Pedir trae los bytes de una variante. Es la única puerta a la red y la pone
	// quien construye la pantalla: el binario le pasa `api.Photos.Variant` y un test
	// le pasa un mapa en memoria.
	Pedir func(ctx context.Context, id, variante string) ([]byte, error)
	// AlLlegar se llama cuando una foto que se pidió ya está: es el `Refresh` del
	// widget que la enseña. Corre en la goroutine de la descarga, así que quien la
	// ponga tiene que envolverla en `fyne.Do` y en el cerrojo de pintado del widget.
	AlLlegar func(id string)
	// Variante es cuál se pide (`miniatura` por defecto, `grande` en la del visor).
	Variante string

	mu                           sync.Mutex
	libre                        *sync.Cond
	crudas                       map[string][]byte
	ordenC                       []string
	pesoC                        int
	pintadas                     map[clavePintada]*image.RGBA
	ordenP                       []clavePintada
	pixeles                      int
	enVuelo                      map[string]bool
	preparando                   map[clavePintada]bool
	fallidas                     map[string]bool
	epoca                        uint64
	versiones                    map[string]uint64
	ctx                          context.Context
	cancelar                     context.CancelFunc
	descargas                    []descargaFoto
	preparaciones                []preparacionFoto
	descargando, preparandoAhora int

	// vivas son los repintados de lo que ENSEÑA cada foto ahora mismo: las teselas y
	// el lienzo del visor se apuntan al nacer, y la app las olvida todas cada vez que
	// rehace la pantalla. Así una foto que llega repinta SU tesela y no la galería
	// entera, y lo que ya no está en pantalla no se queda colgado de la caché.
	muVivas sync.Mutex
	vivas   map[string][]func()
}

// NuevaFototeca crea la caché.
func NuevaFototeca() *Fototeca {
	f := &Fototeca{
		crudas:     map[string][]byte{},
		pintadas:   map[clavePintada]*image.RGBA{},
		enVuelo:    map[string]bool{},
		preparando: map[clavePintada]bool{},
		fallidas:   map[string]bool{},
		vivas:      map[string][]func(){},
		versiones:  map[string]uint64{},
	}
	f.ctx, f.cancelar = context.WithCancel(context.Background())
	f.libre = sync.NewCond(&f.mu)
	return f
}

// apuntar registra quién repinta la foto `id` cuando llegue.
func (f *Fototeca) apuntar(id string, repintar func()) {
	if id == "" {
		return
	}
	f.muVivas.Lock()
	defer f.muVivas.Unlock()
	f.vivas[id] = append(f.vivas[id], repintar)
}

// Olvidar suelta a todos los apuntados. La llama quien rehace la pantalla, ANTES de
// construir la nueva: lo de la vieja ya no se ve y no hay que repintarlo.
func (f *Fototeca) Olvidar() {
	f.muVivas.Lock()
	defer f.muVivas.Unlock()
	clear(f.vivas)
}

// Cancelar retira el trabajo de la pantalla anterior y conserva las fotos ya listas.
// Olvidar sólo retira repintados: también se llama al reconstruir la misma pantalla.
func (f *Fototeca) Cancelar() {
	f.mu.Lock()
	f.cancelar()
	f.epoca++
	f.ctx, f.cancelar = context.WithCancel(context.Background())
	f.descargas, f.preparaciones = nil, nil
	clear(f.enVuelo)
	clear(f.preparando)
	f.libre.Broadcast()
	f.mu.Unlock()
	f.Olvidar()
}

func (f *Fototeca) trabajo(id string) trabajoFoto {
	return trabajoFoto{epoca: f.epoca, version: f.versiones[id], ctx: f.ctx}
}

func (f *Fototeca) vigente(id string, t trabajoFoto) bool {
	return t.epoca == f.epoca && t.version == f.versiones[id] && t.ctx.Err() == nil
}

// enLaInterfaz es `fyne.Do`. Es una variable por los tests: el driver de pruebas de
// Fyne ejecuta `fyne.Do` EN LA MISMA goroutine que lo llama, así que un repintado que
// llega desde la goroutine que prepara la foto correría a la vez que el test pinta.
// `TestMain` la apaga, y los tests repintan después de `Esperar`.
var enLaInterfaz = fyne.Do

// avisar repinta lo que enseña `id`, en el hilo de la interfaz, y llama a AlLlegar.
func (f *Fototeca) avisar(id string) {
	f.muVivas.Lock()
	quienes := append([]func(){}, f.vivas[id]...)
	f.muVivas.Unlock()
	if len(quienes) > 0 {
		enLaInterfaz(func() {
			for _, q := range quienes {
				q()
			}
		})
	}
	if f.AlLlegar != nil {
		f.AlLlegar(id)
	}
}

// Tirar echa una foto de las dos cachés y de la lista de fallidas: la siguiente vez se
// vuelve a pedir. Es lo que hace falta después de girarla, cuando lo guardado ya no es
// lo que el servicio sirve.
func (f *Fototeca) Tirar(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.versiones[id]++
	delete(f.enVuelo, id)
	for k := range f.preparando {
		if k.id == id {
			delete(f.preparando, k)
		}
	}
	if datos, ok := f.crudas[id]; ok {
		f.pesoC -= len(datos)
		delete(f.crudas, id)
		f.ordenC = slices.DeleteFunc(f.ordenC, func(x string) bool { return x == id })
	}
	for k := range f.pintadas {
		if k.id == id {
			f.pixeles -= k.w * k.h
			delete(f.pintadas, k)
		}
	}
	f.ordenP = slices.DeleteFunc(f.ordenP, func(k clavePintada) bool { return k.id == id })
	delete(f.fallidas, id)
}

// Poner mete unos bytes en la caché sin pasar por la red. Es lo que usa un test, y
// también la subida, que ya tiene el fichero en la mano.
func (f *Fototeca) Poner(id string, datos []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.guardarCrudas(id, datos)
}

// guardarCrudas escribe con el candado ya tomado.
func (f *Fototeca) guardarCrudas(id string, datos []byte) {
	if _, ya := f.crudas[id]; ya {
		return
	}
	f.crudas[id], f.ordenC, f.pesoC = datos, append(f.ordenC, id), f.pesoC+len(datos)
	for f.pesoC > TopeBytes && len(f.ordenC) > 1 {
		viejo := f.ordenC[0]
		f.ordenC, f.pesoC = f.ordenC[1:], f.pesoC-len(f.crudas[viejo])
		delete(f.crudas, viejo)
	}
}

// Tesela es el mapa de bits de la foto `id` recortado y escalado a w×h píxeles de
// DISPOSITIVO, con `radio` de esquina redondeada. Devuelve nil si todavía no está:
// entonces ya se ha puesto en cola, y `AlLlegar` avisará.
//
// # Por qué NO decodifica aquí, medido
//
// Esto lo llama el generador del raster, o sea el hilo que PINTA. Preparar una tesela
// cuesta 14,2 ms en el piloto (5,2 de decodificar el JPEG de 512 px y 8,7 de
// remuestrear con Catmull-Rom), y una pantalla son veinticuatro: 340 ms con la interfaz
// congelada, cada vez que entra una fila nueva al desplazarse. Así que aquí solo hay un
// acierto de mapa o un nil, y el trabajo va a `preparar`, en segundo plano.
func (f *Fototeca) Tesela(id string, w, h int, radio float32) *image.RGBA {
	if id == "" || w <= 0 || h <= 0 {
		return nil
	}
	k := clavePintada{id, w, h}
	f.mu.Lock()
	if m, ok := f.pintadas[k]; ok {
		f.mu.Unlock()
		return m
	}
	_, hay := f.crudas[id]
	enCola := f.preparando[k]
	if hay && !enCola {
		f.preparando[k] = true
		f.preparaciones = append(f.preparaciones, preparacionFoto{k, radio, f.trabajo(id)})
		if f.preparandoAhora < preparacionesSimultaneas {
			f.preparandoAhora++
			go f.prepararCola()
		}
	}
	f.mu.Unlock()
	if !hay {
		f.pedir(id)
		return nil
	}
	return nil
}

// Pintada es la última tesela que se preparó de `id`, del tamaño que sea, o nil. Es lo
// que enseña el visor mientras prepara la del tamaño nuevo.
func (f *Fototeca) Pintada(id string) *image.RGBA {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.ordenP) - 1; i >= 0; i-- {
		if k := f.ordenP[i]; k.id == id {
			return f.pintadas[k]
		}
	}
	return nil
}

// preparar decodifica, recorta y remuestrea FUERA del hilo que pinta, y guarda el
// resultado.
func (f *Fototeca) prepararCola() {
	for {
		f.mu.Lock()
		if len(f.preparaciones) == 0 {
			f.preparaciones = nil
			f.preparandoAhora--
			f.libre.Broadcast()
			f.mu.Unlock()
			return
		}
		p := f.preparaciones[0]
		f.preparaciones[0] = preparacionFoto{}
		f.preparaciones = f.preparaciones[1:]
		f.mu.Unlock()
		f.prepararTrabajo(p)
	}
}

func (f *Fototeca) preparar(k clavePintada, radio float32) {
	f.mu.Lock()
	p := preparacionFoto{k, radio, f.trabajo(k.id)}
	f.mu.Unlock()
	f.prepararTrabajo(p)
}

func (f *Fototeca) prepararTrabajo(p preparacionFoto) {
	k := p.clave
	f.mu.Lock()
	if !f.vigente(k.id, p.trabajoFoto) {
		f.mu.Unlock()
		return
	}
	crudas := f.crudas[k.id]
	f.mu.Unlock()
	m := recortarYEscalar(crudas, k.w, k.h, p.radio)
	f.mu.Lock()
	defer f.libre.Broadcast()
	if !f.vigente(k.id, p.trabajoFoto) {
		f.mu.Unlock()
		return
	}
	delete(f.preparando, k)
	if m == nil {
		// Los bytes pueden haber salido de la caché mientras esperaba su turno.
		if len(crudas) > 0 {
			f.fallidas[k.id] = true
		}
		f.mu.Unlock()
		return
	}
	if _, ya := f.pintadas[k]; !ya {
		f.pintadas[k], f.ordenP = m, append(f.ordenP, k)
		f.pixeles += k.w * k.h
		for f.pixeles > TopePixeles && len(f.ordenP) > 1 {
			vieja := f.ordenP[0]
			f.ordenP = f.ordenP[1:]
			f.pixeles -= vieja.w * vieja.h
			delete(f.pintadas, vieja)
		}
	}
	f.mu.Unlock()
	f.avisar(k.id)
}

// TeselaYa es Tesela sin segundo plano: prepara la tesela en ESTA goroutine y la
// devuelve. La usan los guardianes de píxel —que no tienen a nadie a quien esperar— y
// el visor, que enseña UNA foto y no puede empezar con el hueco en blanco.
func (f *Fototeca) TeselaYa(id string, w, h int, radio float32) *image.RGBA {
	if id == "" || w <= 0 || h <= 0 {
		return nil
	}
	k := clavePintada{id, w, h}
	f.mu.Lock()
	m, listo := f.pintadas[k]
	_, hay := f.crudas[id]
	f.mu.Unlock()
	if listo {
		return m
	}
	if !hay {
		f.pedir(id)
		return nil
	}
	f.preparar(k, radio)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pintadas[k]
}

// Precargar descarga una foto antes de que se vea: el visor pide así la anterior y la
// siguiente mientras se mira una, y pasar de foto ya no espera a la red (`precargar` en
// PhotosApp.tsx). Se queda en la caché de bytes, que es lo caro; la tesela a su tamaño
// se prepara al pintarla.
func (f *Fototeca) Precargar(id string) {
	f.mu.Lock()
	_, ya := f.crudas[id]
	f.mu.Unlock()
	if !ya {
		f.pedir(id)
	}
}

// pedir lanza UNA descarga por foto: sin el registro de las que van en vuelo, cada
// repintado mientras llega la primera lanzaría otra, y en una galería que se desplaza
// eso son cientos de peticiones por segundo.
func (f *Fototeca) pedir(id string) {
	if f.Pedir == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.enVuelo[id] || f.fallidas[id] {
		return
	}
	f.enVuelo[id] = true
	f.descargas = append(f.descargas, descargaFoto{id, f.trabajo(id)})
	if f.descargando < descargasSimultaneas {
		f.descargando++
		go f.descargarCola()
	}
}

func (f *Fototeca) descargarCola() {
	for {
		f.mu.Lock()
		if len(f.descargas) == 0 {
			f.descargas = nil
			f.descargando--
			f.libre.Broadcast()
			f.mu.Unlock()
			return
		}
		d := f.descargas[0]
		f.descargas[0] = descargaFoto{}
		f.descargas = f.descargas[1:]
		vigente := f.vigente(d.id, d.trabajoFoto)
		f.mu.Unlock()
		if vigente {
			f.descargar(d)
		}
	}
}

func (f *Fototeca) descargar(d descargaFoto) {
	variante := f.Variante
	if variante == "" {
		variante = VarianteMiniatura
	}
	datos, err := f.Pedir(d.ctx, d.id, variante)
	f.mu.Lock()
	defer f.libre.Broadcast()
	if !f.vigente(d.id, d.trabajoFoto) {
		f.mu.Unlock()
		return
	}
	delete(f.enVuelo, d.id)
	if err != nil || len(datos) == 0 {
		// Un error real no se reintenta en cada repintado. Cancelar no marca fallida.
		f.fallidas[d.id] = true
		f.mu.Unlock()
		return
	}
	f.guardarCrudas(d.id, datos)
	f.mu.Unlock()
	f.avisar(d.id)
}

// Esperar bloquea hasta que no quede nada por descargar ni por preparar.
//
// Existe para los guardianes de píxel: la tesela se prepara en otra goroutine, así que
// pintar y comparar en la misma línea compararía el hueco reservado. Un test que no
// espera pasa siempre, y eso es peor que no tenerlo. Fuera de un test no lo llama
// nadie: la interfaz no espera a una foto, la enseña cuando llega.
func (f *Fototeca) Esperar() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for len(f.enVuelo) > 0 || len(f.preparando) > 0 || f.descargando > 0 || f.preparandoAhora > 0 {
		f.libre.Wait()
	}
}

// Memoria es lo que ocupan las dos cachés, en bytes. Es para el informe y para el
// guardián del tope, no para decidir nada en caliente.
func (f *Fototeca) Memoria() (crudas, pintadas int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pesoC, f.pixeles * bytesPorPixe
}

// recortarYEscalar es `object-fit: cover` con las esquinas de `rounded-sm`: recorta el
// centro de la fuente a la proporción de la caja, remuestrea a la caja y borra las
// esquinas.
func recortarYEscalar(crudas []byte, w, h int, radio float32) *image.RGBA {
	src, _, err := image.Decode(bytes.NewReader(crudas))
	if err != nil {
		return nil
	}
	rec := recorteCover(src.Bounds(), w, h)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	escalador(rec, w, h).Scale(dst, dst.Bounds(), src, rec, draw.Src, nil)
	redondear(dst, radio)
	return dst
}

// # El núcleo de remuestreo: Catmull-Rom SIEMPRE, y por qué no la opción barata
//
// Aquí hubo una heurística —bilineal aproximado hasta ×2 de reducción y Catmull-Rom
// por encima— que parecía razonable y que la MEDICIÓN tumbó. Se deja escrito porque
// el razonamiento sigue sonando bien y volvería solo.
//
// La medida es la energía de aliasing: se reducen rayas verticales de UN píxel —la
// frecuencia más alta que cabe en una imagen, o sea el tejado, la verja o el estor— y
// se mira la desviación típica de lo que sale. Reducir rayas alternas tiene que dar
// gris uniforme, así que toda desviación es error. En niveles de 255:
//
//	512→402 (×1,27, tesela HiDPI)   ApproxBiLinear 73,89   BiLinear 36,32   CatmullRom 37,59
//	512→201 (×2,55, tesela normal)  ApproxBiLinear 73,89   BiLinear  6,48   CatmullRom  1,87
//
// `ApproxBiLinear` NO es un bilineal escalado: interpola los 2×2 más cercanos del
// ORIGEN sin filtrar, así que su error no depende de la razón y es el mismo a ×1,27
// que a ×2,55. La frontera en ×2 no existía. A ×2,55 es CUARENTA veces peor que
// Catmull-Rom.
//
// Cuesta 8,67 ms contra 3,11 (medido en el piloto, fuente de 512 px). Se paga: desde
// que la tesela se prepara en segundo plano, esos milisegundos no congelan nada, y
// este producto no puede permitirse que una foto con una verja salga con muaré.
//
// El `Scaler` se REUTILIZA por tamaño: con él la misma llamada baja a 7,33 ms y, sobre
// todo, de 6,7 MB de reservas a 40 KB. Veinticuatro teselas por pantalla eran 160 MB
// de basura por pantallazo.
func escalador(origen image.Rectangle, w, h int) xdraw.Scaler {
	k := claveEscalador{origen.Dx(), origen.Dy(), w, h}
	muEscaladores.Lock()
	defer muEscaladores.Unlock()
	if s, ok := escaladores[k]; ok {
		return s
	}
	// ponytail: se vacía entero al pasar del tope en vez de expulsar el más viejo.
	// Los tamaños distintos que hay en una sesión son un puñado —las variantes del
	// servicio por las dos o tres anchuras de tesela—, así que el tope no se toca
	// salvo al arrastrar el borde de la ventana, y ahí volver a construirlos cuesta
	// microsegundos.
	if len(escaladores) >= topeEscaladores {
		clear(escaladores)
	}
	s := xdraw.CatmullRom.NewScaler(w, h, origen.Dx(), origen.Dy())
	escaladores[k] = s
	return s
}

type claveEscalador struct{ sw, sh, dw, dh int }

const topeEscaladores = 32

var (
	muEscaladores sync.Mutex
	escaladores   = map[claveEscalador]xdraw.Scaler{}
)

// recorteCover es el rectángulo de la fuente que el navegador enseña con
// `object-fit: cover`: el mayor con la proporción de la caja, centrado.
func recorteCover(b image.Rectangle, w, h int) image.Rectangle {
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 || w <= 0 || h <= 0 {
		return b
	}
	// Se compara sw/sh con w/h sin dividir, para no perder precisión ni dividir por
	// cero: sw*h > w*sh quiere decir que la fuente es más apaisada que la caja.
	if sw*h > w*sh {
		nuevo := sh * w / h
		x := b.Min.X + (sw-nuevo)/2
		return image.Rect(x, b.Min.Y, x+nuevo, b.Max.Y)
	}
	nuevo := sw * h / w
	y := b.Min.Y + (sh-nuevo)/2
	return image.Rect(b.Min.X, y, b.Max.X, y+nuevo)
}

// redondear desvanece lo que queda fuera de un rectángulo redondeado: es el
// `overflow-hidden` + `rounded-sm` del `<li>` de la web, que Fyne no sabe hacer porque
// su recorte es RECTANGULAR (está en los techos del toolkit).
//
// Con antialias en el borde, como el navegador: la cobertura de un píxel es cuánto de
// él cae dentro del arco, muestreada en una rejilla de 4×4.
//
// # Se escalan los CUATRO canales, no solo el alfa. Visto en la captura
//
// `image.RGBA` es alfa PREMULTIPLICADO. Bajar solo el byte del alfa deja un píxel
// inválido —con R, G y B por encima de A— y la conversión de color de Go, que divide
// por el alfa para desmultiplicar, se DESBORDA: en la primera captura de la galería el
// arco de cada tesela salía como una línea casi negra (valores de 12, 58 y 76 sobre un
// fondo de 248) en vez de fundirse. No lo dijo ningún número: lo dijo mirar el PNG.
func redondear(m *image.RGBA, radio float32) {
	r := int(radio)
	if r <= 0 {
		return
	}
	w, h := m.Bounds().Dx(), m.Bounds().Dy()
	if 2*r > w {
		r = w / 2
	}
	if 2*r > h {
		r = h / 2
	}
	for _, esquina := range [4][2]int{{0, 0}, {w - r, 0}, {0, h - r}, {w - r, h - r}} {
		ox, oy := esquina[0], esquina[1]
		// El centro del arco de esta esquina, en coordenadas de la imagen.
		cx, cy := float32(r), float32(r)
		if ox > 0 {
			cx = float32(w - r)
		}
		if oy > 0 {
			cy = float32(h - r)
		}
		for y := oy; y < oy+r; y++ {
			for x := ox; x < ox+r; x++ {
				a := cobertura(float32(x), float32(y), cx, cy, float32(r))
				if a == 255 {
					continue
				}
				i := m.PixOffset(x, y)
				for c := range 4 {
					m.Pix[i+c] = uint8(uint32(m.Pix[i+c]) * uint32(a) / 255) //nolint:gosec // cabe: `a` y el canal son 0..255, así que el producto partido por 255 nunca pasa de 255
				}
			}
		}
	}
}

// cobertura es cuánto del píxel (x, y) cae dentro del círculo de centro (cx, cy) y
// radio r, en 0..255, muestreando 4×4.
func cobertura(x, y, cx, cy, r float32) uint8 {
	const n = 4
	dentro := 0
	for i := range n {
		for j := range n {
			dx := x + (float32(i)+0.5)/n - cx
			dy := y + (float32(j)+0.5)/n - cy
			if dx*dx+dy*dy <= r*r {
				dentro++
			}
		}
	}
	return uint8(dentro * 255 / (n * n))
}
