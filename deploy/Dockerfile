FROM golang:1.23-alpine AS build
ARG SERVICE
WORKDIR /src
COPY . .
RUN if [ "$SERVICE" = "analytics-service" ] || [ "$SERVICE" = "analytics-worker" ]; then go build -o /out/service ./services/${SERVICE}; else go build -o /out/service ./services/${SERVICE}/cmd/${SERVICE}; fi
FROM alpine:3.20
RUN adduser -D app
USER app
COPY --from=build /out/service /usr/local/bin/service
EXPOSE 8080 50051 50052 50053 50054 9101 9102 9103 9104
ENTRYPOINT ["/usr/local/bin/service"]
