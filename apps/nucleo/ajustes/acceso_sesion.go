// SPDX-License-Identifier: Apache-2.0
package ajustes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// AccesoSesion contains only opaque identity and incarnation witnesses. Refresh
// preserves the incarnation; account changes and logout invalidate A→B→A work.
// Suspendido blocks new work while the owner drains other native app processes.
type AccesoSesion struct {
	Version    int    `json:"version"`
	Identidad  string `json:"identity_hash"`
	Revision   string `json:"revision"`
	Activo     bool   `json:"active"`
	Suspendido bool   `json:"paused,omitempty"`
}

var (
	hashDrive                        = regexp.MustCompile(`^[0-9a-f]{64}$`)
	revisionDrive                    = regexp.MustCompile(`^[0-9a-f]{32}$`)
	almacenSesionValido              = regexp.MustCompile(`^sesion-[0-9a-f]{32}$`)
	ErrTrabajoCompartidoNoDisponible = errors.New("ajustes: shared private session work requires the native session broker or app group")
	ErrAccesoSesion                  = errors.New("ajustes: session work witness is no longer current")
)

func leerAcceso(ruta string) (AccesoSesion, error) {
	data, err := os.ReadFile(ruta)
	if err != nil {
		return AccesoSesion{}, err
	}
	var access AccesoSesion
	if len(data) > 1024 || json.Unmarshal(data, &access) != nil || access.Version != 1 || !revisionDrive.MatchString(access.Revision) || (access.Activo && !hashDrive.MatchString(access.Identidad)) {
		return AccesoSesion{}, ErrAccesoSesion
	}
	return access, nil
}

func escribirAcceso(ruta, identidad string, activo bool, escribir func(string, []byte) error, quitar func(string) error) error {
	if activo && !hashDrive.MatchString(identidad) {
		return ErrAccesoSesion
	}
	previous, _ := leerAcceso(ruta)
	if previous.Version == 1 && previous.Activo == activo && previous.Identidad == identidad {
		return nil
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	access := AccesoSesion{Version: 1, Identidad: identidad, Revision: hex.EncodeToString(nonce), Activo: activo}
	data, err := json.Marshal(access)
	if err != nil {
		return err
	}
	if err := escribir(ruta, data); err != nil {
		removeErr := quitar(ruta)
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		return errors.Join(err, removeErr)
	}
	return nil
}

func rutaAccesoSesion(almacen string) (string, error) {
	if !almacenSesionValido.MatchString(almacen) {
		return "", ErrAccesoSesion
	}
	dir, err := Carpeta()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, almacen+".acceso.json"), nil
}

// LeerAccesoSesion also serves context-free UI current checks. The native read
// has its own short deadline; operations use LeerAccesoSesionCon.
//
//nolint:contextcheck // Context-free native UI callbacks require a bounded metadata snapshot.
func LeerAccesoSesion(almacen string) (AccesoSesion, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return LeerAccesoSesionCon(ctx, almacen)
}

// LeerAccesoSesionCon preserves a caller deadline through the native broker.
func LeerAccesoSesionCon(ctx context.Context, almacen string) (AccesoSesion, error) {
	if err := ctx.Err(); err != nil {
		return AccesoSesion{}, err
	}
	if client, native := clienteTrabajoPlataforma(); native {
		result, handled, err := client.aplicar(ctx, "leer", pedidoTrabajoBroker{Version: 1, Cuenta: almacen})
		if handled {
			return result.Acceso, err
		}
	}
	route, err := rutaAccesoSesion(almacen)
	if err != nil {
		return AccesoSesion{}, err
	}
	return leerAcceso(route)
}

func conAccesoSesion(ctx context.Context, almacen string, fn func(string) error) error {
	route, err := rutaAccesoSesion(almacen)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(route+".cerrojo", os.O_CREATE|os.O_RDWR, permisoFichero)
	if err != nil {
		return err
	}
	release, err := CerrojoFichero(ctx, file)
	if err != nil {
		return err
	}
	defer release()
	return fn(route)
}

