import { useState, type ReactNode } from "react";
import {
  Alert,
  Button,
  Card,
  Col,
  DatePicker,
  Form,
  Input,
  InputNumber,
  Row,
  Select,
  Space,
  Tag,
  theme,
  Typography,
} from "antd";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import {
  DollarOutlined,
  FieldTimeOutlined,
  FileExcelOutlined,
  FilePdfOutlined,
  UserAddOutlined,
} from "@ant-design/icons";
import dayjs from "dayjs";

import type { LoanStatusRow } from "src/api";
import LineItemsTable from "src/components/line-items/table";
import CustomerSearchResults from "src/components/cash-book/search-results";
import Slip, { useSlipColors } from "src/components/cash-book/slip";
import {
  customerIdentifiers,
  movementKindColor,
  movementKindLabel,
} from "src/components/cash-book/shared";
import type { CashBookState } from "src/components/cash-book/use-cash-book";
import { PAYMENT_METHODS, paymentMethodLabel } from "src/types/payment";
import { unitsToCents } from "src/utils/currency";

const { Option } = Select;

// Open-loan items shown on the customer card before "Show all".
const SLIPS_SHOWN = 4;

const figure = { fontVariantNumeric: "tabular-nums" as const, whiteSpace: "nowrap" as const };

// Cash vs. loan sale, as two large buttons: the loan one takes the slip's
// "owed" tint when selected, so the choice reads the same way as the summary
// slip beside it.
const SaleTypeButtons = ({ cb }: { cb: CashBookState }) => {
  const { token } = theme.useToken();
  const loanColors = useSlipColors("open");
  const { saleMode, setSaleMode } = cb;
  const base = { flex: 1, height: 52, fontSize: 16, fontWeight: 600 };
  return (
    <div role="group" aria-label={t`Sale type`} style={{ display: "flex", gap: 10 }}>
      <Button
        aria-pressed={saleMode === "cash"}
        onClick={() => setSaleMode("cash")}
        icon={<DollarOutlined />}
        style={{
          ...base,
          borderWidth: 2,
          borderColor: saleMode === "cash" ? token.colorPrimary : token.colorBorder,
          color: saleMode === "cash" ? token.colorPrimary : token.colorTextSecondary,
        }}
      >
        <Trans>Cash sale</Trans>
      </Button>
      <Button
        aria-pressed={saleMode === "loan"}
        onClick={() => setSaleMode("loan")}
        icon={<FieldTimeOutlined />}
        style={{
          ...base,
          borderWidth: 2,
          borderColor: saleMode === "loan" ? loanColors.border : token.colorBorder,
          background: saleMode === "loan" ? loanColors.background : undefined,
          color: saleMode === "loan" ? token.colorText : token.colorTextSecondary,
        }}
      >
        <Trans>Loan sale</Trans>
      </Button>
    </div>
  );
};

