// SPDX-License-Identifier: Apache-2.0
package ajustes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ErrProtocoloTrabajoSesion never authorizes a local fallback for a failed native broker.
var ErrProtocoloTrabajoSesion = errors.New("ajustes: native session work protocol is unavailable or inconsistent")

const limiteTrabajoBroker = 16 << 10

type pedidoTrabajoBroker struct {
	Version    int            `json:"v"`
	Cuenta     string         `json:"account"`
	Acceso     *AccesoSesion  `json:"expected,omitempty"`
	Ambito     *storage.Scope `json:"scope,omitempty"`
	Cabecera   string         `json:"scope_header,omitempty"`
	Contexto   string         `json:"context,omitempty"`
	Identidad  string         `json:"identity_hash,omitempty"`
	Activa     *bool          `json:"active,omitempty"`
	Pausada    *bool          `json:"paused,omitempty"`
	Exclusiva  *bool          `json:"exclusive,omitempty"`
	Operacion  string         `json:"operation,omitempty"`
	Permiso    string         `json:"permit,omitempty"`
	Pendientes *int           `json:"pending,omitempty"`
	Fallo      bool           `json:"failed,omitempty"`
	Desecho    string         `json:"discard,omitempty"`
	Otros      bool           `json:"other_apps,omitempty"`
}

type registroTrabajoBroker struct {
	Pendientes      int    `json:"pending"`
	Incierto        bool   `json:"dirty"`
	Desecho         string `json:"discard,omitempty"`
	DesechoAplicado bool   `json:"discard_applied"`
	TodoConDesecho  bool   `json:"all_discard_requested"`
}

func (r *registroTrabajoBroker) UnmarshalJSON(data []byte) error {
	var wire struct {
		Pendientes      *int   `json:"pending"`
		Incierto        *bool  `json:"dirty"`
		Desecho         string `json:"discard,omitempty"`
		DesechoAplicado *bool  `json:"discard_applied"`
		TodoConDesecho  *bool  `json:"all_discard_requested"`
	}
	if json.Unmarshal(data, &wire) != nil || wire.Pendientes == nil || wire.Incierto == nil || wire.DesechoAplicado == nil || wire.TodoConDesecho == nil {
		return ErrProtocoloTrabajoSesion
	}
	*r = registroTrabajoBroker{Pendientes: *wire.Pendientes, Incierto: *wire.Incierto, Desecho: wire.Desecho, DesechoAplicado: *wire.DesechoAplicado, TodoConDesecho: *wire.TodoConDesecho}
	return nil
}

type respuestaTrabajoBroker struct {
	Version   int                    `json:"v"`
	Cuenta    string                 `json:"account"`
	Accion    string                 `json:"action"`
	Acceso    json.RawMessage        `json:"access"`
	Ambito    *storage.Scope         `json:"scope,omitempty"`
	Cabecera  string                 `json:"scope_header,omitempty"`
	Operacion string                 `json:"operation,omitempty"`
	Permiso   string                 `json:"permit,omitempty"`
	Trabajo   *registroTrabajoBroker `json:"work,omitempty"`
}

type resultadoTrabajoBroker struct {
	Acceso    AccesoSesion
	Operacion string
	Permiso   string
	Trabajo   *registroTrabajoBroker
}

// This client carries only opaque witnesses and local work metadata. Its Android
// transport authenticates the elected broker; errors never authorize a file fallback.
type clienteTrabajoBroker struct {
	disponible func() (bool, error)
	pedir      func(context.Context, string, string) (string, error)
}

func accesoTrabajoValido(access AccesoSesion) bool {
	return access.Version == 1 && revisionDrive.MatchString(access.Revision) &&
		((access.Activo && hashDrive.MatchString(access.Identidad)) || (!access.Activo && access.Identidad == "")) &&
		(!access.Suspendido || access.Activo)
}

func requiereAmbitoTrabajo(accion string) bool {
	switch accion {
	case "iniciar", "terminar", "reconciliar", "confirmar-desecho":
		return true
	default:
		return false
	}
}

