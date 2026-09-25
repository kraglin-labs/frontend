package handlers

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"strings"
)

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"new_password"`
}

// Render Forgot Password Page View
func ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/layout.html", "templates/forgot.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

// Step 1: Request a reset code
func ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	email := r.FormValue("email")

	reqBody, _ := json.Marshal(ForgotPasswordRequest{Email: email})
	resp, err := http.Post(BackendURL+"/api/auth/forgot-password", "application/json", bytes.NewBuffer(reqBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<span style="color: var(--danger);">Unable to process the request. Try again later.</span>`))
		return
	}
	defer resp.Body.Close()

	// Success — swap in the reset form, carrying the email forward
	tmpl, err := template.ParseFiles("templates/partials/reset.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Retarget", "#forgot-container")
	w.Header().Set("HX-Reswap", "innerHTML")
	tmpl.Execute(w, map[string]string{"Email": email})
}

// Step 2: Reset with code + new password
func ResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reqBody, _ := json.Marshal(ResetPasswordRequest{
		Email:       r.FormValue("email"),
		Code:        r.FormValue("code"),
		NewPassword: r.FormValue("new_password"),
	})

	resp, err := http.Post(BackendURL+"/api/auth/reset-password", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<span style="color: var(--danger);">Unable to connect to backend server.</span>`))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		msg := strings.ToLower(string(body))
		w.WriteHeader(resp.StatusCode)

		switch {
		case strings.Contains(msg, "at least 8"):
			w.Write([]byte(`<span style="color: var(--danger);">Password must be at least 8 characters.</span>`))
		case strings.Contains(msg, "too many"):
			w.Write([]byte(`<span style="color: var(--danger);">Too many wrong attempts — request a new code.</span>`))
		default:
			w.Write([]byte(`<span style="color: var(--danger);">Invalid or expired code. Please try again.</span>`))
		}
		return
	}

	// Success — back to login with the new password
	w.Header().Set("HX-Redirect", "/login")
	w.WriteHeader(http.StatusOK)
}
