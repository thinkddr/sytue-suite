// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

// # Una carpeta por cuenta y organización
//
// Decidido por el dueño (FYS-04): «Sytue Drive – <organización>», atada a la persona y a
// la organización con que se conectó. Al cambiar de organización o de cuenta, o al cerrar
// sesión, la de antes se para y se queda como está, sin borrar ni subir nada; al volver,
// se reanuda la suya. Era UNA carpeta para todo, y al cambiar de organización la misma
// pasaba a ser de la otra: se quitaba lo de una y lo nuevo subía a la otra.
//
// De quién es cada carpeta se apunta en `<config>/sytue/drive-carpetas.json`; solo lo
// escribe el Hub, que es uno por sesión del sistema.

// Cuenta es de quién es una carpeta: la persona (su `sub`) y su organización (`tenant`).
type Cuenta struct{ Emisor, Sub, Org string }

// vinculo es una carpeta apuntada a su cuenta.
type vinculo struct {
	Emisor string `json:"issuer,omitempty"`
	Sub    string `json:"sub"`
	Org    string `json:"org"`
	Ruta   string `json:"ruta"`
}

// carpetaVieja es la carpeta de antes de atarlas, sin cuenta: `%USERPROFILE%\Sytue Drive`.
const carpetaVieja = "Sytue Drive"

var errSinCuenta = errors.New("nube: sin sesión no hay carpeta de Drive")

// Carpeta es la de `c`, ya apuntada: la que tenía o, la primera vez,
// `%USERPROFILE%\Sytue Drive – <organización>`, con « (2)» si ese nombre ya lo tiene otra
// cosa. Una cuenta personal es «Personal».
//
// La carpeta de antes, `Sytue Drive` a secas, pasa a ser de la primera cuenta que la
// pida, con su nombre y con todo lo que tiene dentro. Renombrarla pide darla de baja en
// Windows y volverla a registrar, y lo que eso hace con los marcadores no se ha podido ver
// en un Windows: mejor un nombre de antes que un fichero que suba vacío.
func Carpeta(c Cuenta) (string, error) {
	ctx, parar := context.WithTimeout(context.Background(), time.Minute)
	defer parar()
	return CarpetaCon(ctx, c)
}

// CarpetaCon serializa el vínculo entre procesos, también cuando se abre desde
// una app distinta de la que ya está atendiendo CFAPI.
func CarpetaCon(ctx context.Context, c Cuenta) (string, error) {
	fichero, err := ficheroVinculos()
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(fichero+".cerrojo", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return "", err
	}
	soltar, err := ajustes.CerrojoFichero(ctx, f)
	if err != nil {
		return "", err
	}
	defer soltar()
	return carpeta(c, registrada)
}

// Conectada es la carpeta de `c` si ya estaba conectada en este equipo, también la de
// antes de atarlas: el Hub la vuelve a atender al arrancar o al volver a esa cuenta, sin
// que nadie pulse nada. Una cuenta que nunca se conectó no apunta ninguna.
func Conectada(c Cuenta) (string, bool) { return conectada(c, registrada) }

func conectada(c Cuenta, registrada func(string) bool) (string, bool) {
	casa, vs, err := vinculos()
	if err != nil || c.Sub == "" || c.Org == "" {
		return "", false
	}
	if i := slices.IndexFunc(vs, func(v vinculo) bool { return v.Emisor == c.Emisor && v.Sub == c.Sub && v.Org == c.Org }); i >= 0 {
		return vs[i].Ruta, registrada(vs[i].Ruta)
	}
	if vieja := filepath.Join(casa, carpetaVieja); c.Emisor == "" && registrada(vieja) && !atada(vs, vieja) {
		ruta, err := carpeta(c, registrada)
		return ruta, err == nil
	}
	return "", false
}

