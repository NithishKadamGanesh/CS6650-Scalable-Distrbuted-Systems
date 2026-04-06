package db

import (
	"database/sql"
	"fmt"
	"time"
)

// ── Domain models ───────────────────────────────────────────

type ShoppingCart struct {
	CartID     int        `json:"cart_id"`
	CustomerID int        `json:"customer_id"`
	Items      []CartItem `json:"items,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type CartItem struct {
	ItemID    int       `json:"item_id"`
	ProductID int       `json:"product_id"`
	Quantity  int       `json:"quantity"`
	AddedAt   time.Time `json:"added_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ── CREATE: POST /shopping-carts ────────────────────────────
// Inserts a new row into shopping_carts.
// Single INSERT — no transaction needed.

func CreateCart(customerID int) (int, error) {
	result, err := DB.Exec(
		"INSERT INTO shopping_carts (customer_id) VALUES (?)",
		customerID,
	)
	if err != nil {
		return 0, fmt.Errorf("insert cart: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}

	return int(id), nil
}

// ── READ: GET /shopping-carts/{id} ──────────────────────────
// Uses a LEFT JOIN to fetch the cart + all items in a single query.
// LEFT JOIN ensures we still return the cart even if it has 0 items.
// Performance: with INDEX on cart_items.cart_id, this is an index
// lookup + sequential scan of the item rows — well under 50ms.

func GetCart(cartID int) (*ShoppingCart, error) {
	rows, err := DB.Query(`
		SELECT
			c.cart_id,
			c.customer_id,
			c.created_at,
			c.updated_at,
			ci.item_id,
			ci.product_id,
			ci.quantity,
			ci.added_at,
			ci.updated_at
		FROM shopping_carts c
		LEFT JOIN cart_items ci ON c.cart_id = ci.cart_id
		WHERE c.cart_id = ?
	`, cartID)
	if err != nil {
		return nil, fmt.Errorf("query cart: %w", err)
	}
	defer rows.Close()

	var cart *ShoppingCart

	for rows.Next() {
		// item fields are nullable (LEFT JOIN with no items)
		var (
			cCartID     int
			cCustomerID int
			cCreatedAt  time.Time
			cUpdatedAt  time.Time
			iItemID     sql.NullInt64
			iProductID  sql.NullInt64
			iQuantity   sql.NullInt64
			iAddedAt    sql.NullTime
			iUpdatedAt  sql.NullTime
		)

		if err := rows.Scan(
			&cCartID, &cCustomerID, &cCreatedAt, &cUpdatedAt,
			&iItemID, &iProductID, &iQuantity, &iAddedAt, &iUpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		// First row: initialize the cart
		if cart == nil {
			cart = &ShoppingCart{
				CartID:     cCartID,
				CustomerID: cCustomerID,
				Items:      []CartItem{},
				CreatedAt:  cCreatedAt,
				UpdatedAt:  cUpdatedAt,
			}
		}

		// Append item if it exists (not NULL from LEFT JOIN)
		if iItemID.Valid {
			cart.Items = append(cart.Items, CartItem{
				ItemID:    int(iItemID.Int64),
				ProductID: int(iProductID.Int64),
				Quantity:  int(iQuantity.Int64),
				AddedAt:   iAddedAt.Time,
				UpdatedAt: iUpdatedAt.Time,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	// cart == nil means no rows returned → cart doesn't exist
	return cart, nil
}

// ── ADD ITEM: POST /shopping-carts/{id}/items ───────────────
// Uses a TRANSACTION to:
//   1. Verify the cart exists (SELECT with row lock)
//   2. UPSERT the item (INSERT ... ON DUPLICATE KEY UPDATE)
//
// The UNIQUE(cart_id, product_id) constraint makes the upsert safe:
//   - New product → INSERT
//   - Existing product → UPDATE quantity (additive)
//
// Transaction ensures atomicity: if the cart is deleted between
// the existence check and the insert, the FK constraint catches it.

func AddItemToCart(cartID, productID, quantity int) error {
	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // no-op if committed

	// Step 1: Verify cart exists
	var exists int
	err = tx.QueryRow(
		"SELECT 1 FROM shopping_carts WHERE cart_id = ?",
		cartID,
	).Scan(&exists)

	if err == sql.ErrNoRows {
		return fmt.Errorf("CART_NOT_FOUND")
	}
	if err != nil {
		return fmt.Errorf("check cart: %w", err)
	}

	// Step 2: Upsert item — add quantity if product already in cart
	_, err = tx.Exec(`
		INSERT INTO cart_items (cart_id, product_id, quantity)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE
			quantity = quantity + VALUES(quantity),
			updated_at = CURRENT_TIMESTAMP
	`, cartID, productID, quantity)
	if err != nil {
		return fmt.Errorf("upsert item: %w", err)
	}

	// Step 3: Touch the cart's updated_at
	_, err = tx.Exec(
		"UPDATE shopping_carts SET updated_at = CURRENT_TIMESTAMP WHERE cart_id = ?",
		cartID,
	)
	if err != nil {
		return fmt.Errorf("touch cart: %w", err)
	}

	return tx.Commit()
}
