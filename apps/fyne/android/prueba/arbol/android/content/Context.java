// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.content;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * Un Context de MENTIRA para ArbolDocumentosPrueba: apunta lo que se lanza. Un Intent con
 * paquete (explícito) solo se lanza si ese paquete está en {@link #instalados} y acepta su
 * tipo; si no, ActivityNotFoundException, como Android.
 */
public class Context {
    public final ContentResolver cr = new ContentResolver();
    public final List<Intent> lanzados = new ArrayList<>();
    /** Paquete instalado → tipos que declara para «Abrir con». */
    public final Map<String, Set<String>> instalados = new HashMap<>();

    public ContentResolver getContentResolver() { return cr; }

    public void startActivity(Intent i) {
        if (i.paquete != null) {
            Set<String> tipos = instalados.get(i.paquete);
            if (tipos == null || !tipos.contains(i.tipo)) {
                throw new ActivityNotFoundException("No Activity found to handle " + i.accion + " in " + i.paquete);
            }
        }
        lanzados.add(i);
    }
}
