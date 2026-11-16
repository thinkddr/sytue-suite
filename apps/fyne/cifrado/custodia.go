// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

// La clave de recuperación de cada cuenta de Matrix la guarda el servicio Rooms
// ENVUELTA con la contraseña de la persona, en este dispositivo (ADR 0082). Nadie
// enseña, pide ni guarda la clave: la persona solo teclea su contraseña de Sytue,
// una vez por cuenta y dispositivo. Ni el operador ni Sytue ID pueden abrirla.

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
)

// Diagnostico es el resultado de Diagnosticar. La clave de PasoProteger vive solo
// en memoria hasta que se envuelve; nunca se pinta.
type Diagnostico struct {
	Paso        Paso
	Status      mx.VerificationStatus
	Envoltorios []recoverywrap.Wrapper
	clave       string
	claveID     string
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
	if status.Verified {
		d.Paso = PasoListo
		if len(d.Envoltorios) == 0 && keyID != "" {
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

// deLaClave son los envoltorios de contraseña de la clave actual de la cuenta: los
// de otra clave ya no abren nada.
func deLaClave(wrappers []recoverywrap.Wrapper, keyID string) []recoverywrap.Wrapper {
	var out []recoverywrap.Wrapper
	for _, w := range wrappers {
		if keyID != "" && w.KeyID == keyID && w.Kind == recoverywrap.KindPassword {
			out = append(out, w)
		}
	}
	return out
}

// envolver es la custodia que BootstrapRecoveryWrapped y ResetRecoveryWrapped
// llaman ANTES de publicar la clave.
func envolver(custodia Custodia, account, password string) mx.RecoveryEscrow {
	return func(ctx context.Context, keyID, recoveryKey string) error {
		w, err := recoverywrap.WrapPassword(password, account, keyID, recoveryKey)
		if err != nil {
			return err
		}
		_, err = custodia.Servicio.PutMatrixRecoveryWrapper(ctx, custodia.Cuenta, w)
		return err
	}
}

// Desbloquear abre la clave con la contraseña y recupera el dispositivo. Una
// contraseña que no abre ningún envoltorio es recoverywrap.ErrWrongSecret.
func Desbloquear(ctx context.Context, client Recuperable, envoltorios []recoverywrap.Wrapper, password string) (string, error) {
	if password == "" {
		return "", recoverywrap.ErrWrongSecret
	}
	for _, w := range envoltorios {
		key, err := recoverywrap.UnwrapPassword(password, client.AccountUserID(), w)
		if err != nil {
			continue
		}
		return key, client.Recover(ctx, key)
	}
	return "", recoverywrap.ErrWrongSecret
}

// Crear configura el cifrado de la cuenta: la clave nueva se envuelve con la
// contraseña y se guarda antes de publicarla, y no se enseña. Si la custodia
// falla, no se publica nada y el reintento usa la misma clave.
func Crear(ctx context.Context, client Recuperable, custodia Custodia, password string, uia mautrix.UIACallback) error {
	if custodia.Servicio == nil {
		return ErrSinCustodia
	}
	if password == "" {
		return recoverywrap.ErrInvalid
	}
	if err := client.BootstrapRecoveryWrapped(ctx, uia, envolver(custodia, client.AccountUserID(), password)); err != nil {
		return err
	}
	return client.AcknowledgeRecoverySetup(ctx)
}

// Proteger envuelve con la contraseña una clave que este dispositivo ya tiene.
func Proteger(ctx context.Context, client Recuperable, custodia Custodia, d Diagnostico, password string) error {
	if custodia.Servicio == nil {
		return ErrSinCustodia
	}
	if d.clave == "" || d.claveID == "" || password == "" {
		return recoverywrap.ErrInvalid
	}
	return envolver(custodia, client.AccountUserID(), password)(ctx, d.claveID, d.clave)
}

// Restablecer crea identidad y clave nuevas con la clave envuelta antes de
// publicarlas. La clave nueva no sale de aquí.
func Restablecer(ctx context.Context, client Restablecible, custodia Custodia, password string, uia mautrix.UIACallback) error {
	if custodia.Servicio == nil {
		return ErrSinCustodia
	}
	if password == "" {
		return recoverywrap.ErrInvalid
	}
	return client.ResetRecoveryWrapped(ctx, uia, envolver(custodia, client.AccountUserID(), password))
}

// contrasenaErronea distingue «otra vez» de «algo ha fallado».
func contrasenaErronea(err error) bool { return errors.Is(err, recoverywrap.ErrWrongSecret) }

// conClave devuelve un diagnóstico de PasoProteger para una clave recién abierta:
// tras cambiar la contraseña de Sytue se abre con la vieja y se vuelve a envolver.
func conClave(d Diagnostico, keyID, key string) Diagnostico {
	d.Paso, d.clave, d.claveID = PasoProteger, key, keyID
	return d
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
