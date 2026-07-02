# SHIELD Platform — Documentação de Engenharia (Enterprise)

> Conjunto de documentos que permite a uma equipe multidisciplinar **desenvolver, testar, implantar e manter** a plataforma SHIELD do zero.
> Padrão de referência: Google/Microsoft/Amazon/Stripe engineering docs.
> **Fonte de verdade do produto:** [`shield-platform.md`](../../shield-platform.md). **Estado atual:** [`committee-review-v0.1.0.md`](../committee-review-v0.1.0.md).

## Como ler

| # | Documento | Público principal |
|---|-----------|-------------------|
| 00 | [Premissas e perguntas abertas](00-assumptions.md) | Todos |
| 01 | [Visão geral, personas e casos de uso](01-overview.md) | Product, Stakeholders |
| 02 | [Requisitos funcionais](02-functional-requirements.md) | Product, Eng |
| 03 | [Requisitos não funcionais (SLA/SLO/SLI, LGPD, ISO)](03-nfr.md) | Eng, SRE, Compliance |
| 04 | [Arquitetura](04-architecture.md) | Arquitetura, Eng |
| 05 | [Modelagem de dados](05-data-model.md) | Eng, DBA |
| 06 | [APIs (REST + GraphQL) + OpenAPI](06-api.md) · [openapi.yaml](api/openapi.yaml) | Eng, Integrações |
| 07 | [Banco de dados](07-database.md) | DBA, Eng |
| 08 | [Front-end](08-frontend.md) | Front, UX |
| 09 | [Backend](09-backend.md) | Backend |
| 10 | [Infraestrutura](10-infrastructure.md) | DevOps, SRE |
| 11 | [DevOps / CI-CD](11-devops.md) | DevOps |
| 12 | [Segurança (threat model, OWASP, STRIDE…)](12-security.md) | Security |
| 13 | [IA aplicada (ai-svc)](13-ai.md) | ML, Security |
| 14 | [Observabilidade](14-observability.md) | SRE |
| 15 | [Testes](15-testing.md) | QA, Eng |
| 16 | [Roadmap](16-roadmap.md) | Product |
| 17 | [Backlog (epics→subtasks)](17-backlog.md) | Product, Eng |
| 18 | [Riscos](18-risks.md) | Liderança |
| 19 | [Custos](19-costs.md) | Finance, Liderança |
| 20 | [Melhorias](20-improvements.md) | Arquitetura |
| 99 | [Checklists (dev/QA/sec/deploy/prod)](99-checklists.md) | Todos |

## Convenções

- **IDs:** requisitos funcionais `RF-xxx`, não funcionais `RNF-xxx`, riscos `RISK-xxx`, ADRs `ADR-xxxx`.
- **Prioridade:** MoSCoW (Must/Should/Could/Won't) + faixa de roadmap (MVP/V1/V2/V3).
- **Estimativa:** *story points* (Fibonacci) + T-shirt (S/M/L/XL).
- **Diagramas:** Mermaid (renderizável no GitHub).
- **Premissas** marcadas com `> **Premissa:**`. **Perguntas abertas** consolidadas no doc 00.

## Estado de implementação (v0.1.0)

Implementado ≈ 5% da plataforma: núcleo do `obfuscator-svc` (engine de ofuscação smali, CLI). Todo o restante desta documentação é **greenfield a construir**. Ver a matriz de gap em [00-assumptions.md](00-assumptions.md#gap-implementado-vs-documentado).
