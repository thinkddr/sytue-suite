package delivery

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
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

	raw, soltar, err := in.recibir(context.Background(), bytes.NewReader([]byte("12345678")))
	if err != nil {
		t.Fatalf("un mensaje del tamaño justo tiene que entrar: %v", err)
	}
	defer soltar()
	if string(raw) != "12345678" {
		t.Fatalf("lo releído del temporal no es lo recibido: %q", raw)
	}
	sinTemporales(t, tmp)
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
