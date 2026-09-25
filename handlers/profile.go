package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// Render Profile Page — requires a valid session
func ProfilePage(w http.ResponseWriter, r *http.Request) {
	isLoggedIn, user := fetchUserSession(r)
	if !isLoggedIn {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	cartCount := 0
	if isLoggedIn {
		cartCount = fetchCartCount(r)
	}

	tmpl := parseWithFuncs("templates/layout.html", "templates/profile.html")
	tmpl.Execute(w, PageData{IsLoggedIn: true, User: user, CartCount: cartCount})
}

// Proxy PUT /api/me/address — HTMX submits the form with hx-put
func UpdateAddress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie("access_token")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`<span style="color: var(--danger);">Your session expired. Please log in again.</span>`))
		return
	}

	reqBody, _ := json.Marshal(Address{
		Country: r.FormValue("country"),
		State:   r.FormValue("state"),
		LGA:     r.FormValue("lga"),
		Town:    r.FormValue("town"),
		Street:  r.FormValue("street"),
	})

	req, _ := http.NewRequest("PUT", BackendURL+"/api/me/address", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cookie.Value)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<span style="color: var(--danger);">Unable to connect to backend server.</span>`))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		switch resp.StatusCode {
		case http.StatusBadRequest:
			w.Write([]byte(`<span style="color: var(--danger);">All fields are required (country/state/LGA/town max 100 chars, street max 200).</span>`))
		case http.StatusUnauthorized:
			w.Write([]byte(`<span style="color: var(--danger);">Your session expired. Please log in again.</span>`))
		default:
			w.Write([]byte(`<span style="color: var(--danger);">Could not save address. Try again.</span>`))
		}
		return
	}

	w.Write([]byte(`<span style="color: #4ade80;">Address saved.</span>`))
}

// Proxy POST /api/account/delete — requires password confirmation
func DeleteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie("access_token")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`<span style="color: var(--danger);">Your session expired. Please log in again.</span>`))
		return
	}

	reqBody, _ := json.Marshal(map[string]string{
		"password": r.FormValue("password"),
	})

	req, _ := http.NewRequest("POST", BackendURL+"/api/account/delete", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cookie.Value)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<span style="color: var(--danger);">Unable to connect to backend server.</span>`))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		w.Write([]byte(`<span style="color: var(--danger);">Incorrect password. Account not deleted.</span>`))
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}
