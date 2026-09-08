import { useEffect, useState } from "react";
import type { PaymentTerm } from "src/types/models";
import { Link, useLocation, useNavigate } from "react-router";
import { Button, Col, Row, Table } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { CheckSquareOutlined, ScheduleOutlined } from "@ant-design/icons";
import filter from "lodash/filter";
import includes from "lodash/includes";

import { paymentTermsAtom, setPaymentTermsAtom } from "src/atoms/payment-term";
import PaymentTermForm from "src/components/payment-terms/form";
import PageHeader from "src/components/page-header";

const searchAtom = atom<string>("");

function SettingsPaymentTerms() {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();

  const paymentTerms = useAtomValue(paymentTermsAtom);
  const setPaymentTerms = useSetAtom(setPaymentTermsAtom);
  const [search, setSearch] = useAtom(searchAtom);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/settings/payment-terms") {
      setLoading(true);
      setPaymentTerms().finally(() => setLoading(false));
    }
  }, [location, setPaymentTerms]);

  const filtered = search
    ? filter(paymentTerms, (pt: PaymentTerm) =>
        includes(pt.name.toLowerCase(), search.toLowerCase()),
      )
    : paymentTerms;

  return (
    <>
      <PageHeader
        icon={<ScheduleOutlined />}
        title={<Trans>Payment terms</Trans>}
        search={{ placeholder: t`Search`, onChange: setSearch }}
        actions={
          <Link to="/settings/payment-terms" state={{ paymentTermModal: true }}>
            <Button type="primary">
              <Trans>New payment term</Trans>
            </Button>
          </Link>
        }
      />

      <Row style={{ marginTop: 16 }}>
        <Col span={24}>
          <Table
            dataSource={filtered}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            onRow={(record: PaymentTerm) => ({
              onClick: () =>
                navigate("/settings/payment-terms", {
                  state: { paymentTermModal: true, paymentTermId: record.id },
                }),
              style: { cursor: "pointer" },
            })}
          >
            <Table.Column
              title={<Trans>Name</Trans>}
              key="name"
              sorter={(a: PaymentTerm, b: PaymentTerm) => a.name.localeCompare(b.name)}
              render={(pt: PaymentTerm) => (
                <Link
                  to="/settings/payment-terms"
                  state={{ paymentTermModal: true, paymentTermId: pt.id }}
                  onClick={(e) => e.stopPropagation()}
                >
                  {pt.name}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Default</Trans>}
              align="center"
              dataIndex="isDefault"
              key="isDefault"
              sorter={(a: PaymentTerm, b: PaymentTerm) =>
                (a.isDefault ? 1 : 0) - (b.isDefault ? 1 : 0)
              }
              render={(value) => (value ? <CheckSquareOutlined /> : "—")}
            />
          </Table>
        </Col>
      </Row>

      <PaymentTermForm />
    </>
  );
}

export default SettingsPaymentTerms;
