# 02 — Requisitos Funcionais

Prioridade: **MoSCoW** + faixa (MVP/V1/V2/V3). Dependências por ID.

## Índice de módulos
IAM · Tenant/RBAC · Apps · Policy · Build/Job · Engine · RASP · Assinatura · Artefatos/Retrace · Relatórios/SBOM · Telemetria · IA · CLI · CI/CD · Billing · Admin.

---

### RF-001 — Autenticação (SSO/OIDC/MFA)
- **Descrição:** Login por OIDC/OAuth2 com MFA (TOTP/WebAuthn) e SSO SAML/OIDC.
- **Fluxo:** usuário → IdP → callback → sessão (JWT curto + refresh rotativo).
- **Entradas:** credenciais/assertion SAML. **Saídas:** access token, refresh token.
- **Regras:** deny-by-default; MFA obrigatório para papéis com acesso a chaves; sessão expira (RNF-SEC).
- **Prioridade:** Must / MVP. **Dependências:** —.

### RF-002 — Gestão de Tenants e RBAC
- **Descrição:** CRUD de organizações, usuários, papéis e permissões finas (RBAC + ABAC por tenant).
- **Entradas:** dados do tenant/usuário/papel. **Saídas:** entidades persistidas + eventos.
- **Regras:** isolamento estrito entre tenants; papéis customizados; matriz papel×permissão.
- **Prioridade:** Must / MVP. **Dep.:** RF-001.

### RF-003 — Cadastro de Aplicativos
- **Descrição:** Registrar apps (bundleId, plataforma, chaves de assinatura vinculadas via ref).
- **Regras:** chave de assinatura nunca em claro (ref para HSM/KMS); um app pertence a um tenant.
- **Prioridade:** Must / MVP. **Dep.:** RF-002.

### RF-004 — Policy-as-Code
- **Descrição:** Criar/validar/versionar políticas de proteção (YAML/JSON), assinadas.
- **Fluxo:** editar → validar (policy-svc) → versionar → assinar → disponível para build.
- **Entradas:** spec da policy. **Saídas:** policy versionada + assinatura.
- **Regras:** toda policy é imutável por versão; assinatura obrigatória; validação de esquema/compatibilidade.
- **Prioridade:** Must / V1 (MVP: presets fixos). **Dep.:** RF-002.

### RF-005 — Upload de binário (resumable)
- **Descrição:** Upload multipart resumable (tus) de APK/AAB/IPA para object store cifrado.
- **Regras:** validação de formato/tamanho; *malware pre-scan* (ClamAV/YARA); verificar entitlement; gera `build_id`.
- **Entradas:** binário. **Saídas:** `artifactUploadId`.
- **Prioridade:** Must / MVP. **Dep.:** RF-003.

### RF-006 — Criar e orquestrar Build
- **Descrição:** Iniciar build (`policyId` + `artifactUploadId`), acompanhar máquina de estados (§2.3).
- **Fluxo:** `QUEUED→ANALYZING→PLANNING→PROTECTING→REPACKAGING→SIGNING→READY|FAILED`.
- **Regras:** idempotência por `Idempotency-Key`; retry idempotente; event-sourcing.
- **Entradas:** policyId, uploadId, platform. **Saídas:** `buildId`, `statusUrl`.
- **Prioridade:** Must / MVP. **Dep.:** RF-004, RF-005.

### RF-007 — Análise estática do binário
- **Descrição:** Identificar container, manifest/Info.plist, DEX/libs, arquitetura, SDK; produzir *Binary Manifest*.
- **Saídas:** inventário estruturado + *call graph*/reachability.
- **Prioridade:** Must / MVP. **Dep.:** RF-006.

### RF-008 — Engine de Ofuscação
- **Descrição:** Aplicar passes: rename (classe/membro reachability-aware), string enc (XOR/AES-GCM), metadata removal, control-flow (opaque/reorder), constant enc, resource obfuscation, native protection.
- **Regras:** idempotente sobre a IR; emite *evidence*; keep-rules automáticas (manifesto/reflection/JNI); **gate de corretude** (differential testing).
- **Prioridade:** Must / MVP (subset). **Dep.:** RF-007.

### RF-009 — Code Virtualization (VM)
- **Descrição:** Virtualizar *hot spots* de segurança em bytecode proprietário polimórfico.
- **Regras:** aplicação cirúrgica guiada por *risk map*; overhead dentro do budget.
- **Prioridade:** Should / V3. **Dep.:** RF-008, RF-012.

