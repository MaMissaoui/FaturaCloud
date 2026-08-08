import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Row, Select, Table, Tag } from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { AuditOutlined } from "@ant-design/icons";
import find from "lodash/find";

import { useDateFormatter } from "src/utils/date";
import {
  JOURNAL_ENTRY_STATUSES,
  journalEntryStatusColor,
  journalEntryStatusLabel,
} from "src/types/journal-entry";
import {
  journalEntriesAtom,
  journalEntryFiltersAtom,
  setJournalEntriesAtom,
} from "src/atoms/journal-entry";
import { journalsAtom, setJournalsAtom } from "src/atoms/journal";
import PageHeader from "src/components/page-header";

const JournalEntries = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const formatDate = useDateFormatter();

  const entries = useAtomValue(journalEntriesAtom);
  const setEntries = useSetAtom(setJournalEntriesAtom);
  const [filters, setFilters] = useAtom(journalEntryFiltersAtom);
  const journals = useAtomValue(journalsAtom);
  const setJournals = useSetAtom(setJournalsAtom);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/accounting/journal-entries") {
      setJournals();
      setLoading(true);
      setEntries().finally(() => setLoading(false));
    }
    // filters is intentionally a dependency so changing the journal/status
    // filter re-fetches from the server.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location, setEntries, setJournals, filters]);

  return (
    <>
      <PageHeader
        icon={<AuditOutlined />}
        title={<Trans>Journal Entries</Trans>}
        extra={
          <>
            <Select
              allowClear
              placeholder={t`All journals`}
              style={{ width: 180 }}
              value={filters.journalId}
              onChange={(journalId) => setFilters({ ...filters, journalId })}
              options={journals.map((j) => ({ value: j.id, label: `${j.code} · ${j.name}` }))}
            />
            <Select
              allowClear
              placeholder={t`All statuses`}
              style={{ width: 160, marginLeft: 8 }}
              value={filters.status}
              onChange={(status) => setFilters({ ...filters, status })}
              options={JOURNAL_ENTRY_STATUSES.map((status) => ({
                value: status,
                label: journalEntryStatusLabel(status),
              }))}
            />
          </>
        }
        actions={
          <Button type="primary" onClick={() => navigate("/accounting/journal-entries/new")}>
            <Trans>New journal entry</Trans>
          </Button>
        }
      />

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={entries}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            onRow={(record: any) => ({
              onClick: () => navigate(`/accounting/journal-entries/${record.id}`),
              style: { cursor: "pointer" },
            })}
          >
            <Table.Column
              title={<Trans>Entry #</Trans>}
              dataIndex="entryNumber"
              key="entryNumber"
              width={100}
              render={(n: number | null) => n ?? "—"}
            />
            <Table.Column
              title={<Trans>Date</Trans>}
              dataIndex="date"
              key="date"
              render={(v: number) => formatDate(v)}
            />
            <Table.Column
              title={<Trans>Journal</Trans>}
              dataIndex="journalId"
              key="journalId"
              render={(journalId: string) => {
                const journal = find(journals, { id: journalId });
                return journal ? journal.name : "—";
              }}
            />
            <Table.Column
              title={<Trans>Description</Trans>}
              key="description"
              render={(entry: any) => (
                <Link
                  to={`/accounting/journal-entries/${entry.id}`}
                  onClick={(e) => e.stopPropagation()}
                >
                  {entry.description}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Reference</Trans>}
              dataIndex="reference"
              key="reference"
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column
              title={<Trans>Status</Trans>}
              dataIndex="status"
              key="status"
              render={(status: string) => (
                <Tag
                  color={journalEntryStatusColor[status as keyof typeof journalEntryStatusColor]}
                >
                  {journalEntryStatusLabel(status)}
                </Tag>
              )}
            />
          </Table>
        </Col>
      </Row>
    </>
  );
};

export default JournalEntries;
