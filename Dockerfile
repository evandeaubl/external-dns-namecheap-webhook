FROM docker.io/library/golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod ./
COPY cmd/ ./cmd/
COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/namecheap-webhook ./cmd/namecheap-webhook

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/namecheap-webhook /namecheap-webhook

ENTRYPOINT ["/namecheap-webhook"]
