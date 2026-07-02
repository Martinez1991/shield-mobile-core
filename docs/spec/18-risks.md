# 18 — Matriz de Riscos

Escala: Impacto (1–5) × Probabilidade (1–5) → Severidade. Categorias: Técnico, Negócio, Segurança, Operação, Infra, Financeiro, Legal.

| ID | Categoria | Risco | Imp | Prob | Sev | Mitigação | Dono |
|----|-----------|-------|:---:|:----:|:---:|-----------|------|
| RISK-01 | Técnico | Proteção quebra o app (corretude) | 5 | 4 | **Crítico** | Golden apps + differential testing (gate); keep-rules; reachability-aware | Eng Engine |
| RISK-02 | Segurança | Parser de binário hostil → RCE no worker | 5 | 3 | **Crítico** | Fuzzing contínuo; gVisor sem egress; memory-safe (Rust) | Security |
| RISK-03 | Segurança | Vazamento de chave de assinatura | 5 | 2 | **Alto** | Chave só no HSM/KMS; broker; dual control; rotação | Security |
| RISK-04 | Segurança | Cross-tenant leak | 5 | 2 | **Alto** | Isolamento estrito; per-tenant encryption; RLS; testes de tenancy | Platform |
| RISK-05 | Segurança | Bypass da proteção por atacante | 4 | 5 | **Alto** | Defense-in-depth; reação distribuída; VM polimórfica; red-team permanente; feeds | Security |
| RISK-06 | Técnico | IR smali-texto limita flattening/VM | 4 | 5 | **Alto** | Migrar p/ IR tipada (ADR-0005, #20) | Arquitetura |
| RISK-07 | Operação | Falso-positivo de RASP bloqueia usuário legítimo | 4 | 3 | **Médio** | report-only → ramp-up; allowlist; telemetria antes de reação dura | Eng RASP |
| RISK-08 | Técnico | Update de OS/toolchain quebra pipeline | 3 | 4 | **Médio** | Compat matrix (device farm); engine feeds desacoplados; monitorar betas | Eng |
| RISK-09 | Negócio | Escopo de plataforma (145–210 PM) vs recursos | 5 | 4 | **Crítico** | Faseamento rígido (MVP Android); focar diferencial; não construir tudo | Product |
| RISK-10 | Financeiro | Custo de build (CPU-intensivo) corrói margem | 4 | 3 | **Médio** | Spot; cache content-addressed; budget de overhead; pricing usage-based | Finance |
| RISK-11 | Legal/LGPD | Telemetria com PII sem base legal/DPIA | 4 | 3 | **Médio** | Pseudonimização no ingest; opt-in; DPIA; DPO | Compliance |
| RISK-12 | Operação | Dependência de apktool/toolchains externas | 3 | 3 | **Médio** | Vendoring; versões fixas; fallback binary rewriting | Eng |
| RISK-13 | Negócio | Concorrentes maduros (Guardsquare/Arxan) | 4 | 4 | **Alto** | Diferenciais (IA risk-driven, DevSecOps, feedback loop) | Product |
| RISK-14 | Segurança | RASP toy dá falsa sensação de segurança | 4 | 4 | **Alto** | Hardening RASP; KPI de eficácia; não vender além do entregue | Security |

## Mapa de calor (Prob × Impacto)

```
Impacto
  5 |            RISK-01,09        RISK-02,03,04
  4 |         RISK-10,11    RISK-13,14   RISK-05,06
  3 |             RISK-12   RISK-07,08
  2 |
  1 |
     +---------------------------------------------
        1        2        3        4        5   Probabilidade
```

## Top-5 a mitigar já
RISK-01 (corretude), RISK-02 (parser/RCE), RISK-09 (escopo), RISK-05/14 (eficácia/RASP), RISK-06 (IR). Todos com issue ou ADR associados.
