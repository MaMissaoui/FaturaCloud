import type { FormInstance, Rule } from "antd/es/form";

// A line item added via "Add line item" has no stored id yet — add() never
// sets one, only a row already persisted server-side does. Used to make a
// field required going forward without retroactively blocking saves on
// existing rows that predate the requirement (those keep whatever they
// already have, product or not).
export function requiredForNewLineItem(
  form: FormInstance,
  fieldName: number,
  message: string,
): Rule {
  return {
    validator: (_, value) => {
      const rowId = form.getFieldValue(["lineItems", fieldName, "id"]);
      if (!rowId && !value) {
        return Promise.reject(new Error(message));
      }
      return Promise.resolve();
    },
  };
}
