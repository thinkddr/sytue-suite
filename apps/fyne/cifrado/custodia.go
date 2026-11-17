// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

// La clave de recuperación de cada cuenta de Matrix la guarda el servicio Rooms
// ENVUELTA con la clave de desbloqueo de la persona (ADR 0084): la que trae su inicio
// de sesión en Sytue ID, que es lo único que la persona teclea. Nadie enseña, pide ni
// guarda la clave de recuperación, y este panel no pide ninguna contraseña: sin clave
// de desbloqueo en el dispositivo, ofrece volver a entrar en Sytue ID. Ni el operador
// ni Sytue ID pueden abrir los envoltorios.

import (
	"context"
	"errors"

	"maunium.net/go/mautrix"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
)

// CustodioClave es el servicio Rooms (api.Rooms): los envoltorios de cada cuenta
// y, solo para la transición desde ADR 0081, la copia con KEK de la de Rooms.
type CustodioClave interface {
	MatrixRecoveryWrappers(context.Context, api.MatrixRecoveryAccount) (api.MatrixRecoveryWrappers, error)
	PutMatrixRecoveryWrapper(context.Context, api.MatrixRecoveryAccount, recoverywrap.Wrapper) (string, error)
	MatrixRecoveryKey(context.Context) (string, error)
}

// Custodia es la cuenta de Matrix de un panel y dónde se guardan sus envoltorios.
type Custodia struct {
	Servicio CustodioClave
	Cuenta   api.MatrixRecoveryAccount
}

// Recuperable es lo que la custodia necesita de *mx.Client.
type Recuperable interface {
	VerificationStatus(context.Context) (mx.VerificationStatus, error)
	Recover(context.Context, string) error
	BootstrapRecoveryWrapped(context.Context, mautrix.UIACallback, mx.RecoveryEscrow) error
	AcknowledgeRecoverySetup(context.Context) error
	DefaultRecoveryKeyID(context.Context) (string, error)
	LocalRecoveryKey(context.Context) (string, string, error)
	AccountUserID() string
}

// Restablecible es lo que «No tengo otro dispositivo» necesita de *mx.Client.
type Restablecible interface {
	ResetRecoveryWrapped(context.Context, mautrix.UIACallback, mx.RecoveryEscrow) error
	AccountUserID() string
}

// Paso es lo que le falta a este dispositivo para abrir los datos cifrados.
type Paso int

const (
	// PasoListo: verificado y la clave ya está envuelta (o no hay nada que hacer).
	PasoListo Paso = iota
	// PasoDesbloquear: hay envoltorios de la clave de la cuenta; falta la contraseña.
	PasoDesbloquear
	// PasoProteger: este dispositivo tiene la clave pero el servicio no la guarda
	// envuelta; falta la contraseña con la que envolverla.
	PasoProteger
	// PasoCrear: la cuenta no tiene cifrado (o quedó a medias); se crea con la
	// contraseña que envolverá su clave.
	PasoCrear
	// PasoVerificar: la cuenta tiene una clave que el servicio no conoce. Solo
	// queda verificar el dispositivo desde otro (o, en Rooms, restablecer).
	PasoVerificar
	// PasoIniciarSesion: a este dispositivo le falta la clave de desbloqueo de Sytue
	// ID (o lo que abre los envoltorios de la ADR 0082): se vuelve a entrar en Sytue
	// ID, que es donde la persona mete su clave.
	PasoIniciarSesion
)

// Material es la clave de desbloqueo de la persona (ADR 0084), la que trajo el inicio
// de sesión en Sytue ID, y lo que abre una vez sus envoltorios de la ADR 0082.
type Material struct {
	Clave  []byte
	Legado []recoverywrap.LegacyIKM
}

func (m Material) valido() bool { return len(m.Clave) == recoverywrap.UnlockKeyLen }

// Borrar pone a cero esta copia de la clave y de lo que abría los envoltorios de la
// ADR 0082 en cuanto el paso que la usaba acaba (ADR 0085): la sesión da una copia por
// paso, y ninguna se queda en memoria más de lo que dura ese paso.
func (m Material) Borrar() {
	clear(m.Clave)
	for _, l := range m.Legado {
		clear(l.IKM)
	}
}

// ErrSinDesbloqueo es crear, abrir o envolver sin la clave de desbloqueo de Sytue ID
// (o sin lo que abre un envoltorio de la ADR 0082): toca volver a entrar en Sytue ID.
var ErrSinDesbloqueo = errors.New("cifrado: falta la clave de desbloqueo de Sytue ID")

