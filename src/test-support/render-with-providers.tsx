import { Suspense } from "react";
import type { ReactElement, ReactNode } from "react";
import { render } from "@testing-library/react";
import { App, ConfigProvider } from "antd";
import { I18nProvider } from "@lingui/react";
import { i18n } from "@lingui/core";
import { Provider as JotaiProvider, createStore } from "jotai";
import { MemoryRouter } from "react-router";

// Shared harness for issue #175's component tests — every provider a real
// page/component under src/routes or src/components can depend on: Jotai
// (a fresh store per render, so tests don't leak atom state into each
// other), antd's ConfigProvider + App (App.useApp()'s message/modal/
// notification statics throw without an <App> ancestor — PaymentPanel's
// message.error calls need this), Lingui's I18nProvider (vitest.setup.ts
// already activates the "en" locale with no catalog loaded, so Trans/t
// render their source-locale text), and a MemoryRouter for anything using
// react-router hooks (useNavigate, useParams, Link, ...). Not every
// component under test needs all of these — PaymentPanel (the first smoke
// test) needs no router/Jotai directly — but future tests (the invoice
// detail page the issue itself suggested) will, so the wrapper provides
// all of them rather than growing ad hoc per test.
//
// The Suspense boundary is here for components that use a well-behaved
// async atom — it is NOT a fix for the specific case below, which has no
// known fix yet and needs its own per-test workaround.
//
// KNOWN LIMITATION (investigated, not yet resolved): `organizationAtom`
// (src/atoms/organization.ts) is an async derived atom read via
// `useAtomValue` by several widely-used hooks, including
// `useDatePickerFormat`. In this Jotai 2.20.3 + React 19.2 + jsdom/Vitest
// environment, a component suspending on it never recovers, even though
// the atom itself resolves near-instantly — confirmed via `store.get()`
// directly (resolves in <1ms) and via pre-warming the store's cache before
// mount (still re-suspends and never recovers). This looks like a
// Jotai-React-Suspense-retry integration bug specific to this environment,
// not anything wrong with the atom or the component under test — it did
// not reproduce for `pnpm dev`'s real app, only under Vitest/jsdom.
// Filed as its own follow-up rather than solved here (see the PR body for
// #175); would need real investigation (a minimal jotai+RTL+React19 repro,
// checking for a jotai/RTL version combination known to fix it) to resolve
// properly.
//
// Workaround for any test whose component transitively reads
// `organizationAtom` (PaymentPanel's smoke test needs this, via
// `useDatePickerFormat`): mock the consuming hook directly rather than
// going through the atom —
//   vi.mock("src/utils/date", () => ({
//     useDatePickerFormat: () => "MM/DD/YYYY",
//     useDateTimePickerFormat: () => "MM/DD/YYYY HH:mm",
//   }));
// This is scoped to the test file that needs it (not a global mock here),
// since most future component tests won't touch this hook at all and a
// blanket mock would silently hide the real gap from tests that should
// exercise it once someone does track down the root cause.
export function renderWithProviders(
  ui: ReactElement,
  options?: { route?: string; jotaiStore?: ReturnType<typeof createStore> },
) {
  const store = options?.jotaiStore ?? createStore();
  const route = options?.route ?? "/";

  const Wrapper = ({ children }: { children: ReactNode }) => (
    <JotaiProvider store={store}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <MemoryRouter initialEntries={[route]}>
              <Suspense fallback={null}>{children}</Suspense>
            </MemoryRouter>
          </App>
        </ConfigProvider>
      </I18nProvider>
    </JotaiProvider>
  );

  return { store, ...render(ui, { wrapper: Wrapper }) };
}
