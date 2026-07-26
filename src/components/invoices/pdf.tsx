import { Trans } from "@lingui/react/macro";
import { I18nProvider } from "@lingui/react";
import { Document, Page, Text, View, StyleSheet, Image } from "@react-pdf/renderer";

import { getFormattedNumber } from "src/utils/currencies";
import { formatDate } from "src/utils/date";
import { formatAddress } from "src/utils/address";

const FONT = "Helvetica";
const FONT_BOLD = "Helvetica-Bold";
// Dark slate header block + light neutral cards, matching the reference the
// design was modeled on rather than the app's own UI blue — this is a print
// document, not a screen, and stays legible/professional in mono.
const HEADER_BG = "#1e293b";

const styles = StyleSheet.create({
  page: { fontFamily: FONT, fontSize: 10, color: "#222", padding: 50, flexDirection: "column" },

  // Escapes the page's own padding on three sides so the block reads as a
  // full-bleed banner, then re-applies padding to its own content.
  headerBlock: {
    backgroundColor: HEADER_BG,
    marginTop: -50,
    marginHorizontal: -50,
    paddingHorizontal: 50,
    paddingTop: 36,
    paddingBottom: 28,
    marginBottom: 28,
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "flex-start",
  },
  orgName: { fontFamily: FONT_BOLD, fontSize: 18, color: "#fff" },
  logo: { maxWidth: 120, maxHeight: 44, objectFit: "contain", marginTop: 10 },
  docTitle: { fontFamily: FONT_BOLD, fontSize: 26, textAlign: "right", color: "#fff" },
  docNumberPill: {
    alignSelf: "flex-end",
    backgroundColor: "rgba(255,255,255,0.15)",
    borderRadius: 3,
    paddingVertical: 4,
    paddingHorizontal: 10,
    marginTop: 8,
  },
  docNumberText: { fontSize: 11, color: "#fff" },
  headerMeta: { marginTop: 12, alignItems: "flex-end" },
  headerMetaRow: { flexDirection: "row", marginTop: 3 },
  headerMetaLabel: { fontSize: 9, color: "rgba(255,255,255,0.65)", marginRight: 6 },
  headerMetaValue: { fontFamily: FONT_BOLD, fontSize: 9, color: "#fff" },

  parties: { flexDirection: "row", gap: 16, marginBottom: 24 },
  card: { flex: 1, backgroundColor: "#f8f9fa", borderRadius: 3, padding: 14 },
  partyLabel: {
    fontFamily: FONT_BOLD,
    fontSize: 8,
    color: "#888",
    textTransform: "uppercase",
    letterSpacing: 1,
    marginBottom: 6,
  },
  partyName: { fontFamily: FONT_BOLD, fontSize: 11, marginBottom: 3 },
  partyDetail: { color: "#555", marginBottom: 2, lineHeight: 1.5, fontSize: 9 },

  /* Line items */
  table: { width: "auto" },
  tableHeader: {
    flexDirection: "row",
    backgroundColor: HEADER_BG,
    paddingVertical: 7,
    paddingHorizontal: 8,
  },
  tableHeaderText: {
    fontFamily: FONT_BOLD,
    fontSize: 8,
    color: "#fff",
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  tableRow: {
    flexDirection: "row",
    paddingVertical: 7,
    paddingHorizontal: 8,
    borderBottomWidth: 1,
    borderBottomColor: "#eee",
  },
  tableRowAlt: { backgroundColor: "#f8f9fa" },
  tableCol: { fontFamily: FONT, fontSize: 9 },
  colNumber: { width: "4%" },
  colDescription: { width: "52%" },
  colQuantity: { width: "10%" },
  colUnitPrice: { width: "12%", textAlign: "left" },
  colTax: { width: "10%", textAlign: "left" },
  colTotal: { width: "12%", textAlign: "right", fontFamily: FONT_BOLD },

  belowTable: { flexDirection: "row", gap: 16, marginTop: 20, alignItems: "flex-start" },
  noteCard: {
    flex: 1,
    backgroundColor: "#f8f9fa",
    borderRadius: 3,
    padding: 14,
    borderLeftWidth: 3,
    borderLeftColor: HEADER_BG,
  },
  noteCardTitle: { fontFamily: FONT_BOLD, fontSize: 10, marginBottom: 6 },
  noteCardText: { fontSize: 9, color: "#555", lineHeight: 1.5 },
  qrRow: { flexDirection: "row", alignItems: "center", marginTop: 8, gap: 10 },
  qrImage: { width: 56, height: 56 },
  qrCaption: { fontSize: 8, color: "#777", lineHeight: 1.4, maxWidth: 140 },

  totalsBox: { width: "38%" },
  totalRow: { flexDirection: "row", justifyContent: "space-between", paddingVertical: 4 },
  totalLabel: { fontSize: 9, color: "#555" },
  totalValue: { fontSize: 9 },
  grandTotalRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    borderTopWidth: 2,
    borderTopColor: HEADER_BG,
    marginTop: 6,
    paddingTop: 8,
  },
  grandTotalLabel: { fontFamily: FONT_BOLD, fontSize: 12 },
  grandTotalValue: { fontFamily: FONT_BOLD, fontSize: 12 },

  paymentDetails: { marginTop: 16, flexDirection: "row", gap: 16 },

  footer: {
    position: "absolute",
    bottom: 30,
    left: 50,
    right: 50,
    borderTopStyle: "solid",
    borderTopWidth: 1,
    borderTopColor: "#e0e0e0",
    paddingTop: 8,
    flexDirection: "row",
    justifyContent: "space-between",
  },
  footerText: { fontFamily: FONT, fontSize: 8, color: "#888" },
  pageNumber: { position: "absolute", bottom: 30, right: 50, fontSize: 8, color: "#aaa" },
});

