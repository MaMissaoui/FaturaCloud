import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { App, Form, type FormInstance } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import dayjs, { type Dayjs } from "dayjs";
import get from "lodash/get";
import find from "lodash/find";
import map from "lodash/map";
import sum from "lodash/sum";
import toNumber from "lodash/toNumber";
import { organizationAtom, organizationIdAtom } from "src/atoms/organization";
import { clientsAtom, setClientsAtom } from "src/atoms/client";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import { taxRatesAtom, setTaxRatesAtom } from "src/atoms/tax-rate";
import {
  CreateCashMovement,
  CreateCashSalePayment,
  CreateCashSale,
  ExportDailyCashMovements,
  ExportLoanStatus,
  ExportPaymentHistory,
  GetAccounts,
  GetCashMovementDetails,
  GetDailyCashMovements,
  GetLoanStatus,
  GetPayments,
} from "src/api";
import type {
  CashMovementDetail,
  CreateCashSaleRequest,
  DailyCashMovementRow,
  LoanStatusRow,
} from "src/api";
import { normalizeInventoryValuation } from "src/types/inventory-valuation";
import type { Account, Client, Payment } from "src/types/models";
import { calendarDayMs, useDatePickerFormat } from "src/utils/date";
import { searchClients } from "src/utils/client-search";
import {
  addDecimal,
  calculateTax,
  centsToUnits,
  grossFromNet,
  multiplyDecimal,
  netFromGross,
  unitsToCents,
} from "src/utils/currency";
import { formatOrgCents } from "src/utils/currencies";
import type { NewClientDraft } from "src/components/cash-book/shared";

// search for a customer by name/mobile/IBAN/identity number, then either
// pay off one of their open (loan sale) invoices or record a new sale.
// Cash vs. loan is an explicit choice (saleMode, the "Sale type" toggle
// below), not inferred from the amount received — what actually gets
// recorded is still whatever CreateCashSale computes from it server-side
// (paid iff amountReceived == total), see the "Amount received"/"Deposit
// received" field below for how the two stay in sync without fighting
// each other.
interface UseCashBookOptions {
  // Whether serving a customer narrows the loan-status query to them. The
  // stacked layout does (its report then shows only their loans); the
  // two-tab layout keeps every customer's rows for its loan register and
  // narrows on the client side instead.
  scopeLoanStatusToClient?: boolean;
}

// Optional scope for the loan-status / payment-history exports, for a layout
// whose report shows a customer the hook isn't serving (the loan register's
// selected customer). Anything left out falls back to the hook's own state.
export interface CashBookExportScope {
  clientId?: string;
  openOnly?: boolean;
}

