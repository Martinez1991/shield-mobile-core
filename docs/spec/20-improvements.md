# 20 — Melhorias

Consolidação de recomendações por área (deriva do [committee review](../committee-review-v0.1.0.md) e das issues). Prioridade ⇒ ROI.

## Arquitetura
- **IR tipada (dexlib2/LLVM)** substituindo smali-texto — destrava flattening real e VM de fluxo (ADR-0005, #20). *Débito estruturante nº1.*
- Bootstrap do control plane como **modular monolith** e extrair serviços conforme escala (evita overhead prematuro de microservices).
- **Outbox pattern** + sagas para consistência build→sign.

## Segurança
- Fechar RC1–RC4: **gate de corretude** (#3), **keep-rules de manifesto** (#4), **reforço de strings** (#5), **RASP hardening** (#11).
- **KPI de eficácia** via red-team automatizado (#9) — sem isso não há prova de valor de segurança.
- Fuzzing de parsers (#10); corrigir senha em argv (#6); DevSecOps completo no CI (#7).

## UX
- Construir o **dashboard** e o **design system** (a11y AA) — sair de 2.5 para ≥7/10.
- Remover *footguns* da CLI (flags após path; preset+rename no-op silencioso → erro/aviso claro).

## Performance
- **Cache content-addressed** de análise (#12); paralelizar passes por classe; benchmark em APK grande; *checkpoint* de IR para retry granular.

## Custos
- **Spot + bin-packing** de workers; budget de overhead no Planner (IA); tiering de logs/telemetria; right-sizing + Savings Plans.

## Escalabilidade
- Fila + KEDA + particionamento por tenant; *priority queues* por plano; multi-region só em V2 (Q5).

## Qualidade
- *Mutation testing* no engine; cobertura no CI; testes de tenancy; *property-based testing* de invariantes da IR.

## DevOps / Automação
- CI/CD completo (SAST/DAST/SCA/secret/IaC/container/SBOM/cosign); GitOps (ArgoCD); ambientes efêmeros por PR; **engine feeds** como produto (defesa contínua sem rebuild).
- Automatizar **retrace** (usar mapping) e geração de "protection diff" assinado para auditoria.

## Produto (diferenciação)
- Priorizar os 5 diferenciais (IA risk-driven, VM polimórfica, reação RASP distribuída, DevSecOps nativo, feedback loop) — é onde se ganha vs Guardsquare/Arxan/AppDome.

## Priorização (resumo)

| Prioridade | Itens | ROI |
|-----------|-------|-----|
| Crítica | #3 corretude, #4 keep-rules, #5 strings | Muito Alto |
| Alta | #6 argv, #7 CI, #9 red-team, #10 fuzz | Alto |
| Média | #11 RASP, #12 perf/cache, #13 job-svc | Médio |
| Estratégica | #20 IR tipada, dashboard/UX | Alto (longo prazo) |