func PonerAccesoSesion(almacen, identidad string, activo bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	return PonerAccesoSesionCon(ctx, almacen, identidad, activo)
}

func PonerAccesoSesionCon(ctx context.Context, almacen, identidad string, activo bool) error {
	return PonerAccesoSesionContextoCon(ctx, almacen, identidad, "", activo)
}

// PonerAccesoSesionContextoCon verifies an active Android context against the
// broker vault. The raw tuple is transient; native files contain only its hash.
func PonerAccesoSesionContextoCon(ctx context.Context, almacen, identidad, contexto string, activo bool) error {
	if client, native := clienteTrabajoPlataforma(); native {
		_, handled, err := client.aplicar(ctx, "publicar", pedidoTrabajoBroker{Version: 1, Cuenta: almacen, Identidad: identidad, Contexto: contexto, Activa: &activo})
		if handled {
			return err
		}
	}
	return conAccesoSesion(ctx, almacen, func(route string) error { return escribirAcceso(route, identidad, activo, EscribirEntero, os.Remove) })
}

func mismaSesion(actual, esperado AccesoSesion) bool {
	return actual.Version == 1 && actual.Activo && actual.Identidad == esperado.Identidad && actual.Revision == esperado.Revision
}

// SuspenderAccesoSesion changes admission without rotating the witness. A token
// refresh cannot clear a pause, and an old logout cannot pause a newer login.
func SuspenderAccesoSesion(ctx context.Context, almacen string, esperado AccesoSesion, suspender bool) error {
	if client, native := clienteTrabajoPlataforma(); native {
		_, handled, err := client.aplicar(ctx, "pausar", pedidoTrabajoBroker{Version: 1, Cuenta: almacen, Acceso: &esperado, Pausada: &suspender})
		if handled {
			return err
		}
	}
	return conAccesoSesion(ctx, almacen, func(route string) error {
		actual, err := leerAcceso(route)
		if err != nil {
			return err
		}
		if !mismaSesion(actual, esperado) {
			return ErrAccesoSesion
		}
		actual.Suspendido = suspender
		data, err := json.Marshal(actual)
		if err != nil {
			return err
		}
		return EscribirEntero(route, data)
	})
}

// CerrojoTrabajoSesion shares a lease between the products of exactly this
// identity. It checks the witness AFTER acquiring the lease, before any data IO.
// Other identities and OIDC refresh do not wait for this network lifetime.
func CerrojoTrabajoSesion(ctx context.Context, almacen string, esperado AccesoSesion, exclusivo bool) (func(), error) {
	available, err := EstadoTrabajoSesionCompartido()
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, ErrTrabajoCompartidoNoDisponible
	}
	if client, native := clienteTrabajoPlataforma(); native {
		release, handled, err := client.cerrojo(ctx, pedidoTrabajoBroker{Version: 1, Cuenta: almacen, Acceso: &esperado, Exclusiva: &exclusivo}, descriptorTrabajoPlataforma)
		if handled {
			return release, err
		}
		return nil, ErrTrabajoCompartidoNoDisponible
	}
	route, err := rutaAccesoSesion(almacen)
	if err != nil {
		return nil, err
	}
	if !esperado.Activo || !hashDrive.MatchString(esperado.Identidad) || !revisionDrive.MatchString(esperado.Revision) {
		return nil, ErrAccesoSesion
	}
	file, err := os.OpenFile(route+"."+esperado.Identidad+".trabajo", os.O_CREATE|os.O_RDWR, permisoFichero)
	if err != nil {
		return nil, err
	}
	var release func()
	if exclusivo {
		release, err = CerrojoFichero(ctx, file)
	} else {
		release, err = CerrojoFicheroCompartido(ctx, file)
	}
	if err != nil {
		return nil, err
	}
	actual, err := LeerAccesoSesionCon(ctx, almacen)
	if err != nil || !mismaSesion(actual, esperado) || (!exclusivo && actual.Suspendido) {
		release()
		return nil, errors.Join(ErrAccesoSesion, err)
	}
	return release, nil
}
