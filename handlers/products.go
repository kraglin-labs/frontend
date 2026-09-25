package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

const productPageSize = 20

type Product struct {
	ID          string   `json:"ID"`
	Name        string   `json:"Name"`
	Description string   `json:"Description"`
	Price       string   `json:"Price"`
	Stock       int      `json:"Stock"`
	Category    string   `json:"Category"`
	Images      []string `json:"Images"`
	PodEligible bool     `json:"PodEligible"`
	Active      bool     `json:"Active"`
	CreatedAt   string   `json:"CreatedAt"`
	UpdatedAt   string   `json:"UpdatedAt"`
}

type productListResponse struct {
	Products []Product `json:"products"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
}

// formatMoney turns "4999.99" into "₦4,999.99"
func formatMoney(s string) string {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return "₦" + s
	}
	neg := f < 0
	f = math.Abs(f)
	intPart := int64(f)
	frac := int64(math.Round((f - float64(intPart)) * 100))

	digits := strconv.FormatInt(intPart, 10)
	var grouped string
	for len(digits) > 3 {
		grouped = "," + digits[len(digits)-3:] + grouped
		digits = digits[:len(digits)-3]
	}
	grouped = digits + grouped

	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s₦%s.%02d", sign, grouped, frac)
}

// parseWithFuncs parses templates with the shared helper functions.
func parseWithFuncs(files ...string) *template.Template {
	root := filepath.Base(files[0])
	return template.Must(template.New(root).Funcs(template.FuncMap{
		"money": formatMoney,
		"add":   func(a, b int) int { return a + b },
		"sub":   func(a, b int) int { return a - b },
	}).ParseFiles(files...))
}

// Templates parsed once at startup, not per request.
var productCardTmpl = parseWithFuncs("templates/partials/product-card.html")
var loadMoreTmpl = parseWithFuncs("templates/partials/load-more.html")

func fetchProducts(limit, offset int) ([]Product, error) {
	resp, err := http.Get(fmt.Sprintf("%s/api/products?limit=%d&offset=%d", BackendURL, limit, offset))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var list productListResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	return list.Products, nil
}

func fetchProduct(id string) (Product, int, error) {
	resp, err := http.Get(fmt.Sprintf("%s/api/products/%s", BackendURL, id))
	if err != nil {
		return Product{}, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Product{}, resp.StatusCode, nil
	}

	var p Product
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return Product{}, 0, err
	}
	return p, 200, nil
}

// renderCards renders product cards into an HTML string.
func renderCards(products []Product) template.HTML {
	var buf bytes.Buffer
	for _, p := range products {
		productCardTmpl.Execute(&buf, p)
	}
	return template.HTML(buf.String())
}

// ---------- products list page ----------

type productsPageData struct {
	PageData
	CardsHTML    template.HTML
	LoadMoreHTML template.HTML
	ShowMore     bool
	NextOffset   int
	LoadError    bool
}

// ProductsPage — shop page, first page of products, server-rendered.
func ProductsPage(w http.ResponseWriter, r *http.Request) {
	isLoggedIn, user := fetchUserSession(r)

	products, err := fetchProducts(productPageSize, 0)

	data := productsPageData{PageData: PageData{IsLoggedIn: isLoggedIn, User: user}}
	if err != nil {
		data.LoadError = true
	} else {
		data.CardsHTML = renderCards(products)
		data.ShowMore = len(products) == productPageSize
		data.NextOffset = productPageSize
	}
	if data.ShowMore {
		var buf bytes.Buffer
		loadMoreTmpl.Execute(&buf, data.NextOffset)
		data.LoadMoreHTML = template.HTML(buf.String())
	}

	tmpl := parseWithFuncs("templates/layout.html", "templates/products.html")
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

// ProductsMore — HTMX load-more endpoint, appends the next batch.
func ProductsMore(w http.ResponseWriter, r *http.Request) {
	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}

	products, err := fetchProducts(productPageSize, offset)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<p style="color: var(--danger);">Couldn't load more products. <button class="icon-btn" onclick="htmx.trigger(this, 'loadMore')">Retry</button></p>`))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(renderCards(products)))

	if len(products) == productPageSize {
		var buf bytes.Buffer
		loadMoreTmpl.Execute(&buf, offset+productPageSize)
		buf.WriteTo(w)
	}
}

// ---------- single product page ----------

type productPageData struct {
	PageData
	Product  Product
	NotFound bool
}

// ProductPage — /product/{id}
func ProductPage(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/product/")
	if id == r.URL.Path || id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	isLoggedIn, user := fetchUserSession(r)
	p, status, err := fetchProduct(id)

	data := productPageData{PageData: PageData{IsLoggedIn: isLoggedIn, User: user}}
	if err != nil {
		http.Error(w, "Backend unreachable", http.StatusBadGateway)
		return
	}
	if status == http.StatusNotFound {
		data.NotFound = true
	} else {
		data.Product = p
	}

	tmpl := parseWithFuncs("templates/layout.html", "templates/product.html")
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}
