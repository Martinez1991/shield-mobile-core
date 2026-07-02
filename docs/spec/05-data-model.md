# 05 — Modelagem de Dados

## Modelo conceitual
Tenant possui Usuários, Apps e Licenças. App produz Builds sob uma Policy. Build gera Report, registra Applied Protections e Build Events. App recebe Field Telemetry. Tenant escreve Audit Log.

## MER (lógico)

```mermaid
erDiagram
  TENANT ||--o{ USER : has
  TENANT ||--o{ APP : owns
  TENANT ||--o{ LICENSE : holds
  TENANT ||--o{ API_KEY : issues
  TENANT ||--o{ AUDIT_LOG : writes
  USER }o--o{ ROLE : assigned
  ROLE ||--o{ PERMISSION : grants
  APP ||--o{ BUILD : produces
  APP ||--o{ SIGNING_KEY_REF : uses
  POLICY ||--o{ BUILD : applied_to
  BUILD ||--o{ APPLIED_PROTECTION : records
  BUILD ||--|| SECURITY_REPORT : generates
  BUILD ||--o{ BUILD_EVENT : logs
  APP ||--o{ FIELD_TELEMETRY : receives

  TENANT { uuid id PK; string name; string plan; jsonb limits; timestamptz created_at }
  USER { uuid id PK; uuid tenant_id FK; citext email; bool mfa_enabled; timestamptz last_login }
  ROLE { uuid id PK; uuid tenant_id FK; string name }
  PERMISSION { uuid id PK; string resource; string action }
  APP { uuid id PK; uuid tenant_id FK; string bundle_id; string platform; timestamptz created_at }
  SIGNING_KEY_REF { uuid id PK; uuid app_id FK; string provider; string key_uri }
  POLICY { uuid id PK; uuid tenant_id FK; int version; jsonb spec; bytea signature; timestamptz created_at }
  BUILD { uuid id PK; uuid app_id FK; uuid policy_id FK; string status; jsonb metrics; timestamptz created_at }
  APPLIED_PROTECTION { uuid id PK; uuid build_id FK; string technique; jsonb evidence }
  SECURITY_REPORT { uuid id PK; uuid build_id FK; jsonb coverage; jsonb sbom }
  BUILD_EVENT { uuid id PK; uuid build_id FK; string type; jsonb payload; timestamptz ts }
  FIELD_TELEMETRY { uuid id PK; uuid app_id FK; string signal; string device_class; string region; timestamptz ts }
  API_KEY { uuid id PK; uuid tenant_id FK; string prefix; bytea hash; jsonb scopes; timestamptz expires_at }
  LICENSE { uuid id PK; uuid tenant_id FK; jsonb entitlements; timestamptz valid_until }
  AUDIT_LOG { uuid id PK; uuid tenant_id FK; string actor; string action; jsonb data; bytea prev_hash; bytea hash; timestamptz ts }
```

## Modelo físico (PostgreSQL, DDL de referência)

```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "citext";

CREATE TABLE tenant (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  plan text NOT NULL DEFAULT 'free',
  limits jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  bundle_id text NOT NULL,
  platform text NOT NULL CHECK (platform IN ('android','ios')),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, bundle_id, platform)
);

CREATE TABLE policy (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  version int NOT NULL,
  spec jsonb NOT NULL,
  signature bytea NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, version)
);

CREATE TABLE build (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id uuid NOT NULL REFERENCES app(id) ON DELETE CASCADE,
  policy_id uuid NOT NULL REFERENCES policy(id),
  status text NOT NULL DEFAULT 'QUEUED'
    CHECK (status IN ('QUEUED','ANALYZING','PLANNING','PROTECTING','REPACKAGING','SIGNING','READY','FAILED')),
  idempotency_key text,
  metrics jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE build_event (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  build_id uuid NOT NULL REFERENCES build(id) ON DELETE CASCADE,
  type text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}',
  ts timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES tenant(id),
  actor text NOT NULL,
  action text NOT NULL,
  data jsonb NOT NULL DEFAULT '{}',
  prev_hash bytea,
  hash bytea NOT NULL,
  ts timestamptz NOT NULL DEFAULT now()
);
```

## Índices, constraints, particionamento

| Objeto | Definição | Motivo |
|--------|-----------|--------|
| `idx_build_app_created` | `build(app_id, created_at DESC)` | Listagem de builds por app |
| `idx_build_status` | `build(status) WHERE status <> 'READY'` (parcial) | Scheduler/monitoração |
| `uq_build_idem` | `UNIQUE (app_id, idempotency_key)` | Idempotência de criação |
| `idx_event_build` | `build_event(build_id, ts)` | Reconstrução event-sourcing |
| RLS | `ROW LEVEL SECURITY` por `tenant_id` | Isolamento multi-tenant (defense-in-depth) |
| Partição | `build_event` por RANGE(ts) mensal | Volume/retenção |

## Triggers / Views / Procedures

```sql
-- Trigger: encadeamento de hash da auditoria (imutabilidade)
CREATE OR REPLACE FUNCTION audit_chain() RETURNS trigger AS $$
DECLARE last_hash bytea;
BEGIN
  SELECT hash INTO last_hash FROM audit_log
    WHERE tenant_id = NEW.tenant_id ORDER BY ts DESC LIMIT 1;
  NEW.prev_hash := last_hash;
  NEW.hash := digest(coalesce(last_hash,'') || NEW.actor || NEW.action || NEW.data::text, 'sha256');
  RETURN NEW;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_audit_chain BEFORE INSERT ON audit_log
  FOR EACH ROW EXECUTE FUNCTION audit_chain();

-- View: builds prontos por app (para dashboard)
CREATE VIEW v_ready_builds AS
  SELECT b.app_id, b.id, b.metrics, b.created_at
  FROM build b WHERE b.status = 'READY';
```

## ClickHouse (telemetria de campo)

```sql
CREATE TABLE field_telemetry (
  app_id UUID, signal LowCardinality(String), device_class LowCardinality(String),
  region LowCardinality(String), app_version String, ts DateTime
) ENGINE = MergeTree
PARTITION BY toYYYYMM(ts) ORDER BY (app_id, signal, ts)
TTL ts + INTERVAL 180 DAY;
```

> **Segredos/chaves nunca ficam nesta base** — apenas *refs* (`key_uri`) para Vault/HSM. `POLICY.spec` é versionada e assinada; `AUDIT_LOG` é hash-chained.
