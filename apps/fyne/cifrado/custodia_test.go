// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"testing"

	"maunium.net/go/mautrix"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	claveDePrueba  = "EsTb 1234 5678 9ABC DEFG HJKL MNPQ RSTU VWXY Zabc defg hijk"
	claveNueva     = "EsTc 1234 5678 9ABC DEFG HJKL MNPQ RSTU VWXY Zabc defg hijk"
	cuentaDePrueba = "@sytue_storage_drive_acme_p:matrix.test"
)

// clienteFalso hace de *mx.Client. Recover solo acepta `valida`.
// BootstrapRecoveryWrapped y ResetRecoveryWrapped llaman a la custodia con la
// clave nueva y solo «publican» (cambian la cuenta) si la custodia la aceptó.
type clienteFalso struct {
	status     mx.VerificationStatus
	keyID      string
	valida     string
	local      string
	recuperado []string
	publicadas int
	ackeadas   int
}

func (f *clienteFalso) VerificationStatus(context.Context) (mx.VerificationStatus, error) {
	return f.status, nil
}

func (f *clienteFalso) Recover(_ context.Context, key string) error {
	f.recuperado = append(f.recuperado, key)
	if key != f.valida {
		return errors.New("wrong key")
	}
	f.status = mx.VerificationStatus{HasIdentity: true, Verified: true}
	return nil
}

func (f *clienteFalso) publicar(ctx context.Context, escrow mx.RecoveryEscrow, keyID, key string) error {
	if escrow == nil {
		return mx.ErrNoRecoveryEscrow
	}
	if err := escrow(ctx, keyID, key); err != nil {
		return err
	}
	f.publicadas++
	f.keyID, f.valida, f.local = keyID, key, key
	f.status = mx.VerificationStatus{HasIdentity: true, Verified: true}
	return nil
}

func (f *clienteFalso) BootstrapRecoveryWrapped(ctx context.Context, _ mautrix.UIACallback, escrow mx.RecoveryEscrow) error {
	return f.publicar(ctx, escrow, "k-nueva", claveNueva)
}

func (f *clienteFalso) ResetRecoveryWrapped(ctx context.Context, _ mautrix.UIACallback, escrow mx.RecoveryEscrow) error {
	return f.publicar(ctx, escrow, "k-reset", claveNueva)
}

func (f *clienteFalso) AcknowledgeRecoverySetup(context.Context) error { f.ackeadas++; return nil }

func (f *clienteFalso) DefaultRecoveryKeyID(context.Context) (string, error) { return f.keyID, nil }

func (f *clienteFalso) LocalRecoveryKey(context.Context) (string, string, error) {
	if f.local == "" {
		return "", "", mx.ErrRecoveryRequired
	}
	return f.keyID, f.local, nil
}

func (f *clienteFalso) AccountUserID() string { return cuentaDePrueba }

// servicioFalso es Rooms con /matrix/recovery-wrappers y la copia de ADR 0081.
type servicioFalso struct {
	envoltorios []recoverywrap.Wrapper
	cuentas     []api.MatrixRecoveryAccount
	legacy      string
	errPut      error
}

func (s *servicioFalso) MatrixRecoveryWrappers(_ context.Context, account api.MatrixRecoveryAccount) (api.MatrixRecoveryWrappers, error) {
	s.cuentas = append(s.cuentas, account)
	return api.MatrixRecoveryWrappers{Wrappers: s.envoltorios, LegacyEscrow: s.legacy != ""}, nil
}

func (s *servicioFalso) PutMatrixRecoveryWrapper(_ context.Context, account api.MatrixRecoveryAccount, w recoverywrap.Wrapper) (string, error) {
	s.cuentas = append(s.cuentas, account)
	if s.errPut != nil {
		return "", s.errPut
	}
	if err := w.Validate(); err != nil {
		return "", err
	}
	kept := s.envoltorios[:0]
	for _, old := range s.envoltorios {
		if old.KeyID == w.KeyID && old.Kind != w.Kind {
			kept = append(kept, old)
		}
	}
	kept = append(kept, w)
	s.envoltorios, s.legacy = kept, ""
	return "w", nil
}

func (s *servicioFalso) MatrixRecoveryKey(context.Context) (string, error) { return s.legacy, nil }

var cuentaAislada = api.MatrixRecoveryAccount{Product: "drive", IdentityVersion: storage.IsolatedIdentity}

