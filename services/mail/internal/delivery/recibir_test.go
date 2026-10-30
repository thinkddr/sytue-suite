package delivery

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/auth"
	"git.sytue.eu/sytue/one/services/mail/internal/blobs"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/mime"
)

// COM-06: el MX no pide credenciales, así que lo que un mensaje entrante puede hacerle
// gastar al pod tiene que estar acotado antes de procesarlo.

func inboundDe(t *testing.T, maxBytes int64) (*Inbound, string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	return &Inbound{maxBytes: maxBytes}, tmp
}

func sinTemporales(t *testing.T, dir string) {
	t.Helper()
	quedan, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(quedan) != 0 {
		t.Fatalf("quedan %d temporales en disco tras recibir", len(quedan))
	}
}

func TestRecibirCortaPorTamanoYNoDejaTemporales(t *testing.T) {
	in, tmp := inboundDe(t, 8)

	if _, _, err := in.recibir(context.Background(), bytes.NewReader(make([]byte, 9))); !errors.Is(err, domain.ErrMessageTooLarge) {
		t.Fatalf("9 bytes con tope de 8: err = %v, se esperaba ErrMessageTooLarge", err)
	}
	sinTemporales(t, tmp)

	r, soltar, err := in.recibir(context.Background(), bytes.NewReader([]byte("12345678")))
	if err != nil {
		t.Fatalf("un mensaje del tamaño justo tiene que entrar: %v", err)
	}
	defer soltar()
	leido, err := io.ReadAll(r)
	if err != nil || string(leido) != "12345678" {
		t.Fatalf("lo releído del temporal no es lo recibido: %q (err=%v)", leido, err)
	}
	sinTemporales(t, tmp)
}

type regBlobMemoria struct{ listo bool }

func (r *regBlobMemoria) ReservarBlobCompartido(context.Context, string, string, int64) (bool, bool, error) {
	return false, r.listo, nil
}

func (r *regBlobMemoria) MarcarBlobListo(context.Context, string) error       { r.listo = true; return nil }
func (r *regBlobMemoria) LiberarBlobCompartido(context.Context, string) error { return nil }

type almBlobMemoria struct{ cuerpo []byte }

func (a *almBlobMemoria) Put(_ context.Context, _ tenancy.Tenant, _ string, r io.Reader, _ int64, _ objectstore.PutOptions) error {
	var err error
	a.cuerpo, err = io.ReadAll(r)
	return err
}

// Una vez admitido en el fichero temporal, el parseo MIME, la verificación DKIM y la
// subida al almacén de blobs trabajan en flujo sobre el propio lector del fichero sin
// volcar el mensaje entero a un []byte en memoria (COM-06).
func TestProcesadoEnFlujoDesdeFichero(t *testing.T) {
	in, tmp := inboundDe(t, 4096)
	eml := "From: Ana <ana@acme.example>\r\n" +
		"To: Luis <luis@sytue.eu>\r\n" +
		"Subject: Flujo desde temporal\r\n" +
		"Message-ID: <flujo-1@acme.example>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"Cuerpo procesado desde el fichero temporal sin cargarlo en RAM.\r\n"

	r, soltar, err := in.recibir(context.Background(), strings.NewReader(eml))
	if err != nil {
		t.Fatalf("recibir: %v", err)
	}
	defer soltar()
	sinTemporales(t, tmp)

	env, err := mime.ParseSobre(io.NewSectionReader(r, 0, r.Size()), r.Size())
	if err != nil {
		t.Fatalf("ParseSobre: %v", err)
	}
	if env.Subject != "Flujo desde temporal" || env.FromAddr != "ana@acme.example" || env.SizeBytes != int64(len(eml)) {
		t.Fatalf("sobre inesperado: %+v", env)
	}

	res, doms, err := auth.VerifyDKIMReader(context.Background(), nil, io.NewSectionReader(r, 0, r.Size()))
	if err != nil || res != auth.ResultNone || len(doms) != 0 {
		t.Fatalf("VerifyDKIMReader sin firmas: res=%v doms=%v err=%v", res, doms, err)
	}

	reg, alm := &regBlobMemoria{}, &almBlobMemoria{}
	key, err := blobs.GuardarMensajeDesde(context.Background(), reg, alm, tenancy.Tenant{Slug: "acme"}, io.NewSectionReader(r, 0, r.Size()), r.Size())
	if err != nil {
		t.Fatalf("GuardarMensajeDesde: %v", err)
	}
	if !strings.HasPrefix(key, "shared/mensajes/acme/") || string(alm.cuerpo) != eml {
		t.Fatalf("blob guardado key=%q len=%d", key, len(alm.cuerpo))
	}
}

// Dos mensajes del tamaño máximo no están en memoria a la vez: el segundo espera a que
// el primero termine de procesarse.
func TestRecibirNoSubeAMemoriaMasDeUnMensajeMaximo(t *testing.T) {
	in, _ := inboundDe(t, 8)
	cuerpo := func() *bytes.Reader { return bytes.NewReader(make([]byte, 8)) }

	_, soltarPrimero, err := in.recibir(context.Background(), cuerpo())
	if err != nil {
		t.Fatal(err)
	}
	segundo := make(chan error, 1)
	go func() {
		_, soltar, err := in.recibir(context.Background(), cuerpo())
		if err == nil {
			soltar()
		}
		segundo <- err
	}()
	select {
	case err := <-segundo:
		t.Fatalf("el segundo mensaje subió a memoria con el primero aún dentro (err = %v)", err)
	case <-time.After(150 * time.Millisecond):
	}
	soltarPrimero()
	select {
	case err := <-segundo:
		if err != nil {
			t.Fatalf("al liberar el primero, el segundo tiene que entrar: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("el segundo mensaje no entró tras liberar el primero")
	}
}

// Los pequeños sí conviven: el presupuesto es de bytes, no de mensajes.
func TestRecibirDejaConvivirALosPequenos(t *testing.T) {
	in, _ := inboundDe(t, 8)
	for range 4 {
		_, soltar, err := in.recibir(context.Background(), bytes.NewReader(make([]byte, 2)))
		if err != nil {
			t.Fatalf("cuatro mensajes de 2 bytes caben en 8: %v", err)
		}
		t.Cleanup(soltar)
	}
}

// El disco también está acotado: con todas las recepciones ocupadas, la siguiente se
// rechaza en el acto (451 en el MX) en vez de abrir otro temporal.
func TestRecibirRechazaCuandoNoQuedanRecepciones(t *testing.T) {
	in, tmp := inboundDe(t, 8)
	recepciones, _ := in.cupos()
	if !recepciones.TryAcquire(recepcionesALaVez) {
		t.Fatal("no se pudo ocupar el cupo de recepciones")
	}
	if _, _, err := in.recibir(context.Background(), bytes.NewReader([]byte("hola"))); !errors.Is(err, errOcupado) {
		t.Fatalf("err = %v, se esperaba errOcupado", err)
	}
	sinTemporales(t, tmp)
	recepciones.Release(recepcionesALaVez)
	if _, soltar, err := in.recibir(context.Background(), bytes.NewReader([]byte("hola"))); err != nil {
		t.Fatalf("con hueco tiene que entrar: %v", err)
	} else {
		soltar()
	}
}
