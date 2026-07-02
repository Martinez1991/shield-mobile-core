# Relatório do Comitê de Revisão Técnica — SHIELD Platform

**Versão avaliada:** v0.1.0 · **Data:** 2026-07-02 · **Escopo:** repositório completo, código-fonte, testes, scripts de validação e `shield-platform.md`.

> **Nota metodológica.** Existe uma **divergência estrutural entre o artefato entregue e a solução documentada** que domina a avaliação:
>
> - **`shield-platform.md`** descreve uma **plataforma enterprise SaaS/on-prem** (17 microsserviços, K8s, Kafka, RASP nativo, VM, IA, dashboard, API REST/GraphQL, billing, multi-tenant, compliance) — estimada **no próprio documento em 145–210 pessoas-mês**.
> - **O que foi construído** é o **núcleo de um único componente** (`obfuscator-svc`, §3/§6/§8): uma **CLI Go de ~27 arquivos** que opera sobre smali. **~5% da plataforma descrita.**
>
> Este relatório **pontua o artefato como ele existe hoje**, tratando o `.md` como *design intent*. Adotamos duas lentes onde relevante: *(A) qualidade do MVP de engine* vs *(B) completude como plataforma*.

---

## 1. Resumo Executivo

O SHIELD entrega um **MVP de engine de ofuscação Android tecnicamente sólido e validado com rigor incomum**: cada transformação é confirmada montando em DEX real (`smali`/`baksmali`), e os componentes criptográficos (AES-GCM) e a VM têm o algoritmo cross-checado na JVM. Código limpo, `gofmt`/`vet` clean, zero dependências, determinístico (P2), honestamente escopado.

Três verdades incômodas se impõem:

1. **Não é uma plataforma — é um binário.** Sem infraestrutura, API, dashboard, autenticação, multi-tenancy, observabilidade, CI/CD, persistência ou serviço. §1, §10–§15, §21 do documento: **0% implementadas**.
2. **A corretude runtime nunca foi provada.** "Monta em DEX" ≠ "passa na verificação da ART" ≠ "o app roda igual". **Nenhum APK protegido foi executado em device nem verificado pela ART.** O gate de *golden apps + differential testing* (§20) **não existe**. Maior risco de confiança do projeto.
3. **Como produto de segurança, os módulos são imaturos.** A "criptografia" de strings é reversível por design (chave no artefato), o RASP é *toy* e trivialmente contornável, e a VM cobre uma fração desprezível de código real. A **eficácia de proteção** nunca foi medida contra um adversário (jadx/Frida/Ghidra).

**Veredito:** excelente prova de conceito de engine com disciplina de validação exemplar; **imaturo como produto de segurança** e **inexistente como plataforma**.

---

## 2. Pontos Fortes

| # | Força | Evidência |
|---|-------|-----------|
| F1 | Disciplina de validação acima da média | Round-trip DEX + cross-check JVM para AES e VM; 31 testes; scripts reproduzíveis. |
| F2 | Determinismo real (P2) | Seed → saída idêntica; testado. Builds auditáveis. |
| F3 | Qualidade de código Go | Modular, memory-safe, zero-dep, comentado, `vet`/`gofmt` limpos. |
| F4 | Honestidade de engenharia | Marca explicitamente o que não é verificável (flattening/ART). |
| F5 | VM polimórfica por build | Permutação de opcodes por seed, validada como bijeção. |
| F6 | Privacy touch no `analyze` | Segredos mascarados, nunca persistidos em claro. |

---

## 3. Pontos Fracos e Riscos Críticos

### Críticos (bloqueiam produção)

- **RC1 — Corretude não verificada em runtime.** Sem execução em ART/device e sem *differential testing*, não há evidência de que o app protegido se comporte como o original. Risco totalmente não mitigado.
- **RC2 — Rename sem keep-rules de manifesto.** O `AndroidManifest.xml` não é lido. Renomear classes sob um `includePrefix` com Activities/Services/Providers/Receivers **quebra o app**. Só há keep manual.
- **RC3 — RASP é security theater.** Poucos sinais; reação não acionada; injetado **após** o rename → fica em smali de texto plano, não ofuscado → neutralizável em segundos. "Reação distribuída" (§6.1) inexistente.
- **RC4 — "Criptografia" de strings é reversível por design.** Chave = `SHA-256(material embarcado)`; material no APK. Qualquer atacante decifra 100% das strings. Não pode ser rotulado como *encryption/confidentiality*.

### Altos

- **RA1 — Nonce AES-GCM determinístico** (`nonce = SHA256(seed‖plaintext)[:12]`, chave única). Plaintexts iguais → ciphertext idêntico; seed reutilizado → mesma chave.
- **RA2 — Senha de keystore em `argv`** (`--ks-pass pass:<senha>`, CWE-214). Usar `file:`/`env:`.
- **RA3 — Cobertura da VM ≈ nula** em apps reais (só `static` int *straight-line*).
- **RA4 — Limite de 64K métodos/multidex não gerenciado.**
- **RA5 — Semântica de `String ==`/switch/intern** com string decifrada (`new String`).

### Médios

- **RM1** — Rename por token `L…;` pode corromper string literals com esse padrão. **RM2** — Parsers smali via regex, sem fuzzing (§23 lista como crítico). **RM3** — `apktool`/`apksigner` do `PATH` sem pinning. **RM4** — Sem AAB/split APKs reais. **RM5** — DevSecOps documentado (SLSA L3, SBOM, cosign) não implementado; sem CI.

---

## 4. Avaliação por Critério (resumo)

