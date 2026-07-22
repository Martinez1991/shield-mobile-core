# SHIELD Mobile Core — Engine de Ofuscação (Community Edition)

> **Núcleo open-source** da plataforma SHIELD, licenciado **Apache-2.0** (ver
> [`LICENSE`](LICENSE)). Contém o engine de proteção e todas as transformações,
> os gates de execução (golden/ART, host, arm64/qemu, on-device, iOS Simulator),
> a CLI, o `native-svc` (LLVM) e o SDK. É **Go puro, stdlib-only, zero dependências
> externas e determinístico**. As camadas de plataforma/Enterprise (worker + fila,
> observabilidade OTel, ingest de RASP, RBAC/SSO, compliance, antifraude, cloud)
> vivem em um repositório separado e consomem este core como um módulo Go.

Ferramenta de ofuscação de código Android que implementa o **Engine de Ofuscação**
(Seção 3 de [`shield-platform.md`](shield-platform.md)) como uma CLI real. O **engine
é Go puro (stdlib, zero dependências) e determinístico** — este repositório não puxa
nenhuma dependência externa (`go.mod` sem `require`).

A engine opera sobre **Smali** — a representação editável de bytecode Dalvik produzida
por `baksmali`/`apktool` — usada como uma **SHIELD-IR simplificada** (doc §2.2 estágios
6–7 e §4). Isso entrega um obfuscador Android que roda e é testável **offline hoje**,
sem escrever um parser DEX do zero.

```
APK ──(apktool d)──▶ smali ──[ passes SHIELD ]──▶ smali ──(apktool b)──▶ APK ──(apksigner)──▶ .apk protegido
                                    ▲
                          é aqui que esta ferramenta atua
```

## Instalação / build

Requer Go 1.26+.

```bash
go build -o shield ./cmd/shield      # Linux/macOS
go build -o shield.exe ./cmd/shield  # Windows
```

## Uso

```bash
# Análise estática: inventário + detecção de segredos (doc §2.2/§9.1)
shield analyze examples/smali
shield analyze examples/smali --json

# Ofuscar um projeto smali já decodificado
shield obfuscate examples/smali --out out --policy examples/policy-prod-high.json
shield obfuscate examples/smali --out out --preset prod-high --report out/report.json
# Logs estruturados (text|json) correlacionados por build_id:
shield obfuscate examples/smali --out out --preset prod-high --log-format json --verbose
# Cache content-addressed (determinismo P2): mesmo input+policy -> reusa a saída (~15x mais rápido):
shield obfuscate examples/smali --out out --preset prod-high --cache .shield-cache

# Round-trip completo de APK (requer apktool no PATH; apksigner p/ assinar)
# A senha do keystore NUNCA vai no argv (CWE-214): use --ks-pass-file ou a env SHIELD_KS_PASS.
SHIELD_KS_PASS=senha shield protect app.apk --out app-protegido.apk --preset prod-high \
  --ks release.jks --ks-alias chave
# ou: shield protect app.apk --out out.apk --ks release.jks --ks-alias chave --ks-pass-file pass.txt

# Policy-as-code
shield policy show prod-high
shield policy validate examples/policy-prod-high.json

# Retrace de stack trace ofuscado (usa o mapping.txt de renames de classe)
shield retrace out/mapping.txt crash.txt   # ou: cat crash.txt | shield retrace out/mapping.txt
```

Códigos de saída (doc §12): `0` ok · `≥10` falha de proteção · `≥20` falha de policy.

## Docker & GitHub Action

Imagem oficial (CLI Go, CGO-free, distroless — publicada em `ghcr.io/martinez1991/shield-mobile-core`):

```bash
docker run --rm -v "$PWD:/work" ghcr.io/martinez1991/shield-mobile-core:latest \
  obfuscate --config shield.yml
```

GitHub Action (lê o `shield.yml` e roda a proteção no pipeline):

```yaml
- uses: Martinez1991/shield-mobile-core@v0.7.0
  with:
    command: obfuscate      # obfuscate | analyze | policy
    config: shield.yml
```

A imagem `:latest` (distroless) cobre `analyze`/`obfuscate`/`policy`/`retrace`
(incl. `--config`). O `protect` completo (round-trip de APK) precisa
adicionalmente de **apktool/apksigner** — use a variante **`-toolchain`**, que já
os empacota:

