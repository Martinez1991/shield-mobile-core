# 12 — Segurança

Dupla natureza: **(A) segurança da plataforma** e **(B) eficácia das proteções entregues**. A plataforma manipula binários de terceiros (potencialmente maliciosos) e detém chaves de assinatura → o **worker é tratado como zona hostil** (P5).

## Modelo de ameaça — STRIDE (plataforma)

| Ameaça | Vetor | Mitigação |
|--------|-------|-----------|
| **S**poofing | Falsa identidade de usuário/serviço/runner | OIDC+MFA; mTLS interno; enrollment de runner por token+mTLS |
| **T**ampering | Alterar binário/policy/artefato | Policy assinada; audit hash-chained; imagens cosign; SBOM/SLSA |
| **R**epudiation | Negar ação | Audit log imutável append-only |
| **I**nfo disclosure | Vazar binário/segredo/chave | Cifra em repouso (SSE-KMS) + por tenant; Vault; chave só no HSM/broker; binários TTL+purge |
| **D**oS | Exaustão via builds/uploads | Rate-limit por tenant; quotas; fila com backpressure; WAF/Shield |
| **E**levation | Escapar do worker / cross-tenant | gVisor sem egress, seccomp, non-root; RLS + per-tenant keys; least-privilege IAM |

## DREAD (priorização — 1..10)

| Risco | D | R | E | A | D | Score | Sev |
|-------|---|---|---|---|---|-------|-----|
| Vazamento de chave de assinatura | 10 | 3 | 4 | 8 | 3 | **28** | Crítico |
| Cross-tenant leak | 9 | 4 | 4 | 8 | 4 | **29** | Crítico |
| Parser de binário hostil (RCE no worker) | 9 | 6 | 5 | 7 | 6 | **33** | Crítico |
| Bypass da proteção (output) | 6 | 8 | 6 | 9 | 7 | **36** | Alto |
| Proteção quebra o app | 7 | 8 | 8 | 9 | 8 | **40** | Alto |

## Privacidade — LINDDUN
- **Linkability/Identifiability:** telemetria de campo pseudonimizada no ingest; sem IP/IMEI/device-id em claro.
- **Non-repudiation/Detectability:** audit necessário para segurança, mas escopado por tenant.
- **Disclosure:** binários e mapping cifrados; acesso a mapping restrito (`build:mapping`).
- **Unawareness/Non-compliance:** DPA por tenant, consentimento opt-in para telemetria, DPO (LGPD).

## OWASP Top 10 (2021) — plataforma

| Item | Status/Controle |
|------|-----------------|
| A01 Broken Access Control | RBAC/ABAC deny-by-default, RLS por tenant, testes de tenancy |
| A02 Cryptographic Failures | TLS 1.3, SSE-KMS, Vault; **corrigir** framing de "string encryption" como reversível (RC4) |
| A03 Injection | Go/Rust + queries parametrizadas; validação de entrada; sem shell interpolado |
| A04 Insecure Design | Threat modeling por feature; **RASP não pode ser theater** (RC3) |
| A05 Security Misconfig | PSS restricted, distroless, IaC scan, benchmarks CIS |
| A06 Vulnerable Components | SCA (govulncheck/Trivy/OSV), *pinned deps*, SBOM |
| A07 Auth Failures | MFA, senhas fora (só OIDC), lockout, rotação de token |
| A08 Integrity Failures | cosign, SLSA provenance, policy assinada |
| A09 Logging Failures | logs estruturados, alertas; **implementar** (hoje ausente) |
| A10 SSRF | Sem egress no worker; allowlist de saída no control plane; validação de URLs de webhook |

## OWASP API Top 10 (2023)
BOLA/BFLA → authz por objeto + testes; excessive data exposure → DTOs/GraphQL scoping; unrestricted resource consumption → rate-limit/quotas; SSRF em webhooks → allowlist; inventory → catálogo OpenAPI versionado.

## OWASP MASVS/MASTG (eficácia do output)
Benchmark do artefato protegido: **MASVS-RESILIENCE** (R) — anti-tamper, anti-debug, anti-hooking, obfuscation, device binding. **KPI:** esforço/tempo de reversão medido por *red-team* (jadx/Ghidra/Frida) — hoje **não medido** (issue #9).

## AppSec — achados atuais (do código v0.1.0)

| ID | Achado | CWE | Correção (issue) |
|----|--------|-----|------------------|
| SEC-1 | Senha de keystore em argv (`--ks-pass pass:`) | CWE-214 | file:/env: (#6) |
| SEC-2 | Chave AES derivada de material embarcado | CWE-321 | material fragmentado + device entropy (#5) |
| SEC-3 | Nonce GCM determinístico, chave única | CWE-323 | revisar esquema (#5) |
| SEC-4 | Parsers smali sem fuzzing | — | fuzzing (#10) |
| SEC-5 | Sem sandbox no CLI (doc prevê gVisor) | — | worker isolado (#18) |

## Frameworks de referência
- **STRIDE/DREAD/LINDDUN** (acima), **MITRE ATT&CK for Mobile** (mapear técnicas do adversário do app), **MITRE ATLAS** (se IA exposta — ver §13), **NIST CSF/800-53** (governança), **CIS Controls**, **ISO 27001 Anexo A**, **PCI SAQ-A**.

## DevSecOps — ferramentas sugeridas
SAST: CodeQL/Semgrep · DAST: OWASP ZAP · IAST: opcional · SCA: govulncheck/Trivy/OSV · Secret scan: gitleaks · IaC scan: Checkov/tfsec · Container scan: Trivy/Grype · SBOM: syft · Assinatura: cosign · Fuzzing: go-fuzz/AFL++/libFuzzer (parsers).
