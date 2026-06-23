import { Document, Page, Text, View, StyleSheet, Image } from "@react-pdf/renderer";
import { I18nProvider } from "@lingui/react";
import { i18n } from "@lingui/core";
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
  colPrice: { width: 80, textAlign: "right" },
  colTotal: { width: 80, textAlign: "right", fontFamily: FONT_BOLD },
  headerText: { fontFamily: FONT_BOLD, fontSize: 8, color: "#888", textTransform: "uppercase", letterSpacing: 0.5 },
  totals: { marginTop: 16, alignItems: "flex-end" },
  totalRow: { flexDirection: "row", paddingVertical: 4 },
  totalLabel: { fontFamily: FONT_BOLD, fontSize: 10, width: 80, textAlign: "right", marginRight: 16 },
  totalValue: { fontSize: 10, width: 80, textAlign: "right" },
  grandTotalLabel: { fontFamily: FONT_BOLD, fontSize: 12, width: 80, textAlign: "right", marginRight: 16 },
  grandTotalValue: { fontFamily: FONT_BOLD, fontSize: 12, width: 80, textAlign: "right" },
  notes: { marginTop: 24 },
  notesLabel: { fontFamily: FONT_BOLD, fontSize: 8, color: "#888", textTransform: "uppercase", letterSpacing: 1, marginBottom: 6 },
  notesText: { color: "#444", lineHeight: 1.6 },
  footer: { marginTop: 40, borderTopWidth: 1, borderTopColor: "#e0e0e0", paddingTop: 12 },
  footerText: { fontSize: 9, color: "#888", textAlign: "center" },
});

interface Props {
  order: any;
  lineItems: any[];
  client: any;
  organization: any;
  locale?: string;
}

const OrderConfirmationPDF = ({ order, lineItems, client, organization, locale }: Props) => {
  const dateLocale = locale ?? "en";
  const currency = organization?.currency ?? "EUR";
  const logoSrc = organization?.logo ? `data:image/png;base64,${organization.logo}` : null;
  const fmt = (cents: number) =>
    Intl.NumberFormat(dateLocale, { style: "currency", currency }).format(cents / 100);

  const subtotal = lineItems.reduce((sum, item) => {
    const qty = item.quantity ?? 1;
    const price = item.unitPrice ?? 0;
    return sum + qty * price;
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
              <Text style={styles.docTitle}>ORDER CONFIRMATION</Text>
              <Text style={styles.docNumber}>{order.orderNumber}</Text>
            </View>
          </View>

          {/* From / To */}
          <View style={styles.parties}>
            <View style={styles.partyBlock}>
              <Text style={styles.partyLabel}>From</Text>
              {logoSrc && <Text style={styles.partyName}>{organization?.name ?? ""}</Text>}
              {organization?.address && <Text style={styles.partyDetail}>{organization.address}</Text>}
              {organization?.email && <Text style={styles.partyDetail}>{organization.email}</Text>}
              {organization?.phone && <Text style={styles.partyDetail}>{organization.phone}</Text>}
              {organization?.vatin && <Text style={styles.partyDetail}>VAT: {organization.vatin}</Text>}
            </View>
            <View style={[styles.partyBlock, { paddingLeft: 24 }]}>
              <Text style={styles.partyLabel}>Bill To</Text>
              <Text style={styles.partyName}>{client?.name ?? ""}</Text>
              {client?.address && <Text style={styles.partyDetail}>{client.address}</Text>}
              {client?.email && <Text style={styles.partyDetail}>{client.email}</Text>}
              {client?.vatin && <Text style={styles.partyDetail}>VAT: {client.vatin}</Text>}
            </View>
          </View>

          {/* Meta row */}
          <View style={styles.meta}>
            <View style={styles.metaItem}>
              <Text style={styles.metaLabel}>Order Date</Text>
              <Text style={styles.metaValue}>{dayjs(order.orderDate).format("L")}</Text>
            </View>
            {order.deliveryDate && (
              <View style={styles.metaItem}>
                <Text style={styles.metaLabel}>Expected Delivery</Text>
                <Text style={styles.metaValue}>{dayjs(order.deliveryDate).format("L")}</Text>
              </View>
            )}
            {order.shippingAddress && (
              <View style={styles.metaItem}>
                <Text style={styles.metaLabel}>Ship To</Text>
                <Text style={styles.metaValue}>{order.shippingAddress}</Text>
              </View>
            )}
          </View>

          <View style={styles.divider} />

          {/* Items table */}
          <View style={styles.tableHeader}>
            <Text style={[styles.colNum, styles.headerText]}>#</Text>
            <Text style={[styles.colDesc, styles.headerText]}>Description</Text>
            <Text style={[styles.colQty, styles.headerText]}>Qty</Text>
            <Text style={[styles.colPrice, styles.headerText]}>Unit Price</Text>
            <Text style={[styles.colTotal, styles.headerText]}>Total</Text>
          </View>

          {lineItems.map((item, idx) => {
            const qty = item.quantity ?? 1;
            const price = item.unitPrice ?? 0;
            return (
              <View key={item.id ?? idx} style={styles.tableRow}>
                <Text style={styles.colNum}>{idx + 1}</Text>
                <Text style={styles.colDesc}>{item.description}</Text>
                <Text style={styles.colQty}>{qty % 1 === 0 ? String(qty) : qty.toFixed(2)}</Text>
                <Text style={styles.colPrice}>{fmt(price)}</Text>
                <Text style={styles.colTotal}>{fmt(qty * price)}</Text>
              </View>
            );
          })}

          {/* Totals */}
          <View style={styles.totals}>
            <View style={styles.totalRow}>
              <Text style={styles.grandTotalLabel}>Total</Text>
              <Text style={styles.grandTotalValue}>{fmt(subtotal)}</Text>
            </View>
          </View>

          {order.notes && (
            <View style={styles.notes}>
              <Text style={styles.notesLabel}>Notes</Text>
              <Text style={styles.notesText}>{order.notes}</Text>
            </View>
          )}

          <View style={styles.footer}>
            <Text style={styles.footerText}>
              Thank you for your order. This document confirms your order as listed above.
            </Text>
          </View>
        </Page>
      </Document>
    </I18nProvider>
  );
};

export default OrderConfirmationPDF;
