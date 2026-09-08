import { Trans } from "@lingui/react/macro";
import { I18nProvider } from "@lingui/react";
import { Document, Page, Text, View, StyleSheet, Image } from "@react-pdf/renderer";

import { getFormattedNumber } from "src/utils/currencies";
import { formatDate } from "src/utils/date";
import { formatAddress } from "src/utils/address";

// A plain, text-first layout modeled on how Tunisian invoicing tools
// commonly present the two figures a Tunisian buyer/seller pair actually
// cares about beyond the EU-style total: the fiscal stamp (timbre fiscal,
// a flat non-taxable duty) and, when the client withholds tax at source,
// the net amount the seller will actually receive — see invoice.tsx's
// derivation comment for why the withholding base excludes the stamp.
// Deliberately no colored header banner (unlike pdf.tsx) — this layout is
// the "no logo, no bells" reference the Tunisia support was modeled on.
const FONT = "Helvetica";
const FONT_BOLD = "Helvetica-Bold";
const TOTAL_DUE_BG = "#dbeafe";
const NET_DUE_BG = "#d1fae5";

const styles = StyleSheet.create({
  page: { fontFamily: FONT, fontSize: 10, color: "#222", padding: 50 },

  header: { flexDirection: "row", justifyContent: "space-between", marginBottom: 32 },
  sellerName: { fontFamily: FONT_BOLD, fontSize: 13, marginBottom: 4 },
  sellerDetail: { fontSize: 9, color: "#444", lineHeight: 1.5 },
  logo: { maxWidth: 120, maxHeight: 44, objectFit: "contain", marginBottom: 8 },

  docBlock: { alignItems: "flex-end" },
  docLabel: {
    fontSize: 8,
    color: "#888",
    textTransform: "uppercase",
    letterSpacing: 1,
    marginBottom: 2,
  },
  docNumber: { fontFamily: FONT_BOLD, fontSize: 14, marginBottom: 10 },
  docMetaLabel: { fontSize: 8, color: "#888", marginTop: 6 },
  docMetaValue: { fontSize: 9, color: "#222" },

  billTo: { marginBottom: 24 },
  billToLabel: { fontSize: 8, color: "#888", textTransform: "uppercase", marginBottom: 4 },
  billToName: { fontFamily: FONT_BOLD, fontSize: 11, marginBottom: 3 },
  billToDetail: { fontSize: 9, color: "#444", lineHeight: 1.5 },

  table: { width: "auto" },
  tableHeader: {
    flexDirection: "row",
    borderBottomWidth: 1,
    borderBottomColor: "#222",
    paddingBottom: 6,
  },
  tableHeaderText: {
    fontSize: 8,
    color: "#888",
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  tableRow: {
    flexDirection: "row",
    paddingVertical: 8,
    borderBottomWidth: 1,
    borderBottomColor: "#eee",
  },
  tableCol: { fontFamily: FONT, fontSize: 9 },
  colDescription: { width: "40%" },
  colQuantity: { width: "12%", textAlign: "right" },
  colUnitPrice: { width: "18%", textAlign: "right" },
  colTax: { width: "12%", textAlign: "right" },
  colTotal: { width: "18%", textAlign: "right" },

  belowTable: { flexDirection: "row", justifyContent: "space-between", marginTop: 20 },
  paymentTerms: { fontSize: 9, color: "#555", maxWidth: "45%" },

  totalsBox: { width: "45%" },
  totalRow: { flexDirection: "row", justifyContent: "space-between", paddingVertical: 4 },
  totalLabel: { fontSize: 9, color: "#555" },
  totalValue: { fontSize: 9 },
  highlightRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    borderRadius: 3,
    paddingVertical: 6,
    paddingHorizontal: 10,
    marginTop: 6,
  },
  highlightLabel: { fontFamily: FONT_BOLD, fontSize: 10 },
  highlightValue: { fontFamily: FONT_BOLD, fontSize: 10 },

  footer: {
    position: "absolute",
    bottom: 30,
    left: 50,
    right: 50,
    borderTopWidth: 1,
    borderTopColor: "#e0e0e0",
    paddingTop: 8,
    flexDirection: "row",
    justifyContent: "space-between",
  },
  footerText: { fontFamily: FONT, fontSize: 8, color: "#888" },
  pageNumber: { position: "absolute", bottom: 30, right: 50, fontSize: 8, color: "#aaa" },
});