### RF-010 — RASP inject
- **Descrição:** Injetar SDK RASP (root/jailbreak/frida/magisk/hook/debug/emulator/overlay/accessibility/tamper/screen/SSL-pinning), modelo detecção→flag→reação diferida.
- **Regras:** reação configurável por *risk tier* (report/degrade/fake-data/crash/wipe); *feeds* de assinatura atualizáveis.
- **Prioridade:** Must / V1 (MVP: root/debug/frida). **Dep.:** RF-008.

### RF-011 — Repackage e Assinatura
- **Descrição:** Recompilar IR→DEX/Mach-O, realinhar, reempacotar; assinar via *Signer Broker* (HSM/KMS) v2/v3/v4 (Android) ou entregar unsigned/re-sign (iOS).
- **Regras:** broker nunca expõe a chave; *dual control* opcional; compatível com Play App Signing.
- **Prioridade:** Must / MVP. **Dep.:** RF-008.

### RF-012 — IA: Risk Map
- **Descrição:** Detectar código sensível (crypto, APIs críticas, segredos, fluxo financeiro) e produzir `risk_score∈[0,1]` + recomendações por símbolo.
- **Regras:** nenhum segredo persistido em claro (hash/localização); modelos versionados/assinados.
- **Prioridade:** Should / V2. **Dep.:** RF-007.

### RF-013 — Entrega de artefato + Retrace
- **Descrição:** Download por URL pré-assinada (TTL curto); `mapping` cifrado para retrace; report + SBOM.
- **Regras:** mapping cifrado por chave do tenant; evento `artifact.ready`→webhook.
- **Prioridade:** Must / MVP. **Dep.:** RF-011.

### RF-014 — Relatórios de segurança + SBOM
- **Descrição:** Report por build (before/after, cobertura de proteção, evidências), SBOM, "protection diff" assinado.
- **Prioridade:** Should / V1. **Dep.:** RF-011.

### RF-015 — Telemetria de campo (RASP callbacks)
- **Descrição:** Ingerir callbacks RASP (tampering/rooting/hooking por app/versão/região) em ClickHouse.
- **Regras:** pseudonimização no ingest; opt-in; nenhum PII em claro.
- **Prioridade:** Should / V2. **Dep.:** RF-010.

### RF-016 — CLI
- **Descrição:** `shield login|analyze|protect|build|status|download|sign|report|policy validate` com `--json` e exit codes padronizados.
- **Prioridade:** Must / MVP. **Dep.:** RF-006.

### RF-017 — Integrações CI/CD
- **Descrição:** GitHub Action, GitLab template, Azure/Bitbucket/Jenkins/CircleCI, Terraform provider, imagens Docker; *gate* por threshold.
- **Prioridade:** Should / V1. **Dep.:** RF-016.

### RF-018 — Billing / Usage
- **Descrição:** Medição de uso (builds), faturamento (Stripe), quotas/entitlements.
- **Prioridade:** Should / V1. **Dep.:** RF-002.

### RF-019 — Auditoria
- **Descrição:** Audit log imutável (append-only, hash-chained) de quem fez o quê/quando; exportável.
- **Prioridade:** Must / V1. **Dep.:** RF-002.

### RF-020 — Painel administrativo (SPA)
- **Descrição:** Dashboard multi-tenant: usuários, apps, proteções, builds ao vivo, relatórios, licenças, integrações, observabilidade de proteção.
- **Prioridade:** Should / V1. **Dep.:** RF-006, RF-014.

### RF-021 — Self-hosted Runner (híbrido)
- **Descrição:** Runner que registra no control plane (mTLS+enrollment), executa build localmente, devolve status+hashes.
- **Prioridade:** Could / V-Enterprise. **Dep.:** RF-006.

### RF-022 — Engine feeds (defesa contínua)
- **Descrição:** Distribuir assinaturas de detecção/técnicas como artefatos versionados/assinados (`.shieldpack`), importáveis em air-gapped.
- **Prioridade:** Should / V2. **Dep.:** RF-010.

## Rastreabilidade (RF → UC → Backlog)
Ver matriz em [17-backlog.md](17-backlog.md#rastreabilidade). Cada RF mapeia para ≥1 épico e critérios de aceite testáveis.
