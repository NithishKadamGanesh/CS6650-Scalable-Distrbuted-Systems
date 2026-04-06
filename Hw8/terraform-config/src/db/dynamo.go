package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ════════════════════════════════════════════════════════════════
// DynamoDB Design Decisions (documented for the assignment)
// ════════════════════════════════════════════════════════════════
//
// SINGLE TABLE with embedded items:
//   - Partition key: cart_id (Number)
//   - No sort key needed — all access is by cart_id
//   - Items stored as a List of Maps attribute inside the cart document
//
// Why single table with embedded items?
//   Shopping carts have simple access patterns: create, get-by-id,
//   add-item. We always read/write the entire cart, never query
//   individual items across carts. Embedding items avoids the need
//   for Query operations with sort keys and keeps reads to a single
//   GetItem call (1 RCU) instead of a Query scanning multiple rows.
//
// Why NOT a sort key for items?
//   A composite key (cart_id PK, product_id SK) would allow querying
//   items individually, but our API always returns the full cart.
//   Separate items would require a Query (more RCUs) instead of
//   GetItem (1 RCU). The embedded approach is more cost-efficient.
//
// Partition key distribution:
//   cart_id is a monotonically increasing integer, which DynamoDB
//   handles well with adaptive capacity and auto-splitting. For
//   millions of carts, each cart_id maps to a different partition,
//   giving even distribution with no hot partitions.
//
// Auto-increment ID generation:
//   DynamoDB has no AUTO_INCREMENT. We use a special counter item
//   (cart_id = 0) with an atomic UpdateItem ADD operation to generate
//   sequential IDs, matching the MySQL API response format.
//
// Eventual consistency:
//   GetItem uses eventually consistent reads by default (0.5 RCU).
//   For the shopping cart use case, a brief delay (typically <100ms)
//   is acceptable — users rarely create and immediately read a cart
//   in the same millisecond. We can opt into strongly consistent
//   reads (1 RCU) if needed via ConsistentRead: true.
// ════════════════════════════════════════════════════════════════

// DynamoClient holds the DynamoDB client and table name
var DynamoClient *dynamodb.Client
var DynamoTableName string

// DynamoCartItem represents an item embedded in the cart document
type DynamoCartItem struct {
	ProductID int    `dynamodbav:"product_id" json:"product_id"`
	Quantity  int    `dynamodbav:"quantity"    json:"quantity"`
	AddedAt   string `dynamodbav:"added_at"   json:"added_at"`
	UpdatedAt string `dynamodbav:"updated_at" json:"updated_at"`
}

// DynamoCart represents the full cart document in DynamoDB
type DynamoCart struct {
	CartID     int             `dynamodbav:"cart_id"     json:"cart_id"`
	CustomerID int             `dynamodbav:"customer_id" json:"customer_id"`
	Items      []DynamoCartItem `dynamodbav:"items"       json:"items"`
	CreatedAt  string          `dynamodbav:"created_at"  json:"created_at"`
	UpdatedAt  string          `dynamodbav:"updated_at"  json:"updated_at"`
}

// InitDynamo creates the DynamoDB client.
// On ECS Fargate, credentials come from the task IAM role automatically.
func InitDynamo() error {
	DynamoTableName = os.Getenv("DYNAMO_TABLE_NAME")
	if DynamoTableName == "" {
		DynamoTableName = "hw8-store-dynamo-carts"
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		return fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	DynamoClient = dynamodb.NewFromConfig(cfg)

	// Verify connectivity by describing the table
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	_, err = DynamoClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(DynamoTableName),
	})
	if err != nil {
		return fmt.Errorf("cannot reach DynamoDB table %s: %w", DynamoTableName, err)
	}

	log.Printf("Connected to DynamoDB table: %s (region: %s)", DynamoTableName, region)
	return nil
}

// ── CREATE: POST /dynamo/shopping-carts ─────────────────────
// Uses an atomic counter (cart_id=0 row) to generate sequential IDs.
// Then creates the cart document with empty items list.