func carpeta(c Cuenta, registrada func(string) bool) (string, error) {
	if c.Sub == "" || c.Org == "" {
		return "", errSinCuenta
	}
	casa, vs, err := vinculos()
	if err != nil {
		return "", err
	}
	if i := slices.IndexFunc(vs, func(v vinculo) bool { return v.Emisor == c.Emisor && v.Sub == c.Sub && v.Org == c.Org }); i >= 0 {
		return vs[i].Ruta, nil
	}
	ruta := filepath.Join(casa, carpetaVieja)
	if c.Emisor != "" || !registrada(ruta) || atada(vs, ruta) {
		etiqueta := c.Org
		if (auth.Reclamos{Tenant: c.Org}).EsCuentaPersonal() {
			etiqueta = "Personal"
		}
		nombre := carpetaVieja + " – " + etiqueta
		if !nombreValido(nombre) {
			// El `tenant` sale del token: nada que no sea un nombre, y menos «../».
			return "", errors.New("nube: la organización no da un nombre de carpeta: " + c.Org)
		}
		ocupado := func(n string) bool {
			r := filepath.Join(casa, n)
			_, err := os.Lstat(r)
			return err == nil || atada(vs, r) // lo que ya hay no se hace de Drive: subiría
		}
		if ocupado(nombre) {
			nombre = libre(nombre, true, "", ocupado)
		}
		ruta = filepath.Join(casa, nombre)
	}
	return ruta, guardarVinculos(append(vs, vinculo{Emisor: c.Emisor, Sub: c.Sub, Org: c.Org, Ruta: ruta}))
}

// atada dice si `ruta` ya es la carpeta de alguna cuenta.
func atada(vs []vinculo, ruta string) bool {
	return slices.ContainsFunc(vs, func(v vinculo) bool { return strings.EqualFold(v.Ruta, ruta) })
}

// vinculos son la carpeta personal, donde van las de Drive, y lo apuntado.
func vinculos() (casa string, vs []vinculo, err error) {
	if casa, err = os.UserHomeDir(); err != nil {
		return "", nil, err
	}
	fichero, err := ficheroVinculos()
	if err != nil {
		return "", nil, err
	}
	crudo, err := os.ReadFile(fichero)
	if errors.Is(err, os.ErrNotExist) {
		return casa, nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	// Ilegible es un error, no «nada apuntado»: con la lista vacía, otra cuenta se
	// quedaría la carpeta de quien ya la tenía.
	err = json.Unmarshal(crudo, &vs)
	return casa, vs, err
}

func ficheroVinculos() (string, error) {
	dir, err := ajustes.Carpeta()
	return filepath.Join(dir, "drive-carpetas.json"), err
}

// guardarVinculos lo escribe entero, de una vez: o lo de antes o lo nuevo.
func guardarVinculos(vs []vinculo) error {
	fichero, err := ficheroVinculos()
	if err != nil {
		return err
	}
	crudo, err := json.MarshalIndent(vs, "", "  ")
	if err != nil {
		return err
	}
	tmp := fichero + ".nuevo"
	if err := os.WriteFile(tmp, crudo, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, fichero)
}

// ErrOtraCuenta es que la sesión ya no es la de la carpeta.
var ErrOtraCuenta = errors.New("nube: la sesión ya no es la de esta carpeta")

// Atada es la sesión de la carpeta de `c`: solo da tokens de esa persona en esa
// organización. Con los de otra, todo lo de la carpeta falla y no se hace nada —ni se
// quita ni se sube— aunque el Hub aún no se haya enterado del cambio (FYS-04).
func Atada(s api.Session, c Cuenta) api.Session { return sesionAtada{s, c} }

type sesionAtada struct {
	api.Session
	c Cuenta
}

func (s sesionAtada) AccessToken(ctx context.Context) (string, error) {
	t, err := s.Session.AccessToken(ctx)
	if err != nil {
		return "", err
	}
	if r := auth.ReclamosDe(t); r.Sub != s.c.Sub || r.Tenant != s.c.Org {
		return "", ErrOtraCuenta
	}
	return t, nil
}
