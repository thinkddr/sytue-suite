// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// La RECETA: qué se le hace a una foto, sin tocar el original.
//
// Es la pieza central de Sytue Photos y la única que usan las dos mitades del
// producto, así que conviene decir por qué es así.
//
// EDICIÓN NO DESTRUCTIVA. Editar no reescribe píxeles: añade un paso a una
// lista. El original sigue en Drive intacto, cualquier ajuste se deshace
// quitando su paso, y volver a la foto de origen es borrar la receta. Guardar el
// resultado en su lugar sería más simple y es exactamente lo que impide
// arrepentirse tres meses después.
//
// EL MISMO CÁLCULO EN LOS DOS LADOS. El navegador previsualiza sobre una copia
// reducida para responder al instante; el servidor exporta aplicando LA MISMA
// receta al original. Si los dos no coinciden, lo que se descarga no es lo que
// se vio, que es el peor fallo posible en un editor. De ahí que los pasos se
// definan con parámetros numéricos explícitos y no con «lo que haga la librería
// de turno»: el contrato es esta estructura, no una implementación.
//
// LA HUELLA IDENTIFICA EL RESULTADO. `Huella()` da un hash estable de (versión +
// pasos), y con él se nombran las derivadas: misma foto y misma receta ⇒ mismo
// fichero, así que la caché acierta sola y cambiar un deslizador invalida solo
// lo que cambió. Por eso la serialización tiene que ser CANÓNICA —claves
// ordenadas, números normalizados—: dos recetas iguales escritas distinto
// tendrían huellas distintas y se recalcularía todo sin motivo.

// VersionReceta es el formato. Sube cuando cambia el SIGNIFICADO de un paso, no
// cuando se añade uno nuevo: una receta guardada tiene que seguir dando la misma
// imagen dentro de tres años o la edición no destructiva es una promesa falsa.
const VersionReceta = 1

// TipoPaso es la operación. Cadena y no número: una receta se lee en la base de
// datos y en el log, y `"exposicion"` se entiende sin tabla de consulta.
type TipoPaso string

const (
	PasoRecorte     TipoPaso = "recorte"     // x, y, ancho, alto en fracción del lienzo
	PasoGiro        TipoPaso = "giro"        // grados: 90, 180, 270
	PasoEspejo      TipoPaso = "espejo"      // eje: "h" o "v"
	PasoExposicion  TipoPaso = "exposicion"  // pasos de diafragma, -5..+5
	PasoContraste   TipoPaso = "contraste"   // -1..+1
	PasoSaturacion  TipoPaso = "saturacion"  // -1..+1
	PasoTemperatura TipoPaso = "temperatura" // -1..+1 (frío→cálido)
)

// Paso es una operación con sus parámetros.
//
// Los parámetros van en un mapa de números y no en campos por tipo: son quince
// operaciones previstas y una estructura con quince punteros nulos es ilegible.
// Lo que sostiene la corrección no es el tipo, es `Valida()` — y esa comprueba
// rangos, que es lo que de verdad se equivoca.
type Paso struct {
	Tipo   TipoPaso           `json:"tipo"`
	Params map[string]float64 `json:"params,omitempty"`
	// Eje solo lo usa el espejo. Aparte de Params porque no es un número y
	// meterlo como 0/1 obligaría a recordar cuál es cuál.
	Eje string `json:"eje,omitempty"`
}

// rangos declara qué parámetros admite cada paso y entre qué valores. Tabla y no
// un `switch` con quince ramas: añadir una operación es una línea aquí, y
// `Valida()` no cambia.
var rangos = map[TipoPaso]map[string][2]float64{
	PasoRecorte:     {"x": {0, 1}, "y": {0, 1}, "ancho": {0, 1}, "alto": {0, 1}},
	PasoGiro:        {"grados": {0, 270}},
	PasoEspejo:      {},
	PasoExposicion:  {"valor": {-5, 5}},
	PasoContraste:   {"valor": {-1, 1}},
	PasoSaturacion:  {"valor": {-1, 1}},
	PasoTemperatura: {"valor": {-1, 1}},
}

