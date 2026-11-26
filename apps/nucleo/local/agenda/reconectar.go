// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"context"
	"errors"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// Reconectar vuelve a entrar en una cuenta que el servidor rechazó (EstadoReconectar)
// sin cambiar su id: sus calendarios y libretas de aquí, y lo que ya se sincronizó,
// siguen siendo los mismos. Con DAV prueba la contraseña nueva contra las URL de la
// cuenta y, si entra, la guarda; con Google o Microsoft repite el permiso en el
// navegador (abrir) y guarda el token de refresco nuevo.
func (r *Registro) Reconectar(ctx context.Context, id, clave string, abrir func(string) error) error {
	var c Cuenta
	ok := false
	for _, x := range r.Cuentas() {
		if x.ID == id {
			c, ok = x, true
		}
	}
	if !ok {
		return ErrSinCuenta
	}
	switch c.Proveedor {
	case ProveedorGoogle, ProveedorMicrosoft:
		o, err := r.oauthDe(c)
		if err != nil {
			return err
		}
		if r.red != nil && r.HTTP == nil {
			defer r.red.Permitir(red.Destino{Proposito: red.OAuth, Host: c.OAuth, Motivo: "reconectar"})()
		}
		t, err := o.Conectar(ctx, r.cliente(red.OAuth), abrir)
		if err != nil {
			return err
		}
		if t.Refresco == "" {
			return errors.New("agenda: el proveedor no dio acceso sin conexión (falta el token de refresco)")
		}
		if err := r.secretos.Guardar(secretoDe(id, "refresco"), t.Refresco); err != nil {
			return err
		}
	default:
		a := Alta{Proveedor: c.Proveedor, Rotulo: c.Rotulo, Usuario: c.Usuario, Clave: clave}
		if c.Da(Calendario) {
			a.Calendario = c.Calendario
		}
		if c.Da(Contactos) {
			a.Contactos = c.Contactos
		}
		if a.Calendario == "" && a.Contactos == "" {
			return ErrSinServicio
		}
		if _, err := r.Probar(ctx, a); err != nil {
			return err
		}
		if err := r.secretos.Guardar(secretoDe(id, "contrasena"), clave); err != nil {
			return err
		}
	}
	// Los sincronizadores abiertos llevan la credencial vieja: se rehacen al siguiente
	// uso (su estado está en disco, no se pierde nada).
	r.mu.Lock()
	for k := range r.sincs {
		if strings.HasPrefix(k, id+"/") {
			delete(r.sincs, k)
		}
	}
	r.mu.Unlock()
	r.PonerEstado(id, EstadoOK)
	return nil
}
