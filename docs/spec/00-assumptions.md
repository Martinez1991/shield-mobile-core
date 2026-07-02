# 00 — Premissas e Perguntas Abertas

Esta documentação descreve a **plataforma-alvo**. Onde o `shield-platform.md` ou o código atual não determinam uma decisão, adotamos as premissas abaixo (marcadas para revisão pelos stakeholders).

## Premissas adotadas

| ID | Área | Premissa | Alternativa considerada |
|----|------|----------|-------------------------|
| PR-01 | Entrega | **SaaS multi-tenant** é o alvo primário; on-prem/híbrido são secundários (V-Enterprise). | On-prem-first (regulados) |
| PR-02 | Cloud | **AWS** como provedor de referência (EKS, RDS, MSK, S3, KMS, CloudFront). Abstração via Terraform para portabilidade GKE/AKS. | GCP / Azure / multi-cloud |
| PR-03 | Linguagens | Engines em **Rust**; control/build plane e CLI em **Go**; IA em **Python**; SDK Android **Kotlin/C**, iOS **Swift/C**; dashboard **React/Next.js + TS**. | Conforme §16 do doc |
| PR-04 | Billing/PCI | Pagamentos via **Stripe** (redirect/Elements). Plataforma **não armazena PAN** → escopo **PCI-DSS SAQ-A**. | Adyen; billing próprio (fora de escopo) |
| PR-05 | IdP | **Keycloak** (self-host) para IAM; federação SAML/OIDC (Okta/Azure AD/Google). | Auth0/Cognito |
| PR-06 | Segredos/Chaves | **Vault** (segredos dinâmicos) + **KMS** (cloud) / **HSM PKCS#11** (on-prem). Chaves de assinatura nunca saem do broker. | AWS Secrets Manager |
| PR-07 | Mensageria | **Kafka (MSK)** backbone + **NATS** para filas de baixa latência. | RabbitMQ |
| PR-08 | Dados | **PostgreSQL** (source of truth), **ClickHouse** (telemetria RASP), **Redis** (cache/locks), **Elasticsearch/OpenSearch** (logs). | — |
| PR-09 | Isolamento worker | Workers de build em **gVisor** (sandbox), sem egress, FS efêmero (P5). | Kata Containers |
| PR-10 | Retenção | Binários do cliente: **TTL curto + purge pós-build**; opção *zero-retention* (híbrido/on-prem). | Retenção configurável por tenant |
| PR-11 | Regiões | MVP **single-region** (us-east-1 / sa-east-1 para Brasil-LGPD); multi-region em V2+. | Multi-region desde o início |
| PR-12 | IA | `ai-svc` com **GNN + gradient boosting** para risco + **LLM pequeno fine-tuned** para PII/segredos, *self-hosted* (sem enviar código do cliente a terceiros). | LLM gerenciado (viola PR-10) |
| PR-13 | Compliance | Metas: **LGPD** (obrigatório BR), **ISO 27001** e **SOC 2 Tipo II** (V-Enterprise), **SLSA L3** para supply chain. | — |
| PR-14 | Assinatura iOS | Fluxo padrão entrega **artefato para o cliente assinar** (certificado Apple do cliente); re-assinatura só quando o cliente delega. | Re-assinatura sempre |

## Perguntas abertas (requerem decisão de negócio)

| # | Pergunta | Impacto | Recomendação |
|---|----------|---------|--------------|
| Q1 | Mercado inicial: BR-first (LGPD) ou US/EU (GDPR)? | Região, compliance, i18n | BR-first (sa-east-1), preparar GDPR |
| Q2 | Modelo de preço: por build, por app, por seat ou usage-based? | billing-svc, quotas | Usage-based (por build) + tiers |
| Q3 | iOS no MVP ou só V2? | Esforço, go-to-market | V2 (Android-first, §17 do doc) |
| Q4 | On-prem air-gapped é requisito de venda inicial? | Empacotamento Helm/OCI, licença offline | Só V-Enterprise |
| Q5 | SLA contratual alvo (99.9% vs 99.95%)? | Arquitetura HA/multi-AZ/region, custo | 99.9% control plane no MVP |
| Q6 | Haverá processamento de dados de usuários finais do app (telemetria RASP com IP/device)? | LGPD/DPIA, anonimização | Pseudonimizar no ingest; DPIA |

## Gap: implementado vs documentado

| Camada | Documentado | Implementado (v0.1.0) | Gap |
|--------|-------------|-----------------------|-----|
| Engine ofuscação (§3) | Completo | Rename, strings XOR/AES, VM, reorder, opaque, junk, RASP inject | ~60% do §3, sem corretude runtime |
| RASP (§6) | SDK nativo, 18 módulos | Smali toy (root/debug/emulador) | ~10% |
| VM (§8) | Polimórfica, aninhada | Int static straight-line | ~15% |
| Control plane (§1.2) | 17 microsserviços | — | 0% |
| API/Dashboard (§10-11) | REST+GraphQL+SPA | — | 0% |
| Infra/Observab. (§1.3,§21) | K8s/Kafka/OTel | — | 0% |
| IA (§9) | Risk map | — | 0% |

> **Regra de ouro (do committee review):** não construir novas features sem antes fechar os 3 riscos críticos (RC1 corretude runtime, RC2 keep-rules de manifesto, RC4 strings reversíveis). Ver [issues #3–#8](https://github.com/Martinez1991/shield-platform/issues).
