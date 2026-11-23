// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// El traspaso de la autoridad del correo de Matrix a la bóveda (ADR 0087).
//
// Quien ya no puede abrir su correo de Matrix no tenía salida: la autoridad de
// Matrix solo cambia con la firma de la raíz anterior. El traspaso es esa salida,
// y cuesta lo mismo que un vaciado: Sytue ID solo registra una clave con el motivo
// `traspaso` con identificación reciente (también en un alta), con auditoría y
// aviso por correo. Rooms no ve `auth_time`; ve el motivo y la fecha de la clave.
//
// Las reglas:
//
//   - Rooms da la autoridad de la bóveda sobre una de Matrix solo si la clave
//     activa tiene el motivo `traspaso` y es POSTERIOR al último cambio de la
//     autoridad de Matrix (un traspaso viejo no gana a una rotación de Matrix que
//     la persona hizo después).
//   - La primera vez que lo ve, lo deja escrito (matrix_storage_vault_handovers),
//     y desde entonces el ámbito es de la bóveda para siempre, aunque la clave
//     cambie después por continuidad o vaciado.
//   - Desde ese momento rechaza las pruebas de Matrix del ámbito (acciones de
//     entrada, vallas, recibos y cambios de raíz): sus dispositivos quedan fuera.
//   - La generación no baja: la de la bóveda se cuenta encima de la de Matrix que
//     había al traspasar (base + generación en Sytue ID). Sin autoridad de Matrix,
//     la base es 0 y todo queda como estaba.

const motivoTraspaso = "traspaso"

// maxBaseBoveda deja sitio para contar la generación de la bóveda encima de la de
// Matrix sin salir de lo que admite una autoridad (< 2^53).
const maxBaseBoveda = int64(1) << 52

// traspasoVigente: la clave activa es de un traspaso y llegó después del último
// cambio de la autoridad de Matrix.
func traspasoVigente(clave ClaveBoveda, ultimoCambioMatrix time.Time) bool {
	return clave.Motivo == motivoTraspaso && !clave.Creada.IsZero() && clave.Creada.After(ultimoCambioMatrix)
}

// registroSobre cuenta la generación de la clave encima de la base.
func registroSobre(clave ClaveBoveda, base int64) (vaultproof.Registered, error) {
	reg := clave.Registered
	if base < 0 || base >= maxBaseBoveda || reg.Generation < 1 || reg.Generation >= maxBaseBoveda {
		return vaultproof.Registered{}, matrixproof.ErrUnverified
	}
	reg.Generation += base
	return reg, nil
}

// autoridadSobre es la autoridad de la bóveda contada encima de la base.
func autoridadSobre(scope storage.Scope, clave ClaveBoveda, base int64) (matrixproof.Authority, error) {
	reg, err := registroSobre(clave, base)
	if err != nil {
		return matrixproof.Authority{}, err
	}
	return vaultproof.Authority(scope, reg)
}

// estadoTraspaso es lo que Rooms sabe del traspaso de un ámbito con autoridad de Matrix.
type estadoTraspaso struct {
	hecho  bool
	base   int64     // la generación de Matrix sobre la que se cuenta la bóveda
	ultimo time.Time // el último cambio de la autoridad de Matrix
}

func traspasoTx(ctx context.Context, tx pgx.Tx, scope storage.Scope) (estadoTraspaso, error) {
	var out estadoTraspaso
	if !productosConEntradaDeBoveda[scope.Product] {
		return out, nil
	}
	if err := tx.QueryRow(ctx, `SELECT updated_at FROM matrix_storage_authorities WHERE tenant=$1 AND subject=$2 AND product=$3`, scope.Tenant, scope.Subject, scope.Product).Scan(&out.ultimo); err != nil {
		return out, err
	}
	err := tx.QueryRow(ctx, `SELECT matrix_generation FROM matrix_storage_vault_handovers WHERE tenant=$1 AND subject=$2 AND product=$3`, scope.Tenant, scope.Subject, scope.Product).Scan(&out.base)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return out, nil
	case err != nil:
		return out, err
	}
	out.hecho = true
	return out, nil
}

// sinTraspasoTx falla con `err` si el ámbito ya pasó a la bóveda: es lo que rechaza
// las pruebas y los cambios de Matrix después del traspaso.
func sinTraspasoTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, err error) error {
	estado, errEstado := traspasoTx(ctx, tx, scope)
	if errEstado != nil {
		return errEstado
	}
	if estado.hecho {
		return err
	}
	return nil
}

// baseDeBoveda dice sobre qué generación de Matrix se cuenta la bóveda de un
// ámbito: 0 sin autoridad de Matrix; la del traspaso si lo hubo; y si hay
// autoridad de Matrix sin traspaso, la bóveda no vale (ErrUnverified).
func (s *Store) baseDeBoveda(ctx context.Context, subject, product string) (int64, error) {
	if s.baseBoveda != nil {
		return s.baseBoveda(ctx, subject, product)
	}
	var base int64
	err := s.withMatrixAuthority(ctx, subject, product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		_, err := matrixAuthorityTx(ctx, tx, scope)
		if errors.Is(err, matrixproof.ErrRootEnrollment) {
			return nil
		}
		if err != nil {
			return err
		}
		estado, err := traspasoTx(ctx, tx, scope)
		if err != nil {
			return err
		}
		if !estado.hecho {
			return matrixproof.ErrUnverified
		}
		base = estado.base
		return nil
	})
	return base, err
}

