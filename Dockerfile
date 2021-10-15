FROM golang:1.16.3-alpine3.13 AS build

ADD . /app
WORKDIR /app

RUN CGO_ENABLED=0 GOOS=linux go build

FROM alpine:3.13 AS app

COPY --from=build /app .

ENTRYPOINT [ "./gimme-snowflake-creds" ]
