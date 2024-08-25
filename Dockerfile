FROM golang:1.23.0-alpine3.20 AS build
WORKDIR /build/
RUN apk add --no-cache git
RUN git clone https://github.com/morststs/zeptohttpd.git
WORKDIR /build/zeptohttpd/src/
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath

FROM scratch
COPY --from=build /build/zeptohttpd/src/zeptohttpd /
COPY --from=build /build/zeptohttpd/public /public
COPY --from=build /build/zeptohttpd/config/config.json /
EXPOSE 80
CMD ["/zeptohttpd"]
