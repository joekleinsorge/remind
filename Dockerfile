FROM golang:1.26.7-bookworm@sha256:e8c859f5632dcfde7b32d2012b4351728f6437930887c2f6a91ea242459e5514 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY web ./web
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/remind . && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

WORKDIR /app
COPY --from=build /out/remind /app/remind
COPY --from=build --chown=65532:65532 /out/data /data
COPY email.tmpl /app/email.tmpl

ENTRYPOINT ["/app/remind"]
CMD ["--serve=:8080", "--state=/data/remind-state.json", "--clippings=/data/clippings.txt"]
EXPOSE 8080
