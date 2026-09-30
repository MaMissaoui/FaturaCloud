import type { ReactNode } from "react";
import { Alert, Button, Empty, theme, Typography } from "antd";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { UserAddOutlined } from "@ant-design/icons";

import {
  CashBookCustomerRow,
  clientDetailLine,
  customerIdentifiers,
} from "src/components/cash-book/shared";
import type { CashBookState } from "src/components/cash-book/use-cash-book";

// The customer pick-list under the Cash Book search field: the match count,
// one row per match (arrow keys and Enter drive it from the field, see
// useCashBook's onSearchKeyDown), a "keep typing" footer past the first 25
// and a "Create" button when nothing matches. Shared by both layouts.
const CustomerSearchResults = ({ cb }: { cb: CashBookState }) => {
  const {
    token: { colorPrimary, colorTextSecondary },
  } = theme.useToken();
  const {
    search,
    needle,
    loanStatusFailed,
    searchResults,
    debtorCount,
    visibleSearchResults,
    hiddenResultCount,
    MAX_SEARCH_RESULTS,
    activeResultIndex,
    setActiveResultIndex,
    openLoanByClient,
    resultAriaLabel,
    selectClient,
    openNewClientModal,
    money,
  } = cb;

  // Renders `text` with its first case-insensitive occurrence of `term`
  // highlighted, so a phone/CIN match is obvious rather than something to
  // trust.
  const highlight = (text: string, term: string): ReactNode => {
    if (!term) return text;
    const at = text.toLowerCase().indexOf(term);
    if (at < 0) return text;
    return (
      <>
        {text.slice(0, at)}
        <Typography.Text style={{ color: colorPrimary, fontWeight: 600 }}>
          {text.slice(at, at + term.length)}
        </Typography.Text>
        {text.slice(at + term.length)}
      </>
    );
  };

  return (
    <>
      {loanStatusFailed && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
          message={
            <Trans>Open-loan figures couldn't be loaded — any loan shown may be incomplete</Trans>
          }
        />
      )}

      <div
        aria-live="polite"
        style={{
          marginBottom: 4,
          fontSize: 13,
          fontWeight: 600,
          color: colorTextSecondary,
        }}
      >
        <Trans>
          {searchResults.length} matches · {debtorCount} with an open loan
        </Trans>
      </div>
      {visibleSearchResults.length === 0 ? (
        <Empty description={t`No matching customers`} style={{ marginBottom: 16 }}>
          <Button
            type="dashed"
            icon={<UserAddOutlined />}
            onClick={() => openNewClientModal(search)}
          >
            {t`Create`} "{search}"
          </Button>
        </Empty>
      ) : (
        <>
          <div
            id="cash-book-results"
            role="listbox"
            aria-label={t`Search results`}
            style={{ marginBottom: hiddenResultCount > 0 ? 4 : 16 }}
          >
            {visibleSearchResults.map((client: any, index: number) => {
              const openLoan = openLoanByClient.get(client.id) ?? 0;
              return (
                <CashBookCustomerRow
                  key={client.id}
                  id={`cash-book-result-${client.id}`}
                  active={index === activeResultIndex}
                  ariaLabel={resultAriaLabel(client)}
                  title={clientDetailLine(client)}
                  name={highlight(client.name, needle)}
                  meta={customerIdentifiers(client, (text) => highlight(text, needle))}
                  outstanding={openLoan}
                  moneyText={money(openLoan)}
                  onSelect={() => selectClient(client)}
                  onHover={() => setActiveResultIndex(index)}
                />
              );
            })}
          </div>
          {hiddenResultCount > 0 && (
            <div style={{ marginBottom: 16, fontSize: 12, color: colorTextSecondary }}>
              <Trans>
                Showing the first {MAX_SEARCH_RESULTS} — keep typing to narrow {hiddenResultCount}{" "}
                more
              </Trans>
            </div>
          )}
        </>
      )}
    </>
  );
};

export default CustomerSearchResults;