// Diagnostico es el resultado de Diagnosticar. La clave de PasoProteger vive solo
// en memoria hasta que se envuelve; nunca se pinta.
type Diagnostico struct {
	Paso        Paso
	Status      mx.VerificationStatus
	Envoltorios []recoverywrap.Wrapper
	// Legado es la petición `sytue_legacy` de los envoltorios de la ADR 0082 de la clave
	// actual: lo que la página de inicio de sesión de Sytue ID tiene que abrir también.
	Legado  string
	clave   string
	claveID string
	// migrado: se abrió un envoltorio de la ADR 0082 y ya está envuelta con la clave
	// de desbloqueo; lo que abría el viejo sobra.
	migrado bool
}

// ErrSinCustodia es crear o restablecer el cifrado sin dónde guardar la clave.
var ErrSinCustodia = mx.ErrNoRecoveryEscrow

// Diagnosticar decide el paso sin pedir nada a la persona: con la copia con KEK
// de ADR 0081 recupera el dispositivo solo (y pide envolverla); lo demás lo dice
// el estado de la cuenta y los envoltorios de su clave actual.
func Diagnosticar(ctx context.Context, client Recuperable, custodia Custodia) (Diagnostico, error) {
	status, err := client.VerificationStatus(ctx)
	if err != nil {
		return Diagnostico{Status: status}, err
	}
	d := Diagnostico{Status: status}
	if custodia.Servicio == nil {
		d.Paso = pasoSinCustodia(status)
		return d, nil
	}
	if status.RecoverySetupPending {
		// Una configuración a medias se reanuda con su misma clave (y se custodia).
		d.Paso = PasoCrear
		return d, nil
	}
	stored, err := custodia.Servicio.MatrixRecoveryWrappers(ctx, custodia.Cuenta)
	if err != nil {
		return d, err
	}
	keyID, err := client.DefaultRecoveryKeyID(ctx)
	if err != nil {
		return d, err
	}
	d.Envoltorios = deLaClave(stored.Wrappers, keyID)
	if !conSytueID(d.Envoltorios) {
		d.Legado = recoverywrap.LegacyRequest(d.Envoltorios)
	}
	if status.Verified {
		d.Paso = PasoListo
		// Un dispositivo verificado con la clave a mano la envuelve con la clave de
		// desbloqueo si aún no lo está (cuentas de la ADR 0082 o de antes).
		if !conSytueID(d.Envoltorios) && keyID != "" {
			if localID, key, err := client.LocalRecoveryKey(ctx); err == nil && localID == keyID {
				d.Paso, d.clave, d.claveID = PasoProteger, key, keyID
			}
		}
		return d, nil
	}
	if len(d.Envoltorios) > 0 {
		d.Paso = PasoDesbloquear
		return d, nil
	}
	if stored.LegacyEscrow && keyID != "" {
		if key, err := custodia.Servicio.MatrixRecoveryKey(ctx); err == nil && key != "" && client.Recover(ctx, key) == nil {
			d.Paso, d.clave, d.claveID = PasoProteger, key, keyID
			d.Status, err = client.VerificationStatus(ctx)
			return d, err
		}
	}
	if !status.HasIdentity {
		d.Paso = PasoCrear
		return d, nil
	}
	d.Paso = PasoVerificar
	return d, nil
}

// pasoSinCustodia: sin servicio no se puede crear (no hay dónde guardar la clave)
// ni desbloquear; Crear lo dirá con ErrSinCustodia.
func pasoSinCustodia(status mx.VerificationStatus) Paso {
	switch {
	case status.Verified && !status.RecoverySetupPending:
		return PasoListo
	case status.HasIdentity:
		return PasoVerificar
	default:
		return PasoCrear
	}
}

// deLaClave son los envoltorios de la clave actual de la cuenta: los de otra clave ya
// no abren nada.
func deLaClave(wrappers []recoverywrap.Wrapper, keyID string) []recoverywrap.Wrapper {
	var out []recoverywrap.Wrapper
	for _, w := range wrappers {
		if keyID != "" && w.KeyID == keyID {
			out = append(out, w)
		}
	}
	return out
}

func conSytueID(wrappers []recoverywrap.Wrapper) bool {
	for _, w := range wrappers {
		if w.Kind == recoverywrap.KindSytueID {
			return true
		}
	}
	return false
}

// envolver es la custodia que BootstrapRecoveryWrapped y ResetRecoveryWrapped
// llaman ANTES de publicar la clave: el envoltorio `sytue-id` de la clave de
// desbloqueo, que se guarda en Rooms.
func envolver(custodia Custodia, account string, m Material) mx.RecoveryEscrow {
	return func(ctx context.Context, keyID, recoveryKey string) error {
		if !m.valido() {
			return ErrSinDesbloqueo
		}
		w, err := recoverywrap.WrapSytueID(m.Clave, account, keyID, recoveryKey)
		if err != nil {
			return err
		}
		_, err = custodia.Servicio.PutMatrixRecoveryWrapper(ctx, custodia.Cuenta, w)
		return err
	}
}

