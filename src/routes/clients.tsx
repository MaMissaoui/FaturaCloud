import { useEffect, useState } from "react";
import { Link, Outlet, useLocation, useNavigate } from "react-router";
import { Button, Col, Input, Space, Table, Typography, Row, Tag, Tooltip } from "antd";
import { atom, useAtom, useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { PhoneOutlined, TeamOutlined, GlobalOutlined } from "@ant-design/icons";
import isEmpty from "lodash/isEmpty";
import filter from "lodash/filter";
import get from "lodash/get";
import includes from "lodash/includes";
import some from "lodash/some";
import toString from "lodash/toString";

import { clientsAtom, setClientsAtom } from "src/atoms/client";
import ClientForm from "src/components/clients/form";

const { Title } = Typography;

const searchAtom = atom<string>("");

const Clients = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const clients = useAtomValue(clientsAtom);
  const setClients = useSetAtom(setClientsAtom);
  const [search, setSearch] = useAtom(searchAtom);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/clients") {
      setLoading(true);
      setClients().finally(() => setLoading(false));
    }
  }, [location, setClients]);

  const searchClients = () => {
    return filter(clients, (client: any) => {
      return some(
        ["name", "code", "registration_number", "address", "emails", "phone", "vatin", "website"],
        (field) => {
          const value = get(client, field);
          return includes(toString(value).toLowerCase(), search.toLowerCase());
        },
      );
    });
  };

  return (
    <>
      <Row>
        <Col span={12}>
          <Title level={3} style={{ marginTop: 0, marginBottom: 0 }}>
            <TeamOutlined style={{ marginRight: 8 }} />
            <Trans>Clients</Trans>
          </Title>
        </Col>
        <Col span={12} style={{ display: "flex", justifyContent: "flex-end" }}>
          <Space style={{ alignItems: "start" }}>
            <Input.Search
              placeholder={t`Search text`}
              onChange={(e) => setSearch(e.target.value)}
            />
            <Link to="/clients" state={{ clientModal: true }}>
              <Button type="primary" style={{ marginBottom: 10 }}>
                <Trans>New client</Trans>
              </Button>
            </Link>
          </Space>
        </Col>
      </Row>
      <Row>
        <Col span={24}>
          <Table
            dataSource={search ? searchClients() : clients}
            pagination={{ defaultPageSize: 25, showSizeChanger: true, hideOnSinglePage: true }}
            rowKey="id"
            loading={loading}
            onRow={(record: any) => ({
              onClick: () => navigate("/clients", { state: { clientModal: true, clientId: record.id } }),
              style: { cursor: "pointer" },
            })}
          >
            <Table.Column
              title={<Trans>Name</Trans>}
              key="name"
              sorter={(a: any, b: any) => (a.name < b.name ? -1 : a.name === b.name ? 0 : 1)}
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
              title={<Trans>Address</Trans>}
              dataIndex="address"
              key="address"
              sorter={(a: any, b: any) => (a.address ?? "").localeCompare(b.address ?? "")}
            />
            <Table.Column
              title={<Trans>Emails</Trans>}
              dataIndex="emails"
              key="emails"
              sorter={(a: any, b: any) => (a.emails ?? "").localeCompare(b.emails ?? "")}
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
              sorter={(a: any, b: any) => (a.phone ?? "").localeCompare(b.phone ?? "")}
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
              sorter={(a: any, b: any) => (a.vatin ?? "").localeCompare(b.vatin ?? "")}
            />
            <Table.Column
              title={<Trans>Website</Trans>}
              dataIndex="website"
              key="website"
              width={60}
              align="center"
              sorter={(a: any, b: any) => (a.website ?? "").localeCompare(b.website ?? "")}
              render={(website) =>
                website ? (
                  <Tooltip title={website}>
                    <a href={website} target="_blank" rel="noreferrer noopener" onClick={(e) => e.stopPropagation()}>
                      <GlobalOutlined style={{ fontSize: 16 }} />
                    </a>
                  </Tooltip>
                ) : null
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
