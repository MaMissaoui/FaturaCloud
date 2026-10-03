import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { InputNumber } from "antd";

import NumberInput, { DecimalSeparatorContext } from "./number-input";

const type = (text: string) => {
  const input = screen.getByRole("spinbutton");
  fireEvent.change(input, { target: { value: text } });
  fireEvent.blur(input);
  return input as HTMLInputElement;
};

describe("NumberInput", () => {
  it("reads a comma and a numpad point alike when the separator is a comma", () => {
    for (const text of ["12,5", "12.5"]) {
      const onChange = vi.fn();
      const { unmount } = render(
        <DecimalSeparatorContext value=",">
          <NumberInput onChange={onChange} />
        </DecimalSeparatorContext>,
      );
      const input = type(text);
      expect(onChange).toHaveBeenLastCalledWith(12.5);
      expect(input.value).toBe("12,5");
      unmount();
    }
  });

  it("keeps antd's behaviour without a separator", () => {
    const onChange = vi.fn();
    render(<NumberInput onChange={onChange} />);
    type("12.5");
    expect(onChange).toHaveBeenLastCalledWith(12.5);
  });

  // Why the wrapper exists: antd alone drops the comma, so "12,5" is 125.
  it("documents that plain InputNumber reads 12,5 as 125", () => {
    const onChange = vi.fn();
    render(<InputNumber onChange={onChange} />);
    type("12,5");
    expect(onChange).toHaveBeenLastCalledWith(125);
  });
});
