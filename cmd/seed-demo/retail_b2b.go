package main

import (
	"fmt"
	"math"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// The retail shop's business customers. Counter sales are due the day they
// are made, so with Cash Book sales alone nothing is ever "not due yet", no
// invoice carries a discount, a fiscal stamp or a withholding, and orders and
// deliveries stay empty. A real appliance shop also sells to a handful of
// businesses on account — a hotel fitting out its rooms, a clinic, a school —
// and those go through the ordinary documents:
//
//   - an order, confirmed, then delivered (sometimes in two deliveries),
//     each delivery invoiced with the client's payment terms. Only the
//     delivery moves stock — an ordinary invoice never does — so the
//     appliances are reserved in the on-hand mirror when the order is taken;
//   - a monthly maintenance contract invoiced to the clients that have one;
//   - every business invoice carries the 1-dinar fiscal stamp, many a
//     discount (remise), and those to a public body or a large company the
//     1% withholding (retenue à la source) they keep back;
//   - most are paid by transfer or cheque around their due date, some in two
//     parts, some late, a few never; one is cancelled, and the last days leave
//     two invoices in draft.

type businessClient struct {
	name, city string
	termsDays  int
	// withholding is the client's retenue à la source (percent), 0 for none.
	withholding float64
	// contract is the client's monthly maintenance fee (cents, before tax),
	// 0 for none.
	contract int64
	// kinds are the appliance kinds the client buys.
	kinds []string
	// currency is "" for a Tunisian client (dinars) or the currency an
	// export client is billed in; countryCode is the client's country.
	// Exports are zero-rated and carry no fiscal stamp.
	currency, countryCode string
}

var businessClients = []businessClient{
	{name: "Hôtel Les Palmiers", city: "Hammamet", termsDays: 60, contract: 45000,
		kinds: []string{"Climatiseur", "Téléviseur", "Réfrigérateur", "Chauffe-eau", "Climatisation professionnelle"}},
	{name: "Clinique El Amen", city: "Nabeul", termsDays: 45, withholding: 1, contract: 32000,
		kinds: []string{"Climatiseur", "Réfrigérateur", "Fontaine à eau", "Chauffe-eau", "Climatisation professionnelle"}},
	{name: "École privée Ibn Khaldoun", city: "Nabeul", termsDays: 30, withholding: 1,
		kinds: []string{"Climatiseur", "Fontaine à eau", "Ventilateur"}},
	{name: "Café-Restaurant Le Golfe", city: "Hammamet", termsDays: 30,
		kinds: []string{"Réfrigérateur", "Congélateur", "Four à micro-ondes", "Cafetière", "Lave-vaisselle"}},
	{name: "Résidence Les Jasmins", city: "Nabeul", termsDays: 30,
		kinds: []string{"Chauffe-eau", "Climatiseur", "Cuisinière gaz/électrique"}},
	{name: "Société Sahel Logistique", city: "Sousse", termsDays: 60, withholding: 1,
		kinds: []string{"Climatiseur", "Fontaine à eau", "Four à micro-ondes", "Réfrigérateur", "Climatisation professionnelle"}},
	{name: "Cabinet d'architecture Ben Ammar", city: "Tunis", termsDays: 30,
		kinds: []string{"Climatiseur", "Cafetière", "Fontaine à eau"}},
	{name: "Restaurant Dar Zitoun", city: "Hammamet", termsDays: 45,
		kinds: []string{"Congélateur", "Réfrigérateur", "Cuisinière gaz/électrique", "Lave-vaisselle"}},
	{name: "Société Libyenne d'Équipement Hôtelier", city: "Tripoli", termsDays: 60, currency: "EUR", countryCode: "LY",
		kinds: []string{"Climatiseur", "Réfrigérateur", "Téléviseur", "Climatisation professionnelle"}},
}

type businessClientRef struct {
	businessClient
	id string
}

// setupBusinessClients creates the business customers, each with its tax
// number (MF), address and payment terms, and the maintenance contract
// service they're billed for.
func (s *Seeder) setupBusinessClients() error {
	for _, bc := range businessClients {
		req := db.CreateClientRequest{
			OrganizationID:  s.orgID,
			Name:            strPtr(bc.name),
			Vatin:           strPtr(s.rng.VATIN()),
			Phone:           strPtr(s.rng.Phone()),
			Street:          strPtr(s.rng.Street()),
			HouseNumber:     strPtr(fmt.Sprintf("%d", s.rng.IntRange(1, 120))),
			City:            strPtr(bc.city),
			CountryCode:     nonEmptyStrPtr(orDefault(bc.countryCode, s.orgProfile.countryCode)),
			DefaultCurrency: nonEmptyStrPtr(bc.currency),
			Emails:          strPtr(fmt.Sprintf(`["%s"]`, s.rng.Email(bc.name))),
		}
		var cl db.Client
		if err := s.c.Post("/api/clients", req, &cl); err != nil {
			return fmt.Errorf("business client %q: %w", bc.name, err)
		}
		s.businessClients = append(s.businessClients, businessClientRef{businessClient: bc, id: cl.ID})
		s.stats.Clients++
	}
	var p db.Product
	if err := s.c.Post("/api/products", db.CreateProductRequest{
		OrganizationID: s.orgID, Name: "Contrat d'entretien climatisation (forfait mensuel)", Type: "service",
		SKU: strPtr("CONTRAT-ENT"), Price: 40000, TaxRateID: strPtr(s.standardTax.id),
	}, &p); err != nil {
		return fmt.Errorf("maintenance contract product: %w", err)
	}
	s.contractProductID = p.ID
	return nil
}

// maybeStartBusinessOrder takes a business order now and then: about one a
// week on average, more in the spring when hotels and offices fit out for the
// summer, and more as the business grows.
func (s *Seeder) maybeStartBusinessOrder(day time.Time) error {
	if len(s.businessClients) == 0 {
		return nil
	}
	chance := 0.18 * s.growthFactor(day)
	if m := day.Month(); m >= time.March && m <= time.June {
		chance *= 1.6
	}
	if !s.rng.Chance(chance) {
		return nil
	}
	return s.createBusinessOrder(day, Pick(s.rng, s.businessClients))
}

// businessLine is one appliance of a business order, with its order line id
// once the order exists.
type businessLine struct {
	product     productRef
	quantity    float64
	orderLineID string
	// unitPrice is in the order's currency (cents): the catalog price, or
	// for an export client that price converted at the order's rate.
	unitPrice int64
}

// exportRate is the order's TND-per-unit rate for an export client (1 unit
// of the client's currency = rate dinars), 0 for a Tunisian client.
func (s *Seeder) exportRate(client businessClientRef) float64 {
	if client.currency == "" {
		return 0
	}
	return math.Round(s.rng.Float64Range(3.30, 3.45)*10000) / 10000
}

func (s *Seeder) createBusinessOrder(day time.Time, client businessClientRef) error {
	// Two or three of the appliance kinds the client buys, several units each,
	// as many as are on the shelf.
	rate := s.exportRate(client)
	var lines []businessLine
	kinds := append([]string(nil), client.kinds...)
	Shuffle(s.rng, kinds)
	for _, kind := range kinds[:min(len(kinds), s.rng.IntRange(1, 3))] {
		var candidates []productRef
		for _, p := range s.stockProducts() {
			if p.kind == kind && s.onHand(p.id) >= 2 {
				candidates = append(candidates, p)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		p := Pick(s.rng, candidates)
		qty := math.Min(float64(s.rng.IntRange(2, 6)), s.onHand(p.id))
		s.adjustOnHand(p.id, -qty) // reserved now; the delivery takes it out server-side
		price := p.priceCents
		if rate > 0 {
			price = int64(math.Round(float64(p.priceCents) / rate))
		}
		lines = append(lines, businessLine{product: p, quantity: qty, unitPrice: price})
	}
	if len(lines) == 0 {
		return nil // nothing the client wants is in stock today
	}

	items := make([]db.CreateOrderLineItemRequest, len(lines))
	for i, l := range lines {
		items[i] = db.CreateOrderLineItemRequest{
			ProductID: strPtr(l.product.id), Description: l.product.name,
			Quantity: l.quantity, UnitPrice: float64(l.unitPrice),
		}
	}
	orderReq := db.CreateOrderRequest{
		OrganizationID: s.orgID, ClientID: &client.id, Status: "draft",
		OrderDate: midnightUTC(day), LineItems: items,
	}
	if rate > 0 {
		orderReq.Currency = strPtr(client.currency)
		orderReq.ExchangeRate = float64Ptr(rate)
	}
	var order db.Order
	if err := s.c.Post("/api/orders", orderReq, &order); err != nil {
		return fmt.Errorf("business order for %s: %w", client.name, err)
	}
	s.stats.Orders++
	if err := s.c.Patch("/api/orders/"+order.ID+"/status", map[string]string{"status": "confirmed"}, nil); err != nil {
		return fmt.Errorf("confirm order %s: %w", order.OrderNumber, err)
	}
	var orderLines []db.OrderLineItem
	if err := s.c.Get("/api/orders/"+order.ID+"/line-items", &orderLines); err != nil {
		return fmt.Errorf("read back order %s: %w", order.OrderNumber, err)
	}
	for i := range lines {
		lines[i].orderLineID = orderLines[i].ID
	}

	// A few orders fall through before delivery: the stock goes back.
	if s.rng.Chance(0.06) {
		cancelDay := businessDaysLater(day, s.rng.IntRange(1, 5))
		if cancelDay.After(s.cfg.EndDate) {
			return nil
		}
		s.sched.Schedule(cancelDay, func() error {
			if err := s.c.Patch("/api/orders/"+order.ID+"/status", map[string]string{"status": "cancelled"}, nil); err != nil {
				return fmt.Errorf("cancel order %s: %w", order.OrderNumber, err)
			}
			for _, l := range lines {
				s.adjustOnHand(l.product.id, l.quantity)
			}
			return nil
		})
		return nil
	}

	shipDay := businessDaysLater(day, s.rng.IntRange(2, 6))
	if shipDay.After(s.cfg.EndDate) {
		return nil // still to deliver when the run ends: an open order
	}
	if s.rng.Chance(0.3) && anyMoreThanOne(lines) {
		// Delivered in two goes: about half now, the rest a week or two later.
		first := make([]businessLine, 0, len(lines))
		rest := make([]businessLine, 0, len(lines))
		for _, l := range lines {
			now := math.Ceil(l.quantity / 2)
			first = append(first, businessLine{product: l.product, quantity: now, orderLineID: l.orderLineID, unitPrice: l.unitPrice})
			if l.quantity-now > 0 {
				rest = append(rest, businessLine{product: l.product, quantity: l.quantity - now, orderLineID: l.orderLineID, unitPrice: l.unitPrice})
			}
		}
		secondDay := businessDaysLater(shipDay, s.rng.IntRange(5, 12))
		s.sched.Schedule(shipDay, func() error {
			return s.deliverBusinessOrder(shipDay, order, client, first, false, rate)
		})
		if !secondDay.After(s.cfg.EndDate) {
			s.sched.Schedule(secondDay, func() error {
				return s.deliverBusinessOrder(secondDay, order, client, rest, true, rate)
			})
		}
		return nil
	}
	s.sched.Schedule(shipDay, func() error { return s.deliverBusinessOrder(shipDay, order, client, lines, true, rate) })
	return nil
}

func anyMoreThanOne(lines []businessLine) bool {
	for _, l := range lines {
		if l.quantity > 1 {
			return true
		}
	}
	return false
}

// deliverBusinessOrder ships one delivery of the order, marks it delivered
// the next business day, and invoices what it carried. last marks the
// order's final delivery, which moves the order itself to shipped and then
// delivered.
func (s *Seeder) deliverBusinessOrder(day time.Time, order db.Order, client businessClientRef, lines []businessLine, last bool, rate float64) error {
	items := make([]db.CreateDeliveryLineItemRequest, len(lines))
	for i, l := range lines {
		items[i] = db.CreateDeliveryLineItemRequest{
			OrderLineItemID: strPtr(l.orderLineID), ProductID: strPtr(l.product.id),
			Description: l.product.name, Quantity: l.quantity,
		}
	}
	var delivery db.OutboundDelivery
	if err := s.c.Post("/api/deliveries", db.CreateDeliveryRequest{
		OrganizationID: s.orgID, OrderID: &order.ID, DeliveryDate: midnightUTC(day), LineItems: items,
	}, &delivery); err != nil {
		return fmt.Errorf("delivery for order %s: %w", order.OrderNumber, err)
	}
	s.stats.Deliveries++
	serials, err := s.deliverySerialNumbers(delivery)
	if err != nil {
		return err
	}
	ship := map[string]any{"status": "shipped"}
	if len(serials) > 0 {
		ship["serialNumbers"] = serials
	}
	if err := s.c.Patch("/api/deliveries/"+delivery.ID+"/status", ship, nil); err != nil {
		return fmt.Errorf("ship delivery %s: %w", delivery.DeliveryNumber, err)
	}
	if last {
		if err := s.c.Patch("/api/orders/"+order.ID+"/status", map[string]string{"status": "shipped"}, nil); err != nil {
			return fmt.Errorf("mark order %s shipped: %w", order.OrderNumber, err)
		}
	}
	arrived := businessDaysLater(day, 1)
	if !arrived.After(s.cfg.EndDate) {
		s.sched.Schedule(arrived, func() error {
			if err := s.c.Patch("/api/deliveries/"+delivery.ID+"/status", map[string]string{"status": "delivered"}, nil); err != nil {
				return fmt.Errorf("mark delivery %s delivered: %w", delivery.DeliveryNumber, err)
			}
			if last {
				if err := s.c.Patch("/api/orders/"+order.ID+"/status", map[string]string{"status": "delivered"}, nil); err != nil {
					return fmt.Errorf("mark order %s delivered: %w", order.OrderNumber, err)
				}
			}
			return nil
		})
	}

	invLines := make([]db.CreateInvoiceLineItemRequest, len(lines))
	totals := make([]lineItem, len(lines))
	for i, l := range lines {
		// An export is zero-rated.
		tax := l.product.taxRateID
		if rate > 0 {
			tax = s.zeroTax.id
		}
		invLines[i] = db.CreateInvoiceLineItemRequest{
			Description: strPtr(l.product.name), Quantity: l.quantity, UnitPrice: float64(l.unitPrice),
			TaxRate: strPtr(tax), ProductID: strPtr(l.product.id),
		}
		totals[i] = lineItem{quantity: l.quantity, unitPriceCents: l.unitPrice, taxPercent: s.taxPercentFor(tax)}
	}
	note := fmt.Sprintf("Livraison %s — commande %s", delivery.DeliveryNumber, order.OrderNumber)
	return s.issueBusinessInvoice(day, client, invLines, totals, note, true, rate)
}

// deliverySerialNumbers picks, for every serialized line of a delivery, that
// many units currently in stock (oldest first), keyed by the delivery's line
// id as PATCH .../status takes them. Nil when no line is serialized.
func (s *Seeder) deliverySerialNumbers(delivery db.OutboundDelivery) (map[string][]string, error) {
	var lines []db.OutboundDeliveryLineItem
	if err := s.c.Get("/api/deliveries/"+delivery.ID+"/line-items", &lines); err != nil {
		return nil, fmt.Errorf("read back delivery %s line items: %w", delivery.DeliveryNumber, err)
	}
	var out map[string][]string
	for _, l := range lines {
		if l.ProductID == nil {
			continue
		}
		p, ok := s.productByID(*l.ProductID)
		if !ok || !p.serialized {
			continue
		}
		var units []db.SerialNumber
		if err := s.c.Get("/api/products/"+p.id+"/serial-numbers", &units); err != nil {
			return nil, fmt.Errorf("serial numbers of %s: %w", p.name, err)
		}
		if out == nil {
			out = map[string][]string{}
		}
		for _, u := range units {
			if len(out[l.ID]) == int(l.Quantity) {
				break
			}
			if u.InStock == 1 {
				out[l.ID] = append(out[l.ID], u.SerialNumber)
			}
		}
		if len(out[l.ID]) < int(l.Quantity) {
			return nil, fmt.Errorf("delivery %s: %s has %d units in stock, %v to ship", delivery.DeliveryNumber, p.name, len(out[l.ID]), l.Quantity)
		}
	}
	return out, nil
}

// maybeBillContracts invoices each maintenance contract on the first business
// day of the month.
func (s *Seeder) maybeBillContracts(day time.Time) error {
	if !day.Equal(businessDaysLater(time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC), 0)) {
		return nil
	}
	for _, client := range s.businessClients {
		if client.contract == 0 {
			continue
		}
		description := fmt.Sprintf("Contrat d'entretien climatisation — %s %d", frenchMonths[day.Month()], day.Year())
		lines := []db.CreateInvoiceLineItemRequest{{
			Description: strPtr(description), Quantity: 1, UnitPrice: float64(client.contract),
			TaxRate: strPtr(s.standardTax.id), ProductID: strPtr(s.contractProductID),
		}}
		totals := []lineItem{{quantity: 1, unitPriceCents: client.contract, taxPercent: s.standardTax.percent}}
		if err := s.issueBusinessInvoice(day, client, lines, totals, "", false, 0); err != nil {
			return err
		}
	}
	return nil
}

// issueBusinessInvoice creates, sends and schedules the payment of one
// business invoice. allowDiscount lets an appliance invoice carry a remise;
// a contract invoice is at its agreed price.
func (s *Seeder) issueBusinessInvoice(day time.Time, client businessClientRef, lines []db.CreateInvoiceLineItemRequest, totals []lineItem, note string, allowDiscount bool, rate float64) error {
	var discount int64
	if allowDiscount && s.rng.Chance(0.5) {
		gross, _, _ := computeTotals(totals)
		discount = gross * int64(s.rng.IntRange(3, 8)) / 100
	}
	// An export carries no fiscal stamp, and is in the client's currency at
	// the order's rate.
	stamp := int64(fiscalStampCents)
	currency := s.cfg.Currency
	var exchangeRate *float64
	if rate > 0 {
		stamp, currency, exchangeRate = 0, client.currency, float64Ptr(rate)
	}
	subTotal, taxTotal, total := computeTotalsWithDiscount(totals, discount, stamp)
	dueDate := midnightUTC(day.AddDate(0, 0, client.termsDays))
	req := db.CreateInvoiceRequest{
		OrganizationID:    s.orgID,
		Number:            s.invoiceNum.next(day.Year()),
		State:             "draft",
		ClientID:          client.id,
		Date:              midnightUTC(day),
		DueDate:           &dueDate,
		Currency:          currency,
		ExchangeRate:      exchangeRate,
		Total:             total,
		TaxTotal:          taxTotal,
		SubTotal:          subTotal,
		LineItems:         lines,
		PaymentTerms:      strPtr(fmt.Sprintf("Paiement à %d jours", client.termsDays)),
		CustomerNotes:     nonEmptyStrPtr(note),
		FiscalStampAmount: stamp,
		DiscountAmount:    discount,
	}
	// The withholding (retenue à la source) is informational on the invoice
	// (db/invoice.go): the client keeps it back and hands over a tax
	// certificate for it, and the payment recorded settles the whole total.
	if client.withholding > 0 {
		withheld := int64(math.Round(float64(total-stamp) * client.withholding / 100))
		req.WithholdingTaxRate = float64Ptr(client.withholding)
		req.WithholdingTaxAmount = int64Ptr(withheld)
	}

	// The last days leave a couple of invoices in draft: being prepared,
	// not sent yet.
	draft := s.cfg.EndDate.Sub(day) < 4*24*time.Hour && s.stats.DraftInvoices < 2
	var inv db.Invoice
	if err := s.c.Post("/api/invoices", req, &inv); err != nil {
		return fmt.Errorf("invoice for %s: %w", client.name, err)
	}
	s.stats.Invoices++
	if draft {
		s.stats.DraftInvoices++
		return nil
	}
	if err := s.c.Patch("/api/invoices/"+inv.ID+"/state", map[string]string{"state": "sent"}, nil); err != nil {
		return fmt.Errorf("send invoice %s: %w", inv.Number, err)
	}
	if note == "" && s.stats.CancelledInvoices == 0 && s.rng.Chance(0.05) {
		// Issued to the wrong client: cancelled the same day and issued again.
		if err := s.c.Patch("/api/invoices/"+inv.ID+"/state", map[string]string{"state": "cancelled"}, nil); err != nil {
			return fmt.Errorf("cancel invoice %s: %w", inv.Number, err)
		}
		s.stats.CancelledInvoices++
		return s.issueBusinessInvoice(day, client, lines, totals, note, allowDiscount, rate)
	}

	terms := client.termsDays
	pay := func(at time.Time, amount int64) {
		// An export client's transfer arrives at that day's rate, a little
		// off the invoice's, so the bank posts a realized exchange gain or
		// loss (db/payment.go).
		var payRate *float64
		if rate > 0 {
			payRate = float64Ptr(math.Round(rate*s.rng.Float64Range(0.98, 1.02)*10000) / 10000)
		}
		s.schedulePayment(at, inv.ID, amount, client.id, "invoice", currency, payRate, s.cashAccountID)
	}
	switch {
	case s.rng.Chance(0.72): // around the due date
		pay(businessDaysLater(day, terms+s.rng.IntRange(-12, 10)), total)
	case s.rng.Chance(0.45): // in two parts
		first := total / 2
		firstDay := businessDaysLater(day, s.rng.IntRange(terms/2, terms))
		pay(firstDay, first)
		pay(businessDaysLater(firstDay, s.rng.IntRange(15, 40)), total-first)
	case s.rng.Chance(0.85): // late
		pay(businessDaysLater(day, terms+s.rng.IntRange(30, 100)), total)
	default:
		// Disputed and never paid.
	}
	return nil
}
