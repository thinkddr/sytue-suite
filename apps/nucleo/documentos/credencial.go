// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package documentos

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// # La credencial de Drive en el sistema
//
// El proveedor de Android y GVfs no saben de OIDC: entran en el WebDAV de Drive con
// Basic. La contraseña es UNA de aplicación de Sytue ID por equipo, con su etiqueta, que
// se ve y se revoca en «Mi cuenta» como las del correo. Va en el almacén seguro del
// sistema (`auth.GuardarSecreto`: Android Keystore, Secret Service), nunca en claro, y
// sin refrescos: no caduca, se revoca.
//
//   - [Asegurar], con sesión: si no hay credencial la crea, y conecta el sistema.
//   - [Retirar], al cerrar sesión con el token aún vivo: desconecta, borra y revoca.
//   - [Olvidar], al ver que la sesión ya no está: desconecta y borra la contraseña, y deja
//     el identificador para revocarla en el siguiente [Asegurar].

// CuentaSecreto es la cuenta del almacén donde va la credencial. Documentos.java la lee
// de `secreto-drive-sistema.bin` con la misma clave del Keystore que la sesión.
const CuentaSecreto = "drive-sistema"

// Credencial es lo que el sistema necesita para entrar: la URL de WebDAV, el usuario
// (el correo, que Sytue ID contrasta) y la contraseña. ID es el de Sytue ID, para
// revocarla; sin Clave es una que queda por revocar.
type Credencial struct {
	ID      string `json:"id"`
	URL     string `json:"url,omitempty"`
	Usuario string `json:"usuario,omitempty"`
	Clave   string `json:"clave,omitempty"`
}

// Cuenta es lo que hace falta de Sytue ID; la da `*api.Account`, y el guardián, un doble.
type Cuenta interface {
	Me(ctx context.Context) (api.Profile, error)
	CreateAppPassword(ctx context.Context, etiqueta string) (api.AppPasswordNueva, error)
	RevokeAppPassword(ctx context.Context, id string) error
}

var _ Cuenta = (*api.Account)(nil)

// El almacén, en variables para que el guardián ponga dobles. `conectar`,
// `desconectar` y `hayProveedor` (si este sistema tiene dónde enseñar Drive) son de
// cada sistema: documentos_android.go, documentos_linux.go y documentos_otros.go.
var (
	leer    = auth.LeerSecreto
	guardar = auth.GuardarSecreto
	borrar  = auth.BorrarSecreto
)

func cargarCredencial() (Credencial, error) {
	crudo, err := leer(CuentaSecreto)
	if err != nil || len(crudo) == 0 {
		return Credencial{}, err
	}
	var c Credencial
	err = json.Unmarshal(crudo, &c)
	return c, err
}

func guardarCredencial(c Credencial) error {
	crudo, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return guardar(CuentaSecreto, crudo)
}

// Asegurar deja Drive en el sistema con la credencial de este equipo: la que ya hay si
// es de esta `url`, o una nueva con `etiqueta`. Si quedaba una por revocar, la revoca
// antes. Sin proveedor en este sistema, ErrNoDisponible y no crea nada.
func Asegurar(ctx context.Context, c Cuenta, url, etiqueta string) error {
	if !hayProveedor {
		return ErrNoDisponible
	}
	vieja, err := cargarCredencial()
	if err != nil {
		return err
	}
	if vieja.Clave != "" && vieja.URL == url {
		return conectar(vieja)
	}
	if vieja.ID != "" {
		if err := revocar(ctx, c, vieja.ID); err != nil {
			return err // se intenta en el siguiente: no se apilan contraseñas vivas
		}
		desconectar(vieja)
	}
	yo, err := c.Me(ctx)
	if err != nil {
		return err
	}
	nueva, err := c.CreateAppPassword(ctx, etiqueta)
	if err != nil {
		return err
	}
	cred := Credencial{ID: nueva.ID, URL: url, Usuario: yo.Email, Clave: nueva.Password}
	if err := guardarCredencial(cred); err != nil {
		// Sin dónde guardarla (un Linux sin llavero), no se deja viva una que nadie
		// podrá revocar desde aquí.
		return errors.Join(err, revocar(ctx, c, nueva.ID), borrar(CuentaSecreto))
	}
	return conectar(cred)
}

// Guardada dice si este equipo tiene credencial de Drive en el sistema, viva o por
// revocar. Solo lee el almacén.
func Guardada() bool {
	c, err := cargarCredencial()
	return err == nil && c.ID != ""
}

// Retirar quita Drive del sistema y revoca la contraseña. Va ANTES de cerrar la
// sesión, con el token vivo. Si la revocación falla, deja el identificador para el
// siguiente [Asegurar].
func Retirar(ctx context.Context, c Cuenta) error {
	cred, err := cargarCredencial()
	if err != nil || cred.ID == "" {
		return err
	}
	desconectar(cred)
	if err := revocar(ctx, c, cred.ID); err != nil {
		return errors.Join(err, guardarCredencial(Credencial{ID: cred.ID}))
	}
	return borrar(CuentaSecreto)
}

// Olvidar quita Drive del sistema cuando la sesión ya no está y no hay token con el que
// revocar: borra la contraseña y deja el identificador.
func Olvidar() error {
	cred, err := cargarCredencial()
	if err != nil || cred.Clave == "" {
		return err
	}
	desconectar(cred)
	return guardarCredencial(Credencial{ID: cred.ID})
}

// revocar no cuenta como fallo una que ya no existe (revocada en «Mi cuenta»).
func revocar(ctx context.Context, c Cuenta, id string) error {
	if err := c.RevokeAppPassword(ctx, id); err != nil && !api.EsNoEncontrado(err) {
		return err
	}
	return nil
}

// avisar apunta lo que no ha salido al conectar o desconectar: el sistema es de quien
// lo usa y un fallo suyo no para la app.
func avisar(que string, err error) {
	if err != nil {
		slog.Warn("documentos: "+que, "error", err)
	}
}
