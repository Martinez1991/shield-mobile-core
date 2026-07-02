# 15 — Plano de Testes

## Pirâmide e tipos

| Tipo | Escopo | Ferramentas | Meta |
|------|--------|-------------|------|
| **Unitário** | Cada pass, parsers, serviços | `go test`, `cargo test`, *property-based* (proptest/quickcheck) | Cobertura ≥ 80% núcleo |
| **Correção semântica** ⭐ | App protegido == original | Golden apps + Espresso/XCTest + differential testing | **100% (gate)** |
| **Integração** | Pipeline upload→download | kind/docker-compose, Pact (contratos) | Fluxos críticos |
| **E2E** | Jornadas de usuário | Playwright (dashboard), CLI e2e | Top jornadas |
| **Compatibilidade** | SDK levels, ABIs, devices | Firebase Test Lab/BrowserStack (Android 8–15, iOS 15–18) | Matriz |
| **Resistência (adversarial)** ⭐ | Proteção resiste? | jadx/Ghidra/Frida/objection; regressão de bypass | KPI esforço-de-reversão |
| **Carga/Stress** | Builds concorrentes, API | k6/Gatling | 1k builds simultâneos (V2) |
| **Chaos** | Falhas | Chaos Mesh (matar worker mid-build) | retry idempotente |
| **Segurança** | Plataforma | SAST/DAST/SCA/secret/IaC/container scan; **fuzzing** de parsers (AFL++/libFuzzer) | 0 high/critical |
| **Performance** | Overhead runtime + build | Bench de startup/frame/tamanho | Dentro do budget |

⭐ = gate de release.

## Gate de release (obrigatório)
`corretude 100% nos golden apps` **E** `overhead dentro do budget` **E** `fuzzing dos parsers sem crash` **E** `red-team regression sem regressões` **E** `SAST/SCA sem high/critical`.

## Testes por escala (capacidade)

| Usuários/builds | Expectativa | Limitação / ação |
|-----------------|-------------|------------------|
| 100 | OK trivial | — |
| 1.000 | OK | HPA + Redis; monitorar fila |
| 10.000 | Requer fila + KEDA | Particionar Kafka por tenant; spot workers |
| 100.000 | Multi-nodegroup, cache content-addressed | *priority queues* por plano |
| 1.000.000 | Multi-region, sharding, backpressure | Revisar RDS (sharding/Citus), ClickHouse cluster |

## Cobertura e qualidade
- Cobertura reportada no CI (codecov); *mutation testing* (go-mutesting) em módulos críticos do engine.
- Testes de contrato entre serviços (Pact broker); testes de tenancy (garantia anti cross-tenant).

## Estado atual (v0.1.0)
- 31 testes Go; validação de *assembly* (smali/baksmali) e algoritmo na JVM (AES/VM). **Faltam:** golden apps + differential testing (issue #3), red-team (issue #9), fuzzing (issue #10), carga/chaos (plataforma inexistente).
