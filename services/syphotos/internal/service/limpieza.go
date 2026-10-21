// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// Limpieza borra del almacén lo generado para fotos que ya no existen.
//
// Vive aquí y no en el handler porque lo necesitan DOS: quien vacía la papelera
// a mano y el worker que la vacía sola. Escrito dos veces, una de las dos se
// quedaría atrás en cuanto cambien las variantes.
type Limpieza struct {
	objetos *objectstore.Client
}

func NuevaLimpieza(o *objectstore.Client) *Limpieza { return &Limpieza{objetos: o} }

// BorrarDerivadas se lleva lo que sabe nombrar y devuelve cuántas no pudo.
//
// A LA BUENA DE DIOS Y A PROPÓSITO: las derivadas son caché, así que un fallo
// aquí no puede impedir que la papelera se vacíe —la fila ya se fue— ni
// convertirse en un error para quien lo pidió. Lo que no se borre queda
// huérfano y ocupando, que es un problema de espacio, no de corrección.
//
// Se borran las de la receta que estuviera vigente y las del ORIGINAL, que son
// las dos que este servicio fabrica.
//
// ponytail: las de ediciones anteriores se quedan. Para barrer todo lo de una
// foto haría falta listar por prefijo (`a/<id>/`), que `objectstore` no ofrece;
// cuando el espacio lo pida, ahí está el sitio.
func (l *Limpieza) BorrarDerivadas(ctx context.Context, tenant string, activo uuid.UUID, huella string) int {
	if l == nil || l.objetos == nil {
		return 0
	}
	huellas := []string{HuellaDelOriginal()}
	if huella != "" && huella != HuellaDelOriginal() {
		huellas = append(huellas, huella)
	}
	t := tenancy.Tenant{Slug: tenant}
	fallos := 0
	for _, h := range huellas {
		claves, err := domain.ClavesDeLasDerivadas(activo, h)
		if err != nil {
			continue
		}
		for _, clave := range claves {
			if err := l.objetos.Delete(ctx, t, clave); err != nil &&
				!errors.Is(err, objectstore.ErrNotFound) {
				fallos++
			}
		}
	}
	return fallos
}

// HuellaDelOriginal nombra la foto sin editar. Se calcula una vez y no se
// escribe a mano: es constante para una versión de la receta y a mano quedaría
// desincronizada en cuanto cambie la forma canónica.
func HuellaDelOriginal() string { return huellaDelOriginal }

var huellaDelOriginal = func() string {
	h, err := domain.RecetaVacia().Huella()
	if err != nil {
		panic("syphotos: la receta vacía no tiene huella: " + err.Error())
	}
	return h
}()
