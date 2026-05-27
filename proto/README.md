# Protocol Buffer Schema Documentation

## Domain Structure

This project follows Domain-Driven Design (DDD) principles with proto files organized by bounded contexts:

```
proto/
├── event/v1/
│   └── event.proto
├── endpoint/v1/
│   └── endpoint.proto
├── delivery/v1/
│   └── delivery.proto
└── common/v1/
    └── common.proto
```

## Design Principles

### 1. Aggregate/Bounded Context Ownership
Proto files are mapped to domain boundaries (Event, Endpoint, Delivery), not database tables
- Event aggregate: Manages event lifecycle and idempotency
- Endpoint aggregate: Manages webhook endpoint configuration
- Delivery aggregate: Manages delivery state and retry logic

### 2. Separate Persistent Models from API Contracts
Proto files represent API contracts, not database schemas
- Domain types in `internal/domain/` contain business logic
- Proto messages represent external-facing contracts
- Mapping layers translate between these layers

### 3. Strong Naming Conventions
- Messages: PascalCase (e.g., `CreateEventRequest`)
- Fields: snake_case (e.g., `idempotency_key`)
- Services: PascalCase with "Service" suffix (e.g., `EventService`)
- Enums: PascalCase with value prefix (e.g., `DELIVERY_STATUS_PENDING`)

### 4. Stable Field Numbering
- Field numbers 1-15: Frequently populated fields
- Field numbers 16-2047: Less common fields
- Reserved ranges: 19000-19999 (protocol buffer reserved)
- Never reuse deleted field numbers

### 5. Deterministic SQL-to-Proto Type Mapping

| SQL Type | Proto Type | Go Type | Notes |
|----------|------------|---------|-------|
| UUID | string | string | Explicit UUID validation |
| TEXT | string | string | No length in proto, validated at runtime |
| JSONB | string | string | Serialized as JSON string |
| BOOLEAN | bool | bool | |
| INT | int32 | int32 | Use appropriate validation |
| TIMESTAMPTZ | google.protobuf.Timestamp | time.Time | Always timezone-aware |
| TEXT[] | repeated string | []string | Array of strings |
| NULL | google.protobuf.*Value | *T | Wrapper types for optional fields |

### 6. Explicit Nullability Rules
- Required fields: Direct type (e.g., `string id = 1`)
- Optional fields: Wrapper types (e.g., `google.protobuf.StringValue secret = 5`)
- Query parameters: Use wrappers for optional filters
- Never rely on proto3's default nullability

### 7. Query Classification Rules

**Read Operations (Query Models)**
- `Get*` - Single entity by identifier
- `List*` - Multiple entities with pagination
- `Count*` - Aggregate counts
- Return optimized read models, not full domain models

**Write Operations (Command Models)**
- `Create*` - Entity creation
- `Update*` - Entity modification
- `Delete*` - Entity deletion
- Separate request/response messages for validation

### 8. Prevent Leaky Relational Modeling
- Use domain concepts, not table structures
- One-to-many: Use repeated fields, not nested messages with IDs
- Many-to-many: Model as domain relationships, not junction tables
- No foreign key fields in proto messages (use logical IDs)

### 9. Explicit Pagination Contracts

**Cursor-based Pagination**
```protobuf
message TimestampCursorPaginationRequest {
  google.protobuf.Timestamp cursor_timestamp = 1;
  string cursor_id = 2;
  int32 limit = 3;
  bool backward = 4;
}
```

**Pagination Response**
```protobuf
message PaginationResponse {
  string next_cursor = 1;
  bool has_more = 2;
  int32 total_count = 3;
}
```

- Never use offset-based pagination (performance issues)
- Cursors must be opaque to clients
- Always include `has_more` flag
- Enforce maximum page size limits

### 10. Version Enforcement
- URL path: `/proto/{domain}/v1/{message}.proto`
- Package name: `webhook.v1`
- Breaking changes: Increment version (v1 -> v2)
- Non-breaking changes: Same version, new optional fields

### 11. Domain-Level Semantic Types

**Event Domain**
- `Event`: Core aggregate representing received webhook events
- Operations: Create, retrieve, list with cursor pagination
- Invariants: Idempotency key uniqueness, topic validation

**Endpoint Domain**
- `Endpoint`: Configuration for webhook delivery targets
- Operations: CRUD management
- Invariants: URL validity, topic subscription, circuit breaker state

**Delivery Domain**
- `Delivery`: State machine for event delivery attempts
- Operations: Creation, status updates, retry queue management
- Invariants: Status transitions, attempt limits, retry backoff

### 12. Explicit Read and Write Models

**Read Models (Query)**
- Optimized for querying and filtering
- Denormalized where appropriate
- Separate from write model operations
- Example: `ListEventsResponse`, `ListEndpointsResponse`

**Write Models (Command)**
- Enforce domain invariants
- Separate request/response for validation
- Never expose internal state
- Example: `CreateEventRequest`, `UpdateEndpointRequest`

### 13. Dangerous Query Constraint Prevention

**Prevented Patterns**
- No `SELECT *` equivalents
- No unbounded result sets (always require limit)
- No wildcard filtering (explicit filter fields only)
- No ad-hoc sorting (fixed sort order only)
- No direct SQL injection vectors

**Enforced Safeguards**
- All pagination requests require `limit` validation
- Cursor values are opaque and server-generated
- Filter fields explicitly typed and validated
- Sort order fixed and documented

## Validation Rules

Using `validate` annotations for runtime validation:

```protobuf
string id = 1 [(validate.rules).string.uuid = true];
string url = 2 [(validate.rules).string.uri = true];
repeated string topics = 3 [(validate.rules).repeated = {min_items: 1, max_items: 50}];
int32 limit = 4 [(validate.rules).int32 = {min: 1, max: 100}];
```

## Service RPC Conventions

### Naming
- `Create*`: Entity creation
- `Get*`: Retrieve single entity
- `List*`: Retrieve multiple entities (paginated)
- `Update*`: Modify existing entity
- `Delete*`: Remove entity
- `Count*`: Aggregate operations

### Error Handling
- Use gRPC status codes
- Include error details in response messages
- Validate inputs before processing
- Handle idempotency for write operations

## Usage Examples

### Creating an Event
```protobuf
CreateEventRequest {
  idempotency_key: "evt_123abc",
  topic: "order.created",
  payload: '{"order_id":"123","total":99.99}'
}
```

### Listing Events with Pagination
```protobuf
ListEventsRequest {
  cursor_received_at: "2024-01-01T00:00:00Z",
  cursor_id: "uuid-here",
  limit: 50,
  topic: "order.created"
}
```

### Updating an Endpoint
```protobuf
UpdateEndpointRequest {
  id: "endpoint-uuid",
  active: true,
  timeout_ms: 10000
}
```

## Migration Guide

When updating proto files:

1. **Non-breaking changes**: Add new optional fields to existing messages
2. **Breaking changes**: Create new version (`v2/`) with updated package
3. **Deprecation**: Mark deprecated fields with `[deprecated = true]`
4. **Compatibility**: Maintain v1 until clients migrate

## References

- [Protocol Buffers Language Guide](https://developers.google.com/protocol-buffers/docs/proto3)
- [API Design Guide](https://cloud.google.com/apis/design)
- [Well-Known Types](https://developers.google.com/protocol-buffers/docs/reference/google.protobuf)