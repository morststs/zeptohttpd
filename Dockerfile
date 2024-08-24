FROM golang:1.23.0-alpine3.20 AS build
WORKDIR /build
COPY ./src /build
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath

FROM scratch
ADD zeptohttpd /
COPY --from=build /build/zeptohttpd /
ADD ./public /public
ADD config.json /
EXPOSE 80
CMD ["/zeptohttpd"]
