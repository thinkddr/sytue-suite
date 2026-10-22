// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/guiones"
)

// # Scripts de usuario (ROADMAP-OFFICE automatizacion-1, pieza A)
//
// Modelo del almacén de scripts: el guion, sus versiones INMUTABLES, el manifiesto de
// permisos que declara qué puede hacer, la aprobación de un administrador y el registro
// de ejecuciones. El servidor NO ejecuta el código —corre aislado en el navegador (pieza
// B)—: aquí se decide QUIÉN puede ejecutar QUÉ versión sobre QUÉ documento, con qué
// cuota, y se deja constancia. La escritura sigue pasando por /ops, que reautoriza la
// ACL y la protección en cada operación: un script nunca es una vía para saltárselas.

// ManifiestoVersion es la versión del esquema del manifiesto que este servidor entiende.
const ManifiestoVersion = 1

// Cotas del manifiesto y del código. Las que no dependen del producto (código, nombre,
// topes y valores por defecto de una ejecución) son las de `platform/pkg/guiones`, que
// comparten Docs y Bases; las del manifiesto de Docs viven aquí.
const (
	MaxCodigoScript   = guiones.MaxCodigo
	MaxNombreScript   = guiones.MaxNombre
	MaxComandosScript = 100
	MaxAmbitoScript   = 50
	TopeOpsScript     = guiones.TopeOps
	TopeMsScript      = guiones.TopeMs
	OpsPorDefecto     = guiones.OpsPorDefecto
	MsPorDefecto      = guiones.MsPorDefecto
)

// Cuotas por hora móvil: las FIJAS de `guiones`, las mismas para todos los productos.
const (
	CuotaEjecucionesUsuarioHora = guiones.CuotaEjecucionesUsuarioHora
	CuotaEjecucionesOrgHora     = guiones.CuotaEjecucionesOrgHora
	CuotaMsUsuarioHora          = guiones.CuotaMsUsuarioHora
)

// Estados de una versión.
const (
	EstadoBorrador  = guiones.EstadoBorrador
	EstadoPendiente = guiones.EstadoPendiente
	EstadoAprobada  = guiones.EstadoAprobada
	EstadoRechazada = guiones.EstadoRechazada
)

// Ámbitos de visibilidad de un script.
const (
	AmbitoPrivado = "private"
	AmbitoOrg     = "org"
)

// Resultados de una ejecución.
const (
	ResEjecutando = guiones.ResEjecutando
	ResOK         = guiones.ResOK
	ResError      = guiones.ResError
	ResDenegado   = guiones.ResDenegado
	ResTiempo     = guiones.ResTiempo
	ResLimite     = guiones.ResLimite
)

var (
	ErrScriptInvalido        = errors.New("docs: script inválido")
	ErrScriptSinAprobar      = errors.New("docs: la versión no está aprobada para la organización")
	ErrCuotaScripts          = errors.New("docs: cuota de ejecuciones agotada")
	ErrConflictoEstadoScript = errors.New("docs: la versión no está en un estado que admita esa transición")
	ErrHashNoCoincide        = errors.New("docs: el hash no corresponde al código de esa versión")
)

// AmbitoManifiesto acota dónde actúa el script dentro del documento. Vacío = todo el documento.
type AmbitoManifiesto struct {
	Sheets []string `json:"sheets,omitempty"`
	Ranges []string `json:"ranges,omitempty"`
}

// ManifiestoScript es lo que el script declara necesitar. El usuario lo ve antes de la
// primera ejecución y cualquier cambio exige un consentimiento nuevo (el cliente compara
// el hash del manifiesto canónico).
type ManifiestoScript struct {
	V          int              `json:"v"`
	Commands   []string         `json:"commands"`   // comandos del registro común que puede despachar
	Read       []string         `json:"read"`       // familias de datos que puede leer
	Write      bool             `json:"write"`      // si puede editar el documento
	Scope      AmbitoManifiesto `json:"scope"`      // hojas y rangos
	MaxOps     int              `json:"max_ops"`    // operaciones por ejecución
	MaxMs      int              `json:"max_ms"`     // tiempo por ejecución
	Unattended bool             `json:"unattended"` // si puede correr sin que nadie lo pulse
}

// comandosJSON es la copia de build del registro común de comandos de la cinta
// (apps/nucleo/office/comandos.json; la publica y la vigila scripts/sync-office-catalog.sh).
//
//go:embed comandos.json
var comandosJSON []byte