// Desbloquear abre la clave con la clave de desbloqueo (o, una vez, con lo que la
// página de inicio de sesión calculó para un envoltorio de la ADR 0082) y recupera
// el dispositivo, que queda verificado por las firmas cruzadas. Sin nada que la abra
// es ErrSinDesbloqueo; un envoltorio `sytue-id` de otra clave de desbloqueo (la
// persona empezó de cero) es recoverywrap.ErrWrongSecret. `legado` dice que se abrió
// uno de la 0082.
func Desbloquear(ctx context.Context, client Recuperable, envoltorios []recoverywrap.Wrapper, m Material) (key string, legado bool, err error) {
	if !m.valido() {
		return "", false, ErrSinDesbloqueo
	}
	account := client.AccountUserID()
	viejo := false
	for _, w := range envoltorios {
		if w.Kind != recoverywrap.KindSytueID {
			continue
		}
		key, err := recoverywrap.UnwrapSytueID(m.Clave, account, w)
		if err != nil {
			viejo = true
			continue
		}
		return key, false, client.Recover(ctx, key)
	}
	for _, l := range m.Legado {
		if key, err := recoverywrap.OpenLegacy(l, account, envoltorios); err == nil {
			return key, true, client.Recover(ctx, key)
		}
	}
	if viejo {
		return "", false, recoverywrap.ErrWrongSecret
	}
	return "", false, ErrSinDesbloqueo
}

// retiradorDeViejos es el servicio que sabe retirar los envoltorios de la ADR 0082
// con el `sytue-id` comprobado (api.Rooms).
type retiradorDeViejos interface {
	PutMatrixRecoveryWrapperRetiringOlder(context.Context, api.MatrixRecoveryAccount, recoverywrap.Wrapper) (string, error)
}

// RetirarViejos retira los envoltorios de la ADR 0082 de la clave actual una vez que
// ESTE dispositivo ha comprobado que su envoltorio `sytue-id` abre con la clave de
// desbloqueo (ADR 0085): el servicio lo guarda otra vez y borra los viejos en la misma
// transacción, así que la cuenta nunca se queda sin uno que abra. Dice si lo hizo.
func RetirarViejos(ctx context.Context, client Recuperable, custodia Custodia, m Material) (bool, error) {
	retirador, ok := custodia.Servicio.(retiradorDeViejos)
	if !ok || !m.valido() {
		return false, nil
	}
	keyID, err := client.DefaultRecoveryKeyID(ctx)
	if err != nil || keyID == "" {
		return false, err
	}
	stored, err := custodia.Servicio.MatrixRecoveryWrappers(ctx, custodia.Cuenta)
	if err != nil {
		return false, err
	}
	envoltorios := deLaClave(stored.Wrappers, keyID)
	var actual *recoverywrap.Wrapper
	viejos := false
	for i, w := range envoltorios {
		if w.Kind == recoverywrap.KindSytueID {
			actual = &envoltorios[i]
		} else {
			viejos = true
		}
	}
	if actual == nil || !viejos {
		return false, nil
	}
	// Que no abra aquí no es un fallo: es que no toca retirar nada (otra clave de
	// desbloqueo, la persona empezó de cero en otro sitio).
	if _, err := recoverywrap.UnwrapSytueID(m.Clave, client.AccountUserID(), *actual); err != nil {
		return false, nil //nolint:nilerr // no abrir aquí es la respuesta «no se retira», no un error.
	}
	_, err = retirador.PutMatrixRecoveryWrapperRetiringOlder(ctx, custodia.Cuenta, *actual)
	return err == nil, err
}

// Crear configura el cifrado de la cuenta: la clave nueva se envuelve con la clave
// de desbloqueo y se guarda antes de publicarla, y no se enseña. Si la custodia
// falla, no se publica nada y el reintento usa la misma clave.
func Crear(ctx context.Context, client Recuperable, custodia Custodia, m Material, uia mautrix.UIACallback) error {
	if custodia.Servicio == nil {
		return ErrSinCustodia
	}
	if !m.valido() {
		return ErrSinDesbloqueo
	}
	if err := client.BootstrapRecoveryWrapped(ctx, uia, envolver(custodia, client.AccountUserID(), m)); err != nil {
		return err
	}
	return client.AcknowledgeRecoverySetup(ctx)
}

