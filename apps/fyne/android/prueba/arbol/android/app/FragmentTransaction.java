// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.app;

/** De MENTIRA para ArbolDocumentosPrueba. */
public class FragmentTransaction {
    private final Activity a;
    private final FragmentManager fm;
    private Fragment anadido;
    private Fragment quitado;

    FragmentTransaction(Activity a, FragmentManager fm) {
        this.a = a;
        this.fm = fm;
    }

    public FragmentTransaction add(Fragment f, String etiqueta) {
        anadido = f;
        return this;
    }

    public FragmentTransaction remove(Fragment f) {
        quitado = f;
        return this;
    }

    public int commitAllowingStateLoss() {
        if (anadido != null) {
            fm.vivos++;
            anadido.activity = a;
            anadido.onCreate(null);
        }
        if (quitado != null) {
            fm.vivos--;
        }
        return 0;
    }
}
