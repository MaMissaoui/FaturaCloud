import type { FormInstance } from "antd";
import {
  Button,
  Card,
  Checkbox,
  Col,
  Collapse,
  Drawer,
  Form,
  Input,
  InputNumber,
  Row,
  Select,
  Space,
  Upload,
  theme,
} from "antd";
import { UploadOutlined, DeleteOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import compact from "lodash/compact";
import uniq from "lodash/uniq";
import map from "lodash/map";

import { CSRF_HEADER } from "src/api/client";
import { DATE_FORMATS, type DateFormatKey, getDateFormatLabel } from "src/utils/date";
import { countries } from "src/utils/countries";
import { getDefaultFractionDigits } from "src/utils/currencies";
import { useCountryOptions } from "src/hooks/useCountryOptions";
import BrandColorPicker from "src/components/organizations/brand-color-picker";
import { invoicePDFLayoutOptions } from "src/components/invoices/layouts";
import OrganizationMembersPanel, {
  type OrganizationMembersPanelProps,
} from "src/components/organizations/organization-members-panel";
import OrganizationDangerZone, {
  type OrganizationDangerZoneProps,
} from "src/components/organizations/organization-danger-zone";

const currencies = compact(uniq(map(countries, "currency_code")));

interface AccountOption {
  value: string;
  label: string;
}

interface OrganizationEditDrawerProps {
  open: boolean;
  isEdit: boolean;
  editingId: string | null;
  form: FormInstance;
  submitting: boolean;
  onClose: () => void;
  onSubmit: (values: any) => void;
  activeSections: string[];
  onActiveSectionsChange: (keys: string[]) => void;
  hasLogo: boolean;
  logoKey: number;
  logoBusy: boolean;
  // Two distinct failure paths, not one: an <img> load failure (e.g. the
  // logo was deleted server-side but hasLogo is still stale) just hides the
  // broken image; an Upload failure shows a toast. Conflating them would
  // either silently hide a real upload-error toast or wrongly hide the
  // image on an unrelated toast-worthy failure.
  onLogoImgError: () => void;
  onLogoUploaded: () => void;
  onLogoUploadError: () => void;
  onLogoRemove: () => void;
  leafAccountOptions: AccountOption[];
  // null when this actor doesn't administer editingId (or there's no
  // editingId yet) — same condition the parent used to gate these two Cards
  // inline before this component existed.
  membersPanelProps: OrganizationMembersPanelProps | null;
  dangerZoneProps: OrganizationDangerZoneProps | null;
}

// The Details Card + Collapse (Appearance/Logo/Banking/Address/E-invoicing/
// Formatting/Accounting) + Members/Danger-zone Cards, all inside one Drawer
// and one Form. Extracted from src/routes/organizations/index.tsx (F151,
// 2026-09-09) — that page still owns every piece of state and every
// fetch/handler; this component only owns rendering plus the two
// `Form.useWatch` reads below (moved in from the parent so a keystroke in
// either watched field no longer re-renders the organizations list Table,
// which now lives in a sibling component).
export default function OrganizationEditDrawer({
  open,
  isEdit,
  editingId,
  form,
  submitting,
  onClose,
  onSubmit,
  activeSections,
  onActiveSectionsChange,
  hasLogo,
  logoKey,
  logoBusy,
  onLogoImgError,
  onLogoUploaded,
  onLogoUploadError,
  onLogoRemove,
  leafAccountOptions,
  membersPanelProps,
  dangerZoneProps,
}: OrganizationEditDrawerProps) {
  const { token } = theme.useToken();
  const watchedCountryCode = Form.useWatch("country_code", form);
  const watchedFiscalStampEnabled = Form.useWatch("fiscalStampEnabled", form);
  const countryOptions = useCountryOptions(watchedCountryCode);

  return (
    <Drawer
      title={isEdit ? <Trans>Edit organization</Trans> : <Trans>New organization</Trans>}
      open={open}
      placement="right"
      size={640}
      onClose={onClose}
      footer={
        <Space style={{ justifyContent: "flex-end", width: "100%", display: "flex" }}>
          <Button onClick={onClose}>
            <Trans>Cancel</Trans>
          </Button>
          <Button type="primary" loading={submitting} onClick={() => form.submit()}>
            <Trans>Save</Trans>
          </Button>
        </Space>
      }
    >
      <Form form={form} layout="vertical" onFinish={onSubmit}>
        <Card size="small" title={<Trans>Details</Trans>} style={{ marginBottom: 12 }}>
          <Row gutter={[16, 0]}>
            <Col xs={24} md={16}>
              <Form.Item
                name="name"
                label={<Trans>Name</Trans>}
                rules={[{ required: true, message: t`This field is required!` }]}
              >
                <Input />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item name="code" label={<Trans>Code</Trans>}>
                <Input
                  maxLength={20}
                  onChange={(e) => form.setFieldValue("code", e.target.value.toUpperCase())}
                />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="country" label={<Trans>Country</Trans>}>
                <Select showSearch placeholder={t`Select country`}>
                  {countries.map((c) => (
                    <Select.Option key={c.name} value={c.name}>
                      {c.name}
                    </Select.Option>
                  ))}
                </Select>
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="currency" label={<Trans>Currency</Trans>}>
                <Select
                  showSearch
                  onChange={(c: string) =>
                    form.setFieldValue("minimum_fraction_digits", getDefaultFractionDigits(c))
                  }
                >
                  {currencies.map((c) => (
                    <Select.Option key={c} value={c}>
                      {c}
                    </Select.Option>
                  ))}
                </Select>
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="email" label={<Trans>E-mail</Trans>}>
                <Input />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="phone" label={<Trans>Phone</Trans>}>
                <Input />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="website" label={<Trans>Website</Trans>}>
                <Input />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="registration_number" label={<Trans>Registration number</Trans>}>
                <Input />
              </Form.Item>
            </Col>
          </Row>
        </Card>

        <Collapse
          size="small"
          activeKey={activeSections}
          onChange={(keys) => onActiveSectionsChange(keys as string[])}
          style={{ marginBottom: 12 }}
          items={compact([
            {
              key: "appearance",
              label: <Trans>Appearance</Trans>,
              forceRender: true,
              children: (
                <Form.Item
                  name="brandColor"
                  label={<Trans>Brand color</Trans>}
                  tooltip={
                    <Trans>
                      Accent color used across the app while this organization is selected.
                    </Trans>
                  }
                  style={{ marginBottom: 0 }}
                >
                  <BrandColorPicker />
                </Form.Item>
              ),
            },
            isEdit && editingId
              ? {
                  key: "logo",
                  label: <Trans>Logo</Trans>,
                  forceRender: true,
                  children: (
                    <Space direction="vertical" size={12}>
                      {hasLogo && (
                        <img
                          key={logoKey}
                          src={`/api/organizations/${editingId}/logo?t=${logoKey}`}
                          alt="logo"
                          onError={onLogoImgError}
                          style={{
                            maxWidth: 240,
                            maxHeight: 80,
                            objectFit: "contain",
                            border: `1px solid ${token.colorBorderSecondary}`,
                            borderRadius: 6,
                            padding: 8,
                            display: "block",
                          }}
                        />
                      )}
                      <Space>
                        <Upload
                          accept="image/png,image/jpeg,image/jpg,image/gif,image/webp"
                          showUploadList={false}
                          name="file"
                          action={`/api/organizations/${editingId}/logo`}
                          headers={{ [CSRF_HEADER]: "1" }}
                          onChange={({ file }) => {
                            if (file.status === "done") onLogoUploaded();
                            else if (file.status === "error") onLogoUploadError();
                          }}
                        >
                          <Button icon={<UploadOutlined />} loading={logoBusy}>
                            {hasLogo ? t`Change logo` : t`Upload logo`}
                          </Button>
                        </Upload>
                        {hasLogo && (
                          <Button
                            danger
                            icon={<DeleteOutlined />}
                            loading={logoBusy}
                            onClick={onLogoRemove}
                          >
                            <Trans>Remove logo</Trans>
                          </Button>
                        )}
                      </Space>
                    </Space>
                  ),
                }
              : null,
            {
              key: "banking",
              label: <Trans>Banking</Trans>,
              forceRender: true,
              children: (
                <Row gutter={[16, 0]}>
                  <Col xs={24} md={12}>
                    <Form.Item name="bank_name" label={<Trans>Bank name</Trans>}>
                      <Input />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={12}>
                    <Form.Item name="iban" label="IBAN">
                      <Input />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={12}>
                    <Form.Item name="bic" label="BIC">
                      <Input
                        maxLength={11}
                        onChange={(e) => form.setFieldValue("bic", e.target.value.toUpperCase())}
                      />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={12}>
                    <Form.Item name="vatin" label="VATIN" style={{ marginBottom: 0 }}>
                      <Input />
                    </Form.Item>
                  </Col>
                </Row>
              ),
            },
            {
              key: "address",
              label: <Trans>Address</Trans>,
              forceRender: true,
              children: (
                <Row gutter={[16, 0]}>
                  <Col xs={24} md={12}>
                    <Form.Item name="country_code" label={<Trans>Country</Trans>}>
                      <Select
                        showSearch
                        allowClear
                        placeholder={t`Select a country`}
                        options={countryOptions}
                        filterOption={(input, option) =>
                          (option?.label ?? "").toLowerCase().includes(input.toLowerCase())
                        }
                      />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={16}>
                    <Form.Item name="street" label={<Trans>Street</Trans>}>
                      <Input />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={8}>
                    <Form.Item name="house_number" label={<Trans>House number</Trans>}>
                      <Input />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={8}>
                    <Form.Item name="postal_code" label={<Trans>Postal code</Trans>}>
                      <Input />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={16}>
                    <Form.Item name="city" label={<Trans>City</Trans>} style={{ marginBottom: 0 }}>
                      <Input />
                    </Form.Item>
                  </Col>
                </Row>
              ),
            },
            {
              key: "einvoicing",
              label: <Trans>E-invoicing</Trans>,
              forceRender: true,
              children: (
                <Row gutter={[16, 0]}>
                  <Col xs={24}>
                    <Form.Item
                      name="tax_number"
                      label={<Trans>Tax number</Trans>}
                      style={{ marginBottom: 0 }}
                    >
                      <Input />
                    </Form.Item>
                  </Col>
                </Row>
              ),
            },
            {
              key: "formatting",
              label: <Trans>Formatting</Trans>,
              forceRender: true,
              children: (
                <Row gutter={[16, 0]}>
                  <Col xs={24} md={8}>
                    <Form.Item name="date_format" label={<Trans>Date format</Trans>}>
                      <Select placeholder={t`Select date format`}>
                        {Object.keys(DATE_FORMATS).map((key) => (
                          <Select.Option
                            key={key}
                            value={DATE_FORMATS[key as DateFormatKey] ?? "AUTO"}
                          >
                            {getDateFormatLabel(key as DateFormatKey)}
                          </Select.Option>
                        ))}
                      </Select>
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={8}>
                    <Form.Item name="minimum_fraction_digits" label={<Trans>Decimal places</Trans>}>
                      <InputNumber min={0} max={10} style={{ width: "100%" }} />
                    </Form.Item>
                  </Col>
                  <Col xs={24} md={8}>
                    <Form.Item
                      name="invoiceLayout"
                      label={<Trans>Invoice PDF layout</Trans>}
                      style={{ marginBottom: 0 }}
                      tooltip={
                        <Trans>Which template invoices for this organization render with.</Trans>
                      }
                    >
                      <Select placeholder={t`Default`} allowClear>
                        {invoicePDFLayoutOptions().map((option) => (
                          <Select.Option key={option.value} value={option.value}>
                            {option.label}
                          </Select.Option>
                        ))}
                      </Select>
                    </Form.Item>
                  </Col>
                </Row>
              ),
            },
            isEdit && editingId
              ? {
                  key: "accounting",
                  label: <Trans>Accounting</Trans>,
                  forceRender: true,
                  children: (
                    <Row gutter={[16, 0]}>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="defaultArAccountId"
                          label={<Trans>Accounts receivable</Trans>}
                          tooltip={<Trans>Used for the AR line when a sales invoice posts.</Trans>}
                        >
                          <Select
                            allowClear
                            showSearch
                            placeholder={t`None`}
                            options={leafAccountOptions}
                            optionFilterProp="label"
                          />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="defaultApAccountId"
                          label={<Trans>Accounts payable</Trans>}
                          tooltip={<Trans>Used for the AP line when a vendor bill posts.</Trans>}
                        >
                          <Select
                            allowClear
                            showSearch
                            placeholder={t`None`}
                            options={leafAccountOptions}
                            optionFilterProp="label"
                          />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="defaultRevenueAccountId"
                          label={<Trans>Default revenue account</Trans>}
                          tooltip={
                            <Trans>
                              Used for a sales invoice line whose product has no override.
                            </Trans>
                          }
                        >
                          <Select
                            allowClear
                            showSearch
                            placeholder={t`None`}
                            options={leafAccountOptions}
                            optionFilterProp="label"
                          />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="defaultExpenseAccountId"
                          label={<Trans>Default expense account</Trans>}
                          tooltip={
                            <Trans>
                              Used for a vendor bill line whose product has no override.
                            </Trans>
                          }
                        >
                          <Select
                            allowClear
                            showSearch
                            placeholder={t`None`}
                            options={leafAccountOptions}
                            optionFilterProp="label"
                          />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="defaultCashAccountId"
                          label={<Trans>Default cash account</Trans>}
                        >
                          <Select
                            allowClear
                            showSearch
                            placeholder={t`None`}
                            options={leafAccountOptions}
                            optionFilterProp="label"
                          />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="defaultInventoryAccountId"
                          label={<Trans>Default inventory account</Trans>}
                          tooltip={
                            <Trans>
                              Used to capitalize a stock-enabled product's value when it's received
                              or adjusted.
                            </Trans>
                          }
                        >
                          <Select
                            allowClear
                            showSearch
                            placeholder={t`None`}
                            options={leafAccountOptions}
                            optionFilterProp="label"
                          />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="defaultGRNIAccountId"
                          label={<Trans>Default GRNI account</Trans>}
                          tooltip={
                            <Trans>
                              Goods Received Not Invoiced — accrues a liability when a receipt is
                              received, cleared when the matching vendor bill is approved.
                            </Trans>
                          }
                        >
                          <Select
                            allowClear
                            showSearch
                            placeholder={t`None`}
                            options={leafAccountOptions}
                            optionFilterProp="label"
                          />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="defaultCOGSAccountId"
                          label={<Trans>Default COGS account</Trans>}
                          tooltip={
                            <Trans>
                              Cost of goods sold, recognized against inventory when a shipment
                              ships.
                            </Trans>
                          }
                        >
                          <Select
                            allowClear
                            showSearch
                            placeholder={t`None`}
                            options={leafAccountOptions}
                            optionFilterProp="label"
                          />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="defaultInventoryAdjustmentAccountId"
                          label={<Trans>Default inventory adjustment account</Trans>}
                          tooltip={
                            <Trans>
                              Counter-account for a manual stock adjustment's signed inventory value
                              change.
                            </Trans>
                          }
                        >
                          <Select
                            allowClear
                            showSearch
                            placeholder={t`None`}
                            options={leafAccountOptions}
                            optionFilterProp="label"
                          />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="defaultImportCostsPayableAccountId"
                          label={<Trans>Default import costs payable account</Trans>}
                          tooltip={
                            <Trans>
                              Credited for the freight/customs allocated to a receipt whose purchase
                              order belongs to an import — separate from GRNI, which stays valued at
                              the vendor's goods price only.
                            </Trans>
                          }
                        >
                          <Select
                            allowClear
                            showSearch
                            placeholder={t`None`}
                            options={leafAccountOptions}
                            optionFilterProp="label"
                          />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="datevClearingAccountId"
                          label={<Trans>DATEV clearing account</Trans>}
                          tooltip={
                            <Trans>
                              Synthetic counter-account for a manual journal entry with more than
                              one line on both sides (no natural anchor line) when exporting to
                              DATEV. Leave blank if you don't use DATEV.
                            </Trans>
                          }
                          style={{ marginBottom: 0 }}
                        >
                          <Select
                            allowClear
                            showSearch
                            placeholder={t`None`}
                            options={leafAccountOptions}
                            optionFilterProp="label"
                          />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="datev_consultant_number"
                          label={<Trans>DATEV consultant number</Trans>}
                          tooltip={
                            <Trans>Required to generate a DATEV export (1001–9999999).</Trans>
                          }
                        >
                          <Input placeholder={t`e.g. 1001`} />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="datev_client_number"
                          label={<Trans>DATEV client number</Trans>}
                          tooltip={<Trans>Required to generate a DATEV export (1–99999).</Trans>}
                        >
                          <Input placeholder={t`e.g. 456`} />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="fiscalStampEnabled"
                          valuePropName="checked"
                          label=" "
                          tooltip={
                            <Trans>
                              Shows a fiscal stamp (flat, non-taxable duty) field on this
                              organization's invoices, independent of the PDF layout.
                            </Trans>
                          }
                        >
                          <Checkbox>
                            <Trans>Enable fiscal stamp</Trans>
                          </Checkbox>
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          name="withholdingTaxEnabled"
                          valuePropName="checked"
                          label=" "
                          tooltip={
                            <Trans>
                              Shows a withholding tax rate field on this organization's invoices,
                              independent of the PDF layout.
                            </Trans>
                          }
                        >
                          <Checkbox>
                            <Trans>Enable withholding tax</Trans>
                          </Checkbox>
                        </Form.Item>
                      </Col>
                      {!!watchedFiscalStampEnabled && (
                        <>
                          <Col xs={24} md={12}>
                            <Form.Item
                              name="defaultFiscalStampAmount"
                              label={<Trans>Default fiscal stamp amount</Trans>}
                              tooltip={
                                <Trans>
                                  Prefills a new invoice's stamp amount. The statutory amount
                                  changes by law from time to time, so this is a plain editable
                                  default, not enforced.
                                </Trans>
                              }
                            >
                              <InputNumber min={0} precision={3} style={{ width: "100%" }} />
                            </Form.Item>
                          </Col>
                          <Col xs={24} md={12}>
                            <Form.Item
                              name="defaultStampDutyAccountId"
                              label={<Trans>Stamp duty account</Trans>}
                              tooltip={
                                <Trans>
                                  Liability account credited when an invoice's fiscal stamp posts —
                                  the seller collects it on behalf of the state, so it's never
                                  revenue.
                                </Trans>
                              }
                            >
                              <Select
                                allowClear
                                showSearch
                                placeholder={t`None`}
                                options={leafAccountOptions}
                                optionFilterProp="label"
                              />
                            </Form.Item>
                          </Col>
                        </>
                      )}
                    </Row>
                  ),
                }
              : null,
          ])}
        />

        {membersPanelProps && <OrganizationMembersPanel {...membersPanelProps} />}
        {dangerZoneProps && <OrganizationDangerZone {...dangerZoneProps} />}
      </Form>
    </Drawer>
  );
}