func envoltorio(t *testing.T, password, keyID, key string) recoverywrap.Wrapper {
	t.Helper()
	w, err := recoverywrap.WrapPassword(password, cuentaDePrueba, keyID, key)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// Dispositivo nuevo con la clave envuelta: se pide la contraseña (nunca la clave)
// y con ella se abre. La equivocada no llega ni a intentar recuperar.
func TestCustodiaDesbloqueaConLaContrasenaYNuncaPideLaClave(t *testing.T) {
	client := &clienteFalso{status: mx.VerificationStatus{HasIdentity: true}, keyID: "k1", valida: claveDePrueba}
	service := &servicioFalso{envoltorios: []recoverywrap.Wrapper{envoltorio(t, "secreta", "k0", "vieja"), envoltorio(t, "secreta", "k1", claveDePrueba)}}
	d, err := Diagnosticar(t.Context(), client, Custodia{Servicio: service, Cuenta: cuentaAislada})
	if err != nil || d.Paso != PasoDesbloquear || len(d.Envoltorios) != 1 {
		t.Fatalf("Diagnosticar = %+v, %v; quiero desbloquear con el envoltorio de k1", d, err)
	}
	if service.cuentas[0] != cuentaAislada {
		t.Fatalf("se pidió otra cuenta: %+v", service.cuentas)
	}
	if _, err := Desbloquear(t.Context(), client, d.Envoltorios, "otra"); !errors.Is(err, recoverywrap.ErrWrongSecret) || len(client.recuperado) != 0 {
		t.Fatalf("contraseña equivocada: %v, recuperado=%v", err, client.recuperado)
	}
	if _, err := Desbloquear(t.Context(), client, d.Envoltorios, "secreta"); err != nil || !client.status.Verified {
		t.Fatalf("Desbloquear: %v", err)
	}
}

// Cuenta sin cifrado: se crea con la contraseña, la clave va envuelta al servicio
// ANTES de publicarse, no se devuelve para enseñarla y se cierra la configuración.
func TestCustodiaCreaYEnvuelveAntesDePublicarSinEnsenarLaClave(t *testing.T) {
	client := &clienteFalso{}
	service := &servicioFalso{}
	custodia := Custodia{Servicio: service, Cuenta: cuentaAislada}
	d, err := Diagnosticar(t.Context(), client, custodia)
	if err != nil || d.Paso != PasoCrear {
		t.Fatalf("Diagnosticar = %+v, %v; quiero crear", d, err)
	}
	if err := Crear(t.Context(), client, custodia, "secreta", nil); err != nil {
		t.Fatal(err)
	}
	if client.publicadas != 1 || client.ackeadas != 1 || len(service.envoltorios) != 1 {
		t.Fatalf("publicadas=%d ack=%d envoltorios=%d", client.publicadas, client.ackeadas, len(service.envoltorios))
	}
	if got, err := recoverywrap.UnwrapPassword("secreta", cuentaDePrueba, service.envoltorios[0]); err != nil || got != claveNueva {
		t.Fatalf("lo custodiado no abre la clave nueva: %v", err)
	}
	if d, err := Diagnosticar(t.Context(), client, custodia); err != nil || d.Paso != PasoListo {
		t.Fatalf("tras crear: %+v, %v", d, err)
	}
}

// Si el servicio no acepta el envoltorio, no se publica nada ni se enseña la
// clave: el paso sigue siendo crear y se puede reintentar.
func TestCustodiaFallidaNoPublicaYSePuedeReintentar(t *testing.T) {
	client := &clienteFalso{}
	service := &servicioFalso{errPut: errors.New("503")}
	custodia := Custodia{Servicio: service, Cuenta: cuentaAislada}
	if err := Crear(t.Context(), client, custodia, "secreta", nil); err == nil {
		t.Fatal("se creó el cifrado sin custodia")
	}
	if client.publicadas != 0 || client.ackeadas != 0 || client.status.HasIdentity {
		t.Fatal("se publicó una clave que nadie podrá volver a abrir")
	}
	if d, _ := Diagnosticar(t.Context(), client, custodia); d.Paso != PasoCrear {
		t.Fatalf("tras el fallo el paso es %v; quiero crear otra vez", d.Paso)
	}
	if err := Crear(t.Context(), client, Custodia{}, "secreta", nil); !errors.Is(err, ErrSinCustodia) {
		t.Fatalf("sin servicio: %v", err)
	}
	service.errPut = nil
	if err := Crear(t.Context(), client, custodia, "secreta", nil); err != nil || client.publicadas != 1 {
		t.Fatalf("reintento: %v", err)
	}
}

// Una cuenta con una clave que el servicio no conoce no pide ninguna clave: solo
// verificar el dispositivo. Ya verificado, si este dispositivo tiene la clave (las
// cuentas de almacenamiento la guardan), se pide la contraseña para envolverla.
func TestClaveDesconocidaSoloVerificaYDespuesSeEnvuelve(t *testing.T) {
	client := &clienteFalso{status: mx.VerificationStatus{HasIdentity: true}, keyID: "k1"}
	service := &servicioFalso{}
	custodia := Custodia{Servicio: service, Cuenta: cuentaAislada}
	d, err := Diagnosticar(t.Context(), client, custodia)
	if err != nil || d.Paso != PasoVerificar || len(client.recuperado) != 0 {
		t.Fatalf("Diagnosticar = %+v, %v; quiero verificar sin tocar nada", d, err)
	}
	// Verificado desde otro dispositivo: la clave local se envuelve.
	client.status, client.local = mx.VerificationStatus{HasIdentity: true, Verified: true}, claveDePrueba
	d, err = Diagnosticar(t.Context(), client, custodia)
	if err != nil || d.Paso != PasoProteger {
		t.Fatalf("verificado con la clave local: %+v, %v; quiero proteger", d, err)
	}
	if err := Proteger(t.Context(), client, custodia, d, "secreta"); err != nil || len(service.envoltorios) != 1 {
		t.Fatalf("Proteger: %v", err)
	}
	if got, _ := recoverywrap.UnwrapPassword("secreta", cuentaDePrueba, service.envoltorios[0]); got != claveDePrueba {
		t.Fatal("se envolvió otra clave")
	}
	if d, _ := Diagnosticar(t.Context(), client, custodia); d.Paso != PasoListo {
		t.Fatalf("ya envuelta: %v", d.Paso)
	}
}

// Transición desde ADR 0081: con la copia con KEK el dispositivo se recupera solo,
// sin que nadie vea ni teclee la clave, y se pide la contraseña para envolverla
// (al guardar el envoltorio el servicio borra la copia con KEK).
func TestLaCopiaConKEKRecuperaSolaYPasaASerUnEnvoltorio(t *testing.T) {
	client := &clienteFalso{status: mx.VerificationStatus{HasIdentity: true}, keyID: "k1", valida: claveDePrueba}
	service := &servicioFalso{legacy: claveDePrueba}
	custodia := Custodia{Servicio: service}
	d, err := Diagnosticar(t.Context(), client, custodia)
	if err != nil || d.Paso != PasoProteger || !d.Status.Verified || len(client.recuperado) != 1 {
		t.Fatalf("Diagnosticar = %+v, %v", d, err)
	}
	if err := Proteger(t.Context(), client, custodia, d, "secreta"); err != nil || service.legacy != "" {
		t.Fatalf("la copia con KEK sigue: %v", err)
	}
	// Una copia que no abre nada no se envuelve: se queda en verificar.
	client = &clienteFalso{status: mx.VerificationStatus{HasIdentity: true}, keyID: "k1", valida: "otra"}
	if d, _ := Diagnosticar(t.Context(), client, Custodia{Servicio: &servicioFalso{legacy: claveDePrueba}}); d.Paso != PasoVerificar {
		t.Fatalf("copia que no abre: %v", d.Paso)
	}
}

// Restablecer (Rooms): la clave nueva va envuelta antes de publicarse; sin
// custodia no se restablece nada.
func TestRestablecerEnvuelveAntesDePublicar(t *testing.T) {
	client := &clienteFalso{status: mx.VerificationStatus{HasIdentity: true}, keyID: "k1"}
	failing := &servicioFalso{errPut: errors.New("503")}
	if err := Restablecer(t.Context(), client, Custodia{Servicio: failing}, "secreta", nil); err == nil || client.publicadas != 0 {
		t.Fatal("se restableció sin custodia")
	}
	service := &servicioFalso{envoltorios: []recoverywrap.Wrapper{envoltorio(t, "secreta", "k1", claveDePrueba)}}
	if err := Restablecer(t.Context(), client, Custodia{Servicio: service}, "secreta", nil); err != nil || client.publicadas != 1 {
		t.Fatalf("Restablecer: %v", err)
	}
	if len(service.envoltorios) != 1 || service.envoltorios[0].KeyID != "k-reset" {
		t.Fatalf("quedan envoltorios de la clave vieja: %+v", service.envoltorios)
	}
}
