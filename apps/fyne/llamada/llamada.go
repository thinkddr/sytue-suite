// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package llamada

import (
	"encoding/base64"
	"fmt"
	"image"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/livekit/server-sdk-go/v2/e2ee"
	e2eetypes "github.com/livekit/server-sdk-go/v2/e2ee/types"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/tphakala/go-opus/opus"
)

// Disponible dice que la llamada nativa existe en este destino (no en js/wasm).
const Disponible = true

// Llamada es una sala conectada.
type Llamada struct {
	o       Opciones
	room    *lksdk.Room
	claves  *e2ee.ExternalKeyProvider
	caducar *time.Timer

	camara, pantalla videoPropio

	mu           sync.Mutex
	remotos      map[string]*remoto // por identidad
	manos        map[string]bool    // manos alzadas, por identidad (la propia incluida)
	colas        map[string]*cola   // por SID de pista de audio
	pub          *lksdk.LocalTrackPublication
	pista        *lksdk.LocalTrack
	micro        bool
	reconectando bool
	colgada      bool
	cerrarAudio  func()
	enviar       chan []int16
	pararEnvio   chan struct{}
	ultimoAviso  time.Time
}

type remoto struct {
	video, compartida image.Image
}

// Entrar conecta con el SFU con la conexión que dio el servicio.
func Entrar(url, token string, o Opciones) (*Llamada, error) {
	if url == "" || token == "" {
		return nil, ErrSinSala
	}
	keys, err := prepararCifrado(&o)
	if err != nil {
		return nil, err
	}
	l := &Llamada{o: o, claves: keys, remotos: map[string]*remoto{}, colas: map[string]*cola{}, manos: map[string]bool{}}
	l.camara = videoPropio{fuenteLK: livekit.TrackSource_CAMERA, talla: TallaCamara}
	l.pantalla = videoPropio{fuenteLK: livekit.TrackSource_SCREEN_SHARE, talla: TallaPantalla}
	cb := lksdk.NewRoomCallback()
	cb.OnTrackSubscribed = l.alSuscribir
	cb.OnTrackUnsubscribed = func(t *webrtc.TrackRemote, _ *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
		l.mu.Lock()
		delete(l.colas, t.ID())
		if r := l.remotos[rp.Identity()]; r != nil && t.Kind() == webrtc.RTPCodecTypeVideo {
			r.video, r.compartida = nil, nil
		}
		l.mu.Unlock()
		l.cambio()
	}
	cb.OnParticipantConnected = func(*lksdk.RemoteParticipant) { l.cambio() }
	cb.OnParticipantDisconnected = func(rp *lksdk.RemoteParticipant) {
		l.mu.Lock()
		delete(l.remotos, rp.Identity())
		delete(l.manos, rp.Identity())
		l.mu.Unlock()
		l.cambio()
	}
	cb.OnActiveSpeakersChanged = func([]lksdk.Participant) { l.cambio() }
	cb.OnTrackMuted = func(lksdk.TrackPublication, lksdk.Participant) { l.cambio() }
	cb.OnTrackUnmuted = func(lksdk.TrackPublication, lksdk.Participant) { l.cambio() }
	cb.OnReconnecting = func() { l.ponerReconectando(true) }
	cb.OnReconnected = func() { l.ponerReconectando(false) }
	cb.OnRecordingStatusChanged = func(bool) { l.cambio() }
	cb.OnDataPacket = func(d lksdk.DataPacket, p lksdk.DataReceiveParams) {
		if !l.claveVigente() {
			return
		}
		u, ok := d.(*lksdk.UserDataPacket)
		if !ok {
			return
		}
		s, ok := DescodificarSenal(u.Payload)
		if !ok {
			return
		}
		l.aplicar(p.SenderIdentity, s)
	}
	cb.OnDisconnectedWithReason = func(motivo lksdk.DisconnectionReason) {
		l.mu.Lock()
		colgada := l.colgada
		l.mu.Unlock()
		if !colgada && l.o.AlSalir != nil {
			l.o.AlSalir(string(motivo))
		}
		l.Colgar()
	}
	l.room = lksdk.NewRoom(cb)
	options := []lksdk.ConnectOption{lksdk.WithAutoSubscribe(true)}
	if keys != nil {
		options = append(options, lksdk.WithDataEncryption(&lksdk.EncryptionOptions{KeyProvider: keys}))
	}
	if err := l.room.JoinWithToken(url, token, options...); err != nil {
		return nil, err
	}
	if o.Cifrado != nil {
		l.mu.Lock()
		if !l.colgada {
			l.caducar = time.AfterFunc(time.Until(o.Cifrado.Caduca), func() {
				l.Colgar()
				if l.o.AlSalir != nil {
					l.o.AlSalir("encryption_expired")
				}
			})
		}
		l.mu.Unlock()
	}
	// Sin tarjeta se sigue dentro, sin oír ni hablar: el `PreJoin` de la web tampoco
	// deja a nadie fuera por no tener permiso de micrófono.
	_ = l.CambiarAudio(o.Audio)
	if o.Micro {
		_ = l.Micro(true)
	}
	return l, nil
}

