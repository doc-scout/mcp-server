# Dependency Audits

Find services with risky or outdated dependencies.

## "Which Go services depend on pgx directly?"

**Tool call:**
```
search_nodes(query="pgx")
→ Entity: pgx (service — dependency node)

traverse_graph(entity="pgx", direction="incoming", relation_type="depends_on")
→ [billing-service (distance 1), auth-service (distance 1)]
```

**Claude response:** "billing-service and auth-service have a direct depends_on edge to pgx."

## "Which services have no OpenAPI spec?"

**Tool call:**
```
list_entities(type="service")
→ [billing-service, checkout-service, payment-worker, frontend-app, auth-service]

list_entities(type="api")
→ [Billing API]

list_relations(type="exposes_api")
→ [billing-service → exposes_api → Billing API]
```

**Claude response:** "checkout-service, payment-worker, frontend-app, and auth-service have no OpenAPI spec (no exposes_api relation found)."
