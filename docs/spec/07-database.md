# 07 — Banco de Dados

## Topologia (database-per-service)

| Serviço | Store | Papel |
|---------|-------|-------|
| iam/tenant/license/billing/policy/job/report | **PostgreSQL** (schema por serviço ou instância) | Source of truth transacional |
| telemetry | **ClickHouse** | Telemetria RASP (alto volume/analítico) |
| job/gateway | **Redis** | Cache, locks distribuídos, rate-limit, dedup |
| logs | **OpenSearch/Elasticsearch** | Busca/observabilidade |
| artefatos | **S3/MinIO** | Object store (SSE-KMS, lifecycle/TTL) |

> **Premissa:** MVP pode usar **1 cluster Postgres** com **schema-per-service** (custo), evoluindo para instâncias separadas em V2. RLS por `tenant_id` como defesa adicional.

## Tabelas principais (Postgres)
Ver DDL completo em [05-data-model.md](05-data-model.md). Núcleo: `tenant, user, role, permission, app, signing_key_ref, policy, build, applied_protection, security_report, build_event, field_telemetry(ref), api_key, license, audit_log`.

## Tipos e convenções
- PK `uuid` (`gen_random_uuid()`); timestamps `timestamptz` (UTC); dinheiro em `numeric(12,4)`; e-mail `citext`; documentos flexíveis em `jsonb` (spec de policy, evidence, metrics).
- Enums via `CHECK` (portabilidade) em vez de `ENUM` nativo.
- FKs com `ON DELETE CASCADE` para dados dependentes do tenant.

## Migrações e versionamento
- Ferramenta: **golang-migrate** (Go) / **sqlx-migrate**; um diretório por serviço (`/migrations`).
- Nomenclatura: `NNNN_descricao.up.sql` / `.down.sql`; imutáveis após merge.
- **Expand-and-contract** para zero-downtime: (1) adiciona coluna/tabela → (2) deploy código dual-read/write → (3) backfill → (4) remove antigo em release seguinte.
- Migrações rodam em job de deploy (init container), com lock; rollback via `.down` testado em staging.

## Estratégia de dados
- **Retenção:** `build_event` particionado mensal; binários/artefatos com TTL curto + purge; telemetria TTL 180d (ClickHouse).
- **Backup/PITR:** WAL archiving + snapshot diário (RPO 15 min / RTO 4 h — ver NFR).
- **Anonimização:** telemetria pseudonimizada no ingest (sem IP/IMEI em claro) — LGPD.
- **Índices:** ver tabela em [05-data-model.md](05-data-model.md#indices-constraints-particionamento).
