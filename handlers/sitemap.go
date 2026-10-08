package handlers

import (
	"encoding/xml"
	"net/http"
)

// Sitemap serves /sitemap.xml for search engines.
// Lives in handlers/ so it can reuse fetchProducts and the Product type.
func Sitemap(w http.ResponseWriter, r *http.Request) {
	const base = "https://sethstore.shop"

	type urlEntry struct {
		Loc      string `xml:"loc"`
		Lastmod  string `xml:"lastmod,omitempty"`
		Priority string `xml:"priority,omitempty"`
	}
	type urlset struct {
		XMLName xml.Name   `xml:"urlset"`
		Xmlns   string     `xml:"xmlns,attr"`
		URLs    []urlEntry `xml:"url"`
	}

	entries := []urlEntry{
		{Loc: base + "/", Priority: "1.0"},
	}

	// Page through all products using the existing helper.
	// The API caps limit at productPageSize, so loop until a short page.
	for offset := 0; ; offset += productPageSize {
		products, err := fetchProducts(productPageSize, offset)
		if err != nil {
			break // backend down: serve sitemap with what we have
		}
		if len(products) == 0 {
			break
		}
		for _, p := range products {
			if !p.Active {
				continue
			}
			e := urlEntry{
				Loc:      base + "/product/" + p.ID,
				Priority: "0.8",
			}
			if p.UpdatedAt != "" {
				e.Lastmod = p.UpdatedAt
			}
			entries = append(entries, e)
		}
		if len(products) < productPageSize {
			break
		}
	}

	payload, err := xml.Marshal(urlset{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  entries,
	})
	if err != nil {
		http.Error(w, "sitemap error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Write([]byte(xml.Header))
	w.Write(payload)
}
