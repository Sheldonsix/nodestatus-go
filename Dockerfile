FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/nodestatus-go ./cmd/nodestatus-go

FROM alpine:3.22

RUN addgroup -S nodestatus \
  && adduser -S -G nodestatus nodestatus \
  && mkdir -p /data \
  && chown -R nodestatus:nodestatus /data

COPY --from=build /out/nodestatus-go /usr/local/bin/nodestatus-go

USER nodestatus
EXPOSE 35601

ENV PORT=35601 \
  DATABASE=/data/db.sqlite \
  WEB_USERNAME=admin \
  WEB_SECRET=node-secret

ENTRYPOINT ["/usr/local/bin/nodestatus-go"]
