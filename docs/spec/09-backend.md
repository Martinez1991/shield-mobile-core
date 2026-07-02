# 09 — Backend

## Serviços (bounded contexts)

| Serviço | Stack | Responsabilidade | Comunicação |
|---------|-------|------------------|-------------|
| `gateway` (BFF) | Go + Envoy | authz edge, rate-limit, GraphQL/REST | REST/GraphQL |
| `iam-svc` | Go + PG | Auth, OIDC, MFA, sessões | gRPC/REST |
| `tenant-svc` | Go + PG | Tenants, usuários, RBAC | gRPC + eventos |
| `policy-svc` | Rust + PG | Validação/versionamento/assinatura de policy | gRPC |
| `license-svc` | Go + PG | Licenças, quotas, entitlements | eventos |
| `billing-svc` | Go + PG | Uso, faturamento (Stripe) | eventos |
| `job-svc` | Go + PG + Redis | Ciclo de vida do build, event-sourcing, retry | gRPC + Kafka |
| `analyzer-svc` | Rust/C++ | Descompilação, análise estática, features | consome fila |
| `obfuscator-svc` | **Rust** (semente: Go v0.1.0) | Ofuscação Dalvik/IR | consome fila |
| `native-svc` | C++/Rust (LLVM) | Proteção C/C++/Rust/Swift/ObjC | consome fila |
| `vm-svc` | Rust | Virtualização (bytecode) | consome fila |
| `rasp-svc` | Rust + Go | Injeção SDK RASP + config | consome fila |
| `repackage-svc` | Go + toolchains | Recompilação, alinhamento, assinatura | consome fila |
| `signer-broker` | Go | Assina via KMS/HSM sem expor a chave | gRPC (interno) |
| `ai-svc` | Python (Triton/ONNX) | Risk map, PII/segredos | gRPC |
| `telemetry-svc` | Go + ClickHouse | Ingest de RASP callbacks | HTTP ingest |
| `report-svc` | Go + PG | Reports, SBOM, evidências | eventos |
| `notify-svc` | Go | Webhooks, e-mail, Slack, CI callbacks | eventos |

## Camadas (por serviço — Hexagonal/Clean)

```
cmd/            entrypoint
internal/
  domain/       entidades, regras, ports (interfaces)
  app/          casos de uso (orquestração)
  adapters/
    http/       controllers/handlers
    grpc/       handlers
    repo/       repositórios (Postgres/Redis)
    queue/      produtores/consumidores Kafka/NATS
    ext/        Stripe, KMS, Vault, object store
  platform/     telemetria, config, health
```
- **Controllers**: validação de entrada, authz, mapeamento p/ casos de uso.
- **Casos de uso (app)**: regras de aplicação, transações, publicação de eventos.
- **Repositories**: acesso a dados atrás de *ports*; sem lógica de negócio.
- **Middlewares**: authn/z, rate-limit, request-id/trace, recovery, RLS context, idempotência.

## Workers / Jobs / Filas
- **Worker de build**: consome `job.created`, roda o pipeline de passes em pod efêmero (gVisor), publica progresso (`build_event`) e `artifact.ready`. *Checkpoint* da IR em object store para retry granular.
- **Filas**: Kafka (backbone, particionado por tenant), NATS (sinalização/fan-out), Redis (locks/dedup/rate-limit).
- **Jobs agendados**: purge de binários/artefatos (TTL), rotação de chaves, recomputo de risco (feedback loop), reconciliação de billing.

## Cache
- Redis: sessão, rate-limit, *distributed locks* (idempotência), cache de análise (memoização por content-hash de libs/.so).
- HTTP: `ETag`/`Cache-Control` em catálogos (`/protections`).

## Padrões de resiliência
- Timeouts + retries com backoff/jitter; circuit breaker (mesh); *bulkhead* por pool de worker; *outbox pattern* para publicação transacional de eventos; DLQ para mensagens venenosas; *sagas* no fluxo build→sign.

## O engine (obfuscator-svc) — estado e evolução
- **Hoje (v0.1.0, Go):** passes sobre smali-texto (rename, strings XOR/AES, VM int, reorder, opaque, junk, RASP inject), validados montando em DEX.
- **Alvo:** port para **Rust** + **IR tipada** (ADR-0005), keep-rules de manifesto, gate de corretude (differential testing), fuzzing dos parsers. Ver [issues #3–#20](https://github.com/Martinez1991/shield-platform/issues).
