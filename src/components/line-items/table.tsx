import type { ReactNode } from "react";
import { Button, Form, Input, InputNumber, Select, Table } from "antd";
import type { FormInstance } from "antd/es/form";
import { DeleteOutlined, PlusOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import map from "lodash/map";
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { requiredForNewLineItem } from "src/utils/line-items";

const { Option } = Select;
const { TextArea } = Input;

interface LineItemField {
  name: number;
  key: number;
  index: number;
}

// Generic drag-and-drop row for `reorderable` tables. The visible drag handle
// itself is document-specific (invoices renders it inside its Product cell)
// and stays page-owned via a `kind: "custom"` column that calls useSortable
// with the same row key — this component only supplies the sortable <tr>.
const SortableRow = ({ children, ...props }: any) => {
  const { setNodeRef, transform, transition, isDragging } = useSortable({
    id: props["data-row-key"],
  });
  const style = {
    ...props.style,
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };
  return (
    <tr {...props} ref={setNodeRef} style={style}>
      {children}
    </tr>
  );
};

export type LineItemColumn =
  | { kind: "index" }
  | {
      kind: "product";
      products: any[];
      width?: number;
      required?: boolean;
      onSelect?: (productId: string, fieldName: number, form: FormInstance) => void;
    }
  | { kind: "description"; required?: boolean; rows?: number }
  | { kind: "quantity"; label?: ReactNode; width?: number }
  | { kind: "unit"; placeholder?: string; width?: number }
  | { kind: "unitPrice"; name?: string; label?: ReactNode; width?: number }
  | { kind: "taxRate"; taxRates: any[]; width?: number }
  | {
      kind: "custom";
      key: string;
      title?: ReactNode;
      width?: number;
      align?: "left" | "right" | "center";
      onCell?: () => Record<string, unknown>;
      render: (field: LineItemField) => ReactNode;
    };

interface LineItemsTableProps {
  name?: string;
  columns: LineItemColumn[];
  disabled?: boolean;
  addLabel?: ReactNode;
  defaultNewRow?: Record<string, unknown>;
  // Enables drag-to-reorder (invoices only, so far). The shell owns the
  // generic dnd-kit wiring (sensors, DndContext/SortableContext, the sortable
  // row) and reorders the form's array field directly; the drag handle itself
  // is rendered by the page via a `kind: "custom"` column using useSortable
  // with the same row key.
  reorderable?: boolean;
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
  reorderable = false,
}: LineItemsTableProps) => {
  const form = Form.useFormInstance();

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (active.id === over?.id) return;
    const items = form.getFieldValue(name) || [];
    const oldIndex = parseInt(active.id as string);
    const newIndex = parseInt(over?.id as string);
    if (!isNaN(oldIndex) && !isNaN(newIndex)) {
      form.setFieldValue(name, arrayMove(items, oldIndex, newIndex));
    }
  };

  return (
    <Form.List name={name}>
      {(fields, { add, remove }) => {
        const table = (
          <Table
            dataSource={fields.map((field, index) => ({ ...field, index }))}
            pagination={false}
            size="middle"
            locale={{ emptyText: t`No line items` }}
            rowKey={(r) => r.index.toString()}
            style={{ marginTop: 8 }}
            components={reorderable ? { body: { row: SortableRow } } : undefined}
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
                          <TextArea rows={col.rows ?? 1} autoSize disabled={disabled} />
                        </Form.Item>
                      )}
                    />
                  );
                case "quantity":
                  return (
                    <Table.Column<LineItemField>
                      title={col.label ?? <Trans>Qty</Trans>}
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
                case "unit":
                  return (
                    <Table.Column<LineItemField>
                      title={<Trans>Unit</Trans>}
                      key="unit"
                      width={col.width ?? 110}
                      render={(field) => (
                        <Form.Item name={[field.name, "unit"]} noStyle>
                          <Input
                            placeholder={col.placeholder ?? t`pcs, kg, m…`}
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
                        <Form.Item name={[field.name, col.name ?? "unitPrice"]} noStyle>
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
                case "taxRate":
                  return (
                    <Table.Column<LineItemField>
                      title={<Trans>Tax</Trans>}
                      key="taxRate"
                      width={col.width ?? 130}
                      render={(field) => (
                        <Form.Item name={[field.name, "taxRate"]} noStyle>
                          <Select
                            allowClear
                            placeholder={t`None`}
                            style={{ width: "100%" }}
                            disabled={disabled}
                          >
                            {map(col.taxRates, (r: any) => (
                              <Option key={r.id} value={r.id}>
                                {r.name}
                              </Option>
                            ))}
                          </Select>
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
                      onCell={col.onCell}
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
        );

        return (
          <>
            {reorderable ? (
              <DndContext
                sensors={sensors}
                collisionDetection={closestCenter}
                onDragEnd={handleDragEnd}
              >
                <SortableContext
                  items={fields.map((_, index) => index.toString())}
                  strategy={verticalListSortingStrategy}
                >
                  {table}
                </SortableContext>
              </DndContext>
            ) : (
              table
            )}

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
        );
      }}
    </Form.List>
  );
};

export default LineItemsTable;
