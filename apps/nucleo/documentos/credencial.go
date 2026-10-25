// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package documentos

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"

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
	ID         string      `json:"id"`
	URL        string      `json:"url,omitempty"`
	Usuario    string      `json:"usuario,omitempty"`
	Clave      string      `json:"clave,omitempty"`
	Dueno      Identidad   `json:"owner,omitempty"`
	Pendientes []Pendiente `json:"pending,omitempty"`
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
	return asegurar(ctx, c, url, etiqueta, Identidad{})
}

func asegurar(ctx context.Context, c Cuenta, url, etiqueta string, identidad Identidad) error {
	if !hayProveedor {
		return ErrNoDisponible
	}
	soltar, err := excluir(ctx)
	if err != nil {
		return err
	}
	defer soltar()
	vieja, err := cargarCredencial()
	if err != nil {
		return err
	}
	if vieja.Clave != "" && vieja.URL == url && vieja.Dueno == identidad {
		return conectar(ctx, vieja)
	}
	// Retirar la contraseña ANTES de conectar otra cuenta, incluso si revocar
	// falla. Conservar sólo los identificadores permite recuperarlo sin exponerla.
	pendientes := vieja.Pendientes
	if vieja.ID != "" {
		pendientes = append(pendientes, Pendiente{ID: vieja.ID, Dueno: vieja.Dueno})
	}
	base := Credencial{Pendientes: pendientes}
	if err := guardarCredencial(base); err != nil {
		return err
	}
	if vieja.Clave != "" {
		desconectar(vieja)
	}
	base, err = revocarPendientes(ctx, c, base, identidad)
	if err != nil {
		return err
	}
	yo, err := c.Me(ctx)
	if err != nil {
		return err
	}
	if identidad.Sujeto != "" && yo.ID != identidad.Sujeto {
		return errors.New("documentos: la cuenta ha cambiado durante la conexión")
	}
	nueva, err := c.CreateAppPassword(ctx, etiqueta)
	if err != nil {
		return err
	}
	if identidad.completa() && (nueva.OrgID == "" || nueva.Tenant != identidad.Organizacion) {
		base.Pendientes = append(base.Pendientes, Pendiente{ID: nueva.ID, Dueno: identidad})
		return errors.Join(errors.New("documentos: el servidor no ha fijado la organización de la credencial"), guardarCredencial(base))
	}
	cred := Credencial{ID: nueva.ID, URL: url, Usuario: yo.Email, Clave: nueva.Password, Dueno: identidad, Pendientes: base.Pendientes}
	if err := ctx.Err(); err != nil {
		base.Pendientes = append(base.Pendientes, Pendiente{ID: nueva.ID, Dueno: identidad})
		return errors.Join(err, guardarCredencial(base))
	}
	if err := guardarCredencial(cred); err != nil {
		// Una contraseña sin almacén se revoca; los identificadores anteriores
		// siguen guardados y no se borran al fallar esta nueva escritura.
		return errors.Join(err, revocar(ctx, c, nueva.ID))
	}
	return conectar(ctx, cred)
}

func revocarPendientes(ctx context.Context, c Cuenta, cred Credencial, identidad Identidad) (Credencial, error) {
	for i := 0; i < len(cred.Pendientes); {
		p := cred.Pendientes[i]
		if p.Dueno != identidad {
			i++
			continue
		}
		if err := revocar(ctx, c, p.ID); err != nil {
			return cred, err
		}
		cred.Pendientes = slices.Delete(cred.Pendientes, i, i+1)
		if err := guardarCredencial(cred); err != nil {
			return cred, err
		}
	}
	return cred, nil
}

// Guardada dice si este equipo tiene credencial de Drive en el sistema, viva o por
// revocar. Solo lee el almacén.
func Guardada() bool {
	c, err := cargarCredencial()
	return err == nil && (c.ID != "" || len(c.Pendientes) != 0)
}

// Retirar quita Drive del sistema y revoca la contraseña. Va ANTES de cerrar la
// sesión, con el token vivo. Si la revocación falla, deja el identificador para el
// siguiente [Asegurar].
func Retirar(ctx context.Context, c Cuenta) error { return retirar(ctx, c, nil) }

func retirar(ctx context.Context, c Cuenta, identidad *Identidad) error {
	soltar, err := excluir(ctx)
	if err != nil {
		return err
	}
	defer soltar()
	cred, err := cargarCredencial()
	if err != nil {
		return err
	}
	// La API antigua retira al dueño de la credencial activa, nunca las de
	// terceros que estén pendientes. La suite pasa su identidad explícita.
	dueno := cred.Dueno
	if identidad != nil {
		dueno = *identidad
		if cred.Dueno != dueno {
			// Una retirada que esperaba el cerrojo puede llegar después del alta
			// de otra cuenta. Revoca sus pendientes sin tocar la conexión nueva.
			cred, err = revocarPendientes(ctx, c, cred, dueno)
			if err == nil && cred.ID == "" && cred.Clave == "" && len(cred.Pendientes) == 0 {
				return borrar(CuentaSecreto)
			}
			return err
		}
	}
	if cred.ID != "" {
		cred.Pendientes = append(cred.Pendientes, Pendiente{ID: cred.ID, Dueno: cred.Dueno})
	}
	vieja := cred
	cred = Credencial{Pendientes: cred.Pendientes}
	if err := guardarCredencial(cred); err != nil {
		return err
	}
	desconectar(vieja)
	cred, err = revocarPendientes(ctx, c, cred, dueno)
	if err != nil {
		return err
	}
	if len(cred.Pendientes) == 0 {
		return borrar(CuentaSecreto)
	}
	return nil
}

// Olvidar quita Drive del sistema cuando la sesión ya no está y no hay token con el que
// revocar: borra la contraseña y deja el identificador.
func Olvidar() error { return OlvidarCon(context.Background()) }

// OlvidarCon retira la credencial atendiendo al plazo del ciclo de sesión.
func OlvidarCon(ctx context.Context) error {
	soltar, err := excluir(ctx)
	if err != nil {
		return err
	}
	defer soltar()
	cred, err := cargarCredencial()
	if err != nil || cred.Clave == "" {
		return err
	}
	vieja := cred
	cred.URL, cred.Usuario, cred.Clave = "", "", ""
	if err := guardarCredencial(cred); err != nil {
		return err
	}
	desconectar(vieja)
	return nil
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
