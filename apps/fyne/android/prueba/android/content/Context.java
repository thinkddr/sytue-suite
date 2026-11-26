// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.content;

import java.util.HashSet;
import java.util.Set;

/**
 * Un Context de MENTIRA para CarretePrueba (no va en ninguna APK: está en android/prueba).
 * La prueba pone los permisos concedidos en {@link #concedidos} y el servicio del sistema
 * que devuelve getSystemService en {@link #servicio}.
 */
public class Context {
    public final ContentResolver resolver = new ContentResolver();
    public final Set<String> concedidos = new HashSet<>();
    public Object servicio;

    public ContentResolver getContentResolver() {
        return resolver;
    }

    /** 0 (PERMISSION_GRANTED) o -1, como en Android. */
    public int checkSelfPermission(String permiso) {
        return concedidos.contains(permiso) ? 0 : -1;
    }

    public <T> T getSystemService(Class<T> clase) {
        return clase.isInstance(servicio) ? clase.cast(servicio) : null;
    }
}
