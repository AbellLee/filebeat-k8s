FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/control-server ./server/cmd/control-server

FROM alpine:3.22
RUN adduser -D -H -u 10001 appuser
COPY --from=build /out/control-server /usr/local/bin/control-server
USER 10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/control-server"]
