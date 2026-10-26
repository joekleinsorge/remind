FROM golang:1.26.7-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY web ./web
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/remind . && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/remind /app/remind
COPY --from=build --chown=65532:65532 /out/data /data
COPY email.tmpl /app/email.tmpl

ENTRYPOINT ["/app/remind"]
CMD ["--serve=:8080", "--state=/data/remind-state.json", "--clippings=/data/clippings.txt"]
EXPOSE 8080
