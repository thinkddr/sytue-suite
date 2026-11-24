// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	fynetest "fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvaulttest"
)

var scopeBoveda = storage.Scope{Tenant: "acme", Subject: "persona-1", Product: "notes"}

// banco de pruebas: la puerta contra el servidor falso, con el «hilo de la
// interfaz» en manos de la prueba.
type bancoBoveda struct {
	t      *testing.T
	fake   *pqvaulttest.Fake
	puerta *PuertaBoveda
	ui     chan func()
	clave  []byte
	listas []*pqvault.Vault
	entrar func(context.Context) error
	cambio bool
}

func nuevoBancoBoveda(t *testing.T) *bancoBoveda {
	t.Helper()
	fynetest.NewTempApp(t)
	fake, drive, account := pqvaulttest.New(t, scopeBoveda)
	b := &bancoBoveda{t: t, fake: fake, ui: make(chan func(), 8), clave: claveBoveda(1)}
	puerta, err := NuevaPuertaBoveda(PuertaBovedaConfig{
		Scope: scopeBoveda, Server: drive, Registry: account,
		Clave: func() []byte { return bytes.Clone(b.clave) },
		Entrar: func(ctx context.Context) error {
			if b.entrar != nil {
				return b.entrar(ctx)
			}
			return nil
		},
		Intencion: func(ctx context.Context) (context.Context, error) {
			if b.cambio {
				return nil, auth.ErrAuthorizationChanged
			}
			return ctx, nil
		},
		Lista:     func(_ context.Context, v *pqvault.Vault) { b.listas = append(b.listas, v) },
		Translate: func(k string) string { return k },
		Dispatch:  func(f func()) { b.ui <- f },
	})
	if err != nil {
		t.Fatal(err)
	}
	b.puerta = puerta
	t.Cleanup(func() {
		puerta.Cerrar()
		for _, v := range b.listas {
			v.Close()
		}
	})
	return b
}

func claveBoveda(n byte) []byte { return bytes.Repeat([]byte{n}, recoverywrap.UnlockKeyLen) }

// esperar ejecuta en el «hilo de la interfaz» lo que la puerta le mande al acabar.
func (b *bancoBoveda) esperar() EstadoBoveda {
	b.t.Helper()
	select {
	case f := <-b.ui:
		f()
	case <-time.After(10 * time.Second):
		b.t.Fatal("la puerta no terminó")
	}
	return b.puerta.Estado()
}

// sembrar deja en el servidor una bóveda con datos sellada con otra clave.
func (b *bancoBoveda) sembrar(clave []byte) {
	b.t.Helper()
	vault, err := pqvault.Open(b.t.Context(), pqvault.Config{Scope: scopeBoveda, Server: b.puerta.cfg.Server, Registry: b.puerta.cfg.Registry, UnlockKey: clave})
	if err != nil {
		b.t.Fatal(err)
	}
	defer vault.Close()
	rec := storage.Record{Version: 1, Scope: scopeBoveda, ObjectID: "note_1", RevisionID: "rev_1", Role: "content", Payload: []byte(`{"t":"hola"}`)}
	if _, err := vault.Put(b.t.Context(), "txn_1", rec); err != nil {
		b.t.Fatal(err)
	}
}

func TestPuertaBovedaAbreYEntregaLaBoveda(t *testing.T) {
	b := nuevoBancoBoveda(t)
	b.puerta.Iniciar(t.Context())
	if b.puerta.Estado() != BovedaAbriendo || !b.puerta.vista.progreso.Visible() {
		t.Fatal("mientras abre, lo dice")
	}
	if got := b.esperar(); got != BovedaLista || len(b.listas) != 1 || b.listas[0] == nil {
		t.Fatalf("la bóveda nueva se crea y se entrega: %v", got)
	}
	if b.puerta.vista.otraVez.Visible() || b.puerta.vista.vaciar.Visible() || b.puerta.vista.progreso.Visible() {
		t.Fatal("abierta, no ofrece salidas")
	}
}

func TestPuertaBovedaSinClavePideEntrarYLuegoAbre(t *testing.T) {
	b := nuevoBancoBoveda(t)
	b.clave = nil
	b.puerta.Iniciar(t.Context())
	if got := b.esperar(); got != BovedaEntrar || !b.puerta.vista.entrar.Visible() {
		t.Fatalf("sin la clave en el dispositivo hay que entrar en Sytue ID: %v", got)
	}
	b.entrar = func(context.Context) error {
		b.clave = claveBoveda(1)
		return nil
	}
	fynetest.Tap(b.puerta.vista.entrar)
	if got := b.esperar(); got != BovedaLista || len(b.listas) != 1 {
		t.Fatalf("tras entrar, la bóveda se abre sin más pasos: %v", got)
	}
}

