FROM golang:1.23-alpine AS build
ARG SERVICE
WORKDIR /src
COPY . .
RUN go build -o /out/service ./services/${SERVICE}/cmd/${SERVICE}
FROM alpine:3.20
RUN adduser -D app
USER app
COPY --from=build /out/service /usr/local/bin/service
EXPOSE 8080 50051 50052 50053
ENTRYPOINT ["/usr/local/bin/service"]
