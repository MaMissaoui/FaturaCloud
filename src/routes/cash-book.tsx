import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
} from "react";
import {
  Alert,
  App,
  Button,
  Card,
  Checkbox,
  Col,
  DatePicker,
  Divider,
  Empty,
  Form,
  Grid,
  Input,
  InputNumber,
  Modal,
  Radio,
  Row,
  Select,
  Space,
  Table,
  Tag,
  theme,
  Tooltip,
  Typography,
} from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import {
  ArrowLeftOutlined,
  DollarOutlined,
  FieldTimeOutlined,
  FileExcelOutlined,
  FilePdfOutlined,
  UserAddOutlined,
  WalletOutlined,
} from "@ant-design/icons";
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
import type { Account, Client, Payment } from "src/types/models";
import LineItemsTable from "src/components/line-items/table";
import PageHeader from "src/components/page-header";
import { useDatePickerFormat } from "src/utils/date";
import { searchClients } from "src/utils/client-search";
import { dateSorter, moneySorter, numberSorter, textSorter } from "src/utils/sort";
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
import { PAYMENT_METHODS, paymentMethodLabel } from "src/types/payment";

const { Option } = Select;
const { TextArea } = Input;

// A new customer picked in the "New customer" modal below — kept as local
// draft state, not created via a separate API call, until the whole sale is
// submitted. This is what makes CreateCashSale's client+invoice+payment
// creation genuinely atomic (see db/cash_sale.go's CreateCashSale doc
// comment) rather than a client-creation call followed by a separate sale.
interface NewClientDraft {
  name: string;
  // Mandatory, like the modal field (a walk-in is identified by phone).
  phone: string;
  phone2?: string;
  phone3?: string;
  address?: string;
  identity_number?: string;
  iban?: string;
  guarantor?: string;
}

// Labels/colors for CashMovementDetail.kind — see
// db/cash_movement_details.go's CashMovementDetail doc comment for what
// each one means and why it's computed from payment history, not the
// invoice's current state.
const movementKindLabel = (kind: CashMovementDetail["kind"]) => {
  switch (kind) {
    case "sale":
      return t`Sale`;
    case "loan":
      return t`Loan (deposit)`;
    case "repayment":
      return t`Loan repayment`;
    case "withdrawal":
      return t`Withdrawal`;
  }
};
const movementKindColor = (kind: CashMovementDetail["kind"]) => {
  switch (kind) {
    case "sale":
      return "green";
    case "loan":
      return "gold";
    case "repayment":
      return "blue";
    case "withdrawal":
      return "default";
  }
};

// One-line customer summary — the same identifying fields the search results
// show (customer no., phone, CIN, IBAN, address). Shared by the search list
// and the loan-status customer filter so a cashier can tell two same-named
// customers apart without leaving the report.
const clientDetailLine = (c: any): string => {
  const address =
    c.address ||
    [
      [c.house_number, c.street].filter(Boolean).join(" "),
      [c.postal_code, c.city].filter(Boolean).join(" "),
    ]
      .filter(Boolean)
      .join(", ");
  return [
    c.code ? `${t`Customer no.`} ${c.code}` : null,
    [c.phone, c.phone2, c.phone3].filter(Boolean).join(" / ") || null,
    c.identity_number ? `${t`CIN`} ${c.identity_number}` : null,
    c.iban,
    address || null,
    c.guarantor ? `${t`Guarantor`}: ${c.guarantor}` : null,
  ]
    .filter(Boolean)
    .join(" · ");
};

// One selectable customer in the Cash Book pick-list. A grid rather than
// List.Item.Meta: the name and its identifying fields on the left, a
// fixed-width right rail for the open-loan amount so the figures line up as a
// real column, and a single row height so a screenful holds ~11 results
// instead of 8. A `listbox` option driven from the search field's arrow keys —
// the row itself isn't focusable (the combobox is the single tab stop).
const CashBookCustomerRow = ({
  name,
  meta,
  outstanding,
  moneyText,
  active,
  id,
  ariaLabel,
  title,
  onSelect,
  onHover,
}: {
  name: ReactNode;
  meta: ReactNode;
  outstanding: number;
  moneyText: string;
  active: boolean;
  id?: string;
  ariaLabel: string;
  title?: string;
  onSelect: () => void;
  onHover?: () => void;
}) => {
  const {
    token: {
      colorPrimary,
      colorTextSecondary,
      colorWarningText,
      colorBorderSecondary,
      controlItemBgHover,
      controlItemBgActive,
      borderRadius,
    },
  } = theme.useToken();
  const [hovered, setHovered] = useState(false);

  return (
    <div
      id={id}
      role="option"
      aria-selected={active}
      aria-label={ariaLabel}
      title={title}
      onClick={onSelect}
      onMouseEnter={() => {
        setHovered(true);
        onHover?.();
      }}
      onMouseLeave={() => setHovered(false)}
      style={{
        display: "grid",
        gridTemplateColumns: "minmax(0, 1fr) max-content",
        columnGap: 16,
        alignItems: "center",
        minHeight: 56,
        padding: "8px 12px",
        borderBottom: `1px solid ${colorBorderSecondary}`,
        // The grid's left track is minmax(0, 1fr) so this row's own min-width
        // never lets a long name push the money rail off-screen.
        minWidth: 0,
        borderRadius,
        cursor: "pointer",
        transition: "background-color 120ms ease",
        background: active ? controlItemBgActive : hovered ? controlItemBgHover : undefined,
        outline: active ? `2px solid ${colorPrimary}` : undefined,
        outlineOffset: active ? -2 : undefined,
      }}
    >
      <div style={{ minWidth: 0 }}>
        <div
          style={{
            fontSize: 15,
            fontWeight: 600,
            lineHeight: 1.35,
            whiteSpace: "nowrap",
            overflow: "hidden",
            textOverflow: "ellipsis",
          }}
        >
          {name}
        </div>
        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            gap: 8,
            marginTop: 2,
            fontSize: 13,
            lineHeight: 1.4,
            color: colorTextSecondary,
          }}
        >
          {meta}
        </div>
      </div>
      {/* The rail keeps its width even with nothing to show, so the amounts
          above and below it stay in one column; a zero balance renders
          nothing rather than "0,00". */}
      <div
        style={{
          minWidth: 132,
          display: "flex",
          flexDirection: "column",
          alignItems: "flex-end",
          gap: 2,
        }}
      >
        {outstanding > 0 ? (
          <>
            <span style={{ fontSize: 11, letterSpacing: ".02em", color: colorTextSecondary }}>
              {t`Open loan`}
            </span>
            <span
              style={{
                fontSize: 15,
                fontWeight: 600,
                color: colorWarningText,
                fontVariantNumeric: "tabular-nums",
              }}
            >
              {moneyText}
            </span>
          </>
        ) : null}
      </div>
    </div>
  );
};

