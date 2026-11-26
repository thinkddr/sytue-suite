// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.database;

import java.io.Closeable;

/** El Cursor de Android, de MENTIRA para CarretePrueba: lo que usa el carrete. */
public interface Cursor extends Closeable {
    boolean moveToNext();

    boolean isNull(int columna);

    long getLong(int columna);

    String getString(int columna);

    @Override
    void close();
}
