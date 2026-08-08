import { useEffect, useMemo, useState } from "react";
import { useParams, useNavigate } from "react-router";
import { Button, Checkbox, Drawer, Form, Input, Popconfirm, Select, Space, Tooltip } from "antd";
import { useSetAtom, useAtomValue } from "jotai";
import { loadable } from "src/utils/loadable";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined } from "@ant-design/icons";
import isEmpty from "lodash/isEmpty";
import { GetTaxRateUsageCount } from "src/api";

import { taxRateIdAtom, taxRateAtom, deleteTaxRateAtom } from "src/atoms/tax-rate";
import { accountsAtom, setAccountsAtom } from "src/atoms/account";
import Section from "src/components/form-section";
import ScrollShadow from "src/components/scroll-shadow";

const loadableTaxRateAtom = loadable(taxRateAtom);

// BT-118 VAT category code (UNTDID 5305 subset EN 16931 accepts). Must stay
// in sync with taxRateCategoryCodes in db/tax_rate.go.
const TAX_RATE_CATEGORY_CODES = ["S", "Z", "E", "AE", "K", "G", "O", "L", "M"] as const;

// Categories other than standard rate normally need a BT-120 exemption
// reason so the exported XRechnung line isn't rejected as invalid.
const CATEGORIES_REQUIRING_EXEMPTION_REASON = new Set(["Z", "E", "AE", "K", "G", "O", "L", "M"]);

const useTaxRateCategoryLabels = (): Record<(typeof TAX_RATE_CATEGORY_CODES)[number], string> => ({
  S: t`Standard rate`,
  Z: t`Zero rated goods`,
  E: t`Exempt from tax`,
  AE: t`VAT reverse charge`,
  K: t`Intra-community supply (EEA)`,
  G: t`Free export item, tax not charged`,
  O: t`Outside scope of tax`,
  L: t`Canary Islands general indirect tax`,
  M: t`Tax for production, services and importation in Ceuta and Melilla`,
});

