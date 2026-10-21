// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"time"

	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
)

// Apunte es una línea del asiento: una cuenta y un importe a UN lado.
type Apunte struct {
	Cuenta   string
	Concepto string
	Debe     int64
	Haber    int64
	// Tercero es el contacto (cliente/proveedor) al que se imputa la línea de
	// contrapartida. Vacío en las de ingreso, gasto o impuesto.
	Tercero string
}

// Asiento es el apunte doble completo de un documento, listo para el libro.
type Asiento struct {
	Diario   string
	Fecha    time.Time
	Concepto string
	Moneda   string
	Apuntes  []Apunte
}

// MonedaPorDefecto es la divisa del asiento cuando el documento no dice otra.
const MonedaPorDefecto = "EUR"

// AsientoDe traduce un documento en su asiento por partida doble, según lo que
// declara el bloque `contable` de su modelo.
//
// ESTA ES LA ÚNICA LÓGICA DE NEGOCIO NO GENÉRICA DEL SERVICIO, y cabe en una
// función porque la diferencia entre una factura de venta y una de compra son
// dos cuentas y un signo — no dos módulos. Lo que cambia entre sectores (las
// cuentas del plan, el diario) es dato del manifiesto.
//
// SIGNOS. Cada importe se coloca en su lado natural y, si viene NEGATIVO, cambia
// de lado con su valor absoluto. Eso es exactamente una factura rectificativa, y
// tratarla como un caso aparte es cómo se cuelan los abonos con el signo al
// revés. Como total = base + iva, la suma de los dos lados cuadra sea cual sea
// la combinación de signos.
func AsientoDe(m *catalogo.Modelo, etiqueta string, data map[string]any) (Asiento, error) {
	c := m.Contable
	if c == nil {
		return Asiento{}, fmt.Errorf("el modelo %q no genera asiento: %w", m.Clave, ErrDatos)
	}

	base, err := Centimos(data, c.CampoBase)
	if err != nil {
		return Asiento{}, err
	}
	var iva int64
	if c.CampoIVA != "" {
		if iva, err = Centimos(data, c.CampoIVA); err != nil {
			return Asiento{}, err
		}
	}
	total := base + iva
	if base == 0 && iva == 0 {
		return Asiento{}, fmt.Errorf("documento sin importe: no hay nada que asentar: %w", ErrDatos)
	}

	// `fecha` es obligatoria en los modelos contables (lo comprueba el catálogo
	// al cargar), así que llegar aquí sin ella es un registro escrito antes de
	// que el modelo fuera contable.
	crudoFecha, _ := data["fecha"].(string)
	fecha, err := time.Parse(time.DateOnly, crudoFecha)
	if err != nil {
		return Asiento{}, fmt.Errorf("fecha del documento inválida o ausente: %w", ErrDatos)
	}

	var tercero string
	if c.CampoTercero != "" {
		tercero, _ = data[c.CampoTercero].(string)
	}

	a := Asiento{Diario: c.Diario, Fecha: fecha, Concepto: etiqueta, Moneda: MonedaPorDefecto}
	anotar := func(cuenta string, importe int64, alDebe bool, terceroLinea string) {
		if importe == 0 {
			// Un apunte a cero no dice nada y además lo rechaza la base de datos
			// (`move_lines_un_solo_lado`). Una factura sin IVA no tiene línea de
			// IVA, que es lo correcto y no una excepción.
			return
		}
		if importe < 0 {
			importe, alDebe = -importe, !alDebe
		}
		ap := Apunte{Cuenta: cuenta, Concepto: etiqueta, Tercero: terceroLinea}
		if alDebe {
			ap.Debe = importe
		} else {
			ap.Haber = importe
		}
		a.Apuntes = append(a.Apuntes, ap)
	}

	if c.Cobro {
		// Venta: el cliente DEBE el total; el ingreso y el IVA repercutido van al
		// haber.
		anotar(c.CuentaContrapartida, total, true, tercero)
		anotar(c.CuentaIngresoGasto, base, false, "")
		anotar(c.CuentaIVA, iva, false, "")
	} else {
		// Compra o gasto: el gasto y el IVA soportado al debe; se DEBE el total al
		// proveedor.
		anotar(c.CuentaIngresoGasto, base, true, "")
		anotar(c.CuentaIVA, iva, true, "")
		anotar(c.CuentaContrapartida, total, false, tercero)
	}

	// El cuadre lo GARANTIZA Postgres al confirmar (trigger `move_lines_cuadre`).
	// Esto está aquí solo para que el fallo se cuente con palabras y con el
	// documento delante, en vez de como una violación de restricción a mitad de
	// una transacción.
	var debe, haber int64
	for _, ap := range a.Apuntes {
		debe += ap.Debe
		haber += ap.Haber
	}
	if debe != haber {
		return Asiento{}, fmt.Errorf("asiento descuadrado: debe=%d haber=%d: %w", debe, haber, ErrDatos)
	}
	return a, nil
}

// Centimos lee un campo de tipo dinero. Acepta que falte (vale cero) porque el
// IVA es opcional; el catálogo ya obliga a que la base sea obligatoria en los
// modelos que lo necesitan.
func Centimos(data map[string]any, clave string) (int64, error) {
	if clave == "" {
		return 0, nil
	}
	switch v := data[clave].(type) {
	case nil:
		return 0, nil
	case int64:
		return v, nil
	case float64:
		// Solo puede llegar así leyendo de `data` recién deserializado de jsonb
		// sin UseNumber. Se admite si es entero exacto y se rechaza si no, antes
		// de que un céntimo perdido entre en el libro.
		if v != float64(int64(v)) {
			return 0, fmt.Errorf("%s: importe con decimales (van en céntimos): %w", clave, ErrDatos)
		}
		return int64(v), nil
	default:
		return 0, fmt.Errorf("%s: no es un importe: %w", clave, ErrDatos)
	}
}
