import { useState } from "react";
import {
  Alert,
  Button,
  Card,
  Col,
  DatePicker,
  Form,
  Popconfirm,
  Row,
  Segmented,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
  Upload,
} from "antd";
import type { UploadFile } from "antd";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { DownloadOutlined, ImportOutlined, InboxOutlined, UndoOutlined } from "@ant-design/icons";
import dayjs, { type Dayjs } from "dayjs";

import {
  DownloadLoanImportTemplate,
  GetLoanImports,
  UndoLoanImport,
  UploadLoanRegister,
  type LoanImportBatch,
  type LoanImportProblem,
  type LoanImportReport,
} from "src/api";
import { isOrgAdminAtom, organizationAtom, organizationIdAtom } from "src/atoms/organization";
import { useFetch } from "src/hooks/useFetch";
import { formatOrgCents } from "src/utils/currencies";
import { calendarDayMs, useDateFormatter, useDatePickerFormat } from "src/utils/date";
import { message } from "src/utils/message";

const { Title, Text, Paragraph } = Typography;

// Stable empty value while nothing is loaded (see useFetch).
const NO_BATCHES: LoanImportBatch[] = [];

const severityColor: Record<LoanImportProblem["severity"], string> = {
  error: "red",
  warning: "orange",
  skipped: "default",
};