// Proteger envuelve con la clave de desbloqueo una clave que este dispositivo ya tiene.
func Proteger(ctx context.Context, client Recuperable, custodia Custodia, d Diagnostico, m Material) error {
	if custodia.Servicio == nil {
		return ErrSinCustodia
	}
	if d.clave == "" || d.claveID == "" {
		return recoverywrap.ErrInvalid
	}
	return envolver(custodia, client.AccountUserID(), m)(ctx, d.claveID, d.clave)
}

// Restablecer crea identidad y clave nuevas con la clave envuelta antes de
// publicarlas. La clave nueva no sale de aquí.
func Restablecer(ctx context.Context, client Restablecible, custodia Custodia, m Material, uia mautrix.UIACallback) error {
	if custodia.Servicio == nil {
		return ErrSinCustodia
	}
	if !m.valido() {
		return ErrSinDesbloqueo
	}
	return client.ResetRecoveryWrapped(ctx, uia, envolver(custodia, client.AccountUserID(), m))
}

// Resolver hace sin preguntar nada lo que pide el diagnóstico (ADR 0084): crear el
// cifrado, abrirlo o envolver la clave con la clave de desbloqueo. Sin ella el paso
// queda en PasoIniciarSesion; con un envoltorio de otra clave de desbloqueo, en
// PasoVerificar. Un dispositivo ya verificado sin clave de desbloqueo no se bloquea:
// la envolverá la próxima vez que la tenga.
func Resolver(ctx context.Context, client Recuperable, custodia Custodia, d Diagnostico, m Material, uia mautrix.UIACallback) (Diagnostico, error) {
	switch d.Paso {
	case PasoCrear:
		if custodia.Servicio != nil && !m.valido() {
			d.Paso = PasoIniciarSesion
			return d, nil
		}
		if err := Crear(ctx, client, custodia, m, uia); err != nil {
			return d, err
		}
	case PasoDesbloquear:
		key, legado, err := Desbloquear(ctx, client, d.Envoltorios, m)
		switch {
		case errors.Is(err, ErrSinDesbloqueo):
			d.Paso = PasoIniciarSesion
			return d, nil
		case errors.Is(err, recoverywrap.ErrWrongSecret):
			d.Paso = PasoVerificar
			return d, nil
		case err != nil:
			return d, err
		}
		if legado {
			keyID, err := client.DefaultRecoveryKeyID(ctx)
			if err != nil {
				return d, err
			}
			if err := envolver(custodia, client.AccountUserID(), m)(ctx, keyID, key); err != nil {
				return d, err
			}
		}
		next, err := Diagnosticar(ctx, client, custodia)
		next.migrado = legado
		return next, err
	case PasoProteger:
		if !m.valido() {
			d.olvidar()
			d.Paso = PasoListo
			return d, nil
		}
		if err := Proteger(ctx, client, custodia, d, m); err != nil {
			return d, err
		}
		d.olvidar()
	default:
		return d, nil
	}
	return Diagnosticar(ctx, client, custodia)
}

// olvidar borra de memoria la clave de un diagnóstico.
func (d *Diagnostico) olvidar() { d.clave, d.claveID = "", "" }

// custodiaDeSesion es el servicio Rooms de los productos de almacenamiento: cada
// llamada usa la sesión HTTP vinculada a la identidad de la aplicación que lleva
// el contexto (SessionHTTP), como el bootstrap de su cuenta de Matrix. Fuera de
// esa sesión no se pide nada.
type custodiaDeSesion struct{ roomsURL func(string) string }

func (s custodiaDeSesion) rooms(ctx context.Context) (*api.Rooms, error) {
	bound, err := SessionHTTP(ctx)
	if err != nil {
		return nil, err
	}
	return api.NewRooms(s.roomsURL("rooms"), bound, nil), nil
}

func (s custodiaDeSesion) MatrixRecoveryWrappers(ctx context.Context, account api.MatrixRecoveryAccount) (api.MatrixRecoveryWrappers, error) {
	rooms, err := s.rooms(ctx)
	if err != nil {
		return api.MatrixRecoveryWrappers{}, err
	}
	return rooms.MatrixRecoveryWrappers(ctx, account)
}

func (s custodiaDeSesion) PutMatrixRecoveryWrapper(ctx context.Context, account api.MatrixRecoveryAccount, w recoverywrap.Wrapper) (string, error) {
	rooms, err := s.rooms(ctx)
	if err != nil {
		return "", err
	}
	return rooms.PutMatrixRecoveryWrapper(ctx, account, w)
}

func (s custodiaDeSesion) MatrixRecoveryKey(ctx context.Context) (string, error) {
	rooms, err := s.rooms(ctx)
	if err != nil {
		return "", err
	}
	return rooms.MatrixRecoveryKey(ctx)
}
