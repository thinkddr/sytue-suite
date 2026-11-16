// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.nio.charset.StandardCharsets;
import org.json.JSONException;
import org.json.JSONStringer;

/** One representation for the Go session identity and storage-scope ledger keys. */
final class SesionContextoTrabajo {
    static String contexto(String emisor, String cliente, String sujeto, String organizacion) throws JSONException {
        if (emisor.isEmpty() || cliente.isEmpty() || sujeto.isEmpty()) {
            throw new SecurityException("Incomplete validated work identity");
        }
        while (emisor.endsWith("/")) { emisor = emisor.substring(0, emisor.length() - 1); }
        return codificacionGo(new JSONStringer().array().value(unicode(emisor)).value(unicode(cliente))
                .value(unicode(sujeto)).value(unicode(organizacion)).endArray().toString());
    }

    static String verificarContexto(String recibido, String emisor, String cliente,
            String sujeto, String organizacion) throws JSONException {
        String canonico = contexto(emisor, cliente, sujeto, organizacion);
        if (recibido.getBytes(StandardCharsets.UTF_8).length > 8192 || !canonico.equals(recibido)) {
            throw new SecurityException("Work context does not match the validated vault session");
        }
        return canonico;
    }

    static String ambito(String tenant, String sujeto, String producto) throws JSONException {
        return codificacionGo(new JSONStringer().object().key("tenant").value(unicode(tenant))
                .key("subject").value(unicode(sujeto)).key("product").value(unicode(producto)).endObject().toString());
    }

    static void verificarAmbito(String recibido, String tenant, String sujeto, String producto) throws JSONException {
        if (!ambito(tenant, sujeto, producto).equals(recibido)) {
            throw new SecurityException("Storage scope is not its canonical Go representation");
        }
    }

    // Android's maintained JSON writer does all JSON serialization. These are
    // only its encoding differences from encoding/json.Marshal's defaults:
    // slash escaping, HTML escaping and the two JavaScript line separators.
    private static String codificacionGo(String json) {
        return json.replace("\\/", "/").replace("<", "\\u003c").replace(">", "\\u003e")
                .replace("&", "\\u0026").replace("\u2028", "\\u2028").replace("\u2029", "\\u2029");
    }

    private static String unicode(String valor) {
        for (int i = 0; i < valor.length(); i++) {
            char c = valor.charAt(i);
            if (Character.isHighSurrogate(c)) {
                if (++i >= valor.length() || !Character.isLowSurrogate(valor.charAt(i))) {
                    throw new SecurityException("Invalid Unicode work identity");
                }
            } else if (Character.isLowSurrogate(c)) {
                throw new SecurityException("Invalid Unicode work identity");
            }
        }
        return valor;
    }

    private SesionContextoTrabajo() {}
}
