/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
/**
 * Utilidades XML minimas para WebDAV/CalDAV/CardDAV. Usa DOMParser en el
 * navegador; en el servidor (Node) se inyecta un parser compatible (p.ej.
 * @xmldom/xmldom) via setXmlParser. Evita dependencias pesadas.
 */

export interface XmlParser {
  parseFromString: (text: string, type: string) => Document;
}

// DOMParser cumple XmlParser salvo que restringe `type` a DOMParserSupportedType (más
// estrecho que string); como solo lo llamamos con 'application/xml', el cast es seguro.
let parserImpl: XmlParser | null =
  typeof DOMParser !== 'undefined' ? (new DOMParser() as XmlParser) : null;

/** Inyecta un parser XML (entornos server). */
export function setXmlParser(parser: XmlParser): void {
  parserImpl = parser;
}

export function parseXml(text: string): Document {
  if (!parserImpl) {
    throw new Error(
      '@sytue/api-clients: no hay parser XML. Llama a setXmlParser() en el servidor.',
    );
  }
  return parserImpl.parseFromString(text, 'application/xml');
}

/** Texto de la primera coincidencia local-name dentro de un nodo. */
export function firstText(node: Element | Document, localName: string): string | undefined {
  const elements = node.getElementsByTagNameNS('*', localName);
  return elements.length > 0 ? (elements[0]?.textContent ?? undefined) : undefined;
}