- **Arquitetura — Regular/Bom (engine).** Boa coesão; dívida estrutural: "SHIELD-IR" é smali-texto manipulado por regex, não IR tipada → teto para flattening/virtualização de fluxo.
- **Infraestrutura — Crítico.** Inexistente (binário local).
- **Segurança — Regular/Fraco (para produto de segurança).** Memory-safe, mas A02/A04/A08/A09 OWASP; CWE-321/CWE-214; sem sandbox/DevSecOps; eficácia não medida.
- **Funcionalidades — Regular.** Amplo e raso; sem gate de corretude nem keep-rules de manifesto.
- **UX/UI — 2.5/10.** Sem GUI; só CLI com footguns.
- **Performance — Regular (não medida).** Regex-heavy; só fixtures mínimas.
- **Escalabilidade — Fraco.** Single-shot; sem fila/worker/autoscaling.
- **Observabilidade — Crítico.** Print de sumário apenas.
- **Custos — Bom por omissão (baixo sinal).** CLI barato; sem FinOps.
- **Governança — Fraco.** Privacy/determinismo ok; sem audit/SBOM/compliance/licenciamento.

---

## 5. Benchmark de Mercado

| Dimensão | SHIELD (implementado) | Guardsquare / Arxan / AppDome |
|----------|-----------------------|-------------------------------|
| Ofuscação Dalvik básica | ✅ funcional | ✅ maduro |
| Code virtualization | 🟡 demo (int static) | ✅ produção |
| RASP | 🔴 toy, bypassável | ✅ tripwires nativos + reação |
| iOS / nativo (LLVM) | ❌ | ✅ |
| Eficácia vs RE comprovada | ❌ não medida | ✅ red-team contínuo |
| DevSecOps / integração | 🔴 só CLI | ✅ plugins/actions/orbs |

- **Acima da média:** disciplina de validação de assembly/algoritmo e honestidade de escopo.
- **Abaixo da média:** eficácia, RASP, iOS/nativo, cobertura de VM, corretude runtime, plataforma.

---

## 6. Score Geral

> Lente: solução como entregue. Entre parênteses, leitura "MVP de engine isolado".

| Área | Nota |
|------|:----:|
| Arquitetura | 6.0 |
| Infraestrutura | 2.0 |
| Segurança | 4.0 |
| Funcionalidades | 5.5 |
| UX/UI | 2.5 |
| Escalabilidade | 3.0 |
| Performance | 4.0 |
| Observabilidade | 2.0 |
| Custos | 5.5 |
| Governança | 3.0 |

**Nota Geral: 3.8 / 10** (como plataforma/solução) · **~7.0 / 10** apenas como MVP de engine com validação de assembly.

A distância entre 3.8 e 7.0 é a mensagem central: ~5% de uma plataforma foi construída, e mesmo esse núcleo carece de prova de corretude runtime e de eficácia de segurança.

---

## 7. Plano de Melhorias

| Prioridade | Item | Impacto | Esforço | ROI |
|-----------|------|:-------:|:-------:|:---:|
| Crítica | Gate de corretude runtime (emulador/ART + differential testing de golden apps) | Alto | Médio | Alto |
| Crítica | Keep-rules automáticas do `AndroidManifest.xml` | Alto | Baixo | Muito Alto |
| Crítica | Reetiquetar strings como ofuscação reversível; material de chave fragmentado + device entropy | Alto | Baixo | Muito Alto |
| Alta | Corrigir `--ks-pass pass:` → `file:`/`env:` (CWE-214) | Médio | Baixo | Muito Alto |
| Alta | Red-team automatizado (jadx/Ghidra/Frida) como KPI de reversão | Alto | Médio | Alto |
| Alta | CI (GitHub Actions): test/vet/gofmt + govulncheck + SBOM + cosign | Médio | Baixo | Alto |
| Alta | Fuzzing dos parsers smali (go-fuzz) | Médio | Médio | Alto |
| Média | RASP hardening (auto-ofuscar, tripwires Frida/Xposed/TracerPid, reação acoplada) | Médio | Médio | Médio |
| Média | Guarda de multidex (contagem de métodos pós-injeção) | Médio | Baixo | Alto |
| Média | Benchmarks de performance + cache content-addressed | Médio | Médio | Médio |
| Baixa | Retrace CLI, suporte AAB, observabilidade mínima (logs estruturados) | Baixo | Médio | Médio |

---

## 8. Roadmap

**Curto prazo (0–30 dias) — Provar que não quebra e não mente**
1. Gate de corretude (emulador + differential testing de golden apps).
2. Keep-rules automáticas de manifesto (RC2).
3. Reforçar/reetiquetar esquema de strings (RC4/RA1); corrigir senha em argv (RA2).
4. CI com testes + govulncheck + SBOM + guarda de multidex.

**Médio prazo (30–90 dias) — Provar que protege**
5. Red-team automatizado como KPI de eficácia.
6. Fuzzing dos parsers; endurecer RASP.
7. Ampliar cobertura da VM (com verificação ART no loop).
8. `job-svc` mínimo (API REST §11.1 + máquina de estados §2.3), testável com httptest.

**Longo prazo (90–180 dias) — Virar plataforma**
9. Worker isolado (gVisor/Kata) + fila + autoscaling; observabilidade OTel/Prometheus.
10. AuthN/Z, multi-tenant, audit hash-chained, licenciamento; metas SOC2/ISO.
11. IR tipada real (dexlib2) para destravar flattening/virtualização de fluxo.

---

## 9. Palavra Final

Do ponto de vista de **engenharia e integridade**, é um dos MVPs mais bem-validados avaliados; a recusa em declarar "pronto" o não-verificável deve ser preservada. Do ponto de vista de **produto de segurança e plataforma**, o projeto está no início: falta provar corretude runtime, provar eficácia contra adversário e construir ~95% da plataforma. **Priorizar as 3 melhorias Críticas antes de qualquer nova feature.**
