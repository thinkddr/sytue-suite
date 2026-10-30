// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

type autorizadorMedios interface {
	CallEncryption(context.Context, string, string, string) (api.RoomsCallEncryption, error)
	CallEncryptionReady(context.Context, string, string, string, int64) error
	CallEncryptionReconnect(context.Context, string, string, string, int64) (api.RoomsCall, error)
}

type clavesDeLlamada interface {
	Start(context.Context) error
	CurrentKey() (string, time.Time, bool)
	Close()
}

type (
	conectarMedios    func(string, string, llamada.Opciones) (EnCurso, error)
	crearClavesMedios func(context.Context, api.RoomsCallEncryption) (clavesDeLlamada, error)
)

// llamadaCifrada retains one join nonce across SFU epochs. Media is disconnected
// before replacing a key; only the verified Olm transport ever carries that key.
// The proxy keeps the existing call view and the person's device choices alive.
type llamadaCifrada struct {
	mu                      sync.Mutex
	ctx                     context.Context
	cancel                  context.CancelFunc
	api                     autorizadorMedios
	crear                   crearClavesMedios
	conectar                conectarMedios
	call                    api.RoomsCall
	device                  string
	opciones                llamada.Opciones
	actual                  EnCurso
	claves                  clavesDeLlamada
	binding                 api.RoomsCallEncryption
	ciclo                   uint64
	cerrada                 bool
	micro, camara, pantalla bool
	desconexion             chan uint64
	intervalo               time.Duration
	caducar                 *time.Timer
}

func (c *Cifrado) ConectarLlamada(ctx context.Context, call api.RoomsCall, opciones llamada.Opciones, conectar conectarMedios) (EnCurso, error) {
	client, err := c.Cliente(ctx)
	if err != nil {
		return nil, err
	}
	if call.Identity != client.Subject() {
		return nil, mx.ErrIdentity
	}
	return nuevaLlamadaCifrada(ctx, c.API, client.DeviceID(), call, opciones, conectar,
		func(ctx context.Context, binding api.RoomsCallEncryption) (clavesDeLlamada, error) {
			return client.NewCallKeys(ctx, binding)
		}, 5*time.Second)
}

func nuevaLlamadaCifrada(ctx context.Context, servidor autorizadorMedios, device string, call api.RoomsCall, opciones llamada.Opciones, conectar conectarMedios, crear crearClavesMedios, intervalo time.Duration) (*llamadaCifrada, error) {
	if servidor == nil || conectar == nil || crear == nil || device == "" || call.Identity == "" || call.JoinID == "" || len(call.JoinID) > 255 || call.Room.ID == "" || call.Encryption == nil || intervalo <= 0 {
		return nil, mx.ErrIdentity
	}
	lifetime, cancel := context.WithCancel(ctx)
	l := &llamadaCifrada{ctx: lifetime, cancel: cancel, api: servidor, crear: crear, conectar: conectar, call: call, device: device, opciones: opciones, micro: opciones.Micro, desconexion: make(chan uint64, 1), intervalo: intervalo}
	context.AfterFunc(lifetime, l.Colgar)
	if err := l.ajustar(lifetime, *call.Encryption); err != nil {
		l.Colgar()
		return nil, err
	}
	go l.vigilar(lifetime)
	return l, nil
}

func (l *llamadaCifrada) ajustar(ctx context.Context, binding api.RoomsCallEncryption) error {
	for range 3 {
		err := l.reconectar(ctx, binding)
		var respuesta *api.Error
		if !errors.As(err, &respuesta) || respuesta.Code() != "call_encryption_changed" {
			return err
		}
		binding, err = l.api.CallEncryption(ctx, l.call.Room.ID, l.device, l.call.JoinID)
		if err != nil {
			return err
		}
	}
	return mx.ErrNotReady
}

func (l *llamadaCifrada) validar(binding api.RoomsCallEncryption) error {
	if binding.CallID != l.call.Room.ID || binding.MatrixRoomID == "" || binding.LiveKitRoom == "" || binding.Generation < l.binding.Generation || binding.ExpiresAtMS <= time.Now().UnixMilli() {
		return mx.ErrIdentity
	}
	if l.binding.CallID != "" && binding.Generation == l.binding.Generation && !reflect.DeepEqual(binding, l.binding) {
		return mx.ErrIdentity // an epoch, including its lease and roster, is immutable
	}
	return nil
}

