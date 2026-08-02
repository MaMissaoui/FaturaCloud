import { useEffect, useState, useMemo, useCallback } from "react";
import { createPortal } from "react-dom";
import { useLocation, useNavigate, useParams } from "react-router";
import {
  App,
  Button,
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
  FilePdfOutlined,
  FileTextOutlined,
  SaveOutlined,
  UserAddOutlined,
} from "@ant-design/icons";
import LineItemsTable from "src/components/line-items/table";
import { SaveFile, DownloadInvoiceEInvoice } from "src/api";
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
import { siderAtom } from "src/atoms/generic";
import ClientForm from "src/components/clients/form.tsx";
import InvoicePDF from "src/components/invoices/pdf";
import { currencies } from "src/utils/currencies";
import ExchangeRateFields, {
  prefillExchangeRate,
} from "src/components/currency/exchange-rate-fields";
import { buildSepaCreditTransferPayload } from "src/utils/sepa-qr";
import { generateInvoiceNumber } from "src/utils/invoice";
import { requiredForNewLineItem } from "src/utils/line-items";
import {
  multiplyDecimal,
  divideDecimal,
  calculateTax,
  addDecimal,
  centsToUnits,
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

  // Callback ref that measures width when div is actually rendered
  const containerRef = useCallback((node: HTMLDivElement | null) => {
    if (node) {
      const measureWidth = () => {
        const width = node.offsetWidth;
        setContainerWidth(width - 40); // subtract some padding
      };

      // Initial measurement
      measureWidth();

      // Re-measure on resize
      const handleResize = () => measureWidth();
      window.addEventListener("resize", handleResize);

      // Store cleanup function for later use instead of returning it
      const cleanup = () => {
        window.removeEventListener("resize", handleResize);
      };

      // Store cleanup function on the node for later access
      (node as any)._cleanup = cleanup;
    } else {
      // Node is being unmounted, run cleanup if it exists
      const prevNode = containerRef as any;
      if (prevNode._cleanup) {
        prevNode._cleanup();
      }
    }
    // Don't return anything to avoid the warning
  }, []);

  // Effect to re-measure when sidebar changes
  useEffect(() => {
    // Trigger re-measurement when sidebar state changes
    console.log("Sidebar state changed:", siderCollapsed);
    const timer = setTimeout(() => {
      // Force a complete layout recalculation by triggering resize event
      window.dispatchEvent(new Event("resize"));

      // Wait a bit more for the resize event to be processed
      setTimeout(() => {
        const container = document.querySelector("[data-pdf-container]") as HTMLDivElement;
        if (container && container.parentElement) {
          // Use the parent element's width instead of the container's width
          const parentWidth = container.parentElement.offsetWidth;
          const containerWidth = container.offsetWidth;
          const windowWidth = window.innerWidth;

          console.log("Sidebar change measurements:", {
            containerWidth,
            parentWidth,
            windowWidth,
            siderCollapsed,
            usingParentWidth: true,
            finalWidth: parentWidth - 40,
          });

          // Use parent width instead of container width
          setContainerWidth(parentWidth - 40);
        }
      }, 100);
    }, 500);

    return () => clearTimeout(timer);
  }, [siderCollapsed]);

  useEffect(() => {
    const generatePDF = async () => {
      setLoading(true);
      setError(null);

      try {
        const document = createPDFDocument();
        if (!document) {
          setError("Please select a client to view PDF preview.");
          setLoading(false);
          return;
        }

        const blob = await pdf(document!).toBlob();
        const url = URL.createObjectURL(blob);
        setPdfUrl(url);
      } catch (err) {
        console.error("PDF generation error:", err);
        setError("Error generating PDF preview. Please try again.");
      } finally {
        setLoading(false);
      }
    };

    generatePDF();

    // Cleanup URL when component unmounts
    return () => {
      if (pdfUrl) {
        URL.revokeObjectURL(pdfUrl);
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
  const taxRates = useAtomValue(taxRatesAtom);
  const setTaxRates = useSetAtom(setTaxRatesAtom);
  const deleteInvoice = useSetAtom(deleteInvoiceAtom);
  const duplicateInvoice = useSetAtom(duplicateInvoiceAtom);
  const updateInvoiceState = useSetAtom(updateInvoiceStateAtom);
  const nextInvoiceNumber = useAtomValue(nextInvoiceNumberAtom);
  const [, setSubmitting] = useState(false);
  const [previewMode, setPreviewMode] = useState(false);
  const [downloadingEInvoice, setDownloadingEInvoice] = useState(false);
  const dateFormat = useDatePickerFormat();

  const isNew = id === "new";

  useEffect(() => {
    setClients();
    setProducts();
    setTaxRates();
    if (!isNew) {
      setInvoiceId(id || null);
    }

    // Clean up
    return () => {
      setInvoiceId(null);
    };
  }, [id, isNew, setClients, setProducts, setInvoiceId, setTaxRates]);

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
      overdueCharge: organization?.overdueCharge || 0,
      number: isNew ? nextInvoiceNumber || "" : undefined,
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
    setSubmitting(true);
    await setInvoice({
      ...values,
      subTotal,
      taxTotal,
      total,
      overdueCharge: values.overdueCharge,
    });
    setSubmitting(false);
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
  const total = addDecimal(subTotal, taxTotal);

  // SEPA credit transfer QR ("GiroCode") for the PDF's payment box. Only
  // renders for EUR invoices on an organization with an IBAN — SEPA credit
  // transfers don't exist for other currencies. Regenerated whenever the
  // total, currency, invoice number, or organization's bank details change.
  const watchedCurrency = Form.useWatch("currency", form);
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
      // Ensure line items have the correct totals
      lineItems: formValues.lineItems || [],
    };

    return (
      <InvoicePDF
        invoice={invoiceForPDF}
        client={clientData}
        organization={organization}
        taxRates={taxRates}
        i18n={i18n}
        qrCodeDataUri={qrCodeDataUri}
      />
    );
  };

  const currentInvoiceState =
    !isNew && invoice && typeof invoice === "object" && !("then" in invoice)
      ? ((invoice as any).state ?? "draft")
      : "draft";

  if (!organization) return null;
  if (!isNew && !invoice) return null;

  return (
    <>
      <Row>
        <Col span={24}>
          <Form
            form={form}
            onFinish={handleSubmit}
            layout="vertical"
            initialValues={initialValues}
            style={{ display: previewMode ? "none" : "block" }}
          >
            <Row gutter={24}>
              <Col xs={24} md={24} xl={12}>
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
              </Col>
              <Col xs={24} md={12} xl={6}>
                <Form.Item
                  label={t`Invoice number`}
                  name="number"
                  rules={[{ required: true, message: t`This field is required!` }]}
                >
                  <Input />
                </Form.Item>
              </Col>
              <Col xs={24} md={12} xl={6}>
                <Form.Item
                  label={t`Currency`}
                  name="currency"
                  rules={[{ required: true, message: t`This field is required!` }]}
                >
                  <Select
                    onChange={(currency: string) =>
                      prefillExchangeRate(form, organization?.id, currency, orgCurrency)
                    }
                  >
                    {map(currencies, (currency) => {
                      return (
                        <Option value={currency} key={currency}>
                          {currency}
                        </Option>
                      );
                    })}
                  </Select>
                </Form.Item>
              </Col>
              <ExchangeRateFields currency={watchedCurrency} orgCurrency={orgCurrency} />
            </Row>
            <Row gutter={24}>
              <Col xs={24} md={12} xl={{ span: 4, offset: 12 }}>
                <Form.Item
                  label={t`Date`}
                  name="date"
                  rules={[{ required: true, message: t`This field is required!` }]}
                >
                  <DatePicker style={{ width: "100%" }} format={dateFormat} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12} xl={4}>
                <Form.Item
                  label={t`Due date`}
                  name="dueDate"
                  rules={[{ required: true, message: t`This field is required!` }]}
                >
                  <DatePicker style={{ width: "100%" }} format={dateFormat} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12} xl={4}>
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
            </Row>

            <Row gutter={24}>
              <Col xs={24} md={12}>
                <Form.Item
                  label={t`Buyer reference`}
                  name="buyerReference"
                  tooltip={t`Mandatory for German B2G XRechnung, e.g. a Leitweg-ID.`}
                >
                  <Input />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item label={t`Payment terms`} name="paymentTerms">
                  <Input />
                </Form.Item>
              </Col>
            </Row>

            <Row gutter={16} style={{ marginTop: "20px" }}>
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
                            {map(products, (p: any) => (
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
                          <Select style={{ width: "100%" }} allowClear placeholder={t`Select tax rate`}>
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

            <Row gutter={16}>
              <Col xs={24} xl={8}>
                <Form.Item label={t`Customer note`} name="customerNotes">
                  <TextArea rows={4} />
                </Form.Item>
              </Col>

              {/* Totals */}
              <Col xs={24} xl={{ span: 12, offset: 4 }}>
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
                  <Descriptions.Item label={<Trans>Subtotal</Trans>}>
                    {Intl.NumberFormat(i18n.locale, {
                      style: "currency",
                      currency: watchedCurrency ?? organization.currency ?? "EUR",
                      minimumFractionDigits: organization.minimum_fraction_digits ?? undefined,
                    }).format(subTotal)}
                  </Descriptions.Item>
                  {taxGroups.length > 0 ? (
                    taxGroups.map((group) => (
                      <Descriptions.Item
                        key={group.taxRate?.id}
                        label={`${group.taxRate?.name || "Tax"} ${group.taxRate?.percentage || 0}%`}
                      >
                        {Intl.NumberFormat(i18n.locale, {
                          style: "currency",
                          currency: watchedCurrency ?? organization.currency ?? "EUR",
                          minimumFractionDigits: organization.minimum_fraction_digits ?? undefined,
                        }).format(group.tax)}
                      </Descriptions.Item>
                    ))
                  ) : (
                    <Descriptions.Item label={<Trans>Tax</Trans>}>
                      {Intl.NumberFormat(i18n.locale, {
                        style: "currency",
                        currency: watchedCurrency ?? organization.currency ?? "EUR",
                        minimumFractionDigits: organization.minimum_fraction_digits ?? undefined,
                      }).format(0)}
                    </Descriptions.Item>
                  )}
                  <Descriptions.Item
                    label={
                      <strong>
                        <Trans>Total</Trans>
                      </strong>
                    }
                  >
                    <strong>
                      {Intl.NumberFormat(i18n.locale, {
                        style: "currency",
                        currency: watchedCurrency ?? organization.currency ?? "EUR",
                        minimumFractionDigits: organization.minimum_fraction_digits ?? undefined,
                      }).format(total)}
                    </strong>
                  </Descriptions.Item>
                </Descriptions>
              </Col>
            </Row>

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
                      <Space>
                        {/*!isNew && invoice && (
                          <Dropdown overlay={stateMenu(invoice._id, invoice._rev)} trigger={["click"]}>
                            <StateTag state={invoice.state} style={{ marginTop: 10, marginRight: 20 }} />
                          </Dropdown>
                        )*/}
                        {!isNew && (
                          <Button type="dashed" onClick={() => setPreviewMode(!previewMode)}>
                            {previewMode ? (
                              <>
                                <EditOutlined /> <Trans>Edit</Trans>
                              </>
                            ) : (
                              <>
                                <EyeOutlined /> <Trans>View</Trans>
                              </>
                            )}
                          </Button>
                        )}
                        {!isNew && (
                          <Button
                            onClick={async () => {
                              const document = createPDFDocument();
                              if (!document) return;
                              const blob = await pdf(document).toBlob();
                              await SaveFile(`invoice-${id}.pdf`, blob);
                            }}
                          >
                            <FilePdfOutlined /> PDF
                          </Button>
                        )}
                        {!isNew && (
                          <Button
                            loading={downloadingEInvoice}
                            onClick={async () => {
                              setDownloadingEInvoice(true);
                              try {
                                await DownloadInvoiceEInvoice(id!);
                              } catch (error) {
                                message.error(
                                  error instanceof Error
                                    ? error.message
                                    : t`E-invoice export failed`,
                                );
                              } finally {
                                setDownloadingEInvoice(false);
                              }
                            }}
                          >
                            <FileTextOutlined /> <Trans>E-Invoice (XML)</Trans>
                          </Button>
                        )}
                        {!isNew && currentInvoiceState !== "cancelled" && (
                          <Popconfirm
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
                          </Popconfirm>
                        )}
                        <Button
                          type="primary"
                          disabled={false}
                          loading={false}
                          onClick={() => form.submit()}
                        >
                          <SaveOutlined /> <Trans>Save</Trans>
                        </Button>
                      </Space>
                    </Col>
                  </Row>
                </Footer>,
                // @ts-expect-error - Footer can be null
                document.getElementById("footer"),
              )}
          </Form>
          {previewMode && (
            // PDF Preview Mode
            <PDFPreview createPDFDocument={createPDFDocument} />
          )}
        </Col>
      </Row>

      <ClientForm />
    </>
  );
};

export default InvoiceDetails;
