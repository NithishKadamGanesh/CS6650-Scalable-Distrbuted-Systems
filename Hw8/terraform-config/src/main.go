package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"hw8-store/db"

	"github.com/gorilla/mux"
)

// ════════════════════════════════════════════════════════════════
// Domain models (carried over from HW5)
// ════════════════════════════════════════════════════════════════

type Product struct {
	ProductID    int    `json:"product_id"`
	SKU          string `json:"sku"`
	Manufacturer string `json:"manufacturer"`
	CategoryID   int    `json:"category_id"`
	Weight       int    `json:"weight"`
	SomeOtherID  int    `json:"some_other_id"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// In-memory product store (unchanged from HW5)
var (
	productStore = make(map[int]Product)
	mu           sync.RWMutex
)

// ════════════════════════════════════════════════════════════════
// JSON helpers
// ════════════════════════════════════════════════════════════════

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message, details string) {
	writeJSON(w, status, ErrorResponse{
		Error:   code,
		Message: message,
		Details: details,
	})
}

// ════════════════════════════════════════════════════════════════
// HW5 Product Handlers (in-memory, unchanged)
// ════════════════════════════════════════════════════════════════

func createProduct(w http.ResponseWriter, r *http.Request) {
	var product Product
	if err := json.NewDecoder(r.Body).Decode(&product); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Invalid JSON body", "Ensure request matches Product schema")
		return
	}

	if product.ProductID < 1 || product.SKU == "" || len(product.SKU) > 100 ||
		product.Manufacturer == "" || len(product.Manufacturer) > 200 ||
		product.CategoryID < 1 || product.Weight < 0 || product.SomeOtherID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Missing or invalid required fields", "Check all required properties")
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if _, exists := productStore[product.ProductID]; exists {
		writeError(w, http.StatusConflict, "CONFLICT",
			"Product already exists", "Product ID must be unique")
		return
	}

	productStore[product.ProductID] = product
	writeJSON(w, http.StatusCreated, product)
}

func getProduct(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	productID, err := strconv.Atoi(params["productId"])
	if err != nil || productID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Product ID must be positive", "Minimum value is 1")
		return
	}

	mu.RLock()
	product, exists := productStore[productID]
	mu.RUnlock()

	if !exists {
		writeError(w, http.StatusNotFound, "NOT_FOUND",
			"Product not found", "Product ID does not exist")
		return
	}

	writeJSON(w, http.StatusOK, product)
}

func updateProductDetails(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	productID, err := strconv.Atoi(params["productId"])
	if err != nil || productID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Product ID must be positive", "Minimum value is 1")
		return
	}

	var product Product
	if err := json.NewDecoder(r.Body).Decode(&product); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Invalid JSON body", "Ensure body matches Product schema")
		return
	}

	if product.ProductID != productID {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Path productId must match body product_id", "They must be identical")
		return
	}

	if product.SKU == "" || len(product.SKU) > 100 ||
		product.Manufacturer == "" || len(product.Manufacturer) > 200 ||
		product.CategoryID < 1 || product.Weight < 0 || product.SomeOtherID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Missing or invalid required fields", "Check all required properties")
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if _, exists := productStore[productID]; !exists {
		writeError(w, http.StatusNotFound, "NOT_FOUND",
			"Product not found", "Cannot update non-existing product")
		return
	}

	productStore[productID] = product
	w.WriteHeader(http.StatusNoContent)
}

// ════════════════════════════════════════════════════════════════
// Step I: Shopping Cart Handlers (MySQL-backed)
// ════════════════════════════════════════════════════════════════

func createShoppingCart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CustomerID int `json:"customer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Invalid JSON body", "Ensure request includes customer_id")
		return
	}
	if req.CustomerID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"customer_id must be a positive integer", "Minimum value is 1")
		return
	}

	cartID, err := db.CreateCart(req.CustomerID)
	if err != nil {
		log.Printf("ERROR createCart: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			"Failed to create shopping cart", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]int{
		"shopping_cart_id": cartID,
	})
}

func getShoppingCart(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	cartID, err := strconv.Atoi(params["shoppingCartId"])
	if err != nil || cartID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Shopping cart ID must be a positive integer", "Minimum value is 1")
		return
	}

	cart, err := db.GetCart(cartID)
	if err != nil {
		log.Printf("ERROR getCart(%d): %v", cartID, err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			"Failed to retrieve shopping cart", err.Error())
		return
	}
	if cart == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND",
			"Shopping cart not found", "No cart with this ID exists")
		return
	}

	writeJSON(w, http.StatusOK, cart)
}

