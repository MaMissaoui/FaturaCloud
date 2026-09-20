// Generalizes src/utils/invoice.ts's token engine with {number:N} — a
// zero-padded width no invoice format could express (this app's other five
// document types each hardcoded their own padding in Go: ORD- at 3 digits,
// PO-/DEL-/GR-/PRO- at 4 — see db/document_number.go). Client-side use here
// is preview-only, same as invoice.ts's own generateInvoiceNumber/
// validateInvoiceFormat: the org's own document_number_settings row
// (format + counter) is the actual source of truth, advanced atomically
// server-side on create.

import { i18n } from "@lingui/core";
import { t } from "@lingui/core/macro";

export interface DocumentNumberFormatValidationResult {
  isValid: boolean;
  error?: string;
}

const paddedNumberToken = /^\{number(?::(\d{1,2}))?\}$/;
const plainTokens = new Set(["{year}", "{y}", "{month}", "{m}", "{day}", "{clientCode}"]);

export const validateDocumentNumberFormat = (
  format: string,
): DocumentNumberFormatValidationResult => {
  if (!format || format.trim() === "") {
    return { isValid: false, error: "Number format is required" };
  }

  const matches = format.match(/\{([^}]+)\}/g) || [];
  for (const match of matches) {
    if (plainTokens.has(match) || paddedNumberToken.test(match)) continue;
    const validVariables = `{number}, {number:N}, ${Array.from(plainTokens).join(", ")}`;
    return {
      isValid: false,
      error: t`Invalid variable: ${match}. Valid variables are: ${validVariables}`,
    };
  }

  return { isValid: true };
};

export const generateDocumentNumber = (
  format: string,
  counter: number = 1,
  date: Date = new Date(),
  clientCode: string = "",
): string => {
  if (!format) return "";

  return format.replace(/\{([^}]+)\}/g, (token) => {
    const paddedMatch = paddedNumberToken.exec(token);
    if (paddedMatch) {
      const width = paddedMatch[1] ? parseInt(paddedMatch[1], 10) : 0;
      return width > 0 ? String(counter).padStart(width, "0") : String(counter);
    }
    switch (token) {
      case "{year}":
        return date.getFullYear().toString();
      case "{y}":
        return String(date.getFullYear() % 100).padStart(2, "0");
      case "{month}":
        return String(date.getMonth() + 1).padStart(2, "0");
      case "{m}":
        return date.toLocaleString(i18n.locale || "en", { month: "short" });
      case "{day}":
        return String(date.getDate()).padStart(2, "0");
      case "{clientCode}":
        return clientCode;
      default:
        return token;
    }
  });
};
