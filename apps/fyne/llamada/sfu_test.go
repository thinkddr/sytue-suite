// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package llamada

import (
	"image"
	"math"
	"net"
	"os"
	"os/exec"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/livekit/protocol/auth"

	"git.sytue.eu/sytue/one/apps/fyne/llamada/camara"
)

// tarjeta es el doble de la tarjeta de sonido: cada 20 ms entrega al micrófono un tono
// (si lo tiene) y guarda lo que el altavoz le devuelve.
type tarjeta struct {
	tono float64

	mu    sync.Mutex
	oido  []int16
	parar chan struct{}
}

func (k *tarjeta) Abrir(capturado, reproducir func([]int16)) (func(), error) {
	k.parar = make(chan struct{})
	go func() {
		tic := time.NewTicker(20 * time.Millisecond)
		defer tic.Stop()
		fase := 0
		for {
			select {
			case <-k.parar:
				return
			case <-tic.C:
			}
			mic := make([]int16, Tramo)
			for i := range mic {
				mic[i] = int16(8000 * math.Sin(2*math.Pi*k.tono*float64(fase+i)/Muestreo))
			}
			fase += Tramo
			if k.tono > 0 {
				capturado(mic)
			}
			sale := make([]int16, Tramo)
			reproducir(sale)
			k.mu.Lock()
			k.oido = append(k.oido, sale...)
			k.mu.Unlock()
		}
	}()
	return func() { close(k.parar) }, nil
}

// cruces cuenta los pasos por cero del último segundo oído con volumen: un tono de 440
// Hz da ~880. Cero si no se oye nada.
func (k *tarjeta) cruces() (int, float64) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if len(k.oido) < Muestreo {
		return 0, 0
	}
	s := k.oido[len(k.oido)-Muestreo:]
	n, energia := 0, 0.0
	for i := 1; i < len(s); i++ {
		if (s[i-1] < 0) != (s[i] < 0) {
			n++
		}
		energia += float64(s[i]) * float64(s[i])
	}
	return n, math.Sqrt(energia / float64(len(s)))
}

