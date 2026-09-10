import { describe, expect, it } from "vitest";
import { screen } from "@testing-library/react";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { renderWithProviders } from "./render-with-providers";

// Not a test of app behavior — a permanent regression guard for the tooling
// itself. Issue #175 flagged the Lingui Babel macro transform under Vitest
// as unverified; getting this wrong is silent (Trans/t compile fine without
// the plugin, then throw a runtime error with no obvious cause — see the
// PR description for what that looked like) rather than a build failure, so
// it's worth a standing test rather than a one-off check during that PR.
const UsesLinguiMacros = () => (
  <div>
    <Trans>hello</Trans>
    <span>{t`world`}</span>
  </div>
);

describe("Lingui macros under Vitest", () => {
  it("compiles Trans and t`` and renders their source-locale text", async () => {
    await renderWithProviders(<UsesLinguiMacros />);
    expect(screen.getByText("hello")).toBeInTheDocument();
    expect(screen.getByText("world")).toBeInTheDocument();
  });
});
