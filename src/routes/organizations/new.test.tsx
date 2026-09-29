import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router";
import { App, ConfigProvider } from "antd";
import { I18nProvider } from "@lingui/react";
import { i18n } from "@lingui/core";
import { Provider as JotaiProvider, createStore } from "jotai";
import NewOrganization from "./new";

vi.mock("src/api", () => ({
  CreateOrganization: vi.fn(),
  GetOrganizations: vi.fn(),
  GetOrganization: vi.fn(),
  GetMyOrganizationRole: vi.fn(),
  GetOrganizationLogoDataUri: vi.fn(),
  Logout: vi.fn(),
}));

// Pinned so the test doesn't depend on the runner's own zone.
vi.mock("src/utils/timezones", async (importOriginal) => ({
  ...(await importOriginal<typeof import("src/utils/timezones")>()),
  defaultTimezone: () => "Africa/Tunis",
}));

import { CreateOrganization, GetOrganizations } from "src/api";

async function renderNewOrganization() {
  const store = createStore();
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <JotaiProvider store={store}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <MemoryRouter initialEntries={["/organizations/new"]}>{children}</MemoryRouter>
          </App>
        </ConfigProvider>
      </I18nProvider>
    </JotaiProvider>
  );
  await act(async () => {
    render(<NewOrganization />, { wrapper: Wrapper });
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

describe("NewOrganization", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(GetOrganizations).mockResolvedValue([]);
    vi.mocked(CreateOrganization).mockImplementation(
      async (req: any) => ({ ...req, id: req.id ?? "org_new" }) as never,
    );
  });

  // Audit F156: the server never infers a zone, and an organization created
  // without one reads every date's calendar day in UTC — a picked date then
  // prints a day early anywhere east of UTC.
  it("creates the organization with the browser's time zone prefilled", async () => {
    await renderNewOrganization();
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Test SARL" } });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Create Organization" }));
    });
    await waitFor(() => expect(CreateOrganization).toHaveBeenCalledTimes(1));
    expect(vi.mocked(CreateOrganization).mock.calls[0][0]).toMatchObject({
      name: "Test SARL",
      timezone: "Africa/Tunis",
    });
  });
});
