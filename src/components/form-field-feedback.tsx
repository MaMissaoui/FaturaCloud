import { cloneElement, forwardRef, type ReactElement } from "react";
import { Form } from "antd";

// A `noStyle` Form.Item renders no label, help or error text — that is what
// makes it usable inside a table cell. The catch is that a rule on one (a
// required product or quantity on a new line item) still runs and still blocks
// form.submit(), so a save with a missing value used to do nothing at all: no
// inline error, no toast, no request. Wrap the control in this to get antd's
// red error state on the control plus its message underneath, while keeping the
// cell itself label-less.
//
// It sits *inside* the Form.Item (which clones its single child with
// value/onChange/ref), so it forwards those props through to the real control;
// Form.Item.useStatus reads the enclosing item's validation state.
//
// The control's *own* onChange must survive: pages pass one to auto-fill a
// price/description from the picked product or to back-compute a total, and
// Form.Item's cloned onChange would otherwise replace it wholesale. Compose
// them, invoking the page's handler first (it reads the current row) and then
// Form.Item's (which persists the value).
type FieldFeedbackProps = { children: ReactElement } & Record<string, unknown>;

export const FieldFeedback = forwardRef<unknown, FieldFeedbackProps>((props, ref) => {
  const { children, ...fieldProps } = props;
  const { status, errors } = Form.Item.useStatus();
  const invalid = status === "error";

  const childProps = (children as unknown as { props?: Record<string, unknown> }).props ?? {};
  const childOnChange = childProps.onChange as ((...args: unknown[]) => void) | undefined;
  const fieldOnChange = fieldProps.onChange as ((...args: unknown[]) => void) | undefined;

  return (
    // flex:1/minWidth:0 keeps the Product cell's own flex row (control +
    // copy-to-clipboard icon) laying out exactly as before; outside a flex
    // container the two are inert.
    <div style={{ flex: 1, minWidth: 0 }}>
      {cloneElement(children as ReactElement<Record<string, unknown>>, {
        ...fieldProps,
        onChange: (...args: unknown[]) => {
          childOnChange?.(...args);
          fieldOnChange?.(...args);
        },
        ref,
        ...(invalid ? { status: "error" } : {}),
      })}
      {invalid && errors && errors.length > 0 ? (
        <div className="ant-form-item-explain-error" style={{ fontSize: 12, lineHeight: 1.4 }}>
          {errors[0]}
        </div>
      ) : null}
    </div>
  );
});
