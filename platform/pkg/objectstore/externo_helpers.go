// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// spool vuelca un cuerpo de tamaño desconocido a un fichero temporal para poder
// mandar Content-Length (Google y Graph lo exigen en las subidas grandes). El
// llamador cierra y borra con la función devuelta.
type readSeekAt interface {
	io.ReadSeeker
	io.ReaderAt
}

func spool(r io.Reader, size int64) (body readSeekAt, n int64, cerrar func(), err error) {
	if rs, ok := r.(readSeekAt); ok && size >= 0 {
		return rs, size, func() {}, nil
	}
	tmp, err := os.CreateTemp("", "sytue-conector-*")
	if err != nil {
		return nil, 0, nil, err
	}
	cerrar = func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }
	n, err = io.Copy(tmp, r)
	if err != nil {
		cerrar()
		return nil, 0, nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		cerrar()
		return nil, 0, nil, err
	}
	return tmp, n, cerrar, nil
}

// TokenSource es lo que un backend por consentimiento necesita: un token vigente.
type TokenSource func(ctx context.Context) (string, error)

// conProveedor hace una petición autenticada al proveedor y devuelve la respuesta.
func conProveedor(ctx context.Context, cl *http.Client, tok TokenSource, metodo, u string, body io.Reader, size int64, cab map[string]string) (*http.Response, error) {
	token, err := tok(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, metodo, u, body)
	if err != nil {
		return nil, err
	}
	if size >= 0 {
		req.ContentLength = size
	}
	req.Header.Set("Authorization", "Bearer "+token)
	for k, v := range cab {
		req.Header.Set(k, v)
	}
	return cl.Do(req)
}

func errorDe(res *http.Response, op string) error {
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
	return fmt.Errorf("%s: %s: %s", op, res.Status, strings.TrimSpace(string(body)))
}
