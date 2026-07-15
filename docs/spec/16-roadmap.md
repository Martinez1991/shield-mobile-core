# 16 — Roadmap

> Alinhado ao §17 do `shield-platform.md` e às issues do repositório. **Regra:** fechar riscos críticos (RC1/RC2/RC4) antes de novas features.

| Fase | Prazo | Escopo | Entregas-chave |
|------|-------|--------|----------------|
| **MVP** | 3–4 meses | Provar valor em Android + não quebrar | Engine (rename+strings+control-flow), **gate de corretude (golden apps)**, keep-rules de manifesto, RASP básico (root/debug/frida), re-sign v2/v3, CLI, dashboard mínimo, SaaS single-region, CI/SBOM |
| **V1** | 6–8 meses | Produção Android + fundações | AAB, native protection (LLVM), RASP completo, policy-as-code, GitHub/GitLab actions, RBAC/MFA/SSO, billing, relatórios, autoscaling, observabilidade |
| **V2** | 9–12 meses | iOS + IA | Pipeline iOS (Mach-O, Swift/ObjC), `ai-svc` (risk map), integrações CI/CD completas, telemetria de campo, multi-region |
| **V3** | 12–18 meses | Diferenciação premium | VM polimórfica ampliada (fluxo de controle), proteção adaptativa por IA em produção, self-modifying/runtime codegen, ARM64/RISC-V, IR tipada (dexlib2/LLVM) |
| **Enterprise** | 18–24 meses | On-prem/híbrido + compliance | Air-gapped, HSM PKCS#11, hybrid runners, SLSA L3, SOC2/ISO 27001, SLAs, feeds contínuos |

## Estado atual — v0.2.0 (entregue)

Além do MVP (v0.1.0: engine rename/strings/control-flow, gate de corretude golden/ART, keep-rules de manifesto, RASP básico, CLI), a **v0.2.0** adiantou capacidades de V1 e V3:

