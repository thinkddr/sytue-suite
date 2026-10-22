// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/guiones"
)

// # Scripts de usuario (bases-2, automatización; ADR 0069)
//
// Un script es un guion que cuelga de UNA base. El servidor NO ejecuta su código: corre
// aislado en el navegador de quien lo lanza (el mismo runtime que Docs, `@sytue/scripts`)
// y cada cosa que hace es una petición corriente a esta API con la sesión de esa persona.
// Por eso un script nunca puede más que quien lo ejecuta: no hay camino propio, ni
// credencial de script, ni operación que no pase por `autorizar`.
//
// Lo que sí decide el servidor: qué versiones existen (inmutables), cuál puede ejecutar
// cada persona (la autora la suya; los demás, una APROBADA de un script compartido), con
// qué cuota, y deja el registro de cada ejecución.

// Límites de los scripts de una base.
const (
	MaxScriptsPorBase    = 100
	MaxVersionesDeScript = 200
	MaxTablasDeScript    = MaxTablasPorBase
	// Cada operación de un script es una petición a la API (una transacción): el tope por
	// ejecución es más bajo que el de Docs, donde una operación es un comando del editor.
	TopeOpsScript    = 500
	OpsPorDefecto    = 100
	MaxPaginaRuns    = 200
	maxManifiesto    = 8 * 1024
	ScriptPrivado    = "private" // solo su autora
	ScriptCompartido = "shared"  // lo ejecutan quienes acceden a la base, si la versión está aprobada
)

// OperacionesDeScript es la superficie `sytue.v1` de Bases: la lista CERRADA de lo que un
// script puede pedir, y si escribe. Es corta a propósito: registros y lo necesario para
// encontrarlos. Fuera quedan el esquema, los permisos, compartir, conectores, formularios,
// adjuntos, comentarios, exportar, importar, la auditoría y los propios scripts.
// El cliente tiene su espejo (`apps/frontend/apps/bases/src/lib/scripts/superficie.ts`).
var OperacionesDeScript = map[string]bool{
	"tables.list":    false,
	"fields.list":    false,
	"views.list":     false,
	"records.list":   false,
	"records.get":    false,
	"records.create": true,
	"records.update": true,
	"records.delete": true,
}

// Errores propios de los scripts. Los demás son los del dominio (ErrNoExiste, ErrSinPermiso…).
var (
	ErrScriptSinAprobar = errors.New("bases: la versión no está aprobada")                     // 403
	ErrCuotaScripts     = errors.New("bases: cuota de ejecuciones agotada")                    // 429
	ErrEstadoScript     = errors.New("bases: la versión no admite esa transición")             // 409
	ErrHashNoCoincide   = errors.New("bases: el hash no corresponde al código de esa versión") // 409
)

// ManifiestoScript es lo que el script declara necesitar. La persona lo ve antes de la
// primera ejecución y cualquier cambio pide el consentimiento de nuevo.
type ManifiestoScript struct {
	V int `json:"v"`
	// Ops son las operaciones de OperacionesDeScript que puede pedir.
	Ops []string `json:"ops"`
	// Tables acota a esas tablas de la base (por id). Vacío = todas las que la persona lea.
	Tables []string `json:"tables"`
	// Write: si puede crear, editar o borrar registros.
	Write  bool `json:"write"`
	MaxOps int  `json:"max_ops"`
	MaxMs  int  `json:"max_ms"`
}

