# 17 — Backlog

Hierarquia: **Epic → Feature → User Story → Task/Subtask**. Estimativa em *story points* (SP, Fibonacci) e T-shirt. Prioridade MoSCoW. Critérios de aceite testáveis (Gherkin resumido).

## Épicos

| Epic | RF | Fase | Prioridade |
|------|----|----- |-----------|
| E1 — Corretude e confiança do engine | RF-008 | MVP | Must |
| E2 — Control plane & IAM | RF-001..003 | MVP | Must |
| E3 — Build orchestration | RF-005..007,011 | MVP | Must |
| E4 — Policy-as-code | RF-004 | V1 | Must |
| E5 — RASP | RF-010 | V1 | Must |
| E6 — Dashboard & UX | RF-020 | V1 | Should |
| E7 — CI/CD & DevSecOps | RF-017 | V1 | Should |
| E8 — IA risk map | RF-012 | V2 | Should |
| E9 — iOS pipeline | — | V2 | Should |
| E10 — VM & flattening | RF-009 | V3 | Could |
| E11 — On-prem/híbrido/compliance | RF-021,022 | Ent | Could |

## Detalhamento (amostra — sprints iniciais)

### E1 — Corretude e confiança do engine
- **Feature E1-F1: Gate de corretude (differential testing)**
  - **US:** Como AppSec, quero que o build falhe se o app protegido divergir do original, para não quebrar produção.
  - **Tasks:** T1 harness emulador/ART; T2 suíte golden apps; T3 differential runner; T4 gate no CI.
  - **Aceite:** *Dado* um golden app, *quando* protegido, *então* 0 divergências funcionais e falha bloqueia release.
  - **SP:** 13 (L). **Dep.:** — . **Issue:** #3.
- **Feature E1-F2: Keep-rules automáticas de manifesto**
  - **US:** Como Dev, quero que componentes do AndroidManifest nunca sejam renomeados.
  - **Aceite:** Activity declarada no manifesto sob include prefix permanece com nome original; teste cobre.
  - **SP:** 5 (M). **Issue:** #4.
- **Feature E1-F3: Reforço do esquema de strings** — SP 5. **Issue:** #5.
- **Feature E1-F4: Fuzzing de parsers** — SP 8. **Issue:** #10.

### E2 — Control plane & IAM
- **E2-F1 Auth OIDC+MFA** (SP 13) · **E2-F2 Tenant/RBAC+RLS** (SP 13) · **E2-F3 API keys** (SP 5).
  - **Aceite (RBAC):** *Dado* usuário sem permissão, *quando* acessa recurso de outro tenant, *então* 403 e evento de auditoria.

### E3 — Build orchestration
- **E3-F1 Upload resumable+pre-scan** (SP 8) · **E3-F2 job-svc + máquina de estados (event-sourcing)** (SP 13) · **E3-F3 Signer Broker (KMS/HSM)** (SP 13) · **E3-F4 Corrigir senha em argv** (SP 2, #6).

### E7 — CI/CD & DevSecOps
- **E7-F1 Pipeline CI** (test/vet/gofmt+SAST+SCA+secret+IaC) SP 8 (#7) · **E7-F2 SBOM+cosign+SLSA** SP 8.

> Backlog completo (todas as US/Tasks) é mantido no **GitHub Projects** ligado às [issues](https://github.com/Martinez1991/shield-platform/issues) e milestones (Curto/Médio/Longo).

## Rastreabilidade (RF → Epic → Issue)

| RF | Epic | Issue(s) |
|----|------|----------|
| RF-008 corretude | E1 | #3, #4, #5, #8, #10 |
| RF-011 assinatura | E3 | #6 |
| RF-010 RASP | E5 | #11; **#54 ingest de campo ✅** |
| RF-009 VM | E10 | **#14 ✅, #20 ✅** (IR tipada + flattening + invoke); follow-ups #48–#50 ✅ |
| RF-016 AAB | — | **#16 ✅, #51 ✅** (keep-rules protobuf) |
| RF-018 worker/fila | — | **#18 ✅** (NATS #52 ✅) |
| RF-017 CI/CD | E7 | #7 |
| RF-014 observabilidade | — | **#21 ✅** (OTLP #53 ✅) |
| RF-020 dashboard | E6 | (a criar) |
| RF-012 IA | E8 | (a criar) |

> **✅ Entregue em v0.2.0.** O épico **E10 (VM & flattening)**, planejado para V3, foi antecipado: IR tipada Go-native, flattening com dispatcher central e invoke data-driven, todos verificados em ART real. Ver [§16 Roadmap → Estado atual](16-roadmap.md#estado-atual--v020-entregue).
