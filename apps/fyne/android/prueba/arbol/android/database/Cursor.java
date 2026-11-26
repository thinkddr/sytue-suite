// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.database;

import java.io.Closeable;
import java.util.List;

/** Un cursor de MENTIRA para ArbolDocumentosPrueba, sobre filas en memoria. */
public class Cursor implements Closeable {
    private final List<Object[]> filas;
    private int i = -1;

    public Cursor(List<Object[]> filas) { this.filas = filas; }

    public boolean moveToFirst() {
        i = 0;
        return !filas.isEmpty();
    }

    public boolean moveToNext() { return ++i < filas.size(); }

    public String getString(int c) {
        Object o = filas.get(i)[c];
        return o == null ? null : o.toString();
    }

    public long getLong(int c) { return ((Number) filas.get(i)[c]).longValue(); }

    public boolean isNull(int c) { return filas.get(i)[c] == null; }

    @Override public void close() {}
}
