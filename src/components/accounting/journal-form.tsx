import { useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import { Button, Drawer, Form, Input, Popconfirm, Select, Space, Typography } from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined } from "@ant-design/icons";
import get from "lodash/get";

import { journalIdAtom, journalAtom, journalsAtom, deleteJournalAtom } from "src/atoms/journal";

const JOURNAL_TYPES = ["sales", "purchases", "cash", "bank", "miscellaneous"] as const;

const journalTypeLabel = (type: string): string => {
  switch (type) {
    case "sales":
      return t`Sales`;
    case "purchases":
      return t`Purchases`;
    case "cash":
      return t`Cash`;
    case "bank":
      return t`Bank`;
    case "miscellaneous":
      return t`Miscellaneous`;
    default:
      return type;
  }
};

const JournalForm = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const [form] = Form.useForm();

  const [journalId, setJournalId] = useAtom(journalIdAtom);
  const journals = useAtomValue(journalsAtom);
  const setJournal = useSetAtom(journalAtom);
  const deleteJournal = useSetAtom(deleteJournalAtom);
  const [submitting, setSubmitting] = useState(false);

  const isVisible = get(location.state, "journalModal", false);

  const journal = useMemo(
    () => journals.find((j) => j.id === journalId) ?? null,
    [journals, journalId],
  );

  const handleClose = () => {
    setJournalId(null);
    form.resetFields();
    navigate(location.pathname, { state: { journalModal: false } });
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      await setJournal(values);
      setJournalId(null);
      navigate(location.pathname, { state: { journalModal: false } });
      form.resetFields();
    } catch {
      // message already shown by the atom
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (journalId) {
      setSubmitting(true);
      const deleted = await deleteJournal(journalId);
      if (deleted) handleClose();
      setSubmitting(false);
    }
  };

  useEffect(() => {
    const navJournalId = get(location.state, "journalId");
    if (isVisible && navJournalId) {
      setJournalId(navJournalId);
    } else if (!isVisible) {
      setJournalId(null);
      form.resetFields();
    }
  }, [isVisible, location.state, setJournalId, form]);

  useEffect(() => {
    if (journal) {
      form.setFieldsValue(journal);
    } else if (!journalId) {
      form.resetFields();
    }
  }, [journal, journalId, form]);

  return (
    <Drawer
      title={journalId ? <Trans>Edit journal</Trans> : <Trans>New journal</Trans>}
      open={isVisible}
      placement="right"
      size={420}
      onClose={handleClose}
      footer={
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <div>
            {journalId && !journal?.isSystem && (
              <Popconfirm
                title={<Trans>Are you sure you want to delete this journal?</Trans>}
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
            {journalId && Boolean(journal?.isSystem) && (
              <Typography.Text type="secondary">
                <Trans>Built-in journals can't be deleted</Trans>
              </Typography.Text>
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
          <Input placeholder={t`e.g. VK`} disabled={Boolean(journalId)} />
        </Form.Item>
        <Form.Item
          name="name"
          label={<Trans>Name</Trans>}
          rules={[{ required: true, message: t`Please input a name!` }]}
        >
          <Input placeholder={t`e.g. Sales`} />
        </Form.Item>
        <Form.Item
          name="type"
          label={<Trans>Type</Trans>}
          rules={[{ required: true, message: t`Please select a type!` }]}
        >
          <Select disabled={Boolean(journalId)}>
            {JOURNAL_TYPES.map((type) => (
              <Select.Option key={type} value={type}>
                {journalTypeLabel(type)}
              </Select.Option>
            ))}
          </Select>
        </Form.Item>
      </Form>
    </Drawer>
  );
};

export default JournalForm;
