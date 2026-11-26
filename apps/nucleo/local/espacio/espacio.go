// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package espacio es la identidad y las rutas de Sytue Office, la edición local de la
// suite: sin cuenta, sin cifrado y sin servidor (apps/fyne/PLAN-EDICION-LOCAL.md §5.2).
//
// Es la única fuente de esos valores. Todo es distinto de la edición conectada
// (R-CONVIVE): identificador, esquema, carpeta de ajustes, llavero y carpeta de datos, así
// que las dos pueden estar instaladas a la vez sin pisarse.
package espacio

import (
	"cmp"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// Identidad de la edición.
const (
	Nombre            = "Sytue Office"
	PrefijoID         = "eu.sytue.office."
	PrefijoEsquema    = "sytueoffice-"
	PrefijoEjecutable = "sytue-office-"
	Carpeta           = "sytue-office"
	ServicioLlavero   = "eu.sytue.office"
	CarpetaDatos      = "Sytue Office"
)

// Esquema es el esquema de URL de producto en esta edición. No empieza por «sytue-»
// a propósito: la edición conectada decide por ese prefijo (`api/abrir.go`).
func Esquema(producto string) string { return PrefijoEsquema + producto }

// Rutas son las carpetas de la edición.
type Rutas struct {
	// Ajustes: perfil, IA, nube (sin secretos). `<config>/sytue-office`.
	Ajustes string
	// Cache: se puede borrar sin perder nada. `<caché>/sytue-office`.
	Cache string
	// Datos: lo de la persona, visible y elegible. Por defecto `~/Sytue Office`.
	Datos string
	// ParaEnviar: lo que una app de la edición deja para que Sytue Mail lo adjunte (el PDF
	// de una factura del ERP), `<caché>/sytue-office/para-enviar`, compartida por todas
	// las apps de este equipo y fuera de los datos de la persona (no se sincroniza ni entra
	// en la copia). En iOS, dentro del App Group; vacía donde las apps no comparten
	// ninguna carpeta (Android, iOS sin grupo, la variante de la tienda de macOS): ahí no
	// se adjunta nada entre apps (`nucleo/local/correo.ParaEnviar`).
	ParaEnviar string
}

// CarpetaParaEnviar es el nombre de la carpeta de Rutas.ParaEnviar.
const CarpetaParaEnviar = "para-enviar"

// Producto es la carpeta de datos de un producto: `<Datos>/Datos/<producto>`.
func (r Rutas) Producto(producto string) string {
	return filepath.Join(r.Datos, "Datos", producto)
}

// Biblioteca es la carpeta de documentos de la ofimática: `<Datos>/Documentos`.
func (r Rutas) Biblioteca() string { return filepath.Join(r.Datos, "Documentos") }

// Perfil firma comentarios, cambios y anotaciones (D-30). Se guarda en claro.
type Perfil struct {
	Nombre string `json:"nombre,omitempty"`
	Correo string `json:"correo,omitempty"`
	// Datos es la carpeta de datos elegida; vacío, la de por defecto.
	Datos string `json:"datos,omitempty"`
	// Idioma y Tema son la apariencia: «» sigue al sistema.
	Idioma string `json:"idioma,omitempty"`
	Tema   string `json:"tema,omitempty"`
}

// Espacio es la edición abierta en este equipo.
type Espacio struct {
	Rutas  Rutas
	Perfil Perfil
	// ID es el identificador aleatorio de esta instalación: el «sujeto» de los alcances.
	ID string
}

// GrupoApple es el App Group que comparten en iOS las apps de la edición
// (`group.<ServicioLlavero>`, el que pone `empaquetar.sh` al firmar).
const GrupoApple = "group." + ServicioLlavero

// Abrir lee el espacio. raizMovil es la carpeta de la app en iOS y Android (donde no hay
// carpeta personal visible); vacía en escritorio.
func Abrir(raizMovil string) (*Espacio, error) {
	return AbrirMovil(raizMovil, "")
}

// AbrirMovil es Abrir con, además, la carpeta compartida por todas las apps de la
// edición (el App Group de iOS; vacía si no hay): los AJUSTES —perfil, IA, cuentas,
// secretos— van ahí, para configurarse una vez para todas como en el escritorio. Los
// datos y la caché siguen siendo de cada app. La primera vez se llevan allí los ajustes
// que la app tuviera en su carpeta, sin pisar los que ya hubiera dejado otra.
func AbrirMovil(raizMovil, grupo string) (*Espacio, error) {
	var r Rutas
	if raizMovil != "" {
		r = Rutas{Ajustes: filepath.Join(raizMovil, "ajustes"), Cache: filepath.Join(raizMovil, "cache"), Datos: filepath.Join(raizMovil, "datos")}
		if grupo != "" {
			compartidos := filepath.Join(grupo, "ajustes")
			if err := llevarAjustes(r.Ajustes, compartidos); err != nil {
				return nil, err
			}
			r.Ajustes = compartidos
			r.ParaEnviar = filepath.Join(grupo, CarpetaParaEnviar)
		}
	} else {
		cfg, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		cache, err := os.UserCacheDir()
		if err != nil {
			cache = filepath.Join(cfg, Carpeta, "cache")
		} else {
			cache = filepath.Join(cache, Carpeta)
		}
		casa, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		r = Rutas{Ajustes: filepath.Join(cfg, Carpeta), Cache: cache, Datos: filepath.Join(casa, CarpetaDatos)}
		// En la tienda de macOS cada app vive en su contenedor: la caché no se comparte.
		if !escritorio.DeTienda {
			r.ParaEnviar = filepath.Join(cache, CarpetaParaEnviar)
		}
	}
	if v := os.Getenv("SYTUE_OFFICE_DATOS"); v != "" {
		r.Datos = v
	}
	if err := os.MkdirAll(r.Ajustes, 0o700); err != nil {
		return nil, err
	}
	e := &Espacio{Rutas: r}
	if err := LeerJSON(filepath.Join(r.Ajustes, "perfil.json"), &e.Perfil); err != nil {
		return nil, err
	}
	if d := strings.TrimSpace(e.Perfil.Datos); d != "" {
		e.Rutas.Datos = d
	}
	id, err := almacen.IDInstalacion(filepath.Join(r.Ajustes, "instalacion"))
	if err != nil {
		return nil, err
	}
	e.ID = id
	return e, os.MkdirAll(e.Rutas.Datos, 0o700)
}

// llevarAjustes copia a la carpeta compartida lo que haya en la de la app y no esté ya
// allí (el primero que llega pone el perfil; los demás no lo pisan). Lo de la app se
// deja donde estaba: no se borra nada.
func llevarAjustes(deApp, compartidos string) error {
	return filepath.WalkDir(deApp, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		rel, err := filepath.Rel(deApp, ruta)
		if err != nil {
			return err
		}
		destino := filepath.Join(compartidos, rel)
		if d.IsDir() {
			return os.MkdirAll(destino, 0o700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if _, err := os.Lstat(destino); err == nil {
			return nil
		}
		datos, err := os.ReadFile(ruta) //nolint:gosec // G122: la carpeta de ajustes de la propia app, de quien la ejecuta
		if err != nil {
			return err
		}
		return os.WriteFile(destino, datos, 0o600) //nolint:gosec // G703: la carpeta compartida más la ruta relativa de lo recorrido
	})
}

// Persona es quien firma, para los almacenes.
func (e *Espacio) Persona() biblioteca.Persona {
	return biblioteca.Persona{ID: e.ID, Nombre: cmp.Or(strings.TrimSpace(e.Perfil.Nombre), "Yo")}
}

// GuardarPerfil escribe el perfil.
func (e *Espacio) GuardarPerfil(p Perfil) error {
	e.Perfil = p
	return EscribirJSON(filepath.Join(e.Rutas.Ajustes, "perfil.json"), p)
}

// Ajuste es la ruta de un fichero de ajustes de la edición.
func (e *Espacio) Ajuste(nombre string) string { return filepath.Join(e.Rutas.Ajustes, nombre) }

// LeerJSON lee ruta en v; un fichero que no existe deja v como estaba.
func LeerJSON(ruta string, v any) error {
	raw, err := os.ReadFile(ruta)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

// EscribirJSON escribe v en ruta, legible y sin dejar nunca un fichero a medias.
func EscribirJSON(ruta string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return biblioteca.EscribirAtomico(ruta, append(raw, '\n'))
}