// What the sale is about to record, on a slip: owed (a balance remains) or
// settled. Driven by the amount actually entered, not the sale type, like
// the current layout's tag. The submit button sits on it.
const SaleSummary = ({ cb }: { cb: CashBookState }) => {
  const { token } = theme.useToken();
  const { total, amountReceivedWatched, saleMode, money, form, submitting, registerAccountId } = cb;
  const received = amountReceivedWatched || 0;
  const owes = total > 0 && received < total;
  const change = received > total ? received - total : 0;
  return (
    <Slip tone={owes ? "open" : "settled"} style={{ padding: "16px 18px 18px" }}>
      <div style={{ display: "flex", justifyContent: "space-between", gap: 12 }}>
        <Typography.Text>
          <Trans>Total</Trans>
        </Typography.Text>
        <Typography.Text style={figure}>{money(unitsToCents(total))}</Typography.Text>
      </div>
      <div style={{ display: "flex", justifyContent: "space-between", gap: 12, marginTop: 6 }}>
        <Typography.Text>
          {saleMode === "cash" ? <Trans>Amount received</Trans> : <Trans>Deposit received</Trans>}
        </Typography.Text>
        <Typography.Text style={figure}>{money(unitsToCents(received))}</Typography.Text>
      </div>
      <div
        style={{
          borderTop: `1px solid ${token.colorBorderSecondary}`,
          marginTop: 12,
          paddingTop: 12,
        }}
      >
        <Typography.Text type="secondary">
          {owes ? (
            <Trans>Balance owing</Trans>
          ) : change > 0 ? (
            <Trans>Change due</Trans>
          ) : total > 0 ? (
            <Trans>Fully settled</Trans>
          ) : (
            // Nothing to settle yet: an empty sale isn't "fully settled".
            <Trans>Total</Trans>
          )}
        </Typography.Text>
        <div
          style={{
            ...figure,
            fontSize: 34,
            fontWeight: 700,
            lineHeight: 1.15,
            color: owes ? token.colorText : token.colorPrimary,
          }}
        >
          {money(unitsToCents(owes ? total - received : change > 0 ? change : total))}
        </div>
      </div>
      <Button
        type="primary"
        size="large"
        block
        style={{ marginTop: 16, height: 52, fontSize: 16 }}
        onClick={() => form.submit()}
        loading={submitting}
        // F101: an amount > 0 posts to the register; with none configured it
        // can't be recorded. A zero-deposit loan sale is still allowed.
        disabled={received > 0 && !registerAccountId}
      >
        {saleMode === "cash" ? <Trans>Record cash sale</Trans> : <Trans>Record loan sale</Trans>}
      </Button>
    </Slip>
  );
};

// The customer being served: who they are, and each item still owed on an
// earlier loan as a slip with its own Collect button (payments settle one
// line at a time).
const CustomerCard = ({ cb }: { cb: CashBookState }) => {
  const {
    selectedClient,
    newClientDraft,
    clientName,
    backToSearch,
    loanStatusRows,
    openPayment,
    isToday,
    money,
    dateFormat,
  } = cb;
  const openLines: LoanStatusRow[] = selectedClient
    ? loanStatusRows.filter((r) => r.clientId === selectedClient.id && r.outstanding > 0)
    : [];
  const owed = openLines.reduce((n, r) => n + r.outstanding, 0);
  // A customer with many open items would push the sale form off screen;
  // show the first few and let the cashier expand the rest.
  const [showAll, setShowAll] = useState(false);
  const shownLines = showAll ? openLines : openLines.slice(0, SLIPS_SHOWN);
  const details: any = selectedClient ?? newClientDraft ?? {};
  return (
    <Card
      size="small"
      className="card-head-wrap"
      title={
        <Space wrap size={8}>
          <span style={{ fontSize: 20, fontWeight: 700 }}>{clientName}</span>
          {newClientDraft && (
            <Tag color="blue" style={{ marginInlineStart: 0 }}>
              <Trans>New customer</Trans>
            </Tag>
          )}
        </Space>
      }
      extra={
        <Button onClick={backToSearch}>
          <Trans>Change customer</Trans>
        </Button>
      }
      style={{ marginBottom: 16 }}
    >
      <Space wrap size={[16, 4]} style={{ color: "inherit" }}>
        {customerIdentifiers(details, (text) => text)}
        {details.guarantor ? (
          <span>
            <Trans>Guarantor</Trans>: {details.guarantor}
          </span>
        ) : null}
      </Space>
      {openLines.length > 0 && (
        <>
          <Typography.Paragraph style={{ margin: "14px 0 10px" }}>
            <Trans>Still owed on earlier loans</Trans>:{" "}
            <Typography.Text strong style={figure}>
              {money(owed)}
            </Typography.Text>
          </Typography.Paragraph>
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fill, minmax(240px, 1fr))",
              gap: 12,
            }}
          >
            {shownLines.map((row) => (
              <Slip key={row.lineId} tone="open">
                <div style={{ display: "flex", justifyContent: "space-between", gap: 8 }}>
                  <Typography.Text strong style={{ whiteSpace: "nowrap" }}>
                    {row.invoiceNumber}
                  </Typography.Text>
                  <Typography.Text type="secondary" style={{ whiteSpace: "nowrap" }}>
                    {dayjs(row.date).format(dateFormat)}
                  </Typography.Text>
                </div>
                <Typography.Text
                  ellipsis={{ tooltip: row.productName }}
                  style={{ display: "block", margin: "4px 0 8px" }}
                >
                  {row.productName || "—"}
                </Typography.Text>
                <div
                  style={{
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "center",
                    gap: 8,
                  }}
                >
                  <Typography.Text strong style={{ ...figure, fontSize: 17 }}>
                    {money(row.outstanding)}
                  </Typography.Text>
                  <Button
                    size="small"
                    disabled={!isToday}
                    onClick={() => openPayment(row)}
                    aria-label={`${t`Collect`} ${row.invoiceNumber} ${row.productName}`}
                  >
                    <Trans>Collect</Trans>
                  </Button>
                </div>
              </Slip>
            ))}
          </div>
          {openLines.length > SLIPS_SHOWN && (
            <Button
              type="link"
              style={{ paddingInline: 0, marginTop: 6 }}
              onClick={() => setShowAll(!showAll)}
            >
              {showAll ? (
                <Trans>Show fewer</Trans>
              ) : (
                <Trans>Show all {openLines.length} items</Trans>
              )}
            </Button>
          )}
        </>
      )}
    </Card>
  );
};

