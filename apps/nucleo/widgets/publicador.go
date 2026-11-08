// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package widgets

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// # Publicar tras cambio
//
// Hasta el 2026-11-06 cada app publicaba su widget desde sus cargas de pantalla: con
// lo que la pantalla tuviera a la vista (el mes que se miraba, la carpeta abierta), en
// rutinas sueltas que podían pisarse, y nunca tras una mutación que no recargara esa
// pantalla. El Publicador es el mecanismo ÚNICO: cada app le da una Fuente que lee lo
// justo para el widget, independiente de la pantalla, y le avisa de que algo cambió. Él
// decide cuándo leer (antirrebote, una ronda a la vez) y escribe con PublishDetail
// (ADR 0078, enmienda del 2026-11-06).

// Fuente lee lo justo para el widget, independiente de la pantalla. Es una función
// libre: sin pantalla, sin estado de la interfaz, sin Fyne. Las de las quince apps
// están en apps/nucleo/widgets/fuentes. Recibe un contexto con plazo y con
// api.SinAviso: sus lecturas no cuentan como escrituras del producto.
type Fuente func(ctx context.Context) (Detail, error)

// OpcionesPublicador son las piezas del Publicador. Solo Vista y Fuente son
// obligatorias; el resto tiene los valores de producción.
type OpcionesPublicador struct {
	// Vista captura la sesión adoptada AL EMPEZAR cada ronda (Suite.CapturarWidget).
	Vista Source
	// Fuente lee los datos del widget.
	Fuente Fuente
	// Antirrebote: tras un aviso se espera esto sin otro aviso antes de leer.
	Antirrebote time.Duration
	// EsperaMax: una ráfaga continua de avisos publica como tarde a este plazo.
	EsperaMax time.Duration
	// Plazo de cada ronda: lectura y escritura.
	Plazo time.Duration
	// Latido: con la app viva, se vuelve a leer al menos cada tanto (y a la
	// medianoche local + 5 s, lo que llegue antes).
	Latido time.Duration
	// MinEscrituras es el intervalo mínimo entre rondas disparadas por escrituras
	// del propio producto (AvisarEscritura).
	MinEscrituras time.Duration
	// Reloj es la hora; nil, time.Now.
	Reloj func() time.Time
	// Programar ejecuta f en su propia rutina pasado d y devuelve cómo cancelarlo;
	// nil, time.AfterFunc. Las pruebas lo sustituyen por un reloj que avanzan a mano.
	Programar func(d time.Duration, f func()) (cancelar func())
}

// Valores de producción.
const (
	AntirreboteWidget   = 1500 * time.Millisecond
	EsperaMaxWidget     = 5 * time.Second
	PlazoWidget         = 20 * time.Second
	LatidoWidget        = 10 * time.Minute
	MinEscriturasWidget = 30 * time.Second
	// VentanaEco: una señal del servidor del propio producto que llega hasta este
	// plazo después de una escritura propia se toma por su eco. El Hub agrupa la
	// señal en ventanas de 3 s (hubnotify.VentanaCambios): la primera sale en el acto
	// y las demás al cerrar la ventana, así que el eco llega como tarde a los ~3 s más
	// la entrega; esto es el doble. Más allá, la señal es de otra persona o de otro
	// aparato y se lee tras el antirrebote, sin esperar al mínimo de las escrituras.
	VentanaEco = 6 * time.Second
)

func (o OpcionesPublicador) completas() OpcionesPublicador {
	if o.Antirrebote <= 0 {
		o.Antirrebote = AntirreboteWidget
	}
	if o.EsperaMax <= 0 {
		o.EsperaMax = EsperaMaxWidget
	}
	if o.Plazo <= 0 {
		o.Plazo = PlazoWidget
	}
	if o.Latido <= 0 {
		o.Latido = LatidoWidget
	}
	if o.MinEscrituras <= 0 {
		o.MinEscrituras = MinEscriturasWidget
	}
	if o.Reloj == nil {
		o.Reloj = time.Now
	}
	if o.Programar == nil {
		o.Programar = func(d time.Duration, f func()) func() {
			t := time.AfterFunc(d, f)
			return func() { t.Stop() }
		}
	}
	return o
}

// errOcupado: ya hay una ronda en curso; queda apuntada «otra vuelta».
var errOcupado = errors.New("widgets: ronda en curso")

