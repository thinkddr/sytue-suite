// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.database;

import java.util.ArrayList;
import java.util.List;

/**
 * El MatrixCursor de Android, de MENTIRA para CarretePrueba: filas en memoria con las
 * columnas pedidas. {@link #vivos} cuenta los que no se han cerrado.
 */
public class MatrixCursor implements Cursor {
    public static int vivos;

    private final String[] columnas;
    private final List<Object[]> filas = new ArrayList<>();
    private int pos = -1;
    private boolean cerrado;

    public MatrixCursor(String[] columnas) {
        this.columnas = columnas.clone();
        vivos++;
    }

    public void addRow(Object[] fila) {
        if (fila.length != columnas.length) {
            throw new IllegalArgumentException("la fila no tiene las columnas pedidas");
        }
        filas.add(fila.clone());
    }

    @Override
    public boolean moveToNext() {
        if (pos + 1 >= filas.size()) {
            pos = filas.size();
            return false;
        }
        pos++;
        return true;
    }

    private Object valor(int columna) {
        if (cerrado || pos < 0 || pos >= filas.size()) {
            throw new IllegalStateException("el cursor no está en una fila");
        }
        return filas.get(pos)[columna];
    }

    @Override
    public boolean isNull(int columna) {
        return valor(columna) == null;
    }

    /** Como en SQLite: un texto que no es número da 0. */
    @Override
    public long getLong(int columna) {
        Object v = valor(columna);
        if (v instanceof Number) {
            return ((Number) v).longValue();
        }
        try {
            return v == null ? 0 : Long.parseLong(v.toString());
        } catch (NumberFormatException e) {
            return 0;
        }
    }

    @Override
    public String getString(int columna) {
        Object v = valor(columna);
        return v == null ? null : v.toString();
    }

    @Override
    public void close() {
        if (!cerrado) {
            cerrado = true;
            vivos--;
        }
    }
}
