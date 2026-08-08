import { useEffect, useState } from "react";
import type { FiscalYear, FiscalPeriod } from "src/types/models";
import { useLocation } from "react-router";
import {
  Alert,
  Button,
  Col,
  DatePicker,
  Form,
  Input,
  Modal,
  Popconfirm,
  Row,
  Table,
  Tag,
} from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { CalendarOutlined, PlusOutlined } from "@ant-design/icons";
import dayjs from "dayjs";

import {
  fiscalYearsAtom,
  setFiscalYearsAtom,
  createFiscalYearAtom,
  fiscalPeriodsAtom,
  loadFiscalPeriodsAtom,
  createFiscalPeriodAtom,
  updateFiscalPeriodStatusAtom,
} from "src/atoms/fiscal-period";
import PageHeader from "src/components/page-header";
import { useDatePickerFormat } from "src/utils/date";

const FiscalPeriods = () => {
  useLingui();
  const location = useLocation();
  const dateFormat = useDatePickerFormat();

  const fiscalYears = useAtomValue(fiscalYearsAtom);
  const setFiscalYears = useSetAtom(setFiscalYearsAtom);
  const createFiscalYear = useSetAtom(createFiscalYearAtom);
  const fiscalPeriodsByYear = useAtomValue(fiscalPeriodsAtom);
  const loadFiscalPeriods = useSetAtom(loadFiscalPeriodsAtom);
  const createFiscalPeriod = useSetAtom(createFiscalPeriodAtom);
  const updateFiscalPeriodStatus = useSetAtom(updateFiscalPeriodStatusAtom);

  const [loading, setLoading] = useState(false);
  const [yearModalOpen, setYearModalOpen] = useState(false);
  const [yearSubmitting, setYearSubmitting] = useState(false);
  const [yearForm] = Form.useForm();

  const [periodModalYearId, setPeriodModalYearId] = useState<string | null>(null);
  const [periodSubmitting, setPeriodSubmitting] = useState(false);
  const [periodForm] = Form.useForm();

  useEffect(() => {
    if (location.pathname === "/accounting/fiscal-periods") {
      setLoading(true);
      setFiscalYears().finally(() => setLoading(false));
    }
  }, [location, setFiscalYears]);

  const handleCreateYear = async (values: any) => {
    setYearSubmitting(true);
    const created = await createFiscalYear({
      name: values.name,
      startDate: values.range[0].startOf("day").valueOf(),
      endDate: values.range[1].endOf("day").valueOf(),
    });
    setYearSubmitting(false);
    if (created) {
      setYearModalOpen(false);
      yearForm.resetFields();
    }
  };

  const handleCreatePeriod = async (values: any) => {
    if (!periodModalYearId) return;
    setPeriodSubmitting(true);
    const created = await createFiscalPeriod({
      fiscalYearId: periodModalYearId,
      name: values.name,
      startDate: values.range[0].startOf("day").valueOf(),
      endDate: values.range[1].endOf("day").valueOf(),
    });
    setPeriodSubmitting(false);
    if (created) {
      setPeriodModalYearId(null);
      periodForm.resetFields();
    }
  };

  return (
    <>
      <PageHeader
        icon={<CalendarOutlined />}
        title={<Trans>Fiscal Periods</Trans>}
        actions={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setYearModalOpen(true)}>
            <Trans>New fiscal year</Trans>
          </Button>
        }
      />

      {fiscalYears.length === 0 && !loading && (
        <Alert
          style={{ marginTop: 16 }}
          type="info"
          showIcon
          message={<Trans>No fiscal year yet</Trans>}
          description={
            <Trans>
              Journal entries can only be posted within an open fiscal year. Create one to start
              using the general ledger.
            </Trans>
          }
        />
      )}

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={fiscalYears}
            pagination={{ hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            expandable={{
              onExpand: (expanded, year) => {
                if (expanded) loadFiscalPeriods(year.id);
              },
              expandedRowRender: (year: FiscalYear) => {
                const periods = fiscalPeriodsByYear[year.id] ?? [];
                return (
                  <div>
                    <Table
                      dataSource={periods}
                      pagination={false}
                      rowKey="id"
                      size="small"
                      footer={() => (
                        <Button
                          size="small"
                          icon={<PlusOutlined />}
                          onClick={() => setPeriodModalYearId(year.id)}
                        >
                          <Trans>New period</Trans>
                        </Button>
                      )}
                    >
                      <Table.Column title={<Trans>Name</Trans>} dataIndex="name" key="name" />
                      <Table.Column
                        title={<Trans>Start</Trans>}
                        dataIndex="startDate"
                        key="startDate"
                        render={(v: number) => dayjs(v).format(dateFormat)}
                      />
                      <Table.Column
                        title={<Trans>End</Trans>}
                        dataIndex="endDate"
                        key="endDate"
                        render={(v: number) => dayjs(v).format(dateFormat)}
                      />
                      <Table.Column
                        title={<Trans>Status</Trans>}
                        dataIndex="status"
                        key="status"
                        render={(status: string) => (
                          <Tag color={status === "open" ? "green" : "default"}>
                            {status === "open" ? t`Open` : t`Closed`}
                          </Tag>
                        )}
                      />
                      <Table.Column
                        key="actions"
                        render={(period: FiscalPeriod) => (
                          <Popconfirm
                            title={
                              period.status === "open" ? (
                                <Trans>Close this period? New entries will be blocked.</Trans>
                              ) : (
                                <Trans>Reopen this period?</Trans>
                              )
                            }
                            onConfirm={() =>
                              updateFiscalPeriodStatus({
                                id: period.id,
                                status: period.status === "open" ? "closed" : "open",
                              })
                            }
                            okText={<Trans>Yes</Trans>}
                            cancelText={<Trans>No</Trans>}
                          >
                            <Button size="small">
                              {period.status === "open" ? (
                                <Trans>Close</Trans>
                              ) : (
                                <Trans>Reopen</Trans>
                              )}
                            </Button>
                          </Popconfirm>
                        )}
                      />
                    </Table>
                  </div>
                );
              },
            }}
          >
            <Table.Column title={<Trans>Name</Trans>} dataIndex="name" key="name" />
            <Table.Column
              title={<Trans>Start</Trans>}
              dataIndex="startDate"
              key="startDate"
              render={(v: number) => dayjs(v).format(dateFormat)}
            />
            <Table.Column
              title={<Trans>End</Trans>}
              dataIndex="endDate"
              key="endDate"
              render={(v: number) => dayjs(v).format(dateFormat)}
            />
            <Table.Column
              title={<Trans>Status</Trans>}
              dataIndex="status"
              key="status"
              render={(status: string) => (
                <Tag color={status === "open" ? "green" : "default"}>
                  {status === "open" ? t`Open` : t`Closed`}
                </Tag>
              )}
            />
          </Table>
        </Col>
      </Row>

      <Modal
        title={<Trans>New fiscal year</Trans>}
        open={yearModalOpen}
        onCancel={() => setYearModalOpen(false)}
        onOk={() => yearForm.submit()}
        confirmLoading={yearSubmitting}
        destroyOnHidden
      >
        <Form form={yearForm} layout="vertical" onFinish={handleCreateYear}>
          <Form.Item
            name="name"
            label={<Trans>Name</Trans>}
            rules={[{ required: true, message: t`Please input a name!` }]}
          >
            <Input placeholder={t`e.g. 2026`} />
          </Form.Item>
          <Form.Item
            name="range"
            label={<Trans>Date range</Trans>}
            rules={[{ required: true, message: t`Please select a date range!` }]}
          >
            <DatePicker.RangePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={<Trans>New fiscal period</Trans>}
        open={Boolean(periodModalYearId)}
        onCancel={() => setPeriodModalYearId(null)}
        onOk={() => periodForm.submit()}
        confirmLoading={periodSubmitting}
        destroyOnHidden
      >
        <Form form={periodForm} layout="vertical" onFinish={handleCreatePeriod}>
          <Form.Item
            name="name"
            label={<Trans>Name</Trans>}
            rules={[{ required: true, message: t`Please input a name!` }]}
          >
            <Input placeholder={t`e.g. January`} />
          </Form.Item>
          <Form.Item
            name="range"
            label={<Trans>Date range</Trans>}
            rules={[{ required: true, message: t`Please select a date range!` }]}
          >
            <DatePicker.RangePicker style={{ width: "100%" }} format={dateFormat} />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
};

export default FiscalPeriods;