// Valida comprueba que el paso existe y que sus parámetros están en rango.
//
// SE RECHAZA LO QUE NO SE ENTIENDE, y aquí sí: un parámetro fuera de rango o
// desconocido no se puede «interpretar con sentido común» —el cliente y el
// servidor lo interpretarían distinto, que es justo lo que rompe el contrato—.
func (p Paso) Valida() error {
	esperados, ok := rangos[p.Tipo]
	if !ok {
		return fmt.Errorf("%w: paso desconocido %q", ErrInvalidInput, p.Tipo)
	}
	if p.Tipo == PasoEspejo && p.Eje != "h" && p.Eje != "v" {
		return fmt.Errorf("%w: el espejo necesita eje «h» o «v», llegó %q", ErrInvalidInput, p.Eje)
	}
	for nombre, v := range p.Params {
		lim, ok := esperados[nombre]
		if !ok {
			return fmt.Errorf("%w: %s no admite el parámetro %q", ErrInvalidInput, p.Tipo, nombre)
		}
		// NaN e infinito NO son «fuera de rango»: las comparaciones con NaN son
		// siempre falsas, así que sin esto se colarían por el `if` de abajo y
		// llegarían al motor de píxeles.
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%w: %s.%s no es un número", ErrInvalidInput, p.Tipo, nombre)
		}
		if v < lim[0] || v > lim[1] {
			return fmt.Errorf("%w: %s.%s = %g, fuera de [%g, %g]",
				ErrInvalidInput, p.Tipo, nombre, v, lim[0], lim[1])
		}
	}
	for nombre := range esperados {
		if _, ok := p.Params[nombre]; !ok {
			return fmt.Errorf("%w: a %s le falta el parámetro %q", ErrInvalidInput, p.Tipo, nombre)
		}
	}
	if p.Tipo == PasoRecorte {
		if p.Params["x"]+p.Params["ancho"] > 1.0000001 || p.Params["y"]+p.Params["alto"] > 1.0000001 {
			return fmt.Errorf("%w: el recorte se sale del lienzo", ErrInvalidInput)
		}
		if p.Params["ancho"] <= 0 || p.Params["alto"] <= 0 {
			return fmt.Errorf("%w: el recorte no puede tener lado cero", ErrInvalidInput)
		}
	}
	return nil
}

// Receta es la lista ORDENADA de pasos. El orden importa y no se toca: recortar
// y luego girar no da lo mismo que girar y luego recortar.
type Receta struct {
	Version int    `json:"version"`
	Pasos   []Paso `json:"pasos"`
}

// Valida comprueba la receta entera.
func (r Receta) Valida() error {
	if r.Version <= 0 || r.Version > VersionReceta {
		return fmt.Errorf("%w: versión de receta %d (esta compilación entiende hasta la %d)",
			ErrInvalidInput, r.Version, VersionReceta)
	}
	// UN TOPE, y no por rendimiento: una receta la escribe la interfaz paso a
	// paso, así que mil pasos significa que algo está en bucle. Sin tope, ese
	// bucle se guarda en la base de datos y se reproduce en cada render.
	if len(r.Pasos) > 256 {
		return fmt.Errorf("%w: %d pasos; el máximo es 256", ErrInvalidInput, len(r.Pasos))
	}
	for i, p := range r.Pasos {
		if err := p.Valida(); err != nil {
			return fmt.Errorf("paso %d: %w", i, err)
		}
	}
	return nil
}

// canonica serializa de forma estable: claves ordenadas y números normalizados.
//
// `encoding/json` ya ordena las claves de un `map[string]…`, pero NO normaliza
// los flotantes: `0.5` y `0.50000000000000001` saldrían distintos y darían dos
// huellas para la misma receta, o sea dos derivadas guardadas de la misma
// imagen. Se redondea a seis decimales, que es más resolución de la que tiene
// cualquier deslizador de la interfaz.
func (r Receta) canonica() ([]byte, error) {
	pasos := make([]map[string]any, 0, len(r.Pasos))
	for _, p := range r.Pasos {
		m := map[string]any{"tipo": string(p.Tipo)}
		if p.Eje != "" {
			m["eje"] = p.Eje
		}
		if len(p.Params) > 0 {
			nombres := make([]string, 0, len(p.Params))
			for n := range p.Params {
				nombres = append(nombres, n)
			}
			sort.Strings(nombres)
			params := make(map[string]string, len(nombres))
			for _, n := range nombres {
				// Como CADENA con seis decimales y no como número: así el
				// redondeo queda hecho en el texto que se va a hashear, y no
				// depende de cómo decida imprimir el flotante quien serialice.
				params[n] = fmt.Sprintf("%.6f", p.Params[n])
			}
			m["params"] = params
		}
		pasos = append(pasos, m)
	}
	return json.Marshal(map[string]any{"version": r.Version, "pasos": pasos})
}

// Huella identifica el RESULTADO de aplicar esta receta. Con ella se nombran las
// derivadas: misma foto y misma huella ⇒ el fichero ya está calculado.
func (r Receta) Huella() (string, error) {
	b, err := r.canonica()
	if err != nil {
		return "", err
	}
	suma := sha256.Sum256(b)
	// Doce hexadecimales: entran en un nombre de fichero, se leen en un log y
	// 48 bits bastan de sobra para lo que esto distingue —las recetas de UNA
	// foto—, que son decenas, no millones.
	return hex.EncodeToString(suma[:])[:12], nil
}

// RecetaVacia es la de una foto sin editar. No es un caso especial: tiene su
// huella como cualquier otra, así que el original también se cachea por el
// mismo camino y no hace falta una rama aparte en el almacén.
func RecetaVacia() Receta { return Receta{Version: VersionReceta, Pasos: []Paso{}} }
