// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.content;

import java.util.LinkedHashMap;
import java.util.Map;

/** Unos ContentValues de MENTIRA para CarretePrueba: un mapa de columna a valor. */
public final class ContentValues {
    public final Map<String, Object> valores = new LinkedHashMap<>();

    public void put(String clave, String valor) {
        valores.put(clave, valor);
    }

    public void put(String clave, Integer valor) {
        valores.put(clave, valor);
    }

    public void put(String clave, Long valor) {
        valores.put(clave, valor);
    }
}
