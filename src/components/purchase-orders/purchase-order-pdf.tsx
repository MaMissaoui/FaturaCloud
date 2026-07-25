import { Document, Page, Text, View, StyleSheet, Image } from "@react-pdf/renderer";
import { I18nProvider } from "@lingui/react";
import { Trans } from "@lingui/react/macro";
import dayjs from "dayjs";

const FONT = "Helvetica";
const FONT_BOLD = "Helvetica-Bold";

const styles = StyleSheet.create({
  page: { fontFamily: FONT, fontSize: 10, color: "#222", padding: 50, flexDirection: "column" },
  header: { flexDirection: "row", justifyContent: "space-between", alignItems: "flex-start", marginBottom: 32 },
  logo: { width: 100, height: 40, objectFit: "contain" },
  docTitle: { fontFamily: FONT_BOLD, fontSize: 22, textAlign: "right", color: "#1a1a1a" },
  docNumber: { fontSize: 11, textAlign: "right", color: "#555", marginTop: 4 },
  orgName: { fontFamily: FONT_BOLD, fontSize: 16 },
  parties: { flexDirection: "row", marginBottom: 28 },
  partyBlock: { flex: 1 },
  partyLabel: { fontFamily: FONT_BOLD, fontSize: 8, color: "#888", textTransform: "uppercase", marginBottom: 6, letterSpacing: 1 },
  partyName: { fontFamily: FONT_BOLD, fontSize: 11, marginBottom: 3 },
  partyDetail: { color: "#555", marginBottom: 2, lineHeight: 1.5 },
  meta: { flexDirection: "row", marginBottom: 24, gap: 40 },
  metaItem: { flexDirection: "column" },
  metaLabel: { fontFamily: FONT_BOLD, fontSize: 8, color: "#888", textTransform: "uppercase", letterSpacing: 1, marginBottom: 3 },
  metaValue: { fontSize: 10 },
  divider: { borderBottomWidth: 1, borderBottomColor: "#e0e0e0", marginBottom: 16 },
  tableHeader: { flexDirection: "row", backgroundColor: "#f5f5f5", paddingVertical: 7, paddingHorizontal: 8, marginBottom: 2 },
  tableRow: { flexDirection: "row", paddingVertical: 7, paddingHorizontal: 8, borderBottomWidth: 1, borderBottomColor: "#f0f0f0" },
  colNum: { width: 28, fontFamily: FONT_BOLD, color: "#888" },
  colDesc: { flex: 1 },
  colQty: { width: 60, textAlign: "right" },
  colUnit: { width: 50, textAlign: "right" },
  colPrice: { width: 80, textAlign: "right" },
  colTotal: { width: 80, textAlign: "right", fontFamily: FONT_BOLD },
  headerText: { fontFamily: FONT_BOLD, fontSize: 8, color: "#888", textTransform: "uppercase", letterSpacing: 0.5 },
  totals: { marginTop: 16, alignItems: "flex-end" },
  totalRow: { flexDirection: "row", paddingVertical: 4 },
  grandTotalLabel: { fontFamily: FONT_BOLD, fontSize: 12, width: 80, textAlign: "right", marginRight: 16 },
  grandTotalValue: { fontFamily: FONT_BOLD, fontSize: 12, width: 80, textAlign: "right" },
  itemCode: { fontSize: 8, color: "#888", marginBottom: 1 },
  notes: { marginTop: 24 },
  notesLabel: { fontFamily: FONT_BOLD, fontSize: 8, color: "#888", textTransform: "uppercase", letterSpacing: 1, marginBottom: 6 },
  notesText: { color: "#444", lineHeight: 1.6 },
  footer: { marginTop: 40, borderTopWidth: 1, borderTopColor: "#e0e0e0", paddingTop: 12 },
  footerText: { fontSize: 9, color: "#888", textAlign: "center" },
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
          <View style={styles.header}>
            <View>
              {logoSrc ? (
                <Image src={logoSrc} style={styles.logo} />
              ) : (
                <Text style={styles.orgName}>{organization?.name ?? ""}</Text>
              )}
            </View>
            <View>
              <Text style={styles.docTitle}>
                <Trans>PURCHASE ORDER</Trans>
              </Text>
              <Text style={styles.docNumber}>{order.orderNumber}</Text>
            </View>
          </View>

          {/* Buyer / Vendor */}
          <View style={styles.parties}>
            <View style={styles.partyBlock}>
              <Text style={styles.partyLabel}>
                <Trans>Ordered By</Trans>
              </Text>
              {logoSrc && <Text style={styles.partyName}>{organization?.name ?? ""}</Text>}
              {organization?.address && <Text style={styles.partyDetail}>{organization.address}</Text>}
              {organization?.email && <Text style={styles.partyDetail}>{organization.email}</Text>}
              {organization?.phone && <Text style={styles.partyDetail}>{organization.phone}</Text>}
              {organization?.vatin && (
                <Text style={styles.partyDetail}>
                  <Trans>VAT</Trans>: {organization.vatin}
                </Text>
              )}
            </View>
            <View style={[styles.partyBlock, { paddingLeft: 24 }]}>
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

          {/* Meta row */}
          <View style={styles.meta}>
            <View style={styles.metaItem}>
              <Text style={styles.metaLabel}>
                <Trans>Order Date</Trans>
              </Text>
              <Text style={styles.metaValue}>{dayjs(order.orderDate).format("L")}</Text>
            </View>
            {order.expectedDate && (
              <View style={styles.metaItem}>
                <Text style={styles.metaLabel}>
                  <Trans>Expected Date</Trans>
                </Text>
                <Text style={styles.metaValue}>{dayjs(order.expectedDate).format("L")}</Text>
              </View>
            )}
            {order.deliveryAddress && (
              <View style={styles.metaItem}>
                <Text style={styles.metaLabel}>
                  <Trans>Deliver To</Trans>
                </Text>
                <Text style={styles.metaValue}>{order.deliveryAddress}</Text>
              </View>
            )}
          </View>

          <View style={styles.divider} />

          {/* Items table */}
          <View style={styles.tableHeader}>
            <Text style={[styles.colNum, styles.headerText]}>#</Text>
            <Text style={[styles.colDesc, styles.headerText]}>
              <Trans>Description</Trans>
            </Text>
            <Text style={[styles.colQty, styles.headerText]}>
              <Trans>Qty</Trans>
            </Text>
            <Text style={[styles.colUnit, styles.headerText]}>
              <Trans>Unit</Trans>
            </Text>
            <Text style={[styles.colPrice, styles.headerText]}>
              <Trans>Unit Cost</Trans>
            </Text>
            <Text style={[styles.colTotal, styles.headerText]}>
              <Trans>Total</Trans>
            </Text>
          </View>

          {lineItems.map((item, idx) => {
            const qty = item.quantity ?? 1;
            const price = item.unitPrice ?? 0;
            return (
              <View key={item.id ?? idx} style={styles.tableRow}>
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
            <View style={styles.notes}>
              <Text style={styles.notesLabel}>
                <Trans>Notes</Trans>
              </Text>
              <Text style={styles.notesText}>{order.notes}</Text>
            </View>
          )}

          <View style={styles.footer}>
            <Text style={styles.footerText}>
              <Trans>Please confirm receipt of this purchase order.</Trans>
            </Text>
          </View>
        </Page>
      </Document>
    </I18nProvider>
  );
};

export default PurchaseOrderPDF;
