import { useEffect } from "react";
import type { FormInstance } from "antd";

const useSaveShortcut = (form: FormInstance, disabled?: boolean) => {
  useEffect(() => {
    if (disabled) return;
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "s") {
        e.preventDefault();
        form.submit();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [form, disabled]);
};

export default useSaveShortcut;
