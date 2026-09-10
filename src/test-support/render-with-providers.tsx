import { Suspense } from "react";
import type { ReactElement, ReactNode } from "react";
import { act, render } from "@testing-library/react";
import { App, ConfigProvider } from "antd";
import { I18nProvider } from "@lingui/react";
import { i18n } from "@lingui/core";
import { Provider as JotaiProvider, createStore } from "jotai";
import type { Atom } from "jotai";
import { MemoryRouter, Route, Routes } from "react-router";
import { organizationAtom } from "src/atoms/organization";

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
// ASYNC ATOM SUSPENSE FIX (issue #202, root-caused and fixed here — not
// just worked around). `organizationAtom` (src/atoms/organization.ts) is
// an async derived atom several widely-used hooks read via `useAtomValue`
// (useDatePickerFormat, among others). A component suspending on it used
// to hang forever under this test environment: this is a confirmed,
// currently-open upstream bug in React 19 + @testing-library/react
// (testing-library/react-testing-library#1375 — "A component suspended
// inside an act scope, but the act call was not awaited", still
// unresolved in the latest published 16.3.3 as of this writing). React's
// Suspense retry only actually flushes if the exact same promise object
// the component's `use()` call is reading is awaited *inside the same*
// `act(async () => {...})` scope as the render itself — a separate act()
// call afterward, or awaiting an unrelated timer/promise for an equivalent
// duration, does NOT trigger the retry (verified empirically: isolated
// down to a bare `use()` + Suspense + setTimeout repro with zero Jotai or
// this app's code involved, before finding the fix). `store.get(atom)`
// returns the identical promise object `useAtomValue` reads (confirmed via
// reference equality), which is what makes this fixable at all: render()
// and `await store.get(organizationAtom)` both happen inside one
// `act(async () => {...})` below. `organizationAtom` is flushed
// unconditionally since so many components transitively depend on it that
// almost any future test would otherwise hit this; `flushAtoms` covers any
// other async atom a specific test additionally needs.
export async function renderWithProviders(
  ui: ReactElement,
  options?: {
    route?: string;
    // The route PATTERN a component's useParams() matches against (e.g.
    // "/invoices/:id") — distinct from `route`, the actual URL being
    // visited (e.g. "/invoices/new"). MemoryRouter alone never populates
    // useParams; it needs an actual <Routes><Route path={path} .../></Routes>
    // match. Defaults to `route` (no :params), which is a no-op wrapper for
    // components that don't read route params — safe for every existing
    // caller that never needed this.
    path?: string;
    jotaiStore?: ReturnType<typeof createStore>;
    flushAtoms?: Atom<unknown>[];
  },
) {
  const store = options?.jotaiStore ?? createStore();
  const route = options?.route ?? "/";
  const path = options?.path ?? route;
  const flushAtoms = [organizationAtom, ...(options?.flushAtoms ?? [])];

  const Wrapper = ({ children }: { children: ReactNode }) => (
    <JotaiProvider store={store}>
      <I18nProvider i18n={i18n}>
        <ConfigProvider>
          <App>
            <MemoryRouter initialEntries={[route]}>
              <Suspense fallback={null}>
                <Routes>
                  <Route path={path} element={children} />
                </Routes>
              </Suspense>
            </MemoryRouter>
          </App>
        </ConfigProvider>
      </I18nProvider>
    </JotaiProvider>
  );

  let result!: ReturnType<typeof render>;
  await act(async () => {
    result = render(ui, { wrapper: Wrapper });
    await Promise.all(flushAtoms.map((a) => store.get(a)));
    // One more macrotask tick after the awaited promises settle — matches
    // the minimal working repro; omitting this still leaves the retry
    // unflushed in some cases even with the exact-promise await above.
    await new Promise((resolve) => setTimeout(resolve, 0));
  });

  return { store, ...result };
}
