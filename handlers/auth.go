package handlers

import (
	"bytes"
	"encoding/json"
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

type Address struct {
	Country string `json:"country"`
	State   string `json:"state"`
	LGA     string `json:"lga"`
	Town    string `json:"town"`
	Street  string `json:"street"`
}

type UserProfile struct {
	ID        string   `json:"id"`
	Email     string   `json:"email"`
	FullName  string   `json:"full_name"`
	IsAdmin   bool     `json:"is_admin"`
	Verified  bool     `json:"verified"`
	Address   *Address `json:"address"`
	CreatedAt string   `json:"created_at"`
}

// fetchUserSession gets the user profile from the Render backend if a cookie exists
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

// Handle Login Proxy Request from HTMX
func Login(w http.ResponseWriter, r *http.Request) {
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

	// Forward any cookies (e.g. refresh_token) set by the Render backend
	for _, cookie := range resp.Cookies() {
		http.SetCookie(w, cookie)
	}

	// Tell HTMX to redirect the browser to the home page on success
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

// Handle Logout — clears the session cookie
func Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	// Tell HTMX (hx-boost) to redirect to home
	w.Header().Set("HX-Redirect", "/")
	if r.Header.Get("HX-Request") == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}
