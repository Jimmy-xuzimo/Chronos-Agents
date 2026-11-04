FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS build
ARG TARGETOS TARGETARCH
ARG COMPONENT=agent
RUN apk add --no-cache python3
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
COPY scripts/licenses.py ./scripts/licenses.py
COPY LICENSE NOTICE THIRD_PARTY_NOTICES.md /out/project-legal/
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags headless -trimpath -ldflags='-s -w' -o /out/chronos ./cmd/$COMPONENT

RUN python3 scripts/licenses.py /out/third-party

FROM alpine:3.22
LABEL org.opencontainers.image.source="https://github.com/Jimmy-xuzimo/Chronos-Agents"
LABEL org.opencontainers.image.licenses="Apache-2.0"
RUN apk add --no-cache ca-certificates tzdata && addgroup -g 10001 chronos && adduser -D -u 10001 -G chronos chronos && mkdir /data && chown chronos:chronos /data
COPY --from=build /out/chronos /usr/local/bin/chronos
COPY --from=build /out/third-party /usr/share/licenses/chronos/third-party
COPY --from=build /out/project-legal /usr/share/licenses/chronos/project
COPY --from=build /out/bom.cdx.json /usr/share/licenses/chronos/bom.cdx.json
USER 10001:10001
VOLUME /data
ENTRYPOINT ["/usr/local/bin/chronos"]
