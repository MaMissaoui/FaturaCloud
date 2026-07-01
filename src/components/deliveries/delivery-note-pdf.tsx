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
  docRef: { fontSize: 9, textAlign: "right", color: "#888", marginTop: 2 },
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
  colQty: { width: 80, textAlign: "right" },
  colUnit: { width: 60, textAlign: "right" },
  headerText: { fontFamily: FONT_BOLD, fontSize: 8, color: "#888", textTransform: "uppercase", letterSpacing: 0.5 },
  itemCode: { fontSize: 8, color: "#888", marginBottom: 1 },
  notes: { marginTop: 24 },
  notesLabel: { fontFamily: FONT_BOLD, fontSize: 8, color: "#888", textTransform: "uppercase", letterSpacing: 1, marginBottom: 6 },
  notesText: { color: "#444", lineHeight: 1.6 },
  signatureArea: { flexDirection: "row", justifyContent: "space-between", marginTop: 48, paddingTop: 24, borderTopWidth: 1, borderTopColor: "#e0e0e0" },
  signatureBlock: { width: "40%" },
  signatureLine: { borderBottomWidth: 1, borderBottomColor: "#999", marginBottom: 6, height: 24 },
  signatureLabel: { fontFamily: FONT_BOLD, fontSize: 8, color: "#888", textTransform: "uppercase", letterSpacing: 0.5 },
});

interface Props {
  delivery: any;
  lineItems: any[];
  client: any;
  organization: any;
  locale?: string;
}

const DeliveryNotePDF = ({ delivery, lineItems, client, organization, locale: _locale }: Props) => {
  const logoSrc = organization?.logo ? `data:image/png;base64,${organization.logo}` : null;

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
              <Text style={styles.docTitle}>DELIVERY NOTE</Text>
              <Text style={styles.docNumber}>{delivery.deliveryNumber}</Text>
              {delivery.orderNumber && (
                <Text style={styles.docRef}>Order: {delivery.orderNumber}</Text>
              )}
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

          {/* Meta row */}
          <View style={styles.meta}>
            <View style={styles.metaItem}>
              <Text style={styles.metaLabel}>Delivery Date</Text>
              <Text style={styles.metaValue}>{dayjs(delivery.deliveryDate).format("L")}</Text>
            </View>
            {delivery.trackingNumber && (
              <View style={styles.metaItem}>
                <Text style={styles.metaLabel}>Tracking</Text>
                <Text style={styles.metaValue}>{delivery.trackingNumber}</Text>
              </View>
            )}
            <View style={styles.metaItem}>
              <Text style={styles.metaLabel}>Status</Text>
              <Text style={[styles.metaValue, { textTransform: "capitalize" }]}>{delivery.status}</Text>
            </View>
          </View>

          <View style={styles.divider} />

          {/* Items table — no prices on delivery notes */}
          <View style={styles.tableHeader}>
            <Text style={[styles.colNum, styles.headerText]}>#</Text>
            <Text style={[styles.colDesc, styles.headerText]}>Description</Text>
            <Text style={[styles.colQty, styles.headerText]}>Qty</Text>
            <Text style={[styles.colUnit, styles.headerText]}>Unit</Text>
          </View>

          {lineItems.map((item, idx) => {
            const qty = item.quantity ?? 1;
            return (
              <View key={item.id ?? idx} style={styles.tableRow}>
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
            <View style={styles.notes}>
              <Text style={styles.notesLabel}>Notes</Text>
              <Text style={styles.notesText}>{delivery.notes}</Text>
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
        </Page>
      </Document>
    </I18nProvider>
  );
};

export default DeliveryNotePDF;
