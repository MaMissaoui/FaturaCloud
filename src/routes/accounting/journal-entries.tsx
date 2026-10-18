import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Row, Select, Table, Tag } from "antd";
import type { TablePaginationConfig } from "antd";
import type { SorterResult } from "antd/es/table/interface";
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
  journalEntriesTotalAtom,
  journalEntryFiltersAtom,
  journalEntryPagingAtom,
  setJournalEntriesAtom,
} from "src/atoms/journal-entry";
import type { JournalEntry } from "src/types/models";
import { journalsAtom, setJournalsAtom } from "src/atoms/journal";
import PageHeader from "src/components/page-header";

const JournalEntries = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const formatDate = useDateFormatter();

  const entries = useAtomValue(journalEntriesAtom);
  const setEntries = useSetAtom(setJournalEntriesAtom);
  const [filters, setFiltersValue] = useAtom(journalEntryFiltersAtom);
  // The list is paged and sorted on the server (audit F176): changing a
  // filter goes back to the first page.
  const [paging, setPaging] = useAtom(journalEntryPagingAtom);
  const total = useAtomValue(journalEntriesTotalAtom);
  const setFilters = (next: typeof filters) => {
    setFiltersValue(next);
    setPaging((p) => ({ ...p, page: 1 }));
  };
  const journals = useAtomValue(journalsAtom);
  const setJournals = useSetAtom(setJournalsAtom);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/accounting/journal-entries") {
      setJournals();
      setLoading(true);
      setEntries().finally(() => setLoading(false));
    }
    // filters and paging are intentionally dependencies so changing the
    // journal/status filter, the page or the sort re-fetches from the server.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location, setEntries, setJournals, filters, paging]);

  const handleTableChange = (
    pagination: TablePaginationConfig,
    _filters: unknown,
    sorter: SorterResult<JournalEntry> | SorterResult<JournalEntry>[],
  ) => {
    const s = Array.isArray(sorter) ? sorter[0] : sorter;
    const order = s?.order === "ascend" ? "asc" : s?.order === "descend" ? "desc" : undefined;
    const sort = order ? String(s.columnKey) : undefined;
    // A new sort starts again from the first page.
    const sortChanged = sort !== paging.sort || order !== paging.order;
    setPaging({
      page: sortChanged ? 1 : (pagination.current ?? 1),
      pageSize: pagination.pageSize ?? paging.pageSize,
      sort,
      order,
    });
  };
  // The sort shown on the column headers follows the server's: by date,
  // newest first, until another column is picked.
  const sortOrderFor = (key: string) => {
    if (!paging.order) return key === "date" ? ("descend" as const) : null;
    if (paging.sort !== key) return null;
    return paging.order === "asc" ? ("ascend" as const) : ("descend" as const);
  };

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
          <Table<JournalEntry>
            dataSource={entries}
            pagination={{
              current: paging.page,
              pageSize: paging.pageSize,
              total,
              showSizeChanger: true,
              hideOnSinglePage: true,
            }}
            onChange={handleTableChange}
            rowKey="id"
            loading={loading}
            onRow={(record: JournalEntry) => ({
              onClick: () => navigate(`/accounting/journal-entries/${record.id}`),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate(`/accounting/journal-entries/${record.id}`);
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
            })}
          >
            <Table.Column<JournalEntry>
              title={<Trans>Entry #</Trans>}
              dataIndex="entryNumber"
              key="entryNumber"
              width={100}
              sorter
              sortOrder={sortOrderFor("entryNumber")}
              render={(n: number | null) => n ?? "—"}
            />
            <Table.Column<JournalEntry>
              title={<Trans>Date</Trans>}
              dataIndex="date"
              key="date"
              sorter
              sortOrder={sortOrderFor("date")}
              // Starts descending (the default), so a click goes to ascending
              // and the next back to the default.
              sortDirections={["descend", "ascend"]}
              render={(v: number) => formatDate(v)}
            />
            <Table.Column<JournalEntry>
              title={<Trans>Journal</Trans>}
              dataIndex="journalId"
              key="journal"
              sorter
              sortOrder={sortOrderFor("journal")}
              render={(journalId: string) => {
                const journal = find(journals, { id: journalId });
                return journal ? journal.name : "—";
              }}
            />
            <Table.Column<JournalEntry>
              title={<Trans>Description</Trans>}
              key="description"
              sorter
              sortOrder={sortOrderFor("description")}
              render={(entry: JournalEntry) => (
                <Link
                  to={`/accounting/journal-entries/${entry.id}`}
                  onClick={(e) => e.stopPropagation()}
                >
                  {entry.description}
                </Link>
              )}
            />
            <Table.Column<JournalEntry>
              title={<Trans>Reference</Trans>}
              dataIndex="reference"
              key="reference"
              sorter
              sortOrder={sortOrderFor("reference")}
              render={(v: string | null) => v ?? "—"}
            />
            <Table.Column<JournalEntry>
              title={<Trans>Status</Trans>}
              dataIndex="status"
              key="status"
              sorter
              sortOrder={sortOrderFor("status")}
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
