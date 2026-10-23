// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"context"
	"image"
	"slices"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/fyne/video"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # La calidad al reproducir
//
// El menú de calidad del reproductor, como el de la web: «Automática» y las calidades
// que existen (los WebM de webm_qualities). Cambiar de calidad abre el WebM de la otra
// calidad, busca por sus Cues el instante en que iba el que suena, y cambia uno por
// otro conservando la posición, la pausa, el volumen y el silencio.
//
// «Automática» elige por el TAMAÑO del reproductor en píxeles FÍSICOS —no tiene
// sentido bajar 1080p para un cuadro de 360 px de alto, ni enseñar 360p a pantalla
// completa en un monitor de 1440— y, como freno, por el RITMO de descarga: si la red
// no da para la calidad que toca, baja. Se vuelve a decidir al cambiar de tamaño
// (redimensionar, pantalla completa, la ventana de imagen dentro de imagen) y cada
// pocos segundos por si la red cambia, sin saltar más de una vez cada `pausaAuto`.

// pausaAuto es lo mínimo entre dos cambios de «Automática»: cada cambio reabre el
// fichero y rellena el búfer, y cambiar en bucle se ve peor que no cambiar.
const pausaAuto = 10 * time.Second

// margenRed es cuánto por encima de la tasa del vídeo ha de ir la red para no bajar.
const margenRed = 1.3

// DecidirAuto es la calidad que elige «Automática»: la menor de las que hay que cubre
// el alto del cuadro en píxeles físicos (con un 10 % de holgura: un 1080p se ve bien en
// un cuadro de 1150), o la mayor si ninguna llega. Después, si se sabe el ritmo de la
// red (kbps > 0) y la tasa de la calidad que suena (`actual` a `kbpsActual`), baja
// mientras la elegida no quepa con margen. La tasa de otra calidad se estima por los
// píxeles (∝ alto²), que la sobrestima al subir: el error va del lado prudente.
func DecidirAuto(altoFisico float32, calidades []int, kbpsRed float64, actual int, kbpsActual float64) int {
	qs := Ordenadas(calidades)
	if len(qs) == 0 {
		return 0
	}
	elegida := qs[0]
	for _, q := range qs {
		if float32(q)*1.1 >= altoFisico {
			elegida = q
		}
	}
	if kbpsRed <= 0 || kbpsActual <= 0 || actual <= 0 {
		return elegida
	}
	tasa := func(q int) float64 { r := float64(q) / float64(actual); return kbpsActual * r * r }
	i := slices.Index(qs, elegida)
	for i < len(qs)-1 && tasa(qs[i])*margenRed > kbpsRed {
		i++
	}
	return qs[i]
}

// fuenteMedida cuenta lo que se lee y el tiempo que se pasa esperándolo: el ritmo de
// descarga de la red, aproximado (lo que ya estaba en el búfer del sistema sale gratis
// y lo sobrestima, así que solo sirve para BAJAR cuando la red claramente no llega).
type fuenteMedida struct {
	Fuente
	mu       sync.Mutex
	bytes    int64
	esperado time.Duration
}

func (m *fuenteMedida) Read(p []byte) (int, error) {
	t := time.Now()
	n, err := m.Fuente.Read(p)
	dt := time.Since(t)
	m.mu.Lock()
	m.bytes += int64(n)
	m.esperado += dt
	// Una ventana deslizante a lo bruto: pasados 8 MB se queda con la mitad.
	if m.bytes > 8<<20 {
		m.bytes, m.esperado = m.bytes/2, m.esperado/2
	}
	m.mu.Unlock()
	return n, err
}

// Kbps es el ritmo medido; 0 si aún no hay bastante para decir nada.
func (m *fuenteMedida) Kbps() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bytes < 256<<10 || m.esperado < 50*time.Millisecond {
		return 0
	}
	return float64(m.bytes) * 8 / m.esperado.Seconds() / 1000
}

// tasaFichero es la tasa media del WebM que suena (tamaño entre duración), en kbps.
func tasaFichero(f Fuente, d time.Duration) float64 {
	s, ok := f.(interface{ Size() int64 })
	if !ok || d <= 0 {
		return 0
	}
	return float64(s.Size()) * 8 / d.Seconds() / 1000
}

// ---- En la app ----

// abrirWebM abre un WebM por rangos, con su medidor, y crea su reproductor (en pausa y
// sin arrancar). Fuera del hilo de la interfaz.
func (a *App) abrirWebM(ruta string) (*fuenteMedida, *video.Reproductor, error) {
	var f Fuente
	var err error
	if a.AbrirMedia != nil {
		f, err = a.AbrirMedia(context.Background(), ruta)
	} else if a.cli != nil {
		f, err = a.cli.Media(context.Background(), ruta)
	}
	if err != nil {
		return nil, nil, err
	}
	m := &fuenteMedida{Fuente: f}
	r, err := video.Nuevo(m, a.Audio)
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return m, r, nil
}

