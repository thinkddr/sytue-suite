// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/eml"
)

// conAdjuntos es un .eml con un PDF y la firma S/MIME del sobre.
const conAdjuntos = "Subject: Presupuesto\r\nMIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=\"b\"\r\n\r\n" +
	"--b\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nOs lo adjunto.\r\n" +
	"--b\r\nContent-Type: application/pdf; name=\"presupuesto.pdf\"\r\nContent-Disposition: attachment; filename=\"presupuesto.pdf\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\nJVBERi0xLjcK\r\n" +
	"--b\r\nContent-Type: application/pkcs7-signature; name=\"smime.p7s\"\r\nContent-Transfer-Encoding: base64\r\n\r\nAAAA\r\n" +
	"--b--\r\n"

// TestLasFirmasDeSobreNoSonAdjuntos: la firma S/MIME viaja como una parte más y no se
// ofrece para bajar; el PDF sí, con su posición ORIGINAL en el mensaje.
func TestLasFirmasDeSobreNoSonAdjuntos(t *testing.T) {
	m, err := eml.Leer(strings.NewReader(conAdjuntos))
	if err != nil {
		t.Fatal(err)
	}
	vis := Visibles(m.Adjuntos)
	if len(m.Adjuntos) != 2 || len(vis) != 1 || vis[0].Nombre != "presupuesto.pdf" || vis[0].Indice != 0 {
		t.Fatalf("partes %d, visibles %+v", len(m.Adjuntos), vis)
	}
	if !EsFirmaDeSobre(eml.Adjunto{Nombre: "x.bin", Tipo: "application/pgp-signature; name=x"}) ||
		!EsFirmaDeSobre(eml.Adjunto{Nombre: "signature.asc", Tipo: "application/octet-stream"}) ||
		EsFirmaDeSobre(eml.Adjunto{Nombre: "informe.pdf", Tipo: "application/pdf"}) {
		t.Error("la firma de sobre se reconoce mal por tipo o por nombre")
	}
}

type driveFalso struct{ nombre string }

func (d *driveFalso) GuardarAdjunto(_ context.Context, nombre, _ string, _ []byte) (api.Referencia, error) {
	d.nombre = nombre
	return api.Referencia{ID: "f1", Ruta: "/Attachments/presupuesto (2).pdf"}, nil
}

// TestGuardarEnDriveEsExplicitoYDiceDondeQueda: el adjunto va a Drive SOLO al pulsar,
// y el aviso dice el nombre GUARDADO, que no siempre es el del adjunto.
func TestGuardarEnDriveEsExplicitoYDiceDondeQueda(t *testing.T) {
	s := servicioConTodas()
	s.paginas[""][0].HasAttachment = true
	s.crudo["m1"] = conAdjuntos
	d := &driveFalso{}
	b := buzonConAvisos(t, s)
	b.ConDrive(d)
	b.Arrancar(context.Background())
	b.Abrir(b.P.Bandeja.Conversaciones[0])
	if d.nombre != "" {
		t.Fatal("se ha guardado en Drive sin que nadie lo pidiera")
	}
	c, _ := b.P.Lector.Cuerpo("m1")
	vis := Visibles(c.Adjuntos)
	if len(vis) != 1 {
		t.Fatalf("el lector tiene %d adjuntos visibles", len(vis))
	}
	b.P.Lector.AlGuardarEnDrive(b.P.Lector.Conversacion.Mensajes[0], vis[0])
	av := b.P.Avisos.Avisos()
	if d.nombre != "presupuesto.pdf" || len(av) != 1 || !strings.Contains(av[0].Descripcion, "presupuesto (2).pdf") {
		t.Errorf("guardado como %q; avisos %+v", d.nombre, av)
	}
	if len(b.P.Lector.Guardando) != 0 {
		t.Error("el botón se queda apagado después de guardar")
	}
}

// TestElNombreDelOriginalEsElDelBFF: el asunto sin lo que un sistema de ficheros no
// admite, y el id cuando no hay asunto.
func TestElNombreDelOriginalEsElDelBFF(t *testing.T) {
	for _, c := range []struct{ asunto, id, quiero string }{
		{"Re: Factura 3/2026 <urgente>", "m1", "Re Factura 3 2026 urgente.eml"},
		{"   ", "m2", "m2.eml"},
	} {
		if got := NombreOriginal(c.asunto, c.id); got != c.quiero {
			t.Errorf("NombreOriginal(%q) = %q y el BFF da %q", c.asunto, got, c.quiero)
		}
	}
}

// TestLaCapturaConAdjuntosSeGuarda deja el PNG del hilo con adjuntos (SYTUE_CAPTURAS).
func TestLaCapturaConAdjuntosSeGuarda(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	for _, c := range []struct {
		nombre string
		tam    fyne.Size
	}{{"escritorio", fyne.NewSize(1280, 744)}, {"movil", fyne.NewSize(360, 740)}} {
		s := servicioConTodas()
		s.paginas[""][0].HasAttachment = true
		s.crudo["m1"] = conAdjuntos
		b := buzonConAvisos(t, s)
		b.ConDrive(&driveFalso{})
		w := test.NewWindow(b.P.Objeto())
		w.SetPadded(false)
		w.Resize(c.tam)
		b.Arrancar(context.Background())
		b.Abrir(b.P.Bandeja.Conversaciones[0])
		b.P.Refrescar()
		guardar(t, filepath.Join(dir, "adjuntos-"+c.nombre+".png"), w.Canvas().Capture())
		w.Close()
	}
}

// TestLaDescargaQueFallaSeDice: con el disco lleno, el adjunto no se da por bajado en
// silencio: avisa con su nombre. Y si lo que falla es el servidor al pedir el original,
// avisa sin culpar al disco.
func TestLaDescargaQueFallaSeDice(t *testing.T) {
	s := servicioConTodas()
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	tr := traductor(t)
	antes := Guardar
	Guardar = func(string, io.Reader) error { return errors.New("no queda espacio") }
	t.Cleanup(func() { Guardar = antes })

	b.descargarAdjunto(Mensaje{}, AdjuntoVisible{Adjunto: eml.Adjunto{Nombre: "informe.pdf"}})
	av := b.P.Avisos.Avisos()
	if len(av) == 0 || av[len(av)-1].Tono != ui.AvisoPeligro || av[len(av)-1].Titulo != tr.T("reader.downloadFailed", nil) ||
		!strings.Contains(av[len(av)-1].Descripcion, "informe.pdf") {
		t.Fatalf("el disco lleno no se dice: %+v", av)
	}
	n := len(b.P.Avisos.Avisos())
	b.descargarOriginal(Mensaje{ID: "no-existe", Asunto: "Hola"}) // el doble contesta 404
	if av := b.P.Avisos.Avisos(); len(av) != n+1 || av[n].Titulo != tr.T("reader.downloadFailed", nil) || av[n].Descripcion != "" {
		t.Errorf("el original que no llega: %+v", av[n:])
	}
}
