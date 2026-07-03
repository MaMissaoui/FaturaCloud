package api

import (
	"net/http"
	"sync"

	"github.com/MaMissaoui/fatura-cloud/db"
)

type handler struct {
	dbMu      sync.RWMutex
	db        *db.Database
	dbPath    string
	backupDir string
	jwtSecret string
	version   string
}

// defaultMaxBody caps ordinary JSON request bodies (comfortably above the
// largest legitimate payload — a base64-encoded organization logo — while
// still guarding against unbounded-body memory exhaustion, including on the
// unauthenticated login route). The database restore upload gets its own,
// much larger limit since it streams a full SQLite file.
const defaultMaxBody = 10 << 20 // 10MB

func limitBody(limit int64, next http.HandlerFunc) http.Handler {
	return http.MaxBytesHandler(next, limit)
}

// NewRouter wires all API routes and returns the mux.
// The caller is responsible for mounting a static file handler at "/" for the
// embedded frontend.
func NewRouter(database *db.Database, dbPath, backupDir, jwtSecret, version string) *http.ServeMux {
	h := &handler{
		db:        database,
		dbPath:    dbPath,
		backupDir: backupDir,
		jwtSecret: jwtSecret,
		version:   version,
	}
	go h.runScheduler()
	go sweepLoginBuckets()

	mux := http.NewServeMux()

	// Public
	mux.Handle("GET /api/version", limitBody(defaultMaxBody, h.getVersion))
	mux.Handle("POST /api/auth/login", limitBody(defaultMaxBody, h.login))
	mux.Handle("POST /api/auth/logout", limitBody(defaultMaxBody, h.logout))

	// Protected — all routes below require a valid JWT
	auth := h.authMiddleware
	adminOnly := h.adminOnly
	protected := func(method, pattern string, handlerFn http.HandlerFunc) {
		mux.Handle(method+" "+pattern, auth(limitBody(defaultMaxBody, handlerFn)))
	}
	adminProtected := func(method, pattern string, handlerFn http.HandlerFunc) {
		mux.Handle(method+" "+pattern, auth(adminOnly(limitBody(defaultMaxBody, handlerFn))))
	}

	// Auth
	protected("GET", "/api/auth/me", h.me)

	// Backup — the whole surface is admin-only; the sidebar already hides it
	// from non-admins, so the API matches that boundary instead of only
	// gating the state-changing operations.
	adminProtected("GET", "/api/backups", h.listBackups)
	adminProtected("POST", "/api/backups", h.triggerBackup)
	adminProtected("POST", "/api/backups/{name}/restore", h.restoreNamedBackup)
	adminProtected("GET", "/api/backup/config", h.getBackupConfig)
	adminProtected("PUT", "/api/backup/config", h.setBackupConfig)
	// Restore uploads stream a full SQLite database file, so this route needs a
	// much larger body limit than the default — matching restoreDatabase's own
	// ParseMultipartForm cap.
	mux.Handle("POST /api/restore", auth(adminOnly(limitBody(256<<20, h.restoreDatabase))))

	// Users (admin only)
	adminProtected("GET", "/api/users", h.listUsers)
	adminProtected("POST", "/api/users", h.createUser)
	adminProtected("GET", "/api/users/{id}", h.getUser)
	adminProtected("PUT", "/api/users/{id}", h.updateUser)
	adminProtected("DELETE", "/api/users/{id}", h.deleteUser)

	// Organizations
	protected("GET", "/api/organizations", h.listOrganizations)
	protected("POST", "/api/organizations", h.createOrganization)
	protected("GET", "/api/organizations/{id}", h.getOrganization)
	protected("PUT", "/api/organizations/{id}", h.updateOrganization)
	protected("DELETE", "/api/organizations/{id}", h.deleteOrganization)
	protected("GET", "/api/organizations/{id}/usage-count", h.getOrganizationUsageCount)

	// Clients
	protected("GET", "/api/organizations/{orgId}/clients", h.listClients)
	protected("POST", "/api/clients", h.createClient)
	protected("GET", "/api/clients/{id}", h.getClient)
	protected("PUT", "/api/clients/{id}", h.updateClient)
	protected("DELETE", "/api/clients/{id}", h.deleteClient)
	protected("GET", "/api/clients/{id}/invoice-count", h.getClientInvoiceCount)

	// Invoices
	protected("GET", "/api/organizations/{orgId}/invoices", h.listInvoices)
	protected("POST", "/api/invoices", h.createInvoice)
	protected("GET", "/api/invoices/{id}", h.getInvoice)
	protected("GET", "/api/invoices/{id}/line-items", h.getInvoiceLineItems)
	protected("PUT", "/api/invoices/{id}", h.updateInvoice)
	protected("PATCH", "/api/invoices/{id}/state", h.updateInvoiceState)
	protected("DELETE", "/api/invoices/{id}", h.deleteInvoice)

	// Tax rates
	protected("GET", "/api/organizations/{orgId}/tax-rates", h.listTaxRates)
	protected("POST", "/api/tax-rates", h.createTaxRate)
	protected("GET", "/api/tax-rates/{id}", h.getTaxRate)
	protected("PUT", "/api/tax-rates/{id}", h.updateTaxRate)
	protected("DELETE", "/api/tax-rates/{id}", h.deleteTaxRate)
	protected("GET", "/api/tax-rates/{id}/usage-count", h.getTaxRateUsageCount)

	// Products
	protected("GET", "/api/organizations/{orgId}/products", h.listProducts)
	protected("POST", "/api/products", h.createProduct)
	protected("GET", "/api/products/{id}", h.getProduct)
	protected("PUT", "/api/products/{id}", h.updateProduct)
	protected("DELETE", "/api/products/{id}", h.deleteProduct)
	protected("GET", "/api/products/{id}/stock-movements", h.listProductStockMovements)

	// Stock movements
	protected("GET", "/api/organizations/{orgId}/stock-movements", h.listStockMovements)
	protected("POST", "/api/stock-movements", h.createStockMovement)
	protected("DELETE", "/api/stock-movements/{id}", h.deleteStockMovement)

	// Orders
	protected("GET", "/api/organizations/{orgId}/orders", h.listOrders)
	protected("POST", "/api/orders", h.createOrder)
	protected("GET", "/api/orders/{id}", h.getOrder)
	protected("GET", "/api/orders/{id}/line-items", h.getOrderLineItems)
	protected("GET", "/api/orders/{id}/delivered-quantities", h.getOrderDeliveredQuantities)
	protected("PUT", "/api/orders/{id}", h.updateOrder)
	protected("PATCH", "/api/orders/{id}/status", h.updateOrderStatus)
	protected("DELETE", "/api/orders/{id}", h.deleteOrder)

	// Outbound deliveries
	protected("GET", "/api/organizations/{orgId}/deliveries", h.listDeliveries)
	protected("GET", "/api/organizations/{orgId}/deliveries/next-number", h.nextDeliveryNumber)
	protected("POST", "/api/deliveries", h.createDelivery)
	protected("GET", "/api/deliveries/{id}", h.getDelivery)
	protected("GET", "/api/deliveries/{id}/line-items", h.getDeliveryLineItems)
	protected("PUT", "/api/deliveries/{id}", h.updateDelivery)
	protected("PATCH", "/api/deliveries/{id}/status", h.updateDeliveryStatus)
	protected("DELETE", "/api/deliveries/{id}", h.deleteDelivery)

	return mux
}
