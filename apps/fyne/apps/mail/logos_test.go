// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioConLogos es el de ejemplo con `SenderLogo`, que cuenta lo que se le pide.
type servicioConLogos struct {
	*servicioFalso
	logos   map[string][]byte
	pedidos []string
}

func (s *servicioConLogos) SenderLogo(_ context.Context, d string) ([]byte, error) {
	s.pedidos = append(s.pedidos, d)
	if p, ok := s.logos[d]; ok {
		return p, nil
	}
	return nil, api.ErrSinLogo
}

func pngPrueba(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 64, 64))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestElLogoSePideUnaVezPorDominioYRepintaAlLlegar: la primera vez no hay logo y se
// pide; al llegar se repinta UNA vez por tanda; un dominio sin logo no se vuelve a
// pedir; y en Spam no se pinta ni se pide nada.
func TestElLogoSePideUnaVezPorDominioYRepintaAlLlegar(t *testing.T) {
	s := &servicioConLogos{servicioFalso: servicioDeEjemplo(), logos: map[string][]byte{"corp.vodafone.es": pngPrueba(t)}}
	s.carpetas = append(s.carpetas, api.Mailbox{ID: "sp", Name: "Junk", Role: "junk"})
	b, pendientes := buzonDePrueba(t, s)
	b.Arrancar(context.Background())

	if b.LogoDe("vodafone@corp.vodafone.es") != nil {
		t.Fatal("la primera vez el logo aún no ha llegado")
	}
	b.LogoDe("ana@gmail.com")
	b.LogoDe("Otra@CORP.vodafone.es")
	if len(s.pedidos) != 2 || s.pedidos[0] != "corp.vodafone.es" || s.pedidos[1] != "gmail.com" {
		t.Fatalf("pedidos %q: uno por dominio, en minúsculas", s.pedidos)
	}
	if b.LogoDe("vodafone@corp.vodafone.es") == nil {
		t.Fatal("el logo llegó y no se devuelve")
	}
	if b.LogoDe("ana@gmail.com") != nil || len(s.pedidos) != 2 {
		t.Fatal("un dominio sin logo no se vuelve a pedir")
	}
	repintados := 0
	for _, f := range *pendientes {
		if f != nil {
			repintados++
		}
	}
	if repintados < 1 || b.pendienteLogos != true {
		t.Fatalf("el repintado se agrupa: %d esperas, pendiente %v", repintados, b.pendienteLogos)
	}

	b.ElegirCarpeta(Carpeta{ID: "sp", Rol: "junk"})
	if b.LogoDe("vodafone@corp.vodafone.es") != nil || b.LogoDe("x@nuevo.com") != nil || len(s.pedidos) != 2 {
		t.Fatalf("en spam no hay logos ni se piden (pedidos %q)", s.pedidos)
	}
}

// TestUnServicioSinLogosDejaLasIniciales: los dobles sin `SenderLogo` (y una cuenta
// que no lo tenga) no rompen nada.
func TestUnServicioSinLogosDejaLasIniciales(t *testing.T) {
	b, _ := buzonDePrueba(t, servicioDeEjemplo())
	b.Arrancar(context.Background())
	if b.LogoDe("x@empresa.com") != nil {
		t.Fatal("sin SenderLogo no hay logo")
	}
}

// TestLaFechaVaEnLaLineaDelRemitenteYElClipEnLaDelAsunto es la fila de MessageList.tsx
// tras quitar la columna de meta: la fecha arriba a la derecha, el clip a la derecha
// del asunto, la columna de texto hasta el margen y el logo en el avatar.
func TestLaFechaVaEnLaLineaDelRemitenteYElClipEnLaDelAsunto(t *testing.T) {
	convs := ejemplo()
	c := convs[0]
	c.ConAdjunto = true
	f := NuevaFilaConversacion()
	f.Logo = image.NewNRGBA(image.Rect(0, 0, 64, 64))
	f.Poner(c, "Sofía Nogueira Paz", "Calendario de despliegues", "11 sept")
	f.Resize(fyne.NewSize(400, AltoFilaConversacion()))
	r := test.WidgetRenderer(f).(*filaRenderer)
	f.Refresh()

	derecha := float32(400) - MargenFila
	fecha := r.fecha.Position()
	if d := fecha.X + ui.Medir(r.fecha.Text, tema.TextoXS, r.peso()).Width - derecha; d > 0.5 || d < -0.5 {
		t.Errorf("la fecha acaba en %g y el margen está en %g", fecha.X+ui.Medir(r.fecha.Text, tema.TextoXS, r.peso()).Width, derecha)
	}
	nombre := r.nombre.Position()
	if fecha.Y < nombre.Y || fecha.Y > nombre.Y+8 {
		t.Errorf("la fecha está en y=%g y el remitente en y=%g: van en la misma línea", fecha.Y, nombre.Y)
	}
	clip := r.clip.Objeto().Position()
	asunto := r.asunto.Position()
	if clip.X+TallaClip > derecha+0.5 || clip.Y < asunto.Y-4 || clip.Y > asunto.Y+8 {
		t.Errorf("el clip está en %v y el asunto en %v: va a la derecha de su línea", clip, asunto)
	}
	if got := AnchoTextoFila(400); derecha-asunto.X-got > 0.5 || got-(derecha-asunto.X) > 0.5 {
		t.Errorf("la columna de texto mide %g y llega al margen (%g)", got, derecha-asunto.X)
	}
	if r.avatar.Foto != f.Logo || !r.avatar.Logo {
		t.Error("el logo del remitente va en el avatar, en modo Logo")
	}
}
