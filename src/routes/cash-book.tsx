import {
  Alert,
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
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import {
  ArrowLeftOutlined,
  DollarOutlined,
  FieldTimeOutlined,
  FileExcelOutlined,
  FilePdfOutlined,
  UserAddOutlined,
  WalletOutlined,
} from "@ant-design/icons";
import dayjs from "dayjs";
import get from "lodash/get";
import find from "lodash/find";
import type { CashMovementDetail, LoanStatusRow } from "src/api";
import type { Payment } from "src/types/models";
import LineItemsTable from "src/components/line-items/table";
import PageHeader from "src/components/page-header";
import { dateSorter, moneySorter, numberSorter, textSorter } from "src/utils/sort";
import { centsToUnits, grossFromNet, unitsToCents } from "src/utils/currency";
import PaymentProductsCell, {
  paymentProductsLabel,
} from "src/components/payments/payment-products-cell";
import { PAYMENT_METHODS, paymentMethodLabel, paymentRowMethodLabel } from "src/types/payment";
import { useCashBook } from "src/components/cash-book/use-cash-book";
import CashBookModals from "src/components/cash-book/modals";
import {
  CashBookCustomerRow,
  clientDetailLine,
  customerIdentifiers,
  movementKindColor,
  movementKindLabel,
} from "src/components/cash-book/shared";

const { Option } = Select;

// search for a customer by name/mobile/IBAN/identity number, then either
// pay off one of their open (loan sale) invoices or record a new sale.
// Cash vs. loan is an explicit choice (saleMode, the "Sale type" toggle
// below), not inferred from the amount received — what actually gets
// recorded is still whatever CreateCashSale computes from it server-side
// (paid iff amountReceived == total), see the "Amount received"/"Deposit
// received" field below for how the two stay in sync without fighting
// each other.
const CashBook = () => {
  const cb = useCashBook();
  const {
    message,
    dateFormat,
    clients,
    products,
    sellableProducts,
    valuedInventory,
    taxRates,
    search,
    setSearch,
    newClientDraft,
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
    downloadingDailyPdf,
    downloadingDailyExcel,
    loanStatusClientId,
    setLoanStatusClientId,
    loadingLoanStatus,
    loanStatusFailed,
    openLoansOnly,
    setOpenLoansOnly,
    downloadingLoanPdf,
    downloadingLoanExcel,
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
    openWithdrawModal,
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
    total,
    amountReceivedWatched,
    currency,
    money,
    handleSubmitSale,
    openPayment,
    clientName,
  } = cb;
  // Below md the loan table's pinned "Record payment" goes icon-only, so it
  // doesn't take a third of a phone-width table.
  const compactActions = !Grid.useBreakpoint().md;
  const {
    token: { colorSuccess, colorError, colorBorder, colorPrimary, colorTextSecondary },
  } = theme.useToken();
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
                    even after today catches up to it. */}
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
                        if (product?.stockEnabled === 1 && (product.stockQuantity ?? 0) <= 0) {
                          // Never blocks the sale (a decision: the counter keeps
                          // selling when the records are off) — stock just
                          // goes negative, the signal that a count is due.
                          const onHand = product.stockQuantity ?? 0;
                          message.warning(
                            t`${product.name}: ${onHand} in stock — this sale will take it below zero.`,
                          );
                        }
                        if (
                          valuedInventory &&
                          product?.stockEnabled === 1 &&
                          product.unitCost == null
                        ) {
                          message.warning(
                            t`${product.name} has no unit cost, so this sale will be refused — give the product a unit cost, or switch the organization's inventory valuation to Quantities only.`,
                          );
                        }
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
                {/* One free-text field for the payment: Reference is what the
              Payment history table and export show, and it becomes the
              journal entry's reference too. A separate Notes field was
              stored but never shown anywhere on this screen. */}
                <Row gutter={16}>
                  <Col xs={24} md={12}>
                    <Form.Item label={t`Reference`} name="reference">
                      <Input />
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
                {/* Hidden until the sale has a total: with no items, 0 received
              "covers" a 0 total and the tag read "Fully settled" next to
              an empty sale. */}
                {total > 0 && (
                  <Space>
                    <Tag color={amountReceivedWatched >= total ? "green" : "gold"}>
                      {amountReceivedWatched >= total ? (
                        <Trans>Fully settled</Trans>
                      ) : (
                        <Trans>Balance owing</Trans>
                      )}
                    </Tag>
                  </Space>
                )}
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
            value — if calendarDayMs ever drifted from the picked
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
            title={<Trans>Product</Trans>}
            key="products"
            sorter={textSorter((p: Payment) => paymentProductsLabel(p))}
            render={(p: Payment) => <PaymentProductsCell payment={p} money={money} />}
          />
          <Table.Column
            title={<Trans>Method</Trans>}
            key="method"
            sorter={textSorter((p: Payment) => paymentRowMethodLabel(p))}
            render={(p: Payment) => paymentRowMethodLabel(p)}
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
            // Pinned so the amount stays on screen when the table is wider
            // than its card (a long product list at 1280px, or any phone).
            fixed="right"
            sorter={moneySorter((p: Payment) => p.amount)}
            render={(p: Payment) => <span style={{ whiteSpace: "nowrap" }}>{money(p.amount)}</span>}
          />
        </Table>
      </Card>

      <CashBookModals cb={cb} />
    </>
  );
};

export default CashBook;
