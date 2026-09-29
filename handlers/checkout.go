package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
)


// IsEmpty helps templates decide whether to show "add address" prompt.
func (a Address) IsEmpty() bool {
	return a.Country == "" && a.State == "" && a.LGA == "" && a.Town == "" && a.Street == ""
}

func (a Address) OneLine() string {
	parts := []string{}
	if a.Street != "" {
		parts = append(parts, a.Street)
	}
	if a.Town != "" {
		parts = append(parts, a.Town)
	}
	if a.LGA != "" {
		parts = append(parts, a.LGA)
	}
	if a.State != "" {
		parts = append(parts, a.State)
	}
	if a.Country != "" {
		parts = append(parts, a.Country)
	}
	return strings.Join(parts, ", ")
}


type BankDetails struct {
	BankName      string `json:"bank_name"`
	AccountName   string `json:"account_name"`
	AccountNumber string `json:"account_number"`
}

type OrderItem struct {
	ProductID string `json:"product_id"`
	Name      string `json:"name"`
	Price     string `json:"price"`
	Quantity  int    `json:"quantity"`
	Subtotal  string `json:"subtotal"`
}

type OrderTimeline struct {
	CreatedAt   *string `json:"created_at"`
	ApprovedAt  *string `json:"approved_at"`
	ShippedAt   *string `json:"shipped_at"`
	DeliveredAt *string `json:"delivered_at"`
	CancelledAt *string `json:"cancelled_at"`
	RejectedAt  *string `json:"rejected_at"`
}

