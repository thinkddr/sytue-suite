// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.app;

/** De MENTIRA para ArbolDocumentosPrueba: añadir un fragmento lo crea en el acto. */
public class FragmentManager {
    private final Activity a;
    /** Los fragmentos que siguen pegados a la actividad. */
    public int vivos;

    FragmentManager(Activity a) { this.a = a; }

    public FragmentTransaction beginTransaction() { return new FragmentTransaction(a, this); }
}
