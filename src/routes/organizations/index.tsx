import { useEffect, useMemo, useState } from "react";
import type { Account, Organization } from "src/types/models";
import {
  App,
  Button,
  Card,
  Checkbox,
  Col,
  Drawer,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Row,
  Select,
  Space,
  Table,
  Typography,
  Upload,
  theme,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import {
  ApartmentOutlined,
  DeleteOutlined,
  ExclamationCircleOutlined,
  UploadOutlined,
} from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { nanoid } from "nanoid";
import filter from "lodash/filter";
import some from "lodash/some";
import get from "lodash/get";
import includes from "lodash/includes";
import toString from "lodash/toString";
import compact from "lodash/compact";
import map from "lodash/map";
import uniq from "lodash/uniq";

import {
  GetOrganizations,
  GetOrganization,
  CreateOrganization,
  UpdateOrganization,
  DeleteOrganization,
  DeleteOrganizationLogo,
  GetOrganizationUsageCount,
  ResetOrganizationData,
  GetAccounts,
  type OrganizationUsageCount,
} from "src/api";
import { CSRF_HEADER } from "src/api/client";
import {
  organizationIdAtom,
  reloadOrganizationAtom,
  setOrganizationsAtom,
} from "src/atoms/organization";
import { isAdminAtom } from "src/atoms/auth";
import { DATE_FORMATS, type DateFormatKey, getDateFormatLabel } from "src/utils/date";
import { countries } from "src/utils/countries";
import { getDefaultFractionDigits } from "src/utils/currencies";
import { useCountryOptions } from "src/hooks/useCountryOptions";
import PageHeader from "src/components/page-header";

const currencies = compact(uniq(map(countries, "currency_code")));
const { Text } = Typography;

export default function Organizations() {
  useLingui();
  const { token } = theme.useToken();
  const { message } = App.useApp();
  const isAdmin = useAtomValue(isAdminAtom);
  const [form] = Form.useForm();

  const [orgs, setOrgs] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [search, setSearch] = useState("");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [usageCounts, setUsageCounts] = useState<Record<string, OrganizationUsageCount>>({});
  const [logoKey, setLogoKey] = useState(0);
  const [hasLogo, setHasLogo] = useState(true);
  const [logoBusy, setLogoBusy] = useState(false);
  const [resetMasterData, setResetMasterData] = useState(false);
  const [resetTransactionalData, setResetTransactionalData] = useState(false);
  const [resetting, setResetting] = useState(false);
  // Fetched per-editingId (not the shared accountsAtom, which is scoped to
  // the globally-selected organization) — this drawer can edit an org other
  // than the currently-selected one, same reasoning as the Logo card above.
  const [editingAccounts, setEditingAccounts] = useState<Account[]>([]);

  const [organizationId, setOrganizationId] = useAtom(organizationIdAtom);
  const refreshGlobalOrgs = useSetAtom(setOrganizationsAtom);
  const reloadActiveOrganization = useSetAtom(reloadOrganizationAtom);

  const watchedCountryCode = Form.useWatch("country_code", form);
  const countryOptions = useCountryOptions(watchedCountryCode);

  const fetchOrgs = async () => {
    setLoading(true);
    try {
      setOrgs(await GetOrganizations());
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchOrgs();
  }, []);

  const filteredOrgs = search
    ? filter(orgs, (org) =>
        some(["name", "code", "email", "phone", "iban", "currency"], (field) =>
          includes(toString(get(org, field)).toLowerCase(), search.toLowerCase()),
        ),
      )
    : orgs;

  const openNew = () => {
    setEditingId(null);
    form.resetFields();
    form.setFieldsValue({ minimum_fraction_digits: 2, currency: "EUR" });
    setDrawerOpen(true);
  };

  const openEdit = async (id: string) => {
    setEditingId(id);
    form.resetFields();
    setHasLogo(true);
    setLogoKey((k) => k + 1);
    setResetMasterData(false);
    setResetTransactionalData(false);
    setEditingAccounts([]);
    setDrawerOpen(true);
    try {
      const org = await GetOrganization(id);
      // Convert null date_format to undefined so the Select shows placeholder
      form.setFieldsValue({ ...org, date_format: org.date_format ?? undefined });
    } catch {}
    try {
      setEditingAccounts(await GetAccounts(id));
    } catch {
      // Accounting selects just render empty if this fails.
    }
  };

  const handleClose = () => {
    setDrawerOpen(false);
    setEditingId(null);
    form.resetFields();
    setResetMasterData(false);
    setResetTransactionalData(false);
    setEditingAccounts([]);
  };

  const leafAccountOptions = useMemo(
    () =>
      editingAccounts
        .filter((a) => !a.isGroup)
        .map((a) => ({ value: a.id, label: `${a.code} · ${a.name}` })),
    [editingAccounts],
  );

  const refreshLogo = () => {
    setHasLogo(true);
    setLogoKey((k) => k + 1);
    if (editingId === organizationId) reloadActiveOrganization();
  };

  const handleLogoRemove = async () => {
    if (!editingId) return;
    setLogoBusy(true);
    try {
      await DeleteOrganizationLogo(editingId);
      setHasLogo(false);
      if (editingId === organizationId) reloadActiveOrganization();
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Logo removal failed`);
    } finally {
      setLogoBusy(false);
    }
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      if (editingId) {
        await UpdateOrganization(editingId, values);
      } else {
        const newOrg = await CreateOrganization({
          ...values,
          id: nanoid(),
          currency: values.currency || "EUR",
          minimum_fraction_digits: values.minimum_fraction_digits ?? 2,
          due_days: 7,
          overdueCharge: 0,
          invoiceNumberFormat: "#{number}",
          invoiceNumberCounter: 0,
        });
        setOrganizationId(newOrg.id);
      }
      await fetchOrgs();
      refreshGlobalOrgs();
      handleClose();
    } catch {
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await DeleteOrganization(id);
      if (id === organizationId) {
        const remaining = orgs.filter((o) => o.id !== id);
        setOrganizationId(remaining.length > 0 ? remaining[0].id : null);
      }
      await fetchOrgs();
      refreshGlobalOrgs();
      message.success(t`Organization deleted`);
    } catch (error) {
      console.error("Failed to delete organization:", error);
      message.error(error instanceof Error ? error.message : t`Organization deletion failed`);
    }
  };

  const fetchUsageCount = async (id: string) => {
    if (usageCounts[id]) return;
    try {
      const counts = await GetOrganizationUsageCount(id);
      setUsageCounts((prev) => ({ ...prev, [id]: counts }));
    } catch {
      // Confirmation still works without the breakdown if this fails.
    }
  };

  const handleReset = async (id: string) => {
    setResetting(true);
    try {
      const deleted = await ResetOrganizationData(id, {
        resetMasterData,
        resetTransactionalData,
      });
      const total = Object.values(deleted).reduce((sum, n) => sum + n, 0);
      message.success(
        total > 0
          ? t`Deleted ${total} record(s)`
          : t`Nothing to delete — this organization was empty`,
      );
      // The counts just shown are now stale (this org has fewer, or no,
      // records); drop the cache entry so the next Popconfirm open refetches.
      setUsageCounts((prev) => {
        const next = { ...prev };
        delete next[id];
        return next;
      });
      setResetMasterData(false);
      setResetTransactionalData(false);
      // Invoice numbering and other profile-derived fields the drawer/settings
      // pages read (e.g. the invoice number preview) just changed under it.
      if (id === organizationId) reloadActiveOrganization();
    } catch (error) {
      console.error("Failed to reset organization data:", error);
      message.error(error instanceof Error ? error.message : t`Reset failed`);
    } finally {
      setResetting(false);
    }
  };

  const isEdit = !!editingId;

  const resetCounts = editingId ? usageCounts[editingId] : undefined;
  const resetBreakdown = resetCounts
    ? [
        ...(resetMasterData
          ? [
              [resetCounts.clients, t`client(s)`],
              [resetCounts.vendors, t`vendor(s)`],
              [resetCounts.products, t`product(s)`],
              [resetCounts.taxRates, t`tax rate(s)`],
            ]
          : []),
        ...(resetMasterData || resetTransactionalData
          ? [
              [resetCounts.invoices, t`invoice(s)`],
              [resetCounts.orders, t`order(s)`],
              [resetCounts.deliveries, t`delivery(ies)`],
              [resetCounts.purchaseOrders, t`purchase order(s)`],
              [resetCounts.inboundDeliveries, t`goods receipt(s)`],
              [resetCounts.incomingInvoices, t`incoming invoice(s)`],
              [resetCounts.stockMovements, t`stock movement(s)`],
            ]
          : []),
      ].filter(([n]) => (n as number) > 0)
    : [];
  const resetSelected = resetMasterData || resetTransactionalData;

  return (
    <>
      <PageHeader
        style={{ marginBottom: 12 }}
        icon={<ApartmentOutlined />}
        title={<Trans>Organizations</Trans>}
        search={{
          placeholder: t`Search`,
          value: search,
          onChange: setSearch,
          allowClear: true,
          onClear: () => setSearch(""),
        }}
        actions={
          <Button type="primary" onClick={openNew}>
            <Trans>New organization</Trans>
          </Button>
        }
      />

      <Table
        dataSource={filteredOrgs}
        rowKey="id"
        loading={loading}
        pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
        size="middle"
        onRow={(record) => ({ onClick: () => openEdit(record.id), style: { cursor: "pointer" } })}
      >
        <Table.Column
          title={<Trans>Name</Trans>}
          dataIndex="name"
          key="name"
          sorter={(a: Organization, b: Organization) => (a.name ?? "").localeCompare(b.name ?? "")}
        />
        <Table.Column
          title={<Trans>Code</Trans>}
          dataIndex="code"
          key="code"
          width={120}
          sorter={(a: Organization, b: Organization) => (a.code ?? "").localeCompare(b.code ?? "")}
        />
        <Table.Column
          title={<Trans>Email</Trans>}
          dataIndex="email"
          key="email"
          sorter={(a: Organization, b: Organization) =>
            (a.email ?? "").localeCompare(b.email ?? "")
          }
        />
        <Table.Column
          title={<Trans>Phone</Trans>}
          dataIndex="phone"
          key="phone"
          width={150}
          sorter={(a: Organization, b: Organization) =>
            (a.phone ?? "").localeCompare(b.phone ?? "")
          }
        />
        <Table.Column
          title="IBAN"
          dataIndex="iban"
          key="iban"
          width={200}
          sorter={(a: Organization, b: Organization) => (a.iban ?? "").localeCompare(b.iban ?? "")}
        />
        <Table.Column
          title={<Trans>Currency</Trans>}
          dataIndex="currency"
          key="currency"
          width={100}
          sorter={(a: Organization, b: Organization) =>
            (a.currency ?? "").localeCompare(b.currency ?? "")
          }
        />
        <Table.Column
          title=""
          key="actions"
          width={80}
          render={(_: unknown, record: Organization) => {
            if (!isAdmin) return null;
            const counts = usageCounts[record.id];
            const breakdown = counts
              ? [
                  [counts.clients, t`client(s)`],
                  [counts.vendors, t`vendor(s)`],
                  [counts.invoices, t`invoice(s)`],
                  [counts.products, t`product(s)`],
                  [counts.orders, t`order(s)`],
                  [counts.purchaseOrders, t`purchase order(s)`],
                  [counts.inboundDeliveries, t`goods receipt(s)`],
                  [counts.incomingInvoices, t`incoming invoice(s)`],
                  [counts.deliveries, t`delivery(ies)`],
                  [counts.taxRates, t`tax rate(s)`],
                ].filter(([n]) => (n as number) > 0)
              : [];
            return (
              <Popconfirm
                title={t`Delete this organization?`}
                description={
                  breakdown.length > 0 ? (
                    <div style={{ maxWidth: 260 }}>
                      <div>
                        <Trans>This will permanently delete:</Trans>
                      </div>
                      <ul style={{ margin: "4px 0", paddingLeft: 18 }}>
                        {breakdown.map(([n, label]) => (
                          <li key={label as string}>
                            {n} {label}
                          </li>
                        ))}
                      </ul>
                      <div>
                        <Trans>This cannot be undone.</Trans>
                      </div>
                    </div>
                  ) : (
                    <Trans>This cannot be undone.</Trans>
                  )
                }
                onOpenChange={(open) => {
                  if (open) fetchUsageCount(record.id);
                }}
                onConfirm={(e) => {
                  e?.stopPropagation();
                  handleDelete(record.id);
                }}
                onCancel={(e) => e?.stopPropagation()}
              >
                <Button size="small" danger onClick={(e) => e.stopPropagation()}>
                  <Trans>Delete</Trans>
                </Button>
              </Popconfirm>
            );
          }}
        />
      </Table>

      <Drawer
        title={isEdit ? <Trans>Edit organization</Trans> : <Trans>New organization</Trans>}
        open={drawerOpen}
        placement="right"
        size={640}
        onClose={handleClose}
        footer={
          <Space style={{ justifyContent: "flex-end", width: "100%", display: "flex" }}>
            <Button onClick={handleClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="primary" loading={submitting} onClick={() => form.submit()}>
              <Trans>Save</Trans>
            </Button>
          </Space>
        }
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
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

          {isEdit && editingId && (
            <Card size="small" title={<Trans>Logo</Trans>} style={{ marginBottom: 12 }}>
              <Space direction="vertical" size={12}>
                {hasLogo && (
                  <img
                    key={logoKey}
                    src={`/api/organizations/${editingId}/logo?t=${logoKey}`}
                    alt="logo"
                    onError={() => setHasLogo(false)}
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
                      if (file.status === "done") refreshLogo();
                      else if (file.status === "error") message.error(t`Logo upload failed`);
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
                      onClick={handleLogoRemove}
                    >
                      <Trans>Remove logo</Trans>
                    </Button>
                  )}
                </Space>
              </Space>
            </Card>
          )}

          <Card size="small" title={<Trans>Banking</Trans>} style={{ marginBottom: 12 }}>
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
                <Form.Item name="vatin" label="VATIN">
                  <Input />
                </Form.Item>
              </Col>
            </Row>
          </Card>

          <Card size="small" title={<Trans>Address</Trans>} style={{ marginBottom: 12 }}>
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
          </Card>

          <Card size="small" title={<Trans>E-invoicing</Trans>} style={{ marginBottom: 12 }}>
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
          </Card>

          <Card size="small" title={<Trans>Formatting</Trans>}>
            <Row gutter={[16, 0]}>
              <Col xs={24} md={12}>
                <Form.Item name="date_format" label={<Trans>Date format</Trans>}>
                  <Select placeholder={t`Select date format`}>
                    {Object.keys(DATE_FORMATS).map((key) => (
                      <Select.Option key={key} value={DATE_FORMATS[key as DateFormatKey] ?? "AUTO"}>
                        {getDateFormatLabel(key as DateFormatKey)}
                      </Select.Option>
                    ))}
                  </Select>
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="minimum_fraction_digits" label={<Trans>Decimal places</Trans>}>
                  <InputNumber min={0} max={10} style={{ width: "100%" }} />
                </Form.Item>
              </Col>
            </Row>
          </Card>

          {isEdit && editingId && (
            <Card size="small" title={<Trans>Accounting</Trans>} style={{ marginBottom: 12 }}>
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
                      <Trans>Used for a sales invoice line whose product has no override.</Trans>
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
                      <Trans>Used for a vendor bill line whose product has no override.</Trans>
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
                        Used to capitalize a stock-enabled product's value when it's received or
                        adjusted.
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
                        Cost of goods sold, recognized against inventory when a shipment ships.
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
                    name="datevClearingAccountId"
                    label={<Trans>DATEV clearing account</Trans>}
                    tooltip={
                      <Trans>
                        Synthetic counter-account for a manual journal entry with more than one
                        line on both sides (no natural anchor line) when exporting to DATEV. Leave
                        blank if you don't use DATEV.
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
                    tooltip={<Trans>Required to generate a DATEV export (1001–9999999).</Trans>}
                  >
                    <Input placeholder={t`e.g. 1001`} />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item
                    name="datev_client_number"
                    label={<Trans>DATEV client number</Trans>}
                    tooltip={<Trans>Required to generate a DATEV export (1–99999).</Trans>}
                    style={{ marginBottom: 0 }}
                  >
                    <Input placeholder={t`e.g. 456`} />
                  </Form.Item>
                </Col>
              </Row>
            </Card>
          )}

          {isEdit && editingId && isAdmin && (
            <Card
              size="small"
              title={<Trans>Danger zone</Trans>}
              style={{ marginTop: 12, borderColor: token.colorErrorBorder }}
            >
              <Space direction="vertical" size={8} style={{ width: "100%" }}>
                <Text type="secondary">
                  <Trans>
                    Permanently delete this organization's data without deleting the organization
                    itself.
                  </Trans>
                </Text>
                <Checkbox
                  checked={resetMasterData}
                  onChange={(e) => {
                    const checked = e.target.checked;
                    setResetMasterData(checked);
                    if (checked) setResetTransactionalData(true);
                  }}
                >
                  <Trans>Master data</Trans>{" "}
                  <Text type="secondary">({t`clients, vendors, products, tax rates`})</Text>
                </Checkbox>
                <Checkbox
                  checked={resetTransactionalData}
                  disabled={resetMasterData}
                  onChange={(e) => setResetTransactionalData(e.target.checked)}
                >
                  <Trans>Transactional data</Trans>{" "}
                  <Text type="secondary">
                    (
                    {t`invoices, orders, deliveries, purchase orders, goods receipts, incoming invoices, stock movements`}
                    )
                  </Text>
                </Checkbox>
                {resetMasterData && (
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    <Trans>
                      Master data can't be reset on its own — documents reference clients and
                      vendors, so transactional data is included automatically.
                    </Trans>
                  </Text>
                )}
                <Popconfirm
                  title={t`Reset this organization's data?`}
                  description={
                    <div style={{ maxWidth: 280 }}>
                      {resetBreakdown.length > 0 ? (
                        <>
                          <div>
                            <Trans>This will permanently delete:</Trans>
                          </div>
                          <ul style={{ margin: "4px 0", paddingLeft: 18 }}>
                            {resetBreakdown.map(([n, label]) => (
                              <li key={label as string}>
                                {n} {label}
                              </li>
                            ))}
                          </ul>
                        </>
                      ) : (
                        <div>
                          <Trans>Nothing matches the current selection.</Trans>
                        </div>
                      )}
                      <div>
                        <Trans>This cannot be undone.</Trans>
                      </div>
                    </div>
                  }
                  okButtonProps={{ danger: true }}
                  onOpenChange={(open) => {
                    if (open) fetchUsageCount(editingId);
                  }}
                  onConfirm={() => handleReset(editingId)}
                  disabled={!resetSelected}
                >
                  <Button
                    danger
                    icon={<ExclamationCircleOutlined />}
                    loading={resetting}
                    disabled={!resetSelected}
                  >
                    <Trans>Reset selected data</Trans>
                  </Button>
                </Popconfirm>
              </Space>
            </Card>
          )}
        </Form>
      </Drawer>
    </>
  );
}
