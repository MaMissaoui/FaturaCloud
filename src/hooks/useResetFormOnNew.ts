import { useEffect, useRef } from "react";
import type { FormInstance } from "antd";

// useResetFormOnNew clears a detail page's form when the route moves from an
// existing document to that type's /new (#432) — e.g. browser Back after a
// create. The page component stays mounted across that move and antd applies
// initialValues only on the Form's first mount, so without this the "new"
// form would open still holding the previous document's fields.
//
// Only that transition resets: on a fresh mount the form already holds its
// initialValues, and resetting then could race a page's own prefill effects.
// Call it before those effects so a prefill on the same move still lands.
const useResetFormOnNew = (form: FormInstance, id: string | undefined) => {
  const prevId = useRef(id);
  useEffect(() => {
    if (id === "new" && prevId.current && prevId.current !== "new") form.resetFields();
    prevId.current = id;
  }, [form, id]);
};

export default useResetFormOnNew;
