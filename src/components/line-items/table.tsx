import type { ReactNode } from "react";
import { Button, Form, Input, InputNumber, Select, Table, Typography } from "antd";
import type { FormInstance } from "antd/es/form";
import { DeleteOutlined, HolderOutlined, PlusOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import find from "lodash/find";
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
import styles from "./table.module.scss";

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

// Row number at rest; on row hover it swaps for the drag handle, replacing
// the old near-invisible glyph that used to sit outside the table border.
const IndexCell = ({ index }: { index: number }) => {
  const { attributes, listeners } = useSortable({ id: index.toString() });
  return (
    <span className={styles.indexCell}>
      <span className={styles.indexNumber}>{index + 1}</span>
      <span className={styles.indexHandle} {...attributes} {...listeners}>
        <HolderOutlined />
      </span>
    </span>
  );
};

// The product picker cell, extracted so it can use Form.useWatch on the
// row's current productId. `offered` is usually a filtered subset of all
// products (purchase orders/inbound deliveries exclude finished goods,
// orders/deliveries/invoices/cash-book exclude components), and a saved line
// can reference a product that filter now excludes — e.g. a product
// reclassified after the document was created. Without re-adding the current
// value as an option, antd's Select has no label to show and falls back to
// rendering the raw product id. `all` is the unfiltered list used only to
// resolve that one selected value; the dropdown still offers just `offered`.
export const ProductSelectCell = ({
  fieldName,
  form,
  offered,
  all,
  disabled,
  rules,
  onSelect,
  optionLabel,
  dropdownLabel,
}: {
  fieldName: number;
  form: FormInstance;
  offered: any[];
  all?: any[];
  disabled?: boolean;
  rules?: any[];
  onSelect?: (productId: string, fieldName: number, form: FormInstance) => void;
  // Label shown once a product is selected (the closed Select / the Product
  // cell). Defaults to the SKU, falling back to the name.
  optionLabel?: (product: any) => string;
  // Label shown for each option in the open dropdown. Defaults to the same as
  // optionLabel — pages that want the name visible while searching pass one.
  dropdownLabel?: (product: any) => string;
}) => {
  const currentId = Form.useWatch(["lineItems", fieldName, "productId"], form);
  const pool = all ?? offered;
  const current = currentId ? find(pool, { id: currentId }) : undefined;
  const options = current && !find(offered, { id: current.id }) ? [...offered, current] : offered;
  const selectedText = current
    ? optionLabel
      ? optionLabel(current)
      : current.sku || current.name
    : "";

  return (
    <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
      <Form.Item name={[fieldName, "productId"]} rules={rules} noStyle>
        <Select
          showSearch
          style={{ width: "100%" }}
          placeholder={t`Select product`}
          // Show the SKU (the option's `label`) in the closed cell, while the
          // open dropdown renders the children (product name · SKU). Without
          // this, antd defaults the closed display to the option's children,
          // so the cell duplicated the name that the description already
          // carries.
          optionLabelProp="label"
          // Search matches on name (what someone remembers) as well as SKU,
          // even though the option label shows the SKU.
          filterOption={(input, option) => {
            const p = find(options, { id: option?.value });
            const needle = input.toLowerCase();
            return (
              !!p &&
              (String(p.name).toLowerCase().includes(needle) ||
                String(p.sku ?? "")
                  .toLowerCase()
                  .includes(needle))
            );
          }}
          disabled={disabled}
          onChange={(productId) => onSelect?.(productId, fieldName, form)}
        >
          {map(options, (p: any) => (
            <Option key={p.id} value={p.id} label={optionLabel ? optionLabel(p) : p.sku || p.name}>
              {/* The dropdown shows the product NAME (with its SKU appended),
                  so a product can be found and identified by name; the closed
                  cell shows the SKU via `label`. */}
              {dropdownLabel ? dropdownLabel(p) : p.sku ? `${p.name} · ${p.sku}` : p.name}
            </Option>
          ))}
        </Select>
      </Form.Item>
      {selectedText && (
        // Copy the product column's value to the clipboard — the SKU code, so
        // it can be pasted into another document/screen without retyping.
        <Typography.Text
          copyable={{ text: selectedText, tooltips: [t`Copy`, t`Copied`] }}
          style={{ flexShrink: 0 }}
          aria-label={t`Copy product`}
        />
      )}
    </div>
  );
};
export type LineItemColumn =
  | { kind: "index" }
  | {
      kind: "product";
      products: any[];
      // Unfiltered product list, used only to resolve the label of a selected
      // product the `products` filter excludes (see ProductSelectCell).
      allProducts?: any[];
      // Custom option/selected label; defaults to SKU (falling back to name).
      optionLabel?: (product: any) => string;
      // Custom dropdown-option label; defaults to the same as optionLabel.
      dropdownLabel?: (product: any) => string;
      width?: number;
      required?: boolean;
      onSelect?: (productId: string, fieldName: number, form: FormInstance) => void;
    }
  | { kind: "description"; required?: boolean; rows?: number; width?: number }
  | {
      kind: "quantity";
      label?: ReactNode;
      width?: number;
      // Defaults to 2 (fractional quantities). A serialized product's line
      // must be a whole number — pages that carry a per-line `serialized`
      // flag (autofilled by their product-select onSelect) pass a function
      // to force precision 0 on those rows specifically; this is UX only,
      // the DB layer is the actual guard.
      precision?: number | ((fieldName: number, form: FormInstance) => number);
    }
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
        // Enter in the last row's Quantity/Unit price field appends a new
        // row — those two are the only inputs common to every document
        // type's column config, and neither is a Select/TextArea, so this
        // can't collide with Enter-selects-option or Enter-inserts-newline
        // behavior elsewhere in the row.
        const addRowOnEnterFromLastRow = (rowIndex: number) => {
          if (!disabled && rowIndex === fields.length - 1) add(defaultNewRow);
        };

        const table = (
          <Table
            className={styles.wrapper}
            dataSource={fields.map((field, index) => ({ ...field, index }))}
            pagination={false}
            size="middle"
            locale={{ emptyText: t`No line items` }}
            rowKey={(r) => r.index.toString()}
            style={{ marginTop: 8 }}
            components={reorderable ? { body: { row: SortableRow } } : undefined}
            // Column widths sum well past a phone-width viewport — without
            // this, antd lets the table force the whole page wider instead
            // of scrolling within itself, taking the surrounding form's
            // totals/submit button out of reach without horizontal scrolling
            // the entire document.
            scroll={{ x: "max-content" }}
            summary={() => {
              if (fields.length === 0) return null;
              const items = form.getFieldValue(name) || [];
              const totalQty = items.reduce(
                (sum: number, item: any) => sum + (Number(item?.quantity) || 0),
                0,
              );
              return (
                <Table.Summary.Row>
                  <Table.Summary.Cell index={0} colSpan={2}>
                    <Typography.Text type="secondary">
                      <Trans>{fields.length} item(s)</Trans>
                    </Typography.Text>
                  </Table.Summary.Cell>
                  <Table.Summary.Cell index={2} align="right">
                    <Typography.Text type="secondary">
                      <Trans>Total qty: {totalQty}</Trans>
                    </Typography.Text>
                  </Table.Summary.Cell>
                </Table.Summary.Row>
              );
            }}
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
                      render={(field) =>
                        reorderable ? <IndexCell index={field.index} /> : field.index + 1
                      }
                    />
                  );
                case "product":
                  return (
                    <Table.Column<LineItemField>
                      title={<Trans>Product</Trans>}
                      key="productId"
                      width={col.width ?? 180}
                      render={(field) => (
                        <ProductSelectCell
                          fieldName={field.name}
                          form={form}
                          offered={col.products}
                          all={col.allProducts}
                          disabled={disabled}
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
                          onSelect={col.onSelect}
                          optionLabel={col.optionLabel}
                          dropdownLabel={col.dropdownLabel}
                        />
                      )}
                    />
                  );
                case "description":
                  return (
                    <Table.Column<LineItemField>
                      title={<Trans>Description</Trans>}
                      key="description"
                      // Every other column here sets an explicit width and
                      // gets it; this one didn't, and in practice that left
                      // it squeezed to a sliver next to the Product column
                      // (table-layout:auto plus an autoSize TextArea don't
                      // reliably claim remaining space) — at this catalog's
                      // longer names, that wrapped the same text the Product
                      // select already shows into a 4-5 line stack. Default
                      // width is deliberately wider than Product's (180) —
                      // it's usually the longest free-text cell in the row.
                      width={col.width ?? 260}
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
                        <Form.Item shouldUpdate noStyle>
                          {() => {
                            const precision =
                              typeof col.precision === "function"
                                ? col.precision(field.name, form)
                                : (col.precision ?? 2);
                            return (
                              <Form.Item
                                name={[field.name, "quantity"]}
                                noStyle
                                rules={[{ required: true, message: t`Required` }]}
                              >
                                <InputNumber
                                  style={{ width: "100%" }}
                                  styles={{ input: { textAlign: "right" } }}
                                  min={0}
                                  precision={precision}
                                  disabled={disabled}
                                  onPressEnter={() => addRowOnEnterFromLastRow(field.index)}
                                />
                              </Form.Item>
                            );
                          }}
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
                            style={{ width: "100%" }}
                            styles={{ input: { textAlign: "right" } }}
                            min={0}
                            precision={2}
                            step={0.01}
                            disabled={disabled}
                            onPressEnter={() => addRowOnEnterFromLastRow(field.index)}
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
                align="center"
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
