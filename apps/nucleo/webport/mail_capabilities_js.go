//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/fallo"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/limits"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// MailCapabilitiesJS es la operación `mail.capabilities`: el tamaño máximo de un
// mensaje según el estado del almacén privado de correo, leído por un puerto de
// origen de solo lectura. La sirven el núcleo completo (`core`) y el módulo de
// correo (`mail`), con sus propios registros de fuentes y políticas: así Mail puede
// pedirla al módulo que ya tiene abierto, sin arrancar un segundo núcleo.
func MailCapabilitiesJS(ctx context.Context, req js.Value, coreSources *Sources, corePolicies *Policies) (any, error) {
	port, _, err := AuthorizedJS(ctx, req)
	if err != nil {
		return nil, fallo.En("puerto", err)
	}
	owner, err := OpenStorage(ctx, port, coreSources, corePolicies)
	if err != nil || owner.FileScope().Product != "mail" || owner.StorageIdentityVersion() != storage.IsolatedIdentity {
		return nil, fallo.En("almacen", ErrRejected)
	}
	source, err := NewSource(owner, false)
	if err != nil {
		return nil, fallo.En("origen", err)
	}
	// Cada «no» del estado con su nombre: el servicio decía `private_ingress_ready:
	// false` a todas las cuentas y aquí solo salía `crypto_rejected`.
	state, err := source.PrivateStorageState(ctx, owner.FileScope())
	switch {
	case err != nil:
		return nil, fallo.En("estado", err)
	case state.Shared:
		return nil, fallo.En("compartido", ErrRejected)
	case !state.PrivateIngressReady:
		return nil, fallo.En("entrada_no_lista", ErrRejected)
	}
	maximum, err := limits.MessageBytes(state.MaxBodyBytes)
	if err != nil {
		return nil, fallo.En("limite", err)
	}
	return map[string]any{"v": 1, "scope": map[string]any{"tenant": state.Scope.Tenant, "subject": state.Scope.Subject, "product": state.Scope.Product}, "max_body_bytes": maximum}, nil
}
