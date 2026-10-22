// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain valida y normaliza los registros de SyERP contra el modelo que
// declara el catálogo, y traduce un documento contable en su asiento por
// partida doble.
//
// Es la frontera de confianza del servicio: aquí es donde un cuerpo JSON
// cualquiera se convierte —o no— en algo que puede entrar en `records.data`.
// Nada escribe sin pasar por `Validar`.
//
// DINERO Y CANTIDADES, la decisión que gobierna el resto:
//   - `dinero` viaja y se guarda en CÉNTIMOS enteros. Nunca en decimal flotante:
//     0,1 + 0,2 no es 0,3 y una factura no puede depender de eso.
//   - `decimal` (cantidades, horas, kilos) viaja como CADENA con su forma
//     canónica y se guarda como cadena. Postgres la castea a `numeric` para
//     sumar, que es exacto; un float64 en medio ya no lo sería. Cuesta un cast
//     en las consultas que agregan y ahorra el error que nadie ve hasta el
//     recuento de inventario.
package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
)

// ErrDatos marca los errores que son culpa de lo que se envió —un importe con
// decimales, una relación que no existe— y no del servicio. El transporte los
// traduce a 422; todo lo demás sube como 500, que es lo correcto cuando la culpa
// no es de quien llama.
var ErrDatos = errors.New("datos inválidos")

// Topes de longitud. Un ERP acaba recibiendo lo que pegue quien migra desde
// otro sistema, así que el tope es explícito y el error dice qué campo fue.
const (
	MaxTexto      = 500
	MaxTextoLargo = 50_000
	// MaxCampos frena un cuerpo que intenta engordar `data` con claves que el
	// modelo no declara. Como las claves desconocidas ya se rechazan, esto solo
	// puede dispararse con un modelo enorme; está por si alguien declara uno.
	MaxCampos = 200
)