func (l *llamadaCifrada) pausar() uint64 {
	l.mu.Lock()
	l.ciclo++
	ciclo := l.ciclo
	if l.caducar != nil {
		l.caducar.Stop()
		l.caducar = nil
	}
	actual := l.actual
	l.actual = nil
	l.mu.Unlock()
	if actual != nil {
		actual.Colgar()
	}
	return ciclo
}

func (l *llamadaCifrada) cerrarClaves() {
	l.mu.Lock()
	keys := l.claves
	l.claves = nil
	l.mu.Unlock()
	if keys != nil {
		keys.Close()
	}
}

// Waiting also reconciles the roster: another participant can rotate the epoch
// before the leader has delivered a key. Requests are bounded to fifteen seconds.
func (l *llamadaCifrada) preparar(ctx context.Context, binding api.RoomsCallEncryption) (string, time.Time, error) {
	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		keys, err := l.instalarClaves(ctx, binding)
		if err != nil {
			return "", time.Time{}, err
		}
		if err := keys.Start(waitCtx); err != nil {
			return "", time.Time{}, err
		}
		key, until, next, err := l.esperarClave(waitCtx, keys, binding, ticker.C)
		if err != nil || next == nil {
			return key, until, err
		}
		keys.Close()
		binding = *next
	}
}

func (l *llamadaCifrada) instalarClaves(ctx context.Context, binding api.RoomsCallEncryption) (clavesDeLlamada, error) {
	if err := l.validar(binding); err != nil {
		return nil, err
	}
	// Use the call lifetime, not the shorter negotiation timeout, for key storage.
	keys, err := l.crear(ctx, binding)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cerrada || ctx.Err() != nil {
		keys.Close()
		return nil, context.Canceled
	}
	l.claves = keys
	l.binding = binding
	return keys, nil
}

func (l *llamadaCifrada) esperarClave(ctx context.Context, keys clavesDeLlamada, binding api.RoomsCallEncryption, ticks <-chan time.Time) (string, time.Time, *api.RoomsCallEncryption, error) {
	for attempts := 0; ; attempts++ {
		if key, until, ready := keys.CurrentKey(); ready {
			return key, until, nil, nil
		}
		select {
		case <-ctx.Done():
			return "", time.Time{}, nil, ctx.Err()
		case <-ticks:
		}
		if attempts%5 == 0 {
			next, err := l.api.CallEncryption(ctx, l.call.Room.ID, l.device, l.call.JoinID)
			if err != nil {
				return "", time.Time{}, nil, err
			}
			if !reflect.DeepEqual(next, binding) {
				return "", time.Time{}, &next, nil
			}
		}
		if _, _, ready := keys.CurrentKey(); !ready {
			if err := keys.Start(ctx); err != nil {
				return "", time.Time{}, nil, err
			}
		}
	}
}

func (l *llamadaCifrada) clave(ctx context.Context, binding api.RoomsCallEncryption) (string, time.Time, error) {
	l.mu.Lock()
	keys := l.claves
	l.mu.Unlock()
	if keys != nil && reflect.DeepEqual(binding, l.binding) {
		key, until, ready := keys.CurrentKey()
		if !ready {
			return "", time.Time{}, llamada.ErrSinClaveMedios
		}
		return key, until, nil
	}
	l.cerrarClaves()
	return l.preparar(ctx, binding)
}

func (l *llamadaCifrada) protegerPlazo(ciclo uint64, until time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cerrada || l.ctx.Err() != nil || !until.After(time.Now()) {
		return mx.ErrClosed
	}
	l.caducar = time.AfterFunc(time.Until(until), func() {
		if l.cerrar(ciclo) && l.opciones.AlSalir != nil {
			l.opciones.AlSalir("encryption_expired")
		}
	})
	return nil
}

