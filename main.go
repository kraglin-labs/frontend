package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
)

const BackendURL = "https://backend-sethstore.onrender.com"

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type UserProfile struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	IsAdmin  bool   `json:"is_admin"`
	Verified bool   `json:"verified"`
}

type PageData struct {
	IsLoggedIn bool
	User       UserProfile
}

// Helper to get user profile from Render backend if cookie exists
func fetchUserSession(r *http.Request) (bool, UserProfile) {
	cookie, err := r.Cookie("access_token")
	if err != nil {
		return false, UserProfile{}
	}

	req, _ := http.NewRequest("GET", BackendURL+"/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+cookie.Value)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return false, UserProfile{}
	}
	defer resp.Body.Close()

	var profile UserProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return false, UserProfile{}
	}

	return true, profile
}

func main() {
	// Serve static files
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Home Page Route
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		isLoggedIn, user := fetchUserSession(r)

		tmpl, err := template.ParseFiles("templates/layout.html", "templates/index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		tmpl.Execute(w, PageData{
			IsLoggedIn: isLoggedIn,
			User:       user,
		})
	})

	// Render Login Page View
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("templates/layout.html", "templates/login.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, nil)
	})

	// Handle Login Proxy Request from HTMX
	http.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		email := r.FormValue("email")
		password := r.FormValue("password")

		reqBody, _ := json.Marshal(LoginRequest{
			Email:    email,
			Password: password,
		})

		resp, err := http.Post(BackendURL+"/api/auth/login", "application/json", bytes.NewBuffer(reqBody))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`<span style="color: var(--danger);">Unable to connect to backend server.</span>`))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			w.WriteHeader(resp.StatusCode)
			w.Write([]byte(`<span style="color: var(--danger);">Invalid email or password. Please try again.</span>`))
			return
		}

		// Decode login response to get the access_token
		var loginResp LoginResponse
		if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`<span style="color: var(--danger);">Error parsing login response.</span>`))
			return
		}

		// Save access_token in a cookie so the frontend can authenticate requests to /api/me
		http.SetCookie(w, &http.Cookie{
			Name:     "access_token",
			Value:    loginResp.AccessToken,
			Path:     "/",
			HttpOnly: true,
			MaxAge:   loginResp.ExpiresIn,
		})

		// Forward the refresh_token cookie set by Render backend
		for _, cookie := range resp.Cookies() {
			http.SetCookie(w, cookie)
		}

		// Tell HTMX to redirect the browser to the home page on success
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
	})

	println("SethOfficial frontend started at http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}
