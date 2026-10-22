// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"
)

// Las DERIVADAS: todo lo que Sytue Photos calcula a partir del original.
//
// Ninguna es un dato: todas se pueden volver a calcular del original —que está
// en Drive— más la receta. Eso es lo que permite tratarlas como caché y no como
// contenido: caducan, se comprimen, se borran cuando aprieta el disco, y nadie
// pierde nada. La copia de seguridad no las incluye a propósito.

// Variante es para qué sirve cada tamaño. El lado mayor en píxeles va en la
// tabla de abajo, no en el nombre: los nombres se guardan en claves de objeto y
// cambiar «1024» por «1200» renombraría todo lo ya generado.
type Variante string

const (
	// VarianteMiniatura es la de la cuadrícula de la línea de tiempo. Se piden
	// cientos de golpe, así que manda el peso por encima del detalle.
	VarianteMiniatura Variante = "miniatura"
	// VariantePrevia es sobre la que EDITA el navegador. No es un tamaño de
	// visualización: es el lienzo de trabajo. Se elige suficientemente pequeña
	// para que un ajuste responda al instante en un portátil modesto, porque
	// mover un deslizador sobre 24 megapíxeles no va fluido en ningún equipo.
	VariantePrevia Variante = "previa"
	// VarianteGrande es la del visor a pantalla completa.
	VarianteGrande Variante = "grande"
)

// VariantesConocidas son todas las que este servicio fabrica, para quien
// necesite recorrerlas —borrar lo generado de una foto, por ejemplo— sin
// repetir la lista y quedarse corto cuando se añada una.
var VariantesConocidas = []Variante{VarianteMiniatura, VariantePrevia, VarianteGrande}

// ladoMayor es el tamaño de cada variante. Se guarda el LADO MAYOR y no el par
// ancho×alto porque una galería tiene fotos verticales y horizontales mezcladas
// y encajar todas en un rectángulo fijo obliga a recortar o a deformar.
var ladoMayor = map[Variante]int{
	VarianteMiniatura: 512,
	VariantePrevia:    2048,
	VarianteGrande:    3200,
}

// LadoMayor devuelve el tamaño de la variante y si existe.
func (v Variante) LadoMayor() (int, bool) {
	n, ok := ladoMayor[v]
	return n, ok
}

// Variantes son todas, en orden de tamaño. Sirve para generarlas al ingerir y
// para que la interfaz sepa qué puede pedir.
func Variantes() []Variante {
	return []Variante{VarianteMiniatura, VariantePrevia, VarianteGrande}
}

// Derivada identifica UN fichero generado.
//
// SIN TENANT, y no por olvido: `platform/pkg/objectstore` antepone
// `tenants/<slug>/` a toda clave (`Tenant.StoragePrefix`). Ponerlo también aquí
// daba `tenants/corporate/t/corporate/…` y, peor, dejaba el aislamiento entre
// organizaciones dependiendo de DOS sitios que pueden discrepar. Lo decide el
// almacén, que es quien lo aplica a todos los servicios por igual.
type Derivada struct {
	ActivoID uuid.UUID
	// Huella de la receta aplicada. La receta vacía tiene la suya, así que el
	// original sin editar entra por este mismo camino y el almacén no necesita
	// una rama especial.
	Huella   string
	Variante Variante
	// Formato es la extensión del fichero generado (sin punto).
	Formato string
}

// Clave es la ruta en el almacén de objetos.
//
// LA HUELLA VA EN LA RUTA, y ahí está todo el diseño: la clave depende de la
// receta, así que editar no invalida nada —escribe en otro sitio— y deshacer
// devuelve un fichero que probablemente siga ahí. Sin eso habría que borrar y
// regenerar en cada ajuste, y volver atrás costaría lo mismo que ir adelante.
//
// El ID del activo va antes que la huella: así todas las versiones de una foto
// caen bajo un prefijo común y limpiar las de una foto borrada es un solo
// borrado por prefijo.
//
// Sobre esta clave, el almacén antepone `tenants/<slug>/` — que es lo que
// permite dar de baja una organización entera con un prefijo, como pide el RGPD.
func (d Derivada) Clave() (string, error) {
	if err := d.valida(); err != nil {
		return "", err
	}
	return path.Join(
		"a", d.ActivoID.String(),
		d.Huella,
		fmt.Sprintf("%s.%s", d.Variante, d.Formato),
	), nil
}

// PrefijoDelActivo es todo lo generado para una foto, sea cual sea la receta. Es
// lo que se borra cuando la foto se va.
func PrefijoDelActivo(id uuid.UUID) string {
	return path.Join("a", id.String()) + "/"
}

// valida rechaza lo que no puede formar una clave segura.
//
// SE COMPRUEBA AQUÍ Y NO EN QUIEN LLAMA porque una clave es una RUTA: un tenant
// con «../» dentro escribiría fuera de su prefijo, que es exactamente la fuga
// entre organizaciones que el prefijo viene a evitar. `path.Join` limpia los
// «..», pero limpiar en silencio es peor: la clave resultante apuntaría a otro
// sitio sin que nadie se entere.
func (d Derivada) valida() error {
	if d.ActivoID == uuid.Nil {
		return fmt.Errorf("%w: derivada sin activo", ErrInvalidInput)
	}
	if d.Huella == "" || !segmentoSeguro(d.Huella) {
		return fmt.Errorf("%w: huella inválida: %q", ErrInvalidInput, d.Huella)
	}
	if _, ok := d.Variante.LadoMayor(); !ok {
		return fmt.Errorf("%w: variante desconocida: %q", ErrInvalidInput, d.Variante)
	}
	if d.Formato == "" || !segmentoSeguro(d.Formato) {
		return fmt.Errorf("%w: formato inválido: %q", ErrInvalidInput, d.Formato)
	}
	return nil
}

// segmentoSeguro admite solo lo que no puede cambiar el significado de una ruta:
// letras, dígitos, guion y guion bajo. Lista blanca y no lista negra — de las
// negras siempre se escapa algo, y aquí lo que se escapa cruza el aislamiento
// entre organizaciones.
func segmentoSeguro(s string) bool {
	if len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_':
		default:
			return false
		}
	}
	return !strings.HasPrefix(s, "-")
}

// ClavesDeLasDerivadas son las claves de todo lo generado para una foto con una
// receta dada. Es lo que hay que borrar cuando la foto se va.
//
// Aquí y no en quien llama porque es la MISMA lista que usa quien las escribe:
// una variante nueva se añade en `VariantesConocidas` y se borra sola. La
// alternativa —repetir «miniatura, previa, grande» en cada sitio que limpie— se
// queda corta en cuanto se añada una y deja basura que nadie sabe nombrar.
func ClavesDeLasDerivadas(activo uuid.UUID, huella string) ([]string, error) {
	claves := make([]string, 0, len(VariantesConocidas)*2)
	for _, v := range VariantesConocidas {
		for _, formato := range []string{"jpg", "png"} {
			c, err := (Derivada{ActivoID: activo, Huella: huella, Variante: v, Formato: formato}).Clave()
			if err != nil {
				return nil, err
			}
			claves = append(claves, c)
		}
	}
	return claves, nil
}
