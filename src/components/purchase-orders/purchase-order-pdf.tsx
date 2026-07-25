import { Document, Page, Text, View, StyleSheet, Image } from "@react-pdf/renderer";
import { I18nProvider } from "@lingui/react";
import { Trans } from "@lingui/react/macro";
import dayjs from "dayjs";

const FONT = "Helvetica";
const FONT_BOLD = "Helvetica-Bold";
// Dark slate header block + light neutral cards — matches the invoice PDF
// (src/components/invoices/pdf.tsx) so every document in the app reads as
// one designed system.
const HEADER_BG = "#1e293b";

const styles = StyleSheet.create({
  page: { fontFamily: FONT, fontSize: 10, color: "#222", padding: 50, flexDirection: "column" },

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
  docTitle: { fontFamily: FONT_BOLD, fontSize: 22, textAlign: "right", color: "#fff" },
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
  colNum: { width: 28, fontFamily: FONT_BOLD, color: "#888" },
  colDesc: { flex: 1 },
  colQty: { width: 60, textAlign: "right" },
  colUnit: { width: 50, textAlign: "right" },
  colPrice: { width: 80, textAlign: "right" },
  colTotal: { width: 80, textAlign: "right", fontFamily: FONT_BOLD },
  itemCode: { fontSize: 8, color: "#888", marginBottom: 1 },

  totals: { marginTop: 16, alignItems: "flex-end" },
  totalRow: { flexDirection: "row", paddingVertical: 4 },
  grandTotalLabel: { fontFamily: FONT_BOLD, fontSize: 12, width: 80, textAlign: "right", marginRight: 16 },
  grandTotalValue: { fontFamily: FONT_BOLD, fontSize: 12, width: 80, textAlign: "right" },

  noteCard: {
    marginTop: 24,
    backgroundColor: "#f8f9fa",
    borderRadius: 3,
    padding: 14,
    borderLeftWidth: 3,
    borderLeftColor: HEADER_BG,
  },
  noteCardTitle: { fontFamily: FONT_BOLD, fontSize: 10, marginBottom: 6 },
  noteCardText: { fontSize: 9, color: "#555", lineHeight: 1.5 },

  footer: { marginTop: 40, borderTopWidth: 1, borderTopColor: "#e0e0e0", paddingTop: 12 },
  footerText: { fontSize: 9, color: "#888", textAlign: "center" },
  pageNumber: { position: "absolute", bottom: 24, right: 50, fontSize: 8, color: "#aaa" },
});

interface Props {
  order: any;
  lineItems: any[];
  vendor: any;
  organization: any;
  i18n: any;
}

