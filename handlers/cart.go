package handlers

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type CartItem struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	Price     string  `json:"price"`
	Image     string  `json:"image"`
	Quantity  int     `json:"quantity"`
	Subtotal  string  `json:"subtotal"`
	Stock     int     `json:"stock"`
	Available bool    `json:"available"`
	Reason    *string `json:"reason"`
}

type Cart struct {
	Items          []CartItem `json:"items"`
	ItemCount      int        `json:"item_count"`
	Subtotal       string     `json:"subtotal"`
	HasUnavailable bool       `json:"has_unavailable"`
	PodAvailable   bool       `json:"pod_available"`
}

// decodeCart handles both key styles — the backend has returned
// snake_case in docs and Capitalized in reality (see /api/products).
func decodeCart(raw []byte) (Cart, error) {
	var c Cart
	if bytes.Contains(raw, []byte(`"item_count"`)) {
		return c, json.Unmarshal(raw, &c)
	}

	// Capitalized fallback
	var alt struct {
		Items []struct {
			ProductID string  `json:"ProductID"`
			Name      string  `json:"Name"`
			Price     string  `json:"Price"`
			Image     string  `json:"Image"`
			Quantity  int     `json:"Quantity"`
			Subtotal  string  `json:"Subtotal"`
			Stock     int     `json:"Stock"`
			Available bool    `json:"Available"`
			Reason    *string `json:"Reason"`
		} `json:"Items"`
		ItemCount      int    `json:"ItemCount"`
		Subtotal       string `json:"Subtotal"`
		HasUnavailable bool   `json:"HasUnavailable"`
		PodAvailable   bool   `json:"PodAvailable"`
	}
	if err := json.Unmarshal(raw, &alt); err != nil {
		return c, err
	}
	for _, it := range alt.Items {
		c.Items = append(c.Items, CartItem(it))
	}
	c.ItemCount = alt.ItemCount
	c.Subtotal = alt.Subtotal
	c.HasUnavailable = alt.HasUnavailable
	c.PodAvailable = alt.PodAvailable
	return c, nil
}

// authedBackendCall proxies to the backend with the user's Bearer cookie.
// Returns the raw body and status code.
func authedBackendCall(r *http.Request, method, path string, payload any) ([]byte, int, error) {
	cookie, err := r.Cookie("access_token")
	if err != nil {
		return nil, http.StatusUnauthorized, nil
	}

	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewBuffer(b)
	}

	req, _ := http.NewRequest(method, BackendURL+path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cookie.Value)

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, err
}

func fetchCart(r *http.Request) (Cart, int, error) {
	raw, status, err := authedBackendCall(r, "GET", "/api/cart", nil)
	if err != nil || status != http.StatusOK {
		return Cart{}, status, err
	}
	cart, err := decodeCart(raw)
	return cart, status, err
}

func fetchCartCount(r *http.Request) int {
	cart, status, err := fetchCart(r)
	if err != nil || status != http.StatusOK {
		return 0
	}
	return cart.ItemCount
}

// Render the cart contents partial into HTML (shared by page + mutations)
func renderCartContents(cart Cart) template.HTML {
	var buf bytes.Buffer
	tmpl := parseWithFuncs("templates/partials/cart-contents.html")
	tmpl.Execute(&buf, cart)
	return template.HTML(buf.String())
}

// Cart mutation responses: swap contents + notify the header badge
func writeCartMutation(w http.ResponseWriter, cart Cart) {
	w.Header().Set("HX-Trigger", `{"cartUpdated": {"count": `+itoa(cart.ItemCount)+`}}`)
	w.Write([]byte(renderCartContents(cart)))
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// AddToCart — the product page form posts here
func AddToCart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	quantity, err := strconv.Atoi(r.FormValue("quantity"))
	if err != nil || quantity < 1 {
		quantity = 1
	}

	raw, status, err := authedBackendCall(r, "POST", "/api/cart/items", map[string]any{
		"product_id": r.FormValue("product_id"),
		"quantity":   quantity,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<span style="color: var(--danger);">Unable to connect to backend server.</span>`))
		return
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
		w.Write([]byte(`<span style="color: var(--danger);">` + template.HTMLEscapeString(backendErrorMessage(raw, "Couldn't add to cart.")) + `</span>`))
		return
	}

	cart, err := decodeCart(raw)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<span style="color: var(--danger);">Added, but couldn't read the cart back.</span>`))
		return
	}

	w.Header().Set("HX-Trigger", `{"cartUpdated": {"count": `+itoa(cart.ItemCount)+`}}`)
	w.Write([]byte(`<span style="color: #4ade80;">✓ Added to cart</span>`))
}

// Extract a readable message from a backend {"error": "..."} body
func backendErrorMessage(raw []byte, fallback string) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != "" {
		return e.Error
	}
	return fallback
}

// CartHandler — GET renders the page, DELETE clears it
func CartHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
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

		data := struct {
			PageData
			Contents template.HTML
		}{
			PageData: PageData{IsLoggedIn: true, User: user, CartCount: cart.ItemCount},
			Contents: renderCartContents(cart),
		}

		tmpl := parseWithFuncs("templates/layout.html", "templates/cart.html")
		tmpl.Execute(w, data)

	case http.MethodDelete:
		raw, status, err := authedBackendCall(r, "DELETE", "/api/cart", nil)
		if err != nil || status != http.StatusOK {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`<p style="color: var(--danger);">Couldn't clear the cart.</p>`))
			return
		}
		cart, _ := decodeCart(raw)
		writeCartMutation(w, cart)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// CartItemHandler — PUT sets quantity, DELETE removes (id in path)
func CartItemHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/cart/items/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	var (
		raw    []byte
		status int
		err    error
	)

	switch r.Method {
	case http.MethodPut:
		quantity, convErr := strconv.Atoi(r.FormValue("quantity"))
		if convErr != nil || quantity < 1 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`<p style="color: var(--danger);">Quantity must be at least 1.</p>`))
			return
		}
		raw, status, err = authedBackendCall(r, "PUT", "/api/cart/items/"+id, map[string]any{"quantity": quantity})

	case http.MethodDelete:
		raw, status, err = authedBackendCall(r, "DELETE", "/api/cart/items/"+id, nil)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<p style="color: var(--danger);">Unable to connect to backend server.</p>`))
		return
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
		w.Write([]byte(`<p style="color: var(--danger);">` + template.HTMLEscapeString(backendErrorMessage(raw, "Cart update failed.")) + `</p>`))
		return
	}

	cart, err := decodeCart(raw)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<p style="color: var(--danger);">Updated, but couldn't read the cart back.</p>`))
		return
	}
	writeCartMutation(w, cart)
}
