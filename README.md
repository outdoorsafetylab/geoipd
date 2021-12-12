# GeoIP2 Auto-Renewing REST API Server

## Prerequisites

Before testing or deploying this server, you have to [acquire your own license key](https://support.maxmind.com/account-faq/license-keys/how-do-i-generate-a-license-key/) for GeoIP2 DB. And it's free for Lite version but still required.

## Run with DockerHub Image

```shell
docker run -it --rm \
    -p 8080:8080 \
    -e GEOIP2_LICENSE_KEY=<your_license_key> \
    outdoorsafetylab/geoipd
```

It will take some time to download the `GeoLite2-City` DB, please wait until it finishes. You can start testing it after seeing a log message `Serving HTTP: [::]:8080`:

```shell
curl "http://localhost:8080/v1/city
```

It will print a useless result since the server detected a bridged IP address inside docker. Use the following command to simulate the scenario after deploying it:

```shell
curl "http://localhost:8080/v1/city?ip=$(curl -4 https://icanhazip.com)"
```
