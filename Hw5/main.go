package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/mux"
)

// Product schema
type Product struct {
	ProductID    int    `json:"product_id"`
	SKU          string `json:"sku"`
	Manufacturer string `json:"manufacturer"`
	CategoryID   int    `json:"category_id"`
	Weight       int    `json:"weight"`
	SomeOtherID  int    `json:"some_other_id"`
}

// Error schema
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}



var (
	productStore = make(map[int]Product)
	mu           sync.RWMutex
)



func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message, details string) {
	err := ErrorResponse{
		Error:   code,
		Message: message,
		Details: details,
	}
	writeJSON(w, status, err)
}


func createProduct(w http.ResponseWriter, r *http.Request) {

	var product Product

	err := json.NewDecoder(r.Body).Decode(&product)
	if err != nil {
		writeError(w, http.StatusBadRequest,
			"INVALID_INPUT",
			"Invalid JSON body",
			"Ensure request matches Product schema")
		return
	}

	// Validate required fields
	if product.ProductID < 1 ||
		product.SKU == "" ||
		len(product.SKU) > 100 ||
		product.Manufacturer == "" ||
		len(product.Manufacturer) > 200 ||
		product.CategoryID < 1 ||
		product.Weight < 0 ||
		product.SomeOtherID < 1 {

		writeError(w, http.StatusBadRequest,
			"INVALID_INPUT",
			"Missing or invalid required fields",
			"Check all required properties")
		return
	}

	mu.Lock()
	defer mu.Unlock()

	// Check uniqueness
	if _, exists := productStore[product.ProductID]; exists {
		writeError(w, http.StatusConflict,
			"CONFLICT",
			"Product already exists",
			"Product ID must be unique")
		return
	}

	productStore[product.ProductID] = product

	writeJSON(w, http.StatusCreated, product)
}


func getProduct(w http.ResponseWriter, r *http.Request) {

	params := mux.Vars(r)

	productID, err := strconv.Atoi(params["productId"])
	if err != nil || productID < 1 {
		writeError(w, http.StatusBadRequest,
			"INVALID_INPUT",
			"Product ID must be positive",
			"Minimum value is 1")
		return
	}

	mu.RLock()
	product, exists := productStore[productID]
	mu.RUnlock()

	if !exists {
		writeError(w, http.StatusNotFound,
			"NOT_FOUND",
			"Product not found",
			"Product ID does not exist")
		return
	}

	writeJSON(w, http.StatusOK, product)
}


func updateProductDetails(w http.ResponseWriter, r *http.Request) {

	params := mux.Vars(r)

	productID, err := strconv.Atoi(params["productId"])
	if err != nil || productID < 1 {
		writeError(w, http.StatusBadRequest,
			"INVALID_INPUT",
			"Product ID must be positive",
			"Minimum value is 1")
		return
	}

	var product Product
	err = json.NewDecoder(r.Body).Decode(&product)
	if err != nil {
		writeError(w, http.StatusBadRequest,
			"INVALID_INPUT",
			"Invalid JSON body",
			"Ensure body matches Product schema")
		return
	}

	if product.ProductID != productID {
		writeError(w, http.StatusBadRequest,
			"INVALID_INPUT",
			"Path productId must match body product_id",
			"They must be identical")
		return
	}

	// Validate fields
	if product.SKU == "" ||
		len(product.SKU) > 100 ||
		product.Manufacturer == "" ||
		len(product.Manufacturer) > 200 ||
		product.CategoryID < 1 ||
		product.Weight < 0 ||
		product.SomeOtherID < 1 {

		writeError(w, http.StatusBadRequest,
			"INVALID_INPUT",
			"Missing or invalid required fields",
			"Check all required properties")
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if _, exists := productStore[productID]; !exists {
		writeError(w, http.StatusNotFound,
			"NOT_FOUND",
			"Product not found",
			"Cannot update non-existing product")
		return
	}

	productStore[productID] = product

	w.WriteHeader(http.StatusNoContent)
}

func main() {

	router := mux.NewRouter()

	router.HandleFunc("/products", createProduct).Methods("POST")
	router.HandleFunc("/products/{productId}", getProduct).Methods("GET")
	router.HandleFunc("/products/{productId}/details", updateProductDetails).Methods("POST")

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", router))
}