func (l *llamadaCifrada) reconectar(ctx context.Context, binding api.RoomsCallEncryption) error {
	ciclo := l.pausar()
	key, until, err := l.clave(ctx, binding)
	if err != nil {
		return err
	}
	if err := l.protegerPlazo(ciclo, until); err != nil {
		return err
	}
	// Reconnect never creates another join or rotates the roster itself.
	next, err := l.api.CallEncryptionReconnect(ctx, l.call.Room.ID, l.device, l.call.JoinID, l.binding.Generation)
	if err != nil {
		return err
	}
	if next.Identity != l.call.Identity || next.JoinID != l.call.JoinID || next.Room.ID != l.call.Room.ID || next.Encryption == nil || !reflect.DeepEqual(*next.Encryption, l.binding) || next.LiveKitURL == "" || next.Token == "" || ctx.Err() != nil {
		return mx.ErrIdentity
	}
	return l.conectarEpoca(ctx, next, ciclo, key, until)
}

func (l *llamadaCifrada) opcionesMedios(ciclo uint64, key string, until time.Time) llamada.Opciones {
	l.mu.Lock()
	opciones := l.opciones
	l.mu.Unlock()
	opciones.RequiereCifrado, opciones.Cifrado, opciones.Micro = true, &llamada.CifradoMedios{Clave: key, Caduca: until}, false
	vigente := func() bool {
		l.mu.Lock()
		defer l.mu.Unlock()
		return !l.cerrada && l.ciclo == ciclo && l.actual != nil && l.ctx.Err() == nil
	}
	opciones.AlCambiar = func() {
		if vigente() && l.opciones.AlCambiar != nil {
			l.opciones.AlCambiar()
		}
	}
	opciones.AlFotograma = func() {
		if vigente() && l.opciones.AlFotograma != nil {
			l.opciones.AlFotograma()
		}
	}
	opciones.AlSenal = func(quien string, senal llamada.Senal) {
		if vigente() && l.opciones.AlSenal != nil {
			l.opciones.AlSenal(quien, senal)
		}
	}
	opciones.AlSalir = func(string) {
		select {
		case l.desconexion <- ciclo:
		default:
		}
	}
	return opciones
}

func (l *llamadaCifrada) conectarEpoca(ctx context.Context, next api.RoomsCall, ciclo uint64, key string, until time.Time) error {
	actual, err := l.conectar(next.LiveKitURL, next.Token, l.opcionesMedios(ciclo, key, until))
	if err != nil {
		return err
	}
	if actual == nil {
		return mx.ErrNotReady
	}
	// A delayed synchronous SFU join cannot resurrect a closed window/account.
	stop := context.AfterFunc(ctx, actual.Colgar)
	adoptada := false
	defer func() {
		if !adoptada {
			actual.Colgar()
		}
		stop()
	}()
	if ctx.Err() != nil || !until.After(time.Now()) {
		return mx.ErrClosed
	}
	if err := l.api.CallEncryptionReady(ctx, l.call.Room.ID, l.device, l.call.JoinID, l.binding.Generation); err != nil {
		return err
	}
	if err := l.reanudarMedios(ctx, actual, next.Role); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cerrada || ctx.Err() != nil || !until.After(time.Now()) {
		return mx.ErrClosed
	}
	l.actual, adoptada = actual, true
	return nil
}

func (l *llamadaCifrada) reanudarMedios(ctx context.Context, actual EnCurso, role string) error {
	permitido, err := esperarPublicacion(ctx, actual, role)
	if err != nil {
		return err
	}
	l.mu.Lock()
	if !permitido {
		l.micro, l.camara, l.pantalla = false, false, false
	}
	micro, camara, pantalla := l.micro, l.camara, l.pantalla
	cerrada := l.cerrada || l.ctx.Err() != nil
	l.mu.Unlock()
	if cerrada {
		return mx.ErrClosed
	}
	// The initial SFU grant is receive-only. Publishing follows acknowledgement of
	// this exact verified device, join nonce and epoch; errors are never ignored.
	if micro {
		if err := actual.Micro(true); err != nil {
			return err
		}
	}
	if camara {
		if err := actual.Camara(true); err != nil {
			return err
		}
	}
	if pantalla {
		if err := actual.Pantalla(true); err != nil {
			return err
		}
	}
	return nil
}

