import { useEffect, useState } from "react";
import { useParams, useNavigate } from "react-router";
import { Button, Checkbox, Drawer, Form, Input, Popconfirm, Space, Tooltip, theme, Typography } from "antd";
import { useSetAtom, useAtomValue } from "jotai";
import { loadable } from "jotai/utils";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined } from "@ant-design/icons";
import isEmpty from "lodash/isEmpty";
import { GetTaxRateUsageCount } from "src/api";

import { taxRateIdAtom, taxRateAtom, deleteTaxRateAtom } from "src/atoms/tax-rate";

const loadableTaxRateAtom = loadable(taxRateAtom);

const Section = ({ children }: { children: React.ReactNode }) => {
  const { token } = theme.useToken();
  return (
    <Typography.Text
      strong
      style={{ color: token.colorPrimary, display: "block", marginBottom: 12, marginTop: 4 }}
    >
      {children}
    </Typography.Text>
  );
};

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
      width={480}
      onClose={handleClose}
      footer={
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <div>
            {id && (
              usageCount ? (
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
              )
            )}
          </div>
          <Space>
            <Button onClick={handleClose}><Trans>Cancel</Trans></Button>
            <Button type="primary" loading={submitting} onClick={() => form.submit()}>
              <Trans>Save</Trans>
            </Button>
          </Space>
        </div>
      }
    >
      {(!id || (taxRate.state === "hasData" && !isEmpty(taxRate.data))) && (
        <Form
          form={form}
          layout="vertical"
          onFinish={handleSubmit}
          initialValues={taxRate.state === "hasData" ? taxRate.data : undefined}
        >
          <Section><Trans>Tax rate</Trans></Section>
          <Form.Item name="name" label={<Trans>Name</Trans>} rules={[{ required: true, message: t`Please input name!` }]}>
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
            <Checkbox><Trans>Default</Trans></Checkbox>
          </Form.Item>
        </Form>
      )}
    </Drawer>
  );
};

export default TaxRateForm;
