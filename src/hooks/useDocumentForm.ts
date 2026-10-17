import { useState } from "react";
import type { FormInstance } from "antd";
import useSaveShortcut from "src/hooks/useSaveShortcut";
import useUnsavedChangesWarning from "src/hooks/useUnsavedChangesWarning";

// useDocumentForm is the edit state every document detail page shares
// (audit F185 — the six pages repeated it, and a fix to one had to be
// repeated five times): whether the form has unsaved changes, Ctrl/Cmd+S to
// save, and a warning before leaving with unsaved changes. The page wires
// markDirty to its Form's onValuesChange and calls markSaved after a save.
export function useDocumentForm(form: FormInstance) {
  const [isDirty, setIsDirty] = useState(false);
  useSaveShortcut(form);
  useUnsavedChangesWarning(isDirty);
  return {
    isDirty,
    markDirty: () => setIsDirty(true),
    markSaved: () => setIsDirty(false),
  };
}
