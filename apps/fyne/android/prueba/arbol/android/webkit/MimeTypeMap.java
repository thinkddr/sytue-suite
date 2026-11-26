// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.webkit;

import java.util.Map;

/** De MENTIRA para ArbolDocumentosPrueba: unas pocas extensiones. */
public final class MimeTypeMap {
    private static final MimeTypeMap UNO = new MimeTypeMap();
    private static final Map<String, String> TIPOS = Map.of("txt", "text/plain", "pdf", "application/pdf",
            "docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "jpg", "image/jpeg");

    public static MimeTypeMap getSingleton() { return UNO; }

    public String getMimeTypeFromExtension(String ext) { return TIPOS.get(ext); }

    public String getExtensionFromMimeType(String tipo) {
        for (Map.Entry<String, String> e : TIPOS.entrySet()) {
            if (e.getValue().equals(tipo)) {
                return e.getKey();
            }
        }
        return null;
    }
}
