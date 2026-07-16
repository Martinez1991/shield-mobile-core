# SHIELD Mobile Core — CLI image. The engine is pure Go / CGO-free, so the final
# image is a tiny distroless static binary (non-root). Covers analyze, obfuscate
# (incl. --config shield.yml), policy and retrace. The full `protect` APK/AAB
# round-trip additionally needs apktool/apksigner — a toolchain image variant is
# tracked as a follow-up; the LLVM native-svc is a separate out-of-tree build.
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
