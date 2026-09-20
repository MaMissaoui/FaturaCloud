import { Button, Col, Form, InputNumber, Row, Select } from "antd";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { DeleteOutlined, PlusOutlined } from "@ant-design/icons";

// The repeatable component/quantity rows of a bill of materials, shared
// between the product edit drawer (form.tsx, edited alongside the rest of
// the product and saved together) and the dedicated BOM maintenance screen
// (bom-editor-drawer.tsx, its own Form saved on its own) — same recipe,
// two entry points, one implementation rather than two Form.Lists that
// could drift. Reads/writes whatever Form it's rendered inside via antd's
// Form.List context, so it takes no `form` prop of its own.
const BOMFields = ({
  fieldName = "bom",
  componentOptions,
}: {
  fieldName?: string;
  componentOptions: { value: string; label: string }[];
}) => (
  <Form.List name={fieldName}>
    {(fields, { add, remove }) => (
      <>
        {fields.map(({ key, name, ...restField }) => (
          // wrap={false} + the Select column's minWidth: 0 are both load-bearing:
          // a long component label (name + SKU, worse once translated — German
          // routinely runs longer than English) sizes the Select's intrinsic
          // content wider than the row, and a flex column's default min-width is
          // its content size, not 0 — without minWidth: 0 the row silently wraps,
          // orphaning the delete button onto its own line below the qty input
          // instead of letting the Select itself truncate with an ellipsis.
          <Row key={key} gutter={[8, 0]} align="middle" wrap={false} style={{ marginBottom: 8 }}>
            <Col flex="auto" style={{ minWidth: 0 }}>
              <Form.Item
                {...restField}
                name={[name, "componentProductId"]}
                rules={[{ required: true, message: t`Component is required` }]}
                style={{ marginBottom: 0 }}
              >
                <Select
                  showSearch
                  style={{ width: "100%" }}
                  placeholder={t`Select component`}
                  optionFilterProp="label"
                  options={componentOptions}
                />
              </Form.Item>
            </Col>
            <Col flex="140px">
              <Form.Item
                {...restField}
                name={[name, "quantityPerUnit"]}
                rules={[{ required: true, message: t`Quantity is required` }]}
                style={{ marginBottom: 0 }}
              >
                <InputNumber min={0.001} style={{ width: "100%" }} placeholder={t`Qty per unit`} />
              </Form.Item>
            </Col>
            <Col flex="32px">
              <Button
                type="text"
                danger
                icon={<DeleteOutlined />}
                onClick={() => remove(name)}
                aria-label={t`Remove component`}
                title={t`Remove component`}
              />
            </Col>
          </Row>
        ))}
        <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add()}>
          <Trans>Add component</Trans>
        </Button>
      </>
    )}
  </Form.List>
);

export default BOMFields;
