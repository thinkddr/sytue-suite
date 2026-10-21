// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
)

// errNoAsumible marca «este rol no sirve aquí» sin confundirlo con un fallo de la
// base: lo primero es una decisión de configuración y lo segundo hay que
// propagarlo. Es interno; nadie fuera necesita distinguirlo.
var errNoAsumible = errors.New("pg: el rol de aplicación no se puede usar")

func isNoAsumible(err error) bool { return errors.Is(err, errNoAsumible) }

// AppRole decide, UNA vez y con pruebas, cómo se garantiza el aislamiento por
// organización en un servicio que quiere correr sus transacciones bajo un rol de
// aplicación.
//
// EL PROBLEMA QUE RESUELVE. `SET LOCAL ROLE <servicio>_app` es defensa en
// profundidad: las políticas de RLS no se aplican a un superusuario ni a un rol
// con BYPASSRLS, así que asumir un rol que no es ninguna de las dos cosas hace
// que se apliquen SIEMPRE, con independencia de cómo se haya conectado el pool.
// Es correcto y barato… mientras el rol se pueda asumir.
//
// En producción no se podía, por dos motivos encadenados que solo existen fuera
// de dev:
//
//   - Desde PostgreSQL 16 la membresía de un rol lleva la opción SET aparte, y
//     sin ella ser miembro no autoriza a asumirlo.
//   - El esquema `public` de una base gestionada pertenece al superusuario del
//     proveedor, no al usuario de la suite, así que este NO PUEDE conceder USAGE
//     sobre él a un tercer rol. `GRANT USAGE ON SCHEMA public TO x` responde
//     «no privileges were granted» y sigue adelante. Sin USAGE, PostgreSQL dice
//     que las tablas «no existen» —no que falte permiso—, que es el error más
//     desorientador posible.
//
// Resultado: ninguna consulta de negocio se ejecutaba, con el servicio arrancado
// y respondiendo a las sondas. En Sytue Integrations el catálogo se sembró con
// cero de 32 plantillas; en Sytue Docs no se podía crear un documento.
//
// LA SALIDA, Y POR QUÉ ES SEGURA. Si el rol de la conexión NO es superusuario y
// NO tiene BYPASSRLS, y todas las tablas con RLS la llevan FORZADA —FORCE ROW
// LEVEL SECURITY aplica las políticas también al DUEÑO de la tabla—, entonces el
// aislamiento ya está garantizado sin asumir ningún rol. En ese caso, y solo en
// ese, se prescinde del SET ROLE.
//
// Si no se cumple, esto DEVUELVE ERROR y el servicio no arranca. Degradar en
// silencio aquí significaría servir peticiones de un cliente con las políticas
// desactivadas, que es exactamente el fallo que estas dos capas existen para
// impedir.
type AppRole struct {
	nombre string
	// usar es atómico porque lo leen las transacciones desde varias goroutines
	// mientras Resolve puede estar escribiéndolo en el arranque.
	usar atomic.Bool
}

// NewAppRole declara el rol que el servicio querría asumir.
func NewAppRole(nombre string) *AppRole { return &AppRole{nombre: nombre} }

// Nombre del rol de aplicación.
func (a *AppRole) Nombre() string { return a.nombre }

// Activo indica si las transacciones deben asumir el rol. Solo tiene sentido tras
// Resolve; antes devuelve false, que es el lado seguro para el arranque: mejor no
// asumirlo que asumirlo sin haber comprobado que se puede.
//
// Tolera el receptor NIL a propósito, y esto no es defensa por si acaso: un Store
// construido sin su AppRole —un constructor que se quedó sin actualizar— hacía
// que el servicio ARRANCARA y muriera con un nil pointer dentro de la primera
// transacción, con el panic apuntando a este paquete y no al constructor que lo
// causó. Pasó en Sytue Docs. Nil significa «no hay rol que asumir», que es
// exactamente lo que ocurre, y el aislamiento no depende de esto: lo sostiene la
// RLS forzada que Resolve verifica.
func (a *AppRole) Activo() bool { return a != nil && a.usar.Load() }

