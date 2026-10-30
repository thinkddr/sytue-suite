// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

type mediosAutorizados struct {
	mu              sync.Mutex
	call            api.RoomsCall
	ready           int64
	denegado        bool
	nonces          []string
	cambiar         func(*api.RoomsCall)
	fallarReady     error
	bloquear        <-chan struct{}
	permissionDelay time.Duration
	permissionAfter time.Time
}

func (m *mediosAutorizados) CallEncryption(ctx context.Context, _, _, nonce string) (api.RoomsCallEncryption, error) {
	m.mu.Lock()
	bloquear := m.bloquear
	m.mu.Unlock()
	if bloquear != nil {
		select {
		case <-ctx.Done():
			return api.RoomsCallEncryption{}, ctx.Err()
		case <-bloquear:
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nonces = append(m.nonces, nonce)
	if m.denegado {
		return api.RoomsCallEncryption{}, mx.ErrUnverified
	}
	if err := ctx.Err(); err != nil {
		return api.RoomsCallEncryption{}, err
	}
	return *m.call.Encryption, nil
}

func (m *mediosAutorizados) CallEncryptionReconnect(ctx context.Context, _, _, nonce string, epoch int64) (api.RoomsCall, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nonces = append(m.nonces, nonce)
	if err := ctx.Err(); err != nil {
		return api.RoomsCall{}, err
	}
	if m.denegado || epoch != m.call.Encryption.Generation {
		return api.RoomsCall{}, mx.ErrUnverified
	}
	call := m.call
	if m.cambiar != nil {
		m.cambiar(&call)
	}
	return call, nil
}

func (m *mediosAutorizados) CallEncryptionReady(ctx context.Context, _, _, nonce string, epoch int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nonces = append(m.nonces, nonce)
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.fallarReady != nil {
		return m.fallarReady
	}
	m.ready = epoch
	m.permissionAfter = time.Now().Add(m.permissionDelay)
	return nil
}

func llamadaDePrueba() api.RoomsCall {
	binding := &api.RoomsCallEncryption{CallID: "call", MatrixRoomID: "!epoch", LiveKitRoom: "epoch-1", Generation: 1, ExpiresAtMS: time.Now().Add(time.Minute).UnixMilli(), LeaderSubject: "person", LeaderDeviceID: "device", Members: []api.RoomsCallMember{{Subject: "person", DeviceID: "device"}}}
	return api.RoomsCall{Room: api.RoomsMeeting{ID: "call"}, Identity: "person", JoinID: "same-join", Token: "opaque", LiveKitURL: "wss://media.invalid", Encryption: binding}
}

type clavesPrueba struct {
	ctx           context.Context
	binding       api.RoomsCallEncryption
	mu            sync.Mutex
	closed, ready bool
	wait          bool
}

func (k *clavesPrueba) Start(context.Context) error {
	k.mu.Lock()
	k.ready = !k.wait
	k.mu.Unlock()
	return nil
}

func (k *clavesPrueba) CurrentKey() (string, time.Time, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	until := time.UnixMilli(k.binding.ExpiresAtMS)
	return base64.StdEncoding.EncodeToString([]byte(strings.Repeat(strconv.FormatInt(k.binding.Generation, 10), 32))), until, !k.closed && k.ready && k.ctx.Err() == nil && until.After(time.Now())
}
func (k *clavesPrueba) Close() { k.mu.Lock(); k.closed = true; k.mu.Unlock() }

type medioPrueba struct {
	mu        sync.Mutex
	server    *mediosAutorizados
	epoch     int64
	o         llamada.Opciones
	closed    bool
	micro     bool
	failMicro error
}

func (m *medioPrueba) Participantes() []llamada.Participante {
	return []llamada.Participante{{Identidad: strconv.FormatInt(m.epoch, 10)}}
}

func (m *medioPrueba) Micro(si bool) error {
	ready := m.PuedePublicar()
	if si && !ready {
		return errors.New("microphone before ready")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failMicro != nil {
		return m.failMicro
	}
	m.micro = si
	return nil
}
func (*medioPrueba) Camara(bool) error                      { return nil }
func (*medioPrueba) Pantalla(bool) error                    { return nil }
func (*medioPrueba) Grabando() bool                         { return false }
func (*medioPrueba) CambiarAudio(llamada.Dispositivo) error { return nil }
func (*medioPrueba) Reconectando() bool                     { return false }
func (m *medioPrueba) PuedePublicar() bool {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	m.server.mu.Lock()
	defer m.server.mu.Unlock()
	return !closed && m.server.ready == m.epoch && !m.server.permissionAfter.After(time.Now()) && m.server.call.Role != "viewer"
}
func (*medioPrueba) Senal(llamada.Senal) error { return nil }
func (m *medioPrueba) Colgar()                 { m.mu.Lock(); m.closed = true; m.mu.Unlock() }

type pruebaCifrada struct {
	t               *testing.T
	server          *mediosAutorizados
	mu              sync.Mutex
	keys            []*clavesPrueba
	media           []*medioPrueba
	changed, exited chan struct{}
	beforeConnect   func()
	failMicro       error
}

func escenarioCifrado(t *testing.T) *pruebaCifrada {
	t.Helper()
	return &pruebaCifrada{t: t, server: &mediosAutorizados{call: llamadaDePrueba()}, changed: make(chan struct{}, 16), exited: make(chan struct{}, 16)}
}

func (p *pruebaCifrada) crear(ctx context.Context, b api.RoomsCallEncryption) (clavesDeLlamada, error) {
	k := &clavesPrueba{ctx: ctx, binding: b}
	p.mu.Lock()
	p.keys = append(p.keys, k)
	p.mu.Unlock()
	return k, nil
}

func (p *pruebaCifrada) conectar(_, _ string, o llamada.Opciones) (EnCurso, error) {
	if !o.RequiereCifrado || o.Cifrado == nil || o.Micro {
		return nil, errors.New("unprotected connection or early publication")
	}
	if p.beforeConnect != nil {
		p.beforeConnect()
	}
	p.server.mu.Lock()
	epoch := p.server.call.Encryption.Generation
	p.server.mu.Unlock()
	m := &medioPrueba{server: p.server, epoch: epoch, o: o, failMicro: p.failMicro}
	p.mu.Lock()
	p.media = append(p.media, m)
	p.mu.Unlock()
	return m, nil
}

func (p *pruebaCifrada) abrir(ctx context.Context) (*llamadaCifrada, error) {
	return nuevaLlamadaCifrada(ctx, p.server, "device", p.server.call, llamada.Opciones{Micro: true, AlCambiar: func() {
		select {
		case p.changed <- struct{}{}:
		default:
		}
	}, AlSalir: func(string) {
		select {
		case p.exited <- struct{}{}:
		default:
		}
	}}, p.conectar, p.crear, 10*time.Millisecond)
}

func esperarCifrado(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("call lifecycle did not progress")
	}
}

func TestLlamadaCifradaAutorizaAntesDePublicarYConservaLease(t *testing.T) {
	p := escenarioCifrado(t)
	l, err := p.abrir(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Colgar()
	if _, _, ready := p.keys[0].CurrentKey(); !ready {
		t.Fatal("negotiation completion canceled the media-key lease")
	}
	if !p.media[0].micro || p.server.ready != 1 {
		t.Fatal("microphone did not follow ready")
	}
	l.Colgar()
	if _, _, ready := p.keys[0].CurrentKey(); ready {
		t.Fatal("closed call retained a usable key")
	}
}

func TestLlamadaCifradaRotaSinCambiarJoinYReconectaConLaMismaClave(t *testing.T) {
	p := escenarioCifrado(t)
	l, err := p.abrir(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Colgar()
	first := p.media[0]
	first.o.AlSalir("network")
	esperarCifrado(t, p.changed)
	p.mu.Lock()
	if len(p.keys) != 1 || len(p.media) != 2 || p.media[1].o.Cifrado.Clave != first.o.Cifrado.Clave {
		p.mu.Unlock()
		t.Fatal("transport reconnect replaced the immutable epoch key")
	}
	p.mu.Unlock()
	p.server.mu.Lock()
	next := *p.server.call.Encryption
	next.Generation, next.LiveKitRoom = 2, "epoch-2"
	p.server.call.Encryption = &next
	p.server.mu.Unlock()
	for {
		participantes := l.Participantes()
		if len(participantes) > 0 && participantes[0].Identidad == "2" {
			break
		}
		esperarCifrado(t, p.changed)
	}
	p.mu.Lock()
	if len(p.keys) != 2 || len(p.media) != 3 || p.media[2].o.Cifrado.Clave == first.o.Cifrado.Clave {
		p.mu.Unlock()
		t.Fatal("new epoch did not replace the key and SFU connection")
	}
	p.mu.Unlock()
	first.mu.Lock()
	closed := first.closed
	first.mu.Unlock()
	if !closed {
		t.Fatal("old media connection survived rotation")
	}
	if _, _, ready := p.keys[0].CurrentKey(); ready {
		t.Fatal("old key survived rotation")
	}
	p.server.mu.Lock()
	defer p.server.mu.Unlock()
	for _, nonce := range p.server.nonces {
		if nonce != "same-join" {
			t.Fatal("reconciliation changed the join nonce")
		}
	}
}

func TestLlamadaCifradaRechazaRespuestaDeOtraConexion(t *testing.T) {
	for _, cambiar := range []func(*api.RoomsCall){func(c *api.RoomsCall) { c.JoinID = "other" }, func(c *api.RoomsCall) { c.Identity = "other" }, func(c *api.RoomsCall) { c.Encryption = nil }} {
		p := escenarioCifrado(t)
		p.server.cambiar = cambiar
		if l, err := p.abrir(t.Context()); err == nil {
			l.Colgar()
			t.Fatal("accepted a response outside this device connection")
		}
		if len(p.media) != 0 {
			t.Fatal("connected media before validating the server response")
		}
	}
}

func TestLlamadaCifradaNoIgnoraFallosReadyOMicro(t *testing.T) {
	for _, etapa := range []string{"ready", "micro"} {
		p := escenarioCifrado(t)
		if etapa == "ready" {
			p.server.fallarReady = mx.ErrUnverified
		} else {
			p.failMicro = errors.New("microphone denied")
		}
		if l, err := p.abrir(t.Context()); err == nil {
			l.Colgar()
			t.Fatal("ignored a publication failure")
		}
		p.media[0].mu.Lock()
		closed := p.media[0].closed
		p.media[0].mu.Unlock()
		if !closed {
			t.Fatal("failed connection retained media")
		}
	}
}

func TestLlamadaCifradaNoResucitaTrasCancelarDuranteJoin(t *testing.T) {
	p := escenarioCifrado(t)
	ctx, cancel := context.WithCancel(t.Context())
	p.beforeConnect = cancel
	if l, err := p.abrir(ctx); err == nil {
		l.Colgar()
		t.Fatal("adopted a late connection after cancellation")
	}
	p.server.mu.Lock()
	ready := p.server.ready
	p.server.mu.Unlock()
	if ready != 0 {
		t.Fatal("acknowledged media after closing the window")
	}
	p.media[0].mu.Lock()
	closed := p.media[0].closed
	p.media[0].mu.Unlock()
	if !closed {
		t.Fatal("late media connection remained active")
	}
}

func TestLlamadaCifradaExpiraSinEsperarAlServidor(t *testing.T) {
	p := escenarioCifrado(t)
	p.server.call.Encryption.ExpiresAtMS = time.Now().Add(100 * time.Millisecond).UnixMilli()
	l, err := p.abrir(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Colgar()
	p.server.mu.Lock()
	p.server.bloquear = make(chan struct{})
	p.server.mu.Unlock()
	esperarCifrado(t, p.exited)
	if l.Participantes() != nil {
		t.Fatal("expired lease retained media")
	}
	if err = l.Micro(true); err == nil {
		t.Fatal("expired lease permitted publication")
	}
}

func TestLlamadaCifradaRevocacionCierraMedios(t *testing.T) {
	p := escenarioCifrado(t)
	l, err := p.abrir(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Colgar()
	p.server.mu.Lock()
	p.server.denegado = true
	p.server.mu.Unlock()
	esperarCifrado(t, p.exited)
	if l.Participantes() != nil {
		t.Fatal("revoked device retained media")
	}
}

func TestLlamadaCifradaReconciliaMientrasEsperaLaClave(t *testing.T) {
	p := escenarioCifrado(t)
	crear := func(ctx context.Context, binding api.RoomsCallEncryption) (clavesDeLlamada, error) {
		keys, err := p.crear(ctx, binding)
		if binding.Generation == 1 {
			keys.(*clavesPrueba).wait = true
			p.server.mu.Lock()
			next := *p.server.call.Encryption
			next.Generation, next.LiveKitRoom = 2, "epoch-2"
			p.server.call.Encryption = &next
			p.server.mu.Unlock()
		}
		return keys, err
	}
	l, err := nuevaLlamadaCifrada(t.Context(), p.server, "device", p.server.call, llamada.Opciones{}, p.conectar, crear, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Colgar()
	if len(p.keys) != 2 || len(p.media) != 1 || l.Participantes()[0].Identidad != "2" {
		t.Fatal("call waited for an obsolete leader instead of reconciling its epoch")
	}
	if _, _, ready := p.keys[0].CurrentKey(); ready {
		t.Fatal("obsolete waiting key remained active")
	}
}

func TestLlamadaCifradaEsperaElPermisoAsincronoDelSDK(t *testing.T) {
	p := escenarioCifrado(t)
	p.server.permissionDelay = 100 * time.Millisecond
	l, err := p.abrir(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Colgar()
	if !l.PuedePublicar() || !p.media[0].micro {
		t.Fatal("call did not wait for the SDK publication permission")
	}
}

func TestLlamadaCifradaPermiteEntrarSoloAEscuchar(t *testing.T) {
	p := escenarioCifrado(t)
	p.server.call.Role = "viewer"
	l, err := p.abrir(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Colgar()
	if l.PuedePublicar() || p.media[0].micro || len(l.Participantes()) == 0 {
		t.Fatal("listener was rejected or published a microphone")
	}
	base := servicioConDos()
	sa, _, _ := salaEn(t, base, 1280)
	v := NuevaVistaLlamada(sa.P.T, "Equipo", l, true)
	if v.Micro() {
		t.Fatal("listener UI showed an active microphone")
	}
	v.AlternarMicro()
	if p.media[0].micro {
		t.Fatal("listener UI published a microphone")
	}
}
