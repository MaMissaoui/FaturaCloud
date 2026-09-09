package api

import (
	"net/http"
	"net/netip"
	"sync"

	"github.com/MaMissaoui/fatura-cloud/db"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type handler struct {
	dbMu      sync.RWMutex
	db        *db.Database
	dbPath    string
	backupDir string
	jwtSecret string
	version   string

	// Reverse proxies allowed to set X-Forwarded-For for login rate-limiting
	// purposes (see clientIP in auth.go). Empty means none are trusted — the
	// direct TCP peer is always used instead.
	trustedProxies []netip.Prefix

	// OIDC SSO — oidcCfg.IssuerURL empty means the feature is disabled.
	// oidcVerifier/oidcOAuth2 are built lazily by ensureOIDC.
	oidcCfg      OIDCConfig
	oidcMu       sync.Mutex
	oidcVerifier *oidc.IDTokenVerifier
	oidcOAuth2   *oauth2.Config
}

// defaultMaxBody caps ordinary JSON request bodies (comfortably above any
// legitimate JSON payload while still guarding against unbounded-body memory
// exhaustion, including on the unauthenticated login route). The organization
// logo upload and the database restore upload each get their own, differently
// sized multipart limits since neither travels as JSON.
const defaultMaxBody = 10 << 20 // 10MB

func limitBody(limit int64, next http.HandlerFunc) http.Handler {
	return http.MaxBytesHandler(next, limit)
}

// NewRouter wires all API routes and returns the mux.
// The caller is responsible for mounting a static file handler at "/" for the
// embedded frontend.
func NewRouter(database *db.Database, dbPath, backupDir, jwtSecret, version string, oidcCfg OIDCConfig, trustedProxies []netip.Prefix) *http.ServeMux {
	h := &handler{
		db:             database,
		dbPath:         dbPath,
		backupDir:      backupDir,
		jwtSecret:      jwtSecret,
		version:        version,
		oidcCfg:        oidcCfg,
		trustedProxies: trustedProxies,
	}
	go h.runScheduler()
	go sweepLoginBuckets()

	mux := http.NewServeMux()

	// Public
	mux.Handle("GET /api/version", limitBody(defaultMaxBody, h.getVersion))
	// login/logout are POST and set/clear the auth cookie, so they carry the
	// same CSRF requirement as any other mutation.
	mux.Handle("POST /api/auth/login", h.csrfRequired(limitBody(defaultMaxBody, h.login)))
	mux.Handle("POST /api/auth/logout", h.csrfRequired(limitBody(defaultMaxBody, h.logout)))
	// OIDC SSO — public; these ARE the auth entry point, not gated by
	// authMiddleware. All three 503 if OIDC_ISSUER_URL isn't configured.
	mux.Handle("GET /api/auth/oidc/enabled", limitBody(defaultMaxBody, h.oidcEnabled))
	mux.Handle("GET /api/auth/oidc/login", limitBody(defaultMaxBody, h.oidcLoginStart))
	mux.Handle("GET /api/auth/oidc/callback", limitBody(defaultMaxBody, h.oidcCallback))

	// withDB holds dbMu's *read* lock for a request's full duration, so a
	// handler's use of h.db can never race a restore's write-locked swap
	// (api/utility.go's swapDatabase). It wraps the innermost handler only —
	// never the two restore routes below, which take the write lock
	// themselves and would deadlock against a held read lock.
	withDB := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			h.dbMu.RLock()
			defer h.dbMu.RUnlock()
			next(w, r)
		}
	}

	// Protected — all routes below require a valid JWT
	auth := h.authMiddleware
	platformAdmin := h.platformAdmin
	// csrf gates state-changing methods on a custom header (see csrfRequired);
	// GET routes pass through it untouched, so it's safe to apply uniformly.
	csrf := h.csrfRequired
	protected := func(method, pattern string, handlerFn http.HandlerFunc) {
		mux.Handle(method+" "+pattern, auth(csrf(limitBody(defaultMaxBody, withDB(handlerFn)))))
	}
	// platformAdminProtected gates the handful of genuinely global routes
	// (user account management, backups, DB restore, countries) that have
	// no natural per-org owner — see api/middleware.go's platformAdmin.
	platformAdminProtected := func(method, pattern string, handlerFn http.HandlerFunc) {
		mux.Handle(method+" "+pattern, auth(platformAdmin(csrf(limitBody(defaultMaxBody, withDB(handlerFn))))))
	}
	// orgAdminProtected gates an org-scoped admin action — the caller must
	// be an admin *of that organization*, not a platform admin. resolve
	// says how to find the org id a given route's request targets (see
	// pathOrgID for the common "it's a path value" case).
	orgAdminProtected := func(method, pattern string, resolve orgIDResolver, handlerFn http.HandlerFunc) {
		mux.Handle(method+" "+pattern, auth(h.orgAdmin(resolve)(csrf(limitBody(defaultMaxBody, withDB(handlerFn))))))
	}
	// orgMemberProtected gates an ordinary org-scoped route on plain
	// membership (any role) — the Phase C counterpart to orgAdminProtected,
	// for routes that only need "the caller belongs to this organization,"
	// not admin privileges within it.
	orgMemberProtected := func(method, pattern string, resolve orgIDResolver, handlerFn http.HandlerFunc) {
		mux.Handle(method+" "+pattern, auth(h.orgMember(resolve)(csrf(limitBody(defaultMaxBody, withDB(handlerFn))))))
	}

	// Auth
	protected("GET", "/api/auth/me", h.me)

	// Backup — the whole surface is platform-admin-only (a global,
	// non-org-scoped concern); the sidebar already hides it from non-admins,
	// so the API matches that boundary instead of only gating the
	// state-changing operations.
	platformAdminProtected("GET", "/api/backups", h.listBackups)
	platformAdminProtected("POST", "/api/backups", h.triggerBackup)
	platformAdminProtected("GET", "/api/backup/config", h.getBackupConfig)
	platformAdminProtected("PUT", "/api/backup/config", h.setBackupConfig)
	// Restore routes swap out h.db under dbMu's *write* lock (see
	// swapDatabase) — they must never be wrapped in withDB's read lock, which
	// would deadlock against it. Registered directly instead of through
	// platformAdminProtected, which folds withDB into every route.
	mux.Handle("POST /api/backups/{name}/restore", auth(platformAdmin(csrf(limitBody(defaultMaxBody, h.restoreNamedBackup)))))
	// Restore uploads stream a full SQLite database file, so this route needs a
	// much larger body limit than the default — matching restoreDatabase's own
	// ParseMultipartForm cap.
	mux.Handle("POST /api/restore", auth(platformAdmin(csrf(limitBody(256<<20, h.restoreDatabase)))))

	// Users (platform admin only — a global, non-org-scoped concern)
	platformAdminProtected("GET", "/api/users", h.listUsers)
	platformAdminProtected("POST", "/api/users", h.createUser)
	platformAdminProtected("GET", "/api/users/{id}", h.getUser)
	platformAdminProtected("PUT", "/api/users/{id}", h.updateUser)
	platformAdminProtected("DELETE", "/api/users/{id}", h.deleteUser)

	// Organizations
	// listOrganizations itself filters to the caller's own memberships
	// (issue #141 Phase C) — no route-level org resolver applies here, since
	// there's no single target org to resolve; the scoping happens inside
	// the handler via GetUserOrganizations.
	protected("GET", "/api/organizations", h.listOrganizations)
	protected("POST", "/api/organizations", h.createOrganization)
	orgMemberProtected("GET", "/api/organizations/{id}", pathOrgID("id"), h.getOrganization)
	orgMemberProtected("PUT", "/api/organizations/{id}", pathOrgID("id"), h.updateOrganization)
	// Deleting an organization cascade-deletes all of its clients, invoices,
	// orders, and deliveries — the caller must be an admin of *this*
	// organization (not necessarily a platform admin).
	orgAdminProtected("DELETE", "/api/organizations/{id}", pathOrgID("id"), h.deleteOrganization)
	orgAdminProtected("POST", "/api/organizations/{id}/reset", pathOrgID("id"), h.resetOrganizationData)
	orgMemberProtected("GET", "/api/organizations/{id}/usage-count", pathOrgID("id"), h.getOrganizationUsageCount)
	orgMemberProtected("GET", "/api/organizations/{id}/logo", pathOrgID("id"), h.getOrganizationLogo)
	orgMemberProtected("POST", "/api/organizations/{id}/logo", pathOrgID("id"), h.uploadOrganizationLogo)
	orgMemberProtected("DELETE", "/api/organizations/{id}/logo", pathOrgID("id"), h.deleteOrganizationLogo)

	// Organization membership — who can access this organization and at what
	// role. Managing membership is itself an org-admin action, same tier as
	// deleting/resetting the organization above. my-role is the one
	// exception — any authenticated user may ask their own role, which is
	// how the frontend decides whether to show org-admin-only UI at all.
	protected("GET", "/api/organizations/{orgId}/my-role", h.getMyOrganizationRole)
	orgAdminProtected("GET", "/api/organizations/{orgId}/members", pathOrgID("orgId"), h.listOrganizationMembers)
	orgAdminProtected("POST", "/api/organizations/{orgId}/members", pathOrgID("orgId"), h.addOrganizationMember)
	orgAdminProtected("PUT", "/api/organizations/{orgId}/members/{userId}", pathOrgID("orgId"), h.updateOrganizationMemberRole)
	orgAdminProtected("DELETE", "/api/organizations/{orgId}/members/{userId}", pathOrgID("orgId"), h.removeOrganizationMember)

	// Document templates (issue #115) — per-org, per-document-type Excel
	// export template overrides. Same protection tier as the logo endpoints
	// (not admin-only): org configuration any user with org access manages.
	orgMemberProtected("GET", "/api/organizations/{orgId}/document-templates", pathOrgID("orgId"), h.listDocumentTemplates)
	orgMemberProtected("GET", "/api/organizations/{orgId}/document-templates/{documentType}", pathOrgID("orgId"), h.getDocumentTemplate)
	orgMemberProtected("POST", "/api/organizations/{orgId}/document-templates/{documentType}", pathOrgID("orgId"), h.uploadDocumentTemplate)
	orgMemberProtected("DELETE", "/api/organizations/{orgId}/document-templates/{documentType}", pathOrgID("orgId"), h.deleteDocumentTemplate)

	// Clients
	// clientOrgID resolves a client route's {id} to its owning organization
	// by reusing GetClient — the same row every one of these handlers
	// already needs (or, for PUT/DELETE, cheaply can fetch), rather than a
	// bespoke organizationId-only query. Reused across every route below
	// keyed on a client's own {id}.
	clientOrgID := func(r *http.Request) (string, error) {
		client, err := h.db.GetClient(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return client.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/clients", pathOrgID("orgId"), h.listClients)
	protected("POST", "/api/clients", h.createClient)
	orgMemberProtected("GET", "/api/clients/{id}", clientOrgID, h.getClient)
	orgMemberProtected("PUT", "/api/clients/{id}", clientOrgID, h.updateClient)
	orgMemberProtected("DELETE", "/api/clients/{id}", clientOrgID, h.deleteClient)
	orgMemberProtected("GET", "/api/clients/{id}/invoice-count", clientOrgID, h.getClientInvoiceCount)

	// Vendors
	// vendorOrgID resolves a vendor route's {id} to its owning organization
	// by reusing GetVendor — same shape as clientOrgID above.
	vendorOrgID := func(r *http.Request) (string, error) {
		vendor, err := h.db.GetVendor(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return vendor.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/vendors", pathOrgID("orgId"), h.listVendors)
	protected("POST", "/api/vendors", h.createVendor)
	orgMemberProtected("GET", "/api/vendors/{id}", vendorOrgID, h.getVendor)
	orgMemberProtected("PUT", "/api/vendors/{id}", vendorOrgID, h.updateVendor)
	orgMemberProtected("DELETE", "/api/vendors/{id}", vendorOrgID, h.deleteVendor)
	orgMemberProtected("GET", "/api/vendors/{id}/document-count", vendorOrgID, h.getVendorDocumentCount)

	// Imports (F114 — consolidated China shipments purchase orders link to)
	orgMemberProtected("GET", "/api/organizations/{orgId}/imports", pathOrgID("orgId"), h.listImports)
	orgMemberProtected("GET", "/api/organizations/{orgId}/imports/next-number", pathOrgID("orgId"), h.nextImportNumber)
	protected("POST", "/api/imports", h.createImport)
	protected("GET", "/api/imports/{id}", h.getImport)
	protected("GET", "/api/imports/{id}/summary", h.getImportSummary)
	protected("PUT", "/api/imports/{id}", h.updateImport)
	protected("DELETE", "/api/imports/{id}", h.deleteImport)

	// Purchase orders
	orgMemberProtected("GET", "/api/organizations/{orgId}/purchase-orders", pathOrgID("orgId"), h.listPurchaseOrders)
	orgMemberProtected("GET", "/api/organizations/{orgId}/purchase-orders/next-number", pathOrgID("orgId"), h.nextPurchaseOrderNumber)
	protected("POST", "/api/purchase-orders", h.createPurchaseOrder)
	protected("GET", "/api/purchase-orders/{id}", h.getPurchaseOrder)
	protected("GET", "/api/purchase-orders/{id}/line-items", h.getPurchaseOrderLineItems)
	protected("GET", "/api/purchase-orders/{id}/received-quantities", h.getPurchaseOrderReceivedQuantities)
	protected("PUT", "/api/purchase-orders/{id}", h.updatePurchaseOrder)
	protected("PATCH", "/api/purchase-orders/{id}/status", h.updatePurchaseOrderStatus)
	protected("DELETE", "/api/purchase-orders/{id}", h.deletePurchaseOrder)
	// Same reasoning as GET /api/invoices/{id}/export above — registered
	// directly on mux, not through protected(), so a LibreOffice PDF
	// conversion never holds dbMu's read lock for its whole duration.
	mux.Handle("GET /api/purchase-orders/{id}/export", auth(csrf(limitBody(defaultMaxBody, h.exportPurchaseOrderDocument))))

	// Inbound deliveries (goods receipts)
	orgMemberProtected("GET", "/api/organizations/{orgId}/inbound-deliveries", pathOrgID("orgId"), h.listInboundDeliveries)
	orgMemberProtected("GET", "/api/organizations/{orgId}/inbound-deliveries/next-number", pathOrgID("orgId"), h.nextInboundDeliveryNumber)
	protected("POST", "/api/inbound-deliveries", h.createInboundDelivery)
	protected("GET", "/api/inbound-deliveries/{id}", h.getInboundDelivery)
	protected("GET", "/api/inbound-deliveries/{id}/line-items", h.getInboundDeliveryLineItems)
	protected("PUT", "/api/inbound-deliveries/{id}", h.updateInboundDelivery)
	protected("PATCH", "/api/inbound-deliveries/{id}/status", h.updateInboundDeliveryStatus)
	protected("DELETE", "/api/inbound-deliveries/{id}", h.deleteInboundDelivery)
	// Same reasoning as GET /api/invoices/{id}/export above — registered
	// directly on mux, not through protected(), so a LibreOffice PDF
	// conversion never holds dbMu's read lock for its whole duration.
	mux.Handle("GET /api/inbound-deliveries/{id}/export", auth(csrf(limitBody(defaultMaxBody, h.exportInboundDeliveryDocument))))

	// Incoming invoices (vendor bills)
	orgMemberProtected("GET", "/api/organizations/{orgId}/incoming-invoices", pathOrgID("orgId"), h.listIncomingInvoices)
	protected("POST", "/api/incoming-invoices", h.createIncomingInvoice)
	protected("GET", "/api/incoming-invoices/{id}", h.getIncomingInvoice)
	protected("GET", "/api/incoming-invoices/{id}/line-items", h.getIncomingInvoiceLineItems)
	protected("GET", "/api/incoming-invoices/{id}/match", h.getIncomingInvoiceMatch)
	protected("PUT", "/api/incoming-invoices/{id}", h.updateIncomingInvoice)
	protected("PATCH", "/api/incoming-invoices/{id}/state", h.updateIncomingInvoiceState)
	protected("DELETE", "/api/incoming-invoices/{id}", h.deleteIncomingInvoice)
	// Same reasoning as GET /api/invoices/{id}/export above — registered
	// directly on mux, not through protected(), so a LibreOffice PDF
	// conversion never holds dbMu's read lock for its whole duration.
	mux.Handle("GET /api/incoming-invoices/{id}/export", auth(csrf(limitBody(defaultMaxBody, h.exportIncomingInvoiceDocument))))

	// Invoices
	// invoiceOrgID resolves an invoice route's {id} to its owning
	// organization by reusing GetInvoice — same shape as clientOrgID above.
	// Reused below for the export route and for GET .../payments.
	invoiceOrgID := func(r *http.Request) (string, error) {
		invoice, err := h.db.GetInvoice(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return invoice.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/invoices", pathOrgID("orgId"), h.listInvoices)
	protected("POST", "/api/invoices", h.createInvoice)
	orgMemberProtected("GET", "/api/invoices/{id}", invoiceOrgID, h.getInvoice)
	orgMemberProtected("GET", "/api/invoices/{id}/line-items", invoiceOrgID, h.getInvoiceLineItems)
	orgMemberProtected("PUT", "/api/invoices/{id}", invoiceOrgID, h.updateInvoice)
	orgMemberProtected("PATCH", "/api/invoices/{id}/state", invoiceOrgID, h.updateInvoiceState)
	orgMemberProtected("DELETE", "/api/invoices/{id}", invoiceOrgID, h.deleteInvoice)
	orgMemberProtected("GET", "/api/invoices/{id}/e-invoice", invoiceOrgID, h.getInvoiceEInvoice)
	// Registered directly on mux, not through protected() — a LibreOffice
	// PDF conversion can take seconds, and protected()'s withDB would hold
	// dbMu's read lock for that whole duration, blocking a pending
	// /api/restore write-lock acquisition and everything behind it (see
	// exportInvoiceDocument's comment). The handler takes its own short RLock
	// around just the DB reads instead. orgMember (not withDB) provides the
	// membership check, same insertion point orgAdminProtected already uses
	// for its own no-withDB routes.
	mux.Handle("GET /api/invoices/{id}/export", auth(h.orgMember(invoiceOrgID)(csrf(limitBody(defaultMaxBody, h.exportInvoiceDocument)))))

	// Dashboard
	orgMemberProtected("GET", "/api/organizations/{orgId}/dashboard", pathOrgID("orgId"), h.getDashboard)

	// Exchange rate prefill (manual entry only — see db/exchange_rate.go)
	orgMemberProtected("GET", "/api/organizations/{orgId}/exchange-rate", pathOrgID("orgId"), h.getLastExchangeRate)

	// Tax rates
	orgMemberProtected("GET", "/api/organizations/{orgId}/tax-rates", pathOrgID("orgId"), h.listTaxRates)
	protected("POST", "/api/tax-rates", h.createTaxRate)
	protected("GET", "/api/tax-rates/{id}", h.getTaxRate)
	protected("PUT", "/api/tax-rates/{id}", h.updateTaxRate)
	protected("DELETE", "/api/tax-rates/{id}", h.deleteTaxRate)
	protected("GET", "/api/tax-rates/{id}/usage-count", h.getTaxRateUsageCount)

	orgMemberProtected("GET", "/api/organizations/{orgId}/payment-terms", pathOrgID("orgId"), h.listPaymentTerms)
	protected("POST", "/api/payment-terms", h.createPaymentTerm)
	protected("PUT", "/api/payment-terms/{id}", h.updatePaymentTerm)
	protected("DELETE", "/api/payment-terms/{id}", h.deletePaymentTerm)

	// Countries — global picklist activation, not per-organization (the
	// new-organization form has no organization yet). Read is available to
	// any authenticated user since every org/vendor/client form needs it;
	// only toggling activation is admin-only.
	protected("GET", "/api/countries/active", h.listActiveCountries)
	platformAdminProtected("PATCH", "/api/countries/{code}", h.setCountryActive)

	// Products
	orgMemberProtected("GET", "/api/organizations/{orgId}/products", pathOrgID("orgId"), h.listProducts)
	protected("POST", "/api/products", h.createProduct)
	protected("GET", "/api/products/{id}", h.getProduct)
	protected("PUT", "/api/products/{id}", h.updateProduct)
	protected("DELETE", "/api/products/{id}", h.deleteProduct)
	protected("GET", "/api/products/{id}/stock-movements", h.listProductStockMovements)
	protected("GET", "/api/products/{id}/serial-numbers", h.listProductSerialNumbers)

	// Stock movements
	orgMemberProtected("GET", "/api/organizations/{orgId}/stock-movements", pathOrgID("orgId"), h.listStockMovements)
	protected("POST", "/api/stock-movements", h.createStockMovement)
	protected("DELETE", "/api/stock-movements/{id}", h.deleteStockMovement)

	// Orders
	// orderOrgID resolves an order route's {id} to its owning organization
	// by reusing GetOrder — same shape as clientOrgID above.
	orderOrgID := func(r *http.Request) (string, error) {
		order, err := h.db.GetOrder(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return order.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/orders", pathOrgID("orgId"), h.listOrders)
	protected("POST", "/api/orders", h.createOrder)
	orgMemberProtected("GET", "/api/orders/{id}", orderOrgID, h.getOrder)
	orgMemberProtected("GET", "/api/orders/{id}/line-items", orderOrgID, h.getOrderLineItems)
	orgMemberProtected("GET", "/api/orders/{id}/delivered-quantities", orderOrgID, h.getOrderDeliveredQuantities)
	orgMemberProtected("PUT", "/api/orders/{id}", orderOrgID, h.updateOrder)
	orgMemberProtected("PATCH", "/api/orders/{id}/status", orderOrgID, h.updateOrderStatus)
	orgMemberProtected("DELETE", "/api/orders/{id}", orderOrgID, h.deleteOrder)
	// Same reasoning as GET /api/invoices/{id}/export above — registered
	// directly on mux, not through protected(), so a LibreOffice PDF
	// conversion never holds dbMu's read lock for its whole duration.
	mux.Handle("GET /api/orders/{id}/export", auth(h.orgMember(orderOrgID)(csrf(limitBody(defaultMaxBody, h.exportOrderDocument)))))

	// Outbound deliveries
	// deliveryOrgID resolves a delivery route's {id} to its owning
	// organization by reusing GetDelivery — same shape as clientOrgID above.
	deliveryOrgID := func(r *http.Request) (string, error) {
		delivery, err := h.db.GetDelivery(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return delivery.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/deliveries", pathOrgID("orgId"), h.listDeliveries)
	orgMemberProtected("GET", "/api/organizations/{orgId}/deliveries/next-number", pathOrgID("orgId"), h.nextDeliveryNumber)
	protected("POST", "/api/deliveries", h.createDelivery)
	orgMemberProtected("GET", "/api/deliveries/{id}", deliveryOrgID, h.getDelivery)
	orgMemberProtected("GET", "/api/deliveries/{id}/line-items", deliveryOrgID, h.getDeliveryLineItems)
	orgMemberProtected("PUT", "/api/deliveries/{id}", deliveryOrgID, h.updateDelivery)
	orgMemberProtected("PATCH", "/api/deliveries/{id}/status", deliveryOrgID, h.updateDeliveryStatus)
	orgMemberProtected("DELETE", "/api/deliveries/{id}", deliveryOrgID, h.deleteDelivery)
	// Same reasoning as GET /api/invoices/{id}/export above — registered
	// directly on mux, not through protected(), so a LibreOffice PDF
	// conversion never holds dbMu's read lock for its whole duration.
	mux.Handle("GET /api/deliveries/{id}/export", auth(h.orgMember(deliveryOrgID)(csrf(limitBody(defaultMaxBody, h.exportDeliveryDocument)))))

	// Chart of accounts
	orgMemberProtected("GET", "/api/organizations/{orgId}/accounts", pathOrgID("orgId"), h.listAccounts)
	protected("POST", "/api/accounts", h.createAccount)
	protected("GET", "/api/accounts/{id}", h.getAccount)
	protected("PUT", "/api/accounts/{id}", h.updateAccount)
	protected("DELETE", "/api/accounts/{id}", h.deleteAccount)

	// Journals
	orgMemberProtected("GET", "/api/organizations/{orgId}/journals", pathOrgID("orgId"), h.listJournals)
	protected("POST", "/api/journals", h.createJournal)
	protected("PUT", "/api/journals/{id}", h.updateJournal)
	protected("DELETE", "/api/journals/{id}", h.deleteJournal)

	// The fiscal year's own row names its organization — {id} is the fiscal
	// year, not the org, so this needs a lookup instead of pathOrgID. Hoisted
	// above its first use (listFiscalPeriods, via GET .../periods) rather
	// than only closeFiscalYear further down, which used to be its sole use.
	fiscalYearOrgID := func(r *http.Request) (string, error) {
		fy, err := h.db.GetFiscalYear(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return fy.OrganizationID, nil
	}

	// Fiscal years / periods
	orgMemberProtected("GET", "/api/organizations/{orgId}/fiscal-years", pathOrgID("orgId"), h.listFiscalYears)
	protected("POST", "/api/fiscal-years", h.createFiscalYear)
	orgMemberProtected("GET", "/api/fiscal-years/{id}/periods", fiscalYearOrgID, h.listFiscalPeriods)
	protected("POST", "/api/fiscal-periods", h.createFiscalPeriod)
	protected("PATCH", "/api/fiscal-periods/{id}/status", h.updateFiscalPeriodStatus)
	orgAdminProtected("POST", "/api/fiscal-years/{id}/close", fiscalYearOrgID, h.closeFiscalYear)

	// Journal entries
	orgMemberProtected("GET", "/api/organizations/{orgId}/journal-entries", pathOrgID("orgId"), h.listJournalEntries)
	protected("POST", "/api/journal-entries", h.createJournalEntry)
	protected("GET", "/api/journal-entries/{id}", h.getJournalEntry)
	protected("GET", "/api/journal-entries/{id}/lines", h.getJournalEntryLines)
	protected("PATCH", "/api/journal-entries/{id}/post", h.postJournalEntry)
	protected("POST", "/api/journal-entries/{id}/reverse", h.reverseJournalEntry)
	protected("DELETE", "/api/journal-entries/{id}", h.deleteJournalEntry)

	// Payments
	orgMemberProtected("GET", "/api/organizations/{orgId}/payments", pathOrgID("orgId"), h.listPayments)
	protected("POST", "/api/payments", h.createPayment)
	protected("GET", "/api/payments/{id}", h.getPayment)
	protected("GET", "/api/payments/{id}/applications", h.getPaymentApplications)
	protected("POST", "/api/payments/{id}/void", h.voidPayment)
	orgMemberProtected("GET", "/api/invoices/{id}/payments", invoiceOrgID, h.getInvoicePayments)
	protected("GET", "/api/incoming-invoices/{id}/payments", h.getIncomingInvoicePayments)

	// Reports
	orgMemberProtected("GET", "/api/organizations/{orgId}/reports/trial-balance", pathOrgID("orgId"), h.getTrialBalance)
	orgMemberProtected("GET", "/api/organizations/{orgId}/reports/profit-and-loss", pathOrgID("orgId"), h.getProfitAndLoss)
	orgMemberProtected("GET", "/api/organizations/{orgId}/reports/balance-sheet", pathOrgID("orgId"), h.getBalanceSheet)
	orgMemberProtected("GET", "/api/organizations/{orgId}/reports/ar-aging", pathOrgID("orgId"), h.getReceivableAging)
	orgMemberProtected("GET", "/api/organizations/{orgId}/reports/ap-aging", pathOrgID("orgId"), h.getPayableAging)
	orgMemberProtected("GET", "/api/organizations/{orgId}/reports/inventory-valuation", pathOrgID("orgId"), h.getInventoryValuation)

	// Reporting — document-derived sales/purchasing analytics, a distinct
	// tier from the GL-derived Reports above (see db/sales_reports.go).
	orgMemberProtected("GET", "/api/organizations/{orgId}/reporting/revenue-trend", pathOrgID("orgId"), h.getRevenueTrend)
	orgMemberProtected("GET", "/api/organizations/{orgId}/reporting/sales-by-client", pathOrgID("orgId"), h.getSalesByClient)
	orgMemberProtected("GET", "/api/organizations/{orgId}/reporting/sales-by-product", pathOrgID("orgId"), h.getSalesByProduct)
	orgMemberProtected("GET", "/api/organizations/{orgId}/reporting/purchases-by-vendor", pathOrgID("orgId"), h.getPurchasesByVendor)
	orgMemberProtected("GET", "/api/organizations/{orgId}/reporting/tax-summary", pathOrgID("orgId"), h.getTaxSummary)

	// GL export — France FEC only; DATEV is deliberately not implemented
	// yet (see db/export_fec.go and the GL Export settings page). Admin-only,
	// same sensitivity class as the database backup download: a full ledger
	// dump for the fiscal year, not a single document.
	orgAdminProtected("GET", "/api/organizations/{orgId}/gl-export/fec", pathOrgID("orgId"), h.getFECExport)
	orgAdminProtected("GET", "/api/organizations/{orgId}/gl-export/datev", pathOrgID("orgId"), h.getDATEVExport)

	return mux
}
