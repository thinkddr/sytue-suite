// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// «Guardar como…» en el MÓVIL ([GuardarEnElMovil]): el selector del sistema no da una
// ruta donde escribir —Android da un `content://` (ACTION_CREATE_DOCUMENT) y iOS exporta
// un fichero que ya existe (UIDocumentPickerViewController)—, y quien llama a
// [GuardarComo] escribe en una ruta (la biblioteca, con un temporal y un renombrado). Así
// que se le da una COPIA DE TRABAJO de la app y un REFLEJO la lleva al documento elegido
// en cuanto aparece, y otra vez cada vez que cambia: Guardar (Ctrl/Cmd+S) sobre el
// documento anclado vuelve a llegar al sitio que eligió la persona.
//
// Lo de cada sistema está en movil_guardar_android.go y movil_guardar_ios.go; esto es lo
// común, sin etiqueta para probarlo en cualquier máquina.

// Lo que espera el reflejo a que la app escriba la copia tras elegir dónde, y cada cuánto
// mira si ha cambiado.
const (
	esperaPrimeraCopia = 10 * time.Minute
	pasoReflejo        = 2 * time.Second
)

// carpetaGuardados es donde viven las copias de trabajo, dentro de `base` (la carpeta
// privada de la app).
func carpetaGuardados(base string) string { return filepath.Join(base, "Guardados") }

// copiaDeTrabajo es la ruta de la copia de trabajo de `nombre` para el destino `clave`:
// una carpeta por destino (dos documentos que se llamen igual en sitios distintos no se
// pisan) y el nombre que dejó la persona, con una de las extensiones pedidas.
func copiaDeTrabajo(base, nombre, clave string, extensiones []string) string {
	h := sha256.Sum256([]byte(clave))
	return filepath.Join(carpetaGuardados(base), hex.EncodeToString(h[:8]), nombrePropuesto(filepath.Base(nombre), extensiones))
}

// reflejos son los que están en marcha en este proceso, por copia de trabajo, y los que
// se recuerdan entre arranques (`Guardados/reflejos.json`: copia → destino).
type reflejos struct {
	mu      sync.Mutex
	vivos   map[string]string
	fichero string // "" no se recuerda nada
	paso    time.Duration
	espera  time.Duration
}

func nuevosReflejos(base string) *reflejos {
	r := &reflejos{vivos: map[string]string{}, paso: pasoReflejo, espera: esperaPrimeraCopia}
	if base != "" {
		r.fichero = filepath.Join(carpetaGuardados(base), "reflejos.json")
	}
	return r
}

// errReintentar: ahora no se pudo copiar (la app sin ventana, la nube sin red), pero el
// destino sigue valiendo; se vuelve a intentar en la próxima vuelta.
var errReintentar = errors.New("ficheros: reintentar el reflejo")

// copiador lleva la copia de trabajo `ruta` a `destino`. nil, hecho; [errReintentar],
// luego; cualquier otro error, ese destino ya no vale (permiso revocado, documento
// borrado) y el reflejo se acaba.
type copiador func(ruta, destino string) error

// vigilar lleva `ruta` a `destino` con `copiar` cada vez que cambie (lo que ya hay se
// toma por llevado). Una copia de trabajo tiene un solo reflejo: volver a elegir destino
// para ella lo cambia. `recordar`: apuntarlo para el próximo arranque (solo donde el
// permiso sobrevive, Android).
func (r *reflejos) vigilar(ruta, destino string, recordar bool, copiar copiador) {
	r.mu.Lock()
	_, ya := r.vivos[ruta]
	r.vivos[ruta] = destino
	if recordar {
		r.guardarLocked()
	}
	r.mu.Unlock()
	if ya {
		return // la rutina que hay ya lee el destino nuevo
	}
	visto, _ := huellaDe(ruta)
	go r.bucle(ruta, visto, copiar)
}

func (r *reflejos) destino(ruta string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.vivos[ruta]
	return d, ok
}

func (r *reflejos) olvidar(ruta string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.vivos, ruta)
	r.guardarLocked()
}

// huellaDe es lo que dice si el fichero ha cambiado: tamaño y fecha. La biblioteca
// escribe con un renombrado, así que nunca se ve a medias.
func huellaDe(ruta string) (string, bool) {
	st, err := os.Stat(ruta)
	if err != nil || !st.Mode().IsRegular() {
		return "", false
	}
	return st.ModTime().UTC().Format(time.RFC3339Nano) + "/" + strconv.FormatInt(st.Size(), 10), true
}

func (r *reflejos) bucle(ruta, visto string, copiar copiador) {
	limite := time.Now().Add(r.espera)
	escrita := visto != ""
	for {
		destino, ok := r.destino(ruta)
		if !ok {
			return
		}
		h, existe := huellaDe(ruta)
		switch {
		case !existe && !escrita && time.Now().After(limite):
			// La app nunca escribió (falló, o la persona se fue): no hay nada que llevar.
			r.olvidar(ruta)
			return
		case existe && h != visto:
			escrita = true
			switch err := copiar(ruta, destino); {
			case err == nil:
				visto = h
			case errors.Is(err, errReintentar):
			default:
				r.olvidar(ruta)
				return
			}
		}
		time.Sleep(r.paso)
	}
}

func (r *reflejos) guardarLocked() {
	if r.fichero == "" {
		return
	}
	raw, err := json.MarshalIndent(r.vivos, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(r.fichero), 0o700); err != nil {
		return
	}
	tmp := r.fichero + ".tmp"
	if os.WriteFile(tmp, raw, 0o600) == nil {
		_ = os.Rename(tmp, r.fichero)
	}
}

// recordados son los reflejos apuntados por un arranque anterior cuya copia sigue ahí.
func (r *reflejos) recordados() map[string]string {
	raw, err := os.ReadFile(r.fichero)
	if err != nil {
		return nil
	}
	var m map[string]string
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	for ruta := range m {
		if _, ok := huellaDe(ruta); !ok {
			delete(m, ruta)
		}
	}
	return m
}

// tiposMovil son los tipos MIME que se dan al selector de Android, que con un tipo
// conocido propone la extensión correcta. Lo que no está va como binario.
var tiposMovil = map[string]string{
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"odt":  "application/vnd.oasis.opendocument.text",
	"ods":  "application/vnd.oasis.opendocument.spreadsheet",
	"odp":  "application/vnd.oasis.opendocument.presentation",
	"pdf":  "application/pdf",
	"txt":  "text/plain",
	"md":   "text/markdown",
	"csv":  "text/csv",
	"html": "text/html",
	"rtf":  "application/rtf",
}

func tipoMovil(nombre string) string {
	if t, ok := tiposMovil[strings.ToLower(strings.TrimPrefix(filepath.Ext(nombre), "."))]; ok {
		return t
	}
	return "application/octet-stream"
}
