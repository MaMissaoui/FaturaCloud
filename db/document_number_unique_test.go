package db

import "testing"

func createTestOrder(t *testing.T, d *Database, orgID, number string) *Order {
	t.Helper()
	order, err := d.CreateOrder(CreateOrderRequest{
		OrganizationID: orgID, OrderNumber: number, Status: "draft", OrderDate: 1700000000000,
	})
	if err != nil {
		t.Fatalf("CreateOrder(%q): %v", number, err)
	}
	return order
}

// A blank number gets the next free one: a number someone typed earlier is
// skipped, and so are numbers reissued after the counter was set back.
func TestDocumentNumberSkipsTakenNumbers(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-dnu-skip"})

	createTestOrder(t, d, org.ID, "ORD-002") // typed by hand; the counter moves to 1
	if got := createTestOrder(t, d, org.ID, "").OrderNumber; got != "ORD-003" {
		t.Fatalf("blank number after a typed ORD-002: got %q, want ORD-003", got)
	}
	preview, err := d.PreviewNextDocumentNumber(org.ID, "order")
	if err != nil || preview != "ORD-004" {
		t.Fatalf("preview: got %q, %v; want ORD-004", preview, err)
	}

	// Setting the counter back reissues only free numbers.
	zero := int64(0)
	if _, err := d.UpdateDocumentNumberSetting(org.ID, "order", UpdateDocumentNumberSettingRequest{Format: "ORD-{number:3}", Counter: &zero}); err != nil {
		t.Fatalf("UpdateDocumentNumberSetting: %v", err)
	}
	if preview, _ := d.PreviewNextDocumentNumber(org.ID, "order"); preview != "ORD-001" {
		t.Fatalf("preview after counter reset: got %q, want ORD-001", preview)
	}
	if got := createTestOrder(t, d, org.ID, "").OrderNumber; got != "ORD-001" {
		t.Fatalf("first blank after counter reset: got %q, want ORD-001", got)
	}
	if got := createTestOrder(t, d, org.ID, "").OrderNumber; got != "ORD-004" {
		t.Fatalf("second blank after counter reset: got %q, want ORD-004 (002 and 003 taken)", got)
	}
}

// An explicit number another document of the type has is refused on create,
// and renumbering to one is refused on update; keeping one's own number isn't.
func TestDocumentNumberRefusesTakenExplicitNumbers(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-dnu-taken"})

	first := createTestOrder(t, d, org.ID, "SO-1")
	second := createTestOrder(t, d, org.ID, "SO-2")
	_, err := d.CreateOrder(CreateOrderRequest{OrganizationID: org.ID, OrderNumber: "SO-1", Status: "draft", OrderDate: 1700000000000})
	mustValidationError(t, err)

	taken := "SO-1"
	_, err = d.UpdateOrder(second.ID, UpdateOrderRequest{OrderNumber: &taken})
	mustValidationError(t, err)

	own := "SO-1"
	if _, err := d.UpdateOrder(first.ID, UpdateOrderRequest{OrderNumber: &own}); err != nil {
		t.Fatalf("resaving its own number: %v", err)
	}
	free := "SO-9"
	if _, err := d.UpdateOrder(second.ID, UpdateOrderRequest{OrderNumber: &free}); err != nil {
		t.Fatalf("renumber to a free number: %v", err)
	}
}

// Uniqueness is per document type and per organization: a delivery and a
// goods receipt may share a number, and so may two organizations.
func TestDocumentNumberScope(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgA, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-dnu-a"})
	orgB, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-dnu-b"})

	if _, err := d.CreateDelivery(CreateDeliveryRequest{OrganizationID: orgA.ID, DeliveryNumber: "X-1", DeliveryDate: 1700000000000}); err != nil {
		t.Fatalf("delivery X-1: %v", err)
	}
	if _, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{OrganizationID: orgA.ID, DeliveryNumber: "X-1", DeliveryDate: 1700000000000}); err != nil {
		t.Fatalf("goods receipt X-1 beside delivery X-1: %v", err)
	}
	if _, err := d.CreateDelivery(CreateDeliveryRequest{OrganizationID: orgB.ID, DeliveryNumber: "X-1", DeliveryDate: 1700000000000}); err != nil {
		t.Fatalf("delivery X-1 in another organization: %v", err)
	}
	_, err := d.CreateDelivery(CreateDeliveryRequest{OrganizationID: orgA.ID, DeliveryNumber: "X-1", DeliveryDate: 1700000000000})
	mustValidationError(t, err)
}

// The unique index is the backstop behind the checks, and its error maps to
// the 409 only for its own table: both delivery tables name the column
// deliveryNumber.
func TestDocumentNumberIndexBackstop(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-dnu-index"})

	insert := `INSERT INTO inbound_deliveries (id, organizationId, deliveryNumber, deliveryDate) VALUES (?, ?, 'GR-X', 1)`
	if _, err := d.DB.Exec(insert, "gr-1", org.ID); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	_, err := d.DB.Exec(insert, "gr-2", org.ID)
	if err == nil {
		t.Fatal("the index let a duplicate goods receipt number in")
	}
	if !isDuplicateDocumentNumber(err, "inbound_delivery") {
		t.Fatalf("not recognized as a goods receipt duplicate: %v", err)
	}
	if isDuplicateDocumentNumber(err, "delivery") {
		t.Fatalf("a goods receipt duplicate was taken for a delivery one: %v", err)
	}
	// Blank numbers are outside the index.
	blank := `INSERT INTO inbound_deliveries (id, organizationId, deliveryNumber, deliveryDate) VALUES (?, ?, '', 1)`
	for _, id := range []string{"gr-b1", "gr-b2"} {
		if _, err := d.DB.Exec(blank, id, org.ID); err != nil {
			t.Fatalf("blank number %s: %v", id, err)
		}
	}
}
