# 99 — Checklists

## Checklist de Desenvolvimento (por feature)
- [ ] RF/US e critérios de aceite (Gherkin) definidos e rastreados a uma issue
- [ ] ADR registrado se houver decisão arquitetural
- [ ] Design da API (OpenAPI/SDL) atualizado *design-first* + lint (Spectral)
- [ ] Modelo de dados/migração (expand-and-contract) revisado por DBA
- [ ] Ports & adapters: sem lógica de negócio no adapter; casos de uso testados
- [ ] Feature flag para rollout gradual (quando aplicável)
- [ ] Logs estruturados + métricas + spans OTel adicionados (sem PII/segredos)
- [ ] Idempotência/timeouts/retries onde há efeitos colaterais
- [ ] Testes: unit + integração + (se fluxo) e2e; property-based p/ IR
- [ ] Documentação atualizada (README do serviço)

## Checklist de QA
- [ ] Casos felizes e de borda cobertos; tabela de equivalência/limite
- [ ] **Corretude semântica**: golden apps sem divergência (gate)
- [ ] Contrato (Pact) verde entre produtor/consumidor
- [ ] Compatibilidade (matriz SDK/ABI/device) quando toca o output
- [ ] Performance dentro do budget (overhead runtime/tamanho, latência API)
- [ ] Acessibilidade (axe-core) AA nas telas alteradas
- [ ] i18n (pt-BR/en) sem *hardcoded strings*
- [ ] Teste de tenancy (sem cross-tenant); authz negativa (403)
- [ ] Cobertura ≥ meta; sem flakiness (retries verdes 3x)

## Checklist de Segurança (por PR e por release)
- [ ] SAST (CodeQL/Semgrep) sem high/critical
- [ ] SCA (govulncheck/Trivy/OSV) sem high/critical; deps *pinned*
- [ ] Secret scan (gitleaks) limpo; nenhum segredo em código/log/argv
- [ ] IaC scan (Checkov/tfsec) sem crítico; PSS restricted
- [ ] Container scan (Trivy/Grype) base distroless, non-root, read-only FS
- [ ] Fuzzing de parsers sem crash no corpus
- [ ] Threat model (STRIDE) revisado para a mudança
- [ ] Authz por objeto (BOLA/BFLA) testada
- [ ] Segredos via Vault; chaves só no broker/HSM; sem egress no worker
- [ ] SBOM gerado + imagens assinadas (cosign) + proveniência SLSA

## Checklist de Deploy
- [ ] Migração de DB testada em staging (up + down); backup/PITR ok
- [ ] Estratégia definida (rolling/blue-green/canary) + critério de rollback
- [ ] Health checks (`/livez` `/readyz`) e probes configurados
- [ ] Dashboards/alertas atualizados; SLO/error budget verificado
- [ ] Feature flags no estado correto; segredos/rotação aplicados
- [ ] Runbook de rollback e on-call notificado
- [ ] Canary observado (taxa de falha por pass, overhead, erros) antes de 100%

## Checklist de Produção (go-live)
- [ ] SLA/SLO/SLI publicados e monitorados; alertas de burn-rate
- [ ] Backup/DR testado (restore real); RTO/RPO validados
- [ ] WAF/Shield/rate-limit ativos; IAM least-privilege revisado
- [ ] LGPD: DPA, base legal, DPIA (telemetria), pseudonimização, DPO
- [ ] Auditoria imutável ativa; retenção configurada
- [ ] Purge de binários/artefatos (TTL) funcionando
- [ ] Runbooks de incidente; escalonamento on-call; status page
- [ ] Pentest/red-team inicial concluído; achados high tratados
- [ ] Custo por build monitorado (FinOps); budgets/alertas
- [ ] Gate de release cumprido (corretude 100% golden apps)
