import { useEffect, useState, useMemo, useRef } from "react";
import { createPortal } from "react-dom";
import { useLocation, useNavigate, useParams } from "react-router";
import {
  App,
  Button,
  Card,
  DatePicker,
  Divider,
  Form,
  Input,
  InputNumber,
  Row,
  Col,
  Select,
  Space,
  Descriptions,
  Layout,
  Popconfirm,
  theme,
  Spin,
  Tooltip,
  Typography,
} from "antd";
import { useAtom, useAtomValue, useSetAtom } from "jotai";
import { loadable } from "src/utils/loadable";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import {
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  EyeOutlined,
  FileExcelOutlined,
  FilePdfOutlined,
  FileTextOutlined,
  ScheduleOutlined,
  SaveOutlined,
  UserAddOutlined,
} from "@ant-design/icons";
import LineItemsTable from "src/components/line-items/table";
import { SaveFile, DownloadInvoiceEInvoice, ExportInvoiceDocument } from "src/api";
import QRCode from "qrcode";
import { pdf } from "@react-pdf/renderer";
import { Document, Page } from "react-pdf";
import dayjs from "dayjs";

// Import CSS for react-pdf (v10 dropped the esm/ path segment)
import "react-pdf/dist/Page/AnnotationLayer.css";
import "react-pdf/dist/Page/TextLayer.css";

// Configure PDF.js worker
import { pdfjs } from "react-pdf";
pdfjs.GlobalWorkerOptions.workerSrc = new URL(
  "pdfjs-dist/build/pdf.worker.min.mjs",
  import.meta.url,
).toString();

import get from "lodash/get";
import includes from "lodash/includes";
import isString from "lodash/isString";
import lowerCase from "lodash/lowerCase";
import find from "lodash/find";
import filter from "lodash/filter";
import map from "lodash/map";
import sum from "lodash/sum";
import isNumber from "lodash/isNumber";
import toNumber from "lodash/toNumber";

import { clientsAtom, setClientsAtom } from "src/atoms/client";
import { productsAtom, setProductsAtom } from "src/atoms/product";
import { useDatePickerFormat } from "src/utils/date";
import {
  invoiceIdAtom,
  invoiceAtom,
  deleteInvoiceAtom,
  duplicateInvoiceAtom,
  updateInvoiceStateAtom,
} from "src/atoms/invoice";
import { organizationAtom, nextInvoiceNumberAtom } from "src/atoms/organization";
import { taxRatesAtom, setTaxRatesAtom } from "src/atoms/tax-rate";
import { paymentTermsAtom, setPaymentTermsAtom } from "src/atoms/payment-term";
import { siderAtom } from "src/atoms/generic";
import ClientForm from "src/components/clients/form.tsx";
import { formatAddressOneLine } from "src/utils/address";
import { getInvoicePDFLayout, isCustomTemplateLayout } from "src/components/invoices/layouts";
import { CurrencySelect, showExchangeRateFields } from "src/components/currency/currency-fields";
import PaymentPanel from "src/components/payments/payment-panel";
import { buildSepaCreditTransferPayload } from "src/utils/sepa-qr";
import { generateInvoiceNumber } from "src/utils/invoice";
import { requiredForNewLineItem } from "src/utils/line-items";
import {
  multiplyDecimal,
  divideDecimal,
  calculateTax,
  addDecimal,
  subtractDecimal,
  centsToUnits,
  unitsToCents,
} from "src/utils/currency";

const { TextArea } = Input;
const { Option } = Select;
const { Footer } = Layout;

// invoiceAtom is async; reading it with plain useAtom/useAtomValue makes it
// throw to the nearest Suspense boundary whenever invoiceIdAtom changes after
// mount. That boundary is the single top-level one wrapping all routes
// (src/app.tsx), so React tears down and remounts this whole route on every
// invoice fetch — the effect below's cleanup resets invoiceIdAtom to null on
// unmount, the remount sets it back, and the cycle repeats forever. loadable()
// resolves synchronously instead of suspending, same fix as
// src/components/tax-rates/form.tsx.
const loadableInvoiceAtom = loadable(invoiceAtom);