// Publicador publica el widget de UNA app tras cada cambio. Es seguro entre rutinas
// y nada de lo que hace corre en el hilo de la interfaz.
//
//   - Una sola ronda a la vez. Un aviso durante una ronda deja apuntada otra, y solo
//     una, al acabar.
//   - La sesión se captura al EMPEZAR cada ronda; una vista privada no lee nada.
//   - «Sucio» se pone con cada aviso y solo lo quita una ronda que publicó con éxito
//     sin avisos posteriores a su comienzo. Republicar con sucio hace una ronda
//     completa: nunca se publica la caché de antes de una mutación.
//   - La caché del último Detail se guarda solo tras publicar con éxito, en memoria, con
//     la época de esa ronda; con otra época no vale.
//   - Autenticación rechazada: se borra el snapshot de esta app. Error de red: se
//     conserva lo publicado y se reintenta con el siguiente aviso.
//   - Pausar (salir, bloqueo) no es terminal: Reanudar vuelve a publicar.
type Publicador struct {
	o OpcionesPublicador
	// vida es el contexto de las rondas que dispara el propio Publicador (antirrebote,
	// latido, Republicar): no heredan el de nadie, cada una lleva su Plazo.
	vida context.Context

	mu                sync.Mutex
	pausado           bool
	sucio             bool
	avisos            uint64
	enCurso           bool
	otraVuelta        bool
	fin               chan struct{}
	rebote, latido    uint64
	primerAviso       time.Time
	cancelarRebote    func()
	cancelarLatido    func()
	cancelarEscritura func()
	escrituraEnEspera bool
	ultimaEscritura   time.Time
	// ultimaPropia es la última AvisarEscritura, haya disparado ronda o no: de ella
	// cuenta VentanaEco.
	ultimaPropia time.Time
	cache        *Detail
	cacheEpoca   string
}

// NuevoPublicador crea un publicador parado en el sentido de que no lee hasta el
// primer Avisar o Reanudar.
func NuevoPublicador(o OpcionesPublicador) *Publicador {
	return &Publicador{o: o.completas(), vida: context.Background(), sucio: true}
}

// Avisar dice que algo ha cambiado: se lee tras el antirrebote.
func (p *Publicador) Avisar() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.avisarLocked()
}

// AvisarEscritura es Avisar para una escritura del propio producto, con el intervalo
// mínimo entre rondas: la primera tras la calma usa el antirrebote normal, y las
// siguientes dentro de la ventana se agrupan en una ronda al final de ella.
func (p *Publicador) AvisarEscritura() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ultimaPropia = p.o.Reloj()
	p.avisarEscrituraLocked()
}

// AvisarDelServidor es Avisar para lo que el servidor dice del PROPIO producto: su
// señal de cambios (`datos`) o un aviso del Hub. La señal le llega también a quien
// escribe —el servidor no sabe desde qué aparato—, así que justo detrás de una
// escritura propia es casi siempre su eco: dentro de VentanaEco de la última escritura
// propia cuenta como AvisarEscritura y respeta el mínimo; fuera de ella es Avisar, y
// el cambio de otra persona o de otro aparato no espera al final de la ventana de
// MinEscrituras. Sin esto el autoguardado de una nota (una escritura cada pocos
// segundos) hacía una ronda por eco en vez de dos por minuto. Lo que no se puede
// distinguir es el cambio ajeno que llega dentro de VentanaEco de una escritura
// propia: ese espera al final de la ventana de MinEscrituras, como mucho 30 s.
func (p *Publicador) AvisarDelServidor() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.ultimaPropia.IsZero() && p.o.Reloj().Sub(p.ultimaPropia) < VentanaEco {
		p.avisarEscrituraLocked()
		return
	}
	p.avisarLocked()
}

func (p *Publicador) avisarEscrituraLocked() {
	if p.pausado {
		return
	}
	ahora := p.o.Reloj()
	if p.escrituraEnEspera {
		p.marcarLocked()
		return
	}
	if p.ultimaEscritura.IsZero() || ahora.Sub(p.ultimaEscritura) >= p.o.MinEscrituras {
		p.ultimaEscritura = ahora
		p.avisarLocked()
		return
	}
	p.marcarLocked()
	p.escrituraEnEspera = true
	p.cancelarEscritura = p.o.Programar(p.ultimaEscritura.Add(p.o.MinEscrituras).Sub(ahora), func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if !p.escrituraEnEspera {
			return
		}
		p.escrituraEnEspera, p.cancelarEscritura = false, nil
		p.ultimaEscritura = p.o.Reloj()
		p.avisarLocked()
	})
}

