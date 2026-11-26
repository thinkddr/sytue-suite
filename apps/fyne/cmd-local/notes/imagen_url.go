// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/apps/fyne/apps/notes"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// La nota de imagen por URL (apps/notes, imagen_url.go): la imagen de una dirección se
// baja SOLO cuando la persona lo pide —«Cargar la imagen» en la nota, «Volver a cargar la
// imagen» en su «…» o al crearla con la dirección que acaba de escribir—, como las
// imágenes remotas de un correo en Mail: por el permiso puntual del portero
// (`red.Portero.Puntual`, propósito `imagen-remota`), que solo sale por https a
// direcciones públicas de Internet, nada con «Sin conexión», y mientras dura la petición
// Ajustes → Red la enseña como temporal. La pantalla la guarda dentro de la nota, y desde
// entonces se ve sin red; al volver a cargarla, la nueva sustituye a la de dentro solo si
// llega bien.

// bajarImagen es FicherosLocales.BajarImagen con el portero p: los fallos que la pantalla
// sabe decir salen como notes.ErrImagenNoPermitida, notes.ErrImagenSinConexion y
// notes.ErrNoEsImagen (una página en vez de una imagen, o más de notes.MaxImagenRemota).
func bajarImagen(p *red.Portero) func(ctx context.Context, direccion string) ([]byte, error) {
	cliente := p.Puntual(red.ImagenRemota, "")
	return func(ctx context.Context, direccion string) ([]byte, error) {
		u, err := url.Parse(strings.TrimSpace(direccion))
		if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" {
			return nil, notes.ErrImagenNoPermitida
		}
		if p.SinConexion() {
			return nil, notes.ErrImagenSinConexion
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
		if err != nil {
			return nil, notes.ErrImagenNoPermitida
		}
		req.Header.Set("Accept", "image/*")
		resp, err := cliente.Do(req)
		if err != nil {
			switch {
			case p.SinConexion():
				return nil, notes.ErrImagenSinConexion
			case errors.Is(err, red.ErrDenegado):
				// http en una redirección, este equipo o la red de casa.
				return nil, fmt.Errorf("%w: %w", notes.ErrImagenNoPermitida, err)
			}
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("notes: %s contesta %d", u.Hostname(), resp.StatusCode)
		}
		if resp.ContentLength > notes.MaxImagenRemota || strings.HasPrefix(resp.Header.Get("Content-Type"), "text/") {
			return nil, notes.ErrNoEsImagen
		}
		datos, err := io.ReadAll(io.LimitReader(resp.Body, notes.MaxImagenRemota+1))
		if err != nil {
			return nil, err
		}
		if len(datos) > notes.MaxImagenRemota {
			return nil, notes.ErrNoEsImagen
		}
		return datos, nil
	}
}
