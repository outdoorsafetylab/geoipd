FROM golang:1.18-alpine as builder

RUN apk update \
    && apk upgrade \
    && apk add --no-cache \
      curl ca-certificates \
      git make \
    && rm -rf /var/cache/apk/* /tmp/*

RUN mkdir -p /src/
COPY . /src/
WORKDIR /src/

ARG GIT_HASH
ARG GIT_TAG
RUN make clean \
    && make tidy \
    && make GIT_HASH=${GIT_HASH} GIT_TAG=${GIT_TAG}

FROM alpine:3.16

RUN apk update \
    && apk upgrade \
    && apk add --no-cache \
      redis \
    && rm -rf /var/cache/apk/* /tmp/*

COPY --from=builder /src/geoip /usr/sbin/
COPY build/docker.yaml /
COPY build/entrypoint.sh /

ENV GEOIP2_LICENSE_KEY=

EXPOSE 8080

VOLUME ["/var/lib/redis"]
VOLUME ["/var/lib/geoip"]

CMD ["/entrypoint.sh"]
