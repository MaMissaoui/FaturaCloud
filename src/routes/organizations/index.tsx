import { useEffect, useState } from "react";
import {
  Button,
  Card,
  Col,
  Drawer,
  Form,
  Input,
  InputNumber,
  message,
  Popconfirm,
  Row,
  Select,
  Space,
  Table,
  Typography,
} from "antd";
import { useAtom, useSetAtom } from "jotai";
import { ApartmentOutlined } from "@ant-design/icons";
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
  GetOrganizationUsageCount,
  type OrganizationUsageCount,
} from "src/api";
import { organizationIdAtom, setOrganizationsAtom } from "src/atoms/organization";
import { DATE_FORMATS, type DateFormatKey, getDateFormatLabel } from "src/utils/date";
import { countries } from "src/utils/countries";

const { Title } = Typography;
const { TextArea } = Input;

const currencies = compact(uniq(map(countries, "currency_code")));

export default function Organizations() {
  useLingui();
  const [form] = Form.useForm();

  const [orgs, setOrgs] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [search, setSearch] = useState("");
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [usageCounts, setUsageCounts] = useState<Record<string, OrganizationUsageCount>>({});

  const [organizationId, setOrganizationId] = useAtom(organizationIdAtom);
  const refreshGlobalOrgs = useSetAtom(setOrganizationsAtom);

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
      <Row style={{ marginBottom: 16 }}>
        <Col flex="auto">
          <Title level={3} style={{ margin: 0 }}>
            <ApartmentOutlined style={{ marginRight: 8 }} />
            <Trans>Organizations</Trans>
          </Title>
        </Col>
        <Col>
          <Space>
            <Input.Search
              placeholder={t`Search`}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              allowClear
              onClear={() => setSearch("")}
            />
            <Button type="primary" onClick={openNew}>
              <Trans>New organization</Trans>
            </Button>
          </Space>
        </Col>
      </Row>

      <Table
        dataSource={filteredOrgs}
        rowKey="id"
        loading={loading}
        pagination={false}
        size="middle"
        onRow={(record) => ({ onClick: () => openEdit(record.id), style: { cursor: "pointer" } })}
      >
        <Table.Column
          title={<Trans>Name</Trans>}
          dataIndex="name"
          key="name"
          sorter={(a: any, b: any) => (a.name ?? "").localeCompare(b.name ?? "")}
        />
        <Table.Column
          title={<Trans>Code</Trans>}
          dataIndex="code"
          key="code"
          width={120}
          sorter={(a: any, b: any) => (a.code ?? "").localeCompare(b.code ?? "")}
        />
        <Table.Column
          title={<Trans>Email</Trans>}
          dataIndex="email"
          key="email"
          sorter={(a: any, b: any) => (a.email ?? "").localeCompare(b.email ?? "")}
        />
        <Table.Column
          title={<Trans>Phone</Trans>}
          dataIndex="phone"
          key="phone"
          width={150}
          sorter={(a: any, b: any) => (a.phone ?? "").localeCompare(b.phone ?? "")}
        />
        <Table.Column
          title="IBAN"
          dataIndex="iban"
          key="iban"
          width={200}
          sorter={(a: any, b: any) => (a.iban ?? "").localeCompare(b.iban ?? "")}
        />
        <Table.Column
          title={<Trans>Currency</Trans>}
          dataIndex="currency"
          key="currency"
          width={100}
          sorter={(a: any, b: any) => (a.currency ?? "").localeCompare(b.currency ?? "")}
        />
        <Table.Column
          title=""
          key="actions"
          width={80}
          render={(_: any, record: any) => {
            const counts = usageCounts[record.id];
            const breakdown = counts
              ? [
                  [counts.clients, t`client(s)`],
                  [counts.invoices, t`invoice(s)`],
                  [counts.products, t`product(s)`],
                  [counts.orders, t`order(s)`],
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
                          <li key={label as string}>{n} {label}</li>
                        ))}
                      </ul>
                      <div><Trans>This cannot be undone.</Trans></div>
                    </div>
                  ) : (
                    <Trans>This cannot be undone.</Trans>
                  )
                }
                onOpenChange={(open) => { if (open) fetchUsageCount(record.id); }}
                onConfirm={(e) => { e?.stopPropagation(); handleDelete(record.id); }}
                onCancel={(e) => e?.stopPropagation()}
              >
                <Button
                  size="small"
                  danger
                  onClick={(e) => e.stopPropagation()}
                >
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
        width={640}
        onClose={handleClose}
        footer={
          <Space style={{ justifyContent: "flex-end", width: "100%", display: "flex" }}>
            <Button onClick={handleClose}><Trans>Cancel</Trans></Button>
            <Button type="primary" loading={submitting} onClick={() => form.submit()}>
              <Trans>Save</Trans>
            </Button>
          </Space>
        }
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Card title={<Trans>Details</Trans>} style={{ marginBottom: 16 }}>
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
                      <Select.Option key={c.name} value={c.name}>{c.name}</Select.Option>
                    ))}
                  </Select>
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item name="currency" label={<Trans>Currency</Trans>}>
                  <Select showSearch>
                    {currencies.map((c) => (
                      <Select.Option key={c} value={c}>{c}</Select.Option>
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
              <Col xs={24}>
                <Form.Item name="address" label={<Trans>Address</Trans>}>
                  <TextArea rows={3} />
                </Form.Item>
              </Col>
            </Row>
          </Card>

          <Card title={<Trans>Banking</Trans>} style={{ marginBottom: 16 }}>
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
                <Form.Item name="vatin" label="VATIN">
                  <Input />
                </Form.Item>
              </Col>
            </Row>
          </Card>

          <Card title={<Trans>Formatting</Trans>}>
            <Row gutter={[16, 0]}>
              <Col xs={24} md={12}>
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
