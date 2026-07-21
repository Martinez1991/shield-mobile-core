# SHIELD Mobile Core — minimal CLI image (tag `latest` / `vX.Y.Z`). The engine is
# pure Go / CGO-free, so this is a tiny distroless static binary (non-root). Covers
# analyze, obfuscate (incl. --config shield.yml), policy and retrace.
#
# The full `protect` APK/AAB round-trip additionally shells out to apktool and
# apksigner — use the `-toolchain` image variant (Dockerfile.toolchain,
# ghcr.io/<owner>/shield-mobile-core:latest-toolchain) for that. The LLVM
# native-svc is a separate out-of-tree build.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
# The core is zero-dependency (empty go.mod require), so no module download is needed.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/shield ./cmd/shield

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/shield /usr/local/bin/shield
WORKDIR /work
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/shield"]
CMD ["version"]