func (p pedidoTrabajoBroker) validar(accion string) error {
	if p.Version != 1 || !almacenSesionValido.MatchString(p.Cuenta) || (p.Ambito != nil && !p.Ambito.Valid()) ||
		(requiereAmbitoTrabajo(accion) && p.Ambito == nil) {
		return ErrProtocoloTrabajoSesion
	}
	if accion != "leer" && accion != "publicar" && (p.Acceso == nil || !accesoTrabajoValido(*p.Acceso) || !p.Acceso.Activo) {
		return ErrProtocoloTrabajoSesion
	}
	if p.Contexto != "" && accion != "publicar" {
		return ErrProtocoloTrabajoSesion
	}
	if p.Pendientes != nil && (*p.Pendientes < 0 || *p.Pendientes > 1000000) {
		return ErrProtocoloTrabajoSesion
	}
	return p.validarAccion(accion)
}

func (p pedidoTrabajoBroker) validarAccion(accion string) error {
	if p.Otros && accion != "resumir" {
		return ErrProtocoloTrabajoSesion
	}
	switch accion {
	case "leer":
		if p.Ambito != nil || p.Acceso != nil {
			return ErrProtocoloTrabajoSesion
		}
	case "publicar":
		return p.validarPublicacion()
	case "pausar":
		if p.Pausada == nil || p.Ambito != nil {
			return ErrProtocoloTrabajoSesion
		}
	case "cerrojo", "confirmar":
		if p.Exclusiva == nil || p.Ambito != nil {
			return ErrProtocoloTrabajoSesion
		}
	case "soltar":
		if !revisionDrive.MatchString(p.Permiso) || p.Ambito != nil {
			return ErrProtocoloTrabajoSesion
		}
	default:
		return p.validarDiario(accion)
	}
	return nil
}

func (p pedidoTrabajoBroker) validarPublicacion() error {
	if p.Activa == nil || p.Ambito != nil || len(p.Contexto) > 8192 {
		return ErrProtocoloTrabajoSesion
	}
	if *p.Activa {
		if !hashDrive.MatchString(p.Identidad) || p.Contexto == "" {
			return ErrProtocoloTrabajoSesion
		}
	} else if p.Identidad != "" || p.Contexto != "" {
		return ErrProtocoloTrabajoSesion
	}
	return nil
}

func (p pedidoTrabajoBroker) validarDiario(accion string) error {
	switch accion {
	case "iniciar", "resumir":
	case "terminar":
		if !revisionDrive.MatchString(p.Operacion) || (!p.Fallo && p.Pendientes == nil) {
			return ErrProtocoloTrabajoSesion
		}
	case "reconciliar":
		if p.Pendientes == nil || !revisionDrive.MatchString(p.Permiso) {
			return ErrProtocoloTrabajoSesion
		}
	case "desechar":
		if !revisionDrive.MatchString(p.Permiso) {
			return ErrProtocoloTrabajoSesion
		}
	case "confirmar-desecho":
		if !revisionDrive.MatchString(p.Permiso) || !revisionDrive.MatchString(p.Desecho) || (!p.Fallo && p.Pendientes == nil) {
			return ErrProtocoloTrabajoSesion
		}
	default:
		return ErrProtocoloTrabajoSesion
	}
	return nil
}

func (p pedidoTrabajoBroker) codificar(accion string) (string, pedidoTrabajoBroker, error) {
	if err := p.validar(accion); err != nil {
		return "", p, err
	}
	if p.Ambito == nil {
		if p.Cabecera != "" {
			return "", p, ErrProtocoloTrabajoSesion
		}
	} else {
		cabecera, err := storage.EncodeScopeHeader(*p.Ambito)
		if err != nil || (p.Cabecera != "" && p.Cabecera != cabecera) {
			return "", p, ErrProtocoloTrabajoSesion
		}
		p.Cabecera = cabecera
	}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > limiteTrabajoBroker {
		return "", p, ErrProtocoloTrabajoSesion
	}
	return string(raw), p, nil
}

func decodificarAccesoTrabajo(raw json.RawMessage) (AccesoSesion, error) {
	var wire struct {
		Version    int    `json:"version"`
		Identidad  string `json:"identity_hash"`
		Revision   string `json:"revision"`
		Activo     *bool  `json:"active"`
		Suspendido bool   `json:"paused"`
	}
	if len(raw) > 1024 || json.Unmarshal(raw, &wire) != nil || wire.Activo == nil {
		return AccesoSesion{}, ErrProtocoloTrabajoSesion
	}
	access := AccesoSesion{Version: wire.Version, Identidad: wire.Identidad, Revision: wire.Revision, Activo: *wire.Activo, Suspendido: wire.Suspendido}
	if !accesoTrabajoValido(access) {
		return AccesoSesion{}, ErrProtocoloTrabajoSesion
	}
	return access, nil
}