// The purchase order is sent to the vendor, so it carries prices and totals —
// unlike a goods receipt note. Strings are translated via the passed-in i18n
// (the invoices/pdf.tsx pattern), not hardcoded English.
const PurchaseOrderPDF = ({ order, lineItems, vendor, organization, i18n }: Props) => {
  const currency = order?.currency ?? organization?.currency ?? "EUR";
  // organizationAtom already resolves logo to a ready-to-use data URI
  // (fetched from GET /organizations/{id}/logo) — don't re-wrap it.
  const logoSrc = organization?.logo ?? null;
  // Takes cents — the caller pre-multiplies.
  const fmt = (cents: number) =>
    Intl.NumberFormat(i18n.locale, { style: "currency", currency }).format(cents / 100);

  const subtotal = lineItems.reduce((total, item) => {
    const qty = item.quantity ?? 1;
    const price = item.unitPrice ?? 0;
    return total + qty * price;
  }, 0);

  return (
    <I18nProvider i18n={i18n}>
      <Document>
        <Page size="A4" style={styles.page}>
          {/* Header */}
          <View style={styles.headerBlock}>
            <View>
              <Text style={styles.orgName}>{organization?.name ?? ""}</Text>
              {logoSrc && <Image src={logoSrc} style={styles.logo} />}
            </View>
            <View>
              <Text style={styles.docTitle}>
                <Trans>PURCHASE ORDER</Trans>
              </Text>
              <View style={styles.docNumberPill}>
                <Text style={styles.docNumberText}>{order.orderNumber}</Text>
              </View>
              <View style={styles.headerMeta}>
                <View style={styles.headerMetaRow}>
                  <Text style={styles.headerMetaLabel}>
                    <Trans>Order Date</Trans>:
                  </Text>
                  <Text style={styles.headerMetaValue}>{dayjs(order.orderDate).format("L")}</Text>
                </View>
                {order.expectedDate && (
                  <View style={styles.headerMetaRow}>
                    <Text style={styles.headerMetaLabel}>
                      <Trans>Expected Date</Trans>:
                    </Text>
                    <Text style={styles.headerMetaValue}>{dayjs(order.expectedDate).format("L")}</Text>
                  </View>
                )}
                {order.deliveryAddress && (
                  <View style={styles.headerMetaRow}>
                    <Text style={styles.headerMetaLabel}>
                      <Trans>Deliver To</Trans>:
                    </Text>
                    <Text style={styles.headerMetaValue}>{order.deliveryAddress}</Text>
                  </View>
                )}
              </View>
            </View>
          </View>

          {/* Buyer / Vendor */}
          <View style={styles.parties}>
            <View style={styles.card}>
              <Text style={styles.partyLabel}>
                <Trans>Ordered By</Trans>
              </Text>
              <Text style={styles.partyName}>{organization?.name ?? ""}</Text>
              {organization?.address && <Text style={styles.partyDetail}>{organization.address}</Text>}
              {organization?.email && <Text style={styles.partyDetail}>{organization.email}</Text>}
              {organization?.phone && <Text style={styles.partyDetail}>{organization.phone}</Text>}
              {organization?.vatin && (
                <Text style={styles.partyDetail}>
                  <Trans>VAT</Trans>: {organization.vatin}
                </Text>
              )}
            </View>
            <View style={styles.card}>
              <Text style={styles.partyLabel}>
                <Trans>Vendor</Trans>
              </Text>
              <Text style={styles.partyName}>{vendor?.name ?? ""}</Text>
              {vendor?.address && <Text style={styles.partyDetail}>{vendor.address}</Text>}
              {vendor?.phone && <Text style={styles.partyDetail}>{vendor.phone}</Text>}
              {vendor?.vatin && (
                <Text style={styles.partyDetail}>
                  <Trans>VAT</Trans>: {vendor.vatin}
                </Text>
              )}
            </View>
          </View>

          {/* Items table */}
          <View style={styles.tableHeader}>
            <Text style={[styles.colNum, styles.tableHeaderText]}>#</Text>
            <Text style={[styles.colDesc, styles.tableHeaderText]}>
              <Trans>Description</Trans>
            </Text>
            <Text style={[styles.colQty, styles.tableHeaderText]}>
              <Trans>Qty</Trans>
            </Text>
            <Text style={[styles.colUnit, styles.tableHeaderText]}>
              <Trans>Unit</Trans>
            </Text>
            <Text style={[styles.colPrice, styles.tableHeaderText]}>
              <Trans>Unit Cost</Trans>
            </Text>
            <Text style={[styles.colTotal, styles.tableHeaderText]}>
              <Trans>Total</Trans>
            </Text>
          </View>

          {lineItems.map((item, idx) => {
            const qty = item.quantity ?? 1;
            const price = item.unitPrice ?? 0;
            return (
              <View
                key={item.id ?? idx}
                style={[styles.tableRow, ...(idx % 2 === 1 ? [styles.tableRowAlt] : [])]}
              >
                <Text style={styles.colNum}>{idx + 1}</Text>
                <View style={styles.colDesc}>
                  {item.sku && <Text style={styles.itemCode}>{item.sku}</Text>}
                  <Text>{item.description}</Text>
                </View>
                <Text style={styles.colQty}>{qty % 1 === 0 ? String(qty) : qty.toFixed(2)}</Text>
                <Text style={styles.colUnit}>{item.unit ?? ""}</Text>
                <Text style={styles.colPrice}>{fmt(price)}</Text>
                <Text style={styles.colTotal}>{fmt(qty * price)}</Text>
              </View>
            );
          })}

          {/* Totals */}
          <View style={styles.totals}>
            <View style={styles.totalRow}>
              <Text style={styles.grandTotalLabel}>
                <Trans>Total</Trans>
              </Text>
              <Text style={styles.grandTotalValue}>{fmt(subtotal)}</Text>
            </View>
          </View>

          {order.notes && (
            <View style={styles.noteCard}>
              <Text style={styles.noteCardTitle}>
                <Trans>Notes</Trans>
              </Text>
              <Text style={styles.noteCardText}>{order.notes}</Text>
            </View>
          )}

          <View style={styles.footer}>
            <Text style={styles.footerText}>
              <Trans>Please confirm receipt of this purchase order.</Trans>
            </Text>
          </View>

          {/* react-pdf's render prop runs outside React's normal reconciler
              (resolveDynamicNodes), so hooks-based i18n like <Trans> throws
              "Invalid hook call" here — plain text only, like the other PDFs
              that don't translate at all. */}
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

export default PurchaseOrderPDF;
