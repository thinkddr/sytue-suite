// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.graphics;

import java.util.HashMap;
import java.util.Map;

/**
 * Un BitmapFactory de MENTIRA para CarretePrueba: abre lo que la prueba pone en
 * {@link #IMAGENES} (ruta → {ancho, alto}); lo demás, null, como una foto que este Android
 * no sabe abrir.
 */
public final class BitmapFactory {
    public static final Map<String, int[]> IMAGENES = new HashMap<>();

    private BitmapFactory() {}

    public static Bitmap decodeFile(String ruta) {
        int[] lados = IMAGENES.get(ruta);
        return lados == null ? null : new Bitmap(lados[0], lados[1]);
    }
}
