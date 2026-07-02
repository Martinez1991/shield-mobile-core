# 03 — Requisitos Não Funcionais

## Performance

| ID | Requisito | Alvo |
|----|-----------|------|
| RNF-PERF-01 | Latência API (leitura) | P99 < 300 ms |
| RNF-PERF-02 | Latência build (app médio ~50 MB) | P95 < 5 min |
| RNF-PERF-03 | Overhead runtime do app protegido (startup) | < 15% (budget por policy) |
| RNF-PERF-04 | Overhead de tamanho do artefato | < 20% (VM cirúrgica) |
| RNF-PERF-05 | Throughput de builds concorrentes | ≥ 1.000 simultâneos (V2) |

## Escalabilidade
- Workers *stateless/efêmeros*, autoscaling **KEDA por lag de fila** + HPA por CPU (RNF-SCAL-01).
- Particionamento Kafka por tenant (fairness) (RNF-SCAL-02).
- *Content-addressed cache* de análise de libs comuns (RNF-SCAL-03).
- Metas por escala em [15-testing.md](15-testing.md) e §7 do committee review.

## Disponibilidade / HA

| Métrica | Control plane | Build plane |
|---------|---------------|-------------|
| SLA (contratual) | **99.9%** (MVP) → 99.95% (V2) | 99.5% |
| Topologia | Multi-AZ (MVP), Multi-region (V2) | Filas duráveis + retry |
| Degradação graciosa | Fila absorve picos; builds enfileiram | — |

## SLA / SLO / SLI

| SLO | SLI (medição) | Alvo | Error budget |
|-----|---------------|------|--------------|
| Disponibilidade API | % de requisições 2xx/3xx sobre total (exclui 4xx cliente) | 99.9%/30d | 43 min/mês |
| Latência API | P99 de `http.server.duration` | < 300 ms | 1% acima |
| Sucesso de build | builds `READY` / (READY+FAILED sistêmico) | ≥ 99% | 1% |
| Corretude (golden) | golden apps sem divergência | 100% | 0 (bloqueia release) |

## Segurança (resumo; detalhes em [12-security.md](12-security.md))
- OAuth2/OIDC + MFA; RBAC/ABAC deny-by-default; JWT curto + refresh rotativo.
- TLS 1.3 em trânsito, mTLS interno (service mesh); AES-256 SSE-KMS em repouso; envelope encryption por tenant.
- Segredos em Vault; chaves em KMS/HSM via Signer Broker; worker isolado (gVisor), sem egress.
- Alinhado a **OWASP ASVS L2** (control plane) e **OWASP MASVS** (SDKs/output).

## Observabilidade (detalhes em [14-observability.md](14-observability.md))
- Logs estruturados JSON correlacionados por `build_id`/`trace_id` (binários *scrubbed*).
- Métricas Prometheus por estágio; tracing OTel distribuído; 4º pilar: **observabilidade da proteção** (RASP callbacks).

## Auditoria
- Audit log **append-only, hash-chained** (imutabilidade verificável), retenção ≥ 1 ano, exportável (RNF-AUD-01).

## LGPD (Brasil)
| Controle | Implementação |
|----------|---------------|
| Base legal | Contrato (B2B); telemetria de campo por *opt-in* |
| Minimização | Binários com TTL curto + purge; telemetria pseudonimizada |
| Titular | Endpoints de acesso/eliminação; DPA por tenant |
| DPIA | Obrigatória para telemetria de campo (Q6) |
| Transferência internacional | Região BR (sa-east-1) para dados BR; cláusulas-padrão |
| Encarregado (DPO) | Nomeado; canal de contato |

## PCI DSS
- **SAQ-A** (não armazena/processa PAN; Stripe hospeda pagamento). Sem CHD na plataforma → escopo mínimo. Reavaliar se billing próprio (fora de escopo).

## ISO 27001 / SOC 2
- SoA (Statement of Applicability) mapeando Anexo A; políticas de acesso, mudança, incidentes, BCP; SOC 2 Tipo II (Security, Availability, Confidentiality) em V-Enterprise.

## OWASP ASVS / Top 10 / MASVS
- **ASVS L2** para APIs; **Top 10** endereçado em [12-security.md](12-security.md); **MASVS-L2 + MASVS-R (resiliência)** como *benchmark do output* protegido.

## Resiliência / Backup / DR

| Item | Definição |
|------|-----------|
| Backup Postgres | PITR (WAL) + snapshot diário; retenção 30d |
| Backup ClickHouse | Snapshot diário; TTL analítico |
| Object store | Versionamento + replicação cross-region (V2) |
| Chaos | Matar worker mid-build → retry idempotente (RNF-RES-01) |

| Objetivo | Alvo (MVP) | Alvo (V2) |
|----------|-----------|-----------|
| **RTO** (control plane) | 4 h | 1 h |
| **RPO** (metadados) | 15 min | 5 min |
| **RTO** (build plane) | Best-effort (rebuild) | 30 min |
| **RPO** (artefatos) | 0 (regeneráveis do input) | 0 |

> **Premissa:** artefatos protegidos são **regeneráveis** a partir do input + policy + versão de engine (P2 determinismo) → RPO de artefato = 0 sem backup do output.