// PDF Preview component that generates blob manually (like PDF download)
const PDFPreview: React.FC<{ createPDFDocument: () => React.ReactElement<any> | null }> = ({
  createPDFDocument,
}) => {
  const [pdfUrl, setPdfUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [containerWidth, setContainerWidth] = useState(0);
  const siderCollapsed = useAtomValue(siderAtom);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const pdfUrlRef = useRef<string | null>(null);

  // Measures the container once it's mounted and on every window resize —
  // a stable effect bound to the ref, replacing a callback-ref pattern that
  // stored its cleanup on the callback function itself (which never
  // receives node-unmount calls), leaking the resize listener on every
  // mount/unmount.
  useEffect(() => {
    const node = containerRef.current;
    if (!node) return;

    const measureWidth = () => setContainerWidth(node.offsetWidth - 40); // subtract some padding
    measureWidth();
    window.addEventListener("resize", measureWidth);
    return () => window.removeEventListener("resize", measureWidth);
  }, []);

  // Re-measure when the sidebar collapses/expands — its own resize event
  // fires before the layout has actually settled, so this waits a beat and
  // re-measures off the container's parent width directly.
  useEffect(() => {
    const timer = setTimeout(() => {
      window.dispatchEvent(new Event("resize"));

      setTimeout(() => {
        const container = containerRef.current;
        if (container && container.parentElement) {
          setContainerWidth(container.parentElement.offsetWidth - 40);
        }
      }, 100);
    }, 500);

    return () => clearTimeout(timer);
  }, [siderCollapsed]);

  useEffect(() => {
    let cancelled = false;

    const generatePDF = async () => {
      setLoading(true);
      setError(null);

      try {
        const document = createPDFDocument();
        if (!document) {
          if (!cancelled) {
            setError(t`Please select a client to view PDF preview.`);
            setLoading(false);
          }
          return;
        }

        const blob = await pdf(document).toBlob();
        if (cancelled) return;

        // Revoke the previous URL (if any) before replacing it — the old
        // effect's cleanup closed over the initial pdfUrl === null, so its
        // `if (pdfUrl)` check was always false and no URL was ever revoked.
        if (pdfUrlRef.current) {
          URL.revokeObjectURL(pdfUrlRef.current);
        }
        const url = URL.createObjectURL(blob);
        pdfUrlRef.current = url;
        setPdfUrl(url);
      } catch (err) {
        if (!cancelled) {
          console.error("PDF generation error:", err);
          setError(t`Error generating PDF preview. Please try again.`);
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    generatePDF();

    return () => {
      cancelled = true;
      if (pdfUrlRef.current) {
        URL.revokeObjectURL(pdfUrlRef.current);
        pdfUrlRef.current = null;
      }
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps -- Intentionally omitting createPDFDocument to prevent re-generation on width changes

  if (loading) {
    return (
      <div style={{ textAlign: "center", padding: "50px" }}>
        <Spin size="large" />
      </div>
    );
  }

  if (error) {
    return <div style={{ textAlign: "center", padding: "50px", color: "red" }}>{error}</div>;
  }

  return (
    <div ref={containerRef} data-pdf-container style={{ width: "100%" }}>
      <Document file={pdfUrl}>
        <Page
          pageNumber={1}
          renderTextLayer={false}
          renderAnnotationLayer={false}
          width={containerWidth > 0 ? containerWidth : undefined}
        />
      </Document>
    </div>
  );
};

const InvoiceDetails: React.FC = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const { id } = useParams<string>();
  const { i18n } = useLingui();
  const {
    token: { colorBgContainer },
  } = theme.useToken();
  const { message } = App.useApp();
  const organization = useAtomValue(organizationAtom);
  const [invoiceId, setInvoiceId] = useAtom(invoiceIdAtom);
  const invoiceLoadable = useAtomValue(loadableInvoiceAtom);
  const setInvoice = useSetAtom(invoiceAtom);
  const invoice = invoiceLoadable.state === "hasData" ? invoiceLoadable.data : undefined;
  const clients = useAtomValue(clientsAtom);
  const setClients = useSetAtom(setClientsAtom);
  const products = useAtomValue(productsAtom);
  const setProducts = useSetAtom(setProductsAtom);
  // A component/intermediate isn't sellable — exclude it from the picker.
  // Unclassified products (category null) stay eligible everywhere.
  const sellableProducts = products.filter((p: any) => p.category !== "component");
  const taxRates = useAtomValue(taxRatesAtom);
  const setTaxRates = useSetAtom(setTaxRatesAtom);
  const paymentTerms = useAtomValue(paymentTermsAtom);
  const setPaymentTerms = useSetAtom(setPaymentTermsAtom);
  const deleteInvoice = useSetAtom(deleteInvoiceAtom);
  const duplicateInvoice = useSetAtom(duplicateInvoiceAtom);
  const updateInvoiceState = useSetAtom(updateInvoiceStateAtom);
  const nextInvoiceNumber = useAtomValue(nextInvoiceNumberAtom);
  const [previewMode, setPreviewMode] = useState(false);
  const [downloadingEInvoice, setDownloadingEInvoice] = useState(false);
  const [downloadingPdf, setDownloadingPdf] = useState(false);
  const [downloadingExcel, setDownloadingExcel] = useState(false);
  // The server-side export (Excel always, PDF only for a custom template)
  // reads persisted rows by invoice id, unlike the default/tunisia PDF
  // button which reads live unsaved form.getFieldsValue() — so a stale
  // export must be prevented instead of silently exporting the last-saved
  // version. Reset per mount (this whole route remounts on invoice id
  // change, see loadableInvoiceAtom's comment), set on any edit, cleared on
  // a successful save.
  const [isDirty, setIsDirty] = useState(false);
  const dateFormat = useDatePickerFormat();

  const isNew = id === "new";

  useEffect(() => {
    setClients();
    setProducts();
    setTaxRates();
    setPaymentTerms();
    if (!isNew) {
      setInvoiceId(id || null);
    }

    // Clean up
    return () => {
      setInvoiceId(null);
    };
  }, [id, isNew, setClients, setProducts, setInvoiceId, setTaxRates, setPaymentTerms]);

  // Navigate to the new invoice after successful creation
  useEffect(() => {
    if (isNew && invoiceId) {
      navigate(`/invoices/${invoiceId}`);
    }
  }, [isNew, invoiceId, navigate]);

  const getInitialValues = (): Record<string, unknown> => {
    // Antd form values are a heterogeneous bag (Dayjs dates, nested line
    // items, strings) that differs between the new-invoice and edit branches,
    // so a loose record is the honest type here rather than a contrived union.
    let values: Record<string, unknown> = {
      currency: organization?.currency ?? "EUR",
      date: dayjs(),
      dueDate: organization?.due_days ? dayjs().add(organization.due_days, "day") : null,
      lineItems: [{ quantity: 1, taxRate: get(find(taxRates, { isDefault: 1 }), "id") }],
      customerNotes: organization?.customerNotes,
      paymentTerms: get(find(paymentTerms, { isDefault: 1 }), "name"),
      overdueCharge: organization?.overdueCharge || 0,
      number: isNew ? nextInvoiceNumber || "" : undefined,
      fiscalStampAmount: organization?.defaultFiscalStampAmount
        ? centsToUnits(organization.defaultFiscalStampAmount)
        : 0,
    };

    if (!isNew && invoice) {
      values = {
        ...invoice,
        lineItems: map(invoice.lineItems, (item) => ({
          ...item,
          total: multiplyDecimal(item.quantity, item.unitPrice),
        })),
      };
    }
    return values;
  };

  const initialValues = getInitialValues();
  const [form] = Form.useForm();

  // Reset form when invoice data changes (e.g., after duplication)
  useEffect(() => {
    if (!isNew && invoice) {
      const newValues = {
        ...invoice,
        lineItems: map(invoice.lineItems, (item) => ({
          ...item,
          total: multiplyDecimal(item.quantity, item.unitPrice),
        })),
      };
      form.resetFields();
      form.setFieldsValue(newValues);
    }
  }, [invoice, isNew, form]);

  const handleSubmit = async (values: any) => {
    await setInvoice({
      ...values,
      subTotal,
      taxTotal,
      total,
      fiscalStampAmount,
      withholdingTaxAmount: withholdingTaxRate ? withholdingTaxAmount : null,
      overdueCharge: values.overdueCharge,
    });
    setIsDirty(false);
  };

  const handleDelete = (id: string) => async () => {
    const success = await deleteInvoice(id);
    if (success) navigate("/invoices");
  };

  const handleDuplicate = (id: string) => async () => {
    const newInvoiceId = await duplicateInvoice(id);
    if (newInvoiceId) {
      navigate(`/invoices/${newInvoiceId}`);
    }
  };

  const lineItems = Form.useWatch("lineItems", form);

  const subTotal = sum(
    map(
      filter(lineItems, (item) => isNumber(get(item, "total"))),
      "total",
    ),
  );
  // Group line items by tax rate and calculate tax for each group
  const taxGroups = useMemo(() => {
    const groups: { [key: string]: { taxRate: any; items: any[]; subtotal: number; tax: number } } =
      {};

    if (lineItems && Array.isArray(lineItems)) {
      lineItems.forEach((item: any) => {
        if (isNumber(get(item, "total")) && get(item, "taxRate")) {
          const taxRateId = get(item, "taxRate");
          const taxRate = find(taxRates, { id: taxRateId });

          if (!groups[taxRateId]) {
            groups[taxRateId] = {
              taxRate,
              items: [],
              subtotal: 0,
              tax: 0,
            };
          }

          groups[taxRateId].items.push(item);
          groups[taxRateId].subtotal = addDecimal(groups[taxRateId].subtotal, item.total);
          groups[taxRateId].tax = taxRate?.percentage
            ? calculateTax(groups[taxRateId].subtotal, taxRate.percentage)
            : 0;
        }
      });
    }

    return Object.values(groups);
  }, [lineItems, taxRates]);

  const taxTotal = sum(map(taxGroups, "tax"));
  // Tunisia invoice support. fiscalStampAmount (timbre fiscal) is a flat,
  // non-taxable charge that changes what's owed, so it's additive into
  // total here exactly as db/invoice_totals.go's validateInvoiceTotals
  // expects server-side — the two must always agree, or every save 409s.
  // withholdingTaxAmount (retenue à la source) is the opposite: it never
  // touches total, only how much of it the client actually wires versus
  // settles with a tax certificate (see the "Net amount due" row below).
  const fiscalStampAmount = toNumber(Form.useWatch("fiscalStampAmount", form)) || 0;
  const withholdingTaxRate = Form.useWatch("withholdingTaxRate", form);
  const total = addDecimal(addDecimal(subTotal, taxTotal), fiscalStampAmount);
  // Derived from TTC (subTotal + taxTotal) excluding the stamp — a
  // withholding percentage is a tax-code rate on the taxable transaction
  // value, not on the state's own stamp duty.
  const withholdingTaxAmount = withholdingTaxRate
    ? calculateTax(addDecimal(subTotal, taxTotal), withholdingTaxRate)
    : 0;
  const netAmountDue = subtractDecimal(total, withholdingTaxAmount);

  // SEPA credit transfer QR ("GiroCode") for the PDF's payment box. Only
  // renders for EUR invoices on an organization with an IBAN — SEPA credit
  // transfers don't exist for other currencies. Regenerated whenever the
  // total, currency, invoice number, or organization's bank details change.
  const watchedCurrency = Form.useWatch("currency", form);
  const watchedClientId = Form.useWatch("clientId", form);
  const selectedClient = find(clients, { id: watchedClientId });
  const selectedClientAddress = selectedClient ? formatAddressOneLine(selectedClient) : "";
  const orgCurrency = organization?.currency ?? "EUR";
  const watchedNumber = Form.useWatch("number", form);
  const [qrCodeDataUri, setQrCodeDataUri] = useState<string | null>(null);
  useEffect(() => {
    const payload = buildSepaCreditTransferPayload({
      beneficiaryName: organization?.name,
      iban: organization?.iban,
      bic: organization?.bic,
      currency: watchedCurrency ?? organization?.currency ?? "EUR",
      amount: total,
      reference: watchedNumber ?? "",
    });
    if (!payload) {
      setQrCodeDataUri(null);
      return;
    }
    let cancelled = false;
    QRCode.toDataURL(payload, { margin: 1, width: 200 })
      .then((uri) => {
        if (!cancelled) setQrCodeDataUri(uri);
      })
      .catch(() => {
        if (!cancelled) setQrCodeDataUri(null);
      });
    return () => {
      cancelled = true;
    };
  }, [
    organization?.name,
    organization?.iban,
    organization?.bic,
    organization?.currency,
    watchedCurrency,
    total,
    watchedNumber,
  ]);

  // Helper function to create PDF document with current form data
  const createPDFDocument = () => {
    // Get current form values to include unsaved changes
    const formValues = form.getFieldsValue();
    const clientId = formValues.clientId;
    const clientData = find(clients, { id: clientId });

    // Return null if no client data found
    if (!clientData) {
      return null;
    }

    // Create merged invoice data with form values and computed totals
    const invoiceForPDF = {
      ...invoice, // Start with database data
      ...formValues, // Override with current form values
      // Use computed totals from the current component state
      subTotal,
      taxTotal,
      total,
      fiscalStampAmount,
      withholdingTaxAmount: withholdingTaxRate ? withholdingTaxAmount : null,
      // Ensure line items have the correct totals
      lineItems: formValues.lineItems || [],
    };

    const Layout = getInvoicePDFLayout(organization?.invoiceLayout);

    return (
      <Layout
        invoice={invoiceForPDF}
        client={clientData}
        organization={organization}
        taxRates={taxRates}
        i18n={i18n}
        qrCodeDataUri={qrCodeDataUri}
      />
    );
  };

  const isCustomTemplate = isCustomTemplateLayout(organization?.invoiceLayout);

  // The server fill-and-convert path (db/xlsx_export.go / db/pdf_convert.go)
  // — used for every Excel export regardless of layout, and for PDF only
  // when there's no client-side React component for "custom" (see
  // getInvoicePDFLayout). Reads persisted rows by invoice id, so it's gated
  // on isDirty below the same way custom-template export always was.
  const handleServerExport = (format: "pdf" | "xlsx") => async () => {
    if (!id) return;
    const setDownloading = format === "xlsx" ? setDownloadingExcel : setDownloadingPdf;
    setDownloading(true);
    try {
      await ExportInvoiceDocument(id, format);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Export failed`);
    } finally {
      setDownloading(false);
    }
  };

  // The PDF button always does the same thing regardless of layout from the
  // user's point of view; only the mechanism differs — client-side
  // react-pdf (reads live unsaved form values, no isDirty gating needed) for
  // every layout with a real component, or the server path above for
  // "custom", which has none.
  const handlePdfDownload = async () => {
    if (isCustomTemplate) {
      await handleServerExport("pdf")();
      return;
    }
    setDownloadingPdf(true);
    try {
      const document = createPDFDocument();
      if (!document) return;
      const blob = await pdf(document).toBlob();
      await SaveFile(`invoice-${id}.pdf`, blob);
    } finally {
      setDownloadingPdf(false);
    }
  };

  const currentInvoiceState =
    !isNew && invoice && typeof invoice === "object" && !("then" in invoice)
      ? ((invoice as any).state ?? "draft")
      : "draft";

  // Footer action groups — built as arrays and filtered before being handed
  // to <Space split>, so a group that's conditionally empty (e.g. every
  // button here needs !isNew) never becomes a stray leading/doubled
  // separator the way a raw conditional child of <Space split> would.
  const viewActions = !isNew
    ? [
        <Button key="view" type="dashed" onClick={() => setPreviewMode(!previewMode)}>
          {previewMode ? (
            <>
              <EditOutlined /> <Trans>Edit</Trans>
            </>
          ) : (
            <>
              <EyeOutlined /> <Trans>View</Trans>
            </>
          )}
        </Button>,
      ]
    : [];

  const exportActions = !isNew
    ? [
        <Tooltip
          key="pdf"
          title={isCustomTemplate && isDirty ? t`Save your changes before exporting` : undefined}
        >
          <Button
            disabled={isCustomTemplate && isDirty}
            loading={downloadingPdf}
            onClick={handlePdfDownload}
          >
            <FilePdfOutlined /> PDF
          </Button>
        </Tooltip>,
        // Always the server fill-and-convert path (db/xlsx_export.go) regardless
        // of layout — even a "default"/"tunisia" invoice has an embedded
        // fallback template (resolveTemplateBytes) to fill.
        <Tooltip key="excel" title={isDirty ? t`Save your changes before exporting` : undefined}>
          <Button
            disabled={isDirty}
            loading={downloadingExcel}
            onClick={handleServerExport("xlsx")}
          >
            <FileExcelOutlined /> <Trans>Excel</Trans>
          </Button>
        </Tooltip>,
        // Deliberately NOT isDirty-gated like PDF/Excel above: db/einvoice.go
        // 409s with a list of every missing mandatory field at once, and
        // that's the retry workflow this button exists for — a user fixing
        // a field the 409 just named needs to be able to retry immediately,
        // not be forced to Save first. It reads persisted data too, but that
        // hazard (a stale-looking export) doesn't apply the same way to a
        // validation-error response.
        <Button
          key="einvoice"
          loading={downloadingEInvoice}
          onClick={async () => {
            setDownloadingEInvoice(true);
            try {
              await DownloadInvoiceEInvoice(id!);
            } catch (error) {
              message.error(error instanceof Error ? error.message : t`E-invoice export failed`);
            } finally {
              setDownloadingEInvoice(false);
            }
          }}
        >
          <FileTextOutlined /> <Trans>E-Invoice (XML)</Trans>
        </Button>,
      ]
    : [];

  const stateActions = [
    ...(!isNew && currentInvoiceState !== "cancelled"
      ? [
          <Popconfirm
            key="cancel"
            title={t`Cancel this invoice?`}
            onConfirm={async () => {
              await updateInvoiceState({ invoiceId: id!, state: "cancelled" });
              setInvoiceId(null);
              setTimeout(() => setInvoiceId(id ?? null), 0);
            }}
            okText={t`Yes`}
            cancelText={t`No`}
          >
            <Button type="dashed" danger>
              <Trans>Cancel invoice</Trans>
            </Button>
          </Popconfirm>,
        ]
      : []),
    <Button key="save" type="primary" onClick={() => form.submit()}>
      <SaveOutlined /> <Trans>Save</Trans>
    </Button>,
  ];

  const footerActionGroups = [viewActions, exportActions, stateActions].filter(
    (group) => group.length > 0,
  );

  if (!organization) return null;
  if (!isNew && !invoice) return null;

  return (
    <>
      <Row>
        <Col span={24}>
          <Form
            form={form}
            onFinish={handleSubmit}
            onValuesChange={() => setIsDirty(true)}
            layout="vertical"
            initialValues={initialValues}
            style={{ display: previewMode ? "none" : "block" }}
          >
            <Card size="small" title={<Trans>Invoice details</Trans>} style={{ marginBottom: 24 }}>
              <Row gutter={24}>
                {/* Left: the two fields that need real room — a searchable
                    dropdown and free text — stacked so the note fills the
                    height the right column's many short fields don't need,
                    instead of getting its own near-empty full-width row. */}
                <Col xs={24} xl={12}>
                  <Form.Item
                    label={t`Select or create a client`}
                    name="clientId"
                    rules={[{ required: true, message: t`This field is required!` }]}
                  >
                    <Select
                      showSearch
                      optionFilterProp="children"
                      filterOption={(input, option) => {
                        const clientName = get(option, ["props", "children"]);
                        if (isString(clientName)) {
                          return includes(lowerCase(clientName), lowerCase(input));
                        }
                        return true;
                      }}
                      onChange={(clientId) => {
                        if (isNew) {
                          const selectedClient = clients.find((c: any) => c.id === clientId);

                          if (organization?.invoiceNumberFormat?.includes("{clientCode}")) {
                            const clientCode = selectedClient?.code || "";

                            // Regenerate invoice number with client code
                            const counter = addDecimal(organization.invoiceNumberCounter || 0, 1);
                            const newNumber = organization.invoiceNumberFormat
                              ? generateInvoiceNumber(
                                  organization.invoiceNumberFormat,
                                  counter,
                                  new Date(),
                                  clientCode,
                                )
                              : "";
                            form.setFieldsValue({ number: newNumber });
                          }

                          // Prefill the buyer reference (e.g. a Leitweg-ID) from the
                          // client's default, but don't clobber a manual edit.
                          if (!form.getFieldValue("buyerReference")) {
                            form.setFieldsValue({
                              buyerReference: selectedClient?.default_buyer_reference || undefined,
                            });
                          }
                        }
                      }}
                      popupRender={(menu) => (
                        <>
                          {menu}
                          <Divider style={{ margin: "8px 0" }} />
                          <Button
                            type="text"
                            block
                            icon={<UserAddOutlined />}
                            onClick={(e) => {
                              e.preventDefault();
                              navigate(location.pathname, { state: { clientModal: true } });
                            }}
                            style={{ textAlign: "left", paddingLeft: 11, paddingRight: 11 }}
                          >
                            <Trans>New client</Trans>
                          </Button>
                        </>
                      )}
                    >
                      {map(clients, (client: any) => (
                        <Select.Option value={client.id} key={client.id}>
                          {get(client, "name", "-")}
                        </Select.Option>
                      ))}
                    </Select>
                  </Form.Item>
                  {selectedClientAddress && (
                    <Typography.Text
                      type="secondary"
                      style={{ display: "block", marginBottom: 16 }}
                    >
                      {selectedClientAddress}
                    </Typography.Text>
                  )}
                  <Form.Item label={t`Customer note`} name="customerNotes">
                    <TextArea rows={4} />
                  </Form.Item>
                  {/* Filling the space this column otherwise leaves empty next
                      to the right column's denser field grid, rather than
                      giving it a full-width row of its own over there. */}
                  <Form.Item
                    label={t`Payment terms`}
                    name="paymentTerms"
                    style={{ marginBottom: 0 }}
                  >
                    <Select
                      showSearch
                      allowClear
                      optionFilterProp="children"
                      placeholder={t`Select payment terms`}
                      popupRender={(menu) => (
                        <>
                          {menu}
                          <Divider style={{ margin: "8px 0" }} />
                          <Button
                            type="text"
                            block
                            icon={<ScheduleOutlined />}
                            onClick={(e) => {
                              e.preventDefault();
                              navigate("/settings/payment-terms");
                            }}
                            style={{ textAlign: "left", paddingLeft: 11, paddingRight: 11 }}
                          >
                            <Trans>Manage payment terms</Trans>
                          </Button>
                        </>
                      )}
                    >
                      {map(paymentTerms, (pt: any) => (
                        <Option key={pt.id} value={pt.name}>
                          {pt.name}
                        </Option>
                      ))}
                    </Select>
                  </Form.Item>
                </Col>

                {/* Right: everything else, packed two-per-row so this column's
                    total height roughly matches the left column's instead of
                    each field getting its own mostly-empty full-width row. */}
                <Col xs={24} xl={12}>
                  <Row gutter={16}>
                    <Col xs={24} md={12}>
                      <Form.Item
                        label={t`Invoice number`}
                        name="number"
                        rules={[{ required: true, message: t`This field is required!` }]}
                      >
                        <Input />
                      </Form.Item>
                    </Col>
                    <CurrencySelect
                      form={form}
                      organizationId={organization?.id}
                      orgCurrency={orgCurrency}
                      xl={12}
                    />
                  </Row>
                  {/* Inlined rather than <ExchangeRateFields>: that component's
                      Cols are xl={4}, sized for a full-width Row — nested one
                      level down here they'd shrink to a quarter of this
                      already-half-width column at real desktop widths. */}
                  {showExchangeRateFields(watchedCurrency, orgCurrency) && (
                    <Row gutter={16}>
                      <Col xs={24} md={12}>
                        <Form.Item
                          label={<Trans>Exchange rate</Trans>}
                          name="exchangeRate"
                          // Same message as <ExchangeRateFields>'s own tooltip
                          // (src/components/currency/currency-fields.tsx) — the
                          // interpolated var is named `currency` there too, so
                          // this reuses that catalog entry instead of adding a
                          // near-duplicate one under a different placeholder name.
                          tooltip={((currency: string) =>
                            t`1 ${currency} = this many ${orgCurrency}`)(watchedCurrency)}
                          rules={[{ required: true, message: t`This field is required!` }]}
                        >
                          <InputNumber
                            min={0}
                            step={0.0001}
                            precision={6}
                            style={{ width: "100%" }}
                          />
                        </Form.Item>
                      </Col>
                      <Col xs={24} md={12}>
                        <Form.Item
                          label={<Trans>Rate date</Trans>}
                          name="exchangeRateDate"
                          rules={[{ required: true, message: t`This field is required!` }]}
                        >
                          <DatePicker style={{ width: "100%" }} />
                        </Form.Item>
                      </Col>
                    </Row>
                  )}
                  <Row gutter={16}>
                    <Col xs={24} md={12}>
                      <Form.Item
                        label={t`Date`}
                        name="date"
                        rules={[{ required: true, message: t`This field is required!` }]}
                      >
                        <DatePicker style={{ width: "100%" }} format={dateFormat} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item
                        label={t`Due date`}
                        name="dueDate"
                        rules={[{ required: true, message: t`This field is required!` }]}
                      >
                        <DatePicker style={{ width: "100%" }} format={dateFormat} />
                      </Form.Item>
                    </Col>
                  </Row>
                  <Row gutter={16}>
                    <Col xs={24} md={12}>
                      <Form.Item
                        label={t`Overdue charge`}
                        name="overdueCharge"
                        help={
                          <span
                            style={{ fontSize: "12px", display: "block", textAlign: "right" }}
                          >{t`Daily %`}</span>
                        }
                      >
                        <InputNumber
                          style={{ width: "100%" }}
                          min={0}
                          max={100}
                          step={0.01}
                          formatter={(value) => `${value} %`}
                          parser={(value) => value?.replace("%", "") as any}
                          placeholder="0%"
                        />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item
                        label={t`Buyer reference`}
                        name="buyerReference"
                        tooltip={t`Mandatory for German B2G XRechnung, e.g. a Leitweg-ID.`}
                      >
                        <Input />
                      </Form.Item>
                    </Col>
                  </Row>

                  {/* Both fields are independent, organization-level opt-ins
                      (see Organizations → Accounting) — deliberately not
                      gated on invoiceLayout, which is only a PDF template
                      choice; any layout can carry either field, and either
                      field can be used without switching layout. */}
                  {(!!organization.fiscalStampEnabled || !!organization.withholdingTaxEnabled) && (
                    <Row gutter={16}>
                      {organization.fiscalStampEnabled ? (
                        <Col xs={24} md={12}>
                          <Form.Item
                            label={<Trans>Fiscal stamp</Trans>}
                            name="fiscalStampAmount"
                            tooltip={
                              <Trans>
                                A flat statutory duty added to the invoice total, not subject to
                                VAT.
                              </Trans>
                            }
                          >
                            <InputNumber style={{ width: "100%" }} min={0} precision={3} />
                          </Form.Item>
                        </Col>
                      ) : null}
                      {organization.withholdingTaxEnabled ? (
                        <Col xs={24} md={12}>
                          <Form.Item
                            label={<Trans>Withholding tax rate</Trans>}
                            name="withholdingTaxRate"
                            tooltip={
                              <Trans>
                                Percentage the client withholds and remits to the tax authority on
                                your behalf. Reduces the net amount you'll actually receive, not the
                                invoice total.
                              </Trans>
                            }
                          >
                            <InputNumber
                              style={{ width: "100%" }}
                              min={0}
                              max={100}
                              addonAfter="%"
                            />
                          </Form.Item>
                        </Col>
                      ) : null}
                    </Row>
                  )}
                </Col>
              </Row>
            </Card>

            <Card size="small" title={<Trans>Line items</Trans>} style={{ marginBottom: 24 }}>
              <Row gutter={16}>
                <Col span={24}>
                  <LineItemsTable
                    reorderable
                    defaultNewRow={{
                      quantity: 1,
                      taxRate: get(find(taxRates, { isDefault: 1 }), "id"),
                    }}
                    columns={[
                      { kind: "index" },
                      {
                        kind: "custom",
                        key: "productId",
                        title: t`Product`,
                        width: 180,
                        render: (field) => (
                          <Form.Item
                            name={[field.name, "productId"]}
                            rules={[
                              requiredForNewLineItem(form, field.name, t`This field is required!`),
                            ]}
                            noStyle
                          >
                            <Select
                              showSearch
                              style={{ width: "100%" }}
                              placeholder={t`Select product`}
                              optionFilterProp="children"
                              onChange={(productId) => {
                                const product = find(products, { id: productId });
                                if (product) {
                                  const lineItems = form.getFieldValue("lineItems");
                                  const quantity = get(lineItems[field.name], "quantity") || 1;
                                  const unitPrice = centsToUnits((product as any).price ?? 0);
                                  lineItems[field.name] = {
                                    ...lineItems[field.name],
                                    description: (product as any).name,
                                    unitPrice,
                                    total: multiplyDecimal(quantity, unitPrice),
                                    ...((product as any).taxRateId
                                      ? { taxRate: (product as any).taxRateId }
                                      : {}),
                                  };
                                  form.setFieldValue("lineItems", [...lineItems]);
                                }
                              }}
                            >
                              {map(sellableProducts, (p: any) => (
                                <Option key={p.id} value={p.id}>
                                  {p.name}
                                  {p.sku ? ` (${p.sku})` : ""}
                                </Option>
                              ))}
                            </Select>
                          </Form.Item>
                        ),
                      },
                      { kind: "description", required: true, rows: 4 },
                      {
                        kind: "custom",
                        key: "quantity",
                        title: t`Qty.`,
                        width: 80,
                        render: (field) => (
                          <Form.Item
                            name={[field.name, "quantity"]}
                            rules={[{ required: true, message: t`This field is required!` }]}
                            noStyle
                          >
                            <InputNumber
                              style={{ width: "100%" }}
                              onChange={(value) => {
                                const total = form.getFieldValue(["lineItems", field.key, "total"]);
                                const unitPrice = form.getFieldValue([
                                  "lineItems",
                                  field.key,
                                  "unitPrice",
                                ]);

                                value = toNumber(value);
                                if (value) {
                                  if (!unitPrice && total) {
                                    form.setFieldValue(
                                      ["lineItems", field.key, "unitPrice"],
                                      divideDecimal(total, value),
                                    );
                                  } else if (unitPrice) {
                                    form.setFieldValue(
                                      ["lineItems", field.key, "total"],
                                      multiplyDecimal(value, unitPrice),
                                    );
                                  }
                                }
                              }}
                            />
                          </Form.Item>
                        ),
                      },
                      {
                        kind: "custom",
                        key: "unitPrice",
                        title: t`Price`,
                        width: 120,
                        render: (field) => (
                          <Form.Item
                            name={[field.name, "unitPrice"]}
                            rules={[{ required: true, message: t`This field is required!` }]}
                            noStyle
                          >
                            <InputNumber
                              style={{ width: "100%" }}
                              onChange={(value) => {
                                const total = form.getFieldValue(["lineItems", field.key, "total"]);
                                const quantity = form.getFieldValue([
                                  "lineItems",
                                  field.key,
                                  "quantity",
                                ]);

                                value = toNumber(value);
                                if (value) {
                                  if (!quantity && total) {
                                    form.setFieldValue(
                                      ["lineItems", field.key, "quantity"],
                                      divideDecimal(total, value),
                                    );
                                  } else if (quantity) {
                                    form.setFieldValue(
                                      ["lineItems", field.key, "total"],
                                      multiplyDecimal(quantity, value),
                                    );
                                  }
                                }
                              }}
                            />
                          </Form.Item>
                        ),
                      },
                      {
                        kind: "custom",
                        key: "taxRate",
                        title: t`Tax %`,
                        width: 120,
                        render: (field) => (
                          <Form.Item name={[field.name, "taxRate"]} noStyle>
                            <Select
                              style={{ width: "100%" }}
                              allowClear
                              placeholder={t`Select tax rate`}
                            >
                              {map(taxRates, (rate: any) => (
                                <Option value={rate.id} key={rate.id}>
                                  {rate.name} {rate.percentage}%
                                </Option>
                              ))}
                            </Select>
                          </Form.Item>
                        ),
                      },
                      {
                        kind: "custom",
                        key: "total",
                        title: t`Total`,
                        width: 120,
                        render: (field) => (
                          <Form.Item
                            name={[field.name, "total"]}
                            rules={[{ required: true, message: t`This field is required!` }]}
                            noStyle
                          >
                            <InputNumber
                              style={{ width: "100%" }}
                              onChange={(value) => {
                                const unitPrice = form.getFieldValue([
                                  "lineItems",
                                  field.key,
                                  "unitPrice",
                                ]);
                                const quantity = form.getFieldValue([
                                  "lineItems",
                                  field.key,
                                  "quantity",
                                ]);

                                value = toNumber(value);
                                if (value) {
                                  if (!quantity && unitPrice) {
                                    form.setFieldValue(
                                      ["lineItems", field.key, "quantity"],
                                      divideDecimal(value, unitPrice),
                                    );
                                  } else if (quantity) {
                                    form.setFieldValue(
                                      ["lineItems", field.key, "unitPrice"],
                                      divideDecimal(value, quantity),
                                    );
                                  }
                                }
                              }}
                            />
                          </Form.Item>
                        ),
                      },
                    ]}
                  />
                </Col>
              </Row>

              <Row gutter={16} style={{ marginTop: 16 }}>
                {/* Totals, right-aligned within the card */}
                <Col xs={24} xl={{ span: 12, offset: 12 }}>
                  <Descriptions
                    column={1}
                    styles={{
                      content: {
                        textAlign: "right",
                        display: "inline-block",
                        minWidth: 120,
                        color: "rgba(0, 0, 0, 0.88)",
                        fontSize: 15,
                        lineHeight: 1.4,
                      },
                      label: {
                        textAlign: "right",
                        display: "inline-block",
                        width: "100%",
                        color: "rgba(0, 0, 0, 0.88)",
                        fontWeight: 500,
                        fontSize: 15,
                        lineHeight: 1.4,
                      },
                    }}
                  >
                    {(() => {
                      const fmt = (value: number) =>
                        Intl.NumberFormat(i18n.locale, {
                          style: "currency",
                          currency: watchedCurrency ?? organization.currency ?? "EUR",
                          minimumFractionDigits: organization.minimum_fraction_digits ?? undefined,
                        }).format(value);
                      return (
                        <>
                          <Descriptions.Item label={<Trans>Subtotal</Trans>}>
                            {fmt(subTotal)}
                          </Descriptions.Item>
                          {taxGroups.length > 0 ? (
                            taxGroups.map((group) => (
                              <Descriptions.Item
                                key={group.taxRate?.id}
                                label={`${group.taxRate?.name || t`Tax`} ${group.taxRate?.percentage || 0}%`}
                              >
                                {fmt(group.tax)}
                              </Descriptions.Item>
                            ))
                          ) : (
                            <Descriptions.Item label={<Trans>Tax</Trans>}>
                              {fmt(0)}
                            </Descriptions.Item>
                          )}
                          {fiscalStampAmount > 0 && (
                            <Descriptions.Item label={<Trans>Fiscal stamp</Trans>}>
                              {fmt(fiscalStampAmount)}
                            </Descriptions.Item>
                          )}
                          <Descriptions.Item
                            label={
                              <strong>
                                <Trans>Total</Trans>
                              </strong>
                            }
                          >
                            <strong>{fmt(total)}</strong>
                          </Descriptions.Item>
                          {withholdingTaxRate ? (
                            <>
                              <Descriptions.Item
                                label={<Trans>Withholding tax ({withholdingTaxRate}%)</Trans>}
                              >
                                -{fmt(withholdingTaxAmount)}
                              </Descriptions.Item>
                              <Descriptions.Item
                                label={
                                  <strong>
                                    <Trans>Net amount due</Trans>
                                  </strong>
                                }
                              >
                                <strong>{fmt(netAmountDue)}</strong>
                              </Descriptions.Item>
                            </>
                          ) : null}
                        </>
                      );
                    })()}
                  </Descriptions>
                </Col>
              </Row>
            </Card>

            {/* Footer menu */}
            {document.getElementById("footer") &&
              createPortal(
                <Footer
                  style={{
                    position: "sticky",
                    bottom: 0,
                    zIndex: 1,
                    padding: 0,
                    background: colorBgContainer,
                    paddingLeft: 16,
                    paddingRight: 16,
                  }}
                >
                  <Row align="middle" justify="space-between" style={{ height: 64 }}>
                    <Col>
                      <Space>
                        {id && !isNew && (
                          <Button type="dashed" onClick={handleDuplicate(id)}>
                            <CopyOutlined /> <Trans>Duplicate</Trans>
                          </Button>
                        )}
                        {id && !isNew && currentInvoiceState !== "paid" && (
                          <Popconfirm
                            title={t`Delete the invoice?`}
                            description={t`Are you sure to delete this invoice?`}
                            onConfirm={handleDelete(id)}
                            okText={t`Yes`}
                            cancelText={t`No`}
                          >
                            <Button type="dashed">
                              <DeleteOutlined /> <Trans>Delete</Trans>
                            </Button>
                          </Popconfirm>
                        )}
                      </Space>
                    </Col>
                    <Col>
                      <Space size="middle" split={<Divider type="vertical" />}>
                        {footerActionGroups.map((group, i) => (
                          <Space key={i}>{group}</Space>
                        ))}
                      </Space>
                    </Col>
                  </Row>
                </Footer>,
                // @ts-expect-error - Footer can be null
                document.getElementById("footer"),
              )}
          </Form>
          {previewMode &&
            (isCustomTemplate ? (
              // getInvoicePDFLayout would silently fall back to the "default"
              // React component for "custom", rendering a different document
              // than the Excel button actually exports — so no live preview
              // for a custom template in v1, only a note.
              <div style={{ padding: 24, textAlign: "center" }}>
                <Trans>
                  Preview isn't available for a custom template — use Excel to see the result.
                </Trans>
              </div>
            ) : (
              <PDFPreview createPDFDocument={createPDFDocument} />
            ))}
        </Col>
      </Row>

      {!isNew && invoice && (
        <Row>
          <Col span={24}>
            <PaymentPanel
              organizationId={organization.id}
              documentType="invoice"
              documentId={id!}
              direction="inbound"
              clientId={(invoice as any).clientId}
              currency={(invoice as any).currency ?? orgCurrency}
              orgCurrency={orgCurrency}
              total={unitsToCents((invoice as any).total ?? 0)}
              hasPostedEntry={currentInvoiceState === "sent" || currentInvoiceState === "paid"}
            />
          </Col>
        </Row>
      )}

      <ClientForm />
    </>
  );
};

export default InvoiceDetails;
