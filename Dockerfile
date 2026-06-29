# Multi-stage build for the Krovara Go binaries. Pick a target with:
#   docker build --target api    -t krovara/api .
#   docker build --target worker -t krovara/worker .
#   docker build --target voip   -t krovara/voip .
#
# All three share the same module download layer.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api    ./cmd/api    \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/voip   ./cmd/voip

# --- API -------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot AS api
COPY --from=build /out/api /api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]

# --- Worker ----------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot AS worker
COPY --from=build /out/worker /worker
USER nonroot:nonroot
ENTRYPOINT ["/worker"]

# --- VoIP (Pion SFU — group voice) -----------------------------------
FROM gcr.io/distroless/static-debian12:nonroot AS voip
COPY --from=build /out/voip /voip
USER nonroot:nonroot
EXPOSE 8083
ENTRYPOINT ["/voip"]
