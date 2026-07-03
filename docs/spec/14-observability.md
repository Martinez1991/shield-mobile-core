# 14 — Observabilidade

Três pilares + um quarto específico do domínio: **observabilidade da proteção** (P7).

> **Implementado em v0.2.0** ([#21](https://github.com/Martinez1991/shield-platform/issues/21)/[#53](https://github.com/Martinez1991/shield-platform/issues/53)/[#54](https://github.com/Martinez1991/shield-platform/issues/54)): `internal/obs` expõe métricas Prometheus por estágio do pipeline (`shield_stage_duration_seconds`, `shield_builds_total`, `shield_queue_depth`) **sem a client lib** (text-exposition hand-rolled), spans por pass exportáveis via **OTLP** ([ADR 0003](../adr/0003-otlp-tracing.md), opt-in `--otlp-endpoint`), e `cmd/rasp-ingest` recebe callbacks de campo (`shield_rasp_events_total`/`_rejected_total`). Dashboards/alertas Grafana e config Prometheus em [`deploy/observability/`](../../deploy/observability/).

## Stack
| Pilar | Ferramenta | Notas |
|-------|-----------|-------|
| Logs | **OpenTelemetry → OpenSearch/Loki** | JSON estruturado, correlacionado por `build_id`/`trace_id`; binários do cliente *scrubbed* |
| Métricas | **Prometheus + Grafana** | Latência por estágio, profundidade de fila, taxa de falha por *pass*, overhead médio, custo/build |
| Tracing | **OpenTelemetry** (OTLP) → Tempo/Jaeger | *spans* por *pass* de proteção (gateway→job→worker→sign) |
| Proteção (P7) | ClickHouse + Grafana | Mapa de tampering/rooting/hooking por app/versão/região; correlação técnica↔resistência |

## Convenções
- **Correlação:** todo request gera `X-Request-Id`; propagado como `trace_id` (W3C tracecontext) por todo o pipeline (incl. worker e Kafka via headers).
- **Log levels:** `error/warn/info/debug`; produção em `info`; sem PII/segredos/binário em log.
- **RED/USE:** métricas RED (Rate/Errors/Duration) por serviço; USE (Utilization/Saturation/Errors) por recurso.

## Métricas-chave (exemplos)
```
shield_build_duration_seconds{stage="protecting",tenant}  (histogram)
shield_build_failures_total{pass,reason}                  (counter)
shield_queue_lag{topic}                                    (gauge)
shield_overhead_ratio{app,metric="startup|size"}          (histogram)
shield_rasp_callbacks_total{signal,region,device_class}    (counter)
http_server_request_duration_seconds{route,status}         (histogram)
```

## Dashboards
1. **Pipeline** — funil de estados, latência por estágio, fila, falhas por pass.
2. **SLO** — disponibilidade/latência API vs error budget.
3. **Proteção em campo** — ataques por região/versão, técnica mais acionada.
4. **Custo** — custo por build, uso de spot, saturação de nós.
5. **Segurança** — tentativas de authz negadas, rate-limit, anomalias.

## Alertas (exemplos)
| Alerta | Condição | Severidade |
|--------|----------|-----------|
| Fila crescente | `queue_lag` > N por 10 min | Alta |
| Falha por pass | *spike* de `build_failures_total{pass}` | Alta |
| Drift de overhead | overhead médio > budget +X% | Média |
| SLO burn rate | error budget queimando 14x (1h) | Crítica |
| Authz anômala | 403 acima do baseline | Média |

## Health checks
- `/livez` (processo vivo), `/readyz` (deps prontas: DB/fila), `/healthz` agregado. K8s liveness/readiness/startup probes. Synthetic checks (Blackbox exporter) nas rotas críticas.

## SLIs → SLOs
Ver [03-nfr.md](03-nfr.md#sla--slo--sli). Error budget policy: se estourar, congela features e prioriza confiabilidade.
