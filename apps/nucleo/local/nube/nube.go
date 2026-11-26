// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package nube conecta Sytue Office, OPCIONALMENTE, con el almacenamiento en la nube de
// la persona. Sin cuentas no abre ni una conexión: las apps trabajan en local.
//
// Dos maneras, de menos a más:
//
//  1. Carpetas que ya sincroniza el sistema ([Sincronizadas]): OneDrive, Google Drive
//     para escritorio, iCloud Drive y la carpeta de Sytue Drive de la edición conectada.
//     No hace falta ninguna cuenta aquí: se abre y se guarda en esa carpeta como en
//     cualquier otra, y su cliente la sube. Incluso se puede elegir como carpeta de datos.
//  2. Cuentas conectadas directamente ([Proveedor]): Sytue Drive (WebDAV con una
//     contraseña de aplicación, sin iniciar sesión con Sytue ID en la app), cualquier
//     WebDAV (Nextcloud…), Google Drive y OneDrive / Microsoft 365 (OAuth con PKCE y
//     redirección a 127.0.0.1, sin secreto de cliente: D-18).
//
// Los tokens y contraseñas van a `secretos`; `nube.json` solo guarda qué cuentas hay.
package nube

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
	"git.sytue.eu/sytue/one/apps/nucleo/local/secretos"
)

// Tipos de cuenta.
const (
	SytueDrive = "sytue"
	WebDAV     = "webdav"
	Google     = "google"
	OneDrive   = "onedrive"
)

// DireccionSytueDrive es el WebDAV de Sytue Drive (servicio drive, ruta /dav).
const DireccionSytueDrive = "https://drive.sytue.eu/dav"

// Elemento es un fichero o una carpeta de la nube.
type Elemento struct {
	ID         string
	Nombre     string
	Carpeta    bool
	Tamano     int64
	Modificado time.Time
	// Tipo es el MIME que da el proveedor (Google lo necesita para exportar los suyos).
	Tipo string
	// Version es la marca de versión del proveedor (el ETag de WebDAV, la `version` de
	// Google, el `eTag` de Graph): cambia cuando cambia el contenido. Vacía si no la da.
	Version string
	// Creado es cuándo se creó, si el proveedor lo da: entre dos de una carpeta que se
	// llaman igual (Google Drive lo admite), el más antiguo va primero ([Ordenar]).
	Creado time.Time
	// Huella es la suma del contenido que da el proveedor, con su algoritmo delante
	// («md5:…» en Google Drive, «sha1:…» en OneDrive personal), para comprobar una copia
	// sin volver a bajarla. Vacía si no la da (WebDAV, OneDrive de empresa).
	Huella string
	// Estable es un identificador que no cambia al renombrar ni al mover: el ID en Google
	// Drive y OneDrive; en WebDAV, el `oc:fileid` si el servidor lo da (Nextcloud,
	// ownCloud). Vacío si no hay ninguno: el ID de WebDAV es su ruta.
	Estable string
}

// Proveedor es una cuenta conectada.
type Proveedor interface {
	// Listar devuelve el contenido de la carpeta id ("" es la raíz).
	Listar(ctx context.Context, carpeta string) ([]Elemento, error)
	// Descargar devuelve el contenido del fichero id y el nombre con que guardarlo.
	Descargar(ctx context.Context, e Elemento) ([]byte, string, error)
	// Subir crea o reemplaza el fichero nombre dentro de la carpeta.
	Subir(ctx context.Context, carpeta, nombre string, datos []byte) (Elemento, error)
}

// Gestor es lo que un proveedor sabe hacer con carpetas y ficheros además de listar,
// bajar y subir. Opcional: quien lo use comprueba si el proveedor lo cumple.
type Gestor interface {
	// CrearCarpeta crea nombre dentro de padre ("" es la raíz).
	CrearCarpeta(ctx context.Context, padre, nombre string) (Elemento, error)
	// Renombrar cambia el nombre de e, en su misma carpeta.
	Renombrar(ctx context.Context, e Elemento, nombre string) (Elemento, error)
	// Mover lleva e a la carpeta destino de la misma cuenta.
	Mover(ctx context.Context, e Elemento, destino string) (Elemento, error)
	// Borrar manda e a la papelera del proveedor si la tiene (Google, OneDrive) y, si no
	// (WebDAV), lo borra.
	Borrar(ctx context.Context, e Elemento) error
}

// Reemplazador sabe subir el contenido nuevo de UN fichero por su identificador, sin
// pasar por su nombre: en una carpeta de Google Drive puede haber dos que se llamen igual,
// y Subir por el nombre pisaría el que no es. Opcional, como [Gestor].
type Reemplazador interface {
	// Reemplazar sube datos encima del fichero e (por e.ID) y devuelve cómo quedó.
	Reemplazar(ctx context.Context, e Elemento, datos []byte) (Elemento, error)
}

// ErrSinCuenta: la cuenta no existe o le falta su secreto.
var ErrSinCuenta = errors.New("nube: cuenta no configurada")

// Cuenta es lo que se guarda de una cuenta, sin secretos.
type Cuenta struct {
	ID      string `json:"id"`
	Tipo    string `json:"tipo"`
	Rotulo  string `json:"rotulo"`
	URL     string `json:"url,omitempty"`     // WebDAV
	Usuario string `json:"usuario,omitempty"` // WebDAV
	// ClienteID es el identificador OAuth con que se conectó (Google, OneDrive).
	ClienteID string `json:"cliente_id,omitempty"`
}

