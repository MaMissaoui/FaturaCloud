import { useEffect, useMemo, useState } from "react";
import type { Client } from "src/types/models";
import { Link, Outlet, useLocation, useNavigate } from "react-router";
import { Button, Col, Empty, Space, Table, Row, Tag } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { PhoneOutlined, TeamOutlined } from "@ant-design/icons";
import isEmpty from "lodash/isEmpty";
import filter from "lodash/filter";
import get from "lodash/get";
import includes from "lodash/includes";
import some from "lodash/some";
import toString from "lodash/toString";

import { clientsAtom, setClientsAtom } from "src/atoms/client";
import { organizationIdAtom } from "src/atoms/organization";
import ClientForm from "src/components/clients/form";
import MassDataExcelActions from "src/components/mass-data/mass-data-excel-actions";
import PageHeader from "src/components/page-header";
import { formatAddressOneLine } from "src/utils/address";

const Clients = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const clients = useAtomValue(clientsAtom);
  const setClients = useSetAtom(setClientsAtom);
  const organizationId = useAtomValue(organizationIdAtom);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/clients") {
      setLoading(true);
      setClients().finally(() => setLoading(false));
    }
  }, [location, setClients]);

  const filtered = useMemo(
    () =>
      filter(clients, (client: Client) => {
        const fieldsMatch = some(
          ["name", "code", "registration_number", "emails", "phone", "vatin", "website"],
          (field) => {
            const value = get(client, field);
            return includes(toString(value).toLowerCase(), search.toLowerCase());
          },
        );
        return (
          fieldsMatch || includes(formatAddressOneLine(client).toLowerCase(), search.toLowerCase())
        );
      }),
    [clients, search],
  );

  return (
    <>
      <PageHeader
        icon={<TeamOutlined />}
        title={<Trans>Clients</Trans>}
        search={{ placeholder: t`Search`, value: search, onChange: setSearch }}
        actions={
          <Space wrap>
            {organizationId && (
              <MassDataExcelActions
                organizationId={organizationId}
                resource="clients"
                filenamePrefix="clients"
                onImported={() => setClients()}
              />
            )}
            <Link to="/clients" state={{ clientModal: true }}>
              <Button type="primary" style={{ marginBottom: 10 }}>
                <Trans>New client</Trans>
              </Button>
            </Link>
          </Space>
        }
      />
      <Row>
        <Col span={24}>
          <Table
            dataSource={filtered}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            locale={{
              emptyText: search ? (
                <Empty description={<Trans>No clients match your search</Trans>} />
              ) : (
                <Empty description={<Trans>No clients yet</Trans>}>
                  <Link to="/clients" state={{ clientModal: true }}>
                    <Button type="primary">
                      <Trans>Create your first client</Trans>
                    </Button>
                  </Link>
                </Empty>
              ),
            }}
            onRow={(record: Client) => ({
              onClick: () =>
                navigate("/clients", { state: { clientModal: true, clientId: record.id } }),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate("/clients", { state: { clientModal: true, clientId: record.id } });
                }
              },
              style: { cursor: "pointer" },
              tabIndex: 0,
              role: "button",
            })}
          >
            <Table.Column
              title={<Trans>Name</Trans>}
              key="name"
              sorter={(a: Client, b: Client) => (a.name ?? "").localeCompare(b.name ?? "")}
              render={(client) => (
                <Link
                  to={`/clients`}
                  state={{ clientModal: true, clientId: client.id }}
                  onClick={(e) => e.stopPropagation()}
                >
                  {client.name}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Code</Trans>}
              dataIndex="code"
              key="code"
              width={100}
              sorter={(a: Client, b: Client) => (a.code ?? "").localeCompare(b.code ?? "")}
            />
            <Table.Column
              title={<Trans>Address</Trans>}
              key="address"
              sorter={(a: Client, b: Client) =>
                formatAddressOneLine(a).localeCompare(formatAddressOneLine(b))
              }
              render={(client: Client) => formatAddressOneLine(client)}
            />
            <Table.Column
              title={<Trans>Emails</Trans>}
              dataIndex="emails"
              key="emails"
              sorter={(a: Client, b: Client) => (a.emails ?? "").localeCompare(b.emails ?? "")}
              render={(emails: string) => {
                if (!emails) return "";
                let parsed: string[];
                try {
                  parsed = JSON.parse(emails);
                } catch {
                  return "";
                }
                return parsed.map((email: string) => <Tag key={email}>{email}</Tag>);
              }}
            />
            <Table.Column
              title={<Trans>Phone</Trans>}
              dataIndex="phone"
              key="phone"
              sorter={(a: Client, b: Client) => (a.phone ?? "").localeCompare(b.phone ?? "")}
              render={(phone) => {
                if (!isEmpty(phone)) {
                  return (
                    <a href={`tel:${phone}`} onClick={(e) => e.stopPropagation()}>
                      <PhoneOutlined />
                      {` ${phone}`}
                    </a>
                  );
                }
              }}
            />
            <Table.Column
              title={<Trans>VATIN</Trans>}
              dataIndex="vatin"
              key="vatin"
              sorter={(a: Client, b: Client) => (a.vatin ?? "").localeCompare(b.vatin ?? "")}
            />
            <Table.Column
              title={<Trans>Identity number</Trans>}
              dataIndex="identity_number"
              key="identity_number"
              sorter={(a: Client, b: Client) =>
                (a.identity_number ?? "").localeCompare(b.identity_number ?? "")
              }
            />
          </Table>
          <Outlet />
        </Col>
      </Row>

      <ClientForm />
    </>
  );
};

export default Clients;
