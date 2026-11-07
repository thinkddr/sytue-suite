// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Metadatos escribe el fyne_metadata_init.go de una app, el que `fyne build` inyectaba
// antes de compilar, para los binarios de iOS que no pasan por `fyne package`.
//
// Desde fyne.io/tools 1.7.4, `fyne build` no admite iOS: los paquetes móviles salen de
// `fyne package --target ios`, y así sale el de TestFlight. Pero el Simulador (que con
// `fyne package` pide un Team ID) y la fase de compilación del proyecto de Xcode (donde
// es Xcode quien arma la app) compilan el binario de Go por su cuenta, y sin este
// fichero la app arranca sin ID, nombre ni metadatos propios: los ajustes no se guardan
// y el enchufe de URL de la sesión no se abre.
//
// Reproduce lo que hace la CLI 1.7.4 (createMetadataInitFile y appData.mergeMetadata
// en fyne.io/tools/cmd/fyne/internal/commands): los --metadata mandan y el resto sale
// de [Release] con -release o de [Development] sin él; el icono es el Icon.png de la app
// (o el de [Details]) reducido a 512 px. Imprime en la salida las etiquetas de
// compilación que `fyne build` añadía ("release", "migrated_fynedo").
//
//	go run ./herramientas/metadatos -dir cmd/rooms -salida cmd/rooms/fyne_metadata_init.go [-release] [-metadata k=v]...
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"fyne.io/fyne/v2"
	"github.com/BurntSushi/toml"
	"golang.org/x/image/draw"
)

// iconoMaximo es el maxIconSize de la CLI: más grande, el icono solo engorda el binario.
const iconoMaximo = 512

type fyneApp struct {
	Details struct {
		Icon, Name, ID, Version string
		Build                   int
	}
	Development map[string]string
	Release     map[string]string
	Migrations  map[string]bool
}

// Datos es lo que va a la plantilla; los nombres siguen a appData de la CLI.
type Datos struct {
	AppID, Name, AppVersion string
	AppBuild                int
	Release                 bool
	CustomMetadata          map[string]string
	Migrations              map[string]bool
	ResGoString             string
	// Lo que no va a la plantilla pero sí a quien compila.
	Etiquetas []string
}

// Opciones son las de la orden: el directorio de la app y lo que la CLI recibía por flags.
type Opciones struct {
	Dir, Version string
	Build        int
	Release      bool
	Metadata     map[string]string
}

// Preparar combina el FyneApp.toml de la app con las opciones como la CLI.
func Preparar(o Opciones) (*Datos, error) {
	var app fyneApp
	if _, err := toml.DecodeFile(filepath.Join(o.Dir, "FyneApp.toml"), &app); err != nil {
		return nil, fmt.Errorf("FyneApp.toml: %w", err)
	}
	if app.Details.ID == "" || app.Details.Name == "" {
		return nil, errors.New("FyneApp.toml no declara ID y Name en [Details]")
	}
	d := &Datos{
		AppID: app.Details.ID, Name: app.Details.Name, AppVersion: o.Version, AppBuild: o.Build,
		Release: o.Release, CustomMetadata: map[string]string{}, Migrations: app.Migrations,
	}
	if d.AppVersion == "" {
		d.AppVersion = app.Details.Version
	}
	if d.AppBuild <= 0 {
		d.AppBuild = app.Details.Build
	}
	for k, v := range o.Metadata {
		d.CustomMetadata[k] = v
	}
	tabla := app.Development
	if o.Release {
		tabla = app.Release
	}
	for k, v := range tabla {
		if _, ya := d.CustomMetadata[k]; !ya {
			d.CustomMetadata[k] = v
		}
	}
	// Como la CLI: primero el Icon.png de la app; si no hay, el de [Details].
	icono := filepath.Join(o.Dir, "Icon.png")
	if _, err := os.Stat(icono); err != nil {
		icono = ""
		if app.Details.Icon != "" {
			icono = filepath.Join(o.Dir, app.Details.Icon)
		}
	}
	d.ResGoString = "nil"
	if icono != "" {
		res, err := recursoIcono(icono)
		if err != nil {
			return nil, err
		}
		d.ResGoString = res
	}
	if o.Release {
		d.Etiquetas = append(d.Etiquetas, "release")
	}
	if app.Migrations["fyneDo"] {
		d.Etiquetas = append(d.Etiquetas, "migrated_fynedo")
	}
	return d, nil
}

