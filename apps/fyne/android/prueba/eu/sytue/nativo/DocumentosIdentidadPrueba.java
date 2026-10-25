// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.io.FileNotFoundException;

/** Account isolation remains testable without an emulator or real user credentials. */
public final class DocumentosIdentidadPrueba {
    private static int comprobaciones;
    private static void comprobar(boolean valor, String caso) {
        comprobaciones++;
        if (!valor) { throw new AssertionError(caso); }
    }
    private static void rechaza(String espacio, String id) throws Exception {
        try { DocumentosIdentidad.ruta(espacio,id); throw new AssertionError("accepted foreign URI"); }
        catch (FileNotFoundException esperado) { comprobaciones++; }
    }
    public static void main(String[] argumentos) throws Exception {
        String[] cuenta = {"https://id.sytue.eu","sytue-mobile","ana","acme"};
        comprobar(DocumentosIdentidad.permite(cuenta,cuenta,false,true),"same account");
        comprobar(!DocumentosIdentidad.permite(cuenta,cuenta,true,true),"locked");
        comprobar(!DocumentosIdentidad.permite(cuenta,cuenta,false,false),"logout");
        for (int i=0;i<cuenta.length;i++) {
            String[] distinta=cuenta.clone(); distinta[i]="another";
            comprobar(!DocumentosIdentidad.permite(cuenta,distinta,false,true),"changed identity "+i);
            distinta=cuenta.clone(); distinta[i]="";
            comprobar(!DocumentosIdentidad.permite(distinta,distinta,false,true),"missing identity "+i);
        }
        String a="0123456789abcdef0123456789abcdef", b="abcdef0123456789abcdef0123456789";
        for (String ruta:new String[]{"/","/Facturas","/Facturas/enero.pdf","/espacio y acento á.txt"}) {
            String id=DocumentosIdentidad.documento(a,ruta);
            comprobar(DocumentosIdentidad.ruta(a,id).equals(ruta),"round trip "+ruta);
            rechaza(b,id);
            rechaza(a,ruta); // grants from the old path-only provider never adopt a new account
        }
        for (String ruta:new String[]{"/../otro","/a/./b","/a\\b","/a\0b",""}) {
            rechaza(a,a+":"+ruta);
        }
        comprobar(DocumentosIdentidad.documento(a,"/").equals(DocumentosIdentidad.documento(a,"/")),"stable root after update");
        System.out.println("DocumentosIdentidadPrueba: "+comprobaciones+" checks passed");
    }
}
