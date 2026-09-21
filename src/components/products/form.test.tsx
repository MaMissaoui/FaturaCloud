import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router";
import { App, ConfigProvider } from "antd";
import { I18nProvider } from "@lingui/react";
import { i18n } from "@lingui/core";
import { Provider as JotaiProvider, createStore } from "jotai";
import { productsAtom } from "src/atoms/product";
import type { Product } from "src/types/models";
import ProductForm from "./form";

// form.tsx is the router-state-driven product drawer, unlike the first
// component smoke test (payment-panel, issue #175) it is Jotai- and
// router-heavy — so this test wraps it in its own MemoryRouter (inside the
// harness's Route element) to seed `location.state`, which is what actually
// opens the drawer and selects the product. The API layer is mocked, the same
// named-function stubbing payment-panel.test.tsx already uses.
vi.mock("src/api", () => ({
  GetProducts: vi.fn(),
  GetProduct: vi.fn(),
  CreateProduct: vi.fn(),
  UpdateProduct: vi.fn(),
  DeleteProduct: vi.fn(),
  GetTaxRates: vi.fn(),
  GetTaxRate: vi.fn(),
  CreateTaxRate: vi.fn(),
  UpdateTaxRate: vi.fn(),
  DeleteTaxRate: vi.fn(),
  GetAccounts: vi.fn(),
  CreateAccount: vi.fn(),
  UpdateAccount: vi.fn(),
  DeleteAccount: vi.fn(),
  GetUnitsOfMeasure: vi.fn(),
  CreateUnitOfMeasure: vi.fn(),
  UpdateUnitOfMeasure: vi.fn(),
  DeleteUnitOfMeasure: vi.fn(),
  GetProductBOM: vi.fn(),
  ReplaceProductBOM: vi.fn(),
  GetOrganizations: vi.fn(),
  GetOrganization: vi.fn(),
  CreateOrganization: vi.fn(),
  UpdateOrganization: vi.fn(),
  GetOrganizationLogoDataUri: vi.fn(),
  GetMyOrganizationRole: vi.fn(),
}));

import {
  GetAccounts,
  GetProductBOM,
  GetProducts,
  GetTaxRates,
  GetUnitsOfMeasure,
  ReplaceProductBOM,
  UpdateProduct,
} from "src/api";

const finishedProduct: Product = {
  id: "p1",
  organizationId: "org_1",
  name: "Widget",
  description: null,
  sku: "WIDGET",
  price: 1000,
  unitCost: null,
  unit: null,
  unitOfMeasureId: null,
  type: "product",
  category: "finished",
  taxRateId: null,
  stockEnabled: 0,
  stockQuantity: 0,
  serialized: 0,
  createdAt: null,
};

// Same provider stack as src/test-support/render-with-providers, minus its
// outer MemoryRouter — this component needs `location.state` to be open, and
// React Router forbids nesting one Router inside another, so the single
// MemoryRouter is constructed here with the drawer-opening state.
async function renderProductForm() {
  const store = createStore();
  store.set(productsAtom, [finishedProduct]);

  const Wrapper = ({ children }: { children: ReactNode }) => (
    <JotaiProvider store={store}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <MemoryRouter
              initialEntries={[
                { pathname: "/products", state: { productModal: true, productId: "p1" } },
              ]}
            >
              {children}
            </MemoryRouter>
          </App>
        </ConfigProvider>
      </I18nProvider>
    </JotaiProvider>
  );

  let result!: ReturnType<typeof render>;
  await act(async () => {
    result = render(<ProductForm />, { wrapper: Wrapper });
    // One macrotask tick so the mocked fetches' effects settle (the same
    // flush render-with-providers performs for its async atoms).
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  return { store, ...result };
}

describe("ProductForm bill of materials safety (F111 / F117)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(GetProducts).mockResolvedValue({ data: [finishedProduct] } as never);
    vi.mocked(GetTaxRates).mockResolvedValue([]);
    vi.mocked(GetAccounts).mockResolvedValue([]);
    vi.mocked(GetUnitsOfMeasure).mockResolvedValue([]);
    vi.mocked(UpdateProduct).mockResolvedValue(finishedProduct);
    vi.mocked(ReplaceProductBOM).mockResolvedValue(undefined as never);
  });

  it("skips the BOM write when the recipe failed to load, while still saving the product", async () => {
    vi.mocked(GetProductBOM).mockRejectedValue(new Error("network down"));

    await renderProductForm();

    // The failure is recorded and surfaced, not swallowed.
    expect(await screen.findByText("Couldn't load this bill of materials")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(UpdateProduct).toHaveBeenCalledTimes(1));
    // The recipe is unknown, so it must be left alone — not replaced with [].
    expect(ReplaceProductBOM).not.toHaveBeenCalled();
    expect(await screen.findByText(/left unchanged/)).toBeInTheDocument();
  });

  it("still writes a legitimately cleared (empty) recipe after a successful load", async () => {
    vi.mocked(GetProductBOM).mockResolvedValue([]);

    await renderProductForm();
    await waitFor(() => expect(GetProductBOM).toHaveBeenCalledWith("p1"));

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(ReplaceProductBOM).toHaveBeenCalledWith("p1", []));
  });

  it("writes the loaded recipe through on save", async () => {
    vi.mocked(GetProductBOM).mockResolvedValue([
      {
        id: "line_1",
        organizationId: "org_1",
        finishedProductId: "p1",
        componentProductId: "c1",
        quantityPerUnit: 2,
        createdAt: null,
        componentName: "Screw",
        componentSku: "SCR",
        componentUnit: null,
      },
    ]);

    await renderProductForm();
    await waitFor(() => expect(GetProductBOM).toHaveBeenCalledWith("p1"));

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(ReplaceProductBOM).toHaveBeenCalledWith("p1", [
        { componentProductId: "c1", quantityPerUnit: 2 },
      ]),
    );
  });

  it("asks before navigating away to the Bill of Materials screen when fields are dirty (F117)", async () => {
    vi.mocked(GetProductBOM).mockResolvedValue([]);

    await renderProductForm();
    await waitFor(() => expect(GetProductBOM).toHaveBeenCalledWith("p1"));

    // Mark the form dirty the way a user does — programmatic prefill via
    // setFieldsValue deliberately leaves isFieldsTouched() false.
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Widget edited" } });
    fireEvent.click(screen.getByText("Open in Bill of Materials screen"));

    expect(
      await screen.findByText(
        "Opening the Bill of Materials screen closes this product and discards your unsaved changes.",
      ),
    ).toBeInTheDocument();
  });
});
