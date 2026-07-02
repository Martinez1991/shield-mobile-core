# 16 — Roadmap

> Alinhado ao §17 do `shield-platform.md` e às issues do repositório. **Regra:** fechar riscos críticos (RC1/RC2/RC4) antes de novas features.

| Fase | Prazo | Escopo | Entregas-chave |
|------|-------|--------|----------------|
| **MVP** | 3–4 meses | Provar valor em Android + não quebrar | Engine (rename+strings+control-flow), **gate de corretude (golden apps)**, keep-rules de manifesto, RASP básico (root/debug/frida), re-sign v2/v3, CLI, dashboard mínimo, SaaS single-region, CI/SBOM |
| **V1** | 6–8 meses | Produção Android + fundações | AAB, native protection (LLVM), RASP completo, policy-as-code, GitHub/GitLab actions, RBAC/MFA/SSO, billing, relatórios, autoscaling, observabilidade |
| **V2** | 9–12 meses | iOS + IA | Pipeline iOS (Mach-O, Swift/ObjC), `ai-svc` (risk map), integrações CI/CD completas, telemetria de campo, multi-region |
| **V3** | 12–18 meses | Diferenciação premium | VM polimórfica ampliada (fluxo de controle), proteção adaptativa por IA em produção, self-modifying/runtime codegen, ARM64/RISC-V, IR tipada (dexlib2/LLVM) |
| **Enterprise** | 18–24 meses | On-prem/híbrido + compliance | Air-gapped, HSM PKCS#11, hybrid runners, SLSA L3, SOC2/ISO 27001, SLAs, feeds contínuos |

## Marcos de qualidade
- **M1 (fim MVP):** UX ≥ 7/10; corretude 100% golden apps; nota geral do committee ≥ 6.
- **M2 (fim V1):** SLA 99.9% control plane; SCA/SAST sem high; SBOM por release.
- **M3 (fim V2):** eficácia de reversão medida (KPI red-team) com meta ≥ baseline de mercado.

## Objetivos por fase (OKR resumido)
- **MVP:** "Proteger sem quebrar" — 0 divergências em golden apps; 3 clientes piloto.
- **V1:** "DevSecOps nativo" — gate no CI de 5 pipelines; NPS piloto ≥ 40.
- **V2:** "Risk-driven + iOS" — overhead −30% vs ligar-tudo; iOS GA.
