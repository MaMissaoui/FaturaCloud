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

// paymentRowMethodLabel labels a recorded payment: a loan's paid-to-date
// brought forward from the paper register (origin "opening") wasn't a payment
// taken in the app, so it reads "Opening balance", not its placeholder method.
export function paymentRowMethodLabel(payment: { method: string; origin?: string | null }): string {
  return payment.origin === "opening" ? t`Opening balance` : paymentMethodLabel(payment.method);
}

export const isOpeningPayment = (payment: { origin?: string | null }) =>
  payment.origin === "opening";

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
