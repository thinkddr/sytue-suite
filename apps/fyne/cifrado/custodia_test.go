// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"bytes"
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
	// abandonada: una cuenta que se quedó a medias (AbandonedRecovery).
	abandonada bool
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

// RestartRecoveryWrapped, como *mx.Client, lo comprueba otra vez y solo sustituye una
// cuenta abandonada.
func (f *clienteFalso) RestartRecoveryWrapped(ctx context.Context, _ mautrix.UIACallback, escrow mx.RecoveryEscrow) error {
	if !f.abandonada {
		return mx.ErrRecoveryExists
	}
	if err := f.publicar(ctx, escrow, "k-rehecha", claveNueva); err != nil {
		return err
	}
	f.abandonada = false
	return nil
}

func (f *clienteFalso) AbandonedRecovery(context.Context) (bool, error) { return f.abandonada, nil }

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

// material es una clave de desbloqueo de prueba, la que trae el inicio de sesión.
func material(t *testing.T) Material {
	t.Helper()
	key, err := recoverywrap.NewUnlockKey()
	if err != nil {
		t.Fatal(err)
	}
	return Material{Clave: key}
}

// envoltorio es el `sytue-id` de la clave `key` con la clave de desbloqueo de `m`.
func envoltorio(t *testing.T, m Material, keyID, key string) recoverywrap.Wrapper {
	t.Helper()
	w, err := recoverywrap.WrapSytueID(m.Clave, cuentaDePrueba, keyID, key)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// envoltorioViejo es uno de contraseña de la ADR 0082.
func envoltorioViejo(t *testing.T, password, keyID, key string) recoverywrap.Wrapper {
	t.Helper()
	w, err := recoverywrap.WrapPassword(password, cuentaDePrueba, keyID, key)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// Dispositivo nuevo con la clave envuelta: se abre con la clave de desbloqueo de la
// sesión, sin pedir nada, y queda verificado. Sin ella, se pide volver a Sytue ID.
func TestCustodiaDesbloqueaSolaConLaClaveDeSytueID(t *testing.T) {
	m := material(t)
	client := &clienteFalso{status: mx.VerificationStatus{HasIdentity: true}, keyID: "k1", valida: claveDePrueba}
	service := &servicioFalso{envoltorios: []recoverywrap.Wrapper{envoltorio(t, m, "k0", "vieja"), envoltorio(t, m, "k1", claveDePrueba)}}
	custodia := Custodia{Servicio: service, Cuenta: cuentaAislada}
	d, err := Diagnosticar(t.Context(), client, custodia)
	if err != nil || d.Paso != PasoDesbloquear || len(d.Envoltorios) != 1 || d.Legado != "" {
		t.Fatalf("Diagnosticar = %+v, %v; quiero desbloquear con el envoltorio de k1", d, err)
	}
	if service.cuentas[0] != cuentaAislada {
		t.Fatalf("se pidió otra cuenta: %+v", service.cuentas)
	}
	// Sin la clave de desbloqueo en el dispositivo: volver a entrar en Sytue ID.
	if got, err := Resolver(t.Context(), client, custodia, d, Material{}, nil); err != nil || got.Paso != PasoIniciarSesion || len(client.recuperado) != 0 {
		t.Fatalf("sin clave: %+v, %v", got, err)
	}
	// Con la de otra persona (o la de antes de empezar de cero): no abre, y queda
	// verificar con otro dispositivo.
	if got, err := Resolver(t.Context(), client, custodia, d, material(t), nil); err != nil || got.Paso != PasoVerificar || len(client.recuperado) != 0 {
		t.Fatalf("otra clave: %+v, %v", got, err)
	}
	got, err := Resolver(t.Context(), client, custodia, d, m, nil)
	if err != nil || got.Paso != PasoListo || !client.status.Verified {
		t.Fatalf("Resolver: %+v, %v", got, err)
	}
}

// Cuenta sin cifrado: se crea sola con la clave de desbloqueo; la clave va envuelta
// al servicio ANTES de publicarse, no se devuelve para enseñarla y se cierra la
// configuración.
func TestCustodiaCreaYEnvuelveAntesDePublicarSinEnsenarLaClave(t *testing.T) {
	m := material(t)
	client := &clienteFalso{}
	service := &servicioFalso{}
	custodia := Custodia{Servicio: service, Cuenta: cuentaAislada}
	d, err := Diagnosticar(t.Context(), client, custodia)
	if err != nil || d.Paso != PasoCrear {
		t.Fatalf("Diagnosticar = %+v, %v; quiero crear", d, err)
	}
	if got, err := Resolver(t.Context(), client, custodia, d, Material{}, nil); err != nil || got.Paso != PasoIniciarSesion || client.publicadas != 0 {
		t.Fatalf("sin clave de desbloqueo no se crea nada: %+v, %v", got, err)
	}
	d, err = Resolver(t.Context(), client, custodia, d, m, nil)
	if err != nil || d.Paso != PasoListo {
		t.Fatalf("Resolver: %+v, %v", d, err)
	}
	if client.publicadas != 1 || client.ackeadas != 1 || len(service.envoltorios) != 1 {
		t.Fatalf("publicadas=%d ack=%d envoltorios=%d", client.publicadas, client.ackeadas, len(service.envoltorios))
	}
	if got, err := recoverywrap.UnwrapSytueID(m.Clave, cuentaDePrueba, service.envoltorios[0]); err != nil || got != claveNueva {
		t.Fatalf("lo custodiado no abre la clave nueva: %v", err)
	}
}

// Si el servicio no acepta el envoltorio, no se publica nada ni se enseña la
// clave: el paso sigue siendo crear y se puede reintentar.
func TestCustodiaFallidaNoPublicaYSePuedeReintentar(t *testing.T) {
	m := material(t)
	client := &clienteFalso{}
	service := &servicioFalso{errPut: errors.New("503")}
	custodia := Custodia{Servicio: service, Cuenta: cuentaAislada}
	if err := Crear(t.Context(), client, custodia, m, nil); err == nil {
		t.Fatal("se creó el cifrado sin custodia")
	}
	if client.publicadas != 0 || client.ackeadas != 0 || client.status.HasIdentity {
		t.Fatal("se publicó una clave que nadie podrá volver a abrir")
	}
	if d, _ := Diagnosticar(t.Context(), client, custodia); d.Paso != PasoCrear {
		t.Fatalf("tras el fallo el paso es %v; quiero crear otra vez", d.Paso)
	}
	if err := Crear(t.Context(), client, Custodia{}, m, nil); !errors.Is(err, ErrSinCustodia) {
		t.Fatalf("sin servicio: %v", err)
	}
	service.errPut = nil
	if err := Crear(t.Context(), client, custodia, m, nil); err != nil || client.publicadas != 1 {
		t.Fatalf("reintento: %v", err)
	}
}

// Una cuenta con una clave que el servicio no conoce no pide ninguna clave: solo
// verificar el dispositivo. Ya verificado, si este dispositivo tiene la clave (las
// cuentas de almacenamiento la guardan), se envuelve sola con la de desbloqueo.
func TestClaveDesconocidaSoloVerificaYDespuesSeEnvuelve(t *testing.T) {
	m := material(t)
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
	// Sin clave de desbloqueo no se bloquea: está verificado, se envolverá otra vez.
	if got, err := Resolver(t.Context(), client, custodia, d, Material{}, nil); err != nil || got.Paso != PasoListo || len(service.envoltorios) != 0 {
		t.Fatalf("sin clave: %+v, %v", got, err)
	}
	if got, err := Resolver(t.Context(), client, custodia, d, m, nil); err != nil || got.Paso != PasoListo || len(service.envoltorios) != 1 {
		t.Fatalf("Resolver: %+v, %v", got, err)
	}
	if got, _ := recoverywrap.UnwrapSytueID(m.Clave, cuentaDePrueba, service.envoltorios[0]); got != claveDePrueba {
		t.Fatal("se envolvió otra clave")
	}
}

// Producción (Calcus, v3.21.0): una cuenta que se quedó a medias bajo la v3.20 tiene
// una clave por defecto que nada abre (con o sin envoltorio), sin identidad ni copia de
// seguridad. No es «una clave desconocida»: se configura otra vez, sola, con una clave
// nueva envuelta antes de publicarla; sin clave de desbloqueo, a Sytue ID primero.
func TestUnaCuentaAMediasSeConfiguraOtraVezSola(t *testing.T) {
	for _, conEnvoltorio := range []bool{false, true} {
		m := material(t)
		client := &clienteFalso{keyID: "k-vieja", abandonada: true}
		service := &servicioFalso{}
		if conEnvoltorio {
			service.envoltorios = []recoverywrap.Wrapper{envoltorio(t, m, "k-vieja", claveDePrueba)}
		}
		custodia := Custodia{Servicio: service, Cuenta: cuentaAislada}
		d, err := Diagnosticar(t.Context(), client, custodia)
		if err != nil || d.Paso != PasoCrear || !d.rehacer {
			t.Fatalf("Diagnosticar = %+v, %v; quiero configurarla otra vez", d, err)
		}
		if got, err := Resolver(t.Context(), client, custodia, d, Material{}, nil); err != nil || got.Paso != PasoIniciarSesion || client.publicadas != 0 {
			t.Fatalf("sin clave de desbloqueo no se publica nada: %+v, %v", got, err)
		}
		got, err := Resolver(t.Context(), client, custodia, d, m, nil)
		if err != nil || got.Paso != PasoListo || client.publicadas != 1 || client.ackeadas != 1 || len(client.recuperado) != 0 {
			t.Fatalf("Resolver = %+v, %v; publicadas=%d", got, err, client.publicadas)
		}
		if len(service.envoltorios) != 1 || service.envoltorios[0].KeyID != "k-rehecha" {
			t.Fatalf("envoltorios: %+v", service.envoltorios)
		}
		if key, err := recoverywrap.UnwrapSytueID(m.Clave, cuentaDePrueba, service.envoltorios[0]); err != nil || key != claveNueva {
			t.Fatalf("lo custodiado no abre la clave nueva: %v", err)
		}
	}
	// Una cuenta que no está abandonada nunca se sustituye: sigue siendo desconocida.
	client := &clienteFalso{status: mx.VerificationStatus{HasIdentity: true}, keyID: "k1"}
	d, err := Diagnosticar(t.Context(), client, Custodia{Servicio: &servicioFalso{}, Cuenta: cuentaAislada})
	if err != nil || d.Paso != PasoVerificar || d.rehacer {
		t.Fatalf("Diagnosticar = %+v, %v; quiero verificar", d, err)
	}
}

// Producción (Mail, v3.21.0): un dispositivo sin verificar de una cuenta cuya clave no
// tiene envoltorio, pero que la guarda él mismo (la clave de propietario del llavero),
// se abre con ella y la envuelve, sin pedir nada ni pasar por «verificar».
func TestLaClaveQueGuardaElDispositivoAbreSinEnvoltorioYSeEnvuelve(t *testing.T) {
	m := material(t)
	client := &clienteFalso{status: mx.VerificationStatus{HasIdentity: true}, keyID: "k1", valida: claveDePrueba, local: claveDePrueba}
	service := &servicioFalso{}
	custodia := Custodia{Servicio: service, Cuenta: cuentaAislada}
	d, err := Diagnosticar(t.Context(), client, custodia)
	if err != nil || d.Paso != PasoProteger {
		t.Fatalf("Diagnosticar = %+v, %v; quiero abrir con la clave de aquí", d, err)
	}
	got, err := Resolver(t.Context(), client, custodia, d, m, nil)
	if err != nil || got.Paso != PasoListo || !client.status.Verified || len(client.recuperado) != 1 {
		t.Fatalf("Resolver = %+v, %v", got, err)
	}
	if len(service.envoltorios) != 1 || service.envoltorios[0].KeyID != "k1" || client.publicadas != 0 {
		t.Fatalf("envoltorios=%+v publicadas=%d", service.envoltorios, client.publicadas)
	}
	if key, _ := recoverywrap.UnwrapSytueID(m.Clave, cuentaDePrueba, service.envoltorios[0]); key != claveDePrueba {
		t.Fatal("se envolvió otra clave")
	}
	// Sin clave de desbloqueo el dispositivo funciona igual; se envolverá otra vez.
	client = &clienteFalso{status: mx.VerificationStatus{HasIdentity: true}, keyID: "k1", valida: claveDePrueba, local: claveDePrueba}
	service = &servicioFalso{}
	custodia = Custodia{Servicio: service, Cuenta: cuentaAislada}
	d, _ = Diagnosticar(t.Context(), client, custodia)
	if got, err := Resolver(t.Context(), client, custodia, d, Material{}, nil); err != nil || got.Paso != PasoListo || !got.Status.Verified || len(service.envoltorios) != 0 {
		t.Fatalf("sin clave: %+v, %v", got, err)
	}
}

// Transición desde ADR 0081: con la copia con KEK el dispositivo se recupera solo y
// la clave se envuelve con la de desbloqueo (el servicio borra la copia con KEK).
func TestLaCopiaConKEKRecuperaSolaYPasaASerUnEnvoltorio(t *testing.T) {
	m := material(t)
	client := &clienteFalso{status: mx.VerificationStatus{HasIdentity: true}, keyID: "k1", valida: claveDePrueba}
	service := &servicioFalso{legacy: claveDePrueba}
	custodia := Custodia{Servicio: service}
	d, err := Diagnosticar(t.Context(), client, custodia)
	if err != nil || d.Paso != PasoProteger || !d.Status.Verified || len(client.recuperado) != 1 {
		t.Fatalf("Diagnosticar = %+v, %v", d, err)
	}
	if got, err := Resolver(t.Context(), client, custodia, d, m, nil); err != nil || service.legacy != "" || got.Paso != PasoListo {
		t.Fatalf("la copia con KEK sigue: %+v, %v", got, err)
	}
	// Una copia que no abre nada no se envuelve: se queda en verificar.
	client = &clienteFalso{status: mx.VerificationStatus{HasIdentity: true}, keyID: "k1", valida: "otra"}
	if d, _ := Diagnosticar(t.Context(), client, Custodia{Servicio: &servicioFalso{legacy: claveDePrueba}}); d.Paso != PasoVerificar {
		t.Fatalf("copia que no abre: %v", d.Paso)
	}
}

// Transición desde ADR 0082: un dispositivo nuevo con solo el envoltorio de
// contraseña pide volver a Sytue ID con sus parámetros públicos; con lo que calculó
// la página de inicio de sesión lo abre, lo envuelve con la clave de desbloqueo y
// avisa de que lo viejo sobra. Nunca se teclea nada en la app.
func TestElEnvoltorioDeLa0082SeAbreUnaVezYPasaAlTipoNuevo(t *testing.T) {
	m := material(t)
	viejo := envoltorioViejo(t, "secreta", "k1", claveDePrueba)
	client := &clienteFalso{status: mx.VerificationStatus{HasIdentity: true}, keyID: "k1", valida: claveDePrueba}
	service := &servicioFalso{envoltorios: []recoverywrap.Wrapper{viejo}}
	custodia := Custodia{Servicio: service, Cuenta: cuentaAislada}
	d, err := Diagnosticar(t.Context(), client, custodia)
	if err != nil || d.Paso != PasoDesbloquear || d.Legado != recoverywrap.LegacyRequest([]recoverywrap.Wrapper{viejo}) || d.Legado == "" {
		t.Fatalf("Diagnosticar = %+v, %v", d, err)
	}
	if got, err := Resolver(t.Context(), client, custodia, d, m, nil); err != nil || got.Paso != PasoIniciarSesion || got.Legado != d.Legado {
		t.Fatalf("sin lo que abre el viejo: %+v, %v", got, err)
	}
	ikm, err := recoverywrap.PasswordIKM("secreta", viejo.KDF)
	if err != nil {
		t.Fatal(err)
	}
	m.Legado = []recoverywrap.LegacyIKM{{Kind: recoverywrap.KindPassword, Salt: viejo.KDF.Salt, IKM: ikm}}
	got, err := Resolver(t.Context(), client, custodia, d, m, nil)
	if err != nil || got.Paso != PasoListo || !got.migrado || !client.status.Verified {
		t.Fatalf("Resolver: %+v, %v", got, err)
	}
	if !conSytueID(service.envoltorios) {
		t.Fatal("no se envolvió con la clave de desbloqueo")
	}
}

// Restablecer (Rooms): la clave nueva va envuelta antes de publicarse; sin
// custodia o sin clave de desbloqueo no se restablece nada.
func TestRestablecerEnvuelveAntesDePublicar(t *testing.T) {
	m := material(t)
	client := &clienteFalso{status: mx.VerificationStatus{HasIdentity: true}, keyID: "k1"}
	failing := &servicioFalso{errPut: errors.New("503")}
	if err := Restablecer(t.Context(), client, Custodia{Servicio: failing}, m, nil); err == nil || client.publicadas != 0 {
		t.Fatal("se restableció sin custodia")
	}
	service := &servicioFalso{envoltorios: []recoverywrap.Wrapper{envoltorio(t, m, "k1", claveDePrueba)}}
	if err := Restablecer(t.Context(), client, Custodia{Servicio: service}, Material{}, nil); !errors.Is(err, ErrSinDesbloqueo) || client.publicadas != 0 {
		t.Fatalf("sin clave de desbloqueo: %v", err)
	}
	if err := Restablecer(t.Context(), client, Custodia{Servicio: service}, m, nil); err != nil || client.publicadas != 1 {
		t.Fatalf("Restablecer: %v", err)
	}
	if len(service.envoltorios) != 1 || service.envoltorios[0].KeyID != "k-reset" {
		t.Fatalf("quedan envoltorios de la clave vieja: %+v", service.envoltorios)
	}
}

// servicioQueRetira es Rooms con el guardado que retira los de la 0082 (api.Rooms).
type servicioQueRetira struct {
	servicioFalso
	retiradas int
}

func (s *servicioQueRetira) PutMatrixRecoveryWrapperRetiringOlder(ctx context.Context, account api.MatrixRecoveryAccount, w recoverywrap.Wrapper) (string, error) {
	if w.Kind != recoverywrap.KindSytueID {
		return "", recoverywrap.ErrInvalid
	}
	if _, err := s.PutMatrixRecoveryWrapper(ctx, account, w); err != nil {
		return "", err
	}
	kept := s.envoltorios[:0]
	for _, old := range s.envoltorios {
		if old.KeyID != w.KeyID || old.Kind == recoverywrap.KindSytueID {
			kept = append(kept, old)
		}
	}
	s.envoltorios = kept
	s.retiradas++
	return "w", nil
}

// Con el `sytue-id` comprobado AQUÍ, los envoltorios de la ADR 0082 de esa clave se
// retiran (ADR 0085); sin él, o si no abre con la clave de este dispositivo, no se
// toca nada: la cuenta nunca se queda sin uno que abra.
func TestElSytueIDComprobadoRetiraLosEnvoltoriosDeLa0082(t *testing.T) {
	m := material(t)
	client := &clienteFalso{keyID: "k1", valida: claveDePrueba}
	viejo := envoltorioViejo(t, "una contraseña", "k1", claveDePrueba)
	servicio := &servicioQueRetira{servicioFalso: servicioFalso{envoltorios: []recoverywrap.Wrapper{viejo}}}
	custodia := Custodia{Servicio: servicio, Cuenta: cuentaAislada}

	// Solo el viejo: nada que retirar.
	if hecho, err := RetirarViejos(t.Context(), client, custodia, m); err != nil || hecho {
		t.Fatalf("sin sytue-id: %v %v", hecho, err)
	}
	// Un sytue-id de OTRA clave de desbloqueo: no se comprueba aquí, no se retira.
	otra := material(t)
	servicio.envoltorios = append(servicio.envoltorios, envoltorio(t, otra, "k1", claveDePrueba))
	if hecho, _ := RetirarViejos(t.Context(), client, custodia, m); hecho || servicio.retiradas != 0 {
		t.Fatal("se retiró con un sytue-id que no abre aquí")
	}
	// El de esta clave: abre, se guarda otra vez y se van los viejos.
	servicio.envoltorios = []recoverywrap.Wrapper{viejo, envoltorio(t, m, "k1", claveDePrueba)}
	if hecho, err := RetirarViejos(t.Context(), client, custodia, m); err != nil || !hecho {
		t.Fatalf("con el sytue-id de esta clave: %v %v", hecho, err)
	}
	if len(servicio.envoltorios) != 1 || servicio.envoltorios[0].Kind != recoverywrap.KindSytueID {
		t.Fatalf("quedan %d envoltorios; quiero solo el sytue-id", len(servicio.envoltorios))
	}
	// Un servicio que no sabe retirar (otra implementación) no rompe nada.
	if hecho, err := RetirarViejos(t.Context(), client, Custodia{Servicio: &servicioFalso{envoltorios: []recoverywrap.Wrapper{viejo, envoltorio(t, m, "k1", claveDePrueba)}}}, m); err != nil || hecho {
		t.Fatalf("servicio sin retiro: %v %v", hecho, err)
	}
}

// Cada paso recibe su copia de la clave de desbloqueo y la pone a cero al acabar
// (ADR 0085): ni la clave ni lo que abría los envoltorios de la 0082 se quedan en
// memoria más de lo que dura el paso.
func TestLaCopiaDeLaClaveSeBorraAlAcabarElPaso(t *testing.T) {
	m := Material{
		Clave:  bytes.Repeat([]byte{7}, 32),
		Legado: []recoverywrap.LegacyIKM{{Kind: recoverywrap.KindPassword, IKM: bytes.Repeat([]byte{8}, 32)}},
	}
	m.Borrar()
	if !bytes.Equal(m.Clave, make([]byte, 32)) || !bytes.Equal(m.Legado[0].IKM, make([]byte, 32)) {
		t.Fatal("la copia de la clave sigue en memoria")
	}
}