type Order struct {
	ID              string         `json:"id"`
	OrderNumber     int            `json:"order_number"`
	OrderCode       string         `json:"order_code"`
	Status          string         `json:"status"`
	PaymentMethod   string         `json:"payment_method"`
	Subtotal        string         `json:"subtotal"`
	Items           []OrderItem    `json:"items"`
	ShippingAddress Address        `json:"shipping_address"`
	BankDetails     *BankDetails   `json:"bank_details"`
	PickupCode      string         `json:"pickup_code"`
	CancelReason    string         `json:"cancel_reason"`
	RejectReason    string         `json:"reject_reason"`
	Timeline        OrderTimeline  `json:"timeline"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
}

// IsTerminal returns true when polling should stop.
func (o Order) IsTerminal() bool {
	switch o.Status {
	case "delivered", "cancelled", "rejected":
		return true
	}
	return false
}

// IsPendingBankTransfer matches the exact combination that gets the "I've paid" button.
func (o Order) IsPendingBankTransfer() bool {
	return o.Status == "pending" && o.PaymentMethod == "bank_transfer"
}

func (o Order) CanCancel() bool {
	return o.Status == "pending"
}

// ---------- Backend helpers ----------

// authedBackendRaw is like authedBackendCall but always returns raw body+status.
// (You already have authedBackendCall — reuse it. This wrapper exists for clarity.)

func fetchMe(r *http.Request) (UserProfile, int, error) {
	raw, status, err := authedBackendCall(r, "GET", "/api/me", nil)
	if err != nil || status != http.StatusOK {
		return UserProfile{}, status, err
	}
	var u UserProfile
	err = json.Unmarshal(raw, &u)
	return u, status, err
}

func fetchOrder(r *http.Request, id string) (Order, int, error) {
	raw, status, err := authedBackendCall(r, "GET", "/api/orders/"+id, nil)
	if err != nil || status != http.StatusOK {
		return Order{}, status, err
	}
	var o Order
	err = json.Unmarshal(raw, &o)
	return o, status, err
}

// uuidv4 produces a random v4 UUID using crypto/rand.
func uuidv4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ---------- Page: /checkout ----------

func CheckoutPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	isLoggedIn, user := fetchUserSession(r)
	if !isLoggedIn {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	cart, status, err := fetchCart(r)
	if err != nil || status != http.StatusOK {
		http.Error(w, "Couldn't load cart", http.StatusBadGateway)
		return
	}

	// Empty cart → back to cart page
	if len(cart.Items) == 0 {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	me, _, _ := fetchMe(r)

	// One client_token per checkout attempt. If the browser refreshes the
	// review page, a new token is generated. But we stash the one we hand to
	// the template in a cookie so that if the POST is retried (e.g. user
	// double-clicks) we reuse the same token. The form includes it as a hidden
	// field anyway, so the safest route is: generate here, ship it in HTML.
	token := uuidv4()

	data := struct {
		PageData
		Cart        Cart
		Me          UserProfile
		Address     *Address
		ClientToken string
	}{
		PageData:    PageData{IsLoggedIn: true, User: user, CartCount: cart.ItemCount},
		Cart:        cart,
		Me:          me,
		Address:     me.Address,
		ClientToken: token,
	}

	tmpl := parseWithFuncs("templates/layout.html", "templates/checkout.html")
	tmpl.Execute(w, data)
}

// ---------- Action: POST /checkout ----------

func PlaceOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := r.FormValue("client_token")
	if token == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`<div class="form-error">Missing idempotency token. Please reload the page.</div>`))
		return
	}

	paymentMethod := r.FormValue("payment_method")
	if paymentMethod != "bank_transfer" && paymentMethod != "pay_on_delivery" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`<div class="form-error">Please choose a payment method.</div>`))
		return
	}

	raw, status, err := authedBackendCall(r, "POST", "/api/checkout", map[string]any{
		"client_token":   token,
		"payment_method": paymentMethod,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<div class="form-error">Unable to connect to backend server.</div>`))
		return
	}

	// 201 = new, 200 = idempotent replay. Both are success.
	if status != http.StatusCreated && status != http.StatusOK {
		msg := backendErrorMessage(raw, "Checkout failed.")
		w.WriteHeader(status)
		// Return an OOB swap that fills the error container on the review page.
		w.Write([]byte(`<div class="form-error" id="checkout-error">` + template.HTMLEscapeString(msg) + `</div>`))
		return
	}

	var order Order
	if err := json.Unmarshal(raw, &order); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<div class="form-error">Order placed, but response was unreadable.</div>`))
		return
	}

	// Success: tell HTMX to navigate. HX-Redirect is a full browser navigation.
	w.Header().Set("HX-Redirect", "/orders/"+order.ID)
	w.WriteHeader(http.StatusOK)
}

// ---------- Page: /orders/{id} ----------

// ---------- Page: /orders/{id} ----------

func OrderDetailPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	isLoggedIn, user := fetchUserSession(r)
	if !isLoggedIn {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/orders/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	order, status, err := fetchOrder(r, id)
	if err != nil || status != http.StatusOK {
		http.Error(w, "Couldn't load order", http.StatusBadGateway)
		return
	}

	cartCount := fetchCartCount(r)

	data := struct {
		PageData
		Order Order
	}{
		PageData: PageData{IsLoggedIn: true, User: user, CartCount: cartCount},
		Order:    order,
	}

	tmpl := parseWithFuncs(
		"templates/layout.html",
		"templates/order-detail.html",
		"templates/partials/order-panel.html",
	)
	if err := tmpl.Execute(w, data); err != nil {
		// Log to server so silent template errors stop being silent.
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// ---------- Page: /orders (list) ----------

type orderListItem struct {
	ID            string `json:"id"`
	OrderNumber   int    `json:"order_number"`
	OrderCode     string `json:"order_code"`
	Status        string `json:"status"`
	PaymentMethod string `json:"payment_method"`
	Subtotal      string `json:"subtotal"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func OrdersListPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	isLoggedIn, user := fetchUserSession(r)
	if !isLoggedIn {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	raw, status, err := authedBackendCall(r, "GET", "/api/orders?limit=20&offset=0", nil)
	if err != nil || status != http.StatusOK {
		http.Error(w, "Couldn't load orders", http.StatusBadGateway)
		return
	}

	var resp struct {
		Orders []orderListItem `json:"orders"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		http.Error(w, "Couldn't read orders", http.StatusBadGateway)
		return
	}

	cartCount := fetchCartCount(r)

	data := struct {
		PageData
		Orders []orderListItem
	}{
		PageData: PageData{IsLoggedIn: true, User: user, CartCount: cartCount},
		Orders:   resp.Orders,
	}

	tmpl := parseWithFuncs("templates/layout.html", "templates/orders.html")
	tmpl.Execute(w, data)
}

// ---------- Action: POST /orders/{id}/mark-paid ----------

func MarkOrderPaid(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Path: /orders/{id}/mark-paid
	trimmed := strings.TrimPrefix(r.URL.Path, "/orders/")
	id := strings.TrimSuffix(trimmed, "/mark-paid")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	raw, status, err := authedBackendCall(r, "POST", "/api/orders/"+id+"/mark-paid", nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<div class="form-error">Unable to connect to backend server.</div>`))
		return
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
		w.Write([]byte(`<div class="form-error">` + template.HTMLEscapeString(backendErrorMessage(raw, "Couldn't mark as paid.")) + `</div>`))
		return
	}

	var order Order
	if err := json.Unmarshal(raw, &order); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<div class="form-error">Updated, but couldn't read the order back.</div>`))
		return
	}

	// Render just the order panel, so the page updates in place.
	renderOrderPanel(w, order, r)
}