// comandosDeScript son los comandos que un script puede declarar: los que el registro
// marca con `script: true`. Es una lista MÁS CORTA que la de las macros (decisión del
// dueño, 2026-10-04): fuera quedan la IA, lo que abre diálogos o gestores, lo que sale del
// documento, permisos, protección, plantillas, marca y deshacer/rehacer. Sin marca = no.
var comandosDeScript = sync.OnceValue(func() map[string]bool {
	var filas map[string]struct {
		Script bool `json:"script"`
	}
	if err := json.Unmarshal(comandosJSON, &filas); err != nil || len(filas) == 0 {
		panic("docs: registro de comandos ilegible")
	}
	out := make(map[string]bool, len(filas))
	for id, f := range filas {
		if f.Script {
			out[id] = true
		}
	}
	return out
})

// ComandoDeScript dice si un script puede declarar y despachar ese comando.
func ComandoDeScript(id string) bool { return comandosDeScript()[id] }

var (
	reComando = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]{0,31}$`)
	reRango   = regexp.MustCompile(`^[A-Z]{1,3}\d{1,7}(:[A-Z]{1,3}\d{1,7})?$`)
	// Lo que un script puede declarar leer: celdas, texto del folio, diapositivas, selección y título.
	lecturasValidas = []string{"cells", "selection", "slides", "text", "title"}
)

// ParseManifiesto decodifica y valida un manifiesto (estricto: campos desconocidos y
// datos sobrantes son error) y lo devuelve normalizado: listas sin repetidos y ordenadas,
// cotas por defecto aplicadas. El JSON canónico (json.Marshal del resultado) es el que se guarda.
func ParseManifiesto(crudo []byte) (ManifiestoScript, error) {
	var m ManifiestoScript
	if len(crudo) == 0 || len(crudo) > 16*1024 {
		return m, fmt.Errorf("%w: manifiesto vacío o demasiado grande", ErrScriptInvalido)
	}
	dec := json.NewDecoder(bytes.NewReader(crudo))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, fmt.Errorf("%w: manifiesto ilegible", ErrScriptInvalido)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return m, fmt.Errorf("%w: datos tras el manifiesto", ErrScriptInvalido)
	}
	if m.V != ManifiestoVersion {
		return m, fmt.Errorf("%w: versión de manifiesto no soportada", ErrScriptInvalido)
	}
	if err := m.validarListas(); err != nil {
		return m, err
	}
	if m.MaxOps == 0 {
		m.MaxOps = OpsPorDefecto
	}
	if m.MaxMs == 0 {
		m.MaxMs = MsPorDefecto
	}
	if m.MaxOps < 1 || m.MaxOps > TopeOpsScript || m.MaxMs < 100 || m.MaxMs > TopeMsScript {
		return m, fmt.Errorf("%w: límites fuera de rango", ErrScriptInvalido)
	}
	m.Commands = ordenadoSinRepetir(m.Commands)
	m.Read = ordenadoSinRepetir(m.Read)
	m.Scope.Sheets = ordenadoSinRepetir(m.Scope.Sheets)
	m.Scope.Ranges = ordenadoSinRepetir(m.Scope.Ranges)
	return m, nil
}

// validarListas comprueba los nombres de comandos, las lecturas y el ámbito.
func (m ManifiestoScript) validarListas() error {
	if len(m.Commands) > MaxComandosScript {
		return fmt.Errorf("%w: demasiados comandos", ErrScriptInvalido)
	}
	for _, c := range m.Commands {
		if !reComando.MatchString(c) {
			return fmt.Errorf("%w: nombre de comando no válido", ErrScriptInvalido)
		}
		if !ComandoDeScript(c) {
			return fmt.Errorf("%w: el comando «%s» no está permitido en scripts", ErrScriptInvalido, c)
		}
	}
	for _, l := range m.Read {
		if !slices.Contains(lecturasValidas, l) {
			return fmt.Errorf("%w: lectura desconocida", ErrScriptInvalido)
		}
	}
	if len(m.Scope.Sheets) > MaxAmbitoScript || len(m.Scope.Ranges) > MaxAmbitoScript {
		return fmt.Errorf("%w: ámbito demasiado largo", ErrScriptInvalido)
	}
	for _, h := range m.Scope.Sheets {
		if h == "" || len([]rune(h)) > 64 {
			return fmt.Errorf("%w: nombre de hoja no válido", ErrScriptInvalido)
		}
	}
	for _, r := range m.Scope.Ranges {
		if !reRango.MatchString(r) {
			return fmt.Errorf("%w: rango no válido", ErrScriptInvalido)
		}
	}
	return nil
}

func ordenadoSinRepetir(l []string) []string {
	out := slices.Clone(l)
	if out == nil {
		out = []string{}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// HashCodigoScript es el SHA-256 hexadecimal del código: lo que una aprobación avala.
func HashCodigoScript(codigo string) string { return guiones.HashCodigo(codigo) }

// ValidarCodigoScript comprueba lo que comparten crear un script y añadir una versión.
func ValidarCodigoScript(codigo string) error {
	if !guiones.CodigoValido(codigo) {
		return fmt.Errorf("%w: el código está vacío o supera %d bytes", ErrScriptInvalido, MaxCodigoScript)
	}
	return nil
}

// Script es un guion y sus metadatos.
type Script struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Kind       Kind      `json:"kind"`
	Scope      string    `json:"scope"`
	DocumentID string    `json:"document_id,omitempty"`
	AuthorSub  string    `json:"author_sub"`
	CreatedAt  time.Time `json:"created_at"`
	// Última versión y la última aprobada (0 = ninguna), para la lista.
	LatestVersion   int `json:"latest_version"`
	ApprovedVersion int `json:"approved_version"`
}

// VersionScript es una versión inmutable. Code solo se rellena cuando quien la pide
// puede ejecutarla o es su autor.
type VersionScript struct {
	ScriptID   string          `json:"script_id"`
	Version    int             `json:"version"`
	Code       string          `json:"code,omitempty"`
	CodeHash   string          `json:"code_hash"`
	Manifest   json.RawMessage `json:"manifest"`
	Status     string          `json:"status"`
	CreatedBy  string          `json:"created_by"`
	CreatedAt  time.Time       `json:"created_at"`
	ReviewedBy string          `json:"reviewed_by,omitempty"`
	ReviewedAt *time.Time      `json:"reviewed_at,omitempty"`
}

// EjecucionScript es una línea del registro de auditoría. No lleva contenido: ni lo
// leído ni lo escrito, solo quién, qué, dónde, con qué resultado y cuánto costó.
type EjecucionScript struct {
	ID         int64     `json:"id"`
	ScriptID   string    `json:"script_id"`
	Version    int       `json:"version"`
	CodeHash   string    `json:"code_hash"`
	DocumentID string    `json:"document_id"`
	Sub        string    `json:"sub"`
	Outcome    string    `json:"outcome"`
	ErrorCode  string    `json:"error_code,omitempty"`
	DurationMs *int      `json:"duration_ms,omitempty"`
	Ops        int       `json:"ops"`
	StartedAt  time.Time `json:"started_at"`
}

// UsoCuota es lo consumido en la última hora.
type UsoCuota = guiones.UsoCuota

// DecidirCuota dice si cabe una ejecución más que reserva `maxMs`. PURA.
func DecidirCuota(u UsoCuota, maxMs int) error {
	switch guiones.DecidirCuota(u, maxMs) {
	case guiones.CuotaUsuario:
		return fmt.Errorf("%w: demasiadas ejecuciones por hora para esta persona", ErrCuotaScripts)
	case guiones.CuotaOrg:
		return fmt.Errorf("%w: demasiadas ejecuciones por hora en la organización", ErrCuotaScripts)
	case guiones.CuotaTiempo:
		return fmt.Errorf("%w: tiempo de ejecución por hora agotado", ErrCuotaScripts)
	}
	return nil
}

// PuedeEjecutarVersion decide, sin tocar el documento, si `sub` puede ejecutar esa
// versión: el autor siempre la suya (para probarla); los demás solo una versión
// APROBADA de un script de la organización. PURA.
func PuedeEjecutarVersion(s Script, v VersionScript, sub string) error {
	if !guiones.PuedeEjecutar(s.AuthorSub, sub, s.Scope == AmbitoOrg, v.Status) {
		return ErrScriptSinAprobar
	}
	return nil
}

// RolParaEjecutar es el rol mínimo en el documento: editor si el manifiesto escribe, lector si no.
func RolParaEjecutar(m ManifiestoScript) Role {
	if m.Write {
		return RoleEditor
	}
	return RoleViewer
}
