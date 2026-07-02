# 19 — Custos

> **Premissa:** ordens de grandeza para planejamento (AWS, us-east-1/sa-east-1), **não cotação**. Validar com Calculator/negociação. Câmbio e reserva (Savings Plans) alteram materialmente.

## Custo de infraestrutura (SaaS, mensal aproximado)

| Item | MVP (single-region, baixa carga) | V1/V2 (produção, carga média) |
|------|----------------------------------|-------------------------------|
| EKS (control plane + nós baseline) | US$ 300–800 | US$ 1.5k–4k |
| Workers de build (spot, CPU-intensivo) | US$ 200–1k (sob demanda) | US$ 2k–15k (elástico) |
| RDS PostgreSQL (Multi-AZ) | US$ 150–400 | US$ 600–2k |
| MSK (Kafka) | US$ 250–500 | US$ 1k–3k |
| ElastiCache Redis | US$ 60–150 | US$ 300–800 |
| ClickHouse (telemetria) | US$ 100–300 | US$ 500–2k |
| S3 + CloudFront + transfer | US$ 50–200 | US$ 300–1.5k |
| OpenSearch (logs) | US$ 150–400 | US$ 600–2k |
| KMS/Secrets/WAF/Shield | US$ 50–200 | US$ 300–1k |
| Observabilidade (Grafana/Prometheus self-host) | incl. nós | US$ 300–1k |
| **Total infra** | **~US$ 1.5k–4k/mês** | **~US$ 8k–35k/mês** |

## Licenciamento / SaaS de terceiros
- Stripe (% por transação), device farm (Firebase Test Lab/BrowserStack: US$ 100–2k), IdP (Keycloak self-host ~$0 lic.), scanners (muitos OSS: CodeQL/Trivy/Semgrep CE). Certificações (SOC2/ISO): US$ 15k–60k/ano (auditoria) — fase Enterprise.

## Equipe (referência §22 do doc: ~145–210 PM)
- Time-alvo: 10–12 engenheiros sênior (Rust/Go/mobile/ML/SRE/security) + PM + UX + Compliance.
- Custo de pessoal domina o TCO (ordem de grandeza: US$ 1.5M–3M/ano no Brasil; múltiplo em US/EU) — muito acima da infra.

## Direcionadores de custo & otimização

| Direcionador | Otimização | Economia |
|--------------|-----------|----------|
| Workers CPU-intensivos | **Spot** + bin-packing por perfil (VM-heavy vs rename-light) | 50–80% em compute de build |
| Reprocessamento | **Cache content-addressed** (libs/.so por hash) | Evita rebuild |
| Overhead de build longo | Budget no Planner + IA (VM só top-k) | Menos compute/tamanho |
| Dados de logs/telemetria | Tiering/TTL (OpenSearch/ClickHouse) | 30–60% storage |
| RDS sobredimensionado | Right-sizing + Savings Plans/Reserved | 30–50% |
| Multi-region prematuro | Adiar para V2 (Q5/SLA) | Evita duplicação |

## Eficiência financeira
- **Unit economics:** medir **custo por build** (métrica `shield_build_cost`) e precificar usage-based acima do custo marginal com margem-alvo.
- **FinOps:** tags por tenant/ambiente; alertas de budget; relatório de custo por build no dashboard (§10).