// Registro son las cuentas de la edición.
type Registro struct {
	ruta     string
	secretos *secretos.Almacen
	// HTTP es el cliente de red (el del portero); nil, ninguno: falla cerrado.
	HTTP *http.Client
	// Clientes OAuth de la compilación; la persona puede poner el suyo.
	Clientes ClientesOAuth

	mu      sync.Mutex
	cuentas []Cuenta
}

// ClientesOAuth son los identificadores de cliente público de la compilación. Si faltan,
// esa opción pide a la persona el suyo (D-18).
type ClientesOAuth struct {
	Google, GoogleSecreto string // Google exige «secreto» también en apps de escritorio; no es secreto
	Microsoft             string
}

// Abrir lee las cuentas de e.
func Abrir(e *espacio.Espacio, s *secretos.Almacen) *Registro {
	r := &Registro{ruta: e.Ajuste("nube.json"), secretos: s, Clientes: ClientesCompilados()}
	var guardado struct {
		Cuentas []Cuenta `json:"cuentas"`
	}
	_ = espacio.LeerJSON(r.ruta, &guardado)
	r.cuentas = guardado.Cuentas
	return r
}

// Cuentas devuelve las cuentas configuradas.
func (r *Registro) Cuentas() []Cuenta {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.cuentas)
}

func (r *Registro) guardar() error {
	return espacio.EscribirJSON(r.ruta, struct {
		Cuentas []Cuenta `json:"cuentas"`
	}{r.cuentas})
}

func secretoDe(id string) string { return "nube-" + id }

// Añadir guarda una cuenta nueva con su secreto (contraseña o token de refresco).
func (r *Registro) Añadir(c Cuenta, secreto string) (Cuenta, error) {
	if c.Tipo != SytueDrive && c.Tipo != WebDAV && c.Tipo != Google && c.Tipo != OneDrive {
		return c, fmt.Errorf("nube: tipo desconocido %q", c.Tipo)
	}
	if c.Tipo == SytueDrive && c.URL == "" {
		c.URL = DireccionSytueDrive
	}
	if (c.Tipo == SytueDrive || c.Tipo == WebDAV) && !strings.HasPrefix(c.URL, "https://") {
		return c, errors.New("nube: la dirección WebDAV tiene que ser https")
	}
	c.ID = uuid.NewString()
	if strings.TrimSpace(c.Rotulo) == "" {
		c.Rotulo = map[string]string{SytueDrive: "Sytue Drive", WebDAV: "WebDAV", Google: "Google Drive", OneDrive: "OneDrive"}[c.Tipo]
	}
	if err := r.secretos.Guardar(secretoDe(c.ID), secreto); err != nil {
		return c, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cuentas = append(r.cuentas, c)
	return c, r.guardar()
}

// Quitar borra la cuenta y su secreto. Los ficheros descargados se quedan.
func (r *Registro) Quitar(id string) error {
	_ = r.secretos.Borrar(secretoDe(id))
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cuentas = slices.DeleteFunc(r.cuentas, func(c Cuenta) bool { return c.ID == id })
	return r.guardar()
}

func (r *Registro) http() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	// Sin el del portero, ninguno: lo no configurado falla cerrado (R-RED).
	return &http.Client{Transport: red.Denegar{}, Timeout: 60 * time.Second}
}

// Proveedor abre la cuenta id.
func (r *Registro) Proveedor(id string) (Proveedor, error) {
	r.mu.Lock()
	i := slices.IndexFunc(r.cuentas, func(c Cuenta) bool { return c.ID == id })
	var c Cuenta
	if i >= 0 {
		c = r.cuentas[i]
	}
	r.mu.Unlock()
	if i < 0 {
		return nil, ErrSinCuenta
	}
	secreto, err := r.secretos.Leer(secretoDe(id))
	if err != nil {
		return nil, err
	}
	if secreto == "" {
		return nil, ErrSinCuenta
	}
	switch c.Tipo {
	case SytueDrive, WebDAV:
		return &DAV{Base: c.URL, Usuario: c.Usuario, Clave: secreto, HTTP: r.http()}, nil
	case Google:
		return &GoogleDrive{Token: r.token(c, secreto, OAuthGoogle(c.ClienteID, r.Clientes.GoogleSecreto)), HTTP: r.http()}, nil
	case OneDrive:
		return &OneDriveGraph{Token: r.token(c, secreto, OAuthMicrosoft(c.ClienteID)), HTTP: r.http()}, nil
	}
	return nil, ErrSinCuenta
}

// token devuelve una fuente de tokens de acceso que renueva con el de refresco y guarda
// el nuevo de refresco si el proveedor lo rota.
func (r *Registro) token(c Cuenta, refresco string, o OAuth) func(context.Context) (string, error) {
	var (
		mu     sync.Mutex
		acceso string
		vence  time.Time
	)
	return func(ctx context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if acceso != "" && time.Now().Before(vence) {
			return acceso, nil
		}
		t, err := o.Renovar(ctx, r.http(), refresco)
		if err != nil {
			return "", err
		}
		if t.Refresco != "" && t.Refresco != refresco {
			refresco = t.Refresco
			_ = r.secretos.Guardar(secretoDe(c.ID), refresco)
		}
		acceso, vence = t.Acceso, time.Now().Add(time.Duration(max(t.Expira-60, 30))*time.Second)
		return acceso, nil
	}
}
