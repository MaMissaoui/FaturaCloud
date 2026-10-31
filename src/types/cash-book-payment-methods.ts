import { t } from "@lingui/core/macro";

// Mirrors the server's two supported values (db/organization.go, migration
// 0111). The column is NOT NULL DEFAULT 'cash_only' and every write is
// validated, so anything else is treated as the safe default "cash_only".
export type CashBookPaymentMethods = "cash_only" | "cash_and_bank";

export const CASH_BOOK_PAYMENT_METHODS_CASH_ONLY: CashBookPaymentMethods = "cash_only";
export const CASH_BOOK_PAYMENT_METHODS_CASH_AND_BANK: CashBookPaymentMethods = "cash_and_bank";

export const normalizeCashBookPaymentMethods = (value?: string | null): CashBookPaymentMethods =>
  value === CASH_BOOK_PAYMENT_METHODS_CASH_AND_BANK
    ? CASH_BOOK_PAYMENT_METHODS_CASH_AND_BANK
    : CASH_BOOK_PAYMENT_METHODS_CASH_ONLY;

export const cashBookPaymentMethodsOptions = (): {
  value: CashBookPaymentMethods;
  label: string;
}[] => [
  { value: CASH_BOOK_PAYMENT_METHODS_CASH_ONLY, label: t`Cash only` },
  { value: CASH_BOOK_PAYMENT_METHODS_CASH_AND_BANK, label: t`Cash, card and transfer` },
];

export const cashBookPaymentMethodsLabel = (value?: string | null): string =>
  normalizeCashBookPaymentMethods(value) === CASH_BOOK_PAYMENT_METHODS_CASH_AND_BANK
    ? t`Cash, card and transfer`
    : t`Cash only`;

// The method the Cash Book actually uses: only cash_and_bank offers a choice,
// and cash_only always records cash regardless of any stale form value.
export const effectiveCashBookMethod = (cashAndBank: boolean, method?: string | null): string =>
  cashAndBank && method ? method : "cash";