// Micro abre o cierra el micrófono. La primera vez que se abre se publica la pista; al
// cerrarlo se silencia (la web hace lo mismo con `setMicrophoneEnabled(false)`).
func (l *Llamada) Micro(abierto bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.colgada {
		return nil
	}
	if abierto && l.pub == nil {
		options, err := l.opcionesCifrado(lksdk.CodecOpus)
		if err != nil {
			return err
		}
		pista, err := lksdk.NewLocalSampleTrack(webrtc.RTPCodecCapability{
			MimeType: webrtc.MimeTypeOpus, ClockRate: Muestreo, Channels: 2,
		}, options...)
		if err != nil {
			return err
		}
		pub, err := l.room.LocalParticipant.PublishTrack(pista, &lksdk.TrackPublicationOptions{
			Name: "microphone", Source: livekit.TrackSource_MICROPHONE,
		})
		if err != nil {
			return err
		}
		enc, err := opus.NewEncoder(opus.EncoderConfig{SampleRate: Muestreo, Channels: 1, Bitrate: 32000})
		if err != nil {
			return err
		}
		l.pista, l.pub = pista, pub
		l.enviar, l.pararEnvio = make(chan []int16, 8), make(chan struct{})
		go codificar(enc, pista, l.enviar, l.pararEnvio)
	}
	if l.pub != nil {
		l.pub.SetMuted(!abierto)
	}
	l.micro = abierto
	return nil
}

// codificar pasa cada tramo del micrófono a Opus y lo manda al SFU.
func codificar(enc *opus.Encoder, pista *lksdk.LocalTrack, tramos <-chan []int16, parar <-chan struct{}) {
	buf := make([]byte, 1276)
	for {
		select {
		case <-parar:
			return
		case pcm := <-tramos:
			n, err := enc.Encode(pcm, buf)
			if err != nil {
				continue
			}
			_ = pista.WriteSample(media.Sample{Data: buf[:n], Duration: 20 * time.Millisecond}, nil)
		}
	}
}

// capturado recibe un tramo del micrófono. Con el micro cerrado no sale nada; con la
// cola llena se tira el tramo antes que atrasar la voz.
func (l *Llamada) capturado(pcm []int16) {
	l.mu.Lock()
	enviar, abierto := l.enviar, l.micro && !l.colgada
	l.mu.Unlock()
	if !abierto || enviar == nil {
		return
	}
	for len(pcm) >= Tramo {
		select {
		case enviar <- slices.Clone(pcm[:Tramo]):
		default:
		}
		pcm = pcm[Tramo:]
	}
}

// mezclar rellena el altavoz con la suma de todas las voces.
func (l *Llamada) mezclar(out []int16) {
	l.mu.Lock()
	colas := make([]*cola, 0, len(l.colas))
	for _, c := range l.colas {
		colas = append(colas, c)
	}
	l.mu.Unlock()
	Mezclar(out, colas...)
}