// autoridadTraspasada decide la autoridad de un ámbito que TIENE autoridad de
// Matrix. Si el traspaso ya está escrito, la de la bóveda (y sin Sytue ID no hay
// respuesta: nunca se vuelve a Matrix). Si no, la de Matrix, salvo que Sytue ID
// tenga una clave de traspaso vigente: entonces lo escribe y da la de la bóveda.
// Si Sytue ID no contesta, Matrix, como hasta ahora: un traspaso puede esperar.
func (s *Store) autoridadTraspasada(ctx context.Context, subject, product string, scope storage.Scope, matrix matrixproof.Authority, estado estadoTraspaso) (matrixproof.Authority, error) {
	if estado.hecho {
		clave, err := s.clavesBoveda.Activa(ctx, scope)
		if err != nil {
			return matrixproof.Authority{}, domain.ErrMatrixUnavailable
		}
		return autoridadSobre(scope, clave, estado.base)
	}
	clave, err := s.traspasoID.consultar(ctx, scope, s.clavesBoveda)
	if err != nil {
		// Sin saber de Sytue ID, lo de siempre: Matrix. Un traspaso puede esperar.
		return matrix, nil //nolint:nilerr // a propósito: Sytue ID caído no deja sin correo a quien usa Matrix
	}
	if !traspasoVigente(clave, estado.ultimo) {
		return matrix, nil
	}
	base, err := s.registrarTraspaso(ctx, subject, product, matrix, estado.ultimo, clave)
	if errors.Is(err, errTraspasoDesfasado) {
		// La autoridad de Matrix cambió mientras tanto: no hay traspaso. Se contesta
		// lo leído, como cualquier lectura concurrente; la siguiente ve el cambio.
		return matrix, nil
	}
	if err != nil {
		return matrixproof.Authority{}, err
	}
	return autoridadSobre(scope, clave, base)
}

// La consulta de traspaso de una cuenta de Matrix no puede retrasar su correo.
//
// Antes del traspaso, preguntar a Sytue ID es solo para detectarlo: si no
// contesta, la respuesta es la de Matrix de siempre. Por eso esa consulta lleva un
// plazo corto (plazoConsultaTraspaso), y si falla o se pasa de plazo, durante
// pausaConsultaTraspaso no se vuelve a preguntar: se contesta Matrix al momento.
// Lo que se recuerda es la AVERÍA de Sytue ID, nunca un «no hay traspaso»: con
// Sytue ID sano se pregunta en cada lectura, así que un traspaso recién registrado
// se ve en la siguiente (la puerta de la bóveda lee la autoridad justo después de
// registrarlo y no puede encontrarse una respuesta vieja). Durante una avería no
// se detecta, pero tampoco se podría.
//
// Tras el traspaso no hay atajo: sin Sytue ID no hay autoridad (ver arriba).
const (
	plazoConsultaTraspaso = 400 * time.Millisecond
	pausaConsultaTraspaso = 30 * time.Second
)

var errSytueIDEnPausa = errors.New("store: Sytue ID no contestó hace poco; no se le pregunta")

type consultaTraspaso struct {
	mu    sync.Mutex
	hasta time.Time
	ahora func() time.Time // sustituible en pruebas
	plazo time.Duration
}

func (c *consultaTraspaso) reloj() time.Time {
	if c.ahora != nil {
		return c.ahora()
	}
	return time.Now()
}

func (c *consultaTraspaso) consultar(ctx context.Context, scope storage.Scope, claves ClavesBoveda) (ClaveBoveda, error) {
	c.mu.Lock()
	enPausa := c.reloj().Before(c.hasta)
	plazo := c.plazo
	c.mu.Unlock()
	if enPausa {
		return ClaveBoveda{}, errSytueIDEnPausa
	}
	if plazo <= 0 {
		plazo = plazoConsultaTraspaso
	}
	corto, cancel := context.WithTimeout(ctx, plazo)
	defer cancel()
	clave, err := claves.Activa(corto, scope)
	// «No tiene clave» es una respuesta; un fallo o el plazo agotado, una avería.
	// Si quien se cansó fue el que llama, no es culpa de Sytue ID.
	if err != nil && !errors.Is(err, ErrSinClaveDeBoveda) && ctx.Err() == nil {
		c.mu.Lock()
		c.hasta = c.reloj().Add(pausaConsultaTraspaso)
		c.mu.Unlock()
	}
	return clave, err
}

var errTraspasoDesfasado = errors.New("store: la autoridad de Matrix cambió durante el traspaso")

// registrarTraspaso escribe el traspaso sobre la autoridad de Matrix que se leyó,
// y solo si sigue siendo esa. Si otro lo escribió antes, devuelve el suyo.
func (s *Store) registrarTraspaso(ctx context.Context, subject, product string, leida matrixproof.Authority, ultimo time.Time, clave ClaveBoveda) (int64, error) {
	var base int64
	err := s.withMatrixAuthority(ctx, subject, product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		actual, err := matrixAuthorityTx(ctx, tx, scope)
		if err != nil {
			return err
		}
		estado, err := traspasoTx(ctx, tx, scope)
		if err != nil {
			return err
		}
		if estado.hecho {
			base = estado.base
			return nil
		}
		if actual != leida || !estado.ultimo.Equal(ultimo) || !traspasoVigente(clave, estado.ultimo) {
			return errTraspasoDesfasado
		}
		if actual.Generation >= maxBaseBoveda {
			return matrixproof.ErrUnverified
		}
		if _, err := tx.Exec(ctx, `INSERT INTO matrix_storage_vault_handovers(tenant,subject,product,matrix_generation,key_generation,key_id) VALUES($1,$2,$3,$4,$5,$6)`, scope.Tenant, scope.Subject, scope.Product, actual.Generation, clave.Generation, clave.KeyID); err != nil {
			return err
		}
		base = actual.Generation
		return nil
	})
	return base, err
}
