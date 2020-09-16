FROM golang:1.14.9-alpine3.12 as builder

RUN apk update \
    && apk upgrade \
    && apk add --no-cache \
    curl ca-certificates protobuf \
    git make \
    && rm -rf /var/cache/apk/* /tmp/*

RUN mkdir -p /src/geoipd/
COPY . /src/geoipd/
WORKDIR /src/geoipd/

ARG GIT_HASH
ARG GIT_TAG
RUN make clean && make tidy && make GIT_HASH=${GIT_HASH} GIT_TAG=${GIT_TAG}

FROM alpine:3.12

RUN apk update \
    && apk upgrade \
    && apk add --no-cache \
      redis \
    && rm -rf /var/cache/apk/* /tmp/*

COPY --from=builder /src/geoipd/geoipd /usr/sbin/
RUN mkdir -p /etc/geoipd/
COPY .docker/config.yaml /etc/geoipd/
COPY .docker/entrypoint.sh /

ENV GEOIP2_LICENSE_KEY=

EXPOSE 8080

VOLUME ["/var/lib/redis"]
VOLUME ["/var/lib/geoipd"]

CMD ["/entrypoint.sh"]