const TaxRateForm = () => {
  const navigate = useNavigate();
  const { id } = useParams<string>();

  const [form] = Form.useForm();

  const setTaxRateId = useSetAtom(taxRateIdAtom);
  const taxRate = useAtomValue(loadableTaxRateAtom);
  const setTaxRate = useSetAtom(taxRateAtom);
  const deleteTaxRate = useSetAtom(deleteTaxRateAtom);
  const [submitting, setSubmitting] = useState(false);
  const [usageCount, setUsageCount] = useState<number | null>(null);
  const categoryLabels = useTaxRateCategoryLabels();
  const categoryCode = Form.useWatch("category_code", form);
  const exemptionReasonRequired = CATEGORIES_REQUIRING_EXEMPTION_REASON.has(categoryCode);

  const accounts = useAtomValue(accountsAtom);
  const setAccounts = useSetAtom(setAccountsAtom);
  useEffect(() => {
    setAccounts();
  }, [setAccounts]);
  // Output VAT is a liability (owed to the tax authority); input VAT is a
  // reclaimable asset — the same two-account split db/tax_rate.go documents.
  const outputTaxAccountOptions = useMemo(
    () =>
      accounts
        .filter((a) => !a.isGroup && a.type === "liability")
        .map((a) => ({ value: a.id, label: `${a.code} · ${a.name}` })),
    [accounts],
  );
  const inputTaxAccountOptions = useMemo(
    () =>
      accounts
        .filter((a) => !a.isGroup && a.type === "asset")
        .map((a) => ({ value: a.id, label: `${a.code} · ${a.name}` })),
    [accounts],
  );

  const handleClose = () => {
    form.resetFields();
    setTaxRateId(null);
    navigate("/settings/tax-rates");
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    await setTaxRate(values);
    form.resetFields();
    setTaxRateId(null);
    navigate("/settings/tax-rates");
    setSubmitting(false);
  };

  const handleDelete = async () => {
    if (id) {
      setSubmitting(true);
      await deleteTaxRate(id);
      handleClose();
      setSubmitting(false);
    }
  };

  useEffect(() => {
    if (id) {
      setTaxRateId(id);
      GetTaxRateUsageCount(id)
        .then(setUsageCount)
        .catch(() => setUsageCount(0));
    } else {
      form.resetFields();
      setUsageCount(null);
    }
  }, [id, form, setTaxRateId]);

  return (
    <Drawer
      title={id ? <Trans>Edit tax rate</Trans> : <Trans>New tax rate</Trans>}
      open={true}
      placement="right"
      size={480}
      onClose={handleClose}
      footer={
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <div>
            {id &&
              (usageCount ? (
                <Tooltip title={<Trans>Cannot delete: used by {usageCount} invoice(s)</Trans>}>
                  <Button danger disabled icon={<DeleteOutlined />}>
                    <Trans>Delete</Trans>
                  </Button>
                </Tooltip>
              ) : (
                <Popconfirm
                  title={<Trans>Are you sure you want to delete this tax rate?</Trans>}
                  onConfirm={handleDelete}
                  okText={<Trans>Yes</Trans>}
                  cancelText={<Trans>No</Trans>}
                  placement="topRight"
                >
                  <Button danger icon={<DeleteOutlined />} loading={submitting}>
                    <Trans>Delete</Trans>
                  </Button>
                </Popconfirm>
              ))}
          </div>
          <Space>
            <Button onClick={handleClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="primary" loading={submitting} onClick={() => form.submit()}>
              <Trans>Save</Trans>
            </Button>
          </Space>
        </div>
      }
    >
      {(!id || (taxRate.state === "hasData" && !isEmpty(taxRate.data))) && (
        <ScrollShadow>
          <Form
            form={form}
            layout="vertical"
            onFinish={handleSubmit}
            initialValues={
              taxRate.state === "hasData" && taxRate.data ? taxRate.data : { category_code: "S" }
            }
          >
            <Section>
              <Trans>Tax rate</Trans>
            </Section>
            <Form.Item
              name="name"
              label={<Trans>Name</Trans>}
              rules={[{ required: true, message: t`Please input name!` }]}
            >
              <Input placeholder={t`Name`} />
            </Form.Item>
            <Form.Item name="description" label={<Trans>Description</Trans>}>
              <Input.TextArea rows={4} placeholder={t`Description`} />
            </Form.Item>
            <Form.Item
              name="percentage"
              label={<Trans>Percentage</Trans>}
              rules={[{ required: true, message: t`Please input a percentage!` }]}
            >
              <Input placeholder={t`Percentage`} />
            </Form.Item>
            <Form.Item name="isDefault" valuePropName="checked">
              <Checkbox>
                <Trans>Default</Trans>
              </Checkbox>
            </Form.Item>

            <Section>
              <Trans>E-invoicing</Trans>
            </Section>
            <Form.Item
              name="category_code"
              label={<Trans>VAT category</Trans>}
              rules={[{ required: true, message: t`Please select a VAT category!` }]}
            >
              <Select
                options={TAX_RATE_CATEGORY_CODES.map((code) => ({
                  value: code,
                  label: `${code} — ${categoryLabels[code]}`,
                }))}
              />
            </Form.Item>
            <Form.Item
              name="exemption_reason"
              label={<Trans>Exemption reason</Trans>}
              rules={
                exemptionReasonRequired
                  ? [
                      {
                        required: true,
                        message: t`This VAT category requires an exemption reason!`,
                      },
                    ]
                  : []
              }
            >
              <Input placeholder={t`e.g. Intra-community supply`} />
            </Form.Item>

            <Section>
              <Trans>Accounting</Trans>
            </Section>
            <Form.Item
              name="outputTaxAccountId"
              label={<Trans>Output tax account</Trans>}
              tooltip={
                <Trans>Used when this rate applies to a sales invoice — a liability account.</Trans>
              }
            >
              <Select
                allowClear
                showSearch
                placeholder={t`None`}
                options={outputTaxAccountOptions}
                optionFilterProp="label"
              />
            </Form.Item>
            <Form.Item
              name="inputTaxAccountId"
              label={<Trans>Input tax account</Trans>}
              tooltip={
                <Trans>
                  Used when this rate applies to a vendor bill — a reclaimable asset account.
                </Trans>
              }
            >
              <Select
                allowClear
                showSearch
                placeholder={t`None`}
                options={inputTaxAccountOptions}
                optionFilterProp="label"
              />
            </Form.Item>
          </Form>
        </ScrollShadow>
      )}
    </Drawer>
  );
};

export default TaxRateForm;
