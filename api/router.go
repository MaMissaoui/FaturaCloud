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

	mux := http.NewServeMux()

	// Public
	mux.HandleFunc("GET /api/version", h.getVersion)
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("POST /api/auth/logout", h.logout)

	// Protected — all routes below require a valid JWT
	auth := h.authMiddleware
	adminOnly := h.adminOnly
	protected := func(method, pattern string, handlerFn http.HandlerFunc) {
		mux.Handle(method+" "+pattern, auth(http.HandlerFunc(handlerFn)))
	}
	adminProtected := func(method, pattern string, handlerFn http.HandlerFunc) {
		mux.Handle(method+" "+pattern, auth(adminOnly(http.HandlerFunc(handlerFn))))
	}

	// Auth
	protected("GET", "/api/auth/me", h.me)

	// Backup
	protected("GET", "/api/backups", h.listBackups)
	protected("POST", "/api/backups", h.triggerBackup)
	protected("POST", "/api/backups/{name}/restore", h.restoreNamedBackup)
	protected("GET", "/api/backup/config", h.getBackupConfig)
	protected("PUT", "/api/backup/config", h.setBackupConfig)
	protected("POST", "/api/restore", h.restoreDatabase)

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