// ParseManifiesto decodifica y valida un manifiesto (estricto: campos desconocidos y
// datos sobrantes son error) y lo devuelve normalizado: listas sin repetidos y ordenadas,
// cotas por defecto aplicadas. El JSON canónico (json.Marshal del resultado) es el que se
// guarda, se enseña y se ejecuta.
func ParseManifiesto(crudo []byte) (ManifiestoScript, error) {
	var m ManifiestoScript
	mal := func(que string) (ManifiestoScript, error) {
		return ManifiestoScript{}, fmt.Errorf("%w: manifiesto: %s", ErrInvalido, que)
	}
	if len(crudo) == 0 || len(crudo) > maxManifiesto {
		return mal("vacío o demasiado grande")
	}
	dec := json.NewDecoder(bytes.NewReader(crudo))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return mal("ilegible")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return mal("datos de más")
	}
	if m.V != 1 {
		return mal("versión no soportada")
	}
	if len(m.Ops) > 2*len(OperacionesDeScript) || len(m.Tables) > MaxTablasDeScript {
		return mal("listas demasiado largas")
	}
	for _, op := range m.Ops {
		escribe, existe := OperacionesDeScript[op]
		if !existe {
			return mal("operación desconocida")
		}
		// Una operación que escribe sin declarar escritura es un manifiesto que dice dos
		// cosas: se rechaza entero en vez de adivinar cuál vale.
		if escribe && !m.Write {
			return mal("declara una operación que escribe sin declarar escritura")
		}
	}
	for i, t := range m.Tables {
		u, err := uuid.Parse(t)
		if err != nil {
			return mal("tabla no válida")
		}
		m.Tables[i] = u.String()
	}
	if m.MaxOps == 0 {
		m.MaxOps = OpsPorDefecto
	}
	if m.MaxMs == 0 {
		m.MaxMs = guiones.MsPorDefecto
	}
	if m.MaxOps < 1 || m.MaxOps > TopeOpsScript || m.MaxMs < 100 || m.MaxMs > guiones.TopeMs {
		return mal("límites fuera de rango")
	}
	m.Ops = ordenadoSinRepetir(m.Ops)
	m.Tables = ordenadoSinRepetir(m.Tables)
	return m, nil
}

func ordenadoSinRepetir(l []string) []string {
	out := slices.Clone(l)
	if out == nil {
		out = []string{}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// AccionParaEjecutar es lo que quien ejecuta ha de poder hacer en cada tabla del
// manifiesto (o en la base, si no acota): escribir registros si el script escribe, leer si no.
func AccionParaEjecutar(m ManifiestoScript) Accion {
	if m.Write {
		return AccEscribirReg
	}
	return AccLeer
}

// VeScript decide si alguien con acceso a la base ve un script: el privado, solo su
// autora (tampoco la propietaria de la base); el compartido, su autora, quien lo revisa
// y, los demás, cuando ya tiene una versión aprobada.
func VeScript(s Script, sub string, revisa bool) bool {
	if s.AuthorSub == sub {
		return true
	}
	return s.Scope == ScriptCompartido && (revisa || s.ApprovedVersion > 0)
}

// Script es un guion de una base y sus metadatos.
type Script struct {
	ID        string    `json:"id"`
	BaseID    string    `json:"base_id"`
	Name      string    `json:"name"`
	Scope     string    `json:"scope"`
	AuthorSub string    `json:"author_sub"`
	CreatedAt time.Time `json:"created_at"`
	// Última versión y la última aprobada (0 = ninguna).
	LatestVersion   int `json:"latest_version"`
	ApprovedVersion int `json:"approved_version"`
}

// VersionScript es una versión inmutable: código, su hash y el manifiesto canónico.
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

// EjecucionScript es una línea del registro. No lleva contenido: ni lo leído ni lo
// escrito, solo quién, qué versión (con su hash), con qué resultado y cuánto costó.
type EjecucionScript struct {
	ID         int64     `json:"id"`
	ScriptID   string    `json:"script_id"`
	Version    int       `json:"version"`
	CodeHash   string    `json:"code_hash"`
	Sub        string    `json:"sub"`
	Outcome    string    `json:"outcome"`
	ErrorCode  string    `json:"error_code,omitempty"`
	DurationMs *int      `json:"duration_ms,omitempty"`
	Ops        int       `json:"ops"`
	StartedAt  time.Time `json:"started_at"`
}

// EjecucionAutorizada es lo que se devuelve al autorizar: el cliente ejecuta EXACTAMENTE
// este código con este manifiesto y cierra la ejecución con su id.
type EjecucionAutorizada struct {
	RunID    int64           `json:"run_id"`
	Code     string          `json:"code"`
	CodeHash string          `json:"code_hash"`
	Manifest json.RawMessage `json:"manifest"`
}
