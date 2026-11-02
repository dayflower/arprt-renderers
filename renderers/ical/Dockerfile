FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY renderers/ical/go.mod renderers/ical/go.sum ./
RUN go mod download && go mod verify
COPY renderers/ical/semantic.go renderers/ical/semantic_test.go ./
COPY renderers/ical/fixtures ./fixtures
COPY renderers/ical/cmd ./cmd
RUN CGO_ENABLED=0 go test -mod=readonly -c -o /out/semantics.test .
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/ical-probe ./cmd/ical-probe
COPY LICENSE /out/licenses/ARPRT
COPY renderers/ical/licenses /out/licenses
RUN cp /usr/local/go/LICENSE /out/licenses/Go-LICENSE

FROM scratch
COPY --from=build /out /app
USER 1000:1000
ENTRYPOINT ["/app/semantics.test", "-test.v"]
