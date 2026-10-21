// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
)

// La protección de Calcus al salir a un fichero y al volver de él (ROADMAP-OFFICE
// permisos-1): .xlsx (proteccion_ooxml.go) y .ods (proteccion_ods.go).
//
// AL SALIR no va contraseña ni hash: las exportaciones se guardan y las descarga cualquier
// lector, y un hash se puede atacar sin conexión y sin el límite de intentos del servicio.
// El fichero sale protegido, sin contraseña.
//
// AL ENTRAR, el hash de un .xlsx de Excel (SHA-512 con sal, el mismo esquema que guarda el
// servicio) sí se aprovecha. No cabe en el modelo (que nunca lleva ninguno) ni en la
// operación que siembra el contenido: viaja aparte, en `ImportResult.Hashes`, y el cliente
// lo devuelve en el sobre de protección (`proteccion.hashes`) de esa misma escritura. Para
// no cambiar la firma de todos los conversores, el importador de hoja deja el hash en el
// propio JSON bajo `_claveImportada` y `Import` lo saca de ahí antes de devolver nada.

// proteccionHoja es la protección de una hoja en el modelo (`protect`): se conserva tal cual
// al importar y exportar, sin que el servicio la interprete más allá de lo que escribe.
type proteccionHoja struct {
	Protect json.RawMessage `json:"protect,omitempty"`
}

const claveImportada = "_claveImportada"

// protectConClave es el `protect` de un ámbito importado con su hash (si lo trae) aún dentro.
func protectConClave(protect map[string]any, h *ClaveWire) json.RawMessage {
	if protect == nil {
		return nil
	}
	if h != nil {
		protect[claveImportada] = h
	}
	b, _ := json.Marshal(protect)
	return b
}

// extraerClavesImportadas saca del modelo importado los hashes que el importador dejó en
// `protect._claveImportada` y los devuelve por ámbito. El modelo que se devuelve no lleva
// ninguno; si no había, es el mismo.
func extraerClavesImportadas(content string) (string, map[string]ClaveWire) {
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(content), &m) != nil {
		return content, nil
	}
	claves := map[string]ClaveWire{}
	quitado := false
	saca := func(ambito string, protect json.RawMessage) json.RawMessage {
		pm := map[string]json.RawMessage{}
		if json.Unmarshal(protect, &pm) != nil {
			return protect
		}
		var w ClaveWire
		if raw, ok := pm[claveImportada]; ok {
			if json.Unmarshal(raw, &w) == nil {
				claves[ambito] = w
			}
			delete(pm, claveImportada)
			protect, _ = json.Marshal(pm)
			quitado = true
		}
		return protect
	}
	if p, ok := m["protect"]; ok {
		m["protect"] = saca(AmbitoLibro, p)
	}
	var hojas []map[string]json.RawMessage
	if json.Unmarshal(m["sheets"], &hojas) == nil {
		for _, h := range hojas {
			var nombre string
			_ = json.Unmarshal(h["name"], &nombre)
			if p, ok := h["protect"]; ok {
				h["protect"] = saca(AmbitoHoja(nombre), p)
			}
		}
		m["sheets"], _ = json.Marshal(hojas)
	}
	if !quitado {
		return content, nil
	}
	out, _ := json.Marshal(m)
	return string(out), claves
}