- **IR tipada Go-native** (`internal/ir`, [ADR 0001](../adr/0001-typed-ir.md)) — parser estruturado + inferência de tipos + liveness. Não dexlib2; engine segue puro-Go/zero-deps.
- **VM ampliada** (fecha [#14](https://github.com/Martinez1991/shield-platform/issues/14)/[#20](https://github.com/Martinez1991/shield-platform/issues/20)) — branches, ALU int/long, narrowing, objetos, const-string virtualizado, **control-flow flattening** (dispatcher central), e **invoke data-driven** (static/virtual/interface; args e retornos int/long/objeto) — tudo verificado byte-a-byte em ART real.
- **AAB / split** ([#16](https://github.com/Martinez1991/shield-platform/issues/16)) — round-trip de bundle + keep-rules do manifesto protobuf ([#51](https://github.com/Martinez1991/shield-platform/issues/51), parser hand-rolled).
- **Worker sandboxed + fila + autoscaling** ([#18](https://github.com/Martinez1991/shield-platform/issues/18), [ADR 0002](../adr/0002-nats-queue.md)) — `queue.Queue` (Mem/Dir/NATS JetStream), gVisor + no-egress + KEDA.
- **Observabilidade** ([#21](https://github.com/Martinez1991/shield-platform/issues/21), [ADR 0003](../adr/0003-otlp-tracing.md)) — métricas Prometheus por estágio, spans OTel export via OTLP, dashboards/alertas Grafana.
- **Ingest de callbacks RASP em campo** ([#54](https://github.com/Martinez1991/shield-platform/issues/54)) — `cmd/rasp-ingest`, HMAC por-tenant + anti-replay.

> Duas dependências externas foram introduzidas de forma disciplinada (NATS, OTel), confinadas a um pacote; o **núcleo do engine permanece stdlib-only e determinístico**.

### v0.3.0 (entregue) — proteção risk-driven + fundações iOS/nativo

- **Planner risk-driven** (AI risk-map v0, épico [#65](https://github.com/Martinez1991/shield-platform/issues/65)) — features estáticas por método sobre a IR tipada + score heurístico explicável; VM/flattening só nos hot spots acima do threshold; `Result.RiskMap` auditável. Zero-dep, sem ML.
- **Fundação iOS** ([#63](https://github.com/Martinez1991/shield-platform/issues/63)) — detecção/round-trip de IPA (#74) + inspeção Mach-O via `debug/macho` (#75).
- **Fundação nativa** ([#64](https://github.com/Martinez1991/shield-platform/issues/64)) — inspeção de `.so` ELF via `debug/elf` (#81).

> A inspeção binária dos três formatos (Dalvik/IR, Mach-O, ELF) é **stdlib-only**. Os transforms invasivos (LLVM, injeção nativa, re-assinatura) e seus gates de execução ficam adiados (dep. de toolchain/infra), decompostos como sub-issues.

### v0.4.0 (entregue) — proteção de código nativo (LLVM) + análise de binários

- **`native-svc`** (épico [#64](https://github.com/Martinez1991/shield-platform/issues/64), [ADR 0004](../adr/0004-llvm-native-svc.md)) — serviço LLVM **out-of-tree**, invocado como subprocess (nunca ligado ao build Go; engine segue stdlib-only/CGO-free). Quatro passes componíveis sobre bitcode: **flattening** de fluxo, **MBA**, **opaque predicates** e **cifra de strings** (#82, #83).
- **Gates de execução nativos** — cada pass é provado funcionalmente idêntico (host via dlopen), a pipeline compile→transform→link produz um **`.so` arm64 Android** real, e o binário arm64 protegido **executa idêntico sob qemu-user** (contrapartida ISA do gate golden/ART).
- **`shield analyze <ipa|apk|aab>`** ([#87](https://github.com/Martinez1991/shield-platform/issues/87)) — inspeção de binários Mach-O/ELF (arquitetura, seções, símbolos, densidade de segredos) no CLI, reaproveitando as fundações stdlib-only.

> Uma terceira dependência foi introduzida de forma disciplinada (toolchain LLVM), **confinada a um executável separado** — nem `go.mod` nem o engine mudam.

### v0.5.0 (entregue) — RASP nativo + worker de APK (loop nativo fechado)

- **RASP nativo** ([#84](https://github.com/Martinez1991/shield-platform/issues/84)) — `rasp` (anti-debug via `TracerPid`) e `tamper` (self-checksum de seção + patch pós-link `tamper-patch.py`), cada um com gate de execução (silencioso normal / dispara sob tracer / detecta adulteração).
- **Worker Go de APK** ([#82](https://github.com/Martinez1991/shield-platform/issues/82)/[#64](https://github.com/Martinez1991/shield-platform/issues/64)) — `internal/nativesvc.ProtectArchive` + `cmd/shield-nativeapk`: módulos nativos recompiláveis (sidecar de bitcode) são transformados → linkados → tamper-patchados → reempacotados byte-a-byte; `apk-flow-gate.sh` prova o round-trip (roda idêntico + detecta tamper).

> Loop nativo completo: análise → 6 passes (flatten/MBA/opaque/strings/rasp/tamper) → gates de execução (host + arm64 qemu) → round-trip de APK dirigido pelo worker. Falta só um gate on-device/emulador Android arm64 (infra).

### v0.6.0 (entregue) — fundação iOS (strip + Simulador) + gate on-device Android

- **iOS Mach-O strip** ([#76](https://github.com/Martinez1991/shield-platform/issues/76)) — `internal/ios.StripIPA` + `cmd/shield-iosstrip`: strip de símbolos/`__LINKEDIT` do app + frameworks com re-assinatura ad-hoc, round-trip de IPA; gate no runner **macos-14** grátis.
- **Differential no Simulador iOS** ([#78](https://github.com/Martinez1991/shield-platform/issues/78)) — `scripts/ios-simulator-gate.sh`: roda o binário protegido no runtime iOS real via `simctl spawn` e exige saída idêntica.
- **Gate on-device Android** ([#64](https://github.com/Martinez1991/shield-platform/issues/64)) — `ondevice-gate.sh`: binário arm64 protegido roda idêntico + anti-tamper detectado num **Galaxy S24 real**.

> iOS gratuito esgotado sem infra paga: falta só a **re-assinatura de distribuição** ([#77](https://github.com/Martinez1991/shield-platform/issues/77), certificado Apple pago).

## Marcos de qualidade
- **M1 (fim MVP):** UX ≥ 7/10; corretude 100% golden apps; nota geral do committee ≥ 6.
- **M2 (fim V1):** SLA 99.9% control plane; SCA/SAST sem high; SBOM por release.
- **M3 (fim V2):** eficácia de reversão medida (KPI red-team) com meta ≥ baseline de mercado.

## Objetivos por fase (OKR resumido)
- **MVP:** "Proteger sem quebrar" — 0 divergências em golden apps; 3 clientes piloto.
- **V1:** "DevSecOps nativo" — gate no CI de 5 pipelines; NPS piloto ≥ 40.
- **V2:** "Risk-driven + iOS" — overhead −30% vs ligar-tudo; iOS GA.
