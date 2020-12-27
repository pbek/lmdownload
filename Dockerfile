FROM golang:alpine AS build
WORKDIR /src
COPY . .
RUN ./build.sh
FROM alpine:latest AS bin
COPY --from=build /src/bin/lmdownload /bin
RUN addgroup -g 1000 -S app && \
    adduser -u 1000 -S app -G app
USER app
RUN mkdir /home/app/pdf
WORKDIR /home/app/pdf