func addItemsToCart(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	cartID, err := strconv.Atoi(params["shoppingCartId"])
	if err != nil || cartID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Shopping cart ID must be a positive integer", "Minimum value is 1")
		return
	}

	var req struct {
		ProductID int `json:"product_id"`
		Quantity  int `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Invalid JSON body", "Ensure request includes product_id and quantity")
		return
	}
	if req.ProductID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"product_id must be a positive integer", "Minimum value is 1")
		return
	}
	if req.Quantity < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"quantity must be a positive integer", "Minimum value is 1")
		return
	}

	err = db.AddItemToCart(cartID, req.ProductID, req.Quantity)
	if err != nil {
		if strings.Contains(err.Error(), "CART_NOT_FOUND") {
			writeError(w, http.StatusNotFound, "NOT_FOUND",
				"Shopping cart not found", "Cannot add items to non-existing cart")
			return
		}
		log.Printf("ERROR addItem cart=%d product=%d: %v", cartID, req.ProductID, err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			"Failed to add item to cart", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ════════════════════════════════════════════════════════════════
// Step II: Shopping Cart Handlers (DynamoDB-backed)
// ════════════════════════════════════════════════════════════════

func dynamoCreateShoppingCart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CustomerID int `json:"customer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Invalid JSON body", "Ensure request includes customer_id")
		return
	}
	if req.CustomerID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"customer_id must be a positive integer", "Minimum value is 1")
		return
	}

	cartID, err := db.DynamoCreateCart(req.CustomerID)
	if err != nil {
		log.Printf("ERROR dynamo createCart: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			"Failed to create shopping cart", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]int{
		"shopping_cart_id": cartID,
	})
}

func dynamoGetShoppingCart(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	cartID, err := strconv.Atoi(params["shoppingCartId"])
	if err != nil || cartID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Shopping cart ID must be a positive integer", "Minimum value is 1")
		return
	}

	cart, err := db.DynamoGetCart(cartID)
	if err != nil {
		log.Printf("ERROR dynamo getCart(%d): %v", cartID, err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			"Failed to retrieve shopping cart", err.Error())
		return
	}
	if cart == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND",
			"Shopping cart not found", "No cart with this ID exists")
		return
	}

	writeJSON(w, http.StatusOK, cart)
}

func dynamoAddItemsToCart(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	cartID, err := strconv.Atoi(params["shoppingCartId"])
	if err != nil || cartID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Shopping cart ID must be a positive integer", "Minimum value is 1")
		return
	}

	var req struct {
		ProductID int `json:"product_id"`
		Quantity  int `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"Invalid JSON body", "Ensure request includes product_id and quantity")
		return
	}
	if req.ProductID < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"product_id must be a positive integer", "Minimum value is 1")
		return
	}
	if req.Quantity < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT",
			"quantity must be a positive integer", "Minimum value is 1")
		return
	}

	err = db.DynamoAddItemToCart(cartID, req.ProductID, req.Quantity)
	if err != nil {
		if strings.Contains(err.Error(), "CART_NOT_FOUND") {
			writeError(w, http.StatusNotFound, "NOT_FOUND",
				"Shopping cart not found", "Cannot add items to non-existing cart")
			return
		}
		log.Printf("ERROR dynamo addItem cart=%d product=%d: %v", cartID, req.ProductID, err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			"Failed to add item to cart", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ════════════════════════════════════════════════════════════════
// Server entry point
// ════════════════════════════════════════════════════════════════

func main() {
	// Initialize MySQL connection pool + schema migration (Step I)
	if err := db.InitDB(); err != nil {
		log.Fatalf("FATAL: MySQL init failed: %v", err)
	}
	defer db.DB.Close()

	// Initialize DynamoDB client (Step II)
	if err := db.InitDynamo(); err != nil {
		log.Fatalf("FATAL: DynamoDB init failed: %v", err)
	}

	router := mux.NewRouter()

	// ── HW5 Product endpoints (in-memory) ───────────────────
	router.HandleFunc("/products", createProduct).Methods("POST")
	router.HandleFunc("/products/{productId}", getProduct).Methods("GET")
	router.HandleFunc("/products/{productId}/details", updateProductDetails).Methods("POST")

	// ── Step I: Shopping Cart (MySQL) ────────────────────────
	router.HandleFunc("/shopping-carts", createShoppingCart).Methods("POST")
	router.HandleFunc("/shopping-carts/{shoppingCartId}", getShoppingCart).Methods("GET")
	router.HandleFunc("/shopping-carts/{shoppingCartId}/items", addItemsToCart).Methods("POST")

	// ── Step II: Shopping Cart (DynamoDB) — parallel routes ──
	router.HandleFunc("/dynamo/shopping-carts", dynamoCreateShoppingCart).Methods("POST")
	router.HandleFunc("/dynamo/shopping-carts/{shoppingCartId}", dynamoGetShoppingCart).Methods("GET")
	router.HandleFunc("/dynamo/shopping-carts/{shoppingCartId}/items", dynamoAddItemsToCart).Methods("POST")

	// ── Health check ────────────────────────────────────────
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		mysqlOK := "healthy"
		if err := db.DB.Ping(); err != nil {
			mysqlOK = err.Error()
		}

		dynamoOK := "healthy"
		if db.DynamoClient == nil {
			dynamoOK = "not initialized"
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"mysql":    mysqlOK,
			"dynamodb": dynamoOK,
		})
	}).Methods("GET")

	log.Println("Server running on :8080 (MySQL + DynamoDB)")
	log.Fatal(http.ListenAndServe(":8080", router))
}