// CambiarAudio cambia de tarjeta (otro micro u otro altavoz) sin salir de la sala.
// Con nil, se queda sin audio.
func (l *Llamada) CambiarAudio(d Dispositivo) error {
	l.mu.Lock()
	cerrar := l.cerrarAudio
	l.cerrarAudio = nil
	l.mu.Unlock()
	if cerrar != nil {
		cerrar()
	}
	if d == nil {
		return nil
	}
	nuevo, err := d.Abrir(l.capturado, l.mezclar)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.colgada {
		nuevo()
		return nil
	}
	l.cerrarAudio = nuevo
	return nil
}

// Identidad es la de quien mira en la sala.
func (l *Llamada) Identidad() string { return l.room.LocalParticipant.Identity() }

// Grabando dice si la sala se está grabando: lo dice el SFU a todos, lo haya pedido
// quien lo haya pedido.
func (l *Llamada) Grabando() bool { return l.room.IsRecording() }

// Senal manda una mano o una reacción a la sala. La mano propia se aplica aquí además
// de mandarse: LiveKit no devuelve al emisor sus propios datos.
func (l *Llamada) Senal(s Senal) error {
	if !l.claveVigente() {
		return ErrSinClaveMedios
	}
	if err := l.room.LocalParticipant.PublishDataPacket(lksdk.UserData(s.Codificar()), lksdk.WithDataPublishReliable(true)); err != nil {
		return err
	}
	l.aplicar(l.Identidad(), s)
	return nil
}

// claveVigente also fences signals at the exact lease deadline. Plain Cast has
// its separate contract; encrypted Rooms never dispatch or publish after expiry.
func (l *Llamada) claveVigente() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.colgada && (l.o.Cifrado == nil || time.Now().Before(l.o.Cifrado.Caduca))
}

// aplicar anota una señal y la reenvía a quien pinta.
func (l *Llamada) aplicar(quien string, s Senal) {
	if s.Tipo == "mano" {
		l.mu.Lock()
		if s.Alzada {
			l.manos[quien] = true
		} else {
			delete(l.manos, quien)
		}
		l.mu.Unlock()
		l.cambio()
	}
	if l.o.AlSenal != nil {
		l.o.AlSenal(quien, s)
	}
}

// Camara enciende o apaga la cámara propia.
func (l *Llamada) Camara(encendida bool) error {
	l.mu.Lock()
	closed := l.colgada
	l.mu.Unlock()
	if closed && encendida {
		return ErrSinSala
	}
	if !encendida {
		l.camara.apagar()
		l.cambio()
		return nil
	}
	err := l.encender(&l.camara, l.o.Camara)
	l.cambio()
	return err
}

// Pantalla empieza o deja de compartir la pantalla.
func (l *Llamada) Pantalla(encendida bool) error {
	l.mu.Lock()
	closed := l.colgada
	l.mu.Unlock()
	if closed && encendida {
		return ErrSinSala
	}
	if !encendida {
		l.pantalla.apagar()
		l.cambio()
		return nil
	}
	err := l.encender(&l.pantalla, l.o.Pantalla)
	l.cambio()
	return err
}

// Participantes son los que están, uno mismo el primero.
func (l *Llamada) Participantes() []Participante {
	lp := l.room.LocalParticipant
	yo := Participante{
		Identidad: lp.Identity(), Nombre: lp.Name(), Yo: true, Hablando: lp.IsSpeaking(),
		Video: l.camara.fotograma(), Compartida: l.pantalla.fotograma(),
	}
	l.mu.Lock()
	yo.MicroCerrado = !l.micro
	yo.ManoAlzada = l.manos[yo.Identidad]
	l.mu.Unlock()
	remotos := l.room.GetRemoteParticipants()
	out := make([]Participante, 0, 1+len(remotos))
	out = append(out, yo)
	for _, rp := range remotos {
		p := Participante{
			Identidad: rp.Identity(), Nombre: rp.Name(), Hablando: rp.IsSpeaking(),
			MicroCerrado: !rp.IsMicrophoneEnabled(),
		}
		l.mu.Lock()
		if r := l.remotos[rp.Identity()]; r != nil {
			p.Video, p.Compartida = r.video, r.compartida
		}
		p.ManoAlzada = l.manos[rp.Identity()]
		l.mu.Unlock()
		out = append(out, p)
	}
	return out
}