// recursoIcono devuelve el icono como fyne.StaticResource, ya reducido a iconoMaximo y
// con el nombre que le pone metadata.ScaleIcon de la CLI.
func recursoIcono(ruta string) (string, error) {
	contenido, err := os.ReadFile(ruta)
	if err != nil {
		return "", fmt.Errorf("icono: %w", err)
	}
	nombre := filepath.Base(ruta)
	img, err := png.Decode(bytes.NewReader(contenido))
	if err != nil {
		return "", fmt.Errorf("icono %s: %w", ruta, err)
	}
	if img.Bounds().Dx() > iconoMaximo {
		reducido := image.NewNRGBA(image.Rect(0, 0, iconoMaximo, iconoMaximo))
		draw.CatmullRom.Scale(reducido, reducido.Bounds(), img, img.Bounds(), draw.Src, nil)
		var buf bytes.Buffer
		if err := png.Encode(&buf, reducido); err != nil {
			return "", fmt.Errorf("icono reducido: %w", err)
		}
		contenido = buf.Bytes()
		sufijo := "-" + strconv.Itoa(iconoMaximo) + ".png"
		if n := len(nombre); n <= 4 || nombre[n-4] != '.' {
			nombre = "appicon" + sufijo
		} else {
			nombre = nombre[:n-4] + sufijo
		}
	}
	// El GoString del propio Fyne (el fork de apps/fyne/go.mod), como la CLI.
	return fyne.NewStaticResource(nombre, contenido).GoString(), nil
}

// La plantilla es la de la CLI (templates/data/fyne_metadata_init.got) para Fyne 2.6 o
// posterior, que es la de apps/fyne/go.mod. Las cadenas van con %q y no entre comillas
// a pelo: un valor con comillas no rompe el fichero.
var plantilla = template.Must(template.New("metadatos").Funcs(template.FuncMap{
	"q":     strconv.Quote,
	"clave": ordenadas,
	"bool":  claves,
}).Parse(`// Code generated by apps/fyne/herramientas/metadatos. DO NOT EDIT.

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func init() {
	app.SetMetadata(fyne.AppMetadata{
		ID:      {{q .AppID}},
		Name:    {{q .Name}},
		Version: {{q .AppVersion}},
		Build:   {{.AppBuild}},
		Icon:    {{.ResGoString}},
		Release: {{.Release}},
		Custom: map[string]string{
{{- range $k := clave .CustomMetadata}}
			{{q $k}}: {{q (index $.CustomMetadata $k)}},
{{- end}}
		},
		Migrations: map[string]bool{
{{- range $k := bool .Migrations}}
			{{q $k}}: {{index $.Migrations $k}},
{{- end}}
		},
	})
}
`))

func ordenadas(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func claves(m map[string]bool) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Escribir genera el fichero; Go lo compila en la misma pasada que el resto del main.
func Escribir(d *Datos, salida string) error {
	var buf bytes.Buffer
	if err := plantilla.Execute(&buf, d); err != nil {
		return err
	}
	return os.WriteFile(salida, buf.Bytes(), 0o600)
}

type metadatosFlag map[string]string

func (m metadatosFlag) String() string { return "" }

func (m metadatosFlag) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok || k == "" {
		return fmt.Errorf("-metadata espera clave=valor y es %q", v)
	}
	m[k] = val
	return nil
}

func main() {
	o := Opciones{Metadata: metadatosFlag{}}
	salida := flag.String("salida", "", "fichero que se escribe (fyne_metadata_init.go en el directorio de la app)")
	flag.StringVar(&o.Dir, "dir", ".", "directorio de la app (el del FyneApp.toml)")
	flag.StringVar(&o.Version, "version", "", "versión; vacía, la del FyneApp.toml")
	flag.IntVar(&o.Build, "build", 0, "build; 0, el del FyneApp.toml")
	flag.BoolVar(&o.Release, "release", false, "como --release de la CLI: [Release] y la etiqueta release")
	flag.Var(metadatosFlag(o.Metadata), "metadata", "clave=valor, como --metadata de la CLI (se puede repetir)")
	flag.Parse()
	if *salida == "" {
		fmt.Fprintln(os.Stderr, "falta -salida")
		os.Exit(2)
	}
	d, err := Preparar(o)
	if err == nil {
		err = Escribir(d, *salida)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "::error::metadatos de", o.Dir+":", err)
		os.Exit(1)
	}
	fmt.Println(strings.Join(d.Etiquetas, ","))
}
