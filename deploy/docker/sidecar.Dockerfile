FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/control-sidecar ./sidecar/cmd/control-sidecar

FROM alpine:3.22
COPY --from=build /out/control-sidecar /usr/local/bin/control-sidecar
ENTRYPOINT ["/usr/local/bin/control-sidecar"]
