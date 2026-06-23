import "src/styles/base.scss";

// Import devtools styles for development
if (import.meta.env.DEV && import.meta.env.VITE_JOTAI_DEVTOOLS_ENABLED === "true") {
  import("jotai-devtools/styles.css");
}

// Initialize Sentry for error tracking
import { initSentry } from "src/utils/sentry";
initSentry();

import "dayjs/locale/en";
import "dayjs/locale/en-gb";
import "dayjs/locale/et";
import "dayjs/locale/de";
import "dayjs/locale/fi";
import "dayjs/locale/fr";
import "dayjs/locale/el";
import "dayjs/locale/nl";
import "dayjs/locale/pt";
import "dayjs/locale/sv";
import "dayjs/locale/uk";

import { useEffect, useState, useMemo, lazy, Suspense } from "react";
import { BrowserRouter, Navigate, Route, Routes, useNavigate, useLocation } from "react-router";
import { ConfigProvider } from "antd";
import enUS from "antd/locale/en_US";
import enGB from "antd/locale/en_GB";
import etEE from "antd/locale/et_EE";
import deDE from "antd/locale/de_DE";
import fiFI from "antd/locale/fi_FI";
import frFR from "antd/locale/fr_FR";
import elGR from "antd/locale/el_GR";
import nlNL from "antd/locale/nl_NL";
import ptPT from "antd/locale/pt_PT";
import svSE from "antd/locale/sv_SE";
import ukUA from "antd/locale/uk_UA";
import { useAtomValue, useSetAtom } from "jotai";
import { i18n } from "@lingui/core";
import { I18nProvider } from "@lingui/react";
import dayjs from "dayjs";
import localizedFormat from "dayjs/plugin/localizedFormat";

import { localeAtom } from "src/atoms/generic";
import { dynamicActivate } from "src/utils/lingui";

import { organizationIdAtom, setOrganizationsAtom } from "src/atoms/organization";
import { currentUserAtom } from "src/atoms/auth";
import { getToken, clearToken } from "src/api/client";
import { GetMe } from "src/api";
import BaseLayout from "src/layouts/base";
import Clients from "src/routes/clients";
import Products from "src/routes/products";
import Inventory from "src/routes/inventory";
import Orders from "src/routes/orders";
import OrderDetails from "src/routes/orders/details";
import Deliveries from "src/routes/deliveries";
import DeliveryDetails from "src/routes/deliveries/details";
import Index from "src/routes/index";
import Invoices from "src/routes/invoices";
import InvoiceDetails from "src/routes/invoices/details.tsx";
import LoginPage from "src/routes/login";
import SettingsInvoice from "src/routes/settings/invoice";
import SettingsTaxRates from "src/routes/settings/tax-rates";
import OrganizationsList from "src/routes/organizations/index";
import SettingsBackup from "src/routes/settings/backup";
import SettingsUsers from "src/routes/settings/users";
import NewOrganization from "src/routes/organizations/new";

// Components
import Loading from "src/components/loading";
import TaxRateForm from "src/components/tax-rates/form.tsx";

dayjs.extend(localizedFormat);

// Lazy load DevTools for development
const DevTools =
  import.meta.env.DEV && import.meta.env.VITE_JOTAI_DEVTOOLS_ENABLED === "true"
    ? lazy(() => import("jotai-devtools").then((module) => ({ default: module.DevTools })))
    : () => null;

