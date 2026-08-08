import { t } from "@lingui/core/macro";

export type PaymentDirection = "inbound" | "outbound";

export type PaymentMethod = "bank_transfer" | "cash" | "card" | "direct_debit" | "check" | "other";

export const PAYMENT_METHODS: PaymentMethod[] = [
  "bank_transfer",
  "cash",
  "card",
  "direct_debit",
  "check",
  "other",
];

// paymentMethodLabel must be called during render (not hoisted to module
// scope) so the returned label follows the currently-active locale.
export function paymentMethodLabel(method: string): string {
  switch (method) {
    case "bank_transfer":
      return t`Bank transfer`;
    case "cash":
      return t`Cash`;
    case "card":
      return t`Card`;
    case "direct_debit":
      return t`Direct debit`;
    case "check":
      return t`Check`;
    case "other":
      return t`Other`;
    default:
      return method;
  }
}

export type PaymentStatus = "posted" | "voided";

export const paymentStatusColor: Record<PaymentStatus, string | undefined> = {
  posted: "green",
  voided: "volcano",
};

export function paymentStatusLabel(status: string): string {
  switch (status) {
    case "posted":
      return t`Posted`;
    case "voided":
      return t`Voided`;
    default:
      return status;
  }
}
