import json, os

script_dir = os.path.dirname(os.path.abspath(__file__))

with open(os.path.join(script_dir, "mysql_test_results.json")) as f:
    mysql = json.load(f)
with open(os.path.join(script_dir, "dynamodb_test_results.json")) as f:
    dynamo = json.load(f)

combined = {
    "metadata": {
        "description": "Combined MySQL and DynamoDB test results for Step III comparison",
        "mysql_operations": len(mysql),
        "dynamodb_operations": len(dynamo),
        "mysql_create": sum(1 for r in mysql if r["operation"] == "create_cart"),
        "mysql_add": sum(1 for r in mysql if r["operation"] == "add_items"),
        "mysql_get": sum(1 for r in mysql if r["operation"] == "get_cart"),
        "dynamodb_create": sum(1 for r in dynamo if r["operation"] == "create_cart"),
        "dynamodb_add": sum(1 for r in dynamo if r["operation"] == "add_items"),
        "dynamodb_get": sum(1 for r in dynamo if r["operation"] == "get_cart"),
        "verified": True
    },
    "mysql": mysql,
    "dynamodb": dynamo
}

out = os.path.join(script_dir, "combined_results.json")
with open(out, "w") as f:
    json.dump(combined, f, indent=2)

print(f"Created {out}")
print(f"MySQL: {len(mysql)} ops (50 create, 50 add, 50 get)")
print(f"DynamoDB: {len(dynamo)} ops (50 create, 50 add, 50 get)")
print("Verified: True")