// The paper loan register import (db/loan_import.go,
// docs/loan-register-migration-plan.md): download the template, fill it from
// the register, check it (a dry run that writes nothing), then import it as
// one batch — which can be undone until one of its loans is collected.
const SettingsLoanImport = () => {
  const { i18n } = useLingui();
  const organizationId = useAtomValue(organizationIdAtom);
  const organization = useAtomValue(organizationAtom);
  const isOrgAdmin = useAtomValue(isOrgAdminAtom);
  const formatDate = useDateFormatter();
  const dateFormat = useDatePickerFormat();
  const money = (cents: number) => formatOrgCents(cents, organization, i18n.locale);

  const [file, setFile] = useState<File | null>(null);
  const [cutover, setCutover] = useState<Dayjs>(() => dayjs());
  // The last report and what it was for: the Import button only trusts a
  // check of this exact file and cutover date.
  const [checked, setChecked] = useState<{
    file: File;
    cutoverDay: number;
    report: LoanImportReport;
  } | null>(null);
  const [checking, setChecking] = useState(false);
  const [importing, setImporting] = useState(false);
  const [problemFilter, setProblemFilter] = useState<"all" | LoanImportProblem["severity"]>("all");

  const cutoverDay = calendarDayMs(cutover);
  const report =
    checked && checked.file === file && checked.cutoverDay === cutoverDay ? checked.report : null;
  const canImport = !!report && !report.imported && report.errors === 0 && report.loans > 0;

  const {
    data: batches,
    loading: batchesLoading,
    reload: reloadBatches,
  } = useFetch<LoanImportBatch[]>(
    organizationId && isOrgAdmin ? [organizationId] : null,
    () => GetLoanImports(organizationId!),
    NO_BATCHES,
  );

  if (!isOrgAdmin) {
    return (
      <Alert
        type="warning"
        showIcon
        message={<Trans>Only an organization admin can import a loan register.</Trans>}
      />
    );
  }

  const run = async (dryRun: boolean) => {
    if (!organizationId || !file) return;
    const setBusy = dryRun ? setChecking : setImporting;
    setBusy(true);
    try {
      const result = await UploadLoanRegister(organizationId, file, cutoverDay, dryRun);
      setChecked({ file, cutoverDay, report: result });
      setProblemFilter(result.errors > 0 ? "error" : "all");
      if (result.imported) {
        message.success(t`${result.loans} loan(s) imported.`);
        reloadBatches();
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Upload failed`);
    } finally {
      setBusy(false);
    }
  };

  const handleUndo = async (batch: LoanImportBatch) => {
    try {
      const result = await UndoLoanImport(batch.id);
      message.success(
        t`Import undone: ${result.loansRemoved} loan(s) and ${result.customersRemoved} customer(s) removed.`,
      );
      reloadBatches();
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Undo failed`);
    }
  };

  const problems = (report?.problems ?? []).filter(
    (p) => problemFilter === "all" || p.severity === problemFilter,
  );

  return (
    <div style={{ maxWidth: 1100 }}>
      <Title level={3} style={{ marginTop: 0, marginBottom: 20 }}>
        <ImportOutlined style={{ marginRight: 8 }} />
        <Trans>Import loan register</Trans>
      </Title>

      <Space direction="vertical" size="middle" style={{ width: "100%" }}>
        <Card title={<Trans>1. Fill in the template</Trans>}>
          <Space direction="vertical" size={12} style={{ width: "100%" }}>
            <Text type="secondary">
              <Trans>
                One row per item of each loan, open and settled, with the loan's reference from the
                paper register. Its second sheet explains every column. Loans come in as receivables
                brought forward: no stock, revenue or cash moves.
              </Trans>
            </Text>
            <Button
              icon={<DownloadOutlined />}
              onClick={() =>
                organizationId &&
                DownloadLoanImportTemplate(organizationId).catch((error) =>
                  message.error(error instanceof Error ? error.message : t`Download failed`),
                )
              }
            >
              <Trans>Download template</Trans>
            </Button>
          </Space>
        </Card>

        <Card title={<Trans>2. Check, then import</Trans>}>
          <Form layout="vertical">
            <Row gutter={24}>
              <Col xs={24} md={8}>
                <Form.Item
                  label={<Trans>Cutover date</Trans>}
                  tooltip={t`The day the app takes over from the paper register. What's still owed on open loans is brought forward on this date.`}
                >
                  <DatePicker
                    value={cutover}
                    format={dateFormat}
                    allowClear={false}
                    onChange={(value) => value && setCutover(value)}
                    style={{ width: "100%" }}
                  />
                </Form.Item>
              </Col>
              <Col xs={24} md={16}>
                <Form.Item label={<Trans>Register file</Trans>}>
                  <Upload.Dragger
                    accept=".xlsx"
                    maxCount={1}
                    beforeUpload={(f) => {
                      setFile(f);
                      return false;
                    }}
                    onRemove={() => setFile(null)}
                    fileList={
                      file
                        ? [{ uid: "register", name: file.name, status: "done" } as UploadFile]
                        : []
                    }
                  >
                    <p className="ant-upload-drag-icon">
                      <InboxOutlined />
                    </p>
                    <p className="ant-upload-text">
                      <Trans>Drop the filled template here, or click to choose it</Trans>
                    </p>
                  </Upload.Dragger>
                </Form.Item>
              </Col>
            </Row>
            <Space>
              <Button
                type={canImport ? "default" : "primary"}
                disabled={!file}
                loading={checking}
                onClick={() => run(true)}
              >
                <Trans>Check file</Trans>
              </Button>
              <Popconfirm
                title={t`Import ${report?.loans ?? 0} loan(s)?`}
                description={t`They can be undone from the list below until one of them is collected.`}
                onConfirm={() => run(false)}
                okText={t`Import`}
                cancelText={t`Cancel`}
                disabled={!canImport}
              >
                <Button type="primary" disabled={!canImport} loading={importing}>
                  <Trans>Import</Trans>
                </Button>
              </Popconfirm>
            </Space>
          </Form>

          {report && (
            <div style={{ marginTop: 24 }}>
              {report.imported ? (
                <Alert
                  type="success"
                  showIcon
                  message={
                    <Trans>Imported. The loans are now in the Cash Book's Loan status.</Trans>
                  }
                />
              ) : report.errors > 0 ? (
                <Alert
                  type="error"
                  showIcon
                  message={
                    <Trans>
                      {report.errors} problem(s) to fix before importing — nothing was written.
                    </Trans>
                  }
                />
              ) : report.loans === 0 ? (
                <Alert
                  type="info"
                  showIcon
                  message={<Trans>There is nothing new to import in this file.</Trans>}
                />
              ) : (
                <Alert
                  type="success"
                  showIcon
                  message={
                    <Trans>
                      Ready to import. Compare the totals per customer with the paper register
                      first.
                    </Trans>
                  }
                />
              )}

              <Row gutter={[24, 16]} style={{ marginTop: 16 }}>
                <Col xs={12} md={4}>
                  <Statistic title={t`Loans`} value={report.loans} />
                  <Text type="secondary">
                    <Trans>
                      {report.openLoans} open · {report.settledLoans} settled
                    </Trans>
                  </Text>
                </Col>
                <Col xs={12} md={4}>
                  <Statistic
                    title={t`Customers`}
                    value={report.customersMatched + report.customersCreated}
                  />
                  <Text type="secondary">
                    <Trans>{report.customersCreated} new</Trans>
                  </Text>
                </Col>
                <Col xs={12} md={4}>
                  <Statistic title={t`Total`} value={money(report.total)} />
                </Col>
                <Col xs={12} md={4}>
                  <Statistic title={t`Paid`} value={money(report.paid)} />
                </Col>
                <Col xs={12} md={4}>
                  <Statistic title={t`Outstanding`} value={money(report.outstanding)} />
                </Col>
                <Col xs={12} md={4}>
                  <Statistic title={t`Already imported`} value={report.skippedLoans} />
                </Col>
              </Row>

              {report.problems.length > 0 && (
                <Card
                  size="small"
                  className="card-head-wrap"
                  style={{ marginTop: 16 }}
                  title={<Trans>Problems</Trans>}
                  extra={
                    <Segmented
                      size="small"
                      value={problemFilter}
                      onChange={(value) => setProblemFilter(value as typeof problemFilter)}
                      options={[
                        { label: t`All`, value: "all" },
                        { label: t`Errors (${report.errors})`, value: "error" },
                        { label: t`Warnings (${report.warnings})`, value: "warning" },
                        { label: t`Skipped (${report.skippedLoans})`, value: "skipped" },
                      ]}
                    />
                  }
                >
                  <Table
                    size="small"
                    rowKey={(p) => `${p.row}-${p.ref}-${p.message}`}
                    dataSource={problems}
                    pagination={{ pageSize: 20, hideOnSinglePage: true }}
                    columns={[
                      {
                        title: t`Row`,
                        dataIndex: "row",
                        width: 70,
                        render: (row: number) => row || "—",
                      },
                      { title: t`Loan`, dataIndex: "ref", width: 140 },
                      {
                        title: t`Type`,
                        dataIndex: "severity",
                        width: 110,
                        render: (severity: LoanImportProblem["severity"]) => (
                          <Tag color={severityColor[severity]}>
                            {severity === "error"
                              ? t`Error`
                              : severity === "warning"
                                ? t`Warning`
                                : t`Skipped`}
                          </Tag>
                        ),
                      },
                      { title: t`Message`, dataIndex: "message" },
                    ]}
                  />
                </Card>
              )}

              {report.customers.length > 0 && (
                <Card
                  size="small"
                  style={{ marginTop: 16 }}
                  title={<Trans>Totals per customer</Trans>}
                >
                  <Table
                    size="small"
                    rowKey={(c) => `${c.name}-${c.cin}-${c.phone}`}
                    dataSource={report.customers}
                    pagination={{ pageSize: 20, hideOnSinglePage: true }}
                    columns={[
                      {
                        title: t`Customer`,
                        dataIndex: "name",
                        render: (name: string, c) => (
                          <Space size={4}>
                            {name}
                            {c.new && <Tag color="blue">{t`New`}</Tag>}
                          </Space>
                        ),
                      },
                      { title: t`CIN`, dataIndex: "cin" },
                      { title: t`Phone`, dataIndex: "phone" },
                      { title: t`Loans`, dataIndex: "loans", align: "right" },
                      { title: t`Total`, dataIndex: "total", align: "right", render: money },
                      { title: t`Paid`, dataIndex: "paid", align: "right", render: money },
                      {
                        title: t`Outstanding`,
                        dataIndex: "outstanding",
                        align: "right",
                        render: money,
                      },
                    ]}
                  />
                </Card>
              )}
            </div>
          )}
        </Card>

        <Card title={<Trans>Imports</Trans>}>
          <Paragraph type="secondary" style={{ marginTop: 0 }}>
            <Trans>
              Undoing an import removes its loans, and the customers it created if nothing else
              refers to them. It's no longer possible once one of its loans has been collected.
            </Trans>
          </Paragraph>
          <Table
            size="small"
            rowKey="id"
            loading={batchesLoading}
            dataSource={batches}
            pagination={{ pageSize: 10, hideOnSinglePage: true }}
            columns={[
              { title: t`Imported`, dataIndex: "createdAt", render: (v: number) => formatDate(v) },
              { title: t`File`, dataIndex: "fileName", render: (v: string | null) => v ?? "—" },
              { title: t`Cutover`, dataIndex: "cutoverDate", render: (v: number) => formatDate(v) },
              { title: t`Loans`, dataIndex: "loanCount", align: "right" },
              { title: t`New customers`, dataIndex: "customersCreated", align: "right" },
              { title: t`Outstanding`, dataIndex: "outstanding", align: "right", render: money },
              {
                key: "actions",
                align: "right",
                render: (_: unknown, batch: LoanImportBatch) =>
                  batch.undoneAt ? (
                    <Tag>
                      <Trans>Undone {formatDate(batch.undoneAt)}</Trans>
                    </Tag>
                  ) : (
                    <Popconfirm
                      title={t`Undo this import?`}
                      description={t`Its loans are removed; their opening entries are reversed.`}
                      onConfirm={() => handleUndo(batch)}
                      okText={t`Undo`}
                      cancelText={t`Cancel`}
                    >
                      <Button size="small" icon={<UndoOutlined />}>
                        <Trans>Undo</Trans>
                      </Button>
                    </Popconfirm>
                  ),
              },
            ]}
          />
        </Card>
      </Space>
    </div>
  );
};

export default SettingsLoanImport;
