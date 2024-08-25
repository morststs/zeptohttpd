FROM --platform=$BUILDPLATFORM golang:1.23.0-alpine3.20 AS build
ARG TARGETPLATFORM
WORKDIR /build
COPY ./src /build
RUN [ "${TARGETPLATFORM}" = "linux/amd64" ] && CGO_ENABLED=0 GOARCH=amd64 go build -ldflags="-s -w" -trimpath ||:
RUN [ "${TARGETPLATFORM}" = "linux/arm64" ] && CGO_ENABLED=0 GOARCH=arm64 go build -ldflags="-s -w" -trimpath ||:
RUN [ "${TARGETPLATFORM}" = "linux/i386" ] && CGO_ENABLED=0 GOARCH=386 go build -ldflags="-s -w" -trimpath ||:
RUN [ "${TARGETPLATFORM}" = "linux/arm/v5" ] && CGO_ENABLED=0 GOARCH=arm64 GOARM=5 go build -ldflags="-s -w" -trimpath ||:
RUN [ "${TARGETPLATFORM}" = "linux/arm/v6" ] && CGO_ENABLED=0 GOARCH=arm64 GOARM=6 go build -ldflags="-s -w" -trimpath ||:
RUN [ "${TARGETPLATFORM}" = "linux/arm/v7" ] && CGO_ENABLED=0 GOARCH=arm64 GOARM=7 go build -ldflags="-s -w" -trimpath ||:

FROM scratch
COPY --from=build /build/zeptohttpd /
ADD ./public /public
ADD ./config/config.json /
EXPOSE 80
CMD ["/zeptohttpd"]