func TestPuertaBovedaClaveQueNoAbreOfreceEntrarOVaciar(t *testing.T) {
	b := nuevoBancoBoveda(t)
	b.sembrar(claveBoveda(9))
	b.puerta.Iniciar(t.Context())
	if got := b.esperar(); got != BovedaNoAbre {
		t.Fatalf("con datos, una clave que no abre no se da por buena: %v", got)
	}
	v := b.puerta.vista
	if v.mensaje.Text != "vault.wrongKeyDevice" || !v.otraVez.Visible() || !v.vaciar.Visible() || !v.reintenta.Visible() || v.confirmar.Visible() {
		t.Fatal("hay que ofrecer volver a entrar, vaciar y reintentar")
	}
	if b.fake.RecordCount() != 1 {
		t.Fatal("abrir con otra clave no toca los datos")
	}
	// Sin la confirmación, vaciar no hace nada.
	b.puerta.Vaciar(true)
	fynetest.Tap(v.vaciar)
	if !v.confirmar.Visible() || !v.vaciarYa.Disabled() {
		t.Fatal("vaciar pide antes marcar la casilla")
	}
	fynetest.Tap(v.vaciarYa)
	b.puerta.Vaciar(false)
	if b.puerta.Estado() != BovedaNoAbre || b.fake.RecordCount() != 1 {
		t.Fatal("sin la casilla marcada no se vacía nada")
	}
	fynetest.Tap(v.entiendo)
	if v.vaciarYa.Disabled() {
		t.Fatal("con la casilla marcada, se puede vaciar")
	}
	fynetest.Tap(v.vaciarYa)
	if got := b.esperar(); got != BovedaLista || len(b.listas) != 1 {
		t.Fatalf("vaciar estrena la bóveda y la entrega: %v", got)
	}
	if b.fake.RecordCount() != 0 {
		t.Fatal("vaciar borra los datos que ya nadie puede abrir")
	}
}

func TestPuertaBovedaVaciaSeReiniciaSola(t *testing.T) {
	b := nuevoBancoBoveda(t)
	old, err := pqvault.Open(t.Context(), pqvault.Config{Scope: scopeBoveda, Server: b.puerta.cfg.Server, UnlockKey: claveBoveda(9)})
	if err != nil {
		t.Fatal(err)
	}
	old.Close()
	b.puerta.Iniciar(t.Context())
	if got := b.esperar(); got != BovedaLista {
		t.Fatalf("sin nada que perder, no se pregunta: %v", got)
	}
	if b.listas[0].VaultID() == old.VaultID() {
		t.Fatal("la bóveda se reinició")
	}
}

func TestPuertaBovedaFalloOfreceReintentar(t *testing.T) {
	b := nuevoBancoBoveda(t)
	b.puerta.cfg.abrir = func(context.Context, pqvault.Config) (*pqvault.Vault, error) {
		return nil, errors.New("sin red")
	}
	b.puerta.Iniciar(t.Context())
	if got := b.esperar(); got != BovedaFallo || !b.puerta.vista.reintenta.Visible() || !b.puerta.vista.otraVez.Visible() || !b.puerta.vista.vaciar.Visible() {
		t.Fatalf("ante cualquier otro fallo, las mismas salidas: %v", got)
	}
	b.puerta.cfg.abrir = pqvault.Open
	fynetest.Tap(b.puerta.vista.reintenta)
	if got := b.esperar(); got != BovedaLista {
		t.Fatalf("reintentar abre: %v", got)
	}
}

func TestPuertaBovedaOtraCuentaLaCierra(t *testing.T) {
	b := nuevoBancoBoveda(t)
	b.cambio = true
	b.puerta.Iniciar(t.Context())
	if got := b.esperar(); got != BovedaCerrada || len(b.listas) != 0 {
		t.Fatalf("con otra cuenta en la sesión no se abre nada: %v", got)
	}
	b.cambio = false
	b.puerta.Reintentar()
	select {
	case <-b.ui:
		t.Fatal("una puerta cerrada no vuelve a abrir")
	case <-time.After(50 * time.Millisecond):
	}
}

