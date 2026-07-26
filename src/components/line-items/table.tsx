import type { ReactNode } from "react";
import { Button, Form, Input, InputNumber, Select, Table } from "antd";
import type { FormInstance } from "antd/es/form";
import { DeleteOutlined, PlusOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import map from "lodash/map";
import { requiredForNewLineItem } from "src/utils/line-items";

const { Option } = Select;
const { TextArea } = Input;

interface LineItemField {
  name: number;
  key: number;
  index: number;
}

export type LineItemColumn =
  | { kind: "index" }
  | {
      kind: "product";
      products: any[];
      width?: number;
      required?: boolean;
      onSelect?: (productId: string, fieldName: number, form: FormInstance) => void;
    }
  | { kind: "description"; required?: boolean }
  | { kind: "quantity"; width?: number }
  | { kind: "unitPrice"; label?: ReactNode; width?: number }
  | {
      kind: "custom";
      key: string;
      title?: ReactNode;
      width?: number;
      align?: "left" | "right" | "center";
      render: (field: LineItemField) => ReactNode;
    };

interface LineItemsTableProps {
  name?: string;
  columns: LineItemColumn[];
  disabled?: boolean;
  addLabel?: ReactNode;
  defaultNewRow?: Record<string, unknown>;
}

// Shared shell for the six document line-item tables (invoices, orders,
// deliveries, purchase orders, inbound deliveries, incoming invoices), which
// had drifted into disagreeing on column order, labels, and affordances
// despite sharing ~150 lines of identical Form.List/Table scaffolding. Owns
// that scaffolding; per-document columns are configured via `columns` rather
// than forked. Per-document computed columns (received/delivered/match/etc.)
// go through `kind: "custom"` so they stay owned by the page.
const LineItemsTable = ({
  name = "lineItems",
  columns,
  disabled = false,
  addLabel,
  defaultNewRow = { quantity: 1 },
}: LineItemsTableProps) => {
  const form = Form.useFormInstance();

  return (
    <Form.List name={name}>
      {(fields, { add, remove }) => (
        <>
          <Table
            dataSource={fields.map((field, index) => ({ ...field, index }))}
            pagination={false}
            size="middle"
            locale={{ emptyText: t`No line items` }}
            rowKey={(r) => r.index.toString()}
            style={{ marginTop: 8 }}
          >
            {columns.map((col) => {
              switch (col.kind) {
                case "index":
                  return (
                    <Table.Column<LineItemField>
                      title="#"
                      key="index"
                      width={40}
                      align="right"
                      render={(field) => field.index + 1}
                    />
                  );
                case "product":
                  return (
                    <Table.Column<LineItemField>
                      title={<Trans>Product</Trans>}
                      key="productId"
                      width={col.width ?? 180}
                      render={(field) => (
                        <Form.Item
                          name={[field.name, "productId"]}
                          rules={
                            col.required
                              ? [
                                  requiredForNewLineItem(
                                    form,
                                    field.name,
                                    t`This field is required!`,
                                  ),
                                ]
                              : []
                          }
                          noStyle
                        >
                          <Select
                            showSearch
                            style={{ width: "100%" }}
                            placeholder={t`Select product`}
                            optionFilterProp="children"
                            disabled={disabled}
                            onChange={(productId) => col.onSelect?.(productId, field.name, form)}
                          >
                            {map(col.products, (p: any) => (
                              <Option key={p.id} value={p.id}>
                                {p.name}
                                {p.sku ? ` (${p.sku})` : ""}
                              </Option>
                            ))}
                          </Select>
                        </Form.Item>
                      )}
                    />
                  );
                case "description":
                  return (
                    <Table.Column<LineItemField>
                      title={<Trans>Description</Trans>}
                      key="description"
                      render={(field) => (
                        <Form.Item
                          name={[field.name, "description"]}
                          noStyle
                          rules={
                            col.required
                              ? [{ required: true, message: t`Description required` }]
                              : []
                          }
                        >
                          <TextArea rows={1} autoSize disabled={disabled} />
                        </Form.Item>
                      )}
                    />
                  );
                case "quantity":
                  return (
                    <Table.Column<LineItemField>
                      title={<Trans>Qty</Trans>}
                      key="quantity"
                      width={col.width ?? 90}
                      align="right"
                      render={(field) => (
                        <Form.Item
                          name={[field.name, "quantity"]}
                          noStyle
                          rules={[{ required: true, message: t`Required` }]}
                        >
                          <InputNumber
                            style={{ width: "100%", textAlign: "right" }}
                            min={0}
                            precision={2}
                            disabled={disabled}
                          />
                        </Form.Item>
                      )}
                    />
                  );
                case "unitPrice":
                  return (
                    <Table.Column<LineItemField>
                      title={col.label ?? <Trans>Unit price</Trans>}
                      key="unitPrice"
                      width={col.width ?? 110}
                      align="right"
                      render={(field) => (
                        <Form.Item name={[field.name, "unitPrice"]} noStyle>
                          <InputNumber
                            style={{ width: "100%", textAlign: "right" }}
                            min={0}
                            precision={2}
                            step={0.01}
                            disabled={disabled}
                          />
                        </Form.Item>
                      )}
                    />
                  );
                case "custom":
                  return (
                    <Table.Column<LineItemField>
                      title={col.title}
                      key={col.key}
                      width={col.width}
                      align={col.align}
                      render={col.render}
                    />
                  );
                default:
                  return null;
              }
            })}
            {!disabled && (
              <Table.Column<LineItemField>
                key="remove"
                width={40}
                render={(field) => (
                  <Button
                    type="text"
                    danger
                    size="small"
                    icon={<DeleteOutlined />}
                    onClick={() => remove(field.name)}
                    aria-label={t`Remove line item`}
                  />
                )}
              />
            )}
          </Table>

          {!disabled && (
            <Button
              type="default"
              size="small"
              icon={<PlusOutlined />}
              onClick={() => add(defaultNewRow)}
              style={{ marginTop: 12 }}
            >
              {addLabel ?? <Trans>Add line item</Trans>}
            </Button>
          )}
        </>
      )}
    </Form.List>
  );
};

export default LineItemsTable;
