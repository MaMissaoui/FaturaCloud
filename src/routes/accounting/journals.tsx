import { useEffect, useState } from "react";
import type { Journal } from "src/types/models";
import { Link, Outlet, useLocation, useNavigate } from "react-router";
import { Button, Col, Row, Table, Tag } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { BookOutlined } from "@ant-design/icons";

import { journalsAtom, setJournalsAtom } from "src/atoms/journal";
import JournalForm from "src/components/accounting/journal-form";
import PageHeader from "src/components/page-header";

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

const Journals = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const journals = useAtomValue(journalsAtom);
  const setJournals = useSetAtom(setJournalsAtom);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/accounting/journals") {
      setLoading(true);
      setJournals().finally(() => setLoading(false));
    }
  }, [location, setJournals]);

  return (
    <>
      <PageHeader
        icon={<BookOutlined />}
        title={<Trans>Journals</Trans>}
        actions={
          <Link to="/accounting/journals" state={{ journalModal: true }}>
            <Button type="primary">
              <Trans>New journal</Trans>
            </Button>
          </Link>
        }
      />
      <Row>
        <Col span={24}>
          <Table
            dataSource={journals}
            pagination={{ hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            onRow={(record: Journal) => ({
              onClick: () =>
                navigate("/accounting/journals", {
                  state: { journalModal: true, journalId: record.id },
                }),
              style: { cursor: "pointer" },
            })}
          >
            <Table.Column
              title={<Trans>Code</Trans>}
              dataIndex="code"
              key="code"
              width={100}
              sorter={(a: Journal, b: Journal) => a.code.localeCompare(b.code)}
              defaultSortOrder="ascend"
            />
            <Table.Column
              title={<Trans>Name</Trans>}
              key="name"
              sorter={(a: Journal, b: Journal) => a.name.localeCompare(b.name)}
              render={(journal: Journal) => (
                <Link
                  to="/accounting/journals"
                  state={{ journalModal: true, journalId: journal.id }}
                  onClick={(e) => e.stopPropagation()}
                >
                  {journal.name}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Type</Trans>}
              dataIndex="type"
              key="type"
              render={(type: string) => journalTypeLabel(type)}
            />
            <Table.Column
              title={<Trans>Built-in</Trans>}
              dataIndex="isSystem"
              key="isSystem"
              align="center"
              width={100}
              render={(isSystem: number) => (isSystem ? <Tag>{t`Built-in`}</Tag> : null)}
            />
          </Table>
          <Outlet />
        </Col>
      </Row>

      <JournalForm />
    </>
  );
};

export default Journals;
