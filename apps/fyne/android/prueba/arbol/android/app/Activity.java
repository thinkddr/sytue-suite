// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.app;

import android.content.Context;
import android.content.Intent;

/** Una Activity de MENTIRA para ArbolDocumentosPrueba: contesta al selector con lo que se le diga. */
public class Activity extends Context {
    public static final int RESULT_OK = -1;
    public static final int RESULT_CANCELED = 0;

    /** Lo que «elige» la persona: el resultado y el Intent con el árbol y sus permisos. */
    public int resultado = RESULT_CANCELED;
    public Intent respuesta;
    /** El Intent con el que se abrió el selector. */
    public Intent pedido;
    public final FragmentManager fm = new FragmentManager(this);

    public FragmentManager getFragmentManager() { return fm; }
}