const InvoicePDF = ({
  invoice,
  client,
  organization,
  taxRates,
  i18n,
  qrCodeDataUri,
}: {
  invoice: any;
  client: any;
  organization: any;
  taxRates: any;
  i18n: any;
  qrCodeDataUri?: string | null;
}) => {
  const dateFormat = organization?.date_format;
  // Group line items by tax rate and calculate tax for each group
  const taxGroups = (() => {
    const groups: { [key: string]: { taxRate: any; items: any[]; subtotal: number; tax: number } } =
      {};

    invoice.lineItems?.forEach((item: any) => {
      if (item.total) {
        const taxRateId = item.taxRate || "no-tax";
        const taxRate = item.taxRate ? taxRates?.find((rate: any) => rate.id === taxRateId) : null;

        if (!groups[taxRateId]) {
          groups[taxRateId] = {
            taxRate,
            items: [],
            subtotal: 0,
            tax: 0,
          };
        }

        groups[taxRateId].items.push(item);
        groups[taxRateId].subtotal += item.total;
        groups[taxRateId].tax = taxRate?.percentage
          ? (groups[taxRateId].subtotal * taxRate.percentage) / 100
          : 0;
      }
    });

    return Object.values(groups);
  })();

  const clientEmail = (() => {
    if (!client.emails) return null;
    try {
      const emailsArray =
        typeof client.emails === "string" ? JSON.parse(client.emails) : client.emails;
      return Array.isArray(emailsArray) && emailsArray[0] ? emailsArray[0] : null;
    } catch {
      return null;
    }
  })();

  const fmt = (value: number) => getFormattedNumber(value, invoice.currency, i18n.locale, organization);

  return (
    <I18nProvider i18n={i18n}>
      <Document>
        <Page size="A4" style={styles.page}>
          {/* Header */}
          <View style={styles.headerBlock}>
            <View>
              <Text style={styles.orgName}>{organization.name}</Text>
              {organization.logo && <Image src={organization.logo} style={styles.logo} />}
            </View>
            <View>
              <Text style={styles.docTitle}>
                <Trans>Invoice</Trans>
              </Text>
              <View style={styles.docNumberPill}>
                <Text style={styles.docNumberText}>{invoice.number}</Text>
              </View>
              <View style={styles.headerMeta}>
                <View style={styles.headerMetaRow}>
                  <Text style={styles.headerMetaLabel}>
                    <Trans>Date</Trans>:
                  </Text>
                  <Text style={styles.headerMetaValue}>{formatDate(invoice.date, dateFormat)}</Text>
                </View>
                <View style={styles.headerMetaRow}>
                  <Text style={styles.headerMetaLabel}>
                    <Trans>Due date</Trans>:
                  </Text>
                  <Text style={styles.headerMetaValue}>
                    {invoice.dueDate ? formatDate(invoice.dueDate, dateFormat) : "-"}
                  </Text>
                </View>
                {invoice.overdueCharge ? (
                  <View style={styles.headerMetaRow}>
                    <Text style={styles.headerMetaLabel}>
                      <Trans>Overdue charge</Trans>:
                    </Text>
                    <Text style={styles.headerMetaValue}>{invoice.overdueCharge}%</Text>
                  </View>
                ) : null}
              </View>
            </View>
          </View>

          {/* Bill from / Bill to */}
          <View style={styles.parties}>
            <View style={styles.card}>
              <Text style={styles.partyLabel}>
                <Trans>Bill From</Trans>
              </Text>
              <Text style={styles.partyName}>{organization.name}</Text>
              {formatAddress(organization) && (
                <Text style={styles.partyDetail}>{formatAddress(organization)}</Text>
              )}
              {organization.email && <Text style={styles.partyDetail}>{organization.email}</Text>}
              {organization.website && <Text style={styles.partyDetail}>{organization.website}</Text>}
            </View>
            <View style={styles.card}>
              <Text style={styles.partyLabel}>
                <Trans>Bill To</Trans>
              </Text>
              <Text style={styles.partyName}>{client.name}</Text>
              {formatAddress(client) && (
                <Text style={styles.partyDetail}>{formatAddress(client)}</Text>
              )}
              {clientEmail && <Text style={styles.partyDetail}>{clientEmail}</Text>}
              {client.website && <Text style={styles.partyDetail}>{client.website}</Text>}
            </View>
          </View>

          {/* Items table */}
          <View style={styles.table}>
            <View style={styles.tableHeader}>
              <Text style={[styles.tableHeaderText, styles.colNumber]}>#</Text>
              <Text style={[styles.tableHeaderText, styles.colDescription]}>
                <Trans>Description</Trans>
              </Text>
              <Text style={[styles.tableHeaderText, styles.colQuantity]}>
                <Trans>Qty.</Trans>
              </Text>
              <Text style={[styles.tableHeaderText, styles.colUnitPrice]}>
                <Trans>Price</Trans>
              </Text>
              <Text style={[styles.tableHeaderText, styles.colTax]}>
                <Trans>Tax %</Trans>
              </Text>
              <Text style={[styles.tableHeaderText, styles.colTotal]}>
                <Trans>Total</Trans>
              </Text>
            </View>
            {invoice.lineItems?.map((lineItem: any, index: number) => {
              const taxRate = taxRates?.find((rate: any) => rate.id === lineItem.taxRate);
              return (
                <View
                  key={lineItem.id ?? index}
                  style={[styles.tableRow, ...(index % 2 === 1 ? [styles.tableRowAlt] : [])]}
                >
                  <Text style={[styles.tableCol, styles.colNumber]}>{index + 1}</Text>
                  <Text style={[styles.tableCol, styles.colDescription]}>{lineItem.description}</Text>
                  <Text style={[styles.tableCol, styles.colQuantity]}>{lineItem.quantity}</Text>
                  <Text style={[styles.tableCol, styles.colUnitPrice]}>{fmt(lineItem.unitPrice)}</Text>
                  <Text style={[styles.tableCol, styles.colTax]}>
                    {taxRate ? `${taxRate.percentage}%` : ""}
                  </Text>
                  <Text style={[styles.tableCol, styles.colTotal]}>{fmt(lineItem.total)}</Text>
                </View>
              );
            })}
          </View>

          {/* Notes/QR + totals */}
          <View style={styles.belowTable}>
            {qrCodeDataUri ? (
              <View style={styles.noteCard}>
                <Text style={styles.noteCardTitle}>
                  <Trans>Thank you for your business</Trans>
                </Text>
                {invoice.customerNotes && (
                  <Text style={styles.noteCardText}>{invoice.customerNotes}</Text>
                )}
                <View style={styles.qrRow}>
                  <Image src={qrCodeDataUri} style={styles.qrImage} />
                  <Text style={styles.qrCaption}>
                    <Trans>Scan with your banking app to pay this invoice.</Trans>
                  </Text>
                </View>
              </View>
            ) : (
              invoice.customerNotes && (
                <View style={styles.noteCard}>
                  <Text style={styles.noteCardTitle}>
                    <Trans>Notes</Trans>
                  </Text>
                  <Text style={styles.noteCardText}>{invoice.customerNotes}</Text>
                </View>
              )
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
                      `${group.taxRate.name} ${group.taxRate.percentage}%`
                    ) : (
                      <Trans>Tax 0%</Trans>
                    )}
                  </Text>
                  <Text style={styles.totalValue}>{fmt(group.tax)}</Text>
                </View>
              ))}
              <View style={styles.grandTotalRow}>
                <Text style={styles.grandTotalLabel}>
                  <Trans>Total</Trans>
                </Text>
                <Text style={styles.grandTotalValue}>{fmt(invoice.total)}</Text>
              </View>
            </View>
          </View>

          {/* Payment details */}
          {(organization.bank_name || organization.iban) && (
            <View style={styles.paymentDetails}>
              <View style={styles.card}>
                <Text style={styles.partyLabel}>
                  <Trans>Payment Details</Trans>
                </Text>
                {organization.bank_name && (
                  <Text style={styles.partyDetail}>{organization.bank_name}</Text>
                )}
                {organization.iban && <Text style={styles.partyDetail}>IBAN {organization.iban}</Text>}
                {organization.bic && <Text style={styles.partyDetail}>BIC {organization.bic}</Text>}
              </View>
            </View>
          )}

          <View style={styles.footer}>
            <Text style={styles.footerText}>
              {organization.registration_number ? `Reg. nr ${organization.registration_number}` : ""}
            </Text>
            <Text style={styles.footerText}>
              {organization.vatin ? `VATIN ${organization.vatin}` : ""}
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

export default InvoicePDF;