// The identifiers that actually tell two same-named customers apart, in
// priority order: the customer number (the organization's own unique key),
// then the mobile number, then the CIN. IBAN/address/guarantor stay available
// through the row's title tooltip rather than competing for the one line.
const customerIdentifiers = (c: any, highlight: (text: string) => ReactNode): ReactNode => (
  <>
    {c.code ? (
      <Typography.Text code style={{ fontSize: 12 }}>
        {String(c.code)}
      </Typography.Text>
    ) : null}
    {[c.phone, c.phone2, c.phone3].filter(Boolean).length ? (
      <span style={{ fontVariantNumeric: "tabular-nums", fontWeight: 500 }}>
        {highlight([c.phone, c.phone2, c.phone3].filter(Boolean).join(" / "))}
      </span>
    ) : null}
    {c.identity_number ? (
      <span>
        {t`CIN`} {highlight(String(c.identity_number))}
      </span>
    ) : null}
  </>
);

// search for a customer by name/mobile/IBAN/identity number, then either
// pay off one of their open (loan sale) invoices or record a new sale.
// Cash vs. loan is an explicit choice (saleMode, the "Sale type" toggle
// below), not inferred from the amount received — what actually gets
// recorded is still whatever CreateCashSale computes from it server-side
// (paid iff amountReceived == total), see the "Amount received"/"Deposit
// received" field below for how the two stay in sync without fighting
// each other.
const CashBook = () => {
  const { i18n } = useLingui();
  // Below md the loan table's pinned "Record payment" goes icon-only, so it
  // doesn't take a third of a phone-width table.
  const compactActions = !Grid.useBreakpoint().md;
  const { message, modal } = App.useApp();
  const dateFormat = useDatePickerFormat();
  const {
    token: { colorSuccess, colorError, colorBorder, colorPrimary, colorTextSecondary },
  } = theme.useToken();

  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const clients = useAtomValue(clientsAtom);
  const setClients = useSetAtom(setClientsAtom);
  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);
  // A component/intermediate isn't sellable — same filter invoices/orders use.
  const sellableProducts = useMemo(
    () => (products as any[]).filter((p) => p.category !== "component"),
    [products],
  );
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

  // Local-calendar comparison, deliberately not UTC — "today" is what the
  // cashier at the counter means by it, and it's what gates whether new
  // sales/payments/withdrawals can be entered at all. A sale rung up very
  // late at a positive UTC offset (e.g. after 23:00 in Tunis, UTC+1) still
  // lands in the *previous* UTC day's row below — DailyCashMovementRow's
  // own documented limitation, a stated edge case this screen doesn't try
  // to correct.
  const isToday = selectedDate.isSame(dayjs(), "day");

  // Maps the picked calendar date straight to that date's UTC midnight —
  // not selectedDate.valueOf() (local midnight), which the server would
  // floor to the *previous* UTC day at any positive UTC offset, silently
  // fetching yesterday's bucket for a panel labeled "today". See
  // db/gl_reports.go's DailyCashMovementRow doc comment.
  const utcDayMs = (d: Dayjs) => Date.UTC(d.year(), d.month(), d.date());

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
      const dayMs = utcDayMs(selectedDate);
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
        utcDayMs(selectedDate),
        format,
      );
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Export failed`);
    } finally {
      setDownloading(false);
    }
  };

  const handleExportLoanStatus = (format: "xlsx" | "pdf") => async () => {
    if (!organizationId) return;
    const setDownloading = format === "xlsx" ? setDownloadingLoanExcel : setDownloadingLoanPdf;
    setDownloading(true);
    try {
      await ExportLoanStatus(
        organizationId,
        format,
        loanStatusClientId || undefined,
        openLoansOnly,
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
  const handleExportPaymentHistory = (format: "xlsx" | "pdf") => async () => {
    if (!organizationId) return;
    const setDownloading =
      format === "xlsx" ? setDownloadingPaymentsExcel : setDownloadingPaymentsPdf;
    setDownloading(true);
    try {
      await ExportPaymentHistory(
        organizationId,
        format,
        selectedClient?.id || loanStatusClientId || undefined,
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
    setLoanStatusClientId(client.id);
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

  // Renders `text` with its first case-insensitive occurrence of `term`
  // highlighted, so a phone/CIN match is obvious rather than something to
  // trust.
  const highlight = (text: string, term: string) => {
    if (!term) return text;
    const at = text.toLowerCase().indexOf(term);
    if (at < 0) return text;
    return (
      <>
        {text.slice(0, at)}
        <Typography.Text style={{ color: colorPrimary, fontWeight: 600 }}>
          {text.slice(at, at + term.length)}
        </Typography.Text>
        {text.slice(at + term.length)}
      </>
    );
  };

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

  // A stronger border than antd's default (`colorBorderSecondary`, which is
  // nearly invisible in both themes) so the stacked Cash Book panels read as
  // distinct cards, plus one consistent gap between them. Shared by the
  // Cash register, New sale and Loan status cards.
  const sectionCardStyle = {
    marginBottom: 16,
    borderColor: colorBorder,
  };
  // The two section cards that use a Card `title` get a larger, bolder
  // heading than antd's default so they stand out on the counter screen.
  const sectionCardStyles = { header: { fontSize: 18, fontWeight: 600 } };

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
        notes: values.notes || undefined,
        ...(selectedClient ? { clientId: selectedClient.id } : { newClient: newClientDraft! }),
      };

      const result = await CreateCashSale(req);
      message.success(
        amountReceivedCents >= totalCents ? t`Cash sale recorded` : t`Loan sale recorded`,
      );
      setSelectedClient(result.client);
      setNewClientDraft(null);
      setLoanStatusClientId(result.client.id);
      await setClients();
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

  const clientName = selectedClient?.name || newClientDraft?.name || "";

  return (
    <>
      <PageHeader
        icon={<WalletOutlined />}
        title={<Trans>Cash Book</Trans>}
        style={{ marginBottom: 16 }}
        search={
          isToday && !inSale
            ? {
                placeholder: t`Search by name, mobile number, IBAN, or identity number`,
                value: search,
                onChange: setSearch,
                allowClear: true,
                autoFocus: true,
                onSearch: () => {
                  // Enter is the natural motion after typing a phone number at
                  // a counter — take the arrow-key-active row if there is one,
                  // else auto-select when the search has narrowed to a single
                  // customer, instead of making the cashier reach for the
                  // mouse.
                  if (activeResultIndex >= 0 && visibleSearchResults[activeResultIndex]) {
                    selectClient(visibleSearchResults[activeResultIndex]);
                  } else if (searchResults.length === 1) {
                    selectClient(searchResults[0]);
                  }
                },
                // The search field is the combobox that drives the result
                // listbox below (WAI-ARIA pattern: one tab stop, arrows move
                // the active option, Enter selects it).
                inputProps: {
                  role: "combobox",
                  "aria-expanded": searchResults.length > 0,
                  "aria-controls": "cash-book-results",
                  "aria-autocomplete": "list",
                  "aria-activedescendant": activeResultId,
                  onKeyDown: onSearchKeyDown,
                  // Wide enough for the placeholder (name, mobile, IBAN or
                  // identity number) on desktop, capped to the screen on a
                  // phone.
                  style: { width: "min(380px, calc(100vw - 48px))" },
                },
              }
            : undefined
        }
        actions={
          isToday && !inSale ? (
            <Button
              type="primary"
              icon={<UserAddOutlined />}
              onClick={() =>
                openNewClientModal(needle && searchResults.length === 0 ? search : undefined)
              }
            >
              <Trans>New customer</Trans>
            </Button>
          ) : undefined
        }
      />

      {isToday && !inSale && needle && (
        <>
          {loanStatusFailed && (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 12 }}
              message={
                <Trans>
                  Open-loan figures couldn't be loaded — any loan shown may be incomplete
                </Trans>
              }
            />
          )}

          <div
            aria-live="polite"
            style={{
              marginBottom: 4,
              fontSize: 13,
              fontWeight: 600,
              color: colorTextSecondary,
            }}
          >
            <Trans>
              {searchResults.length} matches · {debtorCount} with an open loan
            </Trans>
          </div>
          {visibleSearchResults.length === 0 ? (
            <Empty description={t`No matching customers`} style={{ marginBottom: 16 }}>
              <Button
                type="dashed"
                icon={<UserAddOutlined />}
                onClick={() => openNewClientModal(search)}
              >
                {t`Create`} "{search}"
              </Button>
            </Empty>
          ) : (
            <>
              <div
                id="cash-book-results"
                role="listbox"
                aria-label={t`Search results`}
                style={{ marginBottom: hiddenResultCount > 0 ? 4 : 16 }}
              >
                {visibleSearchResults.map((client: any, index: number) => {
                  const openLoan = openLoanByClient.get(client.id) ?? 0;
                  return (
                    <CashBookCustomerRow
                      key={client.id}
                      id={`cash-book-result-${client.id}`}
                      active={index === activeResultIndex}
                      ariaLabel={resultAriaLabel(client)}
                      title={clientDetailLine(client)}
                      name={highlight(client.name, needle)}
                      meta={customerIdentifiers(client, (text) => highlight(text, needle))}
                      outstanding={openLoan}
                      moneyText={money(openLoan)}
                      onSelect={() => selectClient(client)}
                      onHover={() => setActiveResultIndex(index)}
                    />
                  );
                })}
              </div>
              {hiddenResultCount > 0 && (
                <div style={{ marginBottom: 16, fontSize: 12, color: colorTextSecondary }}>
                  <Trans>
                    Showing the first {MAX_SEARCH_RESULTS} — keep typing to narrow{" "}
                    {hiddenResultCount} more
                  </Trans>
                </div>
              )}
            </>
          )}
        </>
      )}

      {/* Sale-in-progress flow. The cash-register report is hidden while a
      sale is in progress (a standing back-office panel a cashier doesn't
      need mid-transaction); the loan-status report below stays visible,
      prefiltered to this customer's open loans. */}
      {isToday &&
        inSale && (
          // Capped so the sale form doesn't stretch across a wide counter
          // screen, but left-aligned rather than centered, so its edge lines
          // up with the full-width Loan status / Payment history cards below.
          <div style={{ maxWidth: 960 }}>
            <Space align="center" size={12} style={{ marginBottom: 16 }}>
              <Button type="primary" icon={<ArrowLeftOutlined />} onClick={backToSearch}>
                <Trans>Back to search</Trans>
              </Button>
              <Typography.Title level={4} style={{ margin: 0 }}>
                {clientName}
              </Typography.Title>
              {newClientDraft && (
                <Tag color="blue" style={{ marginInlineStart: 0 }}>
                  <Trans>New customer</Trans>
                </Tag>
              )}
            </Space>

            <Card
              size="small"
              title={<Trans>New sale</Trans>}
              style={sectionCardStyle}
              styles={sectionCardStyles}
            >
              {!registerAccountId && (
                // F101: every sale that receives an amount posts it to the
                // register/till account; with none configured the server now
                // 409s rather than silently crediting Bank. Surface that here
                // (not just on submit) so the cashier sees the fix before
                // ringing anything up. A zero-deposit loan sale needs no
                // register account and stays recordable.
                <Typography.Text type="warning" style={{ display: "block", marginBottom: 12 }}>
                  <Trans>
                    No cash register account configured — set defaultCashRegisterAccountId in
                    Organization settings → Accounting before recording a sale with an amount
                    received
                  </Trans>
                </Typography.Text>
              )}
              <Form form={form} layout="vertical" onFinish={handleSubmitSale} scrollToFirstError>
                <Row gutter={16}>
                  <Col xs={24} md={8}>
                    <Form.Item
                      label={<Trans>Date</Trans>}
                      name="date"
                      rules={[{ required: true, message: t`This field is required!` }]}
                    >
                      {/* A forward-dated sale would post into a future day's
                    bucket, which the panel above never shows as "today"
                    even after today catches up to it — see utcDayMs. */}
                      <DatePicker
                        style={{ width: "100%" }}
                        format={dateFormat}
                        disabledDate={(d) => d.isAfter(dayjs(), "day")}
                      />
                    </Form.Item>
                  </Col>
                </Row>

                <LineItemsTable
                  defaultNewRow={{
                    quantity: 1,
                    taxRate: get(find(taxRates, { isDefault: 1 }), "id"),
                  }}
                  columns={[
                    { kind: "index" },
                    {
                      kind: "product",
                      products: sellableProducts,
                      allProducts: products,
                      // The dropdown shows the product name (the shared default
                      // also appends its SKU); the closed cell shows the SKU.
                      onSelect: (productId, fieldName, formInstance) => {
                        const product = find(products, { id: productId }) as any;
                        if (product) {
                          const items = formInstance.getFieldValue("lineItems");
                          // A picked product's own tax rate wins when it has
                          // one; otherwise the row keeps whatever default
                          // it already carried (see defaultNewRow above) —
                          // either way, that rate is what grossFromNet needs
                          // to prefill a tax-inclusive price the cashier
                          // never has to compute themselves.
                          const taxRateId = product.taxRateId || items[fieldName]?.taxRate;
                          const rate = find(taxRates, { id: taxRateId });
                          items[fieldName] = {
                            ...items[fieldName],
                            description: product.name,
                            unitPrice: grossFromNet(
                              centsToUnits(product.price ?? 0),
                              rate?.percentage ?? 0,
                            ),
                            ...(product.taxRateId ? { taxRate: product.taxRateId } : {}),
                          };
                          formInstance.setFieldValue("lineItems", [...items]);
                        }
                      },
                    },
                    { kind: "description", required: true },
                    { kind: "quantity" },
                    { kind: "unitPrice", label: t`Price (tax incl.)` },
                  ]}
                />

                {/* Tax is still computed and posted correctly behind the
              scenes (see netCentsFor above) — this screen just never shows
              the subtotal/tax split, since every price is entered
              tax-inclusive and that's the only number a counter sale
              needs to communicate. */}
                <Row justify="end" style={{ marginTop: 8, marginBottom: 16 }}>
                  <Col>
                    <Typography.Title level={4} style={{ margin: 0 }}>
                      <Trans>Total</Trans>: {money(unitsToCents(total))}
                    </Typography.Title>
                  </Col>
                </Row>

                <Divider />

                {/* An explicit choice, not inferred from the amount field —
              see saleMode's own comment above for why the old
              infer-from-a-number design was a trap. Switching modes never
              clears whatever's already in the amount field (see the
              defaulting effect above); it only changes which default this
              field would have started at. */}
                <Form.Item label={<Trans>Sale type</Trans>}>
                  {/* Solid radio buttons, not a Segmented — the selected
                button is filled with the primary color, which is far more
                legible at a glance than a Segmented's subtle raised thumb,
                and the line below states the active mode in words. */}
                  <Radio.Group
                    value={saleMode}
                    onChange={(e) => setSaleMode(e.target.value as "cash" | "loan")}
                    optionType="button"
                    buttonStyle="solid"
                    style={{ width: "100%", display: "flex" }}
                  >
                    <Radio.Button value="cash" style={{ flex: 1, textAlign: "center" }}>
                      <DollarOutlined /> <Trans>Cash sale</Trans>
                    </Radio.Button>
                    <Radio.Button value="loan" style={{ flex: 1, textAlign: "center" }}>
                      <FieldTimeOutlined /> <Trans>Loan sale</Trans>
                    </Radio.Button>
                  </Radio.Group>
                  <Typography.Text type="secondary" style={{ display: "block", marginTop: 8 }}>
                    {saleMode === "cash" ? (
                      <Trans>Cash sale selected — the full amount is collected now.</Trans>
                    ) : (
                      <Trans>Loan sale selected — collect a deposit now, the balance later.</Trans>
                    )}
                  </Typography.Text>
                </Form.Item>

                <Row gutter={16}>
                  <Col xs={24} md={12}>
                    <Form.Item
                      label={
                        saleMode === "cash"
                          ? t`Amount received (${currency})`
                          : t`Deposit received (${currency})`
                      }
                      name="amountReceived"
                      tooltip={
                        saleMode === "cash"
                          ? t`Defaults to the full total. Entering more just records the change given back — the sale itself is still only ever recorded up to the total.`
                          : t`Optional upfront deposit — leave at 0 for a zero-deposit loan. The remaining balance is collected later.`
                      }
                      extra={
                        amountReceivedWatched > total ? (
                          <Typography.Text type="success">
                            <Trans>
                              Change due: {money(unitsToCents(amountReceivedWatched - total))}
                            </Trans>
                          </Typography.Text>
                        ) : amountReceivedWatched < total ? (
                          <Typography.Text type="warning">
                            <Trans>
                              Balance to collect later:{" "}
                              {money(unitsToCents(total - amountReceivedWatched))}
                            </Trans>
                          </Typography.Text>
                        ) : undefined
                      }
                    >
                      {/* No `max`: a cashier routinely receives more than the
                    total (e.g. a round note) and needs change calculated —
                    handleSubmitSale still clamps what's actually recorded as
                    paid on the invoice to the total. */}
                      <InputNumber
                        style={{ width: "100%" }}
                        min={0}
                        precision={2}
                        onChange={() => setAmountReceivedTouched(true)}
                      />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={12}>
                    <Form.Item label={t`Payment method`} name="paymentMethod">
                      <Select>
                        {PAYMENT_METHODS.map((m) => (
                          <Option key={m} value={m}>
                            {paymentMethodLabel(m)}
                          </Option>
                        ))}
                      </Select>
                    </Form.Item>
                  </Col>
                </Row>
                <Row gutter={16}>
                  <Col xs={24} md={12}>
                    <Form.Item label={t`Reference`} name="reference">
                      <Input />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={12}>
                    <Form.Item label={t`Notes`} name="notes">
                      <TextArea rows={1} autoSize />
                    </Form.Item>
                  </Col>
                </Row>

                {/* Deliberately still driven by the actual amount, not
              saleMode — the toggle above sets *intent* (which default the
              amount field starts at), this reads back the *outcome* that's
              about to be recorded, and the two can legitimately diverge
              (e.g. Cash mode with a cashier knowingly letting someone off
              a few coins short). Color, not just label text, distinguishes
              the two outcomes — a misread status is exactly the mistake a
              fast-moving counter screen should make hard to make — but
              it's carried by a Tag (green, matching this app's "paid"
              state; gold, matching "sent", the state an unsettled sale
              lands in), the same movementKindColor-style pattern used in
              the register table above, rather than recoloring the submit
              button itself: that button (at the foot of this card) stays
              type="primary" like every other primary action in the app, so
              it both respects the organization's own brand color and keeps
              AntD's guaranteed-accessible text contrast instead of the
              solid-green/gold palette's weak contrast at this weight. */}
                <Space>
                  <Tag color={amountReceivedWatched >= total ? "green" : "gold"}>
                    {amountReceivedWatched >= total ? (
                      <Trans>Fully settled</Trans>
                    ) : (
                      <Trans>Balance owing</Trans>
                    )}
                  </Tag>
                </Space>
              </Form>

              {/* The submit button sits at the foot of this card rather than in
            a sticky footer portaled to #footer, so it reads as part of the
            sale it records instead of floating below the loan-status report
            that follows. */}
              <Row align="middle" justify="end" style={{ marginTop: 16 }}>
                <Col>
                  <Button
                    type="primary"
                    onClick={() => form.submit()}
                    loading={submitting}
                    size="large"
                    // F101: an amount > 0 means a payment will post to the
                    // register; with none configured it can't be recorded.
                    // A zero-deposit loan sale (amount 0) is still allowed.
                    disabled={amountReceivedWatched > 0 && !registerAccountId}
                  >
                    {saleMode === "cash" ? (
                      <Trans>Record cash sale</Trans>
                    ) : (
                      <Trans>Record loan sale</Trans>
                    )}
                  </Button>
                </Col>
              </Row>
            </Card>
          </div>
        )}

      {!inSale && (
        <Card size="small" style={sectionCardStyle} loading={loadingDailyMovement}>
          <Row justify="space-between" align="middle" style={{ marginBottom: 12 }}>
            <Col>
              <Typography.Text strong style={{ fontSize: 15 }}>
                <Trans>Cash register</Trans>
              </Typography.Text>
              <div>
                <DatePicker
                  value={selectedDate}
                  onChange={(d) => d && setSelectedDate(d)}
                  format={dateFormat}
                  allowClear={false}
                  disabledDate={(d) => d.isAfter(dayjs(), "day")}
                />
              </div>
            </Col>
            {registerAccountId && (
              <Col>
                <Space>
                  {isToday && (
                    <Button type="primary" onClick={openWithdrawModal}>
                      <Trans>Withdraw</Trans>
                    </Button>
                  )}
                  <Button loading={downloadingDailyPdf} onClick={handleExportDailyMovements("pdf")}>
                    <FilePdfOutlined /> PDF
                  </Button>
                  <Button
                    loading={downloadingDailyExcel}
                    onClick={handleExportDailyMovements("xlsx")}
                  >
                    <FileExcelOutlined /> <Trans>Excel</Trans>
                  </Button>
                </Space>
              </Col>
            )}
          </Row>

          {registerAccountId ? (
            <>
              <Row gutter={[16, 16]}>
                <Col xs={12} sm={6}>
                  <Typography.Text type="secondary" style={{ fontSize: 13, fontWeight: 600 }}>
                    <Trans>Opening</Trans>
                  </Typography.Text>
                  <div>
                    <Typography.Title
                      level={4}
                      style={{ margin: 0, fontVariantNumeric: "tabular-nums" }}
                    >
                      {dailyMovement ? money(dailyMovement.opening) : "—"}
                    </Typography.Title>
                  </div>
                </Col>
                <Col xs={12} sm={6}>
                  <Typography.Text type="secondary" style={{ fontSize: 13, fontWeight: 600 }}>
                    <Trans>In</Trans>
                  </Typography.Text>
                  <div>
                    <Typography.Title
                      level={4}
                      style={{
                        margin: 0,
                        fontVariantNumeric: "tabular-nums",
                        color: colorSuccess,
                      }}
                    >
                      {dailyMovement ? money(dailyMovement.in) : "—"}
                    </Typography.Title>
                  </div>
                </Col>
                <Col xs={12} sm={6}>
                  <Typography.Text type="secondary" style={{ fontSize: 13, fontWeight: 600 }}>
                    <Trans>Out</Trans>
                  </Typography.Text>
                  <div>
                    <Typography.Title
                      level={4}
                      style={{ margin: 0, fontVariantNumeric: "tabular-nums", color: colorError }}
                    >
                      {dailyMovement ? money(dailyMovement.out) : "—"}
                    </Typography.Title>
                  </div>
                </Col>
                <Col xs={12} sm={6}>
                  <Typography.Text type="secondary" style={{ fontSize: 13, fontWeight: 600 }}>
                    <Trans>Closing</Trans>
                  </Typography.Text>
                  <div>
                    <Typography.Title
                      level={4}
                      style={{ margin: 0, fontVariantNumeric: "tabular-nums" }}
                    >
                      {dailyMovement ? money(dailyMovement.closing) : "—"}
                    </Typography.Title>
                  </div>
                </Col>
              </Row>
              {/* Labeled from the fetched row's own date, not the picker's
            value — if utcDayMs above ever drifted from the picked
            calendar date, this would visibly disagree with the picker
            instead of silently hiding the mismatch. */}
              {dailyMovement && (
                <Typography.Text
                  type="secondary"
                  style={{ display: "block", marginTop: 8, fontSize: 13 }}
                >
                  <Trans>Movements for {dayjs(dailyMovement.date).format(dateFormat)}</Trans>
                </Typography.Text>
              )}
              <Table
                dataSource={movementDetails}
                rowKey="id"
                size="small"
                // Paginated like the Loan status and Payment history tables
                // below — a busy day's full list otherwise pushed both of
                // them a screen or more down the page.
                scroll={{ x: "max-content" }}
                pagination={{ hideOnSinglePage: true, defaultPageSize: 10 }}
                style={{ marginTop: 8 }}
                locale={{ emptyText: <Trans>No movements on this date</Trans> }}
              >
                <Table.Column
                  title={<Trans>Time</Trans>}
                  key="time"
                  width={70}
                  sorter={dateSorter((row: CashMovementDetail) => row.date)}
                  render={(row: CashMovementDetail) => dayjs(row.date).format("HH:mm")}
                />
                <Table.Column
                  title={<Trans>Type</Trans>}
                  key="kind"
                  sorter={textSorter((row: CashMovementDetail) => movementKindLabel(row.kind))}
                  render={(row: CashMovementDetail) => (
                    <Tag color={movementKindColor(row.kind)}>{movementKindLabel(row.kind)}</Tag>
                  )}
                />
                <Table.Column
                  title={<Trans>Invoice</Trans>}
                  key="invoiceNumber"
                  sorter={textSorter((row: CashMovementDetail) => row.invoiceNumber ?? "")}
                  render={(row: CashMovementDetail) => row.invoiceNumber || "—"}
                />
                <Table.Column
                  title={<Trans>Customer</Trans>}
                  key="clientName"
                  sorter={textSorter((row: CashMovementDetail) => row.clientName ?? row.note ?? "")}
                  render={(row: CashMovementDetail) => row.clientName ?? row.note ?? "—"}
                />
                <Table.Column
                  title={<Trans>Amount</Trans>}
                  key="amount"
                  align="right"
                  sorter={numberSorter((row: CashMovementDetail) => row.amount)}
                  render={(row: CashMovementDetail) => (
                    <Typography.Text
                      strong
                      type={row.direction === "in" ? "success" : "danger"}
                      style={{ fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}
                    >
                      {row.direction === "in" ? "+" : "−"}
                      {money(row.amount)}
                    </Typography.Text>
                  )}
                />
              </Table>
              {!isToday && (
                <Typography.Text type="warning" style={{ display: "block", marginTop: 8 }}>
                  <Trans>
                    Viewing past movements, read-only. Switch to today to record a sale, payment, or
                    withdrawal.
                  </Trans>
                </Typography.Text>
              )}
            </>
          ) : (
            <Typography.Text type="secondary">
              <Trans>
                No cash register account configured — set one in Organization settings to see
                movements here.
              </Trans>
            </Typography.Text>
          )}
        </Card>
      )}

      {/* Not isToday-gated — a standing report of who owes what, not tied
      to whichever day the panel above happens to be showing. */}
      <Card
        size="small"
        title={<Trans>Loan status</Trans>}
        className="card-head-wrap"
        style={sectionCardStyle}
        styles={sectionCardStyles}
        loading={loadingLoanStatus}
        extra={
          <Space wrap>
            <Checkbox checked={openLoansOnly} onChange={(e) => setOpenLoansOnly(e.target.checked)}>
              <Trans>Open only</Trans>
            </Checkbox>
            <Select
              value={loanStatusClientId || undefined}
              onChange={(value) => setLoanStatusClientId(value ?? "")}
              placeholder={t`All customers`}
              aria-label={t`Customer`}
              allowClear
              showSearch
              // The option children are JSX (name + detail line), so the
              // default optionFilterProp="children" can't stringify them —
              // search by the customer's own fields instead.
              filterOption={(input, option) => {
                const c = (clients as any[]).find((x) => x.id === option?.value);
                if (!c) return false;
                const hay = [
                  c.name,
                  c.code,
                  c.phone,
                  c.phone2,
                  c.phone3,
                  c.identity_number,
                  c.iban,
                  c.guarantor,
                ]
                  .filter(Boolean)
                  .join(" ")
                  .toLowerCase();
                return hay.includes(input.toLowerCase());
              }}
              style={{ width: 260, maxWidth: "calc(100vw - 72px)" }}
              popupMatchSelectWidth={360}
            >
              {(clients as any[]).map((c) => (
                <Option key={c.id} value={c.id} label={c.name}>
                  <div>{c.name}</div>
                  {clientDetailLine(c) && (
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      {clientDetailLine(c)}
                    </Typography.Text>
                  )}
                </Option>
              ))}
            </Select>
            <Button loading={downloadingLoanPdf} onClick={handleExportLoanStatus("pdf")}>
              <FilePdfOutlined /> PDF
            </Button>
            <Button loading={downloadingLoanExcel} onClick={handleExportLoanStatus("xlsx")}>
              <FileExcelOutlined /> <Trans>Excel</Trans>
            </Button>
          </Space>
        }
      >
        <Table
          dataSource={filteredLoanStatusRows}
          rowKey="lineId"
          size="small"
          scroll={{ x: "max-content" }}
          pagination={{ hideOnSinglePage: true, defaultPageSize: 10 }}
          locale={{ emptyText: <Trans>No loan sales</Trans> }}
        >
          <Table.Column
            title={<Trans>Customer</Trans>}
            dataIndex="clientName"
            key="clientName"
            sorter={textSorter((row: LoanStatusRow) => row.clientName)}
          />
          <Table.Column
            title={<Trans>Date</Trans>}
            key="date"
            sorter={dateSorter((row: LoanStatusRow) => row.date)}
            defaultSortOrder="ascend"
            render={(row: LoanStatusRow) => dayjs(row.date).format(dateFormat)}
          />
          <Table.Column
            title={<Trans>Invoice</Trans>}
            key="invoiceNumber"
            sorter={textSorter((row: LoanStatusRow) => row.invoiceNumber)}
            render={(row: LoanStatusRow) => (
              <span style={{ whiteSpace: "nowrap" }}>{row.invoiceNumber || "—"}</span>
            )}
          />
          <Table.Column
            title={<Trans>Product</Trans>}
            key="product"
            sorter={textSorter((row: LoanStatusRow) => row.productName)}
            // Capped so the table fits its card at desktop width: uncapped,
            // the longest "name · SKU" pushed Outstanding and Record payment
            // past the card edge. Truncated inside the cell, since a column
            // width/ellipsis is ignored under scroll.x "max-content"; the
            // full text stays in the hover tooltip.
            render={(row: LoanStatusRow) => {
              const label = row.sku ? `${row.productName} · ${row.sku}` : row.productName || "—";
              return (
                <Tooltip placement="topLeft" title={label}>
                  <span
                    style={{
                      display: "inline-block",
                      maxWidth: 220,
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                      verticalAlign: "bottom",
                    }}
                  >
                    {label}
                  </span>
                </Tooltip>
              );
            }}
          />
          <Table.Column
            title={<Trans>Qty</Trans>}
            key="quantity"
            align="right"
            sorter={numberSorter((row: LoanStatusRow) => row.quantity)}
            render={(row: LoanStatusRow) => row.quantity}
          />
          <Table.Column
            title={<Trans>Amount</Trans>}
            key="amount"
            align="right"
            sorter={moneySorter((row: LoanStatusRow) => row.amount)}
            render={(row: LoanStatusRow) => (
              <span style={{ whiteSpace: "nowrap" }}>{money(row.amount)}</span>
            )}
          />
          <Table.Column
            title={<Trans>Paid</Trans>}
            key="paid"
            align="right"
            sorter={moneySorter((row: LoanStatusRow) => row.paid)}
            render={(row: LoanStatusRow) => (
              <span style={{ whiteSpace: "nowrap" }}>{money(row.paid)}</span>
            )}
          />
          <Table.Column
            title={<Trans>Outstanding</Trans>}
            key="outstanding"
            align="right"
            sorter={moneySorter((row: LoanStatusRow) => row.outstanding)}
            render={(row: LoanStatusRow) => (
              <Typography.Text
                strong
                type={row.outstanding > 0 ? "warning" : "success"}
                style={{ whiteSpace: "nowrap" }}
              >
                {money(row.outstanding)}
              </Typography.Text>
            )}
          />
          <Table.Column
            key="actions"
            align="right"
            // Pinned so the row's main action stays on screen when the
            // product column makes the table wider than its card (it did at
            // 1440px, cutting "Record payment" off at the card edge).
            fixed="right"
            render={(row: LoanStatusRow) =>
              row.outstanding > 0 ? (
                <Tooltip
                  title={
                    !isToday
                      ? t`Switch to today to record a payment`
                      : compactActions
                        ? t`Record payment`
                        : undefined
                  }
                >
                  <Button
                    type="primary"
                    size="small"
                    icon={<DollarOutlined />}
                    disabled={!isToday}
                    onClick={() => openPayment(row)}
                    aria-label={compactActions ? t`Record payment` : undefined}
                  >
                    {!compactActions && <Trans>Record payment</Trans>}
                  </Button>
                </Tooltip>
              ) : null
            }
          />
        </Table>
      </Card>

      {/* Payment history — the counterpart to the loan report above: what has
      actually been collected, newest first, scoped to the customer being
      served when one is selected. */}
      <Card
        size="small"
        title={<Trans>Payment history</Trans>}
        className="card-head-wrap"
        style={sectionCardStyle}
        styles={sectionCardStyles}
        loading={loadingPayments}
        extra={
          <Space wrap>
            <Button loading={downloadingPaymentsPdf} onClick={handleExportPaymentHistory("pdf")}>
              <FilePdfOutlined /> PDF
            </Button>
            <Button loading={downloadingPaymentsExcel} onClick={handleExportPaymentHistory("xlsx")}>
              <FileExcelOutlined /> <Trans>Excel</Trans>
            </Button>
          </Space>
        }
      >
        <Table
          dataSource={paymentHistory}
          rowKey="id"
          size="small"
          scroll={{ x: "max-content" }}
          pagination={{ hideOnSinglePage: true, defaultPageSize: 10 }}
          locale={{ emptyText: <Trans>No payments yet</Trans> }}
        >
          <Table.Column
            title={<Trans>Date</Trans>}
            key="date"
            sorter={dateSorter((p: Payment) => p.date)}
            defaultSortOrder="descend"
            render={(p: Payment) => dayjs(p.date).format(dateFormat)}
          />
          <Table.Column
            title={<Trans>Customer</Trans>}
            key="customer"
            sorter={textSorter(
              (p: Payment) => (p.clientId && clientNameById.get(p.clientId)) || "",
            )}
            render={(p: Payment) => (p.clientId && clientNameById.get(p.clientId)) || "—"}
          />
          <Table.Column
            title={<Trans>Invoice</Trans>}
            key="invoiceNumbers"
            sorter={textSorter((p: Payment) => (p.invoiceNumbers ?? []).join(", "))}
            render={(p: Payment) =>
              p.invoiceNumbers && p.invoiceNumbers.length > 0 ? p.invoiceNumbers.join(", ") : "—"
            }
          />
          <Table.Column
            title={<Trans>Method</Trans>}
            key="method"
            sorter={textSorter((p: Payment) => paymentMethodLabel(p.method))}
            render={(p: Payment) => paymentMethodLabel(p.method)}
          />
          <Table.Column
            title={<Trans>Reference</Trans>}
            key="reference"
            sorter={textSorter((p: Payment) => p.reference ?? "")}
            render={(p: Payment) => p.reference || "—"}
          />
          <Table.Column
            title={<Trans>Amount</Trans>}
            key="amount"
            align="right"
            sorter={moneySorter((p: Payment) => p.amount)}
            render={(p: Payment) => <span style={{ whiteSpace: "nowrap" }}>{money(p.amount)}</span>}
          />
        </Table>
      </Card>

      <Modal
        title={<Trans>New customer</Trans>}
        open={newClientModalOpen}
        onCancel={() => setNewClientModalOpen(false)}
        onOk={() => newClientForm.submit()}
        okText={t`Continue`}
        cancelText={t`Cancel`}
        destroyOnHidden
      >
        <Form
          form={newClientForm}
          layout="vertical"
          onFinish={handleNewClientSubmit}
          scrollToFirstError
        >
          <Form.Item
            label={t`Name`}
            name="name"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <Input autoFocus />
          </Form.Item>
          <Form.Item
            label={t`Mobile number`}
            name="phone"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <Input />
          </Form.Item>
          <Form.Item label={t`Phone 2`} name="phone2">
            <Input />
          </Form.Item>
          <Form.Item label={t`Phone 3`} name="phone3">
            <Input />
          </Form.Item>
          <Form.Item label={t`Address`} name="address">
            <Input />
          </Form.Item>
          <Form.Item label={t`Identity number`} name="identity_number">
            <Input />
          </Form.Item>
          <Form.Item label={t`IBAN`} name="iban">
            <Input />
          </Form.Item>
          <Form.Item label={t`Guarantor`} name="guarantor">
            <Input />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={<Trans>Withdraw from cash register</Trans>}
        open={withdrawModalOpen}
        onCancel={() => setWithdrawModalOpen(false)}
        onOk={() => withdrawForm.submit()}
        confirmLoading={withdrawSubmitting}
        okText={t`Record`}
        cancelText={t`Cancel`}
        destroyOnHidden
      >
        <Form form={withdrawForm} layout="vertical" onFinish={handleWithdrawSubmit}>
          <Form.Item
            label={t`Amount (${currency})`}
            name="amount"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <InputNumber style={{ width: "100%" }} min={0.01} precision={2} autoFocus />
          </Form.Item>
          <Form.Item
            label={t`Destination`}
            name="counterAccountType"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <Select
              onChange={(value) =>
                // Prefill the receiving account from the organization's own
                // defaults: Default cash account (Bank, 1020) for a deposit,
                // Default expense account (5100) for an expense.
                withdrawForm.setFieldValue(
                  "counterAccountId",
                  (value === "expense"
                    ? organization?.defaultExpenseAccountId
                    : organization?.defaultCashAccountId) ?? undefined,
                )
              }
            >
              <Option value="bank">
                <Trans>Deposit to the bank</Trans>
              </Option>
              <Option value="expense">
                <Trans>Spend on an expense (no vendor bill)</Trans>
              </Option>
            </Select>
          </Form.Item>
          <Form.Item
            label={
              withdrawDestination === "expense" ? (
                <Trans>Expense account</Trans>
              ) : (
                <Trans>Bank account</Trans>
              )
            }
            name="counterAccountId"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <Select showSearch optionFilterProp="children">
              {(withdrawDestination === "expense" ? expenseAccounts : bankAccounts).map(
                (a: any) => (
                  <Option key={a.id} value={a.id}>
                    {a.code} — {a.name}
                  </Option>
                ),
              )}
            </Select>
          </Form.Item>
          <Form.Item label={t`Note`} name="note">
            <TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={<Trans>Record payment</Trans>}
        open={!!payingLine}
        onCancel={() => setPayingLine(null)}
        onOk={() => payLineForm.submit()}
        confirmLoading={payingSubmitting}
        okText={t`Record`}
        cancelText={t`Cancel`}
        destroyOnHidden
      >
        {payingLine && (
          <>
            <Typography.Paragraph>
              <Typography.Text strong>{payingLine.clientName}</Typography.Text>
              {payingLine.invoiceNumber && (
                <Typography.Text type="secondary">
                  {" · "}
                  <Trans>Invoice {payingLine.invoiceNumber}</Trans>
                </Typography.Text>
              )}
              <br />
              {payingLine.productName}
              {payingLine.sku ? ` (${payingLine.sku})` : ""} × {payingLine.quantity}
            </Typography.Paragraph>
            <Row gutter={16} style={{ marginBottom: 16 }}>
              <Col span={8}>
                <Typography.Text type="secondary">
                  <Trans>Amount</Trans>
                </Typography.Text>
                <div style={{ whiteSpace: "nowrap" }}>{money(payingLine.amount)}</div>
              </Col>
              <Col span={8}>
                <Typography.Text type="secondary">
                  <Trans>Paid</Trans>
                </Typography.Text>
                <div style={{ whiteSpace: "nowrap" }}>{money(payingLine.paid)}</div>
              </Col>
              <Col span={8}>
                <Typography.Text type="secondary">
                  <Trans>Outstanding</Trans>
                </Typography.Text>
                <div style={{ whiteSpace: "nowrap" }}>
                  <Typography.Text strong>{money(payingLine.outstanding)}</Typography.Text>
                </div>
              </Col>
            </Row>
          </>
        )}
        <Form form={payLineForm} layout="vertical" onFinish={handlePayLineSubmit}>
          <Form.Item
            label={t`Amount received (${currency})`}
            name="amount"
            rules={[
              { required: true, message: t`This field is required!` },
              {
                validator: (_, value) =>
                  payingLine && unitsToCents(toNumber(value) || 0) > payingLine.outstanding
                    ? Promise.reject(
                        new Error(t`Cannot exceed the outstanding balance of this item`),
                      )
                    : Promise.resolve(),
              },
            ]}
          >
            <InputNumber style={{ width: "100%" }} min={0.01} precision={2} autoFocus />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
};

export default CashBook;
