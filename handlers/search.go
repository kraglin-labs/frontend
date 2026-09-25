package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"unicode"
)

// ---- shared filtering ----

func searchMatches(products []Product, q string) []Product {
	ql := strings.ToLower(q)
	var out []Product
	for _, p := range products {
		if strings.Contains(strings.ToLower(p.Name), ql) ||
			strings.Contains(strings.ToLower(p.Category), ql) ||
			strings.Contains(strings.ToLower(p.Description), ql) {
			out = append(out, p)
		}
	}
	return out
}

func distinctCategories(products []Product) []string {
	seen := map[string]bool{}
	var cats []string
	for _, p := range products {
		c := strings.ToLower(p.Category)
		if !seen[c] {
			seen[c] = true
			cats = append(cats, p.Category)
		}
	}
	sort.Strings(cats)
	return cats
}

// Suggest — feeds the header dropdown while typing.
// Terms come from the words in product names/descriptions (like your
// "bracelet / bracelets for men" screenshot), plus full names and categories.
func Suggest(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		w.Write([]byte(""))
		return
	}

	products, err := fetchProducts(100, 0)
	if err != nil {
		w.Write([]byte(`<p style="padding: 1rem; color: var(--danger);">Suggestions unavailable.</p>`))
		return
	}

	matches := searchMatches(products, q)
	ql := strings.ToLower(q)
	seen := map[string]bool{}
	var terms []string

	add := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			return
		}
		seen[t] = true
		terms = append(terms, t)
	}

	// 1. Words from names/descriptions that START with the query
	for _, p := range matches {
		words := strings.FieldsFunc(p.Name+" "+p.Description, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		})
		for _, word := range words {
			if strings.HasPrefix(strings.ToLower(word), ql) && len(terms) < 6 {
				add(word)
			}
		}
	}

	// 2. Full product names containing the query
	for _, p := range matches {
		if len(terms) >= 10 {
			break
		}
		add(p.Name)
	}

	// 3. Matching categories
	for _, c := range distinctCategories(matches) {
		if len(terms) >= 12 {
			break
		}
		if strings.Contains(strings.ToLower(c), ql) {
			add(c)
		}
	}

	if len(terms) == 0 {
		fmt.Fprintf(w, `<p style="padding: 1rem; color: var(--text-muted);">No suggestions for "<strong>%s</strong>". Press Enter to search anyway.</p>`, template.HTMLEscapeString(q))
		return
	}

	var out strings.Builder
	out.WriteString(`<div class="suggest-list">`)
	for _, t := range terms {
		fmt.Fprintf(&out, `<a href="/search?q=%s" class="suggest-item"><span>%s</span><span class="suggest-arrow">›</span></a>`,
			template.URLQueryEscaper(t), template.HTMLEscapeString(t))
	}
	out.WriteString(`</div>`)
	w.Write([]byte(out.String()))
}

type searchPageData struct {
	PageData
	Query          string
	CardsHTML      template.HTML
	Categories     []string
	ActiveCategory string
	ResultCount    int
}

// SearchPage — the full results page at /search?q=...&category=...
func SearchPage(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	isLoggedIn, user := fetchUserSession(r)
	cartCount := 0
	if isLoggedIn {
		cartCount = fetchCartCount(r)
	}

	data := searchPageData{
		PageData:       PageData{IsLoggedIn: isLoggedIn, User: user, CartCount: cartCount},
		Query:          q,
		ActiveCategory: r.URL.Query().Get("category"),
	}

	if q != "" {
		if products, err := fetchProducts(100, 0); err == nil {
			matches := searchMatches(products, q)
			data.Categories = distinctCategories(matches)
			if data.ActiveCategory != "" {
				var filtered []Product
				for _, p := range matches {
					if strings.EqualFold(p.Category, data.ActiveCategory) {
						filtered = append(filtered, p)
					}
				}
				matches = filtered
			}
			data.ResultCount = len(matches)
			data.CardsHTML = renderCards(matches)
		}
	}

	tmpl := parseWithFuncs("templates/layout.html", "templates/search.html")
	tmpl.Execute(w, data)
}
