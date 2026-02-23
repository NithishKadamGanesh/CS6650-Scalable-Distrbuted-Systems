package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
}

var productStore sync.Map

// Generate 100,000 products at startup
func generateProducts() {
	categories := []string{"Electronics", "Books", "Home", "Clothing", "Sports"}
	brands := []string{"Alpha", "Beta", "Gamma", "Delta", "Omega"}

	for i := 1; i <= 100000; i++ {
		product := Product{
			ID:          i,
			Name:        fmt.Sprintf("Product %s %d", brands[i%len(brands)], i),
			Category:    categories[i%len(categories)],
			Description: "Sample description",
			Brand:       brands[i%len(brands)],
		}
		productStore.Store(i, product)
	}
}

// Search endpoint
func searchHandler(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("q"))
	start := time.Now()

	checked := 0
	found := 0
	results := []Product{}

	productStore.Range(func(key, value interface{}) bool {
		if checked >= 100 {
			return false
		}

		product := value.(Product)
		checked++

		if strings.Contains(strings.ToLower(product.Name), query) ||
			strings.Contains(strings.ToLower(product.Category), query) {

			found++
			if len(results) < 20 {
				results = append(results, product)
			}
		}

		return true
	})

	response := map[string]interface{}{
		"products":    results,
		"total_found": found,
		"search_time": time.Since(start).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func main() {
	generateProducts()

	http.HandleFunc("/products/search", searchHandler)
	http.HandleFunc("/health", healthHandler)

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}