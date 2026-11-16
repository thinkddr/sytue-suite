// SPDX-License-Identifier: Apache-2.0
package eml

import (
	"context"
	"errors"
	"io"
)

var (
	ErrLimiteMensaje  = errors.New("eml: mensaje supera el límite autorizado")
	ErrLimiteInvalido = errors.New("eml: límite de mensaje inválido")
)

type lectorContexto struct {
	ctx context.Context
	r   io.Reader
}

func (l lectorContexto) Read(p []byte) (int, error) {
	if err := l.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := l.r.Read(p)
	if current := l.ctx.Err(); current != nil {
		clear(p[:n])
		return 0, current
	}
	return n, err
}
