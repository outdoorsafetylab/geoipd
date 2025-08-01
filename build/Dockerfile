FROM golang:1.24-alpine AS builder

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

# Environment variables (set at runtime via Cloud Run/Docker)
# Note: Sensitive values like GEOIP2_LICENSE_KEY are injected at deployment time
ENV GEOIP2_LICENSE_KEY=
ENV GEOIP2_CLOUD_STORAGE_BUCKET=
ENV GEOIP2_CLOUD_STORAGE_REGION=
ENV REDIS_HOST=127.0.0.1
ENV REDIS_PORT=6379
ENV REDIS_PASS=

EXPOSE 8080

VOLUME ["/var/lib/redis"]
VOLUME ["/var/lib/geoip"]

CMD ["/entrypoint.sh"]
