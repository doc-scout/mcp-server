# Ownership Queries

Common questions DocScout answers with a single tool call.

## "Who owns the checkout service?"

Without DocScout, an AI must read CODEOWNERS files across multiple repos. With DocScout:

**Tool call:**
```
search_nodes(query="checkout-service", type="service")
→ Entity: checkout-service (service)
  Relations: checkout-team → owns → checkout-service

open_nodes(names=["checkout-team"])
→ Entity: checkout-team (team)
  Observations: github_handle:@myorg/checkout-team
```

**Claude response:** "The checkout service is owned by @myorg/checkout-team."

Token cost: ~180 tokens vs ~14,776 tokens reading CODEOWNERS from 8 repos.

---

## "Which teams own more than 3 services?"

**Tool call:**
```
list_entities(type="team")
→ [payments-team, checkout-team, platform-team, ...]

traverse_graph(entity="payments-team", relation_type="owns", direction="outgoing")
→ [billing-service, payment-worker, fraud-service, risk-service]
```

**Claude response:** "payments-team owns 4 services: billing-service, payment-worker, fraud-service, risk-service."