// Republicar es para cuando la marca de sesión cambia sin cambiar quién (una
// renovación): sin red si la caché está limpia y es de la misma época; si no, una
// ronda completa.
func (p *Publicador) Republicar() {
	p.mu.Lock()
	if p.pausado {
		p.mu.Unlock()
		return
	}
	if p.sucio || p.cache == nil {
		p.avisarLocked()
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	p.o.Programar(0, func() { _ = p.ronda(p.vida, true) })
}

// RefrescarYa hace una ronda completa y espera a que acabe. Es para el segundo plano,
// donde el sistema concede un plazo y quiere saber el resultado.
func (p *Publicador) RefrescarYa(ctx context.Context) error {
	for {
		p.mu.Lock()
		if p.pausado {
			p.mu.Unlock()
			return ErrPrivate
		}
		if p.enCurso {
			fin := p.fin
			p.mu.Unlock()
			select {
			case <-fin:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		p.marcarLocked()
		p.mu.Unlock()
		if err := p.ronda(ctx, false); !errors.Is(err, errOcupado) {
			return err
		}
	}
}

// PlazoAlFondo es lo que se da a PublicarPendiente al pasar al fondo: iOS concede
// unos pocos segundos a una app que sale a la pantalla de inicio antes de suspenderla.
const PlazoAlFondo = 4 * time.Second

// PublicarPendiente es para cuando la app deja el primer plano. Si hay algo sin
// publicar —un antirrebote corriendo, una escritura esperando al final de su ventana
// de MinEscrituras, una ronda que falló—, lo publica YA, sin esperar ni al antirrebote
// ni a esa ventana, y espera a que acabe dentro de `ctx`. Sin nada pendiente no lee
// nada. Es justo el momento en que la persona mira el widget: iOS suspende la app
// segundos después, y lo que estaba aplazado no llegaría hasta volver a abrirla.
func (p *Publicador) PublicarPendiente(ctx context.Context) error {
	p.mu.Lock()
	if p.pausado {
		p.mu.Unlock()
		return nil
	}
	if p.enCurso {
		// La ronda en curso quita «sucio» si nada llegó después de empezar; si llegó
		// algo, se publica a continuación.
		fin := p.fin
		p.mu.Unlock()
		select {
		case <-fin:
		case <-ctx.Done():
			return ctx.Err()
		}
		p.mu.Lock()
	}
	if p.pausado || !p.sucio {
		p.mu.Unlock()
		return nil
	}
	p.rebote++
	for _, cancelar := range []func(){p.cancelarRebote, p.cancelarEscritura} {
		if cancelar != nil {
			cancelar()
		}
	}
	p.cancelarRebote, p.cancelarEscritura = nil, nil
	if p.escrituraEnEspera {
		p.escrituraEnEspera, p.ultimaEscritura = false, p.o.Reloj()
	}
	p.primerAviso = time.Time{}
	p.mu.Unlock()
	return p.RefrescarYa(ctx)
}

// RefrescarEnFondo es el refresco que pide el sistema con la app en segundo plano (la
// tarea de refresco de iOS). Si ya hay algo apuntado o una ronda en curso —al lanzar la
// app en frío para esto, la del Reanudar del arranque—, publica eso ya y basta
// (PublicarPendiente): una lectura, no dos. Si no, una ronda completa, que es a lo que
// viene: lo del servidor puede haber cambiado sin que nadie avisara.
func (p *Publicador) RefrescarEnFondo(ctx context.Context) error {
	p.mu.Lock()
	pendiente := p.sucio || p.enCurso
	p.mu.Unlock()
	if pendiente {
		return p.PublicarPendiente(ctx)
	}
	return p.RefrescarYa(ctx)
}

// Pausar para los temporizadores y olvida la caché: al salir o al poner el bloqueo.
// No es terminal.
func (p *Publicador) Pausar() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pausado = true
	p.rebote++
	p.latido++
	for _, cancelar := range []func(){p.cancelarRebote, p.cancelarLatido, p.cancelarEscritura} {
		if cancelar != nil {
			cancelar()
		}
	}
	p.cancelarRebote, p.cancelarLatido, p.cancelarEscritura = nil, nil, nil
	p.escrituraEnEspera, p.otraVuelta = false, false
	p.primerAviso = time.Time{}
	p.cache, p.cacheEpoca, p.sucio = nil, "", true
}

// Reanudar vuelve a publicar tras Pausar (entrar, bloqueo quitado). La primera ronda
// es completa.
func (p *Publicador) Reanudar() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pausado = false
	p.cache, p.cacheEpoca = nil, ""
	p.avisarLocked()
}

func (p *Publicador) marcarLocked() {
	p.avisos++
	p.sucio = true
}

func (p *Publicador) avisarLocked() {
	if p.pausado {
		return
	}
	p.marcarLocked()
	if p.enCurso {
		p.otraVuelta = true
		return
	}
	p.programarLocked(p.o.Antirrebote)
}

func (p *Publicador) programarLocked(d time.Duration) {
	ahora := p.o.Reloj()
	if p.primerAviso.IsZero() {
		p.primerAviso = ahora
	}
	if limite := p.primerAviso.Add(p.o.EsperaMax).Sub(ahora); d > limite {
		d = max(limite, 0)
	}
	if p.cancelarRebote != nil {
		p.cancelarRebote()
	}
	p.rebote++
	n := p.rebote
	p.cancelarRebote = p.o.Programar(d, func() {
		p.mu.Lock()
		if n != p.rebote || p.pausado {
			p.mu.Unlock()
			return
		}
		p.cancelarRebote, p.primerAviso = nil, time.Time{}
		p.mu.Unlock()
		_ = p.ronda(p.vida, false)
	})
}

// armarLatidoLocked programa el siguiente aviso propio: el latido o la medianoche
// local + 5 s, lo que llegue antes (a medianoche, «hoy» cambia sin que nadie toque nada).
func (p *Publicador) armarLatidoLocked() {
	if p.cancelarLatido != nil {
		p.cancelarLatido()
	}
	ahora := p.o.Reloj()
	d := p.o.Latido
	manana := time.Date(ahora.Year(), ahora.Month(), ahora.Day()+1, 0, 0, 5, 0, ahora.Location())
	if hasta := manana.Sub(ahora); hasta > 0 && hasta < d {
		d = hasta
	}
	p.latido++
	n := p.latido
	p.cancelarLatido = p.o.Programar(d, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if n != p.latido {
			return
		}
		p.cancelarLatido = nil
		p.avisarLocked()
	})
}