// Resolve decide el modo y verifica que el aislamiento se sostiene. Debe llamarse
// tras aplicar las migraciones y antes de servir tráfico.
//
// Devuelve el modo elegido en prosa para que quede en el log del arranque: qué
// capa está sujetando el aislamiento no puede ser algo que haya que deducir
// leyendo el código seis meses después.
func (a *AppRole) Resolve(ctx context.Context, p *Pool) (string, error) {
	// Receptor nil = el Store se construyó sin AppRole. Es un error de
	// programación, no de configuración, y tiene que decirlo con esas palabras:
	// el panic que producía en su lugar apuntaba a este paquete y no al
	// constructor culpable, y costó un despliegue averiguarlo.
	if a == nil {
		return "", fmt.Errorf("pg: el Store se construyó sin AppRole " +
			"(falta pg.NewAppRole(appRole) en su constructor)")
	}
	puede, err := a.puedeAsumirse(ctx, p)
	if err != nil {
		return "", err
	}
	if puede {
		a.usar.Store(true)
		return fmt.Sprintf("rol de aplicación %s activo (RLS + rol dedicado)", a.nombre), nil
	}

	conexion, err := RolActual(ctx, p)
	if err != nil {
		return "", fmt.Errorf("pg: comprobar el rol de conexión: %w", err)
	}
	if conexion.EludeRLS() {
		return "", fmt.Errorf(
			"pg: no se puede asumir %s y el rol de conexión elude RLS (superusuario=%t bypassrls=%t): "+
				"sin una de las dos cosas no hay aislamiento por organización",
			a.nombre, conexion.Superusuario, conexion.BypassRLS,
		)
	}

	// La otra mitad de la garantía: una tabla con RLS pero SIN forzarla no aplica
	// sus políticas al dueño, y el dueño de estas tablas es justo el usuario con
	// el que se conecta el servicio. Con una sola así, prescindir del rol abriría
	// un agujero en esa tabla.
	//
	// Entran las ordinarias ('r') y las PARTICIONADAS ('p'), igual que el
	// invariante de pgtest.ExigirRLSForzada: `analytics_events` —la tabla más
	// caliente de la plataforma— es particionada, y filtrando por 'r' el catálogo
	// no la devolvía, así que el guard de arranque era más flojo que el de test
	// justo donde más da. Las particiones se excluyen (`relispartition`): la RLS
	// que se evalúa es la de la relación NOMBRADA en la consulta.
	var sinForce int
	if err := p.QueryRow(ctx, `
		SELECT count(*) FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('r', 'p') AND NOT c.relispartition
		  AND n.nspname = 'public'
		  AND c.relrowsecurity AND NOT c.relforcerowsecurity`).Scan(&sinForce); err != nil {
		return "", fmt.Errorf("pg: comprobar RLS forzada: %w", err)
	}
	if sinForce > 0 {
		return "", fmt.Errorf(
			"pg: no se puede asumir %s y hay %d tabla(s) con RLS sin FORCE: "+
				"sus políticas no se aplicarían al dueño, que es el usuario de la conexión",
			a.nombre, sinForce,
		)
	}

	a.usar.Store(false)
	return fmt.Sprintf(
		"rol de aplicación %s NO asumible; aislamiento sostenido por RLS forzada "+
			"sobre un rol sin superusuario ni BYPASSRLS", a.nombre,
	), nil
}

// puedeAsumirse prueba el SET ROLE de verdad en lugar de deducirlo del catálogo:
// entre la membresía, su opción SET y el USAGE sobre el esquema hay tres formas
// distintas de que falle, y solo intentarlo las cubre todas.
func (a *AppRole) puedeAsumirse(ctx context.Context, p *Pool) (bool, error) {
	err := p.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+a.nombre); err != nil {
			return errNoAsumible
		}
		// Que se pueda asumir no basta: hay que mirar QUÉ se asume. Un rol de
		// aplicación creado (o alterado después) con SUPERUSER o BYPASSRLS se asume
		// sin problema y deja las políticas apagadas, que es justo lo contrario de
		// para lo que existe. Esto no es «no asumible» —no hay modo alternativo que
		// lo arregle—, es un error de aprovisionamiento y se propaga como tal.
		asumido, err := RolActual(ctx, tx)
		if err != nil {
			return fmt.Errorf("leer los atributos de %s: %w", a.nombre, err)
		}
		if asumido.EludeRLS() {
			return fmt.Errorf(
				"el rol de aplicación %s elude RLS (superusuario=%t bypassrls=%t): asumirlo no aísla nada",
				a.nombre, asumido.Superusuario, asumido.BypassRLS,
			)
		}
		// Asumirlo no basta: sin USAGE sobre el esquema, el rol se asume y luego
		// no ve NINGUNA tabla. Es el caso que se dio en producción, y desde fuera
		// se parecía a una base vacía.
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM pg_class LIMIT 1`).Scan(&n); err != nil {
			return errNoAsumible
		}
		var visibles int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE c.relkind = 'r' AND n.nspname = 'public'
			  AND has_table_privilege(current_user, c.oid, 'SELECT')
			  AND has_schema_privilege(current_user, n.oid, 'USAGE')`).Scan(&visibles); err != nil {
			return errNoAsumible
		}
		if visibles == 0 {
			return errNoAsumible
		}
		return nil
	})
	switch {
	case err == nil:
		return true, nil
	case isNoAsumible(err):
		return false, nil
	default:
		return false, fmt.Errorf("pg: probar el rol de aplicación: %w", err)
	}
}