// deVer es el reproductor del vídeo de la página: el suyo o, si la ventana de imagen
// dentro de imagen es de ese vídeo, el de la ventana.
func (a *App) deVer() *video.Reproductor {
	if a.reproductor != nil {
		return a.reproductor
	}
	if a.pip != nil && a.pip.id == a.ver.id {
		return a.pip.r
	}
	return nil
}

// calidadesVer son las calidades del vídeo que se ve, de mayor a menor.
func (a *App) calidadesVer() []int {
	if a.ver.play == nil {
		return nil
	}
	return Ordenadas(a.ver.play.Calidades())
}

// alturaFisica es el alto, en píxeles de la pantalla, de donde se enseña el vídeo: la
// ventana de imagen dentro de imagen si está abierta, si no el cuadro del visor.
func (a *App) alturaFisica() float32 {
	escala := float32(1)
	var alto float32
	switch {
	case a.pip != nil && a.pip.id == a.ver.id:
		escala = a.pip.w.Canvas().Scale()
		alto = a.pip.w.Canvas().Size().Height - altoBarraPiP
	case a.visor != nil:
		if a.w != nil {
			escala = a.w.Canvas().Scale()
		}
		alto = a.visor.Medido().Height
	}
	return alto * escala
}

// objetivoAuto es lo que elige «Automática» ahora mismo.
func (a *App) objetivoAuto() int {
	qs := a.calidadesVer()
	if len(qs) == 0 {
		return 0
	}
	alto := a.alturaFisica()
	if alto <= 0 {
		// Aún sin medir: la de siempre (webm_url).
		return a.ver.enUso
	}
	var red, tasa float64
	if a.ver.medida != nil {
		red = a.ver.medida.Kbps()
		if r := a.deVer(); r != nil {
			tasa = tasaFichero(a.ver.medida.Fuente, r.Duracion())
		}
	}
	return DecidirAuto(alto, qs, red, a.ver.enUso, tasa)
}

// reevaluarAuto vuelve a decidir con «Automática» elegida; si toca otra calidad y ha
// pasado bastante desde el último cambio, cambia.
func (a *App) reevaluarAuto() {
	if a.ver.calidad != 0 || a.deVer() == nil || a.ver.cambiando {
		return
	}
	q := a.objetivoAuto()
	if q == 0 || q == a.ver.enUso || time.Since(a.ver.ultimoAuto) < pausaAuto {
		return
	}
	a.ver.ultimoAuto = time.Now()
	a.ponerCalidad(q)
}

// medidoVisor es AlMedir del visor: al cambiar de tamaño, «Automática» vuelve a
// decidir, un momento después (una ventana que se arrastra manda decenas de tamaños).
func (a *App) medidoVisor(fyne.Size) {
	if a.ver.esperaAuto != nil {
		a.ver.esperaAuto.Stop()
	}
	gen := a.gen
	a.ver.esperaAuto = time.AfterFunc(600*time.Millisecond, func() {
		a.hilo(gen, func() {
			a.ver.ultimoAuto = time.Time{} // cambiar de tamaño es motivo de sobra
			a.reevaluarAuto()
		})
	})
}

// menuCalidad abre el menú colgado de su botón: «Automática» (con la que ha elegido) y
// las calidades, con un tic en la elegida.
func (a *App) menuCalidad(disparador fyne.CanvasObject) {
	qs := a.calidadesVer()
	if len(qs) == 0 {
		return
	}
	auto := a.v.Traduce("player.qualityAuto")
	if a.ver.calidad == 0 && a.ver.enUso > 0 {
		auto = a.v.Traduce("player.qualityAutoNow", i18n.Vars{"quality": Etiqueta(a.ver.enUso)})
	}
	elementos := []fyne.CanvasObject{}
	e := ui.NuevoElementoMenu(auto, 0, func() { a.elegirCalidad(0) })
	e.Marcada = a.ver.calidad == 0
	elementos = append(elementos, e, ui.NuevoSeparadorMenu())
	for _, q := range qs {
		e := ui.NuevoElementoMenu(Etiqueta(q), 0, func() { a.elegirCalidad(q) })
		e.Marcada = a.ver.calidad == q
		elementos = append(elementos, e)
	}
	m := ui.NuevoMenu(elementos...)
	m.Rotulo, m.Arriba = a.v.Traduce("player.quality"), true
	m.Mostrar(disparador)
}

