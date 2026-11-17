// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.security.MessageDigest;
import java.util.Base64;
import org.json.JSONArray;
import org.json.JSONObject;

/** Uses the same synthetic vectors as auth/android_contexto_test.go. */
public final class SesionContextoTrabajoPrueba {
    private interface Accion { void ejecutar() throws Exception; }
    private static int checks;
    private static void exigir(boolean ok, String mensaje) {
        checks++;
        if (!ok) { throw new AssertionError(mensaje); }
    }
    private static void rechaza(Accion accion, String mensaje) throws Exception {
        try { accion.ejecutar(); } catch (SecurityException esperado) { checks++; return; }
        throw new AssertionError(mensaje);
    }
    private static String verificar(String recibido, JSONObject v) throws Exception {
        return SesionContextoTrabajo.verificarContexto(recibido, v.getString("issuer"), v.getString("client"),
                v.getString("subject"), v.getString("organization"));
    }
    private static String sha256(String texto) throws Exception {
        byte[] hash = MessageDigest.getInstance("SHA-256").digest(texto.getBytes(StandardCharsets.UTF_8));
        StringBuilder resultado = new StringBuilder();
        for (byte b : hash) { resultado.append(String.format("%02x", b & 0xff)); }
        return resultado.toString();
    }
    public static void main(String[] args) throws Exception {
        JSONArray casos = new JSONObject(Files.readString(Path.of(args[0]))).getJSONArray("cases");
        exigir(casos.length() >= 5, "missing cross-runtime vectors");
        for (int i = 0; i < casos.length(); i++) {
            JSONObject v = casos.getJSONObject(i);
            String contexto = v.getString("context");
            exigir(verificar(contexto, v).equals(contexto), "Go context changed: " + v.getString("name"));
            exigir(sha256(verificar(contexto, v)).equals(v.getString("identity_hash")), "Go identity hash changed");
            rechaza(() -> verificar(" " + contexto, v), "whitespace created a second session ledger");
            rechaza(() -> verificar(contexto + " []", v), "trailing JSON created a second session ledger");
            rechaza(() -> verificar(contexto.replace("/", "\\/"), v), "alternate slash encoding created a session ledger");
            rechaza(() -> verificar(new JSONArray(contexto).put(2, "foreign-subject").toString(), v), "foreign subject accepted");
            if (contexto.contains("\\u003c")) {
                rechaza(() -> verificar(contexto.replace("\\u003c", "<"), v), "alternate context HTML escaping created a ledger");
            }

            for (String malo : new String[] {"", "[]", "[null,null,null,null]", "[1,2,3,4]", "[", "x".repeat(8193)}) {
                rechaza(() -> verificar(malo, v), "malformed context accepted");
            }
            JSONObject scope = v.getJSONObject("scope");
            String tenant = scope.getString("tenant"), sujeto = scope.getString("subject"), producto = scope.getString("product");
            if (v.getBoolean("scope_valid")) {
                String raw = SesionContextoTrabajo.ambito(tenant, sujeto, producto);
                String header = "v1." + Base64.getUrlEncoder().withoutPadding().encodeToString(raw.getBytes(StandardCharsets.UTF_8));
                exigir(header.equals(v.getString("scope_header")), "scope header differs from Go");
                SesionContextoTrabajo.verificarAmbito(raw, tenant, sujeto, producto);
                rechaza(() -> SesionContextoTrabajo.verificarAmbito(" " + raw, tenant, sujeto, producto), "scope whitespace created a second ledger");
                String reordered = "{\"product\":\"mail\",\"tenant\":" + JSONObject.quote(tenant) + ",\"subject\":" + JSONObject.quote(sujeto) + "}";
                rechaza(() -> SesionContextoTrabajo.verificarAmbito(reordered, tenant, sujeto, producto), "scope order created a second ledger");
                rechaza(() -> SesionContextoTrabajo.verificarAmbito(raw, tenant, "foreign-subject", producto), "foreign scope subject accepted");
                rechaza(() -> SesionContextoTrabajo.verificarAmbito(raw, tenant + "-other", sujeto, producto), "foreign scope tenant accepted");
                rechaza(() -> SesionContextoTrabajo.verificarAmbito(raw.substring(0, raw.length() - 1) + ",\"product\":\"mail\"}", tenant, sujeto, producto), "duplicate scope field created a ledger");

                if (raw.contains("\\u003c")) {
                    rechaza(() -> SesionContextoTrabajo.verificarAmbito(raw.replace("\\u003c", "<"), tenant, sujeto, producto), "alternate HTML escape created a ledger");
                }
            }
        }
        for (String malo : new String[] {"\ud800", "\udc00", "\ud800x"}) {
            rechaza(() -> SesionContextoTrabajo.contexto("https://id.example", "client", malo, "acme"), "invalid UTF16 session accepted");
            rechaza(() -> SesionContextoTrabajo.ambito("acme", malo, "mail"), "invalid UTF16 scope accepted");
        }
        System.out.println("Canonical Android/Go work context: " + checks + " checks passed");
    }
    private SesionContextoTrabajoPrueba() {}
}
