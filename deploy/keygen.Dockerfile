FROM golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/keygen ./services/identity-service/cmd/keygen

FROM alpine:3.20
COPY --from=build /out/keygen /usr/local/bin/keygen
ENTRYPOINT ["/usr/local/bin/keygen"]
