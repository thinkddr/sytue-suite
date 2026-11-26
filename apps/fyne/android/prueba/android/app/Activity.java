// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.app;

import android.content.Context;

/**
 * Una Activity de MENTIRA para CarretePrueba: apunta los permisos que se le piden en
 * {@link #pedidos} y dice si tiene el foco ({@link #foco}).
 */
public class Activity extends Context {
    public String[] pedidos;
    public int peticion;
    public boolean foco = true;

    public void requestPermissions(String[] permisos, int codigo) {
        pedidos = permisos.clone();
        peticion = codigo;
    }

    public boolean hasWindowFocus() {
        return foco;
    }
}
