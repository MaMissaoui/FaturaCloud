import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import {
  Alert,
  Button,
  Card,
  Col,
  DatePicker,
  Descriptions,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { loadable } from "src/utils/loadable";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { DeleteOutlined, PlusOutlined, SaveOutlined, UndoOutlined } from "@ant-design/icons";
import dayjs from "dayjs";
import find from "lodash/find";
import sum from "lodash/sum";

import { useDatePickerFormat, useDateFormatter } from "src/utils/date";
import { journalEntryStatusColor, journalEntryStatusLabel } from "src/types/journal-entry";
import { accountsAtom, setAccountsAtom } from "src/atoms/account";
import { journalsAtom, setJournalsAtom } from "src/atoms/journal";
import { fiscalYearsAtom, setFiscalYearsAtom } from "src/atoms/fiscal-period";
import {
  journalEntryIdAtom,
  journalEntryAtom,
  postJournalEntryAtom,
  reverseJournalEntryAtom,
  deleteJournalEntryAtom,
} from "src/atoms/journal-entry";

// journalEntryAtom is async; reading it with plain useAtom would suspend to
// the app's top-level Suspense boundary every time journalEntryIdAtom
// changes, unmounting/remounting this route — same fix as
// src/routes/purchase-orders/details.tsx.
const loadableEntryAtom = loadable(journalEntryAtom);

const JournalEntryDetails = () => {
  useLingui();
  const { id } = useParams<string>();
  const navigate = useNavigate();
  const dateFormat = useDatePickerFormat();
  const formatDate = useDateFormatter();

  const isNew = id === "new";

  const accounts = useAtomValue(accountsAtom);
  const setAccounts = useSetAtom(setAccountsAtom);
  const journals = useAtomValue(journalsAtom);
  const setJournals = useSetAtom(setJournalsAtom);
  const fiscalYears = useAtomValue(fiscalYearsAtom);
  const setFiscalYears = useSetAtom(setFiscalYearsAtom);

  const [entryId, setEntryId] = useAtom(journalEntryIdAtom);
  const entryLoadable = useAtomValue(loadableEntryAtom);
  const setEntry = useSetAtom(journalEntryAtom);
  const entry = entryLoadable.state === "hasData" ? entryLoadable.data : undefined;
  const postEntry = useSetAtom(postJournalEntryAtom);
  const reverseEntry = useSetAtom(reverseJournalEntryAtom);
  const deleteEntry = useSetAtom(deleteJournalEntryAtom);

  const [form] = Form.useForm();
  const [submitting, setSubmitting] = useState(false);
  const [reverseModalOpen, setReverseModalOpen] = useState(false);
  const [reverseForm] = Form.useForm();

  useEffect(() => {
    setAccounts();
    setJournals();
    setFiscalYears();
    if (!isNew) setEntryId(id ?? null);
    return () => setEntryId(null);
  }, [id, isNew, setAccounts, setJournals, setFiscalYears, setEntryId]);

  useEffect(() => {
    if (isNew && entryId) {
      navigate(`/accounting/journal-entries/${entryId}`);
    }
  }, [isNew, entryId, navigate]);

  const postableAccounts = accounts.filter((a) => !a.isGroup);

  const lines = Form.useWatch("lines", form) ?? [];
  const totalDebit = sum(lines.map((l: any) => Number(l?.debit) || 0));
  const totalCredit = sum(lines.map((l: any) => Number(l?.credit) || 0));
  const isBalanced = lines.length > 0 && Math.abs(totalDebit - totalCredit) < 0.005;

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      await setEntry(values);
    } finally {
      setSubmitting(false);
    }
  };

  const handlePost = async () => {
    if (!id) return;
    setSubmitting(true);
    await postEntry(id);
    setSubmitting(false);
  };

  const handleDelete = async () => {
    if (!id) return;
    const success = await deleteEntry(id);
    if (success) navigate("/accounting/journal-entries");
  };

  const handleReverse = async (values: any) => {
    if (!id) return;
    setSubmitting(true);
    const reversal = await reverseEntry({
      entryId: id,
      reason: values.reason,
      date: values.date.valueOf(),
    });
    setSubmitting(false);
    if (reversal) {
      setReverseModalOpen(false);
      reverseForm.resetFields();
      navigate(`/accounting/journal-entries/${reversal.id}`);
    }
  };

  if (isNew) {
    if (fiscalYears.length === 0) {
      return (
        <Alert
          type="warning"
          showIcon
          message={<Trans>No open fiscal year</Trans>}
          description={
            <Trans>
              A journal entry can only be posted within an open fiscal year. Create one under{" "}
              <Link to="/accounting/fiscal-periods">Fiscal Periods</Link> first.
            </Trans>
          }
        />
      );
    }

    return (
      <Card
        title={<Trans>New journal entry</Trans>}
        extra={
          <Button
            type="primary"
            icon={<SaveOutlined />}
            loading={submitting}
            onClick={() => form.submit()}
          >
            <Trans>Save as draft</Trans>
          </Button>
        }
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={handleSubmit}
          initialValues={{ date: dayjs(), lines: [{}, {}] }}
        >
          <Row gutter={16}>
            <Col xs={24} md={8}>
              <Form.Item
                name="journalId"
                label={<Trans>Journal</Trans>}
                rules={[{ required: true, message: t`Please select a journal!` }]}
              >
                <Select
                  placeholder={t`Select a journal`}
                  options={journals.map((j) => ({ value: j.id, label: `${j.code} · ${j.name}` }))}
                />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item
                name="date"
                label={<Trans>Date</Trans>}
                rules={[{ required: true, message: t`Please select a date!` }]}
              >
                <DatePicker style={{ width: "100%" }} format={dateFormat} />
              </Form.Item>
            </Col>
            <Col xs={24} md={8}>
              <Form.Item name="reference" label={<Trans>Reference</Trans>}>
                <Input placeholder={t`Optional reference`} />
              </Form.Item>
            </Col>
            <Col xs={24}>
              <Form.Item
                name="description"
                label={<Trans>Description</Trans>}
                rules={[{ required: true, message: t`Please input a description!` }]}
              >
                <Input placeholder={t`e.g. Bank fee`} />
              </Form.Item>
            </Col>
          </Row>

          <Form.List name="lines">
            {(fields, { add, remove }) => (
              <>
                <Table
                  dataSource={fields}
                  pagination={false}
                  rowKey="key"
                  size="small"
                  footer={() => (
                    <Button size="small" icon={<PlusOutlined />} onClick={() => add({})}>
                      <Trans>Add line</Trans>
                    </Button>
                  )}
                >
                  <Table.Column
                    title={<Trans>Account</Trans>}
                    key="accountId"
                    render={(field) => (
                      <Form.Item
                        name={[field.name, "accountId"]}
                        style={{ marginBottom: 0 }}
                        rules={[{ required: true, message: t`Required` }]}
                      >
                        <Select
                          showSearch
                          placeholder={t`Select an account`}
                          style={{ minWidth: 220 }}
                          optionFilterProp="label"
                          options={postableAccounts.map((a) => ({
                            value: a.id,
                            label: `${a.code} · ${a.name}`,
                          }))}
                        />
                      </Form.Item>
                    )}
                  />
                  <Table.Column
                    title={<Trans>Description</Trans>}
                    key="description"
                    render={(field) => (
                      <Form.Item name={[field.name, "description"]} style={{ marginBottom: 0 }}>
                        <Input placeholder={t`Optional`} />
                      </Form.Item>
                    )}
                  />
                  <Table.Column
                    title={<Trans>Debit</Trans>}
                    key="debit"
                    width={140}
                    render={(field) => (
                      <Form.Item name={[field.name, "debit"]} style={{ marginBottom: 0 }}>
                        <InputNumber
                          min={0}
                          precision={2}
                          style={{ width: "100%" }}
                          onChange={(v) => {
                            if (v) form.setFieldValue(["lines", field.name, "credit"], undefined);
                          }}
                        />
                      </Form.Item>
                    )}
                  />
                  <Table.Column
                    title={<Trans>Credit</Trans>}
                    key="credit"
                    width={140}
                    render={(field) => (
                      <Form.Item name={[field.name, "credit"]} style={{ marginBottom: 0 }}>
                        <InputNumber
                          min={0}
                          precision={2}
                          style={{ width: "100%" }}
                          onChange={(v) => {
                            if (v) form.setFieldValue(["lines", field.name, "debit"], undefined);
                          }}
                        />
                      </Form.Item>
                    )}
                  />
                  <Table.Column
                    key="actions"
                    width={50}
                    render={(field) => (
                      <Button
                        type="text"
                        danger
                        icon={<DeleteOutlined />}
                        onClick={() => remove(field.name)}
                      />
                    )}
                  />
                </Table>
              </>
            )}
          </Form.List>

          <Row justify="end" style={{ marginTop: 16 }}>
            <Col>
              <Space size="large">
                <Typography.Text>
                  <Trans>Debit</Trans>: {totalDebit.toFixed(2)}
                </Typography.Text>
                <Typography.Text>
                  <Trans>Credit</Trans>: {totalCredit.toFixed(2)}
                </Typography.Text>
                <Tag color={isBalanced ? "green" : "volcano"}>
                  {isBalanced ? <Trans>Balanced</Trans> : <Trans>Not balanced</Trans>}
                </Tag>
              </Space>
            </Col>
          </Row>
        </Form>
      </Card>
    );
  }

  if (!entry) return null;

  const journal = find(journals, { id: entry.journalId });

  return (
    <Card
      title={
        <Space>
          <span>
            <Trans>Journal entry</Trans> {entry.entryNumber ? `#${entry.entryNumber}` : ""}
          </span>
          <Tag
            color={journalEntryStatusColor[entry.status as keyof typeof journalEntryStatusColor]}
          >
            {journalEntryStatusLabel(entry.status)}
          </Tag>
        </Space>
      }
      extra={
        <Space>
          {entry.status === "draft" && (
            <>
              <Popconfirm
                title={<Trans>Post this entry? It cannot be edited afterward.</Trans>}
                onConfirm={handlePost}
                okText={<Trans>Yes</Trans>}
                cancelText={<Trans>No</Trans>}
              >
                <Button type="primary" loading={submitting}>
                  <Trans>Post</Trans>
                </Button>
              </Popconfirm>
              <Popconfirm
                title={<Trans>Delete this draft?</Trans>}
                onConfirm={handleDelete}
                okText={<Trans>Yes</Trans>}
                cancelText={<Trans>No</Trans>}
              >
                <Button danger icon={<DeleteOutlined />}>
                  <Trans>Delete</Trans>
                </Button>
              </Popconfirm>
            </>
          )}
          {entry.status === "posted" && (
            <Button icon={<UndoOutlined />} onClick={() => setReverseModalOpen(true)}>
              <Trans>Reverse</Trans>
            </Button>
          )}
        </Space>
      }
    >
      <Descriptions column={2} size="small" style={{ marginBottom: 16 }}>
        <Descriptions.Item label={<Trans>Journal</Trans>}>
          {journal ? `${journal.code} · ${journal.name}` : "—"}
        </Descriptions.Item>
        <Descriptions.Item label={<Trans>Date</Trans>}>
          {formatDate(entry.date.valueOf ? entry.date.valueOf() : entry.date)}
        </Descriptions.Item>
        <Descriptions.Item label={<Trans>Reference</Trans>}>
          {entry.reference ?? "—"}
        </Descriptions.Item>
        <Descriptions.Item label={<Trans>Description</Trans>}>
          {entry.description}
        </Descriptions.Item>
        {entry.reversalOfEntryId && (
          <Descriptions.Item label={<Trans>Reverses</Trans>}>
            <Link to={`/accounting/journal-entries/${entry.reversalOfEntryId}`}>
              {entry.reversalOfEntryId}
            </Link>
          </Descriptions.Item>
        )}
        {entry.reversalReason && (
          <Descriptions.Item label={<Trans>Reversal reason</Trans>}>
            {entry.reversalReason}
          </Descriptions.Item>
        )}
      </Descriptions>

      <Table dataSource={entry.lines} pagination={false} rowKey="id" size="small">
        <Table.Column
          title={<Trans>Account</Trans>}
          dataIndex="accountId"
          key="accountId"
          render={(accountId: string) => {
            const account = find(accounts, { id: accountId });
            return account ? `${account.code} · ${account.name}` : accountId;
          }}
        />
        <Table.Column
          title={<Trans>Description</Trans>}
          dataIndex="description"
          key="description"
          render={(v: string | null) => v ?? "—"}
        />
        <Table.Column
          title={<Trans>Debit</Trans>}
          dataIndex="debit"
          key="debit"
          align="right"
          render={(v: number) => (v ? v.toFixed(2) : "—")}
        />
        <Table.Column
          title={<Trans>Credit</Trans>}
          dataIndex="credit"
          key="credit"
          align="right"
          render={(v: number) => (v ? v.toFixed(2) : "—")}
        />
      </Table>

      <Modal
        title={<Trans>Reverse journal entry</Trans>}
        open={reverseModalOpen}
        onCancel={() => setReverseModalOpen(false)}
        onOk={() => reverseForm.submit()}
        confirmLoading={submitting}
        destroyOnHidden
      >
        <Form
          form={reverseForm}
          layout="vertical"
          onFinish={handleReverse}
          initialValues={{ date: dayjs() }}
        >
          <Form.Item
            name="date"
            label={<Trans>Reversal date</Trans>}
            rules={[{ required: true, message: t`Please select a date!` }]}
          >
            <DatePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
          <Form.Item
            name="reason"
            label={<Trans>Reason</Trans>}
            rules={[
              { required: true, message: t`Please explain why this entry is being reversed!` },
            ]}
          >
            <Input.TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

export default JournalEntryDetails;