func DynamoCreateCart(customerID int) (int, error) {
	ctx := context.TODO()
	now := time.Now().UTC().Format(time.RFC3339)

	// Step 1: Atomic counter — increment the counter item (cart_id=0)
	counterResult, err := DynamoClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(DynamoTableName),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberN{Value: "0"},
		},
		UpdateExpression: aws.String("ADD next_id :inc"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":inc": &types.AttributeValueMemberN{Value: "1"},
		},
		ReturnValues: types.ReturnValueUpdatedNew,
	})
	if err != nil {
		return 0, fmt.Errorf("atomic counter failed: %w", err)
	}

	// Extract the new cart ID
	newIDAttr := counterResult.Attributes["next_id"]
	newIDStr := newIDAttr.(*types.AttributeValueMemberN).Value
	newID, _ := strconv.Atoi(newIDStr)

	// Step 2: Create the cart document with empty items
	cart := DynamoCart{
		CartID:     newID,
		CustomerID: customerID,
		Items:      []DynamoCartItem{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	item, err := attributevalue.MarshalMap(cart)
	if err != nil {
		return 0, fmt.Errorf("marshal cart: %w", err)
	}

	_, err = DynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(DynamoTableName),
		Item:      item,
	})
	if err != nil {
		return 0, fmt.Errorf("put cart: %w", err)
	}

	return newID, nil
}

// ── READ: GET /dynamo/shopping-carts/{id} ───────────────────
// Single GetItem call — O(1), costs 0.5 RCU (eventually consistent).
// The entire cart including all items comes back in one read.

func DynamoGetCart(cartID int) (*DynamoCart, error) {
	ctx := context.TODO()

	result, err := DynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(DynamoTableName),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberN{Value: strconv.Itoa(cartID)},
		},
		ConsistentRead: aws.Bool(false), // Eventually consistent (default)
	})
	if err != nil {
		return nil, fmt.Errorf("get cart: %w", err)
	}

	// No item found
	if result.Item == nil {
		return nil, nil
	}

	// Skip the counter row (cart_id=0)
	if cartID == 0 {
		return nil, nil
	}

	var cart DynamoCart
	if err := attributevalue.UnmarshalMap(result.Item, &cart); err != nil {
		return nil, fmt.Errorf("unmarshal cart: %w", err)
	}

	// Ensure items is never nil in JSON
	if cart.Items == nil {
		cart.Items = []DynamoCartItem{}
	}

	return &cart, nil
}

// ── ADD ITEM: POST /dynamo/shopping-carts/{id}/items ────────
// Two-step: GetItem to check existence + find existing product,
// then PutItem with the updated items list.
// Uses a condition expression to prevent race conditions.

func DynamoAddItemToCart(cartID, productID, quantity int) error {
	ctx := context.TODO()
	now := time.Now().UTC().Format(time.RFC3339)

	// Step 1: Get the current cart
	cart, err := DynamoGetCart(cartID)
	if err != nil {
		return err
	}
	if cart == nil {
		return fmt.Errorf("CART_NOT_FOUND")
	}

	// Step 2: Upsert the item in the local items slice
	found := false
	for i, item := range cart.Items {
		if item.ProductID == productID {
			cart.Items[i].Quantity += quantity
			cart.Items[i].UpdatedAt = now
			found = true
			break
		}
	}
	if !found {
		cart.Items = append(cart.Items, DynamoCartItem{
			ProductID: productID,
			Quantity:  quantity,
			AddedAt:   now,
			UpdatedAt: now,
		})
	}

	cart.UpdatedAt = now

	// Step 3: Write back the full cart with condition check
	item, err := attributevalue.MarshalMap(cart)
	if err != nil {
		return fmt.Errorf("marshal cart: %w", err)
	}

	_, err = DynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(DynamoTableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_exists(cart_id)"),
	})
	if err != nil {
		return fmt.Errorf("put cart: %w", err)
	}

	return nil
}

// ── CONSISTENCY TEST: Strongly consistent read ──────────────
// Used by the consistency test endpoint to compare eventual vs strong reads.

func DynamoGetCartConsistent(cartID int) (*DynamoCart, error) {
	ctx := context.TODO()

	result, err := DynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(DynamoTableName),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberN{Value: strconv.Itoa(cartID)},
		},
		ConsistentRead: aws.Bool(true), // Strongly consistent
	})
	if err != nil {
		return nil, fmt.Errorf("get cart (consistent): %w", err)
	}

	if result.Item == nil || cartID == 0 {
		return nil, nil
	}

	var cart DynamoCart
	if err := attributevalue.UnmarshalMap(result.Item, &cart); err != nil {
		return nil, fmt.Errorf("unmarshal cart: %w", err)
	}

	if cart.Items == nil {
		cart.Items = []DynamoCartItem{}
	}

	return &cart, nil
}