export const useCashBook = ({ scopeLoanStatusToClient = true }: UseCashBookOptions = {}) => {
  const { i18n } = useLingui();
  const { message, modal } = App.useApp();
  const dateFormat = useDatePickerFormat();

  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const clients = useAtomValue(clientsAtom);
  const setClients = useSetAtom(setClientsAtom);
  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);
  // A component/intermediate isn't sellable — same filter invoices/orders use.
  // Nor, at the counter, is a serialized stock product: the server refuses it
  // (db/invoice_stock.go — no serial picker here yet; sell it through a
  // delivery), so offering it only surfaced the refusal at checkout (audit
  // F158).
  const sellableProducts = useMemo(
    () =>
      (products as any[]).filter(
        (p) => p.category !== "component" && !(p.stockEnabled === 1 && p.serialized === 1),
      ),
    [products],
  );
  // In perpetual valuation a stock product without a unit cost has no cost
  // basis, and the sale's COGS entry is refused (resolveMovementCost) — warn
  // when it's picked rather than at checkout (audit F158).
  const valuedInventory =
    normalizeInventoryValuation(organization?.inventoryValuation) === "perpetual";
  const taxRates = useAtomValue(taxRatesAtom);
  const setTaxRates = useSetAtom(setTaxRatesAtom);

  const [accounts, setAccounts] = useState<Account[]>([]);
  const [search, setSearch] = useState("");
  const [selectedClient, setSelectedClient] = useState<Client | null>(null);
  const [newClientDraft, setNewClientDraft] = useState<NewClientDraft | null>(null);
  const [newClientModalOpen, setNewClientModalOpen] = useState(false);
  const [newClientForm] = Form.useForm();
  const [amountReceivedTouched, setAmountReceivedTouched] = useState(false);
  // Sale type is an explicit choice, not inferred from whatever happens to
  // be in the amount field — the old design defaulted that field to the
  // full total and only became a loan sale via an active downward edit, so
  // the *safe-looking* default (touch nothing, click submit) was actually
  // the unsafe one: a cashier who forgot to lower it recorded a real debt
  // as collected. Defaulting to "loan" here fails the other direction
  // instead — forgetting to switch to Cash leaves a fully-paid sale
  // looking unpaid in AR, which is visible and correctable rather than
  // silently wrong, and this is a retailer where installment sales on
  // big-ticket items are routine, not the exception.
  const [saleMode, setSaleMode] = useState<"cash" | "loan">("loan");
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm();

  // Register daily-movement panel + withdrawal modal — see
  // db/gl_reports.go's GetDailyCashMovements and db/cash_movement.go's
  // CreateCashMovement doc comments. Opening/In/Out/Closing are a GL
  // derivation ("what the books say"), not a physically-counted till
  // figure — labeled accordingly below rather than as "cash in the
  // drawer", the same scoped-out-till-session boundary this screen has
  // always had.
  const registerAccountId = organization?.defaultCashRegisterAccountId;
  const [selectedDate, setSelectedDate] = useState<Dayjs>(() => dayjs());
  const [dailyMovement, setDailyMovement] = useState<DailyCashMovementRow | null>(null);
  const [movementDetails, setMovementDetails] = useState<CashMovementDetail[]>([]);
  const [loadingDailyMovement, setLoadingDailyMovement] = useState(false);
  const [withdrawModalOpen, setWithdrawModalOpen] = useState(false);
  const [withdrawSubmitting, setWithdrawSubmitting] = useState(false);
  const [withdrawForm] = Form.useForm();
  const [downloadingDailyPdf, setDownloadingDailyPdf] = useState(false);
  const [downloadingDailyExcel, setDownloadingDailyExcel] = useState(false);

  // Loan status — a standing report of who owes what, not scoped to
  // selectedDate/isToday at all (unlike everything else on this screen):
  // it should stay visible and useful while glancing at a past day above.
  // Defaults to open loans only, and is prefiltered to the customer being
  // served while a sale is in progress (see selectClient/handleSubmitSale).
  const [loanStatusClientId, setLoanStatusClientId] = useState<string>("");
  const [loanStatusRows, setLoanStatusRows] = useState<LoanStatusRow[]>([]);
  const [loadingLoanStatus, setLoadingLoanStatus] = useState(false);
  // A failed fetch leaves loanStatusRows empty, which would otherwise render
  // every customer as debt-free on the search list — "no loan" and "we don't
  // know" must not be the same pixels on a counter screen.
  const [loanStatusFailed, setLoanStatusFailed] = useState(false);
  const [openLoansOnly, setOpenLoansOnly] = useState(true);
  // The loan line whose "Record payment" modal is open. Payments settle one
  // line at a time (POST /api/cash-sales/{invoiceId}/payments), capped at
  // that line's outstanding balance.
  const [payingLine, setPayingLine] = useState<LoanStatusRow | null>(null);
  const [payingSubmitting, setPayingSubmitting] = useState(false);
  const [payLineForm] = Form.useForm();
  const [downloadingLoanPdf, setDownloadingLoanPdf] = useState(false);
  const [downloadingLoanExcel, setDownloadingLoanExcel] = useState(false);

  // Payment history — every inbound payment for the organization (or the
  // customer being served), shown in its own card below the loan report.
  const [payments, setPayments] = useState<Payment[]>([]);
  const [loadingPayments, setLoadingPayments] = useState(false);
  const [downloadingPaymentsPdf, setDownloadingPaymentsPdf] = useState(false);
  const [downloadingPaymentsExcel, setDownloadingPaymentsExcel] = useState(false);

  // Guards against an in-flight earlier request overwriting a newer one —
  // rapid date picker changes (daily movement) or customer-filter changes
  // (loan status) could otherwise apply whichever response resolved last,
  // leaving stale rows under a newly-selected date/customer. Same shape as
  // products.tsx/inventory.tsx's debounced-search guards.
  const dailyMovementRequestIdRef = useRef(0);
  const loanStatusRequestIdRef = useRef(0);

  // Local-calendar comparison — "today" is what the cashier at the counter
  // means by it, and it's what gates whether new sales/payments/withdrawals
  // can be entered at all. The server buckets the register by the
  // organization's own days (organizations.timezone), so a sale rung up
  // just after midnight, or backdated with the date picker, lands on the
  // day the cashier sees; an organization with no time zone set still gets
  // UTC days (see db/gl_reports.go's DailyCashMovementRow).
  const isToday = selectedDate.isSame(dayjs(), "day");

  useEffect(() => {
    setClients();
    setProducts();
    setTaxRates();
  }, [setClients, setProducts, setTaxRates]);

  useEffect(() => {
    if (!organizationId) return;
    GetAccounts(organizationId)
      .then((accts) => setAccounts((accts as any[]).filter((a) => !a.isGroup)))
      .catch((error) => console.error("Failed to fetch accounts:", error));
  }, [organizationId]);

  const refreshDailyMovement = async () => {
    if (!organizationId || !registerAccountId) return;
    const requestId = ++dailyMovementRequestIdRef.current;
    setLoadingDailyMovement(true);
    try {
      const dayMs = calendarDayMs(selectedDate);
      const [[row], details] = await Promise.all([
        GetDailyCashMovements(organizationId, registerAccountId, dayMs, dayMs),
        GetCashMovementDetails(organizationId, registerAccountId, dayMs, dayMs),
      ]);
      if (requestId !== dailyMovementRequestIdRef.current) return;
      setDailyMovement(row ?? null);
      setMovementDetails(details);
    } catch (error) {
      if (requestId !== dailyMovementRequestIdRef.current) return;
      console.error("Failed to fetch daily cash movements:", error);
      message.error(t`Failed to load cash register movements`);
      setDailyMovement(null);
      setMovementDetails([]);
    } finally {
      if (requestId === dailyMovementRequestIdRef.current) setLoadingDailyMovement(false);
    }
  };

  useEffect(() => {
    refreshDailyMovement();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [organizationId, registerAccountId, selectedDate.valueOf()]);

  const refreshLoanStatus = async () => {
    if (!organizationId) return;
    const requestId = ++loanStatusRequestIdRef.current;
    setLoadingLoanStatus(true);
    try {
      const rows = await GetLoanStatus(organizationId, loanStatusClientId || undefined);
      if (requestId !== loanStatusRequestIdRef.current) return;
      setLoanStatusRows(rows);
      setLoanStatusFailed(false);
    } catch (error) {
      if (requestId !== loanStatusRequestIdRef.current) return;
      console.error("Failed to fetch loan status:", error);
      message.error(t`Failed to load loan status`);
      setLoanStatusRows([]);
      setLoanStatusFailed(true);
    } finally {
      if (requestId === loanStatusRequestIdRef.current) setLoadingLoanStatus(false);
    }
  };

  useEffect(() => {
    refreshLoanStatus();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [organizationId, loanStatusClientId]);

  const refreshPayments = async () => {
    if (!organizationId) return;
    setLoadingPayments(true);
    try {
      setPayments(await GetPayments(organizationId));
    } catch (error) {
      console.error("Failed to fetch payments:", error);
      message.error(t`Failed to load payment history`);
      setPayments([]);
    } finally {
      setLoadingPayments(false);
    }
  };

  useEffect(() => {
    refreshPayments();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [organizationId]);

  const filteredLoanStatusRows = useMemo(
    () => (openLoansOnly ? loanStatusRows.filter((row) => row.outstanding !== 0) : loanStatusRows),
    [loanStatusRows, openLoansOnly],
  );

  // Per-customer open-loan total, derived from the loan-status rows already
  // fetched for the report below (the screen's standing "who owes what"
  // query, unfiltered by client on the search screen) — so the search result
  // can show what a returning customer still owes without a second request.
  const openLoanByClient = useMemo(() => {
    const totals = new Map<string, number>();
    for (const row of loanStatusRows) {
      if (row.outstanding) {
        totals.set(row.clientId, (totals.get(row.clientId) ?? 0) + row.outstanding);
      }
    }
    return totals;
  }, [loanStatusRows]);

  // Payment history: inbound payments, scoped to the customer being served
  // (or the loan-report filter) when one is selected, newest first (GetPayments
  // already orders by date DESC).
  const clientNameById = useMemo(() => {
    const names = new Map<string, string>();
    for (const c of clients as any[]) names.set(c.id, c.name);
    return names;
  }, [clients]);
  const paymentHistory = useMemo(() => {
    const scopeId = selectedClient?.id || loanStatusClientId || "";
    return payments.filter(
      (p) => p.direction === "inbound" && (!scopeId || p.clientId === scopeId),
    );
  }, [payments, selectedClient, loanStatusClientId]);

  const handleExportDailyMovements = (format: "xlsx" | "pdf") => async () => {
    if (!organizationId || !registerAccountId) return;
    const setDownloading = format === "xlsx" ? setDownloadingDailyExcel : setDownloadingDailyPdf;
    setDownloading(true);
    try {
      await ExportDailyCashMovements(
        organizationId,
        registerAccountId,
        calendarDayMs(selectedDate),
        format,
      );
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Export failed`);
    } finally {
      setDownloading(false);
    }
  };

  const handleExportLoanStatus =
    (format: "xlsx" | "pdf", scope?: CashBookExportScope) => async () => {
      if (!organizationId) return;
      const setDownloading = format === "xlsx" ? setDownloadingLoanExcel : setDownloadingLoanPdf;
      setDownloading(true);
      try {
        await ExportLoanStatus(
          organizationId,
          format,
          (scope ? scope.clientId : loanStatusClientId) || undefined,
          scope?.openOnly ?? openLoansOnly,
        );
      } catch (error) {
        message.error(error instanceof Error ? error.message : t`Export failed`);
      } finally {
        setDownloading(false);
      }
    };

  // Scoped the same way the card's own table is (paymentHistory's scopeId):
  // the customer being served while a sale is in progress, else the loan
  // report's customer filter, else every customer.
  const handleExportPaymentHistory =
    (format: "xlsx" | "pdf", scope?: CashBookExportScope) => async () => {
      if (!organizationId) return;
      const setDownloading =
        format === "xlsx" ? setDownloadingPaymentsExcel : setDownloadingPaymentsPdf;
      setDownloading(true);
      try {
        await ExportPaymentHistory(
          organizationId,
          format,
          (scope ? scope.clientId : selectedClient?.id || loanStatusClientId) || undefined,
        );
      } catch (error) {
        message.error(error instanceof Error ? error.message : t`Export failed`);
      } finally {
        setDownloading(false);
      }
    };

  const bankAccounts = useMemo(() => accounts.filter((a: any) => a.type === "asset"), [accounts]);
  const expenseAccounts = useMemo(
    () => accounts.filter((a: any) => a.type === "expense"),
    [accounts],
  );
  const withdrawDestination = Form.useWatch("counterAccountType", withdrawForm);

  const openWithdrawModal = () => {
    withdrawForm.resetFields();
    // Default the destination to a bank deposit and prefill its receiving
    // account from the organization's Default cash account (the Bank account,
    // code 1020 in the default chart) — the same org-level setting the
    // "Deposit to the bank" picker offers, so the cashier usually just
    // confirms it.
    withdrawForm.setFieldsValue({
      counterAccountType: "bank",
      counterAccountId: organization?.defaultCashAccountId ?? undefined,
    });
    setWithdrawModalOpen(true);
  };

  const handleWithdrawSubmit = async (values: any) => {
    if (!organizationId || !registerAccountId || !isToday) return;
    setWithdrawSubmitting(true);
    try {
      await CreateCashMovement({
        organizationId,
        accountId: registerAccountId,
        date: Date.now(),
        counterAccountType: values.counterAccountType,
        counterAccountId: values.counterAccountId,
        amount: unitsToCents(toNumber(values.amount) || 0),
        note: values.note || undefined,
      });
      message.success(t`Cash movement recorded`);
      setWithdrawModalOpen(false);
      await refreshDailyMovement();
    } catch (error) {
      console.error("Failed to record cash movement:", error);
      message.error(error instanceof Error ? error.message : t`Failed to record cash movement`);
    } finally {
      setWithdrawSubmitting(false);
    }
  };

  const inSale = !!selectedClient || !!newClientDraft;

  const resetSaleForm = () => {
    form.resetFields();
    form.setFieldsValue({
      date: dayjs(),
      paymentMethod: "cash",
      // taxRate is still assigned per line (needed for the net/tax split
      // below and the posted GL entry) even though the screen no longer
      // shows a Tax column — every counter sale silently uses the
      // organization's default tax rate unless a picked product overrides
      // it with its own. An organization with no default tax rate records
      // every Cash Book sale as tax-free; that's a master-data
      // precondition for this screen, not something recoverable here.
      lineItems: [{ quantity: 1, taxRate: get(find(taxRates, { isDefault: 1 }), "id") }],
      amountReceived: 0,
    });
    setAmountReceivedTouched(false);
    setSaleMode("loan");
  };

  const selectClient = (client: Client) => {
    setSelectedClient(client);
    setNewClientDraft(null);
    setSearch("");
    resetSaleForm();
    // Serving this customer: scope the loan-status report to them so the
    // cashier sees what they still owe without re-picking them.
    if (scopeLoanStatusToClient) setLoanStatusClientId(client.id);
  };

  const backToSearch = () => {
    setSelectedClient(null);
    setNewClientDraft(null);
    setLoanStatusClientId("");
    setSearch("");
  };

  const needle = search.trim().toLowerCase();
  const searchResults = useMemo(
    () => searchClients(clients as any[], needle, openLoanByClient),
    [clients, needle, openLoanByClient],
  );

  // A one-character query on a large client book shouldn't render thousands of
  // rows; 25 fills any viewport and the footer says how many were held back.
  const MAX_SEARCH_RESULTS = 25;
  const visibleSearchResults = searchResults.slice(0, MAX_SEARCH_RESULTS);
  const hiddenResultCount = searchResults.length - visibleSearchResults.length;
  const debtorCount = searchResults.reduce(
    (n, c) => n + ((openLoanByClient.get(c.id) ?? 0) > 0 ? 1 : 0),
    0,
  );

  // Active row for the combobox: moved by the search field's ArrowUp/Down and
  // by mouse hover, and read by Enter in `onSearch`. Reset whenever the query
  // changes so the arrows start from the top of the new list.
  const [activeResultIndex, setActiveResultIndex] = useState(-1);
  useEffect(() => {
    setActiveResultIndex(-1);
  }, [needle]);

  const onSearchKeyDown = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (!visibleSearchResults.length) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActiveResultIndex((i) => Math.min(i + 1, visibleSearchResults.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActiveResultIndex((i) => Math.max(i - 1, 0));
    }
  };

  const activeResultId =
    activeResultIndex >= 0 && visibleSearchResults[activeResultIndex]
      ? `cash-book-result-${visibleSearchResults[activeResultIndex].id}`
      : undefined;

  // A row's accessible name: the same facts the row shows, rather than the
  // browser deriving it from the whole cell.
  const resultAriaLabel = (c: any): string => {
    const outstanding = openLoanByClient.get(c.id) ?? 0;
    return [
      c.name,
      c.code ? `${t`Customer no.`} ${c.code}` : null,
      [c.phone, c.phone2, c.phone3].filter(Boolean).join(" / ") || null,
      c.identity_number ? `${t`CIN`} ${c.identity_number}` : null,
      outstanding > 0 ? `${t`Open loan`} ${money(outstanding)}` : null,
    ]
      .filter(Boolean)
      .join(". ");
  };

  const openNewClientModal = (prefillName?: string) => {
    newClientForm.resetFields();
    if (prefillName) newClientForm.setFieldValue("name", prefillName);
    setNewClientModalOpen(true);
  };

  const handleNewClientSubmit = (values: any) => {
    const phone = values.phone?.trim();
    // Client-side duplicate-phone check — the primary UX path. The server
    // repeats this check inside CreateCashSale as a backstop (see its doc
    // comment); this is what avoids splitting a returning walk-in
    // customer's history across two client records.
    const existingByPhone = phone
      ? (clients as any[]).find((c) => c.phone && c.phone === phone)
      : null;
    if (existingByPhone) {
      modal.confirm({
        title: t`A customer with this phone number already exists`,
        content: `${existingByPhone.name || ""} — ${phone}`,
        okText: t`Use this customer`,
        cancelText: t`Cancel`,
        onOk: () => {
          setNewClientModalOpen(false);
          selectClient(existingByPhone);
        },
      });
      return;
    }
    setNewClientModalOpen(false);
    setSelectedClient(null);
    setLoanStatusClientId("");
    setNewClientDraft({
      name: values.name,
      phone,
      phone2: values.phone2?.trim() || undefined,
      phone3: values.phone3?.trim() || undefined,
      address: values.address?.trim() || undefined,
      identity_number: values.identity_number || undefined,
      iban: values.iban || undefined,
      guarantor: values.guarantor?.trim() || undefined,
    });
    resetSaleForm();
  };

  // ---- New sale totals ----
  // Counter prices are entered GROSS (tax-inclusive) — the opposite of
  // every other document's unitPrice, which is net with tax added on top.
  // netCentsFor is the single source of truth for the net price behind a
  // gross-priced line: it rounds to cents once, and every downstream
  // number (the totals below, and handleSubmitSale's payload) is derived
  // from that same integer rather than re-deriving from the unrounded
  // gross/(1+rate) value in more than one place — the two can disagree by
  // a cent once quantity amplifies the sub-cent gap, and
  // db/invoice_totals.go's validateInvoiceTotals requires an exact match.
  const lineItems = Form.useWatch("lineItems", form);
  // The tax rate a line should use: its own (from a picked product or an
  // explicit choice) or, when it has none, the organization's default. This
  // is what makes "the price is gross, the tax is determined in the
  // background" hold even for a line added before tax rates loaded (or a
  // hand-typed line that never went through a product's onSelect) — the net
  // and tax are derived from it here and in handleSubmitSale, so a sale is
  // never silently recorded tax-free just because the line carried no rate.
  const defaultTaxRateId = get(find(taxRates, { isDefault: 1 }), "id");
  const effectiveTaxRateId = (item: any) => item?.taxRate || defaultTaxRateId;
  const netCentsFor = (item: any) => {
    const rate = find(taxRates, { id: effectiveTaxRateId(item) });
    return unitsToCents(netFromGross(toNumber(item?.unitPrice) || 0, rate?.percentage ?? 0));
  };
  const taxGroups = useMemo(() => {
    const groups: Record<string, { taxRate: any; subtotal: number; tax: number }> = {};
    ((lineItems || []) as any[]).forEach((item) => {
      const key = effectiveTaxRateId(item) || "";
      const lineNetTotal = multiplyDecimal(
        toNumber(item?.quantity) || 0,
        centsToUnits(netCentsFor(item)),
      );
      if (!groups[key]) {
        groups[key] = {
          taxRate: find(taxRates, { id: effectiveTaxRateId(item) }),
          subtotal: 0,
          tax: 0,
        };
      }
      groups[key].subtotal = addDecimal(groups[key].subtotal, lineNetTotal);
    });
    Object.values(groups).forEach((g) => {
      g.tax = g.taxRate?.percentage ? calculateTax(g.subtotal, g.taxRate.percentage) : 0;
    });
    return Object.values(groups);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lineItems, taxRates]);
  const subTotal = sum(map(taxGroups, "subtotal"));
  const taxTotal = sum(map(taxGroups, "tax"));
  const total = addDecimal(subTotal, taxTotal);
  const amountReceivedWatched = toNumber(Form.useWatch("amountReceived", form));

  // Defaults the amount field to what each mode naturally means — the full
  // total for a cash sale (kept in sync as line items change, so it's
  // still correct if the cashier never touches the field), zero for a
  // loan sale's deposit — but only ever while the cashier hasn't typed a
  // value themselves. Switching modes never overwrites a value they
  // already entered: a cashier who typed a 300 TND deposit, toggled to
  // Cash to glance at it, and toggled back to Loan must still see 300, not
  // a value silently reset out from under them.
  useEffect(() => {
    if (!amountReceivedTouched) {
      form.setFieldValue("amountReceived", saleMode === "cash" ? total : 0);
    }
  }, [total, saleMode, amountReceivedTouched, form]);

  const currency = organization?.currency || "EUR";
  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);

  const handleSubmitSale = async (values: any) => {
    if (!organizationId || !isToday) return;
    if (!selectedClient && !newClientDraft) {
      message.error(t`Pick or create a customer first`);
      return;
    }
    // F101: the register/till account is the only correct destination for
    // money received at the counter — never defaultCashAccountId, which every
    // chart-of-accounts template wires to Bank. Sent explicitly so a sale
    // can't silently fall back server-side, and required up front (below)
    // whenever an amount is actually being recorded so the cashier gets a
    // clear message instead of a Bank posting. A zero-deposit loan sale has
    // no payment to post and so doesn't need one.
    const totalCents = unitsToCents(total);
    const amountReceivedCents = Math.min(
      unitsToCents(toNumber(values.amountReceived) || 0),
      totalCents,
    );
    const bankAccountId = organization?.defaultCashRegisterAccountId ?? undefined;
    if (amountReceivedCents > 0 && !bankAccountId) {
      message.error(
        t`No cash register account configured — set defaultCashRegisterAccountId in Organization settings → Accounting before recording a sale with an amount received`,
      );
      return;
    }

    setSubmitting(true);
    try {
      const req: CreateCashSaleRequest = {
        organizationId,
        date: values.date.valueOf(),
        currency,
        lineItems: (values.lineItems || []).map((item: any) => ({
          description: item.description || null,
          quantity: item.quantity,
          unitPrice: netCentsFor(item),
          taxRate: effectiveTaxRateId(item) || null,
          productId: item.productId || null,
        })),
        subTotal: unitsToCents(subTotal),
        taxTotal: unitsToCents(taxTotal),
        total: totalCents,
        amountReceived: amountReceivedCents,
        paymentMethod: values.paymentMethod || "cash",
        bankAccountId,
        reference: values.reference || undefined,
        ...(selectedClient ? { clientId: selectedClient.id } : { newClient: newClientDraft! }),
      };

      const result = await CreateCashSale(req);
      message.success(
        amountReceivedCents >= totalCents ? t`Cash sale recorded` : t`Loan sale recorded`,
      );
      setSelectedClient(result.client);
      setNewClientDraft(null);
      if (scopeLoanStatusToClient) setLoanStatusClientId(result.client.id);
      await setClients();
      // The sale took its stock-tracked lines out of stock server-side
      // (db/invoice_stock.go) — refresh so the next pick sees current stock.
      await setProducts();
      await refreshDailyMovement();
      await refreshLoanStatus();
      await refreshPayments();
      resetSaleForm();
    } catch (error) {
      console.error("Failed to record sale:", error);
      message.error(error instanceof Error ? error.message : t`Failed to record sale`);
    } finally {
      setSubmitting(false);
    }
  };

  const openPayment = (row: LoanStatusRow) => {
    payLineForm.setFieldsValue({ amount: centsToUnits(row.outstanding) });
    setPayingLine(row);
  };

  // Records the payment against the one loan line, server-side in a single
  // transaction that also moves the invoice to "paid" once its whole balance
  // clears (db/cash_sale_payment.go) — no follow-up state call from here.
  const handlePayLineSubmit = async (values: any) => {
    if (!payingLine) return;
    setPayingSubmitting(true);
    try {
      const result = await CreateCashSalePayment(payingLine.invoiceId, {
        invoiceLineItemId: payingLine.lineId,
        amount: unitsToCents(toNumber(values.amount) || 0),
        date: Date.now(),
        reference: values.reference || undefined,
      });
      message.success(
        result.invoice.state === "paid"
          ? t`Payment recorded — loan fully settled`
          : t`Payment recorded`,
      );
      setPayingLine(null);
    } catch (error) {
      console.error("Failed to record payment:", error);
      message.error(error instanceof Error ? error.message : t`Failed to record payment`);
      return;
    } finally {
      setPayingSubmitting(false);
    }
    // A payment posts to the cash register as well as against the loan, so
    // refresh the register's movements too — a payment recorded while a sale
    // is in progress (the register card is hidden then, see the !inSale gate)
    // would otherwise stay missing from the movements list afterwards.
    await refreshLoanStatus();
    await refreshDailyMovement();
    await refreshPayments();
  };

  // A product picked on a sale line: warn about stock and cost basis, then
  // prefill the line's description, tax rate and gross (tax-inclusive)
  // price. Shared by every layout's line-items table.
  const onProductSelect = (productId: string, fieldName: number, formInstance: FormInstance) => {
    const product = find(products, { id: productId }) as any;
    if (product?.stockEnabled === 1 && (product.stockQuantity ?? 0) <= 0) {
      // Never blocks the sale (a decision: the counter keeps
      // selling when the records are off) — stock just
      // goes negative, the signal that a count is due.
      const onHand = product.stockQuantity ?? 0;
      message.warning(t`${product.name}: ${onHand} in stock — this sale will take it below zero.`);
    }
    if (valuedInventory && product?.stockEnabled === 1 && product.unitCost == null) {
      message.warning(
        t`${product.name} has no unit cost, so this sale will be refused — give the product a unit cost, or switch the organization's inventory valuation to Quantities only.`,
      );
    }
    if (product) {
      const items = formInstance.getFieldValue("lineItems");
      // A picked product's own tax rate wins when it has
      // one; otherwise the row keeps whatever default
      // it already carried (the table's defaultNewRow) —
      // either way, that rate is what grossFromNet needs
      // to prefill a tax-inclusive price the cashier
      // never has to compute themselves.
      const taxRateId = product.taxRateId || items[fieldName]?.taxRate;
      const rate = find(taxRates, { id: taxRateId });
      items[fieldName] = {
        ...items[fieldName],
        description: product.name,
        unitPrice: grossFromNet(centsToUnits(product.price ?? 0), rate?.percentage ?? 0),
        ...(product.taxRateId ? { taxRate: product.taxRateId } : {}),
      };
      formInstance.setFieldValue("lineItems", [...items]);
    }
  };

  const clientName = selectedClient?.name || newClientDraft?.name || "";

  return {
    onProductSelect,
    dateFormat,
    organization,
    clients,
    products,
    sellableProducts,
    taxRates,
    search,
    setSearch,
    selectedClient,
    newClientDraft,
    newClientModalOpen,
    setNewClientModalOpen,
    newClientForm,
    setAmountReceivedTouched,
    saleMode,
    setSaleMode,
    submitting,
    form,
    registerAccountId,
    selectedDate,
    setSelectedDate,
    dailyMovement,
    movementDetails,
    loadingDailyMovement,
    withdrawModalOpen,
    setWithdrawModalOpen,
    withdrawSubmitting,
    withdrawForm,
    downloadingDailyPdf,
    downloadingDailyExcel,
    loanStatusClientId,
    setLoanStatusClientId,
    loanStatusRows,
    loadingLoanStatus,
    loanStatusFailed,
    openLoansOnly,
    setOpenLoansOnly,
    payingLine,
    setPayingLine,
    payingSubmitting,
    payLineForm,
    downloadingLoanPdf,
    downloadingLoanExcel,
    payments,
    loadingPayments,
    downloadingPaymentsPdf,
    downloadingPaymentsExcel,
    isToday,
    filteredLoanStatusRows,
    openLoanByClient,
    clientNameById,
    paymentHistory,
    handleExportDailyMovements,
    handleExportLoanStatus,
    handleExportPaymentHistory,
    bankAccounts,
    expenseAccounts,
    withdrawDestination,
    openWithdrawModal,
    handleWithdrawSubmit,
    inSale,
    selectClient,
    backToSearch,
    needle,
    searchResults,
    MAX_SEARCH_RESULTS,
    visibleSearchResults,
    hiddenResultCount,
    debtorCount,
    activeResultIndex,
    setActiveResultIndex,
    onSearchKeyDown,
    activeResultId,
    resultAriaLabel,
    openNewClientModal,
    handleNewClientSubmit,
    defaultTaxRateId,
    total,
    amountReceivedWatched,
    currency,
    money,
    handleSubmitSale,
    openPayment,
    handlePayLineSubmit,
    clientName,
  };
};

export type CashBookState = ReturnType<typeof useCashBook>;