// ---------- Action: POST /orders/{id}/cancel ----------

func CancelOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	trimmed := strings.TrimPrefix(r.URL.Path, "/orders/")
	id := strings.TrimSuffix(trimmed, "/cancel")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	reason := r.FormValue("reason") // optional

	payload := map[string]any{}
	if reason != "" {
		payload["reason"] = reason
	}

	raw, status, err := authedBackendCall(r, "POST", "/api/orders/"+id+"/cancel", payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<div class="form-error">Unable to connect to backend server.</div>`))
		return
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
		w.Write([]byte(`<div class="form-error">` + template.HTMLEscapeString(backendErrorMessage(raw, "Couldn't cancel the order.")) + `</div>`))
		return
	}

	var order Order
	if err := json.Unmarshal(raw, &order); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<div class="form-error">Cancelled, but couldn't read the order back.</div>`))
		return
	}

	renderOrderPanel(w, order, r)
}

// renderOrderPanel executes just the order-panel block and writes it out.
// Uses the same template set as OrderDetailPage so the two can't drift.
func renderOrderPanel(w http.ResponseWriter, order Order, r *http.Request) {
	tmpl := parseWithFuncs(
		"templates/layout.html",
		"templates/order-detail.html",
		"templates/partials/order-panel.html",
	)
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "order-panel", order); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<div class="form-error">Template error: ` + template.HTMLEscapeString(err.Error()) + `</div>`))
		return
	}
	w.Write(buf.Bytes())
}

// ---------- Action: GET /orders/{id}/panel (polling) ----------

func OrderPanel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// /orders/{id}/panel
	trimmed := strings.TrimPrefix(r.URL.Path, "/orders/")
	id := strings.TrimSuffix(trimmed, "/panel")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	order, status, err := fetchOrder(r, id)
	if err != nil || status != http.StatusOK {
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	renderOrderPanel(w, order, r)
}

// ---------- Action: PUT /profile/address (extend existing) ----------

// If you already have UpdateAddress, keep it. This is a reference for the
// shape the checkout review page expects: after PUT, redirect back to /checkout.
func UpdateAddressFromCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	addr := Address{
		Country: r.FormValue("country"),
		State:   r.FormValue("state"),
		LGA:     r.FormValue("lga"),
		Town:    r.FormValue("town"),
		Street:  r.FormValue("street"),
	}

	raw, status, err := authedBackendCall(r, "PUT", "/api/me/address", addr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<div class="form-error">Unable to connect to backend server.</div>`))
		return
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
		w.Write([]byte(`<div class="form-error">` + template.HTMLEscapeString(backendErrorMessage(raw, "Couldn't save address.")) + `</div>`))
		return
	}

	// HTMX redirect back to checkout review
	w.Header().Set("HX-Redirect", "/checkout")
	w.WriteHeader(http.StatusOK)
}