func esperarPublicacion(ctx context.Context, actual EnCurso, role string) (bool, error) {
	if role == "viewer" {
		return false, nil
	}
	plazo := time.NewTimer(2 * time.Second)
	defer plazo.Stop()
	ticks := time.NewTicker(100 * time.Millisecond)
	defer ticks.Stop()
	for !actual.PuedePublicar() {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-plazo.C:
			return false, nil // still allowed to join as a listener
		case <-ticks.C:
		}
	}
	return true, nil
}

func (l *llamadaCifrada) vigilar(ctx context.Context) {
	ticker := time.NewTicker(l.intervalo)
	defer ticker.Stop()
	defer l.Colgar()
	for {
		force := false
		select {
		case <-l.ctx.Done():
			return
		case ciclo := <-l.desconexion:
			l.mu.Lock()
			force = ciclo == l.ciclo
			l.mu.Unlock()
			if !force {
				continue
			}
		case <-ticker.C:
		}
		next, err := l.api.CallEncryption(ctx, l.call.Room.ID, l.device, l.call.JoinID)
		if err == nil {
			err = l.validar(next)
		}
		if err == nil && (force || !reflect.DeepEqual(next, l.binding)) {
			err = l.ajustar(ctx, next)
		}
		if err != nil {
			l.Colgar()
			if !errors.Is(err, context.Canceled) && l.opciones.AlSalir != nil {
				l.opciones.AlSalir("encryption_unavailable")
			}
			return
		}
		if l.opciones.AlCambiar != nil {
			l.opciones.AlCambiar()
		}
	}
}

func (l *llamadaCifrada) Colgar() { l.cerrar(0) }

func (l *llamadaCifrada) cerrar(ciclo uint64) bool {
	l.mu.Lock()
	if l.cerrada || ciclo != 0 && ciclo != l.ciclo {
		l.mu.Unlock()
		return false
	}
	l.cerrada = true
	l.cancel()
	l.mu.Unlock()
	l.pausar()
	l.cerrarClaves()
	return true
}

func (l *llamadaCifrada) Participantes() []llamada.Participante {
	l.mu.Lock()
	actual := l.actual
	l.mu.Unlock()
	if actual == nil {
		return nil
	}
	return actual.Participantes()
}

func (l *llamadaCifrada) Reconectando() bool {
	l.mu.Lock()
	actual, cerrada := l.actual, l.cerrada
	l.mu.Unlock()
	return !cerrada && (actual == nil || actual.Reconectando())
}
func (*llamadaCifrada) Grabando() bool { return false }

func (l *llamadaCifrada) PuedePublicar() bool {
	l.mu.Lock()
	actual := l.actual
	l.mu.Unlock()
	return actual != nil && actual.PuedePublicar()
}

func (l *llamadaCifrada) operarMedios(fn func(EnCurso) error, guardar func()) error {
	l.mu.Lock()
	actual, ciclo := l.actual, l.ciclo
	l.mu.Unlock()
	if actual == nil {
		return llamada.ErrSinClaveMedios
	}
	if err := fn(actual); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cerrada || ciclo != l.ciclo || l.ctx.Err() != nil {
		return mx.ErrClosed
	}
	if guardar != nil {
		guardar()
	}
	return nil
}

func (l *llamadaCifrada) Micro(si bool) error {
	return l.operarMedios(func(actual EnCurso) error { return actual.Micro(si) }, func() { l.micro = si })
}

func (l *llamadaCifrada) Camara(si bool) error {
	return l.operarMedios(func(actual EnCurso) error { return actual.Camara(si) }, func() { l.camara = si })
}

func (l *llamadaCifrada) Pantalla(si bool) error {
	return l.operarMedios(func(actual EnCurso) error { return actual.Pantalla(si) }, func() { l.pantalla = si })
}

func (l *llamadaCifrada) Senal(s llamada.Senal) error {
	return l.operarMedios(func(actual EnCurso) error { return actual.Senal(s) }, nil)
}

func (l *llamadaCifrada) CambiarAudio(d llamada.Dispositivo) error {
	return l.operarMedios(func(actual EnCurso) error { return actual.CambiarAudio(d) }, func() { l.opciones.Audio = d })
}
