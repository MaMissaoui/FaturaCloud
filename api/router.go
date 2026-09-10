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
	// Batch counterpart to my-role (issue #147) — every organization the
	// caller belongs to, in one request, instead of one my-role call per row
	// on the Organizations list page. A different path shape than the
	// {orgId}/my-role route above (one fewer segment), so there's no route
	// collision to worry about.
	protected("GET", "/api/organizations/my-roles", h.getMyOrganizationRoles)
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
	// Per-document-type page orientation (landscape/portrait) — a setting,
	// not a file, so it's a separate sub-resource rather than folded into
	// the upload/download routes above. Same org-member protection tier.
	orgMemberProtected("GET", "/api/organizations/{orgId}/document-templates/{documentType}/orientation", pathOrgID("orgId"), h.getDocumentTemplateOrientation)
	orgMemberProtected("PUT", "/api/organizations/{orgId}/document-templates/{documentType}/orientation", pathOrgID("orgId"), h.updateDocumentTemplateOrientation)
	orgMemberProtected("DELETE", "/api/organizations/{orgId}/document-templates/{documentType}/orientation", pathOrgID("orgId"), h.deleteDocumentTemplateOrientation)

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
	// importOrgID resolves an import route's {id} to its owning organization
	// by reusing GetImport — same shape as clientOrgID above.
	importOrgID := func(r *http.Request) (string, error) {
		imp, err := h.db.GetImport(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return imp.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/imports", pathOrgID("orgId"), h.listImports)
	orgMemberProtected("GET", "/api/organizations/{orgId}/imports/summaries", pathOrgID("orgId"), h.listImportSummaries)
	orgMemberProtected("GET", "/api/organizations/{orgId}/imports/next-number", pathOrgID("orgId"), h.nextImportNumber)
	protected("POST", "/api/imports", h.createImport)
	orgMemberProtected("GET", "/api/imports/{id}", importOrgID, h.getImport)
	orgMemberProtected("GET", "/api/imports/{id}/summary", importOrgID, h.getImportSummary)
	orgMemberProtected("PUT", "/api/imports/{id}", importOrgID, h.updateImport)
	orgMemberProtected("DELETE", "/api/imports/{id}", importOrgID, h.deleteImport)

	// Purchase orders
	// purchaseOrderOrgID resolves a purchase-order route's {id} to its
	// owning organization by reusing GetPurchaseOrder — same shape as
	// clientOrgID above. Reused below for the export route too.
	purchaseOrderOrgID := func(r *http.Request) (string, error) {
		po, err := h.db.GetPurchaseOrder(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return po.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/purchase-orders", pathOrgID("orgId"), h.listPurchaseOrders)
	orgMemberProtected("GET", "/api/organizations/{orgId}/purchase-orders/next-number", pathOrgID("orgId"), h.nextPurchaseOrderNumber)
	protected("POST", "/api/purchase-orders", h.createPurchaseOrder)
	orgMemberProtected("GET", "/api/purchase-orders/{id}", purchaseOrderOrgID, h.getPurchaseOrder)
	orgMemberProtected("GET", "/api/purchase-orders/{id}/line-items", purchaseOrderOrgID, h.getPurchaseOrderLineItems)
	orgMemberProtected("GET", "/api/purchase-orders/{id}/received-quantities", purchaseOrderOrgID, h.getPurchaseOrderReceivedQuantities)
	orgMemberProtected("PUT", "/api/purchase-orders/{id}", purchaseOrderOrgID, h.updatePurchaseOrder)
	orgMemberProtected("PATCH", "/api/purchase-orders/{id}/status", purchaseOrderOrgID, h.updatePurchaseOrderStatus)
	orgMemberProtected("DELETE", "/api/purchase-orders/{id}", purchaseOrderOrgID, h.deletePurchaseOrder)
	// Same reasoning as GET /api/invoices/{id}/export above — registered
	// directly on mux, not through protected(), so a LibreOffice PDF
	// conversion never holds dbMu's read lock for its whole duration.
	// orgMember (not withDB) provides the membership check, same insertion
	// point orgAdminProtected/the invoice export route already use.
	mux.Handle("GET /api/purchase-orders/{id}/export", auth(h.orgMember(purchaseOrderOrgID)(csrf(limitBody(defaultMaxBody, h.exportPurchaseOrderDocument)))))

	// Inbound deliveries (goods receipts)
	// inboundDeliveryOrgID resolves an inbound-delivery route's {id} to its
	// owning organization by reusing GetInboundDelivery — same shape as
	// clientOrgID above. Reused below for the export route too.
	inboundDeliveryOrgID := func(r *http.Request) (string, error) {
		delivery, err := h.db.GetInboundDelivery(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return delivery.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/inbound-deliveries", pathOrgID("orgId"), h.listInboundDeliveries)
	orgMemberProtected("GET", "/api/organizations/{orgId}/inbound-deliveries/next-number", pathOrgID("orgId"), h.nextInboundDeliveryNumber)
	protected("POST", "/api/inbound-deliveries", h.createInboundDelivery)
	orgMemberProtected("GET", "/api/inbound-deliveries/{id}", inboundDeliveryOrgID, h.getInboundDelivery)
	orgMemberProtected("GET", "/api/inbound-deliveries/{id}/line-items", inboundDeliveryOrgID, h.getInboundDeliveryLineItems)
	orgMemberProtected("PUT", "/api/inbound-deliveries/{id}", inboundDeliveryOrgID, h.updateInboundDelivery)
	orgMemberProtected("PATCH", "/api/inbound-deliveries/{id}/status", inboundDeliveryOrgID, h.updateInboundDeliveryStatus)
	orgMemberProtected("DELETE", "/api/inbound-deliveries/{id}", inboundDeliveryOrgID, h.deleteInboundDelivery)
	// Same reasoning as GET /api/invoices/{id}/export above — registered
	// directly on mux, not through protected(), so a LibreOffice PDF
	// conversion never holds dbMu's read lock for its whole duration.
	// orgMember (not withDB) provides the membership check, same insertion
	// point orgAdminProtected/the invoice export route already use.
	mux.Handle("GET /api/inbound-deliveries/{id}/export", auth(h.orgMember(inboundDeliveryOrgID)(csrf(limitBody(defaultMaxBody, h.exportInboundDeliveryDocument)))))

	// Incoming invoices (vendor bills)
	// incomingInvoiceOrgID resolves an incoming-invoice route's {id} to its
	// owning organization by reusing GetIncomingInvoice — same shape as
	// clientOrgID above. Reused below for the export route and for the
	// GET .../payments route further down (see that section's note).
	incomingInvoiceOrgID := func(r *http.Request) (string, error) {
		invoice, err := h.db.GetIncomingInvoice(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return invoice.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/incoming-invoices", pathOrgID("orgId"), h.listIncomingInvoices)
	protected("POST", "/api/incoming-invoices", h.createIncomingInvoice)
	orgMemberProtected("GET", "/api/incoming-invoices/{id}", incomingInvoiceOrgID, h.getIncomingInvoice)
	orgMemberProtected("GET", "/api/incoming-invoices/{id}/line-items", incomingInvoiceOrgID, h.getIncomingInvoiceLineItems)
	orgMemberProtected("GET", "/api/incoming-invoices/{id}/match", incomingInvoiceOrgID, h.getIncomingInvoiceMatch)
	orgMemberProtected("PUT", "/api/incoming-invoices/{id}", incomingInvoiceOrgID, h.updateIncomingInvoice)
	orgMemberProtected("PATCH", "/api/incoming-invoices/{id}/state", incomingInvoiceOrgID, h.updateIncomingInvoiceState)
	orgMemberProtected("DELETE", "/api/incoming-invoices/{id}", incomingInvoiceOrgID, h.deleteIncomingInvoice)
	// Same reasoning as GET /api/invoices/{id}/export above — registered
	// directly on mux, not through protected(), so a LibreOffice PDF
	// conversion never holds dbMu's read lock for its whole duration.
	// orgMember (not withDB) provides the membership check, same insertion
	// point orgAdminProtected/the invoice export route already use.
	mux.Handle("GET /api/incoming-invoices/{id}/export", auth(h.orgMember(incomingInvoiceOrgID)(csrf(limitBody(defaultMaxBody, h.exportIncomingInvoiceDocument)))))

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
	// taxRateOrgID resolves a tax-rate route's {id} to its owning
	// organization by reusing GetTaxRate — same shape as clientOrgID above.
	taxRateOrgID := func(r *http.Request) (string, error) {
		taxRate, err := h.db.GetTaxRate(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return taxRate.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/tax-rates", pathOrgID("orgId"), h.listTaxRates)
	protected("POST", "/api/tax-rates", h.createTaxRate)
	orgMemberProtected("GET", "/api/tax-rates/{id}", taxRateOrgID, h.getTaxRate)
	orgMemberProtected("PUT", "/api/tax-rates/{id}", taxRateOrgID, h.updateTaxRate)
	orgMemberProtected("DELETE", "/api/tax-rates/{id}", taxRateOrgID, h.deleteTaxRate)
	orgMemberProtected("GET", "/api/tax-rates/{id}/usage-count", taxRateOrgID, h.getTaxRateUsageCount)

	// paymentTermOrgID resolves a payment-term route's {id} to its owning
	// organization by reusing GetPaymentTerm — same shape as clientOrgID
	// above. There is no GET /api/payment-terms/{id} route at all (only
	// PUT/DELETE by id, plus the org-scoped list) — see pendingPhaseCRoutes'
	// prior entries.
	paymentTermOrgID := func(r *http.Request) (string, error) {
		term, err := h.db.GetPaymentTerm(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return term.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/payment-terms", pathOrgID("orgId"), h.listPaymentTerms)
	protected("POST", "/api/payment-terms", h.createPaymentTerm)
	orgMemberProtected("PUT", "/api/payment-terms/{id}", paymentTermOrgID, h.updatePaymentTerm)
	orgMemberProtected("DELETE", "/api/payment-terms/{id}", paymentTermOrgID, h.deletePaymentTerm)

	// Countries — global picklist activation, not per-organization (the
	// new-organization form has no organization yet). Read is available to
	// any authenticated user since every org/vendor/client form needs it;
	// only toggling activation is admin-only.
	protected("GET", "/api/countries/active", h.listActiveCountries)
	platformAdminProtected("PATCH", "/api/countries/{code}", h.setCountryActive)

	// Products
	// productOrgID resolves a product route's {id} to its owning
	// organization by reusing GetProduct — same shape as clientOrgID above.
	productOrgID := func(r *http.Request) (string, error) {
		product, err := h.db.GetProduct(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return product.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/products", pathOrgID("orgId"), h.listProducts)
	protected("POST", "/api/products", h.createProduct)
	orgMemberProtected("GET", "/api/products/{id}", productOrgID, h.getProduct)
	orgMemberProtected("PUT", "/api/products/{id}", productOrgID, h.updateProduct)
	orgMemberProtected("DELETE", "/api/products/{id}", productOrgID, h.deleteProduct)
	orgMemberProtected("GET", "/api/products/{id}/stock-movements", productOrgID, h.listProductStockMovements)
	orgMemberProtected("GET", "/api/products/{id}/serial-numbers", productOrgID, h.listProductSerialNumbers)

	// Stock movements
	// stockMovementOrgID resolves a stock-movement route's {id} to its
	// owning organization by reusing the new GetStockMovement (db/stock.go)
	// — same shape as clientOrgID above. DELETE is the only by-id route here.
	stockMovementOrgID := func(r *http.Request) (string, error) {
		movement, err := h.db.GetStockMovement(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return movement.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/stock-movements", pathOrgID("orgId"), h.listStockMovements)
	protected("POST", "/api/stock-movements", h.createStockMovement)
	orgMemberProtected("DELETE", "/api/stock-movements/{id}", stockMovementOrgID, h.deleteStockMovement)

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
	// accountOrgID resolves an account route's {id} to its owning
	// organization by reusing GetAccount — same shape as clientOrgID above.
	accountOrgID := func(r *http.Request) (string, error) {
		account, err := h.db.GetAccount(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return account.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/accounts", pathOrgID("orgId"), h.listAccounts)
	protected("POST", "/api/accounts", h.createAccount)
	orgMemberProtected("GET", "/api/accounts/{id}", accountOrgID, h.getAccount)
	orgMemberProtected("PUT", "/api/accounts/{id}", accountOrgID, h.updateAccount)
	orgMemberProtected("DELETE", "/api/accounts/{id}", accountOrgID, h.deleteAccount)

	// Journals
	// journalOrgID resolves a journal route's {id} to its owning
	// organization by reusing GetJournal — same shape as clientOrgID above.
	// There is no GET /api/journals/{id} route at all (only PUT/DELETE by
	// id, plus the org-scoped list) — see pendingPhaseCRoutes' prior entries.
	journalOrgID := func(r *http.Request) (string, error) {
		journal, err := h.db.GetJournal(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return journal.OrganizationID, nil
	}
	orgMemberProtected("GET", "/api/organizations/{orgId}/journals", pathOrgID("orgId"), h.listJournals)
	protected("POST", "/api/journals", h.createJournal)
	orgMemberProtected("PUT", "/api/journals/{id}", journalOrgID, h.updateJournal)
	orgMemberProtected("DELETE", "/api/journals/{id}", journalOrgID, h.deleteJournal)

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

	// fiscalPeriodOrgID resolves a fiscal-period route's {id} to its owning
	// organization by reusing GetFiscalPeriod, which carries its own
	// OrganizationID column directly (no need to hop through its fiscal
	// year) — {id} here is a fiscal *period*, not a fiscal *year*, unlike
	// every other route in this section.
	fiscalPeriodOrgID := func(r *http.Request) (string, error) {
		period, err := h.db.GetFiscalPeriod(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return period.OrganizationID, nil
	}

	// Fiscal years / periods
	orgMemberProtected("GET", "/api/organizations/{orgId}/fiscal-years", pathOrgID("orgId"), h.listFiscalYears)
	protected("POST", "/api/fiscal-years", h.createFiscalYear)
	orgMemberProtected("GET", "/api/fiscal-years/{id}/periods", fiscalYearOrgID, h.listFiscalPeriods)
	protected("POST", "/api/fiscal-periods", h.createFiscalPeriod)
	orgMemberProtected("PATCH", "/api/fiscal-periods/{id}/status", fiscalPeriodOrgID, h.updateFiscalPeriodStatus)
	orgAdminProtected("POST", "/api/fiscal-years/{id}/close", fiscalYearOrgID, h.closeFiscalYear)

	// journalEntryOrgID resolves a journal-entry route's {id} to its owning
	// organization by reusing GetJournalEntry — same shape as clientOrgID
	// above.
	journalEntryOrgID := func(r *http.Request) (string, error) {
		entry, err := h.db.GetJournalEntry(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return entry.OrganizationID, nil
	}

	// Journal entries
	orgMemberProtected("GET", "/api/organizations/{orgId}/journal-entries", pathOrgID("orgId"), h.listJournalEntries)
	protected("POST", "/api/journal-entries", h.createJournalEntry)
	orgMemberProtected("GET", "/api/journal-entries/{id}", journalEntryOrgID, h.getJournalEntry)
	orgMemberProtected("GET", "/api/journal-entries/{id}/lines", journalEntryOrgID, h.getJournalEntryLines)
	orgMemberProtected("PATCH", "/api/journal-entries/{id}/post", journalEntryOrgID, h.postJournalEntry)
	orgMemberProtected("POST", "/api/journal-entries/{id}/reverse", journalEntryOrgID, h.reverseJournalEntry)
	orgMemberProtected("DELETE", "/api/journal-entries/{id}", journalEntryOrgID, h.deleteJournalEntry)

	// paymentOrgID resolves a payment route's {id} to its owning
	// organization by reusing GetPayment — same shape as clientOrgID above.
	paymentOrgID := func(r *http.Request) (string, error) {
		payment, err := h.db.GetPayment(r.PathValue("id"))
		if err != nil {
			return "", err
		}
		return payment.OrganizationID, nil
	}

	// Payments
	orgMemberProtected("GET", "/api/organizations/{orgId}/payments", pathOrgID("orgId"), h.listPayments)
	protected("POST", "/api/payments", h.createPayment)
	orgMemberProtected("GET", "/api/payments/{id}", paymentOrgID, h.getPayment)
	orgMemberProtected("GET", "/api/payments/{id}/applications", paymentOrgID, h.getPaymentApplications)
	orgMemberProtected("POST", "/api/payments/{id}/void", paymentOrgID, h.voidPayment)
	orgMemberProtected("GET", "/api/invoices/{id}/payments", invoiceOrgID, h.getInvoicePayments)
	orgMemberProtected("GET", "/api/incoming-invoices/{id}/payments", incomingInvoiceOrgID, h.getIncomingInvoicePayments)

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