// Reconectando dice si se está recuperando la conexión.
func (l *Llamada) Reconectando() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.reconectando
}

// Colgar sale de la sala LIMPIA: el SFU se entera en el acto y libera el sitio, en vez
// de esperar a que caduque la sesión. Se puede llamar más de una vez.
func (l *Llamada) Colgar() {
	l.mu.Lock()
	if l.caducar != nil {
		l.caducar.Stop()
	}
	if l.colgada {
		l.mu.Unlock()
		return
	}
	l.colgada = true
	cerrar, parar := l.cerrarAudio, l.pararEnvio
	l.cerrarAudio, l.pararEnvio = nil, nil
	l.mu.Unlock()
	if cerrar != nil {
		cerrar()
	}
	if parar != nil {
		close(parar)
	}
	l.camara.apagar()
	l.pantalla.apagar()
	l.room.Disconnect()
}

func (l *Llamada) ponerReconectando(si bool) {
	l.mu.Lock()
	l.reconectando = si
	l.mu.Unlock()
	l.cambio()
}

func (l *Llamada) cambio() {
	if l.o.AlCambiar != nil {
		l.o.AlCambiar()
	}
}

// fotograma avisa de un fotograma nuevo, como mucho a 30 por segundo entre todas las
// pistas: con nueve cámaras a 30 fps serían 270 repintados por segundo.
func (l *Llamada) fotograma() {
	if l.o.AlFotograma == nil {
		return
	}
	l.mu.Lock()
	if time.Since(l.ultimoAviso) < 33*time.Millisecond {
		l.mu.Unlock()
		return
	}
	l.ultimoAviso = time.Now()
	l.mu.Unlock()
	l.o.AlFotograma()
}

// alSuscribir arranca la lectura de una pista remota.
func (l *Llamada) alSuscribir(t *webrtc.TrackRemote, pub *lksdk.RemoteTrackPublication, rp *lksdk.RemoteParticipant) {
	switch t.Kind() {
	case webrtc.RTPCodecTypeAudio:
		c := nuevaCola()
		l.mu.Lock()
		l.colas[t.ID()] = c
		l.mu.Unlock()
		decrypt, err := l.descifrador(lksdk.CodecOpus)
		if err != nil {
			return
		}
		go leerVoz(t, c, decrypt)
	case webrtc.RTPCodecTypeUnknown:
		return
	case webrtc.RTPCodecTypeVideo:
		fuente := Camara
		if pub.Source() == livekit.TrackSource_SCREEN_SHARE {
			fuente = Pantalla
		}
		// ponytail: calidad media de la simulcast. Una tesela no pide 1080p; cuando la
		// vista tenga «fijar», la fijada pedirá HIGH con SetVideoQuality.
		if fuente == Camara {
			_ = pub.SetVideoQuality(livekit.VideoQuality_MEDIUM)
		}
		decrypt, err := l.descifrador(lksdk.CodecVP8)
		if err != nil {
			return
		}
		go leerVideo(t, decrypt, func() { rp.WritePLI(t.SSRC()) }, func(img image.Image) {
			l.mu.Lock()
			r := l.remotos[rp.Identity()]
			if r == nil {
				r = &remoto{}
				l.remotos[rp.Identity()] = r
			}
			if fuente == Pantalla {
				r.compartida = img
			} else {
				r.video = img
			}
			l.mu.Unlock()
			l.fotograma()
		})
	}
	l.cambio()
}

// leerVoz descodifica una pista de voz hacia su cola.
func leerVoz(t *webrtc.TrackRemote, c *cola, decrypt e2eetypes.FrameDecryptor) {
	dec, err := opus.NewDecoder(Muestreo, 1)
	if err != nil {
		return
	}
	red := strings.EqualFold(t.Codec().MimeType, "audio/red")
	if !red && !strings.EqualFold(t.Codec().MimeType, webrtc.MimeTypeOpus) {
		return
	}
	pcm := make([]int16, Muestreo*120/1000) // el tramo más largo de Opus: 120 ms
	for {
		pkt, _, err := t.ReadRTP()
		if err != nil {
			return
		}
		carga := pkt.Payload
		if red {
			carga = PrimarioRED(carga)
		}
		if len(carga) == 0 {
			continue
		}
		if decrypt != nil {
			carga, err = decrypt.DecryptFrame(carga)
			if err != nil || len(carga) == 0 {
				continue
			}
		}
		n, err := dec.Decode(carga, pcm)
		if err != nil {
			continue
		}
		c.escribir(pcm[:n])
	}
}

