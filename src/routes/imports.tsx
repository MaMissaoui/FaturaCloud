import { useEffect, useState } from "react";
import type { Import } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Table, Row } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ContainerOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import get from "lodash/get";
import includes from "lodash/includes";
import some from "lodash/some";
import toString from "lodash/toString";

import { importsAtom, setImportsAtom } from "src/atoms/import";
import { setPurchaseOrdersAtom } from "src/atoms/purchase-order";
import { organizationAtom } from "src/atoms/organization";
import ImportForm from "src/components/imports/form";
import PageHeader from "src/components/page-header";
import { useDateFormatter } from "src/utils/date";
import { formatCents } from "src/utils/currency";

const searchAtom = atom<string>("");

const Imports = () => {
  const { i18n } = useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const imports = useAtomValue(importsAtom);
  const setImports = useSetAtom(setImportsAtom);
  const setPurchaseOrders = useSetAtom(setPurchaseOrdersAtom);
  const organization = useAtomValue(organizationAtom);
  const [search, setSearch] = useAtom(searchAtom);
  const [loading, setLoading] = useState(false);
  const formatDate = useDateFormatter();

  useEffect(() => {
    if (location.pathname === "/imports") {
      setLoading(true);
      setImports().finally(() => setLoading(false));
      // Needed for the drawer's linked-purchase-orders card, not this page's
      // own table — fetched here (once, on list mount) rather than inside
      // the drawer so it's already warm by the time a row is clicked.
      setPurchaseOrders();
    }
  }, [location, setImports, setPurchaseOrders]);

  const searchImports = () => {
    return filter(imports, (imp: Import) => {
      return some(["importNumber", "notes"], (field) => {
        const value = get(imp, field);
        return includes(toString(value).toLowerCase(), search.toLowerCase());
      });
    });
  };

  const money = (cents: number) => formatCents(cents, organization?.currency ?? "EUR", i18n.locale);

  return (
    <>
      <PageHeader
        icon={<ContainerOutlined />}
        title={<Trans>Imports</Trans>}
        search={{ placeholder: t`Search text`, onChange: setSearch }}
        actions={
          <Link to="/imports" state={{ importModal: true }}>
            <Button type="primary" style={{ marginBottom: 10 }}>
              <Trans>New import</Trans>
            </Button>
          </Link>
        }
      />
      <Row>
        <Col span={24}>
          <Table
            dataSource={search ? searchImports() : imports}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            onRow={(record: Import) => ({
              onClick: () =>
                navigate("/imports", { state: { importModal: true, importId: record.id } }),
              style: { cursor: "pointer" },
            })}
          >
            <Table.Column
              title={<Trans>Number</Trans>}
              key="importNumber"
              sorter={(a: Import, b: Import) => a.importNumber.localeCompare(b.importNumber)}
              render={(imp: Import) => (
                <Link
                  to="/imports"
                  state={{ importModal: true, importId: imp.id }}
                  onClick={(e) => e.stopPropagation()}
                >
                  {imp.importNumber}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Date</Trans>}
              dataIndex="date"
              key="date"
              sorter={(a: Import, b: Import) => a.date - b.date}
              render={(date: number) => formatDate(date)}
            />
            <Table.Column
              title={<Trans>Freight</Trans>}
              dataIndex="freightCost"
              key="freightCost"
              align="right"
              sorter={(a: Import, b: Import) => a.freightCost - b.freightCost}
              render={(cents: number) => money(cents)}
            />
            <Table.Column
              title={<Trans>Customs</Trans>}
              dataIndex="customsCost"
              key="customsCost"
              align="right"
              sorter={(a: Import, b: Import) => a.customsCost - b.customsCost}
              render={(cents: number) => money(cents)}
            />
            <Table.Column title={<Trans>Notes</Trans>} dataIndex="notes" key="notes" ellipsis />
          </Table>
        </Col>
      </Row>

      <ImportForm />
    </>
  );
};

export default Imports;