const InvoicePDFTunisia = ({
  invoice,
  client,
  organization,
  taxRates,
  i18n,
}: {
  invoice: any;
  client: any;
  organization: any;
  taxRates: any;
  i18n: any;
  qrCodeDataUri?: string | null;
}) => {
  const dateFormat = organization?.date_format;

  // Group line items by tax rate — same shape as pdf.tsx's grouping, kept
  // as a separate copy rather than a shared helper since the two layouts'
  // rendering (colors, which fields show) diverge enough that a shared
  // "taxGroups" util would need to carry layout-specific options anyway.
  const taxGroups = (() => {
    const groups: { [key: string]: { taxRate: any; subtotal: number; tax: number } } = {};
    invoice.lineItems?.forEach((item: any) => {
      if (!item.total) return;
      const taxRateId = item.taxRate || "no-tax";
      const taxRate = item.taxRate ? taxRates?.find((rate: any) => rate.id === taxRateId) : null;
      if (!groups[taxRateId]) {
        groups[taxRateId] = { taxRate, subtotal: 0, tax: 0 };
      }
      groups[taxRateId].subtotal += item.total;
      groups[taxRateId].tax = taxRate?.percentage
        ? (groups[taxRateId].subtotal * taxRate.percentage) / 100
        : 0;
    });
    return Object.values(groups);
  })();

  const fmt = (value: number) =>
    getFormattedNumber(value, invoice.currency, i18n.locale, organization);

  const hasWithholding = !!invoice.withholdingTaxRate;
  const netAmountDue = hasWithholding
    ? invoice.total - (invoice.withholdingTaxAmount || 0)
    : invoice.total;

  return (
    <I18nProvider i18n={i18n}>
      <Document>
        <Page size="A4" style={styles.page}>
          <View style={styles.header}>
            <View>
              {organization.logo && <Image src={organization.logo} style={styles.logo} />}
              <Text style={styles.sellerName}>{organization.name}</Text>
              {formatAddress(organization) && (
                <Text style={styles.sellerDetail}>{formatAddress(organization)}</Text>
              )}
              {organization.phone && <Text style={styles.sellerDetail}>{organization.phone}</Text>}
              {organization.email && <Text style={styles.sellerDetail}>{organization.email}</Text>}
            </View>
            <View style={styles.docBlock}>
              <Text style={styles.docLabel}>
                <Trans>Invoice</Trans>
              </Text>
              <Text style={styles.docNumber}>{invoice.number}</Text>
              <Text style={styles.docMetaLabel}>
                <Trans>Issue date</Trans>
              </Text>
              <Text style={styles.docMetaValue}>{formatDate(invoice.date, dateFormat)}</Text>
              <Text style={styles.docMetaLabel}>
                <Trans>Due date</Trans>
              </Text>
              <Text style={styles.docMetaValue}>
                {invoice.dueDate ? formatDate(invoice.dueDate, dateFormat) : "-"}
              </Text>
            </View>
          </View>

          <View style={styles.billTo}>
            <Text style={styles.billToLabel}>
              <Trans>Bill To</Trans>
            </Text>
            <Text style={styles.billToName}>{client.name}</Text>
            {formatAddress(client) && (
              <Text style={styles.billToDetail}>{formatAddress(client)}</Text>
            )}
            {client.vatin && <Text style={styles.billToDetail}>MF : {client.vatin}</Text>}
          </View>

          <View style={styles.table}>
            <View style={styles.tableHeader}>
              <Text style={[styles.tableHeaderText, styles.colDescription]}>
                <Trans>Description</Trans>
              </Text>
              <Text style={[styles.tableHeaderText, styles.colQuantity]}>
                <Trans>Qty.</Trans>
              </Text>
              <Text style={[styles.tableHeaderText, styles.colUnitPrice]}>
                <Trans>Unit price excl. tax</Trans>
              </Text>
              <Text style={[styles.tableHeaderText, styles.colTax]}>
                <Trans>Tax</Trans>
              </Text>
              <Text style={[styles.tableHeaderText, styles.colTotal]}>
                <Trans>Total excl. tax</Trans>
              </Text>
            </View>
            {invoice.lineItems?.map((lineItem: any, index: number) => {
              const taxRate = taxRates?.find((rate: any) => rate.id === lineItem.taxRate);
              return (
                <View key={lineItem.id ?? index} style={styles.tableRow}>
                  <Text style={[styles.tableCol, styles.colDescription]}>
                    {lineItem.description}
                  </Text>
                  <Text style={[styles.tableCol, styles.colQuantity]}>{lineItem.quantity}</Text>
                  <Text style={[styles.tableCol, styles.colUnitPrice]}>
                    {fmt(lineItem.unitPrice)}
                  </Text>
                  <Text style={[styles.tableCol, styles.colTax]}>
                    {taxRate ? `${taxRate.percentage}%` : ""}
                  </Text>
                  <Text style={[styles.tableCol, styles.colTotal]}>{fmt(lineItem.total)}</Text>
                </View>
              );
            })}
          </View>

          <View style={styles.belowTable}>
            {invoice.paymentTerms ? (
              <Text style={styles.paymentTerms}>
                <Trans>Payment terms</Trans>: {invoice.paymentTerms}
              </Text>
            ) : (
              <View />
            )}

            <View style={styles.totalsBox}>
              <View style={styles.totalRow}>
                <Text style={styles.totalLabel}>
                  <Trans>Subtotal</Trans>
                </Text>
                <Text style={styles.totalValue}>{fmt(invoice.subTotal)}</Text>
              </View>
              {taxGroups.map((group, index) => (
                <View key={`tax-${index}`} style={styles.totalRow}>
                  <Text style={styles.totalLabel}>
                    {group.taxRate ? (
                      `${group.taxRate.name} (${group.taxRate.percentage}%)`
                    ) : (
                      <Trans>Tax 0%</Trans>
                    )}
                  </Text>
                  <Text style={styles.totalValue}>{fmt(group.tax)}</Text>
                </View>
              ))}
              {invoice.fiscalStampAmount > 0 && (
                <View style={styles.totalRow}>
                  <Text style={styles.totalLabel}>
                    <Trans>Fiscal stamp</Trans>
                  </Text>
                  <Text style={styles.totalValue}>{fmt(invoice.fiscalStampAmount)}</Text>
                </View>
              )}
              <View style={[styles.highlightRow, { backgroundColor: TOTAL_DUE_BG }]}>
                <Text style={styles.highlightLabel}>
                  <Trans>Total due</Trans>
                </Text>
                <Text style={styles.highlightValue}>{fmt(invoice.total)}</Text>
              </View>
              {hasWithholding && (
                <>
                  <View style={styles.totalRow}>
                    <Text style={styles.totalLabel}>
                      <Trans>Withholding tax</Trans> ({invoice.withholdingTaxRate}%)
                    </Text>
                    <Text style={styles.totalValue}>-{fmt(invoice.withholdingTaxAmount || 0)}</Text>
                  </View>
                  <View style={[styles.highlightRow, { backgroundColor: NET_DUE_BG }]}>
                    <Text style={styles.highlightLabel}>
                      <Trans>Net amount due</Trans>
                    </Text>
                    <Text style={styles.highlightValue}>{fmt(netAmountDue)}</Text>
                  </View>
                </>
              )}
            </View>
          </View>

          <View style={styles.footer}>
            <Text style={styles.footerText}>
              {organization.registration_number
                ? `Reg. nr ${organization.registration_number}`
                : ""}
            </Text>
            <Text style={styles.footerText}>
              {organization.vatin ? `MF ${organization.vatin}` : ""}
            </Text>
          </View>
          {/* react-pdf's render prop runs outside React's normal reconciler,
              so hooks-based i18n like <Trans> throws inside it — plain text only. */}
          <Text
            style={styles.pageNumber}
            fixed
            render={({ pageNumber, totalPages }) => `Page ${pageNumber} of ${totalPages}`}
          />
        </Page>
      </Document>
    </I18nProvider>
  );
};

export default InvoicePDFTunisia;
