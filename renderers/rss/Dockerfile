FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY renderers/rss/go.mod renderers/rss/go.sum ./
RUN go mod download && go mod verify
COPY renderers/rss/*.go ./
RUN go build -mod=readonly -trimpath -ldflags="-s -w" -o /out/renderer .

FROM build AS tests
RUN go test -mod=readonly -trimpath -c -o /out/renderer.test .

FROM scratch AS test-export
COPY --from=tests /out/renderer.test /renderer.test

FROM scratch AS runtime
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.title="ARPRT RSS renderer" \
      org.opencontainers.image.description="RSS Job renderer maintained by the independent, unofficial ARPRT project; not affiliated with or endorsed by TRMNL." \
      org.opencontainers.image.source="https://github.com/dayflower/arprt-renderers" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.licenses="MIT"
WORKDIR /app
COPY --from=build /out/renderer /app/renderer
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/local/go/LICENSE /usr/share/licenses/arprt-renderer-rss/Go-LICENSE
COPY LICENSE /usr/share/licenses/arprt-renderer-rss/LICENSE
COPY renderers/rss/THIRD_PARTY_NOTICES.md /usr/share/licenses/arprt-renderer-rss/THIRD_PARTY_NOTICES.md
USER 1000:1000
ENTRYPOINT ["/app/renderer"]
