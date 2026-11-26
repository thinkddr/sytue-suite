// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.app;

import android.content.Intent;
import android.os.Bundle;

/** Un Fragment de MENTIRA para ArbolDocumentosPrueba: el selector contesta en el acto. */
public class Fragment {
    Activity activity;

    public void onCreate(Bundle guardado) {}

    public void onActivityResult(int pedido, int resultado, Intent datos) {}

    public final Activity getActivity() { return activity; }

    public void startActivityForResult(Intent i, int pedido) {
        activity.pedido = i;
        onActivityResult(pedido, activity.resultado, activity.respuesta);
    }
}