const SaleForm = ({ cb }: { cb: CashBookState }) => {
  const {
    form,
    handleSubmitSale,
    dateFormat,
    registerAccountId,
    defaultTaxRateId,
    sellableProducts,
    products,
    onProductSelect,
    saleMode,
    currency,
    setAmountReceivedTouched,
  } = cb;
  return (
    <Form form={form} layout="vertical" onFinish={handleSubmitSale} scrollToFirstError>
      <Card
        size="small"
        className="card-head-wrap"
        title={
          <span style={{ fontSize: 18, fontWeight: 600 }}>
            <Trans>New sale</Trans>
          </span>
        }
        extra={
          // A forward-dated sale would post into a future day the register
          // never shows as today.
          <Form.Item
            name="date"
            noStyle
            rules={[{ required: true, message: t`This field is required!` }]}
          >
            <DatePicker
              aria-label={t`Date`}
              format={dateFormat}
              allowClear={false}
              disabledDate={(d) => d.isAfter(dayjs(), "day")}
            />
          </Form.Item>
        }
      >
        {!registerAccountId && (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 12 }}
            message={
              <Trans>
                No cash register account configured — set defaultCashRegisterAccountId in
                Organization settings → Accounting before recording a sale with an amount received
              </Trans>
            }
          />
        )}
        <LineItemsTable
          defaultNewRow={{ quantity: 1, taxRate: defaultTaxRateId }}
          columns={[
            { kind: "index" },
            {
              kind: "product",
              products: sellableProducts,
              allProducts: products,
              onSelect: onProductSelect,
            },
            { kind: "description", required: true },
            { kind: "quantity" },
            { kind: "unitPrice", label: t`Price (tax incl.)` },
          ]}
        />
        <Row gutter={[24, 16]} style={{ marginTop: 20 }}>
          <Col xs={24} lg={13}>
            <Form.Item label={<Trans>Sale type</Trans>} style={{ marginBottom: 16 }}>
              <SaleTypeButtons cb={cb} />
            </Form.Item>
            <Row gutter={12}>
              <Col xs={24} sm={12}>
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
                >
                  <InputNumber
                    size="large"
                    style={{ width: "100%" }}
                    min={0}
                    precision={2}
                    onChange={() => setAmountReceivedTouched(true)}
                  />
                </Form.Item>
              </Col>
              <Col xs={24} sm={12}>
                <Form.Item label={t`Payment method`} name="paymentMethod">
                  <Select size="large">
                    {PAYMENT_METHODS.map((m) => (
                      <Option key={m} value={m}>
                        {paymentMethodLabel(m)}
                      </Option>
                    ))}
                  </Select>
                </Form.Item>
              </Col>
            </Row>
            <Form.Item label={t`Reference`} name="reference" style={{ marginBottom: 0 }}>
              <Input />
            </Form.Item>
          </Col>
          <Col xs={24} lg={11}>
            <SaleSummary cb={cb} />
          </Col>
        </Row>
      </Card>
    </Form>
  );
};

