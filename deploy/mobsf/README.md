# MobSF MAST integration

SHIELD runs a **MAST** (Mobile Application Security Testing) scan by orchestrating
[MobSF](https://github.com/MobSF/Mobile-Security-Framework-MobSF) as an external
service — the client lives in [`internal/mast`](../../internal/mast).

## Why a subprocess/service, not a library

MobSF is **GPL-3.0**. SHIELD calls it over its REST API and never links its code,
so the SHIELD engine stays **Apache-2.0 and dependency-free** (the `mast` client
is pure stdlib). This is the same boundary discipline used for apktool / NDK / qemu.

## Run it

```bash
docker compose -f deploy/mobsf/docker-compose.yml up -d
# grab the REST API key (printed on first start), or set MOBSF_API_KEY yourself:
docker compose -f deploy/mobsf/docker-compose.yml logs mobsf | grep -i "api key"
```

## Use it (Go)

```go
c := mast.New("http://localhost:8000", apiKey)
rep, _ := c.ScanFile(ctx, "app.apk")          // upload → scan → report
fmt.Println(rep.SecurityScore, len(rep.High)) // MobSF security score + high findings
```

## The analyze → protect → verify loop

The value is the **differential**: scan an app, protect it with SHIELD, scan
again, and prove the risk dropped — the risk-plane analogue of the golden/ART
correctness gate.

```go
before, _ := c.ScanFile(ctx, "app.apk")
// ... shield protects app.apk -> app-protected.apk ...
after, _ := c.ScanFile(ctx, "app-protected.apk")
d := mast.Diff(before, after)
fmt.Printf("score %+d, resolved %d high findings\n", d.ScoreDelta, len(d.ResolvedHigh))
```

`mast.Diff` reports the score delta and which high-severity findings SHIELD
resolved — the evidence a compliance/Enterprise report is built on.
