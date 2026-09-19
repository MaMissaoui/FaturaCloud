import { useEffect } from "react";

const useUnsavedChangesWarning = (isDirty: boolean) => {
  useEffect(() => {
    if (!isDirty) return;
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [isDirty]);
};

export default useUnsavedChangesWarning;