// ronda captura, lee (o toma la caché) y publica. Solo una a la vez.
func (p *Publicador) ronda(ctx context.Context, permitirCache bool) error {
	p.mu.Lock()
	if p.pausado {
		p.mu.Unlock()
		return ErrPrivate
	}
	if p.enCurso {
		p.otraVuelta = true
		p.mu.Unlock()
		return errOcupado
	}
	p.enCurso, p.fin = true, make(chan struct{})
	avisos := p.avisos
	var cache *Detail
	var epocaCache string
	if permitirCache && !p.sucio {
		cache, epocaCache = p.cache, p.cacheEpoca
	}
	p.mu.Unlock()

	ctx, parar := context.WithTimeout(ctx, p.o.Plazo)
	defer parar()
	vista := p.o.Vista.View()
	epoca := vista.scope.generation
	var detalle Detail
	var err error
	switch {
	case vista.bundle == "" || epoca == "":
		err = ErrPrivate // sin sesión o sin identidad adoptada: no se lee nada
	case cache != nil && epocaCache == epoca:
		detalle = clonarDetalle(*cache)
	case p.o.Fuente == nil:
		err = ErrInvalid
	default:
		detalle, err = p.o.Fuente(api.SinAviso(ctx))
	}
	if err == nil {
		err = PublishDetail(ctx, vista.bundle, vista.product, vista.scope, detalle, Vigencia)
	}
	rechazada := AutenticacionRechazada(err)
	if rechazada && vista.bundle != "" {
		// El snapshot de ESTA app; la época de la suite no se toca por un rechazo
		// puntual (ADR 0078, enmienda del 2026-11-06).
		_ = ClearBundleIf(ctx, vista.bundle, epoca)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.enCurso = false
	close(p.fin)
	if err == nil && !p.pausado {
		guardado := clonarDetalle(detalle)
		p.cache, p.cacheEpoca = &guardado, epoca
		if p.avisos == avisos {
			p.sucio = false
			p.anularEsperaCubiertaLocked()
		}
	} else {
		p.sucio = true
		if rechazada {
			p.cache, p.cacheEpoca = nil, ""
		}
	}
	// Lo siguiente se programa con la vida del Publicador, no con la de esta ronda: la
	// ronda acaba aquí y su ctx (quizá el de RefrescarYa) muere con ella.
	if !p.pausado {
		if p.otraVuelta {
			p.otraVuelta = false
			p.programarLocked(p.o.Antirrebote)
		}
		p.armarLatidoLocked()
	}
	return err
}

// anularEsperaCubiertaLocked es para una ronda que publicó y empezó después de todo lo
// apuntado, también de las escrituras que esperaban al final de su ventana de
// MinEscrituras (y de su eco): ya están publicadas, y la ronda de esa espera leería lo
// mismo otra vez. El mínimo se conserva: ultimaEscritura no cambia.
func (p *Publicador) anularEsperaCubiertaLocked() {
	if !p.escrituraEnEspera {
		return
	}
	if p.cancelarEscritura != nil {
		p.cancelarEscritura()
	}
	p.escrituraEnEspera, p.cancelarEscritura = false, nil
}

// AutenticacionRechazada dice si el servidor ya no acepta la sesión: el refresh
// rechazado (auth.ErrLoginRequired) o un 401 que sigue tras renovar.
func AutenticacionRechazada(err error) bool {
	if errors.Is(err, auth.ErrLoginRequired) {
		return true
	}
	var e *api.Error
	return errors.As(err, &e) && e.Status == 401
}

func clonarDetalle(d Detail) Detail {
	return Detail{
		Counters: slices.Clone(d.Counters),
		Items:    slices.Clone(d.Items),
		Calendar: cloneCalendar(d.Calendar),
	}
}
