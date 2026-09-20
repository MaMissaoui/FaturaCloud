import { useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import {
  App,
  Button,
  Card,
  Checkbox,
  Col,
  DatePicker,
  Divider,
  Form,
  Input,
  InputNumber,
  Layout,
  List,
  Modal,
  Radio,
  Row,
  Select,
  Space,
  Table,
  Tag,
  theme,
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
  CreateCashSale,
  ExportDailyCashMovements,
  ExportLoanStatus,
  GetAccounts,
  GetCashMovementDetails,
  GetDailyCashMovements,
  GetLoanStatus,
} from "src/api";
import type {
  CashMovementDetail,
  CreateCashSaleRequest,
  DailyCashMovementRow,
  LoanStatusRow,
} from "src/api";
import type { Account, Client } from "src/types/models";
import LineItemsTable from "src/components/line-items/table";
import PageHeader from "src/components/page-header";
import { useDatePickerFormat } from "src/utils/date";
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
const { Footer } = Layout;

// A new customer picked in the "New customer" modal below — kept as local
// draft state, not created via a separate API call, until the whole sale is
// submitted. This is what makes CreateCashSale's client+invoice+payment
// creation genuinely atomic (see db/cash_sale.go's CreateCashSale doc
// comment) rather than a client-creation call followed by a separate sale.
interface NewClientDraft {
  name: string;
  phone?: string;
  identity_number?: string;
  iban?: string;
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

// Cash Book: a single fast-entry screen for a walk-in retail counter —
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
  const { message, modal } = App.useApp();
  const dateFormat = useDatePickerFormat();
  const {
    token: { colorBgContainer },
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
  const [openLoansOnly, setOpenLoansOnly] = useState(true);
  const [downloadingLoanPdf, setDownloadingLoanPdf] = useState(false);
  const [downloadingLoanExcel, setDownloadingLoanExcel] = useState(false);

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
    setLoadingDailyMovement(true);
    try {
      const dayMs = utcDayMs(selectedDate);
      const [[row], details] = await Promise.all([
        GetDailyCashMovements(organizationId, registerAccountId, dayMs, dayMs),
        GetCashMovementDetails(organizationId, registerAccountId, dayMs, dayMs),
      ]);
      setDailyMovement(row ?? null);
      setMovementDetails(details);
    } catch (error) {
      console.error("Failed to fetch daily cash movements:", error);
      message.error(t`Failed to load cash register movements`);
      setDailyMovement(null);
      setMovementDetails([]);
    } finally {
      setLoadingDailyMovement(false);
    }
  };

  useEffect(() => {
    refreshDailyMovement();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [organizationId, registerAccountId, selectedDate.valueOf()]);

  const refreshLoanStatus = async () => {
    if (!organizationId) return;
    setLoadingLoanStatus(true);
    try {
      setLoanStatusRows(await GetLoanStatus(organizationId, loanStatusClientId || undefined));
    } catch (error) {
      console.error("Failed to fetch loan status:", error);
      message.error(t`Failed to load loan status`);
      setLoanStatusRows([]);
    } finally {
      setLoadingLoanStatus(false);
    }
  };

  useEffect(() => {
    refreshLoanStatus();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [organizationId, loanStatusClientId]);

  const filteredLoanStatusRows = useMemo(
    () => (openLoansOnly ? loanStatusRows.filter((row) => row.outstanding !== 0) : loanStatusRows),
    [loanStatusRows, openLoansOnly],
  );

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

  const bankAccounts = useMemo(() => accounts.filter((a: any) => a.type === "asset"), [accounts]);
  const expenseAccounts = useMemo(
    () => accounts.filter((a: any) => a.type === "expense"),
    [accounts],
  );
  const withdrawDestination = Form.useWatch("counterAccountType", withdrawForm);

  const openWithdrawModal = () => {
    withdrawForm.resetFields();
    withdrawForm.setFieldsValue({ counterAccountType: "bank" });
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
  const searchResults = useMemo(() => {
    if (!needle) return [];
    return (clients as any[]).filter((c) =>
      [c.name, c.phone, c.identity_number, c.iban].some(
        (field) => field && String(field).toLowerCase().includes(needle),
      ),
    );
  }, [clients, needle]);

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
      phone: phone || undefined,
      identity_number: values.identity_number || undefined,
      iban: values.iban || undefined,
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
  const netCentsFor = (item: any) => {
    const rate = find(taxRates, { id: item?.taxRate });
    return unitsToCents(netFromGross(toNumber(item?.unitPrice) || 0, rate?.percentage ?? 0));
  };
  const taxGroups = useMemo(() => {
    const groups: Record<string, { taxRate: any; subtotal: number; tax: number }> = {};
    ((lineItems || []) as any[]).forEach((item) => {
      const key = item?.taxRate || "";
      const lineNetTotal = multiplyDecimal(
        toNumber(item?.quantity) || 0,
        centsToUnits(netCentsFor(item)),
      );
      if (!groups[key]) {
        groups[key] = { taxRate: find(taxRates, { id: item?.taxRate }), subtotal: 0, tax: 0 };
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
    // Kept off-screen entirely (see the daily-movement panel above, which
    // already surfaces the same missing-config case) but still resolved
    // and sent explicitly, never left for CreateCashSale's own
    // server-side fallback — defaultCashAccountId is wired to Bank in
    // every chart-of-accounts template, and a sale posted there silently
    // never moves the register balance/report.
    const bankAccountId =
      organization?.defaultCashRegisterAccountId || organization?.defaultCashAccountId;
    if (!bankAccountId) {
      message.error(
        t`No cash register account configured — set one in Organization settings before recording a sale`,
      );
      return;
    }
    const totalCents = unitsToCents(total);
    const amountReceivedCents = Math.min(
      unitsToCents(toNumber(values.amountReceived) || 0),
      totalCents,
    );

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
          taxRate: item.taxRate || null,
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
      resetSaleForm();
    } catch (error) {
      console.error("Failed to record sale:", error);
      message.error(error instanceof Error ? error.message : t`Failed to record sale`);
    } finally {
      setSubmitting(false);
    }
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
                  // Enter is the natural motion after typing a phone number
                  // at a counter — auto-select when the search has narrowed
                  // to one customer instead of making the cashier reach for
                  // the mouse.
                  if (searchResults.length === 1) selectClient(searchResults[0]);
                },
              }
            : undefined
        }
        actions={
          isToday && !inSale ? (
            // Deliberately "dashed", not "primary" like every other list
            // page's create button — this screen's real primary action is
            // recording a sale, not adding a customer, so this stays
            // de-emphasized on purpose.
            <Button
              type="dashed"
              icon={<UserAddOutlined />}
              onClick={() => {
                newClientForm.resetFields();
                if (needle && searchResults.length === 0) {
                  newClientForm.setFieldValue("name", search);
                }
                setNewClientModalOpen(true);
              }}
            >
              <Trans>New customer</Trans>
            </Button>
          ) : undefined
        }
      />

      {isToday && !inSale && needle && (
        <List
          dataSource={searchResults}
          locale={{ emptyText: <Trans>No matching customers</Trans> }}
          style={{ marginBottom: 16 }}
          renderItem={(client: any) => (
            <List.Item
              onClick={() => selectClient(client)}
              style={{ cursor: "pointer" }}
              actions={[
                <Button type="link" onClick={() => selectClient(client)} key="select">
                  <Trans>Select</Trans>
                </Button>,
              ]}
            >
              <List.Item.Meta
                title={client.name}
                description={[client.phone, client.identity_number, client.iban]
                  .filter(Boolean)
                  .join(" · ")}
              />
            </List.Item>
          )}
        />
      )}

      {/* Sale-in-progress flow. The cash-register report is hidden while a
      sale is in progress (a standing back-office panel a cashier doesn't
      need mid-transaction); the loan-status report below stays visible,
      prefiltered to this customer's open loans. */}
      {isToday && inSale && (
        <>
          <Space style={{ marginBottom: 16 }}>
            <Button icon={<ArrowLeftOutlined />} onClick={backToSearch}>
              <Trans>Back to search</Trans>
            </Button>
            <Typography.Text strong>
              {clientName}
              {newClientDraft && (
                <Tag color="blue" style={{ marginLeft: 8 }}>
                  <Trans>New customer</Trans>
                </Tag>
              )}
            </Typography.Text>
          </Space>

          <Card size="small" title={<Trans>New sale</Trans>}>
            <Form form={form} layout="vertical" onFinish={handleSubmitSale}>
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
              button itself: that button (in the sticky footer below,
              reachable without scrolling past however many line items
              were added) stays type="primary" like every other primary
              action in the app, so it both respects the organization's
              own brand color and keeps AntD's guaranteed-accessible text
              contrast instead of the solid-green/gold palette's weak
              contrast at this weight. */}
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
          </Card>

          {document.getElementById("footer") &&
            createPortal(
              <Footer
                style={{
                  position: "sticky",
                  bottom: 0,
                  zIndex: 1,
                  padding: 0,
                  background: colorBgContainer,
                  paddingLeft: 16,
                  paddingRight: 16,
                }}
              >
                <Row align="middle" justify="end" style={{ height: 64 }}>
                  <Col>
                    <Button
                      type="primary"
                      onClick={() => form.submit()}
                      loading={submitting}
                      size="large"
                    >
                      {amountReceivedWatched >= total ? (
                        <Trans>Record cash sale</Trans>
                      ) : (
                        <Trans>Record loan sale</Trans>
                      )}
                    </Button>
                  </Col>
                </Row>
              </Footer>,
              // @ts-expect-error - Footer can be null
              document.getElementById("footer"),
            )}
        </>
      )}

      {!inSale && (
        <Card size="small" style={{ marginBottom: 16 }} loading={loadingDailyMovement}>
          <Row justify="space-between" align="middle" style={{ marginBottom: 12 }}>
            <Col>
              <Typography.Text type="secondary">
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
                    <Button onClick={openWithdrawModal}>
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
              <Row gutter={16}>
                <Col xs={12} sm={6}>
                  <Typography.Text type="secondary">
                    <Trans>Opening</Trans>
                  </Typography.Text>
                  <div>
                    <Typography.Title level={5} style={{ margin: 0 }}>
                      {dailyMovement ? money(dailyMovement.opening) : "—"}
                    </Typography.Title>
                  </div>
                </Col>
                <Col xs={12} sm={6}>
                  <Typography.Text type="secondary">
                    <Trans>In</Trans>
                  </Typography.Text>
                  <div>
                    <Typography.Title level={5} style={{ margin: 0 }}>
                      {dailyMovement ? money(dailyMovement.in) : "—"}
                    </Typography.Title>
                  </div>
                </Col>
                <Col xs={12} sm={6}>
                  <Typography.Text type="secondary">
                    <Trans>Out</Trans>
                  </Typography.Text>
                  <div>
                    <Typography.Title level={5} style={{ margin: 0 }}>
                      {dailyMovement ? money(dailyMovement.out) : "—"}
                    </Typography.Title>
                  </div>
                </Col>
                <Col xs={12} sm={6}>
                  <Typography.Text type="secondary">
                    <Trans>Closing</Trans>
                  </Typography.Text>
                  <div>
                    <Typography.Title level={5} style={{ margin: 0 }}>
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
                <Typography.Text type="secondary" style={{ display: "block", marginTop: 4 }}>
                  <Trans>Movements for {dayjs(dailyMovement.date).format(dateFormat)}</Trans>
                </Typography.Text>
              )}
              <Table
                dataSource={movementDetails}
                rowKey="id"
                size="small"
                pagination={false}
                style={{ marginTop: 8 }}
                locale={{ emptyText: <Trans>No movements on this date</Trans> }}
              >
                <Table.Column
                  title={<Trans>Time</Trans>}
                  key="time"
                  width={70}
                  render={(row: CashMovementDetail) => dayjs(row.date).format("HH:mm")}
                />
                <Table.Column
                  title={<Trans>Type</Trans>}
                  key="kind"
                  render={(row: CashMovementDetail) => (
                    <Tag color={movementKindColor(row.kind)}>{movementKindLabel(row.kind)}</Tag>
                  )}
                />
                <Table.Column
                  title={<Trans>Customer</Trans>}
                  key="clientName"
                  render={(row: CashMovementDetail) => row.clientName ?? row.note ?? "—"}
                />
                <Table.Column
                  title={<Trans>Amount</Trans>}
                  key="amount"
                  align="right"
                  render={(row: CashMovementDetail) => (
                    <Typography.Text type={row.direction === "in" ? "success" : "danger"}>
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
        style={{ marginBottom: 16 }}
        loading={loadingLoanStatus}
        extra={
          <Space>
            <Checkbox checked={openLoansOnly} onChange={(e) => setOpenLoansOnly(e.target.checked)}>
              <Trans>Open only</Trans>
            </Checkbox>
            <Select
              value={loanStatusClientId || undefined}
              onChange={(value) => setLoanStatusClientId(value ?? "")}
              placeholder={t`All customers`}
              allowClear
              showSearch
              optionFilterProp="children"
              style={{ minWidth: 220 }}
            >
              {(clients as any[]).map((c) => (
                <Option key={c.id} value={c.id}>
                  {c.name}
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
          rowKey="invoiceId"
          size="small"
          pagination={{ hideOnSinglePage: true, defaultPageSize: 10 }}
          locale={{ emptyText: <Trans>No loan sales</Trans> }}
        >
          <Table.Column title={<Trans>Customer</Trans>} dataIndex="clientName" key="clientName" />
          <Table.Column title={<Trans>Invoice</Trans>} dataIndex="number" key="number" />
          <Table.Column
            title={<Trans>Date</Trans>}
            key="date"
            render={(row: LoanStatusRow) => dayjs(row.date).format(dateFormat)}
          />
          <Table.Column
            title={<Trans>Original</Trans>}
            key="original"
            align="right"
            render={(row: LoanStatusRow) => money(row.original)}
          />
          <Table.Column
            title={<Trans>Paid</Trans>}
            key="paid"
            align="right"
            render={(row: LoanStatusRow) => money(row.paid)}
          />
          <Table.Column
            title={<Trans>Outstanding</Trans>}
            key="outstanding"
            align="right"
            render={(row: LoanStatusRow) => (
              <Typography.Text strong type={row.outstanding > 0 ? "warning" : "success"}>
                {money(row.outstanding)}
              </Typography.Text>
            )}
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
        <Form form={newClientForm} layout="vertical" onFinish={handleNewClientSubmit}>
          <Form.Item
            label={t`Name`}
            name="name"
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <Input autoFocus />
          </Form.Item>
          <Form.Item label={t`Mobile number`} name="phone">
            <Input />
          </Form.Item>
          <Form.Item label={t`Identity number`} name="identity_number">
            <Input />
          </Form.Item>
          <Form.Item label={t`IBAN`} name="iban">
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
            <Select onChange={() => withdrawForm.setFieldValue("counterAccountId", undefined)}>
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
    </>
  );
};

export default CashBook;