// The till for the chosen day, kept beside the sale rather than hidden
// during one: the opening/in/out/closing figures, withdraw and exports, and
// the day's movements.
const RegisterPanel = ({ cb }: { cb: CashBookState }) => {
  const { token } = theme.useToken();
  const {
    registerAccountId,
    selectedDate,
    setSelectedDate,
    dateFormat,
    dailyMovement,
    movementDetails,
    loadingDailyMovement,
    isToday,
    openWithdrawModal,
    handleExportDailyMovements,
    downloadingDailyPdf,
    downloadingDailyExcel,
    money,
  } = cb;
  const line = (label: ReactNode, value: string) => (
    <div style={{ display: "flex", justifyContent: "space-between", gap: 12, padding: "3px 0" }}>
      <Typography.Text type="secondary">{label}</Typography.Text>
      <Typography.Text style={figure}>{value}</Typography.Text>
    </div>
  );
  return (
    <Card
      size="small"
      className="card-head-wrap"
      loading={loadingDailyMovement}
      title={
        <span style={{ fontSize: 18, fontWeight: 600 }}>
          <Trans>Cash register</Trans>
        </span>
      }
      extra={
        <DatePicker
          aria-label={t`Date`}
          value={selectedDate}
          onChange={(d) => d && setSelectedDate(d)}
          format={dateFormat}
          allowClear={false}
          disabledDate={(d) => d.isAfter(dayjs(), "day")}
        />
      }
    >
      {registerAccountId ? (
        <>
          {line(<Trans>Opening</Trans>, dailyMovement ? money(dailyMovement.opening) : "—")}
          {line(<Trans>In</Trans>, dailyMovement ? `+${money(dailyMovement.in)}` : "—")}
          {line(<Trans>Out</Trans>, dailyMovement ? `−${money(dailyMovement.out)}` : "—")}
          <div
            style={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "baseline",
              gap: 12,
              borderTop: `1px solid ${token.colorBorderSecondary}`,
              marginTop: 6,
              paddingTop: 8,
            }}
          >
            <Typography.Text strong>
              <Trans>Closing</Trans>
            </Typography.Text>
            <span style={{ ...figure, fontSize: 26, fontWeight: 700, color: token.colorPrimary }}>
              {dailyMovement ? money(dailyMovement.closing) : "—"}
            </span>
          </div>
          <Space wrap style={{ marginTop: 14 }}>
            {isToday && (
              <Button onClick={openWithdrawModal}>
                <Trans>Withdraw</Trans>
              </Button>
            )}
            <Button loading={downloadingDailyPdf} onClick={handleExportDailyMovements("pdf")}>
              <FilePdfOutlined /> PDF
            </Button>
            <Button loading={downloadingDailyExcel} onClick={handleExportDailyMovements("xlsx")}>
              <FileExcelOutlined /> <Trans>Excel</Trans>
            </Button>
          </Space>
          <Typography.Title level={5} style={{ margin: "18px 0 4px" }}>
            {dailyMovement ? (
              <Trans>Movements for {dayjs(dailyMovement.date).format(dateFormat)}</Trans>
            ) : (
              <Trans>Movements</Trans>
            )}
          </Typography.Title>
          {movementDetails.length === 0 ? (
            <Typography.Text type="secondary">
              <Trans>No movements on this date</Trans>
            </Typography.Text>
          ) : (
            <ol
              style={{
                listStyle: "none",
                margin: 0,
                padding: 0,
                maxHeight: 460,
                overflowY: "auto",
              }}
            >
              {movementDetails.map((row) => (
                <li
                  key={row.id}
                  style={{
                    display: "grid",
                    gridTemplateColumns: "44px minmax(0, 1fr) auto",
                    columnGap: 10,
                    padding: "8px 0",
                    borderBottom: `1px solid ${token.colorBorderSecondary}`,
                  }}
                >
                  <Typography.Text type="secondary" style={figure}>
                    {dayjs(row.date).format("HH:mm")}
                  </Typography.Text>
                  <span style={{ minWidth: 0 }}>
                    <Tag color={movementKindColor(row.kind)} style={{ marginInlineEnd: 6 }}>
                      {movementKindLabel(row.kind)}
                    </Tag>
                    <Typography.Text
                      type="secondary"
                      ellipsis
                      style={{ display: "block", marginTop: 2 }}
                    >
                      {[row.invoiceNumber, row.clientName ?? row.note].filter(Boolean).join(" ") ||
                        "—"}
                    </Typography.Text>
                  </span>
                  <Typography.Text
                    strong
                    type={row.direction === "in" ? "success" : "danger"}
                    style={figure}
                  >
                    {row.direction === "in" ? "+" : "−"}
                    {money(row.amount)}
                  </Typography.Text>
                </li>
              ))}
            </ol>
          )}
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
            No cash register account configured — set one in Organization settings to see movements
            here.
          </Trans>
        </Typography.Text>
      )}
    </Card>
  );
};