const AppContent = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const [isInitialLoading, setIsInitialLoading] = useState(true);

  // Load locale
  const locale = useAtomValue(localeAtom);

  // Auth
  const setCurrentUser = useSetAtom(currentUserAtom);

  // Organizations
  const organizationId = useAtomValue(organizationIdAtom);
  const setOrganizations = useSetAtom(setOrganizationsAtom);

  // Map locale to Ant Design locale
  const antdLocale = useMemo(() => {
    let baseLocale;
    switch (locale) {
      case "et":
        baseLocale = etEE;
        break;
      case "de":
        baseLocale = deDE;
        break;
      case "fi":
        baseLocale = fiFI;
        break;
      case "fr":
        baseLocale = frFR;
        break;
      case "el":
        baseLocale = elGR;
        break;
      case "nl":
        baseLocale = nlNL;
        break;
      case "pt":
        baseLocale = ptPT;
        break;
      case "sv":
        baseLocale = svSE;
        break;
      case "uk":
        baseLocale = ukUA;
        break;
      case "en-GB":
        baseLocale = enGB;
        break;
      case "en":
      default:
        baseLocale = enUS;
    }

    return baseLocale;
  }, [locale]);

  useEffect(() => {
    dynamicActivate(locale);
  }, [locale]);

  useEffect(() => {
    // Load current user from token on mount
    if (getToken()) {
      GetMe()
        .then(setCurrentUser)
        .catch(() => {
          clearToken();
          if (location.pathname !== "/login") navigate("/login");
        });
    } else if (location.pathname !== "/login") {
      navigate("/login");
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (getToken()) {
      setOrganizations();
    }
  }, [setOrganizations]);

  // Brief loading to prevent CSS flicker
  useEffect(() => {
    const timer = setTimeout(() => {
      setIsInitialLoading(false);
    }, 100);

    return () => clearTimeout(timer);
  }, []);

  // Redirect to index if no organization is selected and we're not on an allowed path
  useEffect(() => {
    const allowedPathsWithoutOrg = ["/", "/organizations/new"];
    const isAllowedPath = allowedPathsWithoutOrg.includes(location.pathname);

    if (organizationId === null && !isAllowedPath) {
      navigate("/");
    }
  }, [organizationId, navigate, location.pathname]);

  // Show loading spinner briefly to prevent CSS flicker
  if (isInitialLoading) {
    return <Loading />;
  }

  return (
    <ConfigProvider
      locale={antdLocale}
      theme={{
        token: {
          borderRadius: 2,
        },
      }}
    >
      {import.meta.env.DEV && import.meta.env.VITE_JOTAI_DEVTOOLS_ENABLED === "true" && (
        <Suspense fallback={null}>
          <DevTools />
        </Suspense>
      )}
      <I18nProvider i18n={i18n}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/" element={<Index />} />
          <Route path="/organizations/new" element={<NewOrganization />} />
          <Route path="/organizations" element={<BaseLayout />}>
            <Route index element={<OrganizationsList />} />
          </Route>
          <Route path="/invoices" element={<BaseLayout />}>
            <Route index element={<Invoices />} />
            <Route path=":id" element={<InvoiceDetails />} />
            <Route path=":id/pdf" element={<InvoiceDetails />} />
          </Route>
          <Route path="/clients" element={<BaseLayout />}>
            <Route index element={<Clients />} />
          </Route>
          <Route path="/products" element={<BaseLayout />}>
            <Route index element={<Products />} />
          </Route>
          <Route path="/inventory" element={<BaseLayout />}>
            <Route index element={<Inventory />} />
          </Route>
          <Route path="/orders" element={<BaseLayout />}>
            <Route index element={<Orders />} />
            <Route path=":id" element={<OrderDetails />} />
          </Route>
          <Route path="/deliveries" element={<BaseLayout />}>
            <Route index element={<Deliveries />} />
            <Route path=":id" element={<DeliveryDetails />} />
          </Route>
          <Route path="/settings" element={<BaseLayout />}>
            <Route index element={<Navigate to="/settings/invoice" />} />
            <Route path="invoice" element={<SettingsInvoice />} />
            <Route path="organization" element={<Navigate to="/organizations" />} />
            <Route path="tax-rates" element={<SettingsTaxRates />}>
              <Route path="new" element={<TaxRateForm />} />
              <Route path=":id" element={<TaxRateForm />} />
            </Route>
            <Route path="backup" element={<SettingsBackup />} />
            <Route path="users" element={<SettingsUsers />} />
          </Route>
        </Routes>
      </I18nProvider>
    </ConfigProvider>
  );
};

const App = () => {
  return (
    <BrowserRouter>
      <AppContent />
    </BrowserRouter>
  );
};

export default App;