// sfuDePrueba arranca un LiveKit en modo --dev (claves devkey/secret) en esta máquina.
// Es el MISMO binario que la suite despliega (`livekit/livekit-server` v1.13.x), sacado
// de la imagen; la ruta la da SYTUE_LIVEKIT_SERVER. Sin ella el guardián se salta: no
// es un test unitario, es la prueba de que los medios pasan de verdad.
func sfuDePrueba(t *testing.T) string {
	bin := os.Getenv("SYTUE_LIVEKIT_SERVER")
	if bin == "" {
		t.Skip("sin SYTUE_LIVEKIT_SERVER: no hay SFU con el que probar")
	}
	cmd := exec.Command(bin, "--dev", "--bind", "127.0.0.1")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	for range 100 {
		if c, err := net.Dial("tcp", "127.0.0.1:7880"); err == nil {
			_ = c.Close()
			return "ws://127.0.0.1:7880"
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("el SFU no abrió el 7880")
	return ""
}

func tokenDePrueba(t *testing.T, identidad string) string {
	at := auth.NewAccessToken("devkey", "secret")
	si := true
	at.SetVideoGrant(&auth.VideoGrant{RoomJoin: true, Room: "prueba", CanPublish: &si, CanSubscribe: &si}).
		SetIdentity(identidad).SetName(identidad)
	tok, err := at.ToJWT()
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// Dos personas en una sala de un LiveKit de verdad: lo que Ana dice por el micro, Bea
// lo oye (el tono, no ruido); la cámara de Ana —publicada por `Camara`, escalada de 720p
// a 640×360 y codificada en VP8— le llega a Bea como imagen, y apagarla suelta la
// cámara; su mano y su reacción le llegan por el canal de datos; y cuando Ana
// cuelga, Bea deja de verla en el acto, que es lo que dice que la salida es limpia y no
// una sesión que caduca.
//
// VISTO EN ROJO (2026-09-25, en el frontal): con `capturado` tirando los tramos del
// micro abierto, Bea no oye nada; y quitando `room.Disconnect()` de Colgar, Bea sigue
// viendo a Ana pasados los quince segundos.
func TestLaLlamadaPasaVozYVideoPorElSFU(t *testing.T) {
	url := sfuDePrueba(t)
	oye := &tarjeta{}
	var muSen sync.Mutex
	var senales []string
	bea, err := Entrar(url, tokenDePrueba(t, "bea"), Opciones{Audio: oye, AlSenal: func(q string, s Senal) {
		muSen.Lock()
		senales = append(senales, q+":"+string(s.Codificar()))
		muSen.Unlock()
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer bea.Colgar()
	habla := &tarjeta{tono: 440}
	cam := &camaraDePrueba{}
	ana, err := Entrar(url, tokenDePrueba(t, "ana"), Opciones{
		Audio: habla, Micro: true,
		Camara: func() (camara.Fuente, error) { return cam, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ana.Colgar()
	if err := ana.Camara(true); err != nil {
		t.Fatal(err)
	}

	esperar := func(que string, cond func() bool) {
		t.Helper()
		for range 150 {
			if cond() {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Error(que)
	}
	esperar("Bea no oye el tono de Ana", func() bool {
		n, rms := oye.cruces()
		return rms > 500 && n > 700 && n < 1060
	})
	esperar("la cámara de Ana no le llega a Bea", func() bool {
		for _, p := range bea.Participantes() {
			if p.Identidad == "ana" && p.Video != nil && p.Video.Bounds().Dx() == 640 {
				return true
			}
		}
		return false
	})
	if ps := bea.Participantes(); len(ps) != 2 || !ps[0].Yo || ps[1].MicroCerrado {
		t.Errorf("Bea ve %+v", ps)
	}
	if yo := ana.Participantes()[0]; yo.Video == nil {
		t.Error("Ana no se ve a sí misma: la previa de su cámara no llega")
	}
	// La mano y la reacción de Ana le llegan a Bea por el canal de datos.
	if err := ana.Senal(Mano(true)); err != nil {
		t.Fatal(err)
	}
	if err := ana.Senal(Reaccion("🎉")); err != nil {
		t.Fatal(err)
	}
	esperar("la mano de Ana no le llega a Bea", func() bool {
		ps := bea.Participantes()
		return len(ps) == 2 && ps[1].ManoAlzada
	})
	esperar("la reacción de Ana no le llega a Bea", func() bool {
		muSen.Lock()
		defer muSen.Unlock()
		return slices.Contains(senales, `ana:{"t":"reaccion","emoji":"🎉"}`)
	})
	if !ana.Participantes()[0].ManoAlzada {
		t.Error("Ana no ve su propia mano: LiveKit no devuelve lo propio")
	}
	// Apagar la cámara la SUELTA (se apaga la luz) y Bea deja de recibir fotogramas.
	if err := ana.Camara(false); err != nil {
		t.Fatal(err)
	}
	esperar("apagada la cámara, no se suelta", func() bool { return cam.cerrada })

	ana.Colgar()
	antes := time.Now()
	esperar("colgada Ana, Bea la sigue viendo", func() bool { return len(bea.Participantes()) == 1 })
	if d := time.Since(antes); d > 5*time.Second {
		t.Errorf("Bea tardó %v en ver salir a Ana", d)
	}
}

// camaraDePrueba es una cámara sintética de 1280×720 que cambia de luma en cada
// fotograma: la llamada la escala a 640×360 y la codifica como una de verdad.
type camaraDePrueba struct {
	n       int
	cerrada bool
}

func (c *camaraDePrueba) Leer() (*image.YCbCr, bool) {
	if c.cerrada {
		return nil, false
	}
	c.n++
	img := camara.I420(1280, 720)
	for i := range img.Y {
		img.Y[i] = byte(c.n * 3)
	}
	for i := range img.Cb {
		img.Cb[i], img.Cr[i] = 90, 200
	}
	return img, true
}

func (c *camaraDePrueba) Cerrar() { c.cerrada = true }
