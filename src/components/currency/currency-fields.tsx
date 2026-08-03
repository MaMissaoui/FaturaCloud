import { Col, DatePicker, Form, InputNumber, Select } from "antd";
import type { FormInstance } from "antd";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import dayjs from "dayjs";
import map from "lodash/map";

import { GetLastExchangeRate } from "src/api";
import { currencies } from "src/utils/currencies";

const { Option } = Select;

/**
 * True whenever the document's currency differs from the organization's —
 * the only time there's anything to convert, and the only time
 * <ExchangeRateFields> renders anything. Exported so callers can decide
 * whether to render the Row wrapping it at all, rather than rendering a Row
 * that's empty whenever this is false.
 */
export const showExchangeRateFields = (
  currency: string | undefined | null,
  orgCurrency: string,
): currency is string => !!currency && currency !== orgCurrency;

/**
 * The currency picker, split out from <ExchangeRateFields> so a page can
 * place it wherever its own header row has room, instead of always pairing
 * it with the (usually-hidden) rate fields in a dedicated row — a lone
 * Currency field in an otherwise-empty Row is what made it look like it was
 * taking up a lot of space for no reason.
 */
export const CurrencySelect = ({
  form,
  organizationId,
  orgCurrency,
  disabled,
  xl = 4,
}: {
  form: FormInstance;
  organizationId: string | undefined;
  orgCurrency: string;
  disabled?: boolean;
  xl?: number;
}) => (
  <Col xs={24} md={12} xl={xl}>
    <Form.Item
      label={<Trans>Currency</Trans>}
      name="currency"
      rules={[{ required: true, message: t`This field is required!` }]}
    >
      <Select
        disabled={disabled}
        onChange={(newCurrency: string) =>
          prefillExchangeRate(form, organizationId, newCurrency, orgCurrency)
        }
      >
        {map(currencies, (c) => (
          <Option value={c} key={c}>
            {c}
          </Option>
        ))}
      </Select>
    </Form.Item>
  </Col>
);

/**
 * The two fields every document currency lives next to: the rate used to
 * convert 1 unit of the document's own currency into the organization's
 * currency, and the date it was captured. Rendered only when the document's
 * currency differs from the organization's — a same-currency document has
 * nothing to convert. See db/exchange_rate.go for the storage/direction
 * convention this mirrors.
 */
const ExchangeRateFields = ({
  currency,
  orgCurrency,
  disabled,
}: {
  currency: string | undefined | null;
  orgCurrency: string;
  disabled?: boolean;
}) => {
  if (!showExchangeRateFields(currency, orgCurrency)) return null;

  return (
    <>
      <Col xs={24} md={12} xl={4}>
        <Form.Item
          label={<Trans>Exchange rate</Trans>}
          name="exchangeRate"
          tooltip={t`1 ${currency} = this many ${orgCurrency}`}
          rules={[{ required: true, message: t`This field is required!` }]}
        >
          <InputNumber
            min={0}
            step={0.0001}
            precision={6}
            style={{ width: "100%" }}
            disabled={disabled}
          />
        </Form.Item>
      </Col>
      <Col xs={24} md={12} xl={4}>
        <Form.Item
          label={<Trans>Rate date</Trans>}
          name="exchangeRateDate"
          rules={[{ required: true, message: t`This field is required!` }]}
        >
          <DatePicker style={{ width: "100%" }} disabled={disabled} />
        </Form.Item>
      </Col>
    </>
  );
};

/**
 * Call from a currency Select's onChange. Switching to the organization's
 * own currency clears both fields (nothing to convert). Switching to a
 * foreign currency prefills the rate from whatever this organization last
 * used for that currency — pure convenience, never auto-applied without the
 * user seeing and confirming it — and dates it today, since that's when this
 * document is being captured regardless of how old the prefilled rate is.
 */
export const prefillExchangeRate = async (
  form: FormInstance,
  organizationId: string | undefined,
  newCurrency: string,
  orgCurrency: string,
) => {
  if (newCurrency === orgCurrency) {
    form.setFieldsValue({ exchangeRate: undefined, exchangeRateDate: undefined });
    return;
  }
  form.setFieldsValue({ exchangeRateDate: dayjs() });
  if (!organizationId) return;
  try {
    const { rate } = await GetLastExchangeRate(organizationId, newCurrency);
    if (rate) form.setFieldsValue({ exchangeRate: rate });
  } catch {
    // No prior rate for this currency (or the lookup failed) — leave the
    // field blank for manual entry rather than blocking the form.
  }
};

export default ExchangeRateFields;
