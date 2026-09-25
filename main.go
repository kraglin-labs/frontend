package main

import (
	"net/http"

	"frontend/handlers"
)

func main() {
	// Serve static files
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Page routes
	http.HandleFunc("/", handlers.Home)
	http.HandleFunc("/login", handlers.LoginPage)
	http.HandleFunc("/register", handlers.RegisterPage)
	http.HandleFunc("/profile", handlers.ProfilePage)
	http.HandleFunc("/forgot-password", handlers.ForgotPasswordPage)

	// Auth routes
	http.HandleFunc("/auth/login", handlers.Login)
	http.HandleFunc("/auth/logout", handlers.Logout)
	http.HandleFunc("/auth/signup", handlers.Signup)
	http.HandleFunc("/auth/verify", handlers.Verify)
	http.HandleFunc("/auth/resend-code", handlers.ResendCode)
	http.HandleFunc("/auth/forgot-password", handlers.ForgotPassword)
	http.HandleFunc("/auth/reset-password", handlers.ResetPassword)

	// Profile routes
	http.HandleFunc("/profile/address", handlers.UpdateAddress)
	http.HandleFunc("/account/delete", handlers.DeleteAccount)

	// Product routes
	http.HandleFunc("/products", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	http.HandleFunc("/products/more", handlers.ProductsMore)
	http.HandleFunc("/product/", handlers.ProductPage)

	// Cart routes
	http.HandleFunc("/cart", handlers.CartHandler)              // GET page / DELETE clear
	http.HandleFunc("/cart/items", handlers.AddToCart)          // POST
	http.HandleFunc("/cart/items/", handlers.CartItemHandler)   // PUT / DELETE (id in path)

	// Search routes
	http.HandleFunc("/search", handlers.SearchPage)
	http.HandleFunc("/search/suggest", handlers.Suggest)

	println("SethOfficial frontend started at http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}
