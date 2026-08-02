import { useEffect, useState } from "react";
import type { Organization } from "src/types/models";
import {
  App,
  Button,
  Card,
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
  Upload,
  theme,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { ApartmentOutlined, DeleteOutlined, UploadOutlined } from "@ant-design/icons";
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
    setDrawerOpen(true);
    try {
      const org = await GetOrganization(id);
      // Convert null date_format to undefined so the Select shows placeholder
      form.setFieldsValue({ ...org, date_format: org.date_format ?? undefined });
    } catch {}
  };

  const handleClose = () => {
    setDrawerOpen(false);
    setEditingId(null);
    form.resetFields();
  };

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

  const isEdit = !!editingId;

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
        </Form>
      </Drawer>
    </>
  );
}