// Vaciar sin la clave en el dispositivo no toca nada (ni suelta la fuente): pide
// entrar, y al volver de Sytue ID termina de vaciar en vez de abrir la bóveda de antes,
// que volvería a fallar. La web lo guarda como intención (sytue.vault-reset).
func TestPuertaBovedaVaciarSinClaveTerminaAlVolverAEntrar(t *testing.T) {
	b := nuevoBancoBoveda(t)
	antes := 0
	b.puerta.cfg.AntesDeVaciar = func(context.Context) error {
		antes++
		return nil
	}
	b.sembrar(claveBoveda(9))
	b.puerta.Iniciar(t.Context())
	if got := b.esperar(); got != BovedaNoAbre {
		t.Fatalf("estado: %v", got)
	}
	b.clave = nil
	b.puerta.PedirVaciado()
	b.puerta.Vaciar(true)
	if got := b.esperar(); got != BovedaEntrar || !b.puerta.vista.entrar.Visible() {
		t.Fatalf("sin clave, vaciar pide entrar: %v", got)
	}
	if antes != 0 || b.fake.RecordCount() != 1 {
		t.Fatal("sin clave no se suelta la fuente ni se vacía nada")
	}
	// Entrar falla (se cancela): el vaciado sigue pendiente.
	b.entrar = func(context.Context) error { return errors.New("cancelado") }
	fynetest.Tap(b.puerta.vista.entrar)
	if got := b.esperar(); got != BovedaFallo {
		t.Fatalf("estado: %v", got)
	}
	b.entrar = func(context.Context) error {
		b.clave = claveBoveda(1)
		return nil
	}
	fynetest.Tap(b.puerta.vista.otraVez)
	if got := b.esperar(); got != BovedaLista || len(b.listas) != 1 {
		t.Fatalf("al volver a entrar se termina de vaciar: %v", got)
	}
	if antes != 1 || b.fake.RecordCount() != 0 {
		t.Fatalf("vació (%d registros) tras soltar la fuente %d vez", b.fake.RecordCount(), antes)
	}
}

// Reintentar es abrir, no vaciar: olvida un vaciado pendiente de entrar.
func TestPuertaBovedaReintentarOlvidaElVaciadoPendiente(t *testing.T) {
	b := nuevoBancoBoveda(t)
	b.sembrar(claveBoveda(9))
	b.puerta.Iniciar(t.Context())
	b.esperar()
	b.clave = nil
	b.puerta.PedirVaciado()
	b.puerta.Vaciar(true)
	if got := b.esperar(); got != BovedaEntrar {
		t.Fatalf("estado: %v", got)
	}
	fynetest.Tap(b.puerta.vista.reintenta)
	if got := b.esperar(); got != BovedaEntrar {
		t.Fatalf("estado: %v", got)
	}
	b.entrar = func(context.Context) error {
		b.clave = claveBoveda(9)
		return nil
	}
	fynetest.Tap(b.puerta.vista.entrar)
	if got := b.esperar(); got != BovedaLista || b.fake.RecordCount() != 1 {
		t.Fatalf("tras reintentar, entrar abre la bóveda de antes sin vaciarla: %v", got)
	}
}

// Si AntesDeVaciar falla (Drive no pudo soltar su fuente), no se vacía nada y no
// queda nada pendiente.
func TestPuertaBovedaAntesDeVaciarFallaNoVacia(t *testing.T) {
	b := nuevoBancoBoveda(t)
	b.puerta.cfg.AntesDeVaciar = func(context.Context) error { return pqvault.ErrSignin }
	b.sembrar(claveBoveda(9))
	b.puerta.Iniciar(t.Context())
	b.esperar()
	b.puerta.PedirVaciado()
	b.puerta.Vaciar(true)
	b.esperar()
	if b.fake.RecordCount() != 1 || b.puerta.vaciarAlEntrar {
		t.Fatal("un fallo antes de vaciar vació o dejó el vaciado pendiente")
	}
}

// IniciarVaciando: la app ya confirmó (la bóveda abría, pero su contenido no cargaba).
func TestPuertaBovedaIniciarVaciando(t *testing.T) {
	b := nuevoBancoBoveda(t)
	antes := 0
	b.puerta.cfg.AntesDeVaciar = func(context.Context) error {
		antes++
		return nil
	}
	b.sembrar(claveBoveda(1))
	b.puerta.IniciarVaciando(t.Context())
	if got := b.esperar(); got != BovedaLista || antes != 1 || b.fake.RecordCount() != 0 {
		t.Fatalf("iniciar vaciando suelta la fuente, vacía y entrega: %v %d %d", got, antes, b.fake.RecordCount())
	}
}
