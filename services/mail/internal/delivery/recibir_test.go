package delivery

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
		t.Fatalf("lo releído de la sección no es lo recibido: %q (err=%v)", leido, err)
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

// Tras la reserva previa, MIME, DKIM y blobs usan secciones independientes del
// mismo búfer RAM. No hacen una segunda copia completa ni un temporal en claro.
func TestProcesadoEnFlujoDesdeMemoriaAcotada(t *testing.T) {
	in, tmp := inboundDe(t, 4096)
	eml := "From: Ana <ana@acme.example>\r\n" +
		"To: Luis <luis@sytue.eu>\r\n" +
		"Subject: Flujo sin copia\r\n" +
		"Message-ID: <flujo-1@acme.example>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"Cuerpo procesado en flujo sin duplicar el mensaje ni escribirlo en disco.\r\n"

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
	if env.Subject != "Flujo sin copia" || env.FromAddr != "ana@acme.example" || env.SizeBytes != int64(len(eml)) {
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

// Con todas las recepciones ocupadas, la siguiente se rechaza antes de leer.
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

// The unknown-length network reader must not be touched while its allocation is
// unavailable. Cancellation releases the admission slot without reading DATA.
type countedInboundReader struct {
	reader io.Reader
	reads  atomic.Int32
}

func (r *countedInboundReader) Read(out []byte) (int, error) {
	r.reads.Add(1)
	return r.reader.Read(out)
}

func TestRecibirReservesBeforeReadingAndHonorsCancellation(t *testing.T) {
	in, tmp := inboundDe(t, 8)
	first, release, err := in.recibir(context.Background(), bytes.NewReader([]byte("12345678")))
	if err != nil {
		t.Fatal(err)
	}
	reader := &countedInboundReader{reader: bytes.NewReader([]byte("secret"))}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer release()
	finished := make(chan error, 1)
	go func() {
		_, done, err := in.recibir(ctx, reader)
		if done != nil {
			done()
		}
		finished <- err
	}()
	select {
	case err := <-finished:
		t.Fatalf("admitted without memory: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if reader.reads.Load() != 0 {
		t.Fatal("read body before memory reservation")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("admission ignored cancel")
	}
	if reader.reads.Load() != 0 {
		t.Fatal("canceled admission read body")
	}
	release()
	release()
	if !bytes.Equal(first.raw, make([]byte, len(first.raw))) {
		t.Fatal("release retained plaintext")
	}
	sinTemporales(t, tmp)
	_, done, err := in.recibir(context.Background(), bytes.NewReader([]byte("12345678")))
	if err != nil {
		t.Fatal(err)
	}
	done()
}

func TestRecibirCancelledOrBusyNeverReads(t *testing.T) {
	in, tmp := inboundDe(t, 8)
	reader := &countedInboundReader{reader: bytes.NewReader([]byte("secret"))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := in.recibir(ctx, reader); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	receptions, _ := in.cupos()
	if !receptions.TryAcquire(recepcionesALaVez) {
		t.Fatal("reserve")
	}
	if _, _, err := in.recibir(context.Background(), reader); !errors.Is(err, errOcupado) {
		t.Fatal(err)
	}
	receptions.Release(recepcionesALaVez)
	if reader.reads.Load() != 0 {
		t.Fatal("rejected request read body")
	}
	sinTemporales(t, tmp)
}

func TestRecibirUnknownLengthIsBoundedAndReleasesAfterReadError(t *testing.T) {
	in, tmp := inboundDe(t, 8)
	reader := &countedInboundReader{reader: bytes.NewReader([]byte("123456789more"))}
	if _, _, err := in.recibir(context.Background(), reader); !errors.Is(err, domain.ErrMessageTooLarge) {
		t.Fatal(err)
	}
	if reader.reader.(*bytes.Reader).Len() != 4 {
		t.Fatal("read beyond one overflow byte")
	}
	for _, body := range []io.Reader{io.MultiReader(bytes.NewReader([]byte("1234")), failingInboundReader{}), bytes.NewReader([]byte("12345678"))} {
		raw, release, err := in.recibir(context.Background(), body)
		if err == nil {
			if raw.Size() != 8 {
				t.Fatal("short success")
			}
			release()
		} else if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal(err)
		}
	}
	sinTemporales(t, tmp)
}

type failingInboundReader struct{}

func (failingInboundReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestRecibirCancellationClosesActiveNetworkReader(t *testing.T) {
	in, tmp := inboundDe(t, 8)
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() {
		_, done, err := in.recibir(ctx, reader)
		if done != nil {
			done()
		}
		finished <- err
	}()
	if _, err := writer.Write([]byte("123")); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("network reader not closed")
	}
	_, done, err := in.recibir(context.Background(), bytes.NewReader([]byte("12345678")))
	if err != nil {
		t.Fatal(err)
	}
	done()
	sinTemporales(t, tmp)
}

type retainedFailingReader struct{ allocated []byte }

func (r *retainedFailingReader) Read(out []byte) (int, error) {
	r.allocated = out
	return copy(out, "private"), io.ErrClosedPipe
}

func TestRecibirClearsAllocationAfterBodyReadFailure(t *testing.T) {
	in, _ := inboundDe(t, 8)
	reader := &retainedFailingReader{}
	if _, _, err := in.recibir(t.Context(), reader); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	if len(reader.allocated) != 8 || !bytes.Equal(reader.allocated, make([]byte, 8)) {
		t.Fatal("failed receive retained plaintext")
	}
	_, release, err := in.recibir(t.Context(), bytes.NewReader([]byte("12345678")))
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestRecibirPlaintextDoesNotDependOnTemporaryFilesystem(t *testing.T) {
	in, _ := inboundDe(t, 8)
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing-directory"))
	body, release, err := in.recibir(t.Context(), strings.NewReader("private"))
	if err != nil {
		t.Fatal("plaintext receiver tried to open a filesystem spool", err)
	}
	defer release()
	raw, err := io.ReadAll(body.section())
	if err != nil || string(raw) != "private" {
		t.Fatal("bounded RAM source lost original", err)
	}
}