// elegirCalidad es lo que hace el menú: una calidad fija, o 0 para «Automática».
func (a *App) elegirCalidad(q int) {
	a.ver.calidad = q
	objetivo := q
	if q == 0 {
		objetivo = a.objetivoAuto()
		a.ver.ultimoAuto = time.Now()
	}
	if objetivo > 0 {
		a.ponerCalidad(objetivo)
	}
	if a.visor != nil {
		a.visor.Rehacer()
	}
}

// ponerCalidad cambia el WebM que suena por el de la calidad `q`, conservando dónde
// iba y si sonaba. El nuevo se abre y se busca mientras el viejo sigue; el cambio es
// al final, de golpe.
func (a *App) ponerCalidad(q int) {
	viejo := a.deVer()
	if q == a.ver.enUso || a.ver.play == nil || viejo == nil || a.ver.cambiando {
		return
	}
	ruta := a.ver.play.WebMURLDe(q)
	if ruta == "" {
		return
	}
	a.ver.cambiando = true
	pos := viejo.Posicion()
	a.lanzar(func() {
		f, r, err := a.abrirWebM(ruta)
		if err == nil {
			r.Buscar(pos) // por sus Cues: el primer fotograma que se entrega ya es el de `pos`
		}
		a.enLaInterfaz(func() {
			if a.deVer() != viejo {
				// Se cerró o se cambió entretanto: esto ya no es de nadie.
				if err == nil {
					r.Cerrar()
					_ = f.Close()
				}
				return
			}
			a.ver.cambiando = false
			if err != nil {
				a.avisar(ui.AvisoPeligro, a.v.Traduce("player.error"))
				return
			}
			a.relevar(viejo, f, r, q)
		})
	})
}

// relevar pone el reproductor nuevo en el sitio del viejo: mismo instante (lo que
// avanzó mientras se abría el otro también), misma pausa, volumen y silencio.
func (a *App) relevar(viejo *video.Reproductor, f *fuenteMedida, r *video.Reproductor, q int) {
	pausado, silencio := viejo.Pausado(), viejo.Silenciado()
	if ahora := viejo.Posicion(); !pausado {
		r.Buscar(ahora)
	}
	r.Silenciar(silencio)
	vol := 1.0
	if a.visor != nil {
		vol = a.visor.volumen
	}
	r.Volumen(vol)
	// El viejo se cierra ANTES de que el nuevo abra la tarjeta de sonido: dos a la vez
	// sonarían encima el uno del otro un instante.
	viejo.Cerrar()
	if a.pip != nil && a.pip.r == viejo {
		_ = a.pip.f.Close()
		a.pip.r, a.pip.f = r, f
	} else {
		if a.fuente != nil {
			_ = a.fuente.Close()
		}
		a.reproductor, a.fuente = r, f
	}
	a.ver.medida, a.ver.enUso = f, q
	a.enchufar(r)
	r.Arrancar()
	if !pausado {
		r.Reproducir()
	}
}

// enchufar lleva los fotogramas y los cambios de estado del reproductor a donde se
// enseñen en cada momento (fotograma lo decide al llegar cada uno). Una vez, ANTES de
// Arrancar: las rutinas del reproductor leen estos campos.
func (a *App) enchufar(r *video.Reproductor) {
	r.AlFotograma = func(img image.Image, _ time.Duration) {
		a.enLaInterfaz(func() { a.fotograma(r, img) })
	}
	r.AlCambiar = func() {
		a.enLaInterfaz(func() {
			if a.pip != nil && a.pip.r == r {
				a.pip.rehacer()
			}
			if a.visor != nil && a.visor.Mando == r {
				a.visor.Rehacer()
			}
			a.actualizarEncendido()
		})
	}
	a.conectar(r)
}

// conectar pone los mandos del visor sobre el reproductor: el de la página, o el de
// la ventana si es del vídeo de la página.
func (a *App) conectar(r *video.Reproductor) {
	enPiP := a.pip != nil && a.pip.r == r
	if a.visor != nil && (!enPiP || (a.ruta == RutaVer && a.pip.id == a.ver.id)) {
		a.visor.Mando = r
		a.visor.Rehacer()
	}
}

// fotograma pinta una imagen del reproductor `r` donde toca; si `r` ya no es el que
// suena, se tira.
func (a *App) fotograma(r *video.Reproductor, img image.Image) {
	switch {
	case a.pip != nil && a.pip.r == r:
		a.pip.imagen.Image = img
		a.pip.imagen.Refresh()
	case a.reproductor == r && a.visor != nil:
		a.visor.Imagen(img)
	}
}
