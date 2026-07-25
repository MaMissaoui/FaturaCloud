// Builds the payload for an EPC069-12 ("GiroCode") SEPA Credit Transfer QR
// code — the format European (especially DACH-region) banking apps scan to
// prefill a bank transfer with the beneficiary's IBAN/BIC, the amount, and a
// reference. SEPA credit transfers are EUR-only, so this only ever applies to
// EUR invoices with an IBAN on the organization.
//
// Field order per the EPC guidelines (LF-separated): Service Tag, Version,
// Character Set, Identification, BIC, Name, IBAN, Amount, Purpose,
// Structured Remittance, Unstructured Remittance, Beneficiary Info. Trailing
// empty fields can be omitted; empty fields before a non-empty one can't be,
// since position (not a delimiter) is what identifies each field.
export interface SepaCreditTransferParams {
  beneficiaryName: string | null | undefined;
  iban: string | null | undefined;
  bic?: string | null;
  currency: string;
  amount: number; // currency units (e.g. euros), not cents
  reference: string;
}

export function buildSepaCreditTransferPayload({
  beneficiaryName,
  iban,
  bic,
  currency,
  amount,
  reference,
}: SepaCreditTransferParams): string | null {
  if (currency !== "EUR") return null;
  if (!iban || !beneficiaryName || !(amount > 0)) return null;

  const cleanIban = iban.replace(/\s+/g, "").toUpperCase();
  const cleanBic = bic ? bic.replace(/\s+/g, "").toUpperCase() : "";
  const name = beneficiaryName.slice(0, 70);
  const amountStr = `EUR${amount.toFixed(2)}`;
  const remittance = reference.slice(0, 140);

  return [
    "BCD",
    "002",
    "1",
    "SCT",
    cleanBic,
    name,
    cleanIban,
    amountStr,
    "",
    "",
    remittance,
  ].join("\n");
}