```bash
docker run --rm -v "$PWD:/work" ghcr.io/martinez1991/shield-mobile-core:latest-toolchain \
  protect app.apk --out app-protected.apk --preset prod-high \
         --ks release.jks --ks-alias chave --ks-pass-file pass.txt
```

Veja **[Assinando o app protegido](#assinando-o-app-protegido-android)** para o
fluxo de assinatura (keystore vs certificado) e a solução dos erros comuns.

## Assinando o app protegido (Android)

Todo APK/AAB protegido precisa ser **assinado** antes de instalar num device ou
publicar. **A assinatura é sempre a última alteração** — ela encapsula o conteúdo
do pacote, então é a única mudança que você pode fazer num app protegido sem
quebrá-lo. **Não rode `zipalign` depois de assinar** (se precisar alinhar, alinhe
*antes*).

### Keystore ≠ certificado

Dois artefatos diferentes, e trocá-los é a causa dos erros mais comuns:

| Artefato | O que é | Para que serve |
|----------|---------|----------------|
| **Keystore** (`.jks`/`.keystore`/`.p12`) | contém sua **chave privada** | **assinar** o app (`apksigner --ks ...`) |
| **Certificado de assinatura** (`.pem`/`.der`) | chave **pública** derivada da privada | vincular o app à sua chave (*key-binding*, anti-adulteração) — **feature de roadmap; o SHIELD ainda não o consome** |

> O certificado **nunca** vai no `--ks`. `--ks` recebe o **keystore** (com a chave
> privada). É seguro compartilhar o certificado público; a chave privada, nunca.

### Duas formas de assinar

**(A) Inline, durante o `protect`** — o `protect` roda `apksigner` no final:

```bash
shield protect app.apk --out app-protected.apk --preset prod-high \
  --ks release.jks --ks-alias chave --ks-pass-file pass.txt
```
A senha **nunca** vai no argv (CWE-214): use `--ks-pass-file <arquivo>` ou a env
`SHIELD_KS_PASS`. O `--ks-alias` é **obrigatório** e precisa bater com a entrada
de chave do keystore. (O `protect` usa a senha do store também para a chave; se a
senha da chave for diferente da senha do store, assine pela forma **(B)**.)

**(B) Separado, depois do `protect`** (recomendado quando você precisa de
`zipalign`, de uma senha de chave distinta, ou de controle total):

```bash
# 1) proteger SEM assinar (sem as flags --ks*)
shield protect app.apk --out app-protected-unsigned.apk --preset prod-high

# 2) (opcional, p/ Google Play) alinhar ANTES de assinar
zipalign -p -f 4 app-protected-unsigned.apk app-aligned.apk

# 3) assinar — última etapa
apksigner sign --ks release.jks --ks-key-alias chave app-aligned.apk

# 4) conferir
apksigner verify --print-certs app-aligned.apk
```

Não sabe o alias? Liste as entradas do keystore:

```bash
keytool -list -v -keystore release.jks
```

### Cenários de publicação

- **APK assinado pelo desenvolvedor** — assine com sua chave (comandos acima). É
  o APK que você instala/distribui.
- **APK via Google Play (Play App Signing)** — assine com a sua **chave de
  upload** (`apksigner sign --ks upload.jks --ks-key-alias upload <apk>`); o Google
  re-assina com a chave de assinatura dele. O APK assinado só com a upload key não
  serve para testar — baixe do Play o APK assinado pelo Google.
- **AAB via Google Play** — assine o bundle com `jarsigner` e a chave de upload:
  ```bash
  jarsigner -keystore upload.jks -keypass <senha-chave> -storepass <senha-store> \
    app-protected.aab <alias>
  ```

### Solução de problemas (erros comuns)

| Erro | Causa | Correção |
|------|-------|----------|
| `keystore ... doesn't have the key` / "não possui a chave" | `--ks-alias` errado, keystore sem chave privada nesse alias, ou senha incorreta | confira com `keytool -list -v -keystore ...`; use o alias/senha corretos; se a senha da chave difere da do store, assine pela forma **(B)** |
| `apksigner failed ... toDerInputStream rejects tag type 66` | você passou o **certificado** (`.pem`/`.der`) no `--ks` | `--ks` recebe o **keystore** (com a chave privada), não o certificado |
| `apktool not found on PATH` | usou a imagem `:latest` (distroless) para `protect` | use a imagem **`:latest-toolchain`** |
| `Invalid register: vNN. Must be between v0 and v15` | cifra de strings usando `invoke` não-range num registrador alto (v16+) — corrigido no `main` (usa `/range`) | atualize a imagem `:latest-toolchain` (`docker pull`) |

> Se o seu app usa **Play App Signing**, o certificado que a Verimatrix (ou
> qualquer ferramenta de *key-binding*) pede é o **App Signing Certificate** do
> Play Console → *Integridade do app*. No SHIELD atual isso não é necessário: você
> apenas **assina** o app protegido com a sua chave (upload key).

## Técnicas implementadas (mapeadas ao documento)

| Doc | Técnica | Status | Notas |
|-----|---------|--------|-------|
| §3.5 | **Metadata Removal** | ✅ | Remove `.line`, `.local`, `.prologue`, `.source` (debug info). |
| §3.3 | **String Encryption** | ✅ | XOR (low-risk) **ou AES-256-GCM** com chave derivada em runtime (`key=SHA-256(material)`, nunca literal); decryptor `Lshield/rt/SH;` injetado. |
| §6 | **RASP (runtime)** | ✅ | Injeta `Lshield/rt/RASP;`: detecção de root/debugger/emulador/**Xposed**/**Frida** + `flags()` bitmask + primitiva `react()` (§6.1). **Auto-ofuscado**: injetado antes dos passes, então suas assinaturas (`su`, `frida`, Build) são cifradas e o control-flow embaralhado (API pública estável). |
| §8 | **Code Virtualization (VM)** | ✅ | Compila métodos `static` para **bytecode próprio** interpretado por `Lshield/rt/VM;` (fetch/decode/dispatch, opcodes embaralhados por build, §8.1). Cobre aritmética **int e long/64-bit**, branches/loops, narrowing, **objetos** (params/`move-object`/`return-object`), **const-string** (pool virtualizado), e **`invoke` data-driven** por reflexão (`static`/`virtual`/`interface`; args e retornos int/long/objeto). Tudo verificado byte-a-byte em **ART real** ([#14](https://github.com/Martinez1991/shield-platform/issues/14)). |
| §3.1 | **Class/Type Renaming** | ✅ | Renomeia classes/tipos *reachability-aware*; **keep-rules automáticas do AndroidManifest.xml** (Activities/Services/Providers/Receivers nunca renomeados), reescreve referências, gera `mapping.txt`. |
| §3.1 | **Member Renaming** | ✅ | Renomeia métodos `private`/`static` e campos `private` (nunca vtable/overrides); enums e classes kept preservados. |
| §3.2 | **Opaque predicates** | ✅ | Predicados always-true + fake branches, *verifier-safe* (reusa registrador local livre no entry, sem realocação). |
| §7 | **Block Reordering** | ✅ | Embaralha basic blocks e reconecta com `goto` (*flattening* de layout, seguro por construção). |
| §3.2/8 | **Control-Flow Flattening** | ✅ | Dispatcher central `packed-switch`: cada bloco vira um case dirigido por um registrador de estado. Dirigido pela **IR tipada** ([ADR 0001](docs/adr/0001-typed-ir.md)) — gate de consistência de tipos (inferência) + registrador de estado morto (liveness), garantindo zero conflito no verificador. Verificado em ART ([#20](https://github.com/Martinez1991/shield-platform/issues/20)). |
| §3.2 | **Junk code** | ✅ | Padding com `nop` no início dos métodos. |
| §9.1 | **Detecção de código sensível** | ✅ (heurística) | Regex (Stripe/AWS/JWT/GCP/private key) + entropia de Shannon. |
| P2 | **Determinismo** | ✅ | Mesmo input + policy + seed ⇒ output idêntico (testado). |
| P4 | **Policy-as-Code** | ✅ | Policy JSON versionável + presets + validação. |

### Plataforma (v0.5.0 → v0.7.0)

| Capacidade | Status | Notas |
|-----------|--------|-------|
| **IR tipada** (`internal/ir`) | ✅ | Parser estruturado + inferência de tipos + liveness ([ADR 0001](docs/adr/0001-typed-ir.md)); base do flattening e do invoke da VM. |
| **Proteção risk-driven** (`internal/risk`) | ✅ | Score de risco por método (features estáticas + heurística explicável, sem ML); `risk.enabled`+`threshold` concentra VM/flattening nos hot spots; `Result.RiskMap` auditável. |
| **Proteção de código nativo** (`native-svc`) | ✅ | Serviço LLVM **out-of-tree** ([ADR 0004](docs/adr/0004-llvm-native-svc.md)) via subprocess; 6 passes sobre bitcode (**flatten/MBA/opaque/strings** + **RASP anti-debug/anti-tamper**), cada um com **gate de execução** (host + arm64 sob qemu). O worker Go (`ProtectArchive`/`cmd/shield-nativeapk`) dirige o round-trip de APK end-to-end. |
| **Análise de binários** (`internal/inspect`) | ✅ | `shield analyze <ipa\|apk\|aab>` reporta Mach-O/ELF (arquitetura, seções, símbolos, densidade de segredos). |
| **iOS** (`internal/ios`, `cmd/shield-iosstrip`) | ✅ strip | Inspeção Mach-O + **metadata strip** (símbolos/`__LINKEDIT`) com re-assinatura ad-hoc, round-trip de IPA; gates no runner **macOS** (`xcrun strip`/`codesign`) e **differential no Simulador iOS** (`simctl spawn`). Re-assinatura de distribuição (cert pago) → roadmap. |
| **Fundação nativa** (`internal/native`) | ✅ análise | Inspeção de `.so` ELF via `debug/elf` (secções/símbolos/`.rodata`). |
| **AAB / App Bundle** | ✅ | Round-trip de bundle (`shield protect app.aab`) preservando entries byte-a-byte; **keep-rules do manifesto protobuf** (parser aapt2 hand-rolled). |
| **Worker sandboxed + fila** | ✅ | `cmd/shield-worker` consome `queue.Queue` (Mem/Dir/**NATS JetStream**); deploy gVisor + no-egress + **KEDA** por profundidade de fila ([`deploy/`](deploy/)). |
| **Observabilidade** | ✅ | Métricas Prometheus por estágio + spans **OTLP** (opt-in); dashboards/alertas Grafana ([`deploy/observability/`](deploy/observability/)). |
| **Ingest RASP em campo** | ✅ | `cmd/rasp-ingest`: callbacks HMAC por-tenant + anti-replay → métricas de tamper por tenant/tipo. |

### Validação de round-trip (DEX real)

O output ofuscado foi validado montando-o de volta em DEX com o *assembler* `smali`
(que faz verificação estrutural de registradores, labels e control-flow) e
desmontando com `baksmali`:

```bash
SMALI_JAR=~/tools/smali-2.5.2.jar BAKSMALI_JAR=~/tools/baksmali-2.5.2.jar \
  ./scripts/validate-roundtrip.sh
# → ROUND-TRIP OK: obfuscated smali assembles to a valid, well-formed DEX; no plaintext secret.
```

O mesmo round-trip roda como teste de integração automatizado quando o jar está
disponível (senão é *skipped*):

```bash
SHIELD_SMALI_JAR=~/tools/smali-2.5.2.jar go test ./internal/engine -run RoundTrip -v
```

Jars: <https://bitbucket.org/JesusFreke/smali/downloads/> (`smali-2.5.2.jar`, `baksmali-2.5.2.jar`).

O **interpretador da VM** (§8) e a cifra **AES-256-GCM** (§3.3) foram validados
também na JVM (mesma semântica de int/cripto que a ART do Android): Go emite o
bytecode/ciphertext e um port Java do algoritmo injetado reproduz os resultados
exatos. Reproduza a VM com:

```bash
./scripts/validate-vm.sh    # → VM-JVM-VALIDATION OK   (só precisa de javac/java)
./scripts/validate-aes.sh   # → AES-JVM-VALIDATION OK  (unmask + SHA-256 + GCM)
```

O material de chave AES é embutido **mascarado** (XOR keystream por build), nunca
como bloco literal, e desmascarado em runtime antes de derivar a chave. Os parsers
smali têm **fuzz tests** (`go test -fuzz`) rodados no CI.

### Red-team (KPI de reversão)

`scripts/redteam.sh` decompila o DEX baseline vs protegido com o **jadx** (decompilador
real) e mede reversibilidade — com um **gate rígido**: se um segredo conhecido sobreviver
na saída protegida, sai com erro (regressão de bypass).

```bash
SMALI_JAR=~/tools/smali-2.5.2.jar JADX_CMD=~/tools/jadx/bin/jadx ./scripts/redteam.sh
# → GATE OK: no known secret leaked into the protected output.
```

Exemplo de saída (fixture): segredo no baseline = 1 arquivo, no protegido = 0; LOC
decompilado 31 → 245 (ruído de proteção); chamadas `SH.d`/`VM.run` presentes. jadx:
<https://github.com/skylot/jadx>.

### Gate de corretude runtime (doc §20, issue #3)

*Differential testing* de **golden apps em ART real**: `testdata/golden` tem um
`main` determinístico exercitando rename/VM/reorder/opaque/strings; o gate
monta o dex **original vs protegido**, roda ambos via `app_process` num emulador
Android e exige **saída byte-idêntica** (divergência = obfuscação quebrou a
semântica → falha). Local: valida a montagem; a execução em ART roda no CI
(workflow `correctness`, emulador).

```bash
SMALI_JAR=~/tools/smali-2.5.2.jar ./scripts/golden-diff.sh   # com device/emulador: GATE OK
```

### Correção semântica (doc §20)

As transformações preservam a semântica por construção:

- **String encryption** reusa o mesmo registrador (sem alterar `.locals`) e retorna
  em runtime exatamente o valor original.
- **Renaming** só toca tipos sob `includePrefixes` (nunca `android/`, `java/`, `kotlin/`,
  libs) e respeita `keepClasses` — protege entry points e reflection.
- **Junk** insere apenas `nop` após os diretivos `.param`/`.annotation`, mantendo a
  verificação do Dalvik.

## Serviço (job-svc)

Fatia do control/build plane (doc §1.2): serviço HTTP que envolve o engine com
máquina de estados (§2.3), API REST (§11.1), **autenticação (API key / JWT
HS256), RBAC deny-by-default, isolamento por tenant, auditoria hash-chained e
quotas** (§14). Storage em memória + disco (sem infra externa).

```bash
SHIELD_API_KEY=devkey SHIELD_TENANT=acme SHIELD_ROLES=developer \
  go run ./cmd/job-svc --addr :8080 --work ./_work
# (ou JWT HS256: SHIELD_JWT_SECRET / SHIELD_JWT_ISS / SHIELD_JWT_AUD)

# POST /v1/builds  -H "X-API-Key: devkey" (multipart: artifact=<zip smali>, policy=prod-high, Idempotency-Key)
# GET  /v1/builds/{id}            -> status + eventos (QUEUED..READY/FAILED)   [tenant-scoped]
# GET  /v1/builds/{id}/report     -> evidência (engine.Result)
# GET  /v1/builds/{id}/artifact   -> zip protegido
# GET  /v1/audit                  -> trilha imutável do tenant + verified
# GET  /healthz /livez /readyz    (público)
```

Códigos: `401` sem/credencial inválida · `403` sem permissão · `402` quota
excedida · `404` build de outro tenant (isolamento).

## Arquitetura do código

```
cmd/shield/            CLI (analyze / obfuscate / protect / policy / retrace / version)
cmd/job-svc/           serviço HTTP (build orchestration + máquina de estados)
internal/smali/        SHIELD-IR: loader + helpers de type descriptor
internal/policy/       Policy-as-Code (JSON) + presets + Planner
internal/engine/       passes: metadata, strings(+SH: xor/aes), member/class
                       rename(+mapping), code virtualization (VM compiler +
                       interpreter generator), block reordering, opaque
                       predicates, junk, RASP injection
internal/analyze/      inventário + detecção de segredos (entropia/regex)
internal/apk/          orquestração do round-trip com apktool/apksigner
examples/              projeto smali de exemplo + policy
```

## Testes

```bash
go test ./...
```

Cobrem: round-trip de cifra (incl. unicode/determinismo), pipeline completo
(metadata/strings/rename/junk + mapping + decifração de volta), recusa de rename
sem escopo, e detecção de segredos com preview mascarado.

## Roadmap (próximas camadas do doc)

Este MVP é o núcleo do `obfuscator-svc` (doc §1.2). A VM (§8) já virtualiza
métodos de aritmética inteira; ampliá-la para fluxo de controle/objetos, e o
control-flow **flattening** com dispatcher central, exigem reconstrução de
tipos/liveness e verificação de runtime da ART (que o smali-texto não carrega),
por isso ficam fora deste MVP que só entrega transforms verificáveis. Também
**não** implementado: proteção nativa via LLVM (§3.7), SDK RASP nativo com
tripwires Frida/Xposed e reação distribuída (§6), e o control/build plane de
microsserviços (§1.2). Ver §17 (Roadmap) e §22 (esforço) do documento.

> ⚠️ Uso autorizado apenas: proteção de apps próprios / com autorização. O `mapping.txt`
> é sensível — guarde-o para *retrace* de stack traces.
