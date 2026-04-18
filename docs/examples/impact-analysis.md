# Impact Analysis

Answer "what breaks if I change X?" without reading any files.

## "What happens if I shut down the database?"

**Tool call:**
```
traverse_graph(entity="database", direction="incoming", depth=3)
→ {
    "entities": [
      {"name": "billing-service", "distance": 1},
      {"name": "checkout-service", "distance": 1},
      {"name": "frontend-app", "distance": 2}
    ]
  }
```

**Claude response:** "Shutting down `database` will directly impact billing-service and checkout-service. frontend-app has an indirect dependency via checkout-service."

## "What is the blast radius of changing the billing API?"

**Tool call:**
```
find_path(from="frontend-app", to="billing-service")
→ path: frontend-app → checkout-service → billing-service (length: 2)

traverse_graph(entity="Billing API", direction="incoming", relation_type="exposes_api")
→ [billing-service]
```
