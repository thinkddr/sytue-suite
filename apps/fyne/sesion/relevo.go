// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

// # El relevo de One: la señal de cambios llega a las apps cerradas de Android
//
// Sin Google no hay push en Android. La única app de la suite que sigue escuchando con
// todo cerrado es One (el Hub), cuyo servicio en primer plano (eu.sytue.nativo.Avisos)
// mantiene vivo su proceso y con él el SSE de la barra. Cuando por ahí llega la señal de
// cambios (`datos`) o un aviso de un producto, One se lo releva a la app de ese producto
// (ADR 0078, enmienda del 2026-11-06, pieza 6): una llamada SIN DATOS al proveedor
// eu.sytue.nativo.RelevoWidgets de esa app, que solo acepta a One con su certificado de
// Play fijado y solo encola su trabajo de refresco (RefrescoWidgets), que lee por su
// cuenta (RefrescarSinVentana). Fuera de Android, nil: no hay a quién relevar.
var relevarWidget func(paquete string)

// relevar releva a la app de `producto` que algo suyo ha cambiado, si esta app es One
// y aquella no lo es. En otra rutina: es una llamada al sistema, y AvisoDelHub corre en
// la de la barra.
func (s *Suite) relevar(producto string) {
	f := relevarWidget
	if f == nil || s.app == nil || s.app.Metadata().ID != bundleOne {
		return
	}
	paquete, ok := bundlePorProducto[producto]
	if !ok || paquete == bundleOne {
		return
	}
	go f(paquete)
}
