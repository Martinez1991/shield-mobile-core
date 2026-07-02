# 06 — APIs

REST para recursos/CI-CD (previsível, cacheável); GraphQL para o dashboard. Ambas atrás do BFF, OAuth2/JWT, rate-limit por tenant. Spec formal: [`api/openapi.yaml`](api/openapi.yaml).

## Convenções

| Aspecto | Padrão |
|---------|--------|
| Versionamento | Prefixo de path `/v1`; *breaking changes* → `/v2` (suporte N-1) |
| Autenticação | `Authorization: Bearer <JWT>` (OIDC) ou API key `X-API-Key` |
| Idempotência | `Idempotency-Key` (POST de build/sign) |
| Paginação | *cursor-based* (`?first=&after=`) |
| Erros | RFC 9457 *Problem Details* (`application/problem+json`) |
| Rate limit | Headers `RateLimit-Limit/Remaining/Reset`; 429 ao exceder |
| Correlação | `X-Request-Id` (propagado no trace) |

## Endpoints REST (núcleo)

| Método | URL | Descrição | Authz | Rate |
|--------|-----|-----------|-------|------|
| POST | `/v1/apps/{appId}/builds` | Inicia build (upload resumable) | `build:create` | 60/min |
| GET | `/v1/builds/{buildId}` | Status + estados | `build:read` | 600/min |
| GET | `/v1/builds/{buildId}/artifact` | URL pré-assinada | `build:read` | 60/min |
| GET | `/v1/builds/{buildId}/mapping` | Mapping cifrado (retrace) | `build:mapping` | 30/min |
| GET | `/v1/builds/{buildId}/report` | Report + SBOM | `build:read` | 60/min |
| GET | `/v1/builds/{buildId}/logs` | Logs paginados | `build:read` | 120/min |
| POST | `/v1/builds/{buildId}/sign` | Dispara assinatura (HSM broker) | `build:sign` | 30/min |
| POST | `/v1/policies` | Cria/atualiza policy | `policy:write` | 30/min |
| GET | `/v1/policies` | Lista policies | `policy:read` | 300/min |
| GET | `/v1/licenses` | Entitlements/quotas | `license:read` | 300/min |
| POST/GET | `/v1/users` | Gestão de usuários | `user:*` | 60/min |
| GET | `/v1/protections` | Catálogo de técnicas | `catalog:read` | 300/min |

### Exemplo — criar build
```http
POST /v1/apps/app_123/builds
Authorization: Bearer <jwt>
Idempotency-Key: 5f3c...
Content-Type: application/json

{ "policyId": "pol_prod_high", "artifactUploadId": "up_abc", "platform": "android" }
```
```http
202 Accepted
{ "buildId": "bld_789", "status": "QUEUED", "statusUrl": "/v1/builds/bld_789" }
```

### Erros (padrão RFC 9457)
```json
{ "type": "https://errors.shield.dev/entitlement-exceeded",
  "title": "Quota de builds excedida", "status": 402,
  "detail": "Plano free permite 10 builds/mês", "instance": "/v1/apps/app_123/builds" }
```

| Status | Quando |
|--------|--------|
| 400 | Validação de payload |
| 401 / 403 | Sem token / sem permissão |
| 402 | Entitlement/quota excedida |
| 404 | Recurso inexistente/tenant sem acesso |
| 409 | Conflito de idempotência |
| 422 | Policy inválida (validação semântica) |
| 429 | Rate limit |
| 5xx | Erro interno (com `X-Request-Id`) |

## GraphQL (dashboard)
```graphql
type Query {
  app(id: ID!): App
  build(id: ID!): Build
  builds(appId: ID!, first: Int, after: String): BuildConnection!
  policies: [Policy!]!
  fieldTelemetry(appId: ID!, range: TimeRange!): TelemetrySummary!
}
type Mutation {
  createBuild(input: CreateBuildInput!): Build!
  upsertPolicy(input: PolicyInput!): Policy!
  rotateApiKey(id: ID!): ApiKey!
}
enum BuildStatus { QUEUED ANALYZING PLANNING PROTECTING REPACKAGING SIGNING READY FAILED }
```

## Governança de API
- Contrato *design-first* (OpenAPI/GraphQL SDL versionado no repo); *linting* (Spectral); *breaking-change check* no CI; portal de docs; SDKs gerados; testes de contrato (Pact) entre serviços.
