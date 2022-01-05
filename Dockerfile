FROM golang:1.17.5-alpine3.15 AS build

ADD . /app
WORKDIR /app

RUN CGO_ENABLED=0 GOOS=linux go build

FROM alpine:3.15 AS app

COPY --from=build /app .

ENTRYPOINT [ "./gimme-snowflake-creds" ]
