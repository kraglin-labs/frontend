package handlers

import (
	"bytes"
	"html/template"
	"net/http"
)

type PageData struct {
	IsLoggedIn bool
	User       UserProfile
	CartCount  int
}

// Home Page — shows the product catalog
func Home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	isLoggedIn, user := fetchUserSession(r)

	cartCount := 0
	if isLoggedIn {
		cartCount = fetchCartCount(r)
	}

	products, err := fetchProducts(productPageSize, 0)

	data := productsPageData{PageData: PageData{IsLoggedIn: isLoggedIn, User: user, CartCount: cartCount}}
	if err != nil {
		data.LoadError = true
	} else {
		data.CardsHTML = renderCards(products)
		data.ShowMore = len(products) == productPageSize
		data.NextOffset = productPageSize
		if data.ShowMore {
			var buf bytes.Buffer
			btn := parseWithFuncs("templates/partials/load-more.html")
			btn.Execute(&buf, data.NextOffset)
			data.LoadMoreHTML = template.HTML(buf.String())
		}
	}

	tmpl := parseWithFuncs("templates/layout.html", "templates/index.html")
	tmpl.Execute(w, data)
}

// Render Login Page View
func LoginPage(w http.ResponseWriter, r *http.Request) {
	tmpl := parseWithFuncs("templates/layout.html", "templates/login.html")
	tmpl.Execute(w, nil)
}