// The new layout's counter tab: search or serve a customer and ring up the
// sale on the left, the till on the right (stacked on a narrow screen).
const Counter = ({ cb }: { cb: CashBookState }) => {
  const {
    isToday,
    inSale,
    search,
    setSearch,
    needle,
    searchResults,
    visibleSearchResults,
    activeResultIndex,
    activeResultId,
    onSearchKeyDown,
    selectClient,
    openNewClientModal,
    setSelectedDate,
  } = cb;
  return (
    <Row gutter={[16, 16]}>
      <Col xs={24} xl={16} style={{ minWidth: 0 }}>
        {!isToday && (
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message={
              <Trans>
                Viewing past movements, read-only. Switch to today to record a sale, payment, or
                withdrawal.
              </Trans>
            }
            action={
              <Button size="small" onClick={() => setSelectedDate(dayjs())}>
                <Trans>Back to today</Trans>
              </Button>
            }
          />
        )}
        {isToday && !inSale && (
          <>
            <div style={{ display: "flex", gap: 10, flexWrap: "wrap", marginBottom: 12 }}>
              <Input.Search
                size="large"
                style={{ flex: "1 1 280px" }}
                placeholder={t`Search by name, mobile number, IBAN, or identity number`}
                aria-label={t`Search by name, mobile number, IBAN, or identity number`}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                allowClear
                autoFocus
                onSearch={() => {
                  // Enter takes the arrow-key-active row, else the single
                  // remaining match (same as the current layout).
                  if (activeResultIndex >= 0 && visibleSearchResults[activeResultIndex]) {
                    selectClient(visibleSearchResults[activeResultIndex]);
                  } else if (searchResults.length === 1) {
                    selectClient(searchResults[0]);
                  }
                }}
                role="combobox"
                aria-expanded={searchResults.length > 0}
                aria-controls="cash-book-results"
                aria-autocomplete="list"
                aria-activedescendant={activeResultId}
                onKeyDown={onSearchKeyDown}
              />
              <Button
                size="large"
                icon={<UserAddOutlined />}
                onClick={() =>
                  openNewClientModal(needle && searchResults.length === 0 ? search : undefined)
                }
              >
                <Trans>New customer</Trans>
              </Button>
            </div>
            {needle ? (
              <CustomerSearchResults cb={cb} />
            ) : (
              <Typography.Paragraph type="secondary">
                <Trans>
                  Find the customer by name, mobile number, IBAN or identity number, or add a new
                  one, to record a sale or collect a loan payment.
                </Trans>
              </Typography.Paragraph>
            )}
          </>
        )}
        {isToday && inSale && (
          <>
            <CustomerCard cb={cb} />
            <SaleForm cb={cb} />
          </>
        )}
      </Col>
      <Col xs={24} xl={8} style={{ minWidth: 0 }}>
        <RegisterPanel cb={cb} />
      </Col>
    </Row>
  );
};

export default Counter;
