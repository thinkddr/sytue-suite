// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.os;

/** De MENTIRA para ArbolDocumentosPrueba: el disco de {@link #ruta} mide 1000 bytes y tiene 400 libres. */
public final class StatFs {
    public static String ruta = "/storage/emulated/0";

    public StatFs(String r) {
        if (!r.equals(ruta)) {
            throw new IllegalArgumentException("no hay disco en " + r);
        }
    }

    public long getTotalBytes() { return 1000; }

    public long getAvailableBytes() { return 400; }
}
