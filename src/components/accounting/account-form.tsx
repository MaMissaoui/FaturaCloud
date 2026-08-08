import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { Button, Drawer, Form, Input, Popconfirm, Select, Space, Switch } from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined } from "@ant-design/icons";
import get from "lodash/get";

import { accountIdAtom, accountAtom, accountsAtom, deleteAccountAtom } from "src/atoms/account";

const ACCOUNT_TYPES = ["asset", "liability", "equity", "revenue", "expense"] as const;

const accountTypeLabel = (type: string): string => {
  switch (type) {
    case "asset":
      return t`Asset`;
    case "liability":
      return t`Liability`;
    case "equity":
      return t`Equity`;
    case "revenue":
      return t`Revenue`;
    case "expense":
      return t`Expense`;
    default:
      return type;
  }
};

const AccountForm = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [form] = Form.useForm();

  const [accountId, setAccountId] = useAtom(accountIdAtom);
  const accounts = useAtomValue(accountsAtom);
  const setAccount = useSetAtom(accountAtom);
  const deleteAccount = useSetAtom(deleteAccountAtom);
  const [submitting, setSubmitting] = useState(false);

  const isVisible = get(location.state, "accountModal", false);

  const account = useMemo(
    () => accounts.find((a) => a.id === accountId) ?? null,
    [accounts, accountId],
  );

  // A group account can't be its own ancestor — exclude it and its
  // descendants from the parent picker. Only group accounts are offered as
  // parents; leaf accounts are never postable headers.
  const parentOptions = useMemo(
    () =>
      accounts
        .filter((a) => a.isGroup && a.id !== accountId)
        .map((a) => ({ value: a.id, label: `${a.code} · ${a.name}` })),
    [accounts, accountId],
  );

  const handleClose = () => {
    setAccountId(null);
    form.resetFields();
    navigate(location.pathname, { state: { accountModal: false } });
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      await setAccount({
        ...values,
        isGroup: values.isGroup ? 1 : 0,
        isActive: values.isActive ? 1 : 0,
      });
      setAccountId(null);
      navigate(location.pathname, { state: { accountModal: false } });
      form.resetFields();
    } catch {
      // message already shown by the atom
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (accountId) {
      setSubmitting(true);
      const deleted = await deleteAccount(accountId);
      if (deleted) handleClose();
      setSubmitting(false);
    }
  };

  useEffect(() => {
    const navAccountId = get(location.state, "accountId");
    if (isVisible && navAccountId) {
      setAccountId(navAccountId);
    } else if (!isVisible) {
      setAccountId(null);
      form.resetFields();
    }
  }, [isVisible, location.state, setAccountId, form]);

  useEffect(() => {
    if (account) {
      form.setFieldsValue({
        ...account,
        isGroup: Boolean(account.isGroup),
        isActive: Boolean(account.isActive),
      });
    } else if (!accountId) {
      form.resetFields();
      form.setFieldsValue({ isActive: true, type: "asset" });
    }
  }, [account, accountId, form]);

  return (
    <Drawer
      title={accountId ? <Trans>Edit account</Trans> : <Trans>New account</Trans>}
      open={isVisible}
      placement="right"
      size={480}
      onClose={handleClose}
      footer={
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <div>
            {accountId && (
              <Popconfirm
                title={<Trans>Are you sure you want to delete this account?</Trans>}
                onConfirm={handleDelete}
                okText={<Trans>Yes</Trans>}
                cancelText={<Trans>No</Trans>}
                placement="topRight"
              >
                <Button danger icon={<DeleteOutlined />} loading={submitting}>
                  <Trans>Delete</Trans>
                </Button>
              </Popconfirm>
            )}
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
      <Form form={form} layout="vertical" onFinish={handleSubmit}>
        <Form.Item
          name="code"
          label={<Trans>Code</Trans>}
          rules={[{ required: true, message: t`Please input a code!` }]}
        >
          <Input placeholder={t`e.g. 1010`} />
        </Form.Item>
        <Form.Item
          name="name"
          label={<Trans>Name</Trans>}
          rules={[{ required: true, message: t`Please input a name!` }]}
        >
          <Input placeholder={t`e.g. Cash`} />
        </Form.Item>
        <Form.Item
          name="type"
          label={<Trans>Type</Trans>}
          rules={[{ required: true, message: t`Please select a type!` }]}
        >
          <Select>
            {ACCOUNT_TYPES.map((type) => (
              <Select.Option key={type} value={type}>
                {accountTypeLabel(type)}
              </Select.Option>
            ))}
          </Select>
        </Form.Item>
        <Form.Item name="parentId" label={<Trans>Parent account</Trans>}>
          <Select
            allowClear
            showSearch
            placeholder={t`None`}
            options={parentOptions}
            optionFilterProp="label"
          />
        </Form.Item>
        <Form.Item
          name="isGroup"
          label={<Trans>Header account</Trans>}
          valuePropName="checked"
          tooltip={
            <Trans>Header accounts organize the chart of accounts and cannot be posted to.</Trans>
          }
        >
          <Switch />
        </Form.Item>
        <Form.Item name="isActive" label={<Trans>Active</Trans>} valuePropName="checked">
          <Switch />
        </Form.Item>
        <Form.Item name="description" label={<Trans>Description</Trans>}>
          <Input.TextArea rows={2} />
        </Form.Item>
      </Form>
    </Drawer>
  );
};

export default AccountForm;
