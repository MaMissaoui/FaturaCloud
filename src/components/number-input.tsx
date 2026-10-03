import { createContext, useContext } from "react";
import { InputNumber, type InputNumberProps } from "antd";

// The decimal separator every NumberInput uses, provided once by the app
// root (src/app.tsx) from the active organization: see inputDecimalSeparator
// in src/utils/currencies.tsx. A plain context rather than an atom read here,
// because these inputs live inside Modals and Drawers, where subscribing to
// the async organization atom freezes the mask.
export const DecimalSeparatorContext = createContext<string | undefined>(undefined);

// antd's InputNumber with the organization's decimal separator, so a comma
// is a decimal point rather than being dropped: without it antd strips the
// "," and "12,5" became 125. With "," set, "12.5" (a numpad's ".") still
// reads as 12.5. A decimalSeparator passed by the caller still wins. Use it
// for every number field instead of InputNumber.
const NumberInput = <T extends number | string = number>(props: InputNumberProps<T>) => {
  const decimalSeparator = useContext(DecimalSeparatorContext);
  return <InputNumber<T> decimalSeparator={decimalSeparator} {...props} />;
};

export default NumberInput;
