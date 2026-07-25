import { Document, Page, Text, View, StyleSheet, Image } from "@react-pdf/renderer";
import { I18nProvider } from "@lingui/react";
import { i18n } from "@lingui/core";
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
  docRef: { fontSize: 9, textAlign: "right", color: "rgba(255,255,255,0.65)", marginTop: 4 },
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
  colQty: { width: 80, textAlign: "right" },
  colUnit: { width: 60, textAlign: "right" },
  itemCode: { fontSize: 8, color: "#888", marginBottom: 1 },

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

  signatureArea: { flexDirection: "row", justifyContent: "space-between", marginTop: 48, paddingTop: 24, borderTopWidth: 1, borderTopColor: "#e0e0e0" },
  signatureBlock: { width: "40%" },
  signatureLine: { borderBottomWidth: 1, borderBottomColor: "#999", marginBottom: 6, height: 24 },
  signatureLabel: { fontFamily: FONT_BOLD, fontSize: 8, color: "#888", textTransform: "uppercase", letterSpacing: 0.5 },
  pageNumber: { position: "absolute", bottom: 24, right: 50, fontSize: 8, color: "#aaa" },
});

interface Props {
  delivery: any;
  lineItems: any[];
  client: any;
  organization: any;
  locale?: string;
}

const DeliveryNotePDF = ({ delivery, lineItems, client, organization, locale: _locale }: Props) => {
  // organizationAtom already resolves logo to a ready-to-use data URI
  // (fetched from GET /organizations/{id}/logo) — don't re-wrap it.
  const logoSrc = organization?.logo ?? null;

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
              <Text style={styles.docTitle}>DELIVERY NOTE</Text>
              <View style={styles.docNumberPill}>
                <Text style={styles.docNumberText}>{delivery.deliveryNumber}</Text>
              </View>
              {delivery.orderNumber && <Text style={styles.docRef}>Order: {delivery.orderNumber}</Text>}
              <View style={styles.headerMeta}>
                <View style={styles.headerMetaRow}>
                  <Text style={styles.headerMetaLabel}>Delivery Date:</Text>
                  <Text style={styles.headerMetaValue}>{dayjs(delivery.deliveryDate).format("L")}</Text>
                </View>
                {delivery.trackingNumber && (
                  <View style={styles.headerMetaRow}>
                    <Text style={styles.headerMetaLabel}>Tracking:</Text>
                    <Text style={styles.headerMetaValue}>{delivery.trackingNumber}</Text>
                  </View>
                )}
                <View style={styles.headerMetaRow}>
                  <Text style={styles.headerMetaLabel}>Status:</Text>
                  <Text style={[styles.headerMetaValue, { textTransform: "capitalize" }]}>
                    {delivery.status}
                  </Text>
                </View>
              </View>
            </View>
          </View>

          {/* From / To */}
          <View style={styles.parties}>
            <View style={styles.card}>
              <Text style={styles.partyLabel}>From</Text>
              <Text style={styles.partyName}>{organization?.name ?? ""}</Text>
              {organization?.address && <Text style={styles.partyDetail}>{organization.address}</Text>}
              {organization?.email && <Text style={styles.partyDetail}>{organization.email}</Text>}
              {organization?.phone && <Text style={styles.partyDetail}>{organization.phone}</Text>}
              {organization?.vatin && <Text style={styles.partyDetail}>VAT: {organization.vatin}</Text>}
            </View>
            <View style={styles.card}>
              <Text style={styles.partyLabel}>Deliver To</Text>
              <Text style={styles.partyName}>{client?.name ?? ""}</Text>
              {client?.address && <Text style={styles.partyDetail}>{client.address}</Text>}
              {delivery.shippingAddress && delivery.shippingAddress !== client?.address && (
                <Text style={[styles.partyDetail, { marginTop: 6 }]}>
                  Ship to: {delivery.shippingAddress}
                </Text>
              )}
              {client?.email && <Text style={styles.partyDetail}>{client.email}</Text>}
            </View>
          </View>

          {/* Items table — no prices on delivery notes */}
          <View style={styles.tableHeader}>
            <Text style={[styles.colNum, styles.tableHeaderText]}>#</Text>
            <Text style={[styles.colDesc, styles.tableHeaderText]}>Description</Text>
            <Text style={[styles.colQty, styles.tableHeaderText]}>Qty</Text>
            <Text style={[styles.colUnit, styles.tableHeaderText]}>Unit</Text>
          </View>

          {lineItems.map((item, idx) => {
            const qty = item.quantity ?? 1;
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
              </View>
            );
          })}

          {delivery.notes && (
            <View style={styles.noteCard}>
              <Text style={styles.noteCardTitle}>Notes</Text>
              <Text style={styles.noteCardText}>{delivery.notes}</Text>
            </View>
          )}

          <View style={styles.signatureArea}>
            <View style={styles.signatureBlock}>
              <View style={styles.signatureLine} />
              <Text style={styles.signatureLabel}>Received by / Date</Text>
            </View>
            <View style={styles.signatureBlock}>
              <View style={styles.signatureLine} />
              <Text style={styles.signatureLabel}>Authorized signature</Text>
            </View>
          </View>

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

export default DeliveryNotePDF;
