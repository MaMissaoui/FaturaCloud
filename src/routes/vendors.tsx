import { useEffect, useMemo, useState } from "react";
import type { Vendor } from "src/types/models";
import { Link, Outlet, useLocation, useNavigate } from "react-router";
import { Button, Col, Empty, Space, Table, Row, Tag } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { PhoneOutlined, SolutionOutlined } from "@ant-design/icons";
import isEmpty from "lodash/isEmpty";
import filter from "lodash/filter";
import get from "lodash/get";
import includes from "lodash/includes";
import some from "lodash/some";
import toString from "lodash/toString";

import { vendorsAtom, setVendorsAtom } from "src/atoms/vendor";
import { organizationIdAtom } from "src/atoms/organization";
import VendorForm from "src/components/vendors/form";
import MassDataExcelActions from "src/components/mass-data/mass-data-excel-actions";
import PageHeader from "src/components/page-header";
import { formatAddressOneLine } from "src/utils/address";

const Vendors = () => {
  useLingui();
  const location = useLocation();
  const navigate = useNavigate();
  const vendors = useAtomValue(vendorsAtom);
  const setVendors = useSetAtom(setVendorsAtom);
  const organizationId = useAtomValue(organizationIdAtom);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (location.pathname === "/vendors") {
      setLoading(true);
      setVendors().finally(() => setLoading(false));
    }
  }, [location, setVendors]);

  const filtered = useMemo(
    () =>
      filter(vendors, (vendor: Vendor) => {
        const fieldsMatch = some(
          ["name", "code", "registration_number", "emails", "phone", "vatin", "website"],
          (field) => {
            const value = get(vendor, field);
            return includes(toString(value).toLowerCase(), search.toLowerCase());
          },
        );
        return (
          fieldsMatch || includes(formatAddressOneLine(vendor).toLowerCase(), search.toLowerCase())
        );
      }),
    [vendors, search],
  );

  return (
    <>
      <PageHeader
        icon={<SolutionOutlined />}
        title={<Trans>Vendors</Trans>}
        search={{ placeholder: t`Search`, value: search, onChange: setSearch }}
        actions={
          <Space wrap>
            {organizationId && (
              <MassDataExcelActions
                organizationId={organizationId}
                resource="vendors"
                filenamePrefix="vendors"
                onImported={() => setVendors()}
              />
            )}
            <Link to="/vendors" state={{ vendorModal: true }}>
              <Button type="primary" style={{ marginBottom: 10 }}>
                <Trans>New vendor</Trans>
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
                <Empty description={<Trans>No vendors match your search</Trans>} />
              ) : (
                <Empty description={<Trans>No vendors yet</Trans>}>
                  <Link to="/vendors" state={{ vendorModal: true }}>
                    <Button type="primary">
                      <Trans>Create your first vendor</Trans>
                    </Button>
                  </Link>
                </Empty>
              ),
            }}
            onRow={(record: Vendor) => ({
              onClick: () =>
                navigate("/vendors", { state: { vendorModal: true, vendorId: record.id } }),
              onKeyDown: (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  navigate("/vendors", { state: { vendorModal: true, vendorId: record.id } });
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
              sorter={(a: Vendor, b: Vendor) => (a.name ?? "").localeCompare(b.name ?? "")}
              render={(vendor) => (
                <Link
                  to={`/vendors`}
                  state={{ vendorModal: true, vendorId: vendor.id }}
                  onClick={(e) => e.stopPropagation()}
                >
                  {vendor.name}
                </Link>
              )}
            />
            <Table.Column
              title={<Trans>Code</Trans>}
              dataIndex="code"
              key="code"
              width={100}
              sorter={(a: Vendor, b: Vendor) => (a.code ?? "").localeCompare(b.code ?? "")}
            />
            <Table.Column
              title={<Trans>Address</Trans>}
              key="address"
              sorter={(a: Vendor, b: Vendor) =>
                formatAddressOneLine(a).localeCompare(formatAddressOneLine(b))
              }
              render={(vendor: Vendor) => formatAddressOneLine(vendor)}
            />
            <Table.Column
              title={<Trans>Emails</Trans>}
              dataIndex="emails"
              key="emails"
              sorter={(a: Vendor, b: Vendor) => (a.emails ?? "").localeCompare(b.emails ?? "")}
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
              sorter={(a: Vendor, b: Vendor) => (a.phone ?? "").localeCompare(b.phone ?? "")}
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
              sorter={(a: Vendor, b: Vendor) => (a.vatin ?? "").localeCompare(b.vatin ?? "")}
            />
          </Table>
          <Outlet />
        </Col>
      </Row>

      <VendorForm />
    </>
  );
};

export default Vendors;