var (
	decimalRe = regexp.MustCompile(`^-?\d{1,15}(\.\d{1,6})?$`)
	uuidRe    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// Validar comprueba `crudo` contra el modelo y devuelve el mapa normalizado que
// se guarda en `records.data`, junto con la etiqueta legible de la fila.
//
// Rechaza las claves que el modelo NO declara en vez de ignorarlas: un campo mal
// escrito que se guarda en silencio es un dato que nadie vuelve a encontrar, y
// en un ERP eso es una factura que no sale en ningún listado.
func Validar(m *catalogo.Modelo, crudo json.RawMessage) (map[string]any, string, error) {
	dec := json.NewDecoder(bytes.NewReader(crudo))
	// UseNumber: sin esto, `10000` (100,00 €) pasa por float64 y un importe
	// suficientemente grande pierde el céntimo por el camino.
	dec.UseNumber()
	var entrada map[string]json.RawMessage
	if err := dec.Decode(&entrada); err != nil {
		return nil, "", fmt.Errorf("cuerpo inválido: %w", err)
	}
	if len(entrada) > MaxCampos {
		return nil, "", fmt.Errorf("demasiados campos (%d)", len(entrada))
	}

	porClave := make(map[string]catalogo.Campo, len(m.Campos))
	for _, f := range m.Campos {
		porClave[f.Clave] = f
	}

	v := validation.New()
	for clave := range entrada {
		if _, ok := porClave[clave]; !ok {
			v.Add(clave, "el modelo "+m.Clave+" no declara este campo")
		}
	}

	data := make(map[string]any, len(m.Campos))
	for _, f := range m.Campos {
		crudoCampo, presente := entrada[f.Clave]
		if !presente || esNulo(crudoCampo) {
			if f.Requerido {
				v.Add(f.Clave, "obligatorio")
			}
			continue
		}
		valor, err := ValorDeCampo(f, crudoCampo)
		if err != nil {
			v.Add(f.Clave, err.Error())
			continue
		}
		// Un texto obligatorio que llega en blanco es un campo sin rellenar, no
		// un campo relleno con nada.
		if s, ok := valor.(string); ok && strings.TrimSpace(s) == "" {
			if f.Requerido {
				v.Add(f.Clave, "obligatorio")
			}
			continue
		}
		data[f.Clave] = valor
	}
	if err := v.Err(); err != nil {
		return nil, "", err
	}
	return data, Etiqueta(m, data), nil
}

// Etiqueta resuelve el nombre legible de la fila: el campo `titulo` del modelo.
// Vacía en los modelos numerados, donde el nombre legible es la referencia y la
// pone el store al reservar el número.
func Etiqueta(m *catalogo.Modelo, data map[string]any) string {
	if m.Titulo == "" {
		return ""
	}
	s, _ := data[m.Titulo].(string)
	return validation.TruncateRunes(strings.TrimSpace(s), 200)
}

func esNulo(crudo json.RawMessage) bool {
	return len(bytes.TrimSpace(crudo)) == 4 && bytes.Equal(bytes.TrimSpace(crudo), []byte("null"))
}

// ValorDeCampo valida y normaliza el valor de UN campo, con las mismas reglas que
// Validar. Es para quien rellena una ficha a medias (Sytue AI): ahí un campo que
// no vale se deja vacío, en vez de rechazar la ficha entera.
func ValorDeCampo(f catalogo.Campo, crudo json.RawMessage) (any, error) {
	switch f.Tipo {
	case catalogo.TipoEntero, catalogo.TipoDinero:
		var n json.Number
		if err := json.Unmarshal(crudo, &n); err != nil {
			return nil, fmt.Errorf("se esperaba un número entero")
		}
		i, err := n.Int64()
		if err != nil {
			// Es el fallo más probable en `dinero`: enviar 12.34 en vez de 1234.
			return nil, fmt.Errorf("entero (los importes van en céntimos, sin decimales)")
		}
		return i, nil

	case catalogo.TipoDecimal:
		return valorDecimal(crudo)

	case catalogo.TipoBooleano:
		var b bool
		if err := json.Unmarshal(crudo, &b); err != nil {
			return nil, fmt.Errorf("se esperaba verdadero o falso")
		}
		return b, nil
	}

	// El resto viaja como cadena.
	var s string
	if err := json.Unmarshal(crudo, &s); err != nil {
		return nil, fmt.Errorf("se esperaba texto")
	}
	s = validation.SanitizeText(strings.TrimSpace(s))
	if err := comprobarTexto(f, s); err != nil {
		return nil, err
	}
	return s, nil
}

// valorDecimal admite cadena o número JSON y devuelve SIEMPRE la forma canónica
// de texto (ver la cabecera del paquete): admitir el número es para no romper a
// quien migra desde otro sistema, no para guardarlo así.
func valorDecimal(crudo json.RawMessage) (any, error) {
	var n json.Number
	if err := json.Unmarshal(crudo, &n); err == nil {
		if !decimalRe.MatchString(n.String()) {
			return nil, fmt.Errorf("decimal con hasta 6 cifras")
		}
		return n.String(), nil
	}
	var s string
	if err := json.Unmarshal(crudo, &s); err != nil {
		return nil, fmt.Errorf("se esperaba un decimal")
	}
	s = strings.TrimSpace(s)
	if !decimalRe.MatchString(s) {
		return nil, fmt.Errorf("decimal con hasta 6 cifras")
	}
	return s, nil
}

// reglasDeTexto es la regla de cada tipo que viaja como cadena. Tabla y no un
// switch: nueve ramas en una función es justo lo que el linter de complejidad
// señala, y además así cada regla se lee —y se cambia— sin leer las otras ocho.
var reglasDeTexto = map[string]func(catalogo.Campo, string) error{
	catalogo.TipoTexto: func(_ catalogo.Campo, s string) error {
		if len([]rune(s)) > MaxTexto {
			return fmt.Errorf("máximo %d caracteres", MaxTexto)
		}
		return nil
	},
	catalogo.TipoTextoLargo: func(_ catalogo.Campo, s string) error {
		if len([]rune(s)) > MaxTextoLargo {
			return fmt.Errorf("máximo %d caracteres", MaxTextoLargo)
		}
		return nil
	},
	catalogo.TipoCorreo: func(_ catalogo.Campo, s string) error {
		if s != "" && !validation.IsEmail(s) {
			return fmt.Errorf("dirección de correo inválida")
		}
		return nil
	},
	catalogo.TipoTelefono: func(_ catalogo.Campo, s string) error {
		if len([]rune(s)) > 40 {
			return fmt.Errorf("máximo 40 caracteres")
		}
		return nil
	},
	catalogo.TipoURL: func(_ catalogo.Campo, s string) error {
		if s == "" {
			return nil
		}
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("URL http(s) inválida")
		}
		return nil
	},
	catalogo.TipoFecha: func(_ catalogo.Campo, s string) error {
		if _, err := time.Parse(time.DateOnly, s); err != nil {
			return fmt.Errorf("fecha AAAA-MM-DD")
		}
		return nil
	},
	catalogo.TipoFechaHora: func(_ catalogo.Campo, s string) error {
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			return fmt.Errorf("fecha y hora RFC3339")
		}
		return nil
	},
	catalogo.TipoSeleccion: func(f catalogo.Campo, s string) error {
		if !slices.Contains(f.Opciones, s) {
			return fmt.Errorf("valor no admitido (%s)", strings.Join(f.Opciones, ", "))
		}
		return nil
	},
	// Que la fila apuntada EXISTA se comprueba en el store, dentro de la
	// transacción: aquí no hay base de datos, y comprobarlo fuera solo diría que
	// existía hace un momento.
	catalogo.TipoRelacion: func(_ catalogo.Campo, s string) error {
		if !uuidRe.MatchString(s) {
			return fmt.Errorf("identificador inválido")
		}
		return nil
	},
}

// comprobarTexto aplica la regla del tipo a un valor ya saneado.
func comprobarTexto(f catalogo.Campo, s string) error {
	regla, hay := reglasDeTexto[f.Tipo]
	if !hay {
		return nil
	}
	return regla(f, s)
}

// EstadoInicial es el primer estado declarado por el modelo, o vacío.
func EstadoInicial(m *catalogo.Modelo) string {
	if len(m.Estados) == 0 {
		return ""
	}
	return m.Estados[0]
}

// EstadoValido indica si el estado pertenece al flujo del modelo.
func EstadoValido(m *catalogo.Modelo, estado string) bool {
	return slices.Contains(m.Estados, estado)
}

// Relaciones devuelve los pares (campo, id) de las relaciones presentes, para
// que el store compruebe de una vez que todas apuntan a filas que existen.
func Relaciones(m *catalogo.Modelo, data map[string]any) map[string]string {
	out := map[string]string{}
	for _, f := range m.Campos {
		if f.Tipo != catalogo.TipoRelacion {
			continue
		}
		if s, ok := data[f.Clave].(string); ok && s != "" {
			out[f.Clave] = s
		}
	}
	return out
}
