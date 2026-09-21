# syntax=docker/dockerfile:1
# Imagen del API de ccp-cloud. Se construye desde la raíz del repo:
#   docker build -f deploy/ccp-cloud/Dockerfile --build-arg VERSION=cloud-v0.1.0 .
FROM golang:1.24-alpine AS build
WORKDIR /src
# Las dependencias en su propia capa: el caché sobrevive a cualquier cambio de
# código, que es lo que se toca en cada tag.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/ccp-cloud ./cmd/ccp-cloud

# distroless: sin shell ni paquetes, y sin root. Por eso el healthcheck del
# compose es el propio binario (`ccp-cloud healthcheck`) y no un curl.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ccp-cloud /ccp-cloud
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/ccp-cloud"]
