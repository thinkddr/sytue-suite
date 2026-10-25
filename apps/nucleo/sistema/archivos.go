// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
)

// # Sytue Drive en el Finder y en Archivos de iOS
//
// Lo enseña una extensión File Provider (`NSFileProviderReplicatedExtension`, macOS 13 e
// iOS 16) que va dentro de la app de Drive: apps/fyne/xcode/proveedor-archivos. La
// extensión la arranca el sistema cuando hace falta, con la app CERRADA, así que no puede
// tirar de la sesión OIDC de la suite —nadie le renovaría el token— y habla WebDAV con
// Drive con una CONTRASEÑA DE APLICACIÓN de este equipo, que se revoca en Mi cuenta.
//
// Aquí está la mitad de la app: guardar esa credencial en el llavero COMPARTIDO con la
// extensión (un grupo de acceso propio, no el de la sesión de la suite: la extensión no
// tiene por qué poder leer el refresh token) y dar de alta o de baja el dominio en
// `NSFileProviderManager`. Solo existe en las compilaciones de Apple que llevan la
// extensión —iOS y macOS—; en las demás, ErrArchivosNoDisponible.

// Lo que la extensión lee del llavero. Los mismos nombres en Proveedor.swift.
const (
	// GrupoLlaveroArchivos es el keychain-access-group que comparten la app de Drive y su
	// extensión: lo declaran los dos en sus derechos (empaquetar.sh, derechos-tienda y
	// derechos-archivos). El equipo es el de generar.py (EQUIPO).
	GrupoLlaveroArchivos = "VHC5QW7BZ3.eu.sytue.drive.shared"
	servicioArchivos     = "eu.sytue.drive.archivos"
	cuentaArchivos       = "webdav"
)

// CredencialArchivos es lo que la extensión necesita para hablar con Drive.
type CredencialArchivos struct {
	Identidad string `json:"identidad"`
	// URL es la raíz WebDAV de Drive (Coordenadas.WebDAVURL), https.
	URL string `json:"url"`
	// Usuario es el correo de quien entra; Sytue ID lo contrasta con el dueño de la clave.
	Usuario string `json:"usuario"`
	// Clave es la contraseña de aplicación, en claro: solo vive en el llavero.
	Clave string `json:"clave"`
}

// ErrArchivosNoDisponible es que esta compilación no lleva la extensión, o que el
// sistema es anterior a macOS 13 o iOS 16.
var ErrArchivosNoDisponible = errors.New("sistema: este sistema no tiene el proveedor de archivos de Drive")

var errCredencialArchivos = errors.New("sistema: credencial de archivos incompleta o sin https")

// ArchivosDisponible dice si aquí se puede poner Drive en el Finder o en Archivos.
func ArchivosDisponible() bool { return archivosDisponible() }

// GuardarCredencialArchivos deja la credencial en el llavero compartido con la extensión,
// sustituyendo la que hubiera.
func GuardarCredencialArchivos(c CredencialArchivos) error {
	u, err := url.Parse(c.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || c.Usuario == "" || c.Clave == "" || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(c.Identidad) {
		return errCredencialArchivos
	}
	crudo, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return guardarCredencialArchivos(crudo)
}

// HayCredencialArchivos dice si ya hay una credencial guardada.
func HayCredencialArchivos() (bool, error) { return hayCredencialArchivos() }

// RegistrarArchivos da de alta el dominio con `nombre` (el que sale en la barra lateral
// del Finder y en Archivos). Volver a darlo de alta no duplica nada.
func RegistrarArchivos(nombre string) error { return registrarArchivos(nombre) }

// OlvidarArchivos da de baja el dominio —el sistema borra su copia local— y la
// credencial. Lo que no había, no es un error.
func OlvidarArchivos() error { return olvidarArchivos() }

// AbrirArchivos abre el dominio de Drive en Finder o en el selector nativo de Archivos.
func AbrirArchivos() error { return abrirArchivos() }
