# SHIELD — Engine de Ofuscação (MVP)

Ferramenta de ofuscação de código Android que implementa o **Engine de Ofuscação**
(Seção 3 de [`shield-platform.md`](shield-platform.md)) como uma CLI real, escrita em
**Go puro (stdlib, zero dependências)**.

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

# Round-trip completo de APK (requer apktool no PATH; apksigner p/ assinar)
# A senha do keystore NUNCA vai no argv (CWE-214): use --ks-pass-file ou a env SHIELD_KS_PASS.
SHIELD_KS_PASS=senha shield protect app.apk --out app-protegido.apk --preset prod-high \
  --ks release.jks --ks-alias chave
# ou: shield protect app.apk --out out.apk --ks release.jks --ks-alias chave --ks-pass-file pass.txt

# Policy-as-code
shield policy show prod-high
shield policy validate examples/policy-prod-high.json
```

Códigos de saída (doc §12): `0` ok · `≥10` falha de proteção · `≥20` falha de policy.

## Técnicas implementadas (mapeadas ao documento)

| Doc | Técnica | Status | Notas |
|-----|---------|--------|-------|
| §3.5 | **Metadata Removal** | ✅ | Remove `.line`, `.local`, `.prologue`, `.source` (debug info). |
| §3.3 | **String Encryption** | ✅ | XOR (low-risk) **ou AES-256-GCM** com chave derivada em runtime (`key=SHA-256(material)`, nunca literal); decryptor `Lshield/rt/SH;` injetado. |
| §6 | **RASP (runtime)** | ✅ básico | Injeta `Lshield/rt/RASP;`: detecção de root/debugger/emulador + `flags()` bitmask (modelo detecção→flag→reação diferida §6.1). |
| §8 | **Code Virtualization (VM)** | ✅ cirúrgico | Compila métodos `static` de aritmética inteira para **bytecode próprio** interpretado por `Lshield/rt/VM;` (fetch/decode/dispatch), com **opcodes embaralhados por build** (polimorfismo §8.1). |
| §3.1 | **Class/Type Renaming** | ✅ | Renomeia classes/tipos *reachability-aware*; **keep-rules automáticas do AndroidManifest.xml** (Activities/Services/Providers/Receivers nunca renomeados), reescreve referências, gera `mapping.txt`. |
| §3.1 | **Member Renaming** | ✅ | Renomeia métodos `private`/`static` e campos `private` (nunca vtable/overrides); enums e classes kept preservados. |
| §3.2 | **Opaque predicates** | ✅ | Predicados always-true + fake branches, *verifier-safe* (reusa registrador local livre no entry, sem realocação). |
| §7 | **Block Reordering** | ✅ | Embaralha basic blocks e reconecta com `goto` (*flattening* de layout, seguro por construção: ordem de execução e tipos preservados). Dispatcher central + ISA polimórfica → roadmap. |
| §3.2 | **Junk code** | ✅ | Padding com `nop` no início dos métodos. |
| §9.1 | **Detecção de código sensível** | ✅ (heurística) | Regex (Stripe/AWS/JWT/GCP/private key) + entropia de Shannon. |
| P2 | **Determinismo** | ✅ | Mesmo input + policy + seed ⇒ output idêntico (testado). |
| P4 | **Policy-as-Code** | ✅ | Policy JSON versionável + presets + validação. |

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

### Correção semântica (doc §20)

As transformações preservam a semântica por construção:

- **String encryption** reusa o mesmo registrador (sem alterar `.locals`) e retorna
  em runtime exatamente o valor original.
- **Renaming** só toca tipos sob `includePrefixes` (nunca `android/`, `java/`, `kotlin/`,
  libs) e respeita `keepClasses` — protege entry points e reflection.
- **Junk** insere apenas `nop` após os diretivos `.param`/`.annotation`, mantendo a
  verificação do Dalvik.

## Arquitetura do código

```
cmd/shield/            CLI (analyze / obfuscate / protect / policy / version)
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
