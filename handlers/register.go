package handlers

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
)

type SignupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type SignupResponse struct {
	Message     string `json:"message"`
	VerifyToken string `json:"verify_token"`
}

type VerifyRequest struct {
	Token string `json:"token"`
	Code  string `json:"code"`
}

type ResendRequest struct {
	Email string `json:"email"`
}

// Render Register Page View
func RegisterPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/layout.html", "templates/register.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

// Step 1: Proxy signup to backend, then swap in the verify-code form
func Signup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reqBody, _ := json.Marshal(SignupRequest{
		Email:    r.FormValue("email"),
		Password: r.FormValue("password"),
		FullName: r.FormValue("full_name"),
	})

	resp, err := http.Post(BackendURL+"/api/auth/signup", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<span style="color: var(--danger);">Unable to connect to backend server.</span>`))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		w.WriteHeader(resp.StatusCode)
		switch resp.StatusCode {
		case http.StatusConflict:
			w.Write([]byte(`<span style="color: var(--danger);">An account with this email already exists.</span>`))
		case http.StatusBadRequest:
			w.Write([]byte(`<span style="color: var(--danger);">Please check your details (valid email, password 8+ characters, full name required).</span>`))
		default:
			w.Write([]byte(`<span style="color: var(--danger);">Signup failed. Please try again later.</span>`))
		}
		return
	}

	var signupResp SignupResponse
	if err := json.NewDecoder(resp.Body).Decode(&signupResp); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<span style="color: var(--danger);">Error parsing signup response.</span>`))
		return
	}

	// Render the "check your email" partial, carrying the verify_token forward
	tmpl, err := template.ParseFiles("templates/partials/verify.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, map[string]string{
		"Email":       r.FormValue("email"),
		"VerifyToken": signupResp.VerifyToken,
	})
}

// Step 2: Proxy verification to backend
func Verify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reqBody, _ := json.Marshal(VerifyRequest{
		Token: r.FormValue("token"),
		Code:  r.FormValue("code"),
	})

	resp, err := http.Post(BackendURL+"/api/auth/verify", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<span style="color: var(--danger);">Unable to connect to backend server.</span>`))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			w.Write([]byte(`<span style="color: var(--danger);">Verification session expired. Please sign up again.</span>`))
		case http.StatusTooManyRequests:
			w.Write([]byte(`<span style="color: var(--danger);">Too many wrong attempts — this code is burned. Request a new one.</span>`))
		default:
			w.Write([]byte(`<span style="color: var(--danger);">Invalid code. Please try again.</span>`))
		}
		return
	}

	// Success — send the user to the login page
	w.Header().Set("HX-Redirect", "/login")
	w.WriteHeader(http.StatusOK)
}

// Resend verification code (anti-enumeration: backend always returns 200)
func ResendCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reqBody, _ := json.Marshal(ResendRequest{
		Email: r.FormValue("email"),
	})

	resp, err := http.Post(BackendURL+"/api/auth/resend-code", "application/json", bytes.NewBuffer(reqBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<span style="color: var(--danger);">Could not resend the code. Try again later.</span>`))
		return
	}
	defer resp.Body.Close()

	w.Write([]byte(`<span style="color: var(--accent);">If the account exists and is unverified, a new code has been sent.</span>`))
}