// PrimarioRED saca la codificación PRIMARIA de una carga RED (RFC 2198), que es como
// la web manda la voz por defecto: cabeceras de 4 bytes con el bit F para las
// redundantes, una de 1 byte para la primaria, y los datos en el mismo orden. La
// primaria es la última. Una carga mal formada devuelve nil.
func PrimarioRED(p []byte) []byte {
	i, redundantes := 0, 0
	for i < len(p) && p[i]&0x80 != 0 {
		if len(p) < i+4 {
			return nil
		}
		b := p[i : i+4]
		redundantes += int(b[2]&0x03)<<8 | int(b[3])
		i += 4
	}
	i++ // la cabecera de la primaria
	if i+redundantes > len(p) {
		return nil
	}
	return p[i+redundantes:]
}

func (l *Llamada) opcionesCifrado(codec lksdk.Codec) ([]lksdk.LocalTrackOptions, error) {
	if l.claves == nil {
		return nil, nil
	}
	enc, err := lksdk.NewFrameEncryptor(l.claves, codec)
	if err != nil {
		return nil, err
	}
	return []lksdk.LocalTrackOptions{lksdk.WithFrameEncryptor(deadlineEncryptor{inner: enc, until: l.o.Cifrado.Caduca})}, nil
}

func (l *Llamada) descifrador(codec lksdk.Codec) (e2eetypes.FrameDecryptor, error) {
	if l.claves == nil {
		return nil, nil //nolint:nilnil // Cast has a separate explicit broadcast transport contract.
	}
	if l.o.Cifrado == nil || !l.o.Cifrado.Caduca.After(time.Now()) {
		return nil, ErrSinClaveMedios
	}
	dec, err := lksdk.NewFrameDecryptor(l.claves, codec, l.room.SifTrailer())
	if err != nil {
		return nil, fmt.Errorf("frame decryptor: %w", err)
	}
	return deadlineDecryptor{inner: dec, until: l.o.Cifrado.Caduca}, nil
}

// Enforce the authenticated deadline on each frame as well as on the connection
// timer: scheduler latency must never authorize a late frame or a stale key.
type deadlineEncryptor struct {
	inner e2eetypes.FrameEncryptor
	until time.Time
}

func (d deadlineEncryptor) EncryptFrame(payload []byte) ([]byte, error) {
	if !d.until.After(time.Now()) {
		return nil, ErrSinClaveMedios
	}
	return d.inner.EncryptFrame(payload)
}

type deadlineDecryptor struct {
	inner e2eetypes.FrameDecryptor
	until time.Time
}

func (d deadlineDecryptor) DecryptFrame(payload []byte) ([]byte, error) {
	if !d.until.After(time.Now()) {
		return nil, ErrSinClaveMedios
	}
	return d.inner.DecryptFrame(payload)
}

func prepararCifrado(o *Opciones) (*e2ee.ExternalKeyProvider, error) {
	if o.RequiereCifrado && o.Cifrado == nil {
		return nil, ErrSinClaveMedios
	}
	if o.Cifrado == nil {
		return nil, nil //nolint:nilnil // Cast has a separate explicit broadcast transport contract.
	}
	copyKey := *o.Cifrado
	o.Cifrado = &copyKey
	raw, err := base64.StdEncoding.DecodeString(o.Cifrado.Clave)
	if err != nil || len(raw) != 32 || !o.Cifrado.Caduca.After(time.Now()) {
		return nil, ErrSinClaveMedios
	}
	clear(raw)
	keys := e2ee.NewExternalKeyProvider()
	if err := keys.SetKeyFromPassphrase(o.Cifrado.Clave, 0); err != nil {
		return nil, err
	}
	return keys, nil
}
