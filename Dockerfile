FROM golang:1.17-alpine3.15 as build

COPY . /opt/clash-prometheus-exporter

WORKDIR /opt/clash-prometheus-exporter
RUN CGO_ENABLED=0 GOOS=linux go build


FROM alpine:3.15

COPY --from=build /opt/clash-prometheus-exporter/clash-prometheus-exporter  /bin/

RUN apk update && \
    apk add ca-certificates && \
    update-ca-certificates

EXPOSE 9869
ENTRYPOINT ["/bin/clash-prometheus-exporter"]
