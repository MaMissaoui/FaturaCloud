import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { App } from "antd";
import { I18nProvider } from "@lingui/react";
import { i18n } from "@lingui/core";
import DocumentExportButtons from "./document-export-buttons";

const renderButtons = (isDirty: boolean, exportDocument: (f: "pdf" | "xlsx") => Promise<unknown>) =>
  render(
    <I18nProvider i18n={i18n}>
      <App>
        <DocumentExportButtons isDirty={isDirty} exportDocument={exportDocument} />
      </App>
    </I18nProvider>,
  );

// The PDF/Excel pair every document detail page shares (audit F185).
describe("DocumentExportButtons", () => {
  it("exports the saved document in the format clicked", async () => {
    const exportDocument = vi.fn().mockResolvedValue(undefined);
    renderButtons(false, exportDocument);
    fireEvent.click(screen.getByRole("button", { name: /PDF/ }));
    fireEvent.click(screen.getByRole("button", { name: /Excel/ }));
    await waitFor(() => expect(exportDocument).toHaveBeenCalledTimes(2));
    expect(exportDocument).toHaveBeenNthCalledWith(1, "pdf");
    expect(exportDocument).toHaveBeenNthCalledWith(2, "xlsx");
  });

  it("is disabled while the form has unsaved changes", () => {
    const exportDocument = vi.fn();
    renderButtons(true, exportDocument);
    expect(screen.getByRole("button", { name: /PDF/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: /Excel/ })).toBeDisabled();
  });
});