func (c clienteTrabajoBroker) aplicar(ctx context.Context, accion string, p pedidoTrabajoBroker) (resultadoTrabajoBroker, bool, error) {
	if err := ctx.Err(); err != nil {
		return resultadoTrabajoBroker{}, true, err
	}
	encoded, fijado, err := p.codificar(accion)
	if err != nil {
		return resultadoTrabajoBroker{}, true, err
	}
	p = fijado
	if c.disponible == nil || c.pedir == nil {
		return resultadoTrabajoBroker{}, true, ErrProtocoloTrabajoSesion
	}
	disponible, err := c.disponible()
	if err != nil {
		return resultadoTrabajoBroker{}, true, errors.Join(ErrProtocoloTrabajoSesion, err)
	}
	if !disponible {
		return resultadoTrabajoBroker{}, false, nil
	} // Only a known old protocol has this result.
	if err := ctx.Err(); err != nil {
		return resultadoTrabajoBroker{}, true, err
	}
	text, err := c.pedir(ctx, accion, encoded)
	if err != nil {
		return resultadoTrabajoBroker{}, true, errors.Join(ErrProtocoloTrabajoSesion, err)
	}
	if err := ctx.Err(); err != nil {
		return resultadoTrabajoBroker{}, true, err
	}
	if len(text) > limiteTrabajoBroker {
		return resultadoTrabajoBroker{}, true, ErrProtocoloTrabajoSesion
	}
	response, err := decodificarRespuestaTrabajo(text, accion, p)
	if err != nil {
		return resultadoTrabajoBroker{}, true, err
	}
	result, err := response.validar(accion, p)
	return result, true, err
}

func decodificarRespuestaTrabajo(text, accion string, p pedidoTrabajoBroker) (respuestaTrabajoBroker, error) {
	var response respuestaTrabajoBroker
	decoder := json.NewDecoder(bytes.NewBufferString(text))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&response) != nil || decoder.Decode(new(any)) != io.EOF || response.Version != 1 || response.Cuenta != p.Cuenta || response.Accion != accion || response.Cabecera != p.Cabecera ||
		(response.Ambito == nil) != (p.Ambito == nil) || (p.Ambito != nil && *response.Ambito != *p.Ambito) {
		return respuestaTrabajoBroker{}, ErrProtocoloTrabajoSesion
	}
	return response, nil
}

func (response respuestaTrabajoBroker) validar(accion string, p pedidoTrabajoBroker) (resultadoTrabajoBroker, error) {
	access, err := decodificarAccesoTrabajo(response.Acceso)
	if err != nil {
		return resultadoTrabajoBroker{}, err
	}
	if accion != "soltar" && p.Acceso != nil && !mismaSesion(access, *p.Acceso) {
		return resultadoTrabajoBroker{}, ErrAccesoSesion
	}
	if err := response.validarAccion(accion, p, access); err != nil {
		return resultadoTrabajoBroker{}, err
	}
	if err := response.validarDiario(accion, p); err != nil {
		return resultadoTrabajoBroker{}, err
	}
	return resultadoTrabajoBroker{Acceso: access, Operacion: response.Operacion, Permiso: response.Permiso, Trabajo: response.Trabajo}, nil
}

func (response respuestaTrabajoBroker) validarAccion(accion string, p pedidoTrabajoBroker, access AccesoSesion) error {
	switch accion {
	case "publicar":
		if access.Activo != *p.Activa || access.Identidad != p.Identidad {
			return ErrAccesoSesion
		}
	case "pausar":
		if access.Suspendido != *p.Pausada {
			return ErrAccesoSesion
		}
	case "confirmar":
		if !*p.Exclusiva && access.Suspendido {
			return ErrAccesoSesion
		}
		if *p.Exclusiva && !revisionDrive.MatchString(response.Permiso) {
			return ErrProtocoloTrabajoSesion
		}
	case "iniciar":
		if access.Suspendido {
			return ErrAccesoSesion
		}
		if !revisionDrive.MatchString(response.Operacion) {
			return ErrProtocoloTrabajoSesion
		}
	}
	return nil
}

