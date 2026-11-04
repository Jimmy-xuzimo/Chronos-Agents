FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS build
ARG TARGETOS TARGETARCH
ARG COMPONENT=agent
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags headless -trimpath -ldflags='-s -w' -o /out/chronos ./cmd/$COMPONENT

FROM alpine:3.22
LABEL org.opencontainers.image.source="https://github.com/Jimmy-xuzimo/Chronos-Agents"
LABEL org.opencontainers.image.licenses="Apache-2.0"
RUN apk add --no-cache ca-certificates tzdata && addgroup -g 10001 chronos && adduser -D -u 10001 -G chronos chronos && mkdir /data && chown chronos:chronos /data
COPY --from=build /out/chronos /usr/local/bin/chronos
USER 10001:10001
VOLUME /data
ENTRYPOINT ["/usr/local/bin/chronos"]