func (response respuestaTrabajoBroker) validarDiario(accion string, p pedidoTrabajoBroker) error {
	switch accion {
	case "iniciar", "terminar", "reconciliar", "resumir", "desechar", "confirmar-desecho":
		if response.Trabajo == nil {
			return ErrProtocoloTrabajoSesion
		}
		return response.Trabajo.validar(accion, p)
	default:
		return nil
	}
}

func (work registroTrabajoBroker) validar(accion string, p pedidoTrabajoBroker) error {
	if work.Pendientes < 0 || work.Pendientes > 1000000 || (work.Desecho != "" && !revisionDrive.MatchString(work.Desecho)) ||
		(work.DesechoAplicado && work.Desecho == "") {
		return ErrProtocoloTrabajoSesion
	}
	if (accion == "iniciar" || p.Fallo) && !work.Incierto {
		return ErrProtocoloTrabajoSesion
	}
	if accion == "reconciliar" && (work.Pendientes != *p.Pendientes || work.Incierto) {
		return ErrProtocoloTrabajoSesion
	}
	if accion == "confirmar-desecho" && !p.Fallo && (work.Desecho != p.Desecho || !work.DesechoAplicado || work.Pendientes != 0 || work.Incierto) {
		return ErrProtocoloTrabajoSesion
	}
	return nil
}

// cerrojo opens a fresh description, waits in the kernel outside Binder, then
// confirms the original witness before permitting application IO.
func (c clienteTrabajoBroker) cerrojo(ctx context.Context, p pedidoTrabajoBroker, abrir func(context.Context, string) (*os.File, error)) (func(), bool, error) {
	lease, native, err := c.arrendar(ctx, p, abrir)
	return lease.soltar, native, err
}

type arrendamientoTrabajoBroker struct {
	soltar  func()
	permiso string
}

func (c clienteTrabajoBroker) arrendar(ctx context.Context, p pedidoTrabajoBroker, abrir func(context.Context, string) (*os.File, error)) (arrendamientoTrabajoBroker, bool, error) {
	if err := ctx.Err(); err != nil {
		return arrendamientoTrabajoBroker{}, true, err
	}
	encoded, fixed, err := p.codificar("cerrojo")
	if err != nil {
		return arrendamientoTrabajoBroker{}, true, err
	}
	p = fixed
	if c.disponible == nil || c.pedir == nil || abrir == nil {
		return arrendamientoTrabajoBroker{}, true, ErrProtocoloTrabajoSesion
	}
	available, err := c.disponible()
	if err != nil {
		return arrendamientoTrabajoBroker{}, true, errors.Join(ErrProtocoloTrabajoSesion, err)
	}
	if !available {
		return arrendamientoTrabajoBroker{}, false, nil
	}
	file, err := abrir(ctx, encoded)
	if err != nil {
		return arrendamientoTrabajoBroker{}, true, err
	}
	if file == nil {
		return arrendamientoTrabajoBroker{}, true, ErrProtocoloTrabajoSesion
	}
	release, err := tomarDescriptorTrabajo(ctx, file, *p.Exclusiva)
	if err != nil {
		return arrendamientoTrabajoBroker{}, true, err
	}
	result, native, err := c.aplicar(ctx, "confirmar", p)
	if err != nil || !native {
		release()
		return arrendamientoTrabajoBroker{}, true, errors.Join(ErrProtocoloTrabajoSesion, err)
	}
	return arrendamientoTrabajoBroker{soltar: c.soltarCerrojo(ctx, p, result.Permiso, release), permiso: result.Permiso}, true, nil
}

func tomarDescriptorTrabajo(ctx context.Context, file *os.File, exclusive bool) (func(), error) {
	if exclusive {
		return CerrojoFichero(ctx, file)
	}
	return CerrojoFicheroCompartido(ctx, file)
}

func (c clienteTrabajoBroker) soltarCerrojo(ctx context.Context, p pedidoTrabajoBroker, permit string, release func()) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			if *p.Exclusiva {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
				p.Permiso = permit
				// Failure cannot erase a dirty record; the kernel lease must still close.
				_, _, _ = c.aplicar(cleanup, "soltar", p)
				cancel()
			}
			release()
		})
	}
}
